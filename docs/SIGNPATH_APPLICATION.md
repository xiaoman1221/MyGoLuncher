# SignPath Foundation 申请材料（草稿）

> 申请前请先向 SignPath 确认：本项目客户端采用 AGPL-3.0 开源，但提供付费的服务器管理、云服务、托管和优先支持。若免费签名计划要求“完全非商业”，本项目可能不符合资格。**不要**隐瞒商业活动。

## 项目信息

- 项目名称：MyGoLuncher
- 项目主页：`https://github.com/<owner>/MyGoLuncher`
- 源代码仓库：`https://github.com/<owner>/MyGoLuncher`
- 许可证：GNU Affero General Public License v3.0（AGPL-3.0）
- 项目类型：Minecraft 启动器，Windows 桌面客户端（Go + Wails + React）
- 维护者：MyGoLuncher / xiaoman1221
- 联系邮箱：`im@manjiuqi.com`

## 项目说明

MyGoLuncher 是一个开源的 Minecraft 启动器，提供版本管理、模组安装、Java 管理、整合包导入和服务器管理。客户端源代码公开；商业服务端、支付后台、云同步、CDN、托管服务和发布签名密钥不公开。

## 构建与产物

- CI：TeamCity，Windows runner（详见 `.teamcity/README.md`）
- 前端：`cd frontend && npm ci && npm run build`
- 后端：`go test ./...`
- 桌面产物：`wails build`（社区构建，不含商业产品配置）
- 签名目标：`build/bin/MyMinecraftGoLuncher.exe`
- 哈希算法：SHA-256 Authenticode
- 时间戳：RFC 3161 / Authenticode timestamp
- 可复现性：公开工作流只使用仓库内代码；产品 API Key 和服务地址通过私有 secret 注入官方构建

## 签名请求

- 签名类型：Windows Authenticode PE
- 摘要：SHA-256
- 时间戳：需要
- 是否需要 EV：以 SignPath 建议为准
- 发布方式：TeamCity 构建产物提交到仓库 `releases/` 目录
- 签名后校验：`signtool verify /pa /v`

## 商业披露

- 客户端：AGPL-3.0 开源
- 付费能力：服务器管理、局域网联机、AI 搜索、云同步/备份、整合包 CDN、服务器托管、优先支持
- 付费能力通过 LicensingGo 激活码的 `features` 控制；真正的付费服务在服务端鉴权
- 免费功能：启动游戏、版本管理、基础模组安装
- 没有把开源代码隐藏在付费墙后；商业版本主要是官方服务、托管和内容

## 申请时需要向 SignPath 明确的问题

1. 以捐赠/付费服务支持的 AGPL 项目是否符合 Foundation 资格？
2. 官方构建包含私有产品配置（不含私钥）是否影响签名？
3. 是否要求项目整体（包括服务端）开源？
4. 是否接受“官方构建”和“社区构建”使用不同签名/发布渠道？

## 如果申请被拒

按商业发布处理：

- 购买 OV/EV Code Signing 证书；
- 或使用 Azure Trusted Signing；
- 或继续发布未签名社区构建，并明确说明 SmartScreen 风险。

## 附：代码签名要求

- 证书必须包含 Code Signing EKU
- PFX/Token 私钥不得提交仓库
- 先 UPX 加壳，再 Authenticode 签名
- 签名后使用 `signtool verify /pa` 验证
