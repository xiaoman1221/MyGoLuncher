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

// ManifestURL is the legacy static signed-manifest endpoint. New builds use
// the LicensingGo update API configured by ConfigureAPI instead.
var ManifestURL = ""

// APIBaseURL and APIKey point at the LicensingGo activation server. When both
// are set, GET /api/v1/update is preferred over the legacy signed manifest.
var (
	APIBaseURL = ""
	APIKey     = ""
)

// ConfigureAPI selects the LicensingGo product update endpoint. It is called
// after the official product configuration is loaded.
func ConfigureAPI(baseURL, apiKey string) {
	APIBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	APIKey = strings.TrimSpace(apiKey)
}

func configured() bool {
	return (APIBaseURL != "" && APIKey != "") || strings.TrimSpace(ManifestURL) != ""
}

// PublicKey is the Ed25519 key used to verify legacy manifest signatures. The
// private key must stay offline; only the public key ships with the client.
var PublicKey = mustPublicKey("/IYyZqpFAy8oGPzLuCnctjJOAws2lQFu94yZlt4NtjY=")

// Artifact is one downloadable package listed in the signed manifest.
type Artifact struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size,omitempty"`
}

func (a Artifact) Valid() bool {
	return strings.TrimSpace(a.URL) != "" && strings.TrimSpace(a.SHA256) != ""
}

// Progress reports update download/apply progress to the UI.
type Progress struct {
	Stage   string  `json:"stage"`
	Current int64   `json:"current"`
	Total   int64   `json:"total"`
	Percent float64 `json:"percent"`
}

// ProgressFunc receives throttled update progress callbacks.
type ProgressFunc func(Progress)

func reportProgress(progress ProgressFunc, stage string, current, total int64) {
	if progress == nil {
		return
	}
	percent := -1.0
	if total > 0 {
		percent = float64(current) / float64(total) * 100
		if percent > 100 {
			percent = 100
		}
	}
	progress(Progress{Stage: stage, Current: current, Total: total, Percent: percent})
}

// ArtifactFamily is the LicensingGo platform map:
// OS -> architecture -> artifact. It also accepts the legacy single-artifact
// shape returned by older manifests.
type ArtifactFamily struct {
	Entries map[string]map[string]Artifact
	Legacy  *Artifact
}

func (f *ArtifactFamily) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.Entries = map[string]map[string]Artifact{}
	var legacy Artifact
	if err := json.Unmarshal(data, &legacy); err == nil && legacy.Valid() {
		f.Legacy = &legacy
	}
	for key, value := range raw {
		if key == "version" || key == "url" || key == "sha256" || key == "size" {
			continue
		}
		var architectures map[string]Artifact
		if err := json.Unmarshal(value, &architectures); err != nil {
			continue
		}
		clean := map[string]Artifact{}
		for arch, artifact := range architectures {
			if artifact.Valid() {
				clean[strings.ToLower(strings.TrimSpace(arch))] = artifact
			}
		}
		if len(clean) > 0 {
			f.Entries[strings.ToLower(strings.TrimSpace(key))] = clean
		}
	}
	// An empty family is tolerated so a manifest can publish only one of
	// web/app; Select reports whether this family has a usable artifact.
	return nil
}

func (f ArtifactFamily) MarshalJSON() ([]byte, error) {
	if len(f.Entries) > 0 {
		return json.Marshal(f.Entries)
	}
	if f.Legacy != nil {
		return json.Marshal(f.Legacy)
	}
	return []byte("null"), nil
}

