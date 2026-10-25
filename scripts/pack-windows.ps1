param(
  [Parameter(Mandatory = $false)][string]$Exe = "",
  [string]$UpxPath = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($Exe)) {
  $Exe = Join-Path $root "build\bin\MyMinecraftGoLuncher.exe"
}
if (-not (Test-Path -LiteralPath $Exe)) { throw "找不到待加壳文件: $Exe" }

if ([string]::IsNullOrWhiteSpace($UpxPath)) {
  $candidate = Get-ChildItem (Join-Path $root ".tools\upx") -Recurse -Filter "upx.exe" -ErrorAction SilentlyContinue |
    Select-Object -First 1
  if ($candidate) {
    $UpxPath = $candidate.FullName
  } else {
    $command = Get-Command upx.exe -ErrorAction SilentlyContinue
    if ($command) { $UpxPath = $command.Source }
  }
}
if ([string]::IsNullOrWhiteSpace($UpxPath) -or -not (Test-Path -LiteralPath $UpxPath)) {
  throw "找不到 UPX。请安装 UPX 或用 -UpxPath 指定 upx.exe；加壳产物可能被杀毒软件误报。"
}

Write-Host "加壳: $Exe"
& $UpxPath --best --lzma --backup $Exe
if ($LASTEXITCODE -ne 0) { throw "UPX 加壳失败，退出码 $LASTEXITCODE" }

& $UpxPath -t $Exe
if ($LASTEXITCODE -ne 0) { throw "UPX 完整性测试失败，退出码 $LASTEXITCODE" }
Write-Host "加壳并测试完成（原始文件保留为 .~ 备份）"
