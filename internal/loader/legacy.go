package loader

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"MyMinecraftGoLuncher/internal/mojang"
)

type legacyForgeInstall struct {
	Install *struct {
		Path        string         `json:"path"`
		FilePath    string         `json:"filePath"`
		VersionInfo map[string]any `json:"versionInfo"`
	} `json:"install"`
	JSON string `json:"json"`
}

// isLegacyForgeVersion reports whether the Forge installer uses the pre-1.13
// install_profile format. PCL2 uses the same version split (major < 20) for its
// direct legacy installation method.
func isLegacyForgeVersion(version string) bool {
	major, err := strconv.Atoi(strings.SplitN(strings.TrimSpace(version), ".", 2)[0])
	return err == nil && major > 0 && major < 20
}

// installForgeLegacy extracts the profile bundled in old Forge installers and
// installs the embedded Maven artifacts without running the installer. This is
// the fallback method used when the official installer cannot run.
func (i *Installer) installForgeLegacy(installerPath, mcVersion, loaderVersion string, progress mojang.ProgressFunc) (string, error) {
	archive, err := zip.OpenReader(installerPath)
	if err != nil {
		return "", fmt.Errorf("打开 Forge 安装器失败: %w", err)
	}
	defer archive.Close()

	raw, err := readInstallerFile(archive.File, "install_profile.json")
	if err != nil {
		return "", fmt.Errorf("读取 Forge 安装配置失败: %w", err)
	}
	var profile legacyForgeInstall
	if err := json.Unmarshal(raw, &profile); err != nil {
		return "", fmt.Errorf("解析 Forge 安装配置失败: %w", err)
	}

	profileID := mcVersion + "-forge-" + loaderVersion
	versionDir := filepath.Join(i.Paths.Versions, profileID)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return "", err
	}
	if progress != nil {
		progress("loader", "正在使用方式 B（兼容安装）安装 Forge…", 0, 1)
	}

	var versionJSON map[string]any
	if profile.Install != nil {
		// Legacy format 2: installer carries a patched Forge jar and versionInfo.
		if strings.TrimSpace(profile.Install.Path) == "" || strings.TrimSpace(profile.Install.FilePath) == "" || len(profile.Install.VersionInfo) == 0 {
			return "", fmt.Errorf("旧版 Forge 安装配置不完整")
		}
		artifactPath, err := mojang.ArtifactPath(profile.Install.Path)
		if err != nil {
			return "", fmt.Errorf("解析 Forge 运行库路径失败: %w", err)
		}
		target := filepath.Join(i.Paths.Libraries, filepath.FromSlash(artifactPath))
		if err := extractInstallerEntry(archive.File, profile.Install.FilePath, target); err != nil {
			return "", fmt.Errorf("释放 Forge 运行库失败: %w", err)
		}
		versionJSON = cloneJSONMap(profile.Install.VersionInfo)
		if versionJSON == nil {
			return "", fmt.Errorf("旧版 Forge 版本信息为空")
		}
		if strings.TrimSpace(stringValue(versionJSON["inheritsFrom"])) == "" {
			versionJSON["inheritsFrom"] = mcVersion
		}
	} else {
		// Legacy format 1: installer carries a complete version JSON and Maven tree.
		if strings.TrimSpace(profile.JSON) == "" {
			return "", fmt.Errorf("安装器不包含旧版 Forge 版本档案")
		}
		rawVersion, err := readInstallerFile(archive.File, strings.TrimPrefix(profile.JSON, "/"))
		if err != nil {
			return "", fmt.Errorf("读取旧版 Forge 版本档案失败: %w", err)
		}
		if err := json.Unmarshal(rawVersion, &versionJSON); err != nil {
			return "", fmt.Errorf("解析旧版 Forge 版本档案失败: %w", err)
		}
	}
	versionJSON["id"] = profileID
	if err := writeProfileJSON(versionDir, profileID, versionJSON); err != nil {
		return "", err
	}

	if err := extractMavenTree(archive.File, i.Paths.Libraries); err != nil {
		return "", fmt.Errorf("释放 Forge 运行库失败: %w", err)
	}
	if err := i.Mojang.MarkProfileInstalled(profileID); err != nil {
		return "", fmt.Errorf("写入 Forge 安装标记失败: %w", err)
	}
	if progress != nil {
		progress("loader", "Forge 安装完成（方式 B）", 1, 1)
	}
	return profileID, nil
}

func readInstallerFile(files []*zip.File, name string) ([]byte, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	for _, file := range files {
		if strings.EqualFold(file.Name, name) {
			reader, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer reader.Close()
			return io.ReadAll(reader)
		}
	}
	return nil, os.ErrNotExist
}

func extractInstallerEntry(files []*zip.File, name, target string) error {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	for _, file := range files {
		if !strings.EqualFold(file.Name, name) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		reader, err := file.Open()
		if err != nil {
			return err
		}
		defer reader.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, reader)
		return err
	}
	return os.ErrNotExist
}

func extractMavenTree(files []*zip.File, librariesDir string) error {
	for _, file := range files {
		if file.FileInfo().IsDir() || !strings.HasPrefix(strings.ToLower(file.Name), "maven/") {
			continue
		}
		relative := strings.TrimPrefix(file.Name[len("maven/"):], "/")
		if relative == "" {
			continue
		}
		target := filepath.Join(librariesDir, filepath.FromSlash(relative))
		cleanLibraries := filepath.Clean(librariesDir) + string(os.PathSeparator)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanLibraries) {
			continue
		}
		if err := extractZipFile(file, target); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(file *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, reader)
	return err
}

func writeProfileJSON(dir, id string, profile map[string]any) error {
	raw, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, id+".json"), raw, 0o644)
}

func cloneJSONMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil
	}
	return result
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