// Select chooses the current operating system and architecture, falling back
// to the any/any package and then the legacy default artifact.
func (f *ArtifactFamily) Select() (Artifact, bool) {
	if f == nil {
		return Artifact{}, false
	}
	osKey := manifestOS()
	archKey := manifestArch()
	if architectures, ok := f.Entries[osKey]; ok {
		if artifact, ok := architectures[archKey]; ok && artifact.Valid() {
			return artifact, true
		}
		if artifact, ok := architectures["any"]; ok && artifact.Valid() {
			return artifact, true
		}
		if artifact, ok := architectures["universal"]; ok && artifact.Valid() {
			return artifact, true
		}
	}
	if architectures, ok := f.Entries["any"]; ok {
		if artifact, ok := architectures[archKey]; ok && artifact.Valid() {
			return artifact, true
		}
		if artifact, ok := architectures["any"]; ok && artifact.Valid() {
			return artifact, true
		}
	}
	if f.Legacy != nil && f.Legacy.Valid() {
		return *f.Legacy, true
	}
	return Artifact{}, false
}

func manifestOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	default:
		return runtime.GOOS
	}
}

func manifestArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	case "arm":
		return "armv7"
	default:
		return runtime.GOARCH
	}
}

// Manifest is the payload signed by the release key.
type Manifest struct {
	Version    string          `json:"version"`
	MinVersion string          `json:"minVersion,omitempty"`
	Notes      string          `json:"notes,omitempty"`
	Mandatory  bool            `json:"mandatory,omitempty"`
	Force      bool            `json:"force,omitempty"`
	Published  string          `json:"publishedAt,omitempty"`
	Web        *ArtifactFamily `json:"web,omitempty"`
	App        *ArtifactFamily `json:"app,omitempty"`
}

// Info is the safe update state returned to the UI.
type Info struct {
	Configured     bool   `json:"configured"`
	CurrentVersion string `json:"currentVersion"`
	Version        string `json:"version,omitempty"`
	MinVersion     string `json:"minVersion,omitempty"`
	Notes          string `json:"notes,omitempty"`
	Mandatory      bool   `json:"mandatory"`
	Force          bool   `json:"force"`
	// Tag summarizes the update policy: "force", "mandatory" or "optional".
	Tag        string `json:"tag"`
	Available  bool   `json:"available"`
	HasWeb     bool   `json:"hasWeb"`
	HasApp     bool   `json:"hasApp"`
	WebVersion string `json:"webVersion,omitempty"`
	AppVersion string `json:"appVersion,omitempty"`
	Published  string `json:"publishedAt,omitempty"`
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
	if !configured() {
		return result, ErrNotConfigured
	}
	target := ManifestURL
	if APIBaseURL != "" && APIKey != "" {
		target = APIBaseURL
	}
	if err := validateRemoteURL(target); err != nil {
		return result, err
	}
	result.Configured = true
	manifest, _, err := fetchManifest(ctx)
	if errors.Is(err, ErrUpdateNotAvailable) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.Version = manifest.Version
	result.MinVersion = manifest.MinVersion
	result.Notes = manifest.Notes
	result.Mandatory = manifest.Mandatory
	result.Force = manifest.Force
	switch {
	case manifest.Force:
		result.Tag = "force"
	case manifest.Mandatory:
		result.Tag = "mandatory"
	default:
		result.Tag = "optional"
	}
	result.Published = manifest.Published
	if compareVersions(manifest.Version, currentVersion) > 0 {
		result.Available = true
	}
	if artifact, ok := manifest.Web.Select(); ok {
		result.HasWeb = artifact.Version != "" && !sameVersion(artifact.Version, currentWebVersion)
		result.WebVersion = artifact.Version
		if result.HasWeb {
			result.Available = true
		}
	}
	if artifact, ok := manifest.App.Select(); ok {
		result.HasApp = compareVersions(artifact.Version, currentVersion) > 0
		result.AppVersion = artifact.Version
		if result.HasApp {
			result.Available = true
		}
	}
	return result, nil
}

