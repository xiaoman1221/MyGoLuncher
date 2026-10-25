// Package updater implements signed OTA manifests, front-end hot updates and
// full executable replacement for MyGoLuncher.
package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/sysutil"
	"MyMinecraftGoLuncher/internal/util"
)

// ManifestURL is injected by the release build. Empty disables update checks.
var ManifestURL = ""

// PublicKey is the Ed25519 key used to verify manifest signatures. The private
// key must stay offline; only the public key ships with the client.
var PublicKey = mustPublicKey("/IYyZqpFAy8oGPzLuCnctjJOAws2lQFu94yZlt4NtjY=")

// Artifact is one downloadable package listed in the signed manifest.
type Artifact struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size,omitempty"`
}

// Manifest is the payload signed by the release key.
type Manifest struct {
	Version    string    `json:"version"`
	MinVersion string    `json:"minVersion,omitempty"`
	Notes      string    `json:"notes,omitempty"`
	Mandatory  bool      `json:"mandatory,omitempty"`
	Published  string    `json:"publishedAt,omitempty"`
	Web        *Artifact `json:"web,omitempty"`
	App        *Artifact `json:"app,omitempty"`
}

// Info is the safe update state returned to the UI.
type Info struct {
	Configured     bool   `json:"configured"`
	CurrentVersion string `json:"currentVersion"`
	Version        string `json:"version,omitempty"`
	MinVersion     string `json:"minVersion,omitempty"`
	Notes          string `json:"notes,omitempty"`
	Mandatory      bool   `json:"mandatory"`
	Available      bool   `json:"available"`
	HasWeb         bool   `json:"hasWeb"`
	HasApp         bool   `json:"hasApp"`
	WebVersion     string `json:"webVersion,omitempty"`
	AppVersion     string `json:"appVersion,omitempty"`
	Published      string `json:"publishedAt,omitempty"`
}

// ErrNotConfigured is returned when no release manifest URL was baked in.
var ErrNotConfigured = errors.New("未配置更新服务器")

func mustPublicKey(encoded string) ed25519.PublicKey {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != ed25519.PublicKeySize {
		panic("invalid OTA public key")
	}
	return ed25519.PublicKey(key)
}

// Check fetches and verifies the update manifest, then compares versions.
func Check(ctx context.Context, currentVersion, currentWebVersion string) (Info, error) {
	result := Info{CurrentVersion: currentVersion}
	if strings.TrimSpace(ManifestURL) == "" {
		return result, ErrNotConfigured
	}
	if err := validateRemoteURL(ManifestURL); err != nil {
		return result, err
	}
	result.Configured = true
	manifest, _, err := fetchManifest(ctx)
	if err != nil {
		return result, err
	}
	result.Version = manifest.Version
	result.MinVersion = manifest.MinVersion
	result.Notes = manifest.Notes
	result.Mandatory = manifest.Mandatory
	result.Published = manifest.Published
	if compareVersions(manifest.Version, currentVersion) > 0 {
		result.Available = true
	}
	if manifest.Web != nil && manifest.Web.URL != "" && manifest.Web.SHA256 != "" {
		result.HasWeb = manifest.Web.Version != "" && !sameVersion(manifest.Web.Version, currentWebVersion)
		result.WebVersion = manifest.Web.Version
		if result.HasWeb {
			result.Available = true
		}
	}
	if manifest.App != nil && manifest.App.URL != "" && manifest.App.SHA256 != "" {
		result.HasApp = compareVersions(manifest.App.Version, currentVersion) > 0
		result.AppVersion = manifest.App.Version
		if result.HasApp {
			result.Available = true
		}
	}
	return result, nil
}

func fetchManifest(ctx context.Context) (Manifest, []byte, error) {
	var manifest Manifest
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ManifestURL, nil)
	if err != nil {
		return manifest, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return manifest, nil, fmt.Errorf("无法连接更新服务器: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return manifest, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return manifest, nil, fmt.Errorf("更新服务器返回 HTTP %d", resp.StatusCode)
	}
	signature, err := fetchSignature(ctx, client, resp.Header.Get("X-MGL-Signature"))
	if err != nil {
		return manifest, nil, err
	}
	if !ed25519.Verify(PublicKey, raw, signature) {
		return manifest, nil, errors.New("更新清单签名无效")
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, nil, fmt.Errorf("解析更新清单失败: %w", err)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return manifest, nil, errors.New("更新清单缺少版本号")
	}
	return manifest, raw, nil
}

func fetchSignature(ctx context.Context, client *http.Client, header string) ([]byte, error) {
	encoded := strings.TrimSpace(header)
	if encoded == "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ManifestURL+".sig", nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("无法下载更新签名: %w", err)
		}
		defer resp.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("更新签名返回 HTTP %d", resp.StatusCode)
		}
		encoded = strings.TrimSpace(string(body))
	}
	signature, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, errors.New("更新签名格式无效")
	}
	return signature, nil
}

