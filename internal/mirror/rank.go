package mirror

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
)

type rankedURL struct {
	url       string
	ok        bool
	bytes     int64
	latencyMS int64
	bps       int64
	index     int
}

// RankURLs measures mirror candidates with a small range request and returns
// successful URLs from fastest to slowest. Failed candidates stay at the end in
// their original order, so the official fallback is never lost.
func RankURLs(ctx context.Context, urls []string, timeout time.Duration) []string {
	if len(urls) < 2 {
		return append([]string(nil), urls...)
	}
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	unique := make([]string, 0, len(urls))
	seen := map[string]bool{}
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true
		unique = append(unique, raw)
	}
	if len(unique) < 2 {
		return unique
	}
	results := make([]rankedURL, len(unique))
	var wg sync.WaitGroup
	for idx, endpoint := range unique {
		wg.Add(1)
		go func(idx int, endpoint string) {
			defer wg.Done()
			results[idx] = rankOneURL(ctx, endpoint, idx, timeout)
		}(idx, endpoint)
	}
	wg.Wait()

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].ok != results[j].ok {
			return results[i].ok
		}
		if results[i].bps != results[j].bps {
			return results[i].bps > results[j].bps
		}
		return results[i].latencyMS < results[j].latencyMS
	})
	out := make([]string, 0, len(results))
	for _, result := range results {
		out = append(out, result.url)
	}
	return out
}

func rankOneURL(ctx context.Context, endpoint string, index int, timeout time.Duration) rankedURL {
	result := rankedURL{url: endpoint, index: index}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result
	}
	req.Header.Set("User-Agent", httpx.UserAgent)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Range", "bytes=0-262143")
	start := time.Now()
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	elapsed := time.Since(start)
	if err != nil || len(data) == 0 {
		return result
	}
	result.ok = true
	result.bytes = int64(len(data))
	result.latencyMS = elapsed.Milliseconds()
	if elapsed > 0 {
		result.bps = int64(float64(len(data)) / elapsed.Seconds())
	}
	return result
}