var ErrUpdateNotAvailable = errors.New("暂无更新")

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func fetchManifest(ctx context.Context) (Manifest, []byte, error) {
	if APIBaseURL != "" && APIKey != "" {
		return fetchAPIManifest(ctx)
	}
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

func fetchAPIManifest(ctx context.Context) (Manifest, []byte, error) {
	var manifest Manifest
	endpoint := strings.TrimRight(APIBaseURL, "/") + "/api/v1/update"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return manifest, nil, err
	}
	req.Header.Set("X-API-Key", APIKey)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return manifest, nil, fmt.Errorf("无法连接授权更新服务: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return manifest, nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return manifest, raw, ErrUpdateNotAvailable
	}
	if resp.StatusCode != http.StatusOK {
		var failure apiError
		_ = json.Unmarshal(raw, &failure)
		message := strings.TrimSpace(failure.Message)
		if message == "" {
			message = strings.TrimSpace(failure.Code)
		}
		if message == "" {
			message = fmt.Sprintf("授权更新服务返回 HTTP %d", resp.StatusCode)
		}
		return manifest, raw, errors.New(message)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, raw, fmt.Errorf("解析更新清单失败: %w", err)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return manifest, raw, errors.New("更新清单缺少版本号")
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
	return ApplyWebWithProgress(ctx, root, nil)
}

// ApplyWebWithProgress is ApplyWeb with download/apply progress callbacks.
func ApplyWebWithProgress(ctx context.Context, root string, progress ProgressFunc) (string, error) {
	if !configured() {
		return "", ErrNotConfigured
	}
	manifest, _, err := fetchManifest(ctx)
	if err != nil {
		return "", err
	}
	if manifest.Web == nil {
		return "", errors.New("更新清单没有前端热更新包")
	}
	artifact, ok := manifest.Web.Select()
	if !ok {
		return "", errors.New("更新清单没有适用于当前平台的前端包")
	}
	cache := filepath.Join(root, "cache", "updates")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	zipPath := filepath.Join(cache, "web-"+safeVersion(artifact.Version)+".zip")
	if err := download(ctx, artifact, zipPath, progress); err != nil {
		return "", err
	}
	reportProgress(progress, "extract", 0, 0)
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
	_ = os.WriteFile(filepath.Join(staging, ".version"), []byte(artifact.Version), 0o644)
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
	reportProgress(progress, "done", 1, 1)
	return dest, nil
}

// ApplyApp downloads and verifies a new executable, then starts a detached
// PowerShell helper that waits for this process to exit before replacing it.
func ApplyApp(ctx context.Context, root, currentExe, currentVersion string) (string, error) {
	return ApplyAppWithGuardProgress(ctx, root, currentExe, currentVersion, nil, nil)
}

// ApplyAppWithGuard is ApplyApp with a final guard invoked after the download
// completes and before the detached replacement helper is started.
func ApplyAppWithGuard(ctx context.Context, root, currentExe, currentVersion string, guard func() error) (string, error) {
	return ApplyAppWithGuardProgress(ctx, root, currentExe, currentVersion, guard, nil)
}

// ApplyAppWithGuardProgress is ApplyAppWithGuard with download progress
// callbacks for the UI.
func ApplyAppWithGuardProgress(ctx context.Context, root, currentExe, currentVersion string, guard func() error, progress ProgressFunc) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("自动替换当前仅支持 Windows")
	}
	if !configured() {
		return "", ErrNotConfigured
	}
	manifest, _, err := fetchManifest(ctx)
	if err != nil {
		return "", err
	}
	if manifest.App == nil {
		return "", errors.New("更新清单没有完整安装包")
	}
	artifact, ok := manifest.App.Select()
	if !ok {
		return "", errors.New("更新清单没有适用于当前平台的完整安装包")
	}
	if compareVersions(artifact.Version, currentVersion) <= 0 {
		return "", errors.New("当前已经是最新版本")
	}
	cache := filepath.Join(root, "cache", "updates")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	newExe := filepath.Join(cache, "MyMinecraftGoLuncher-"+safeVersion(artifact.Version)+".exe")
	if err := download(ctx, artifact, newExe, progress); err != nil {
		return "", err
	}
	if guard != nil {
		if err := guard(); err != nil {
			return "", err
		}
	}
	reportProgress(progress, "prepare", 0, 0)
	script := filepath.Join(cache, "apply-update.ps1")
	// Windows PowerShell 5.1 reads BOM-less scripts as ANSI; the Chinese log
	// messages below would then break parsing. Always write a UTF-8 BOM.
	content := applyUpdateScriptContent()
	if err := os.WriteFile(script, []byte(content), 0o600); err != nil {
		return "", err
	}
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden",
		"-File", script, "-TargetPid", strconv.Itoa(os.Getpid()),
		"-NewExe", newExe, "-TargetExe", currentExe,
		"-ExpectedSHA256", strings.TrimSpace(artifact.SHA256),
	)
	sysutil.HideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动更新助手失败: %w", err)
	}
	_ = cmd.Process.Release()
	reportProgress(progress, "restart", 1, 1)
	return script, nil
}