// ApplyWeb downloads, verifies and swaps the front-end web package. The
// previous directory is kept as web.old for manual rollback.
func ApplyWeb(ctx context.Context, root string) (string, error) {
	if strings.TrimSpace(ManifestURL) == "" {
		return "", ErrNotConfigured
	}
	manifest, _, err := fetchManifest(ctx)
	if err != nil {
		return "", err
	}
	if manifest.Web == nil {
		return "", errors.New("更新清单没有前端热更新包")
	}
	cache := filepath.Join(root, "cache", "updates")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	zipPath := filepath.Join(cache, "web-"+safeVersion(manifest.Web.Version)+".zip")
	if err := download(ctx, *manifest.Web, zipPath); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(cache, "web-staging-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	if err := util.ExtractZip(zipPath, staging, ""); err != nil {
		return "", fmt.Errorf("解压前端热更新失败: %w", err)
	}
	if _, err := os.Stat(filepath.Join(staging, "index.html")); err != nil {
		return "", errors.New("前端热更新包缺少 index.html")
	}
	_ = os.WriteFile(filepath.Join(staging, ".version"), []byte(manifest.Web.Version), 0o644)
	dest := filepath.Join(root, "web")
	backup := filepath.Join(root, "web.old")
	if _, err := os.Stat(dest); err == nil {
		_ = os.RemoveAll(backup)
		if err := os.Rename(dest, backup); err != nil {
			return "", fmt.Errorf("备份旧前端失败: %w", err)
		}
	}
	if err := os.Rename(staging, dest); err != nil {
		_ = os.Rename(backup, dest)
		return "", fmt.Errorf("应用前端热更新失败: %w", err)
	}
	return dest, nil
}

// ApplyApp downloads and verifies a new executable, then starts a detached
// PowerShell helper that waits for this process to exit before replacing it.
func ApplyApp(ctx context.Context, root, currentExe, currentVersion string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("自动替换当前仅支持 Windows")
	}
	if strings.TrimSpace(ManifestURL) == "" {
		return "", ErrNotConfigured
	}
	manifest, _, err := fetchManifest(ctx)
	if err != nil {
		return "", err
	}
	if manifest.App == nil {
		return "", errors.New("更新清单没有完整安装包")
	}
	if compareVersions(manifest.App.Version, currentVersion) <= 0 {
		return "", errors.New("当前已经是最新版本")
	}
	cache := filepath.Join(root, "cache", "updates")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	newExe := filepath.Join(cache, "MyMinecraftGoLuncher-"+safeVersion(manifest.App.Version)+".exe")
	if err := download(ctx, *manifest.App, newExe); err != nil {
		return "", err
	}
	script := filepath.Join(cache, "apply-update.ps1")
	content := fmt.Sprintf(`param([int]$TargetPid, [string]$NewExe, [string]$TargetExe)
$ErrorActionPreference = 'SilentlyContinue'
Wait-Process -Id $TargetPid -Timeout 120
Start-Sleep -Milliseconds 500
Copy-Item -LiteralPath $TargetExe -Destination ($TargetExe + '.bak') -Force
Copy-Item -LiteralPath $NewExe -Destination $TargetExe -Force
Start-Process -FilePath $TargetExe
Remove-Item -LiteralPath $PSCommandPath -Force
`)
	if err := os.WriteFile(script, []byte(content), 0o600); err != nil {
		return "", err
	}
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden",
		"-File", script, "-TargetPid", strconv.Itoa(os.Getpid()),
		"-NewExe", newExe, "-TargetExe", currentExe,
	)
	sysutil.HideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动更新助手失败: %w", err)
	}
	_ = cmd.Process.Release()
	return script, nil
}

func validateRemoteURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return errors.New("更新地址无效")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1") {
		return nil
	}
	return errors.New("更新地址必须使用 HTTPS")
}

func download(ctx context.Context, artifact Artifact, dest string) error {
	if err := validateRemoteURL(artifact.URL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载更新包失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("更新包返回 HTTP %d", resp.StatusCode)
	}
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	hash := sha256.New()
	limit := int64(1 << 30)
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, limit))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return closeErr
	}
	if artifact.Size > 0 && written != artifact.Size {
		_ = os.Remove(dest)
		return fmt.Errorf("更新包大小不匹配: %d != %d", written, artifact.Size)
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(artifact.SHA256)) {
		_ = os.Remove(dest)
		return errors.New("更新包 SHA256 校验失败")
	}
	return nil
}

// CurrentWebVersion reads the version marker left by ApplyWeb.
func CurrentWebVersion(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "web", ".version"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// RollbackWeb restores web.old when a hot update breaks the UI.
func RollbackWeb(root string) error {
	dest := filepath.Join(root, "web")
	backup := filepath.Join(root, "web.old")
	if _, err := os.Stat(backup); err != nil {
		return errors.New("没有可回滚的前端版本")
	}
	broken := dest + ".broken"
	_ = os.RemoveAll(broken)
	if err := os.Rename(dest, broken); err != nil {
		return err
	}
	if err := os.Rename(backup, dest); err != nil {
		_ = os.Rename(broken, dest)
		return err
	}
	return nil
}

func compareVersions(a, b string) int {
	parse := func(value string) []int {
		value = strings.TrimPrefix(strings.TrimSpace(value), "v")
		value = strings.SplitN(value, "-", 2)[0]
		parts := strings.Split(value, ".")
		out := make([]int, len(parts))
		for index, part := range parts {
			out[index], _ = strconv.Atoi(part)
		}
		return out
	}
	left, right := parse(a), parse(b)
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for index := 0; index < length; index++ {
		var l, r int
		if index < len(left) {
			l = left[index]
		}
		if index < len(right) {
			r = right[index]
		}
		if l > r {
			return 1
		}
		if l < r {
			return -1
		}
	}
	return 0
}

func sameVersion(a, b string) bool { return compareVersions(a, b) == 0 }

func safeVersion(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '.' || r == '-' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}
