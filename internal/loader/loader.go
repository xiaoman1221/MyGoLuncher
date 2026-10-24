package loader

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
	"MyMinecraftGoLuncher/internal/mirror"
	"MyMinecraftGoLuncher/internal/mojang"
	"MyMinecraftGoLuncher/internal/store"
	"MyMinecraftGoLuncher/internal/sysutil"
	"MyMinecraftGoLuncher/internal/util"
)

const (
	FabricMeta         = "https://meta.fabricmc.net/v2"
	QuiltMeta          = "https://meta.quiltmc.org/v3"
	forgePromotionsURL = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"
	forgeMavenMetadata = "https://maven.minecraftforge.net/net/minecraftforge/forge/maven-metadata.xml"
	neoforgeMavenBase  = "https://maven.neoforged.net/releases/net/neoforged/neoforge"
	neoforgeLegacyBase = "https://maven.neoforged.net/releases/net/neoforged/forge"
)

// Installer coordinates mod loader profile creation.
type Installer struct {
	Mojang *mojang.Client
	Paths  store.Paths

	sourceMu  sync.Mutex
	sources   []mirror.Source
	ranked    []mirror.Source
	rankedKey string
	rankedAt  time.Time
}

// New creates a loader installer.
func New(client *mojang.Client, paths store.Paths) *Installer {
	return &Installer{
		Mojang: client, Paths: paths,
		sources: []mirror.Source{mirror.Official()},
	}
}

type fabricLoader struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

type fabricLoaderEntry struct {
	Loader fabricLoader `json:"loader"`
}

// LatestVersion resolves the newest stable loader for a game version using the
// official source. Use Installer.LatestVersion to honour configured mirrors.
func LatestVersion(ctx context.Context, loaderType, mcVersion string) (string, error) {
	return latestVersionWithSources(ctx, nil, loaderType, mcVersion)
}

// LatestVersion resolves the newest stable loader through the ranked mirror
// sources for the current loader.
func (i *Installer) LatestVersion(ctx context.Context, loaderType, mcVersion string) (string, error) {
	return latestVersionWithSources(ctx, i.rankedSources(ctx, loaderType, mcVersion), loaderType, mcVersion)
}

// LatestVersionWithSources resolves a loader version through explicit mirror
// sources. It is exported for server-pack/runtime exports.
func LatestVersionWithSources(ctx context.Context, sources []mirror.Source, loaderType, mcVersion string) (string, error) {
	return latestVersionWithSources(ctx, sources, loaderType, mcVersion)
}

func latestVersionWithSources(ctx context.Context, sources []mirror.Source, loaderType, mcVersion string) (string, error) {
	switch Normalize(loaderType) {
	case "fabric":
		var entries []fabricLoaderEntry
		endpoint := fmt.Sprintf("%s/versions/loader/%s", FabricMeta, mcVersion)
		urls := loaderCandidates(sources, mirror.ResourceLoaderMeta, endpoint, mcVersion)
		if err := getJSONCandidates(ctx, urls, &entries); err != nil {
			return "", err
		}
		for _, e := range entries {
			if e.Loader.Stable {
				return e.Loader.Version, nil
			}
		}
		if len(entries) > 0 {
			return entries[0].Loader.Version, nil
		}
	case "quilt":
		var entries []fabricLoaderEntry
		endpoint := fmt.Sprintf("%s/versions/loader/%s", QuiltMeta, mcVersion)
		urls := loaderCandidates(sources, mirror.ResourceLoaderMeta, endpoint, mcVersion)
		if err := getJSONCandidates(ctx, urls, &entries); err != nil {
			return "", err
		}
		for _, e := range entries {
			v := strings.ToLower(e.Loader.Version)
			if !strings.Contains(v, "beta") && !strings.Contains(v, "alpha") && !strings.Contains(v, "rc") {
				return e.Loader.Version, nil
			}
		}
		if len(entries) > 0 {
			return entries[0].Loader.Version, nil
		}
	case "forge":
		promotions, err := forgePromotions(ctx, sources)
		if err != nil {
			return "", err
		}
		if v := promotions[mcVersion+"-latest"]; v != "" {
			return v, nil
		}
		if v := promotions[mcVersion+"-recommended"]; v != "" {
			return v, nil
		}
		return "", fmt.Errorf("Forge 没有适用于 %s 的版本", mcVersion)
	case "neoforge":
		return latestNeoForge(ctx, sources, mcVersion)
	default:
		return "", errors.New("该加载器不支持自动选择版本")
	}
	return "", errors.New("没有找到可用的加载器版本")
}

