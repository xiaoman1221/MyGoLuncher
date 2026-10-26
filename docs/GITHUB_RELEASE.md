# GitHub Actions 官方发布

官方构建工作流：

```text
.github/workflows/release.yml
```

它**不会在每次上传/推送时都构建正式版本**。只有两种触发方式：

1. 推送到 `master` / `main`，且 **commit message 中包含 `vX.Y.Z`**
2. 在 Actions 页面手动运行 `Official release`

普通提交会先执行一个很轻量的 gate；没有版本号时，后续测试、构建和发布全部跳过。

标签推送不会触发本工作流；发布以 commit message 为准。

## 1. 版本号触发规则

以下 commit message 会触发：

```text
release: v1.0.2
fix: prepare v1.0.2 release
v1.0.2
```

以下不会触发：

```text
fix: crash on startup
release: 1.0.2
update docs
```

gate 会提取 `v1.0.2`，然后检查源码版本是否已经是 `1.0.2`。不一致会直接失败。

推送带版本号的 commit 并构建成功后，会自动创建 GitHub Release。

## 2. 自签名证书

Windows、macOS、Linux 都使用自签名证书，不依赖付费代码签名平台或 Apple 公证。

### Windows 自签名 PFX

在 Windows 管理员 PowerShell 中：

```powershell
$cert = New-SelfSignedCertificate `
  -Type CodeSigningCert `
  -Subject "CN=MyGoLuncher Self-Signed" `
  -CertStoreLocation Cert:\CurrentUser\My `
  -KeyExportPolicy Exportable `
  -NotAfter (Get-Date).AddYears(3)

$password = ConvertTo-SecureString "换成你的PFX密码" -AsPlainText -Force
Export-PfxCertificate `
  -Cert $cert `
  -FilePath .\MyGoLuncher-selfsigned.pfx `
  -Password $password
```

为了让 Windows 用户不看到“未知发布者”，需要把证书导入目标机器的：

```text
受信任的根证书颁发机构
受信任的发布者
```

自签名证书不会自动获得公共信任。

### macOS 自签名证书

在“钥匙串访问”中：

```text
钥匙串访问 → 证书助理 → 创建证书
名称：MyGoLuncher Self-Signed
身份类型：自签名根证书
证书类型：代码签名
```

创建后导出为带私钥的 `.p12`。

查看 Identity：

```bash
security find-identity -v -p codesigning
```

例如：

```text
"MyGoLuncher Self-Signed"
```

把该名称填入 `MACOS_SIGNING_IDENTITY`。

macOS 没有 Apple 公证；用户第一次打开时需要右键 → 打开，或在目标机器上信任该证书。

### Linux 自签名 GPG

Linux 没有统一的桌面代码签名机制，使用自签名 GPG 签名 `SHA256SUMS.txt`。

```bash
gpg --full-generate-key
gpg --list-secret-keys --keyid-format LONG
gpg --armor --export-secret-keys <指纹> > linux-selfsigned-private.asc
base64 -w0 linux-selfsigned-private.asc > linux-gpg.b64
```

GPG 私钥和密码只放 GitHub Secrets。

## 3. 配置 GitHub Secrets

进入：

```text
Repository → Settings → Secrets and variables → Actions
```

### Repository secrets

| Secret | 用途 |
|---|---|
| `MGL_PRODUCT_API_KEY` | LicensingGo 产品 API Key |
| `MGL_PRODUCT_SERVER_URL` | 授权服务器地址 |
| `WINDOWS_PFX_BASE64` | Windows 自签名 PFX 的 Base64 |
| `WINDOWS_PFX_PASSWORD` | PFX 密码 |
| `MACOS_CERTIFICATE_BASE64` | macOS 自签名 P12 的 Base64 |
| `MACOS_CERTIFICATE_PASSWORD` | P12 密码 |
| `MACOS_SIGNING_IDENTITY` | 例如 `MyGoLuncher Self-Signed` |
| `LINUX_GPG_PRIVATE_KEY_BASE64` | Linux GPG 私钥的 Base64 |
| `LINUX_GPG_PASSPHRASE` | GPG 私钥密码；无密码时留空 |

### Repository variables（可选）

| Variable | 默认值 |
|---|---|
| `MGL_PRODUCT_NAME` | `MyGoLuncher` |
| `MGL_PRODUCT_SLUG` | `MyGo` |

### Base64 生成

Windows PFX：

```powershell
[Convert]::ToBase64String(
  [IO.File]::ReadAllBytes(".\MyGoLuncher-selfsigned.pfx")
) | Set-Content -NoNewline .\windows-pfx.b64
```

macOS P12：

```bash
base64 -i MyGoLuncher-selfsigned.p12 -o macos-cert.b64
```

Linux GPG：

```bash
base64 -w0 linux-selfsigned-private.asc > linux-gpg.b64
```

