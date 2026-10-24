package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
)

// Kind identifies a download source implementation.
type Kind string

const (
	KindOfficial      Kind = "official"
	KindBMCLAPI       Kind = "bmclapi"
	KindCustomBMCLAPI Kind = "custom-bmclapi"
	KindCustomProxy   Kind = "custom-proxy"
)

// Resource identifies what kind of file is being resolved.
type Resource string

const (
	ResourceManifest   Resource = "manifest"
	ResourceVersion    Resource = "version"
	ResourceClient     Resource = "client"
	ResourceLibrary    Resource = "library"
	ResourceAsset      Resource = "asset"
	ResourceAssetIndex Resource = "asset-index"
	ResourceLogging    Resource = "logging"
)

// Source is one download origin.
type Source struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Kind    Kind   `json:"kind"`
	Custom  bool   `json:"custom"`
}

// CustomConfig is user-provided mirror configuration.
type CustomConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Kind string `json:"kind"`
}

const (
	BMCLAPIBase      = "https://bmclapi2.bangbang93.com"
	OfficialManifest = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
)

// Official is the Mojang source.
func Official() Source {
	return Source{ID: "official", Name: "Mojang 官方", Kind: KindOfficial}
}

// BMCLAPI is the built-in BMCLAPI mirror.
func BMCLAPI() Source {
	return Source{ID: "bmclapi", Name: "BMCLAPI", BaseURL: BMCLAPIBase, Kind: KindBMCLAPI}
}

// Build returns official, BMCLAPI and all valid custom sources.
func Build(customs []CustomConfig) []Source {
	sources := []Source{Official(), BMCLAPI()}
	for _, custom := range customs {
		source, err := NormalizeCustom(custom)
		if err != nil {
			continue
		}
		sources = append(sources, source)
	}
	return sources
}

