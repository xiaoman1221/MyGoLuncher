# Security Policy

## Supported versions

只有最新发布版本和当前 `main` 分支接收安全修复。旧版本请升级。

## Reporting a vulnerability

请将漏洞报告发送到 **im@manjiuqi.com**，主题以 `[SECURITY]` 开头。不要通过公开 Issue 披露可利用细节。

报告时请包含：

- 受影响版本和平台
- 复现步骤
- 影响范围（授权绕过、更新劫持、凭据泄漏、RCE 等）
- 如有修复建议请一并提供

我们会在确认后尽快回复，并在修复发布后致谢（除非你要求匿名）。

## 安全边界

- **授权系统**：客户端授权门禁不是强 DRM；付费能力应由服务端权益校验支撑。
- **代码签名**：官方发布使用 Authenticode 签名；社区构建不冒用官方证书。
- **更新校验**：官方构建通过 HTTPS 从 LicensingGo 更新 API 获取 OTA 清单，并校验安装包 URL、SHA256 与大小；旧式静态清单仍使用 Ed25519 签名，私钥不得进入仓库或 CI 日志。
- **本地凭据**：Windows 使用 HKCU 注册表 + DPAPI；其他平台使用 AES-256-GCM + 0600 密钥文件，不要求用户密码。
- **管理员提示**：程序以 `asInvoker` 运行；检测到管理员/root 时先提示，用户可继续或退出。

## Secret handling

- 产品 API Key、市场/支付密钥、PFX、OTA 私钥、JWT secret 不得提交。
- 官方构建通过私有 CI secret 或本机环境变量注入。
- 如果密钥曾经进入 Git 历史，仅删除当前文件不够；必须 **轮换密钥**，并评估是否重写历史。
- 提交前建议运行 secret scanner（如 gitleaks）：
  ```powershell
  gitleaks detect --source . --redact
  ```

## 已知限制

- 客户端二进制无法做到绝对不可逆向；UPX、去符号、调试器检测只能提高成本。
- 自签名证书不会被 Windows 公共同信任。
- 没有用户密码的本地加密无法防御已取得同一用户权限的攻击者。
