# Contributing to MyGoLuncher

感谢参与。本项目采用 **AGPL-3.0**，并保留商业授权；提交贡献前请阅读本文件。

## 开发环境

- Go 1.25+
- Node.js 20+
- Wails CLI 2.17
- Windows 10/11（发布构建）；Linux/macOS 可参与大部分开发和测试

## 本地构建

```powershell
# 前端
cd frontend
npm ci
npm run build
cd ..

# Go 测试
go vet ./...
go test -count=1 ./...

# 社区构建（没有商业授权服务）
wails build

# 官方/内部构建（需要私有产品配置）
$env:MGL_PRODUCT_API_KEY = "..."
$env:MGL_PRODUCT_SERVER_URL = "https://..."
.\scripts\build-release.ps1 -Pack
```

发布构建、加壳与签名见 `scripts/build-release.ps1`。**不要**把私钥、PFX、产品 API Key 写进脚本或提交到仓库。

## 开源/商业边界

免费功能必须无条件可用：

- 启动游戏
- 版本管理
- 基础模组安装

付费功能通过授权 features 解锁：

- `server`：服务器管理
- `lan`：局域网联机
- `ai`：AI 搜索
- `cloud`：云同步/云备份
- `cdn`：整合包 CDN
- `hosting`：服务器托管
- `priority`：优先支持

客户端中的功能检查只用于界面体验；真正的付费价值必须在服务端实现。不要提交依赖客户端隐藏来实现的“商业秘密”。

## 提交规范

- 使用 DCO（Developer Certificate of Origin）：提交信息末尾加 `Signed-off-by: Name <email>`。
- 新增贡献默认以 AGPL-3.0 授权，并允许项目维护者用于商业授权版本；详见 [CLA.md](CLA.md)。
- 不要提交以下内容：
  - 产品 API Key、JWT secret、管理员密码
  - PFX / OTA 私钥
  - 用户激活码、机器码、令牌
  - 带有个人数据的日志或截图
- 修改付费边界时，同时更新后端 `requireFeature`、前端锁定状态和 `docs/OPEN_CORE.md`。
- 涉及用户数据、授权、更新、签名、加壳的改动必须附测试或明确的手工验证步骤。

## 代码风格

- Go：`gofmt`、`go vet`，错误信息面向用户时使用中文。
- TypeScript/React：随现有风格，不引入不必要的全局状态。
- 新增面向用户的文字必须补齐五种语言（简中、繁中、英文、日文、韩文）。

## 安全问题

请勿公开提交安全漏洞；按 [SECURITY.md](SECURITY.md) 私下报告。
