package loader

import (
	"context"
	"fmt"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
	"MyMinecraftGoLuncher/internal/mirror"
)

func (i *Installer) SetSources(sources []mirror.Source) {
	if len(sources) == 0 {
		sources = []mirror.Source{mirror.Official()}
	}
	i.sourceMu.Lock()
	i.sources = append([]mirror.Source(nil), sources...)
	i.ranked = nil
	i.rankedKey = ""
	i.rankedAt = time.Time{}
	i.sourceMu.Unlock()
}

func (i *Installer) sourceList() []mirror.Source {
	i.sourceMu.Lock()
	defer i.sourceMu.Unlock()
	if len(i.sources) == 0 {
		return []mirror.Source{mirror.Official()}
	}
	return append([]mirror.Source(nil), i.sources...)
}

func (i *Installer) rankedSources(ctx context.Context, loaderType, mcVersion string) []mirror.Source {
	loaderType = Normalize(loaderType)
	key := loaderType + ":" + mcVersion
	i.sourceMu.Lock()
	if i.rankedKey == key && time.Since(i.rankedAt) < 5*time.Minute && len(i.ranked) > 0 {
		ranked := append([]mirror.Source(nil), i.ranked...)
		i.sourceMu.Unlock()
		return ranked
	}
	sources := append([]mirror.Source(nil), i.sources...)
	i.sourceMu.Unlock()
	if len(sources) == 0 {
		sources = []mirror.Source{mirror.Official()}
	}
	resource, original := loaderProbe(loaderType, mcVersion)
	if original == "" || len(sources) < 2 {
		return sources
	}
	results := mirror.Test(ctx, sources, func(source mirror.Source) (string, bool) {
		return source.URL(resource, original, mcVersion)
	}, 4*time.Second)
	ranked := mirror.Ranked(results)
	if len(ranked) == 0 {
		ranked = sources
	}
	i.sourceMu.Lock()
	i.ranked = append([]mirror.Source(nil), ranked...)
	i.rankedKey = key
	i.rankedAt = time.Now()
	i.sourceMu.Unlock()
	return ranked
}

func loaderProbe(loaderType, mcVersion string) (mirror.Resource, string) {
	switch Normalize(loaderType) {
	case "fabric":
		return mirror.ResourceLoaderMeta, FabricMeta + "/versions/loader/" + mcVersion
	case "quilt":
		return mirror.ResourceLoaderMeta, QuiltMeta + "/versions/loader/" + mcVersion
	case "forge":
		return mirror.ResourceLoaderMeta, forgeMavenMetadata
	case "neoforge":
		return mirror.ResourceLoaderMeta, neoforgeMetadataURL(mcVersion)
	default:
		return "", ""
	}
}

func loaderCandidates(sources []mirror.Source, resource mirror.Resource, original, versionID string) []string {
	if len(sources) == 0 {
		return []string{original}
	}
	return mirror.CandidateURLs(sources, resource, original, versionID)
}

func getJSONCandidates(ctx context.Context, urls []string, out any) error {
	if len(urls) == 0 {
		return fmt.Errorf("没有可用的下载地址")
	}
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

func getRawCandidates(ctx context.Context, urls []string) ([]byte, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("没有可用的下载地址")
	}
	var lastErr error
	for _, endpoint := range urls {
		raw, err := httpx.GetRaw(ctx, endpoint)
		if err == nil {
			return raw, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func downloadCandidates(ctx context.Context, urls []string, file httpx.File) error {
	if len(urls) == 0 {
		return fmt.Errorf("没有可用的下载地址")
	}
	return httpx.DownloadAny(ctx, mirror.RankURLs(ctx, urls, 4*time.Second), file)
}