// applyUpdateScriptContent prefixes the helper with a UTF-8 BOM so Windows
// PowerShell 5.1 does not parse the Chinese log strings as ANSI.
func applyUpdateScriptContent() string {
	return "\ufeff" + applyUpdateScript
}

const applyUpdateScript = `param(
  [int]$TargetPid,
  [string]$NewExe,
  [string]$TargetExe,
  [string]$ExpectedSHA256 = ''
)
$ErrorActionPreference = 'Stop'
$targetDir = Split-Path -Parent $TargetExe
$backup = $TargetExe + '.bak'
$errorLog = Join-Path $targetDir 'update-error.log'
$fallbackLog = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'MyGoLuncher/update-error.log'

function Write-UpdateLog([string]$Message) {
  $line = '[' + (Get-Date).ToString('s') + '] ' + $Message
  try {
    Add-Content -LiteralPath $errorLog -Value $line -ErrorAction Stop
  } catch {
    try {
      New-Item -ItemType Directory -Force -Path (Split-Path -Parent $fallbackLog) | Out-Null
      Add-Content -LiteralPath $fallbackLog -Value $line -ErrorAction SilentlyContinue
    } catch {}
  }
}

function Test-ProcessAlive([int]$ProcessId) {
  try { return [bool](Get-Process -Id $ProcessId -ErrorAction Stop) } catch { return $false }
}

function Test-Writable([string]$Path) {
  $probe = Join-Path $Path '.mgl-update-write-test'
  try {
    [IO.File]::WriteAllText($probe, 'ok')
    Remove-Item -LiteralPath $probe -Force -ErrorAction SilentlyContinue
    return $true
  } catch {
    return $false
  }
}

function Test-Elevated {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Program Files installs need elevation to replace the executable. Relaunch the
# same helper through UAC once; if the prompt is cancelled, restart the old app.
if (-not (Test-Writable $targetDir)) {
  if (-not (Test-Elevated)) {
    try {
      Write-UpdateLog "目标目录不可写，正在请求管理员权限: $targetDir"
      $quote = { param([string]$value) '"' + $value.Replace('"', '\"') + '"' }
      $argLine = '-NoProfile -NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -File ' +
        (& $quote $PSCommandPath) + ' -TargetPid ' + $TargetPid +
        ' -NewExe ' + (& $quote $NewExe) +
        ' -TargetExe ' + (& $quote $TargetExe) +
        ' -ExpectedSHA256 ' + (& $quote $ExpectedSHA256)
      Start-Process -FilePath 'powershell.exe' -Verb RunAs -ArgumentList $argLine
      exit
    } catch {
      Write-UpdateLog ('无法请求管理员权限: ' + $_.Exception.Message)
      Start-Process -FilePath $TargetExe -WorkingDirectory $targetDir -ErrorAction SilentlyContinue
      exit 1
    }
  }
}

try {
  Write-UpdateLog "等待启动器退出: $TargetPid"
  $deadline = (Get-Date).AddSeconds(180)
  while ((Test-ProcessAlive $TargetPid) -and ((Get-Date) -lt $deadline)) {
    Start-Sleep -Milliseconds 250
  }
  if (Test-ProcessAlive $TargetPid) {
    throw '启动器仍在运行，已取消替换'
  }
  Start-Sleep -Milliseconds 800

  if (-not (Test-Path -LiteralPath $NewExe)) {
    throw "新版本文件不存在: $NewExe"
  }
  if (-not (Test-Path -LiteralPath $TargetExe)) {
    throw "目标程序不存在: $TargetExe"
  }

  $probe = Join-Path $targetDir '.mgl-update-write-test'
  try {
    [IO.File]::WriteAllText($probe, 'ok')
    Remove-Item -LiteralPath $probe -Force -ErrorAction SilentlyContinue
  } catch {
    throw "目标目录不可写，请将启动器移动到用户目录，或以管理员身份运行更新: $targetDir"
  }

  Copy-Item -LiteralPath $TargetExe -Destination $backup -Force -ErrorAction Stop
  $copied = $false
  $lastError = $null
  for ($i = 0; $i -lt 120; $i++) {
    try {
      Copy-Item -LiteralPath $NewExe -Destination $TargetExe -Force -ErrorAction Stop
      $copied = $true
      break
    } catch {
      $lastError = $_.Exception.Message
      Start-Sleep -Milliseconds 250
    }
  }
  if (-not $copied) {
    throw "无法替换程序文件: $lastError"
  }

  if (-not [string]::IsNullOrWhiteSpace($ExpectedSHA256)) {
    $actual = (Get-FileHash -LiteralPath $TargetExe -Algorithm SHA256).Hash
    if ($actual -ne $ExpectedSHA256.ToUpperInvariant()) {
      throw "替换后 SHA256 校验失败: $actual"
    }
  }

  Write-UpdateLog '替换完成，正在重启启动器'
  Start-Process -FilePath $TargetExe -WorkingDirectory $targetDir
} catch {
  Write-UpdateLog ('ERROR: ' + $_.Exception.Message)
  if (Test-Path -LiteralPath $backup) {
    Copy-Item -LiteralPath $backup -Destination $TargetExe -Force -ErrorAction SilentlyContinue
  }
  Start-Process -FilePath $TargetExe -WorkingDirectory $targetDir -ErrorAction SilentlyContinue
}
`

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

