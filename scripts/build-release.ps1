param(
  [string]$LicenseServerUrl = "",
  [string]$ProductAPIKey = $env:MGL_PRODUCT_API_KEY,
  [string]$ProductServerURL = $env:MGL_PRODUCT_SERVER_URL,
  [string]$ProductName = $env:MGL_PRODUCT_NAME,
  [string]$ProductSlug = $env:MGL_PRODUCT_SLUG,
  [string]$PfxPath = "",
  [string]$TimestampUrl = "http://timestamp.digicert.com",
  [string]$UpxPath = "",
  [switch]$Pack,
  [switch]$AllowHttp
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root

if ([string]::IsNullOrWhiteSpace($ProductServerURL) -and -not [string]::IsNullOrWhiteSpace($LicenseServerUrl)) {
  $ProductServerURL = $LicenseServerUrl
}
if (-not [string]::IsNullOrWhiteSpace($ProductServerURL)) {
  $uri = [Uri]$ProductServerURL
  if ($uri.Scheme -ne "https" -and -not $AllowHttp -and $uri.Host -notin @("localhost", "127.0.0.1")) {
    throw "生产构建必须使用 HTTPS 授权服务地址；本机调试可加 -AllowHttp"
  }
}
if ([string]::IsNullOrWhiteSpace($ProductAPIKey) -xor [string]::IsNullOrWhiteSpace($ProductServerURL)) {
  throw "官方商业构建必须同时提供 ProductAPIKey 和 ProductServerURL"
}

$quote = '"'
$ldflags = "-s -w -X " + $quote + "MyMinecraftGoLuncher/internal/security.Enforce=true" + $quote

$privateFile = Join-Path $root "internal/license/product_private.go"
if (Test-Path -LiteralPath $privateFile) {
  Remove-Item -LiteralPath $privateFile -Force
}
$privateCreated = $false
$buildArgs = @("build", "-trimpath", "-ldflags", $ldflags)
if (-not [string]::IsNullOrWhiteSpace($ProductAPIKey)) {
  $escape = { param([string]$value) $value.Replace('\', '\\').Replace('"', '\"') }
  $goName = if ([string]::IsNullOrWhiteSpace($ProductName)) { "MyGoLuncher" } else { & $escape $ProductName }
  $goSlug = if ([string]::IsNullOrWhiteSpace($ProductSlug)) { "MyGo" } else { & $escape $ProductSlug }
  $content = @"
//go:build mgl_private

package license

func init() {
	ProductAPIKey = "$(& $escape $ProductAPIKey)"
	ProductServerURL = "$(& $escape $ProductServerURL)"
	ProductName = "$goName"
	ProductSlug = "$goSlug"
}
"@
  [System.IO.File]::WriteAllText($privateFile, $content, (New-Object System.Text.UTF8Encoding($false)))
  $privateCreated = $true
  $buildArgs += @("-tags", "mgl_private")
  Write-Host "构建模式: 官方商业构建（私有构建文件，构建后删除）"
} else {
  Write-Host "构建模式: 社区开源构建（不包含商业授权服务）"
}

try {
  & wails @buildArgs
  if ($LASTEXITCODE -ne 0) { throw "wails build 失败，退出码 $LASTEXITCODE" }
} finally {
  if ($privateCreated -and (Test-Path -LiteralPath $privateFile)) {
    Remove-Item -LiteralPath $privateFile -Force
  }
}

$exe = Join-Path $root "build\bin\MyMinecraftGoLuncher.exe"
if (-not (Test-Path -LiteralPath $exe)) { throw "构建产物不存在: $exe" }

if ($Pack) {
  & (Join-Path $PSScriptRoot "pack-windows.ps1") -Exe $exe -UpxPath $UpxPath
  if ($LASTEXITCODE -ne 0) { throw "加壳失败" }
}

if (-not [string]::IsNullOrWhiteSpace($PfxPath)) {
  & (Join-Path $PSScriptRoot "sign-windows.ps1") -Exe $exe -PfxPath $PfxPath -TimestampUrl $TimestampUrl
  if ($LASTEXITCODE -ne 0) { throw "代码签名失败" }
} else {
  Write-Warning "未提供 -PfxPath，本次产物未签名；正式发布前请使用受信任的代码签名证书。"
}

$hash = Get-FileHash -LiteralPath $exe -Algorithm SHA256
Write-Host ("SHA256  " + $hash.Hash + "  " + $exe)
