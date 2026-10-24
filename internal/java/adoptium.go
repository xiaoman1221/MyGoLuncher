package java

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
	"MyMinecraftGoLuncher/internal/mirror"
	"MyMinecraftGoLuncher/internal/util"
)

const adoptiumAPI = "https://api.adoptium.net/v3"

// ReleaseInfo describes Adoptium's available Java feature versions.
type ReleaseInfo struct {
	Available     []int `json:"available"`
	LTS           []int `json:"lts"`
	LatestFeature int   `json:"latestFeature"`
}

// Archive is a downloadable Adoptium JRE archive.
type Archive struct {
	Feature     int    `json:"feature"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Version     string `json:"version"`
	ReleaseName string `json:"releaseName"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Installed   bool   `json:"installed"`
}

// ProgressFunc receives install progress.
type ProgressFunc func(stage, message string, current, total int64)

type releaseResponse struct {
	AvailableLTS      []int `json:"available_lts_releases"`
	AvailableReleases []int `json:"available_releases"`
	MostRecentFeature int   `json:"most_recent_feature_release"`
	MostRecentLTS     int   `json:"most_recent_lts"`
}

type assetResponse struct {
	Binary struct {
		OS      string `json:"os"`
		Arch    string `json:"arch"`
		Package struct {
			Name     string `json:"name"`
			Link     string `json:"link"`
			Checksum string `json:"checksum"`
			Size     int64  `json:"size"`
		} `json:"package"`
	} `json:"binary"`
	Version struct {
		Semver string `json:"semver"`
	} `json:"version"`
	ReleaseName string `json:"release_name"`
}

// Releases queries the currently published feature versions.
func (m *Manager) Releases(ctx context.Context) (ReleaseInfo, error) {
	var raw releaseResponse
	if err := httpx.GetJSON(ctx, adoptiumAPI+"/info/available_releases", &raw); err != nil {
		return ReleaseInfo{}, err
	}
	lts := append([]int{}, raw.AvailableLTS...)
	sort.Ints(lts)
	return ReleaseInfo{
		Available:     raw.AvailableReleases,
		LTS:           lts,
		LatestFeature: raw.MostRecentFeature,
	}, nil
}

// PackageInfo resolves the latest JRE archive for a feature version.
func (m *Manager) PackageInfo(ctx context.Context, feature int) (Archive, error) {
	if feature < 8 || feature > 99 {
		return Archive{}, errors.New("不支持的 Java 版本")
	}
	osName, arch, err := adoptiumPlatform()
	if err != nil {
		return Archive{}, err
	}
	q := url.Values{}
	q.Set("os", osName)
	q.Set("architecture", arch)
	q.Set("image_type", "jre")
	q.Set("vendor", "eclipse")
	endpoint := fmt.Sprintf("%s/assets/latest/%d/hotspot?%s", adoptiumAPI, feature, q.Encode())
	var assets []assetResponse
	if err := getJSONMirrors(ctx, mirror.CandidateURLs(m.sourceList(), mirror.ResourceJavaMeta, endpoint, ""), &assets); err != nil {
		return Archive{}, fmt.Errorf("查询 Java %d 失败: %w", feature, err)
	}
	if len(assets) == 0 || assets[0].Binary.Package.Link == "" {
		return Archive{}, fmt.Errorf("Adoptium 暂未提供适用于当前平台的 Java %d", feature)
	}
	a := assets[0]
	pkg := Archive{
		Feature:     feature,
		Name:        a.Binary.Package.Name,
		URL:         a.Binary.Package.Link,
		SHA256:      a.Binary.Package.Checksum,
		Size:        a.Binary.Package.Size,
		Version:     a.Version.Semver,
		ReleaseName: a.ReleaseName,
		OS:          osName,
		Arch:        arch,
	}
	return pkg, nil
}