通过网页或 CLI 写入 Secrets：

```powershell
gh secret set WINDOWS_PFX_BASE64 < .\windows-pfx.b64
gh secret set MACOS_CERTIFICATE_BASE64 < .\macos-cert.b64
gh secret set LINUX_GPG_PRIVATE_KEY_BASE64 < .\linux-gpg.b64
```

不要提交这些文件。

## 4. 修改版本号

例如发布 `1.0.2`：

```powershell
node .\scripts\set-version.mjs 1.0.2
node .\scripts\check-release-version.mjs 1.0.2
```

正常输出：

```text
release version 1.0.2 is consistent
```

提交并使用带版本号的 commit message：

```powershell
git add .
git commit -m "release: v1.0.2"
git push origin master
```

推送后 gate 检测到 `v1.0.2`，开始测试和构建。

> 不要再用普通 commit 触发正式发布；普通 commit 即使修改了版本文件，也不会通过 gate。

## 5. 手动试构建

进入：

```text
Actions → Official release → Run workflow
```

填写：

```text
version: 1.0.2
publish: false
```

手动运行不受 commit message 限制。

这会构建并上传 Artifacts，但不会创建 GitHub Release。

## 6. 正式发布

### 方式 A：带版本号的 commit

```text
release: v1.0.2
```

推送到 `master` 或 `main` 后，构建成功会自动创建 GitHub Release。

### 方式 B：手动发布

```text
Actions → Official release → Run workflow
version: 1.0.2
publish: true
```

## 7. 发布产物

```text
MyGoLuncher-<version>-windows-amd64.exe
MyGoLuncher-<version>-windows-amd64.zip
MyGoLuncher-<version>-macos-universal.zip
MyGoLuncher-<version>-linux-amd64.tar.gz
MyGoLuncher-<version>-linux-amd64.deb
MyGoLuncher-<version>-linux-amd64.rpm
MyGoLuncher-<version>-web.zip
SHA256SUMS.txt
SHA256SUMS.txt.asc
linux-public-key.asc
```

- Windows exe 使用自签名 PFX 签名
- macOS app 使用自签名证书签名
- Linux tar.gz、deb、rpm 由自签名 GPG 签名的 SHA256SUMS 覆盖
- deb 适用于 Debian/Ubuntu，rpm 适用于 Fedora/RHEL 系
- web zip 根目录包含 `index.html`
- SHA256SUMS.txt.asc 是自签名 GPG 签名

## 8. 验证

### Windows

```powershell
Get-AuthenticodeSignature .\MyGoLuncher-1.0.2-windows-amd64.exe |
  Format-List Status, StatusMessage, SignerCertificate
```

在签名者本机或已导入证书的机器上应为：

```text
Status: Valid
```

没有导入自签名根证书的机器仍会提示未知发布者。

### macOS

```bash
codesign --verify --deep --strict --verbose=2 MyMinecraftGoLuncher.app
codesign -dv --verbose=4 MyMinecraftGoLuncher.app
```

自签名证书不会通过 Apple 公证，Gatekeeper 仍可能拦截；用户需要右键打开并信任证书。

### Linux

```bash
gpg --import linux-public-key.asc
gpg --verify SHA256SUMS.txt.asc SHA256SUMS.txt
sha256sum -c SHA256SUMS.txt
```

`linux-public-key.asc` 会随 Release 一起发布，用于验证自签名 GPG。

## 9. 配置 OTA

发布后使用 GitHub Release 地址和 `SHA256SUMS.txt` 配置 LicensingGo：

```json
{
  "version": "1.0.2",
  "minVersion": "1.0.0",
  "mandatory": false,
  "force": false,
  "web": {
    "version": "1.0.2",
    "url": "https://github.com/<owner>/<repo>/releases/download/v1.0.2/MyGoLuncher-1.0.2-web.zip",
    "sha256": "<SHA256SUMS 中的值>",
    "size": 347631
  },
  "app": {
    "version": "1.0.2",
    "url": "https://github.com/<owner>/<repo>/releases/download/v1.0.2/MyGoLuncher-1.0.2-windows-amd64.exe",
    "sha256": "<SHA256SUMS 中的值>",
    "size": 23955456
  }
}
```

## 10. 注意事项

- commit message 必须包含完整的 `vX.Y.Z`，只写 `1.0.2` 不会触发。
- gate 跳过时属于正常流程，不是构建失败。
- 标签不会触发本工作流。
- 正式发布会自动创建 GitHub Release；如果只想试构建，请使用手动运行并设置 `publish: false`。
- 自签名证书不会自动获得 Windows SmartScreen / macOS Gatekeeper 的公共信任。
- GPG 是 Linux 发布包的签名方式，不是系统级可执行文件签名。
- 证书、PFX/P12、GPG 私钥和密码不要提交到仓库。