func download(ctx context.Context, artifact Artifact, dest string, progress ProgressFunc) error {
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
	total := artifact.Size
	if total <= 0 {
		total = resp.ContentLength
	}
	if total < 0 {
		total = 0
	}
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	hash := sha256.New()
	limit := int64(1 << 30)
	reportProgress(progress, "download", 0, total)
	writer := &progressWriter{
		writer: io.MultiWriter(file, hash),
		total:  total,
		report: func(done, total int64) { reportProgress(progress, "download", done, total) },
	}
	written, copyErr := io.Copy(writer, io.LimitReader(resp.Body, limit))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return closeErr
	}
	reportProgress(progress, "download", written, total)
	if artifact.Size > 0 && written != artifact.Size {
		_ = os.Remove(dest)
		return fmt.Errorf("更新包大小不匹配: %d != %d", written, artifact.Size)
	}
	reportProgress(progress, "verify", written, written)
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(artifact.SHA256)) {
		_ = os.Remove(dest)
		return errors.New("更新包 SHA256 校验失败")
	}
	return nil
}

type progressWriter struct {
	writer io.Writer
	done   int64
	total  int64
	report func(done, total int64)
	last   time.Time
}

func (w *progressWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	w.done += int64(n)
	now := time.Now()
	// Emit at most ~10 progress events per second.
	if w.report != nil && (now.Sub(w.last) >= 100*time.Millisecond || (w.total > 0 && w.done >= w.total)) {
		w.report(w.done, w.total)
		w.last = now
	}
	return n, err
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
