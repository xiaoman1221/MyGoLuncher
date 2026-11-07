package httpx

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const UserAgent = "MyGoLuncher/1.0"

// downloadIdleTimeout aborts a download when the server stops sending data.
// The overall duration is intentionally not capped: large runtimes and
// modpacks can legitimately take a long time on slow links.
const downloadIdleTimeout = 60 * time.Second

var Client = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   24,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
	},
}

// GetJSON fetches and decodes JSON with a bounded response size.
func GetJSON(ctx context.Context, url string, out any) error {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("服务返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 64<<20))
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	return nil
}

// GetRaw fetches a small body (used for cached manifests).
func GetRaw(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("服务返回 HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// File describes a remote file to download.
type File struct {
	URL      string
	Dest     string
	SHA1     string
	SHA256   string
	SHA512   string
	Size     int64
	Verify   bool // verify existing files instead of trusting dimensions
	Attempts int  // per-URL attempts; 0 means 3
	Progress func(done, total int64)
}

// Download fetches one file atomically, optionally verifying a checksum.
func Download(ctx context.Context, f File) error {
	if f.URL == "" {
		return errors.New("下载地址为空")
	}
	if utilExists(f.Dest) {
		if f.Verify {
			if ok, _ := verifyFile(f.Dest, f.SHA1, f.SHA256, f.SHA512); ok {
				if f.Progress != nil {
					f.Progress(fileSize(f.Dest), fileSize(f.Dest))
				}
				return nil
			}
		} else if info, err := os.Stat(f.Dest); err == nil && (f.Size <= 0 || info.Size() == f.Size) {
			if f.Progress != nil {
				f.Progress(info.Size(), info.Size())
			}
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(f.Dest), 0o755); err != nil {
		return err
	}

	attempts := f.Attempts
	if attempts <= 0 {
		attempts = 3
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 700 * time.Millisecond):
			}
		}
		lastErr = downloadAttempt(ctx, f)
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("下载失败 %s: %w", filepath.Base(f.Dest), lastErr)
}

// DownloadAny tries multiple URLs in order, falling over to the next mirror
// when one fails. The destination and checksums are shared between attempts.
func DownloadAny(ctx context.Context, urls []string, f File) error {
	seen := map[string]bool{}
	var lastErr error
	for _, rawURL := range urls {
		if rawURL == "" || seen[rawURL] {
			continue
		}
		seen[rawURL] = true
		next := f
		next.URL = rawURL
		next.Attempts = 2
		if err := Download(ctx, next); err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("没有可用的下载地址")
	}
	return lastErr
}

func downloadAttempt(ctx context.Context, f File) error {
	// Cancel the request when the body stalls. Each successful read resets
	// the timer, so slow but progressing downloads are unaffected.
	idleCtx, cancelIdle := context.WithCancel(ctx)
	defer cancelIdle()
	idleTimer := time.AfterFunc(downloadIdleTimeout, cancelIdle)
	defer idleTimer.Stop()

	req, err := http.NewRequestWithContext(idleCtx, http.MethodGet, f.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = f.Size
	}
	expectedSize := f.Size
	if expectedSize <= 0 {
		expectedSize = total
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Dest), ".download-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	var hasher hash.Hash
	if f.SHA512 != "" {
		hasher = sha512.New()
	} else if f.SHA256 != "" {
		hasher = sha256.New()
	} else if f.SHA1 != "" {
		hasher = sha1.New()
	}
	writer := io.Writer(tmp)
	if hasher != nil {
		writer = io.MultiWriter(tmp, hasher)
	}
	// Never write more than one byte past the expected size. When the size is
	// unknown, the idle timeout is the only bound.
	var reader io.Reader = resp.Body
	if expectedSize > 0 {
		reader = io.LimitReader(resp.Body, expectedSize+1)
	}
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			idleTimer.Reset(downloadIdleTimeout)
			if _, err := writer.Write(buf[:n]); err != nil {
				tmp.Close()
				return err
			}
			done += int64(n)
			if f.Progress != nil {
				f.Progress(done, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmp.Close()
			if idleCtx.Err() != nil && ctx.Err() == nil {
				return errors.New("下载停滞超时")
			}
			return readErr
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if hasher != nil {
		got := hex.EncodeToString(hasher.Sum(nil))
		want := f.SHA512
		if want == "" {
			want = f.SHA256
		}
		if want == "" {
			want = f.SHA1
		}
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("校验失败（期望 %s，实际 %s）", shortHash(want), shortHash(got))
		}
	}
	if expectedSize > 0 && done != expectedSize {
		return fmt.Errorf("文件大小不符（期望 %d，实际 %d）", expectedSize, done)
	}
	return replaceFile(tmpName, f.Dest)
}

// replaceFile atomically moves a freshly downloaded file into place. Windows
// does not allow os.Rename to replace an existing file, so the old file is
// moved aside first and restored if the replacement fails.
func replaceFile(tempPath, dest string) error {
	backup := ""
	if _, err := os.Stat(dest); err == nil {
		backup = dest + ".download-backup"
		for index := 1; ; index++ {
			if _, statErr := os.Stat(backup); os.IsNotExist(statErr) {
				break
			}
			backup = fmt.Sprintf("%s.download-backup.%d", dest, index)
		}
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(tempPath, dest); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
		return err
	}
	if backup != "" {
		_ = os.Remove(backup)
	}
	return nil
}

func withDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, 45*time.Second)
}

func verifyFile(path, sha1sum, sha256sum, sha512sum string) (bool, error) {
	if sha1sum == "" && sha256sum == "" && sha512sum == "" {
		return utilExists(path), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	var h hash.Hash
	want := ""
	switch {
	case sha512sum != "":
		h = sha512.New()
		want = sha512sum
	case sha256sum != "":
		h = sha256.New()
		want = sha256sum
	default:
		h = sha1.New()
		want = sha1sum
	}
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want), nil
}

func utilExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func shortHash(s string) string {
	if len(s) > 10 {
		return s[:10] + "…"
	}
	return s
}
