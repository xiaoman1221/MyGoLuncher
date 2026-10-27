package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/loader"
)

// LoaderVersionInfo is a selectable loader build for a Minecraft version.
type LoaderVersionInfo struct {
	Version     string `json:"version"`
	Stable      bool   `json:"stable"`
	Recommended bool   `json:"recommended"`
}

// GetLoaderVersions lists loader builds that support the selected Minecraft
// version. The frontend uses this for the loader version dropdown.
func (a *App) GetLoaderVersions(loaderType, mcVersion string) ([]LoaderVersionInfo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	loaderType = loader.Normalize(loaderType)
	mcVersion = strings.TrimSpace(mcVersion)
	if loaderType == "" || loaderType == "vanilla" {
		return []LoaderVersionInfo{}, nil
	}
	if mcVersion == "" {
		return nil, errors.New("请先选择 Minecraft 版本")
	}
	ctx, cancel := context.WithTimeout(a.context(), 20*time.Second)
	defer cancel()
	versions, err := a.loaders.AvailableVersions(ctx, loaderType, mcVersion)
	if err != nil {
		return nil, err
	}
	result := make([]LoaderVersionInfo, 0, len(versions))
	for _, version := range versions {
		result = append(result, LoaderVersionInfo{
			Version: version.Version, Stable: version.Stable, Recommended: version.Recommended,
		})
	}
	return result, nil
}
