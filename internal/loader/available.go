package loader

import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"MyMinecraftGoLuncher/internal/mirror"
)

// VersionInfo is one selectable loader build for a Minecraft version.
type VersionInfo struct {
	Version     string `json:"version"`
	Stable      bool   `json:"stable"`
	Recommended bool   `json:"recommended"`
}

// AvailableVersions returns loader builds that explicitly support mcVersion.
// Fabric/Quilt are filtered by their metadata endpoints; Forge/NeoForge are
// filtered from Maven metadata and sorted newest first.
func (i *Installer) AvailableVersions(ctx context.Context, loaderType, mcVersion string) ([]VersionInfo, error) {
	loaderType = Normalize(loaderType)
	mcVersion = strings.TrimSpace(mcVersion)
	if loaderType == "" || loaderType == "vanilla" {
		return []VersionInfo{}, nil
	}
	if mcVersion == "" {
		return nil, fmt.Errorf("请先选择 Minecraft 版本")
	}
	sources := i.rankedSources(ctx, loaderType, mcVersion)
	switch loaderType {
	case "fabric", "quilt":
		base := FabricMeta
		if loaderType == "quilt" {
			base = QuiltMeta
		}
		endpoint := fmt.Sprintf("%s/versions/loader/%s", base, mcVersion)
		var entries []fabricLoaderEntry
		if err := getJSONCandidates(ctx, loaderCandidates(sources, mirror.ResourceLoaderMeta, endpoint, mcVersion), &entries); err != nil {
			return nil, fmt.Errorf("获取加载器版本列表失败: %w", err)
		}
		result := make([]VersionInfo, 0, len(entries))
		seen := map[string]bool{}
		for _, entry := range entries {
			version := strings.TrimSpace(entry.Loader.Version)
			if version == "" || seen[version] {
				continue
			}
			seen[version] = true
			result = append(result, VersionInfo{Version: version, Stable: entry.Loader.Stable})
		}
		sortLoaderVersions(result)
		if len(result) == 0 {
			return nil, fmt.Errorf("%s 没有适用于 %s 的版本", displayLoader(loaderType), mcVersion)
		}
		return result, nil
	case "forge":
		return forgeVersions(ctx, sources, mcVersion)
	case "neoforge":
		return neoForgeVersions(ctx, sources, mcVersion)
	default:
		return nil, fmt.Errorf("不支持的加载器: %s", loaderType)
	}
}

func forgeVersions(ctx context.Context, sources []mirror.Source, mcVersion string) ([]VersionInfo, error) {
	promos, _ := forgePromotions(ctx, sources)
	raw, err := getRawCandidates(ctx, loaderCandidates(sources, mirror.ResourceLoaderMeta, forgeMavenMetadata, mcVersion))
	if err == nil {
		var metadata mavenMetadata
		if xmlErr := xml.Unmarshal(raw, &metadata); xmlErr == nil {
			result := parseMavenLoaderVersions(metadata.Versioning.Versions, mcVersion, false, promos)
			if len(result) > 0 {
				return result, nil
			}
		}
	}
	fallback := promotionLoaderVersions(promos, mcVersion)
	if len(fallback) > 0 {
		return fallback, nil
	}
	if err != nil {
		return nil, fmt.Errorf("获取 Forge 版本列表失败: %w", err)
	}
	return nil, fmt.Errorf("Forge 没有适用于 %s 的版本", mcVersion)
}

func neoForgeVersions(ctx context.Context, sources []mirror.Source, mcVersion string) ([]VersionInfo, error) {
	raw, err := getRawCandidates(ctx, loaderCandidates(sources, mirror.ResourceLoaderMeta, neoforgeMetadataURL(mcVersion), mcVersion))
	if err != nil {
		return nil, fmt.Errorf("获取 NeoForge 版本列表失败: %w", err)
	}
	var metadata mavenMetadata
	if err := xml.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("解析 NeoForge 版本列表失败: %w", err)
	}
	result := parseMavenLoaderVersions(metadata.Versioning.Versions, mcVersion, true, nil)
	if len(result) == 0 {
		return nil, fmt.Errorf("NeoForge 没有适用于 %s 的版本", mcVersion)
	}
	return result, nil
}

func parseMavenLoaderVersions(values []string, mcVersion string, neoForge bool, promos map[string]string) []VersionInfo {
	prefix := mcVersion + "-"
	neoPrefix := strings.TrimPrefix(mcVersion, "1.") + "."
	legacyNeo := neoForge && mcVersion == "1.20.1"
	recommended := ""
	latest := ""
	if promos != nil {
		recommended = promos[mcVersion+"-recommended"]
		latest = promos[mcVersion+"-latest"]
	}
	result := make([]VersionInfo, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		version := ""
		if legacyNeo {
			if !strings.HasPrefix(value, prefix) {
				continue
			}
			version = strings.TrimPrefix(value, prefix)
		} else if neoForge {
			if !strings.HasPrefix(value, neoPrefix) {
				continue
			}
			version = value
		} else {
			if !strings.HasPrefix(value, prefix) {
				continue
			}
			version = strings.TrimPrefix(value, prefix)
		}
		if version == "" || seen[version] {
			continue
		}
		lower := strings.ToLower(version)
		if strings.Contains(lower, "beta") || strings.Contains(lower, "alpha") || strings.Contains(lower, "rc") {
			continue
		}
		seen[version] = true
		result = append(result, VersionInfo{
			Version:     version,
			Stable:      true,
			Recommended: version == recommended || version == latest,
		})
	}
	sortLoaderVersions(result)
	return result
}

func promotionLoaderVersions(promos map[string]string, mcVersion string) []VersionInfo {
	if promos == nil {
		return nil
	}
	result := []VersionInfo{}
	seen := map[string]bool{}
	for _, key := range []string{"latest", "recommended"} {
		version := strings.TrimSpace(promos[mcVersion+"-"+key])
		if version == "" || seen[version] {
			continue
		}
		seen[version] = true
		result = append(result, VersionInfo{Version: version, Stable: true, Recommended: true})
	}
	sortLoaderVersions(result)
	return result
}

func sortLoaderVersions(values []VersionInfo) {
	sort.SliceStable(values, func(i, j int) bool {
		return compareVersions(values[i].Version, values[j].Version) > 0
	})
}
