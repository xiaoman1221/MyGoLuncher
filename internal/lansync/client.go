package lansync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
	"MyMinecraftGoLuncher/internal/util"
)

// LAN transfers bypass system proxies: a corporate proxy must not receive
// local-network files.
var lanClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	},
}

const (
	maxManifestBytes = 16 << 20
	maxManifestFiles = 50000
)

// FetchManifest downloads and validates a peer's manifest.
func FetchManifest(ctx context.Context, baseURL, token string) (Manifest, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/manifest", nil)
	if err != nil {
		return Manifest{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(tokenHeader, token)
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := lanClient.Do(req)
	if err != nil {
		return Manifest{}, fmt.Errorf("连接局域网共享失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes+1))
	if err != nil {
		return Manifest{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("局域网共享返回错误 (%d): %s", resp.StatusCode, truncateText(string(body), 300))
	}
	if len(body) > maxManifestBytes {
		return Manifest{}, errors.New("局域网共享清单过大")
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("解析局域网共享清单失败: %w", err)
	}
	dirs, err := NormalizeDirs(manifest.Dirs)
	if err != nil {
		return Manifest{}, err
	}
	manifest.Dirs = dirs
	if len(manifest.Files) > maxManifestFiles {
		return Manifest{}, errors.New("局域网共享文件数量过多")
	}
	seen := make(map[string]bool, len(manifest.Files))
	for index := range manifest.Files {
		entry := &manifest.Files[index]
		if entry.Size < 0 {
			return Manifest{}, errors.New("局域网共享清单包含无效文件大小")
		}
		rel, err := safeRelativePath(entry.Path, manifest.Dirs)
		if err != nil {
			return Manifest{}, err
		}
		entry.Path = rel
		if len(entry.SHA256) != 64 {
			return Manifest{}, errors.New("局域网共享清单包含无效哈希")
		}
		if _, err := hex.DecodeString(entry.SHA256); err != nil {
			return Manifest{}, errors.New("局域网共享清单包含无效哈希")
		}
		entry.SHA256 = strings.ToLower(entry.SHA256)
		if seen[entry.Path] {
			return Manifest{}, errors.New("局域网共享清单包含重复文件")
		}
		seen[entry.Path] = true
	}
	return manifest, nil
}

// Sync incrementally downloads a shared manifest into localDir. Files with the
// same size and SHA-256 are skipped.
func Sync(ctx context.Context, baseURL, token, localDir string, manifest Manifest, progress ProgressFunc) (SyncResult, error) {
	var result SyncResult
	stat, err := os.Stat(localDir)
	if err != nil || !stat.IsDir() {
		return result, errors.New("目标游戏目录不存在")
	}
	dirs, err := NormalizeDirs(manifest.Dirs)
	if err != nil {
		return result, err
	}
	manifest.Dirs = dirs

	type pending struct {
		entry FileEntry
	}
	var downloads []pending
	var totalBytes int64
	totalFiles := int64(len(manifest.Files))
	for index, entry := range manifest.Files {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		rel, err := safeRelativePath(entry.Path, manifest.Dirs)
		if err != nil {
			return result, err
		}
		full, err := localPath(localDir, rel)
		if err != nil {
			return result, err
		}
		if progress != nil {
			progress("scan", fmt.Sprintf("正在检查 %s", rel), int64(index), totalFiles)
		}
		localStat, statErr := os.Stat(full)
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			downloads = append(downloads, pending{entry: entry})
			totalBytes += entry.Size
			continue
		case statErr != nil:
			return result, fmt.Errorf("读取本地文件 %s 失败: %w", rel, statErr)
		case localStat.IsDir():
			return result, fmt.Errorf("本地路径是目录: %s", rel)
		case localStat.Size() != entry.Size:
			downloads = append(downloads, pending{entry: entry})
			totalBytes += entry.Size
			continue
		}
		localHash, err := hashFile(full)
		if err != nil {
			return result, fmt.Errorf("校验本地文件 %s 失败: %w", rel, err)
		}
		if !strings.EqualFold(localHash, entry.SHA256) {
			downloads = append(downloads, pending{entry: entry})
			totalBytes += entry.Size
			continue
		}
		result.Skipped++
	}
	if progress != nil {
		progress("scan", fmt.Sprintf("检查完成：需要下载 %d 个文件", len(downloads)), totalFiles, totalFiles)
	}

	var downloadedBytes int64
	for index, item := range downloads {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		entry := item.entry
		full, err := localPath(localDir, entry.Path)
		if err != nil {
			return result, err
		}
		if progress != nil {
			progress("download", fmt.Sprintf("下载 %s（%d/%d）", entry.Path, index+1, len(downloads)), downloadedBytes, totalBytes)
		}
		if err := downloadFile(ctx, baseURL, token, entry, full); err != nil {
			return result, err
		}
		result.Downloaded++
		result.Bytes += entry.Size
		downloadedBytes += entry.Size
		if progress != nil {
			progress("download", fmt.Sprintf("已下载 %s", entry.Path), downloadedBytes, totalBytes)
		}
	}
	if progress != nil {
		progress("done", fmt.Sprintf("同步完成：下载 %d，跳过 %d", result.Downloaded, result.Skipped), 1, 1)
	}
	return result, nil
}

func downloadFile(ctx context.Context, baseURL, token string, entry FileEntry, dest string) error {
	fileURL := strings.TrimRight(baseURL, "/") + "/file?path=" + url.QueryEscape(entry.Path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set(tokenHeader, token)
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := lanClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载 %s 失败: %w", entry.Path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("下载 %s 失败 (%d): %s", entry.Path, resp.StatusCode, truncateText(string(body), 200))
	}
	if resp.ContentLength > 0 && resp.ContentLength != entry.Size {
		return fmt.Errorf("下载 %s 大小不一致", entry.Path)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(dest), ".mgl-lan-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temp, hasher), io.LimitReader(resp.Body, entry.Size+1))
	closeErr := temp.Close()
	if copyErr != nil {
		return fmt.Errorf("下载 %s 失败: %w", entry.Path, copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if written != entry.Size {
		return fmt.Errorf("下载 %s 大小不一致", entry.Path)
	}
	if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), entry.SHA256) {
		return fmt.Errorf("下载 %s 哈希校验失败", entry.Path)
	}
	_ = temp.Chmod(0o644)
	if err := util.ReplaceFile(tempName, dest); err != nil {
		return fmt.Errorf("替换 %s 失败: %w", entry.Path, err)
	}
	return nil
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