// Install downloads and extracts the latest JRE for a feature version.
func (m *Manager) Install(ctx context.Context, feature int, progress ProgressFunc) (Runtime, error) {
	for _, rt := range m.Managed(ctx) {
		if rt.Major == feature {
			return rt, nil
		}
	}
	pkg, err := m.PackageInfo(ctx, feature)
	if err != nil {
		return Runtime{}, err
	}
	if pkg.Installed {
		for _, rt := range m.Managed(ctx) {
			if rt.Major == feature {
				return rt, nil
			}
		}
	}
	if err := os.MkdirAll(m.paths.Runtimes, 0o755); err != nil {
		return Runtime{}, err
	}
	archiveName := filepath.Base(pkg.Name)
	if archiveName == "" {
		archiveName = fmt.Sprintf("temurin-%d", feature)
	}
	archivePath := filepath.Join(m.paths.Cache, "java", archiveName)
	if progress != nil {
		progress("download", fmt.Sprintf("正在下载 Temurin Java %d…", feature), 0, pkg.Size)
	}
	urls := mirror.RankURLs(ctx, mirror.CandidateURLs(m.sourceList(), mirror.ResourceJavaArtifact, pkg.URL, strconv.Itoa(feature)), 4*time.Second)
	err = httpx.DownloadAny(ctx, urls, httpx.File{
		Dest:   archivePath,
		SHA256: pkg.SHA256,
		Size:   pkg.Size,
		Progress: func(done, total int64) {
			if progress != nil {
				progress("download", fmt.Sprintf("正在下载 Temurin Java %d…", feature), done, total)
			}
		},
	})
	if err != nil {
		return Runtime{}, fmt.Errorf("下载 Java 失败: %w", err)
	}
	if progress != nil {
		progress("extract", "正在解压 Java 运行时…", 0, 1)
	}
	tempDir, err := os.MkdirTemp(m.paths.Runtimes, ".install-*")
	if err != nil {
		return Runtime{}, err
	}
	defer os.RemoveAll(tempDir)
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		err = util.ExtractZip(archivePath, tempDir, "")
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		err = util.ExtractTarGz(archivePath, tempDir)
	default:
		err = fmt.Errorf("不支持的 Java 归档格式: %s", filepath.Base(archivePath))
	}
	if err != nil {
		return Runtime{}, fmt.Errorf("解压 Java 失败: %w", err)
	}
	exe, err := findJavaExecutable(tempDir)
	if err != nil {
		return Runtime{}, err
	}
	home := filepath.Dir(filepath.Dir(exe))
	target := filepath.Join(m.paths.Runtimes, fmt.Sprintf("temurin-%d", feature))
	if existing, probeErr := Probe(ctx, target); probeErr == nil && existing.Major == feature {
		return existing, nil
	}
	_ = os.RemoveAll(target)
	if err := moveDir(home, target); err != nil {
		return Runtime{}, fmt.Errorf("安装 Java 失败: %w", err)
	}
	rt, err := Probe(ctx, target)
	if err != nil {
		return Runtime{}, fmt.Errorf("Java 安装后检测失败: %w", err)
	}
	rt.Managed = true
	rt.Source = "managed"
	meta := map[string]any{
		"feature":     feature,
		"version":     rt.Version,
		"package":     pkg.Name,
		"installedAt": time.Now(),
	}
	_ = util.WriteJSON(filepath.Join(target, ".managed.json"), meta)
	if progress != nil {
		progress("done", fmt.Sprintf("Java %d 安装完成", feature), 1, 1)
	}
	return rt, nil
}

func findJavaExecutable(root string) (string, error) {
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if strings.Count(rel, string(filepath.Separator)) > 5 {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(d.Name())
		if (lower == "java.exe" || lower == "java") && filepath.Base(filepath.Dir(path)) == "bin" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" {
		return "", errors.New("Java 归档中未找到 java 可执行文件")
	}
	return found, nil
}

func moveDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Windows antivirus can briefly lock files; fall back to a copy.
	if err := util.CopyDirTree(src, dst); err != nil {
		return err
	}
	return nil
}

func getJSONMirrors(ctx context.Context, urls []string, out any) error {
	if len(urls) == 0 {
		return errors.New("没有可用的 Java 下载地址")
	}
	urls = mirror.RankURLs(ctx, urls, 3*time.Second)
	var lastErr error
	for _, endpoint := range urls {
		if err := httpx.GetJSON(ctx, endpoint, out); err == nil {
			return nil
		} else {
			lastErr = err
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	return lastErr
}

func adoptiumPlatform() (string, string, error) {
	var osName string
	switch runtime.GOOS {
	case "windows":
		osName = "windows"
	case "linux":
		osName = "linux"
	case "darwin":
		osName = "mac"
	default:
		return "", "", fmt.Errorf("Adoptium 不支持当前系统: %s", runtime.GOOS)
	}
	arch := ""
	switch runtime.GOARCH {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "aarch64"
	case "386":
		arch = "x86"
	default:
		return "", "", fmt.Errorf("Adoptium 不支持当前架构: %s", runtime.GOARCH)
	}
	return osName, arch, nil
}

// ParseFeature is a small helper for UI-supplied version strings.
func ParseFeature(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 8 {
		return 0, errors.New("请输入有效的 Java 大版本号（如 21）")
	}
	return n, nil
}
