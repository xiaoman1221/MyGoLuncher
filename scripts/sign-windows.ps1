param(
  [Parameter(Mandatory = $false)][string]$Exe = "",
  [Parameter(Mandatory = $true)][string]$PfxPath,
  [string]$TimestampUrl = "",
  [switch]$TrustSelfSigned
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($Exe)) {
  $Exe = Join-Path $root "build\bin\MyMinecraftGoLuncher.exe"
}
if (-not (Test-Path -LiteralPath $Exe)) { throw "找不到待签名文件: $Exe" }
if (-not (Test-Path -LiteralPath $PfxPath)) { throw "找不到代码签名证书: $PfxPath" }
if ([string]::IsNullOrWhiteSpace($env:MGL_SIGN_PFX_PASSWORD)) {
  throw "请通过环境变量 MGL_SIGN_PFX_PASSWORD 提供 PFX 密码"
}

$signtool = Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin" -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue |
  Where-Object { $_.FullName -match "\\x64\\signtool\.exe$" } |
  Sort-Object FullName -Descending |
  Select-Object -First 1
if (-not $signtool) { throw "找不到 Windows SDK signtool.exe" }


if ($TrustSelfSigned) {
  $secure = ConvertTo-SecureString $env:MGL_SIGN_PFX_PASSWORD -AsPlainText -Force
  $cert = Import-PfxCertificate -FilePath $PfxPath -CertStoreLocation Cert:\CurrentUser\My -Password $secure
  if ($cert.Subject -eq $cert.Issuer) {
    $cer = Join-Path $env:TEMP ("mgl-sign-" + [guid]::NewGuid().ToString("N") + ".cer")
    Export-Certificate -Cert $cert -FilePath $cer | Out-Null
    Import-Certificate -FilePath $cer -CertStoreLocation Cert:\CurrentUser\Root | Out-Null
    Import-Certificate -FilePath $cer -CertStoreLocation Cert:\CurrentUser\TrustedPublisher | Out-Null
    Remove-Item -LiteralPath $cer -Force -ErrorAction SilentlyContinue
  }
}

Write-Host "签名: $Exe"
$signArgs = @("sign", "/fd", "SHA256", "/f", $PfxPath, "/p", $env:MGL_SIGN_PFX_PASSWORD)
if (-not [string]::IsNullOrWhiteSpace($TimestampUrl)) {
  $signArgs += @("/tr", $TimestampUrl, "/td", "SHA256")
}
$signArgs += $Exe
& $signtool.FullName @signArgs
if ($LASTEXITCODE -ne 0) { throw "signtool sign 失败，退出码 $LASTEXITCODE" }

& $signtool.FullName verify /pa /v $Exe
if ($LASTEXITCODE -ne 0) { throw "签名验证失败，退出码 $LASTEXITCODE" }
Write-Host "签名并验证完成"
