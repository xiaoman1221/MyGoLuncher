# TeamCity 内部检查（方案 C）

当前分工：

- **TeamCity**：安全检查、后端测试、前端构建
- **GitHub Actions**：三平台官方签名发布，工作流为 [`.github/workflows/release.yml`](../.github/workflows/release.yml)

版本化配置：

```text
.teamcity/settings.kts
```

| 构建类型 | 触发 | 代理要求 | 用途 |
| --- | --- | --- | --- |
| Security Checks & Tests | master 提交 | Linux | 安全扫描、`go test`、前端构建 |
| Windows Commercial Release | 手动 | Windows | 旧的手动发布回退 |
| macOS Commercial Release | 手动 | macOS | 旧的手动发布回退 |
| Linux Commercial Release | 手动 | Linux | 旧的手动发布回退 |

> 三平台正式发布以 GitHub Actions 为主；TeamCity 的三个 Release 构建仅作为内网应急回退，不应与 GitHub Release 同时开启。

安全检查脚本为：

```text
scripts/teamcity-security.mjs
```

检查内容：

1. 敏感文件是否被 Git 跟踪（`product_private.go`、`.env`、PFX/P12、密钥、`.signing/`、`.idea/`、`MGL/`、`.minecraft/`）。
2. 内置规则扫描已跟踪文本文件中的常见密钥/令牌。
3. 若代理安装 `gitleaks`，使用 `.gitleaks.toml` 额外扫描当前工作树。
4. `go vet ./...`
5. `govulncheck ./...`（未安装时自动安装）
6. `npm audit --omit=dev --audit-level=high`
7. `go test -count=1 ./...`
8. `npm ci --prefix frontend` + `npm run build --prefix frontend`

安全检查构建失败会阻断该分支的后续发布准备。三个 TeamCity Release 仅作为手动回退，仍会先运行安全检查。

三个构建都调用同一个跨平台脚本：

```text
scripts/teamcity-release.mjs
```

`settings.kts` 顶部的 `version` 需要与 TeamCity 服务器版本匹配（当前为 `2026.2`）；服务器版本不同时修改该值即可。

## 启用

1. 在项目设置中打开 **Versioned Settings**。
2. 选择 **Kotlin**，Settings location 填 `.teamcity`，启用版本化配置。
3. 确认 VCS Root 指向：

   ```text
   https://git.1v.fit/xiaoman1221/MyGoLuncher.git
   branch: refs/heads/master
   ```

4. 准备三个代理：
   - **Windows**：Go 1.25+、Node.js 22+、Git、UPX、Windows SDK（`signtool.exe`）。
   - **macOS**：Go 1.25+、Node.js 22+、Git、Xcode Command Line Tools（`codesign`、`ditto`）、可选 UPX。
   - **Linux**：Go 1.25+、Node.js 22+、Git、`tar`、UPX、Wails 构建依赖（GTK3 + WebKit2GTK）、可选 `gpg`、可选 `gitleaks`。

安全检查需要访问 Go 模块代理与 npm registry，用于安装 `govulncheck` 和执行 `npm audit`。

Wails CLI 由发布脚本自动安装。

## 构建流程

每个平台构建都会：

1. 校验 `wails.json`、`package.json`、`internal/app/app.go` 等版本字段。
2. 强制要求官方产品配置，通过 `-tags mgl_private` 注入产品 Key 与授权服务地址。
3. `-trimpath`、`-s -w`、启用调试器检测。
4. 按平台加壳/签名/打包。
5. 只写入自己的平台目录：

   ```text
   releases/<版本>/windows/
   releases/<版本>/macos/
   releases/<版本>/linux/
   releases/latest-windows.json
   releases/latest-macos.json
   releases/latest-linux.json
   ```

6. 以 `[teamcity release] v<版本> <平台> build <构建号>` 提交并推送到 `master`。

各平台只修改自己的文件和 `latest-<平台>.json`，并行构建时不会互相覆盖；推送冲突会自动 rebase 重试。

## 必需参数（三个构建相同）