type mavenMetadata struct {
	Versioning struct {
		Versions []string `xml:"versions>version"`
	} `xml:"versioning"`
}

func neoforgeMetadataURL(mcVersion string) string {
	base := neoforgeMavenBase
	if mcVersion == "1.20.1" {
		base = neoforgeLegacyBase
	}
	return base + "/maven-metadata.xml"
}

func latestNeoForge(ctx context.Context, sources []mirror.Source, mcVersion string) (string, error) {
	prefix := strings.TrimPrefix(mcVersion, "1.") + "."
	legacy := mcVersion == "1.20.1"
	endpoint := neoforgeMetadataURL(mcVersion)
	raw, err := getRawCandidates(ctx, loaderCandidates(sources, mirror.ResourceLoaderMeta, endpoint, mcVersion))
	if err != nil {
		return "", fmt.Errorf("无法获取 NeoForge 版本列表: %w", err)
	}
	var metadata mavenMetadata
	if err := xml.Unmarshal(raw, &metadata); err != nil {
		return "", fmt.Errorf("解析 NeoForge 版本列表失败: %w", err)
	}
	candidates := make([]string, 0, len(metadata.Versioning.Versions))
	for _, value := range metadata.Versioning.Versions {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "beta") || strings.Contains(lower, "alpha") || strings.Contains(lower, "rc") {
			continue
		}
		if legacy {
			if strings.HasPrefix(value, mcVersion+"-") {
				candidates = append(candidates, strings.TrimPrefix(value, mcVersion+"-"))
			}
		} else if strings.HasPrefix(value, prefix) {
			candidates = append(candidates, value)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("NeoForge 没有适用于 %s 的版本，请手动填写", mcVersion)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return compareVersions(candidates[i], candidates[j]) > 0
	})
	return candidates[0], nil
}