// NormalizeCustom validates and normalizes one custom mirror.
func NormalizeCustom(custom CustomConfig) (Source, error) {
	raw := strings.TrimSpace(custom.URL)
	if raw == "" {
		return Source{}, errors.New("镜像地址不能为空")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return Source{}, errors.New("镜像地址格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Source{}, errors.New("镜像地址只支持 http/https")
	}
	kind := KindCustomBMCLAPI
	if strings.EqualFold(strings.TrimSpace(custom.Kind), "proxy") {
		kind = KindCustomProxy
	}
	name := strings.TrimSpace(custom.Name)
	if name == "" {
		name = parsed.Host
	}
	id := strings.TrimSpace(custom.ID)
	if id == "" {
		id = "custom-" + sanitizeID(parsed.Host)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	base := strings.TrimRight(parsed.String(), "/")
	return Source{ID: id, Name: name, BaseURL: base, Kind: kind, Custom: true}, nil
}

// Supports reports whether this source can serve a resource type.
func (s Source) Supports(resource Resource) bool {
	switch s.Kind {
	case KindOfficial:
		return true
	case KindBMCLAPI, KindCustomBMCLAPI:
		switch resource {
		case ResourceManifest, ResourceVersion, ResourceClient, ResourceLibrary, ResourceAsset:
			return true
		default:
			return false
		}
	case KindCustomProxy:
		return true
	default:
		return false
	}
}

// URL resolves an official URL through this source.
func (s Source) URL(resource Resource, original, versionID string) (string, bool) {
	if !s.Supports(resource) {
		return "", false
	}
	switch s.Kind {
	case KindOfficial:
		return original, true
	case KindCustomProxy:
		parsed, err := url.Parse(original)
		if err != nil {
			return "", false
		}
		path := parsed.EscapedPath()
		if path == "" {
			path = "/"
		}
		return strings.TrimRight(s.BaseURL, "/") + path, true
	case KindBMCLAPI, KindCustomBMCLAPI:
		base := strings.TrimRight(s.BaseURL, "/")
		switch resource {
		case ResourceManifest:
			return base + "/mc/game/version_manifest_v2.json", true
		case ResourceVersion:
			if versionID == "" {
				return "", false
			}
			return base + "/version/" + url.PathEscape(versionID) + "/json", true
		case ResourceClient:
			if versionID == "" {
				return "", false
			}
			return base + "/version/" + url.PathEscape(versionID) + "/client", true
		case ResourceLibrary:
			parsed, err := url.Parse(original)
			if err != nil || !strings.EqualFold(parsed.Hostname(), "libraries.minecraft.net") {
				return "", false
			}
			return base + "/libraries" + parsed.EscapedPath(), true
		case ResourceAsset:
			parsed, err := url.Parse(original)
			if err != nil || !strings.EqualFold(parsed.Hostname(), "resources.download.minecraft.net") {
				return "", false
			}
			return base + "/assets" + parsed.EscapedPath(), true
		}
	}
	return "", false
}

// ManifestURLs returns all candidate manifest URLs.
func ManifestURLs(sources []Source) []string {
	return urlsFor(sources, ResourceManifest, OfficialManifest, "")
}

// CandidateURLs returns the ranked source URLs for a resource, followed by the
// original official URL as a final fallback.
func CandidateURLs(sources []Source, resource Resource, original, versionID string) []string {
	urls := urlsFor(sources, resource, original, versionID)
	if !contains(urls, original) {
		urls = append(urls, original)
	}
	return urls
}

func urlsFor(sources []Source, resource Resource, original, versionID string) []string {
	seen := map[string]bool{}
	var urls []string
	for _, source := range sources {
		candidate, ok := source.URL(resource, original, versionID)
		if !ok || candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		urls = append(urls, candidate)
	}
	return urls
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// SpeedResult is one source's measured download performance.
type SpeedResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	LatencyMS   int64  `json:"latencyMs"`
	Bytes       int64  `json:"bytes"`
	BytesPerSec int64  `json:"bytesPerSec"`
	Source      Source `json:"-"`
}

// Test measures sources concurrently using a probe URL resolver.
func Test(ctx context.Context, sources []Source, probe func(Source) (string, bool), timeout time.Duration) []SpeedResult {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	results := make([]SpeedResult, len(sources))
	var wg sync.WaitGroup
	for idx, source := range sources {
		wg.Add(1)
		go func(idx int, source Source) {
			defer wg.Done()
			result := SpeedResult{ID: source.ID, Name: source.Name, Source: source}
			probeURL, ok := probe(source)
			if !ok || probeURL == "" {
				result.Error = "该源不支持测速资源"
				results[idx] = result
				return
			}
			result.URL = probeURL
			testCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			req, err := http.NewRequestWithContext(testCtx, http.MethodGet, probeURL, nil)
			if err != nil {
				result.Error = err.Error()
				results[idx] = result
				return
			}
			req.Header.Set("User-Agent", httpx.UserAgent)
			req.Header.Set("Cache-Control", "no-cache")
			req.Header.Set("Range", "bytes=0-524287")
			start := time.Now()
			resp, err := httpx.Client.Do(req)
			if err != nil {
				result.Error = err.Error()
				results[idx] = result
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
				results[idx] = result
				return
			}
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
			elapsed := time.Since(start)
			if readErr != nil {
				result.Error = readErr.Error()
				results[idx] = result
				return
			}
			if len(data) == 0 {
				result.Error = "未返回数据"
				results[idx] = result
				return
			}
			result.OK = true
			result.Bytes = int64(len(data))
			result.LatencyMS = elapsed.Milliseconds()
			result.BytesPerSec = int64(float64(len(data)) / elapsed.Seconds())
			results[idx] = result
		}(idx, source)
	}
	wg.Wait()
	sortResults(results)
	return results
}

func sortResults(results []SpeedResult) {
	// Stable insertion keeps the source order for ties.
	for i := 1; i < len(results); i++ {
		current := results[i]
		j := i - 1
		for j >= 0 && better(current, results[j]) {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = current
	}
}

func better(a, b SpeedResult) bool {
	if a.OK != b.OK {
		return a.OK
	}
	if a.BytesPerSec != b.BytesPerSec {
		return a.BytesPerSec > b.BytesPerSec
	}
	return a.LatencyMS < b.LatencyMS
}

// Ranked returns successful sources in measured order. If no source passed,
// the original order is returned so official fallback still works.
func Ranked(results []SpeedResult) []Source {
	var ranked []Source
	for _, result := range results {
		if result.OK {
			ranked = append(ranked, result.Source)
		}
	}
	if len(ranked) == 0 {
		for _, result := range results {
			ranked = append(ranked, result.Source)
		}
	}
	return ranked
}

// HumanSpeed formats a bytes-per-second measurement.
func HumanSpeed(bytesPerSec int64) string {
	const unit = 1024
	if bytesPerSec < unit {
		return fmt.Sprintf("%d B/s", bytesPerSec)
	}
	value := float64(bytesPerSec)
	units := []string{"KB/s", "MB/s", "GB/s"}
	for _, name := range units {
		value /= unit
		if value < unit || name == "GB/s" {
			return fmt.Sprintf("%.1f %s", value, name)
		}
	}
	return fmt.Sprintf("%d B/s", bytesPerSec)
}

func sanitizeID(value string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(value) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		} else if builder.Len() > 0 {
			builder.WriteByte('-')
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "mirror"
	}
	return result
}