| 参数 | 说明 |
| --- | --- |
| `env.MGL_PRODUCT_API_KEY` | 官方产品 API Key（Password） |
| `env.MGL_PRODUCT_SERVER_URL` | 授权/更新服务地址，必须 HTTPS |
| `env.GIT_PUSH_TOKEN` | 允许推送 `master` 的令牌（Password） |
| `env.GIT_PUSH_USER` | 推送用户名，默认 `xiaoman1221` |
| `env.MGL_GIT_PUSH_URL` | 推送地址，默认仓库 HTTPS 地址 |

## 平台特定参数

### Windows

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `env.MGL_PFX_PATH` | 是 | Authenticode PFX 路径 |
| `env.MGL_SIGN_PFX_PASSWORD` | 是 | PFX 密码（Password） |
| `env.MGL_UPX_PATH` | 否 | `upx.exe` 路径；默认自动查找 |
| `env.MGL_WINDOWS_TIMESTAMP_URL` | 否 | 默认空；自签名证书留空，使用受信任证书时可填 `https://timestamp.digicert.com` |
| `env.MGL_WINDOWS_TRUST_SELF_SIGNED` | 否 | 默认 `true`；自签名证书会导入当前用户 Root/TrustedPublisher 以便 verify |

Windows 构建**强制** UPX 加壳，并在加壳后执行 Authenticode 签名与验证。`env.MGL_WINDOWS_TIMESTAMP_URL` 为空时不请求公网时间戳，避免自签名证书构建卡住或失败。

### macOS

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `env.MGL_MACOS_SIGN_IDENTITY` | 否 | `codesign` 身份名称；留空则产物未签名 |
| `env.MGL_MACOS_TIMESTAMP` | 否 | 默认 `false`；使用自签名证书时保持关闭，使用受信任 Apple 身份时可设为 `true` |
| `env.MGL_UPX` | 否 | 默认 `false`；设为 `true` 会在签名前对主二进制加壳 |

macOS 构建生成 `.app` 并压缩为 zip。设置签名身份后执行 hardened runtime 深度签名与验证。

### Linux

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `env.MGL_UPX` | 否 | 默认 `true` |
| `env.MGL_UPX_PATH` | 否 | `upx` 路径；默认自动查找 |
| `env.MGL_GPG_KEY_ID` | 否 | 设置后对 `SHA256SUMS.txt` 生成 GPG 分离签名 |

Linux 构建生成 tar.gz，内含可执行文件、`LICENSE`、`NOTICE` 与 `README.md`。

## 发布版本（方案 C）

正式发布以 GitHub Actions 为准。

先在本地提升版本并推送：

```bash
node scripts/set-version.mjs 1.0.4
git add .
git commit -m "发布 v1.0.4"
git push origin master
```

然后到 GitHub Actions：

```text
Official release -> Run workflow
version: 1.0.4
publish: true
```

工作流会：

1. 校验版本
2. 后端测试与前端构建
3. 三平台官方编译
4. Windows Authenticode / macOS codesign / Linux GPG
5. 上传 artifacts
6. 创建 GitHub Release

### GitHub Actions Secrets

```text
MGL_PRODUCT_API_KEY
MGL_PRODUCT_SERVER_URL
WINDOWS_PFX_BASE64
WINDOWS_PFX_PASSWORD
MACOS_CERTIFICATE_BASE64
MACOS_CERTIFICATE_PASSWORD
MACOS_SIGNING_IDENTITY
LINUX_GPG_PRIVATE_KEY_BASE64
LINUX_GPG_PASSPHRASE
```

可选 Repository Variables：

```text
MGL_PRODUCT_NAME
MGL_PRODUCT_SLUG
```

### TeamCity 手动回退

内网应急时仍可手动运行 TeamCity 的三个 Release 构建。它们会提交 `releases/` 到内部 Git 仓库，不应与 GitHub Release 并行使用。

## 注意事项

- GitHub Actions 发布是手动触发，避免普通提交自动发布已签名二进制。
- macOS 产物是 GitHub `macos-14` Runner 生成的 universal 包，实际架构以工作流日志为准。
- UPX 可能触发杀毒软件误报，并可能破坏 macOS 代码签名；macOS 默认关闭。
- TeamCity 三个 Release 仅作为手动回退，优先使用 GitHub Actions。