func compareVersions(left, right string) int {
	split := func(value string) []int {
		fields := strings.FieldsFunc(value, func(r rune) bool {
			return r == '.' || r == '-' || r == '+'
		})
		numbers := make([]int, 0, len(fields))
		for _, field := range fields {
			number, _ := strconv.Atoi(field)
			numbers = append(numbers, number)
		}
		return numbers
	}
	a, b := split(left), split(right)
	for i := 0; i < len(a) || i < len(b); i++ {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return strings.Compare(left, right)
}

func forgePromotions(ctx context.Context, sources []mirror.Source) (map[string]string, error) {
	var raw struct {
		Promos map[string]string `json:"promos"`
	}
	if err := getJSONCandidates(ctx, loaderCandidates(sources, mirror.ResourceLoaderMeta, forgePromotionsURL, ""), &raw); err != nil {
		return nil, err
	}
	return raw.Promos, nil
}

// Ensure creates (if necessary) and installs the loader profile, returning the
// version ID that should be launched. javaPath is only needed for Forge and
// NeoForge installers.
func (i *Installer) Ensure(ctx context.Context, mcVersion, loaderType, loaderVersion, javaPath string, workers int, progress mojang.ProgressFunc) (string, error) {
	loaderType = Normalize(loaderType)
	if loaderType == "" || loaderType == "vanilla" {
		return mcVersion, nil
	}
	if !i.Mojang.IsInstalled(mcVersion) {
		if err := i.Mojang.Install(ctx, mcVersion, workers, progress); err != nil {
			return "", fmt.Errorf("安装原版游戏失败: %w", err)
		}
	}
	sources := i.rankedSources(ctx, loaderType, mcVersion)
	if loaderVersion == "" {
		var err error
		loaderVersion, err = latestVersionWithSources(ctx, sources, loaderType, mcVersion)
		if err != nil {
			return "", err
		}
	}
	switch loaderType {
	case "fabric", "quilt":
		profile, err := i.fetchProfile(ctx, sources, loaderType, mcVersion, loaderVersion)
		if err != nil {
			return "", err
		}
		if i.Mojang.IsProfileInstalled(profile.ID) {
			return profile.ID, nil
		}
		if err := i.Mojang.InstallProfileWithSources(ctx, profile, workers, progress, sources); err != nil {
			return "", fmt.Errorf("安装加载器失败: %w", err)
		}
		return profile.ID, nil
	case "forge", "neoforge":
		return i.installForge(ctx, sources, mcVersion, loaderType, loaderVersion, javaPath, progress)
	default:
		return "", fmt.Errorf("不支持的加载器: %s", loaderType)
	}
}

func (i *Installer) fetchProfile(ctx context.Context, sources []mirror.Source, loaderType, mcVersion, loaderVersion string) (mojang.Version, error) {
	base := FabricMeta
	if loaderType == "quilt" {
		base = QuiltMeta
	}
	endpoint := fmt.Sprintf("%s/versions/loader/%s/%s/profile/json", base, mcVersion, loaderVersion)
	var profile mojang.Version
	if err := getJSONCandidates(ctx, loaderCandidates(sources, mirror.ResourceLoaderMeta, endpoint, mcVersion), &profile); err != nil {
		return mojang.Version{}, fmt.Errorf("获取 %s 安装档案失败: %w", strings.ToUpper(loaderType[:1])+loaderType[1:], err)
	}
	if profile.ID == "" {
		return mojang.Version{}, errors.New("加载器档案无效")
	}
	if profile.InheritsFrom == "" {
		profile.InheritsFrom = mcVersion
	}
	return profile, nil
}

func (i *Installer) installForge(ctx context.Context, sources []mirror.Source, mcVersion, loaderType, loaderVersion, javaPath string, progress mojang.ProgressFunc) (string, error) {
	if strings.TrimSpace(javaPath) == "" {
		return "", errors.New("安装 Forge/NeoForge 需要可用的 Java")
	}
	loaderVersion = cleanLoaderVersion(loaderVersion, mcVersion, loaderType)
	installerURL, err := forgeInstallerURL(mcVersion, loaderType, loaderVersion)
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(i.Paths.Cache, "installers")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	installerPath := filepath.Join(cacheDir, filepath.Base(installerURL))
	if progress != nil {
		progress("loader", "正在下载 "+displayLoader(loaderType)+" 安装器…", 0, 1)
	}
	if err := downloadCandidates(ctx, loaderCandidates(sources, mirror.ResourceLoaderArtifact, installerURL, mcVersion), httpx.File{
		Dest: installerPath, Size: 0,
	}); err != nil {
		return "", fmt.Errorf("下载加载器安装器失败: %w", err)
	}
	// Forge's installer is run from the data root so libraries and version
	// profiles land in the same directories used by our launcher.
	profilesPath := filepath.Join(i.Paths.Data, "launcher_profiles.json")
	if !util.Exists(profilesPath) {
		_ = util.WriteJSON(profilesPath, map[string]any{
			"profiles": map[string]any{},
			"settings": map[string]any{},
			"version":  3,
		})
	}
	runCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	display := displayLoader(loaderType)
	if progress != nil {
		progress("loader", "正在执行 "+display+" 安装器（可能需要数分钟）…", 0, 1)
	}
	cmd := exec.CommandContext(runCtx, javaPath, "-jar", installerPath, "--installClient", i.Paths.Data)
	cmd.Dir = i.Paths.Data
	sysutil.HideWindow(cmd)
	output, runErr := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if runCtx.Err() == context.DeadlineExceeded {
		return "", errors.New(display + " 安装超时")
	}
	if runErr != nil {
		return "", fmt.Errorf("%s 安装失败: %v\n%s", display, runErr, tailString(text, 1800))
	}
	id := findInstalledProfile(i.Paths.Versions, mcVersion, loaderType, loaderVersion)
	if id == "" {
		return "", fmt.Errorf("%s 安装器未生成版本档案\n%s", display, tailString(text, 1200))
	}
	if progress != nil {
		progress("loader", display+" 安装完成", 1, 1)
	}
	return id, nil
}

func forgeInstallerURL(mcVersion, loaderType, loaderVersion string) (string, error) {
	switch loaderType {
	case "forge":
		artifact := mcVersion + "-" + loaderVersion
		return fmt.Sprintf("https://maven.minecraftforge.net/net/minecraftforge/forge/%s/forge-%s-installer.jar", artifact, artifact), nil
	case "neoforge":
		// NeoForge used the legacy "forge" artifact for Minecraft 1.20.1 and
		// switched to standalone neoforge coordinates for 1.20.2+.
		if mcVersion == "1.20.1" || strings.HasPrefix(loaderVersion, "47.") {
			legacyArtifact := mcVersion + "-" + loaderVersion
			return fmt.Sprintf("https://maven.neoforged.net/releases/net/neoforged/forge/%s/forge-%s-installer.jar", legacyArtifact, legacyArtifact), nil
		}
		return fmt.Sprintf("https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar", loaderVersion, loaderVersion), nil
	}
	return "", errors.New("未知加载器")
}

func findInstalledProfile(versionsDir, mcVersion, loaderType, loaderVersion string) string {
	candidates := []string{}
	switch loaderType {
	case "forge":
		candidates = append(candidates, mcVersion+"-forge-"+loaderVersion, mcVersion+"-forge"+loaderVersion, "forge-"+loaderVersion)
	case "neoforge":
		candidates = append(candidates, "neoforge-"+loaderVersion, mcVersion+"-neoforge-"+loaderVersion)
	}
	for _, id := range candidates {
		if util.NonEmptyFile(filepath.Join(versionsDir, id, id+".json")) {
			return id
		}
	}
	// Fall back to scanning profiles whose ID references the loader version.
	entries, _ := os.ReadDir(versionsDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if !strings.Contains(id, loaderVersion) {
			continue
		}
		lower := strings.ToLower(id)
		if loaderType == "forge" && !strings.Contains(lower, "forge") {
			continue
		}
		if loaderType == "neoforge" && !strings.Contains(lower, "neoforge") {
			continue
		}
		if util.NonEmptyFile(filepath.Join(versionsDir, id, id+".json")) {
			return id
		}
	}
	return ""
}

func cleanLoaderVersion(version, mcVersion, loaderType string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, loaderType+"-")
	version = strings.TrimPrefix(version, "forge-")
	if strings.HasPrefix(version, mcVersion+"-") {
		version = strings.TrimPrefix(version, mcVersion+"-")
	}
	return version
}

func displayLoader(t string) string {
	if t == "neoforge" {
		return "NeoForge"
	}
	return "Forge"
}

func tailString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…" + s[len(s)-max:]
}

// Normalize translates UI labels to internal loader identifiers.
func Normalize(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fabric":
		return "fabric"
	case "quilt":
		return "quilt"
	case "forge":
		return "forge"
	case "neoforge", "neo-forge", "neo forge":
		return "neoforge"
	case "vanilla", "原版", "":
		return "vanilla"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// ProfileMetadata is a convenience payload for the UI.
type ProfileMetadata struct {
	ID      string `json:"id"`
	MC      string `json:"mc"`
	Loader  string `json:"loader"`
	Version string `json:"version"`
}

// Metadata returns basic information about a generated profile.
func Metadata(id, mc, loaderType, version string) ProfileMetadata {
	return ProfileMetadata{ID: id, MC: mc, Loader: loaderType, Version: version}
}
