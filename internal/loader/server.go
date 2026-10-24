package loader

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
)

type installerFile struct {
	URL     string `json:"url"`
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

// FabricInstallerURL returns the current Fabric installer artifact.
func FabricInstallerURL(ctx context.Context) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var entries []installerFile
	if err := httpx.GetJSON(requestCtx, FabricMeta+"/versions/installer", &entries); err != nil {
		return "", fmt.Errorf("获取 Fabric 安装器失败: %w", err)
	}
	for _, entry := range entries {
		if entry.Stable && entry.URL != "" {
			return entry.URL, nil
		}
	}
	if len(entries) > 0 && entries[0].URL != "" {
		return entries[0].URL, nil
	}
	return "", errors.New("没有可用的 Fabric 安装器")
}

// FabricServerLauncherURL returns the standalone Fabric server launcher jar.
func FabricServerLauncherURL(ctx context.Context, mcVersion, loaderVersion string) (string, error) {
	installer, err := fabricInstallerVersion(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/versions/loader/%s/%s/%s/server/jar", FabricMeta, mcVersion, loaderVersion, installer), nil
}

func fabricInstallerVersion(ctx context.Context) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var entries []installerFile
	if err := httpx.GetJSON(requestCtx, FabricMeta+"/versions/installer", &entries); err != nil {
		return "", fmt.Errorf("获取 Fabric 安装器失败: %w", err)
	}
	for _, entry := range entries {
		if entry.Stable && entry.Version != "" {
			return entry.Version, nil
		}
	}
	if len(entries) > 0 && entries[0].Version != "" {
		return entries[0].Version, nil
	}
	return "", errors.New("没有可用的 Fabric 安装器版本")
}

// QuiltInstallerURL returns the current Quilt installer artifact.
func QuiltInstallerURL(ctx context.Context) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var entries []installerFile
	if err := httpx.GetJSON(requestCtx, QuiltMeta+"/versions/installer", &entries); err != nil {
		return "", fmt.Errorf("获取 Quilt 安装器失败: %w", err)
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.URL) != "" {
			return entry.URL, nil
		}
	}
	return "", errors.New("没有可用的 Quilt 安装器")
}

// ForgeInstallerURL returns the official Forge or NeoForge server installer.
func ForgeInstallerURL(mcVersion, loaderType, loaderVersion string) (string, error) {
	return forgeInstallerURL(mcVersion, loaderType, loaderVersion)
}
