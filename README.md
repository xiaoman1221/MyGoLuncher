# MyMinecraftGoLuncher（MyGoLuncher）

[![Powered by OrcaRouter](https://img.shields.io/badge/Powered_by-OrcaRouter-2563eb)](https://www.orcarouter.ai/ref/ref_00ee071bfd29fc8bb954)

使用 **Go + Wails + React + TypeScript** 构建的跨平台 Minecraft 桌面启动器。界面简洁，支持深浅主题、五种语言、版本管理、资源中心、整合包与局域网联机。

![黑夜模式](docs/screenshot-home-dark.png)

![白天模式](docs/screenshot-home-light.png)

![English](docs/screenshot-home-en.png)

## 功能概览

- **版本管理**：扫描官方启动器、HMCL、PCL2、Prism/MultiMC、Modrinth App、CurseForge、ATLauncher、GDLauncher、FTB App，也可以从任意目录批量导入；支持版本隔离、修复、模组/资源包/光影启停与模组更新。
- **版本下载**：从 Mojang 获取正式版、快照与远古版本，下载客户端、运行库与资源文件；下载与修复免费。
- **多下载源**：官方源、BMCLAPI、清华 Adoptium 与自定义镜像，自动测速回退，文件哈希校验。资源下载另支持 MCIM 镜像。
- **加载器**：Fabric、Quilt、Forge、NeoForge；自动匹配版本并优先使用官方安装器，旧版 Forge 提供兼容安装。
- **资源中心**：搜索并安装 Modrinth / CurseForge 的模组、光影、资源包、数据包与整合包；支持中文别名搜索、依赖安装、冲突检测和 AI 搜索（DeepSeek / Anthropic / OrcaRouter）。
- **整合包与服务器包**：导入 `.mrpack`、CurseForge `.zip`、Prism/MultiMC `.zip`；导出 `.mrpack`、完整备份 `.zip` 与服务器包，并自动剔除仅客户端模组。
- **服务器管理**：绑定 MCSM 实例，增量同步文件，创建回滚快照，远程启动/停止/重启。
- **局域网联机**：自动发现同网段玩家与开放到局域网的房间；支持版本共享、文字聊天、WebRTC 语音与最大 512 MB 的文件传输。
- **账户**：Microsoft 设备码登录、外置登录（LittleSkin / YggdrasilSkinServer / Authlib-Injector）、YSS 一键授权与多离线账户。
- **桌面体验**：后台任务日志与取消、实时游戏控制台、启动动画、游戏语言同步、系统托盘、退出确认与热更新。
- **安全存储**：Windows 使用注册表 + DPAPI；其他平台使用 AES-256-GCM 加密文件。

完整说明、免费/付费边界与已知限制见 [Function.md](Function.md)。

## 技术栈

| 层 | 技术 |
| --- | --- |
| 桌面壳 | Wails v2.17 |
| 后端 | Go 1.25+ |
| 前端 | React 19 + TypeScript + Vite |
| 图标 | Lucide |
| 数据源 | Mojang Piston Meta、Adoptium、Fabric/Quilt Meta、Forge/NeoForge Maven、Modrinth、CurseForge |

## 快速开始

依赖：Go 1.25+、Node.js 20+、Wails CLI v2.17。

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0

cd frontend && npm install && cd ..
wails dev      # 开发
wails build    # 构建，产物在 build/bin/
```

仅调试前端时执行 `cd frontend && npm run dev`，浏览器中会启用模拟后端。

```bash
go test ./...
go vet ./...
cd frontend && npm run build
```

## 便携目录

程序数据默认与可执行文件放在同一目录，移动整个文件夹即可迁移：

```text
<程序目录>/
├── MyMinecraftGoLuncher.exe
├── .minecraft/          # 标准游戏目录（versions/ libraries/ assets/）
└── MGL/                 # 非敏感配置与缓存
    ├── instances.json
    ├── servers.json
    ├── settings.json
    └── cache/
```

可用 `MGL_HOME` 覆盖基础目录，`MYGOLUNCHER_HOME` 为兼容别名。旧版 `MyGoLuncher` 配置目录会自动迁移。程序目录必须可写。

## 授权

本项目以 **AGPL-3.0** 发布，并提供商业授权；商业服务端、官方发布密钥与签名证书保持闭源。

- **免费**：启动与版本管理、下载/修复 Minecraft、Java 管理、基础模组/资源安装、本地备份。
- **付费**：服务器管理（`server`）、局域网联机（`lan`）、AI 搜索（`ai`）、云同步（`cloud`）、整合包 CDN（`cdn`）、服务器托管（`hosting`）、优先支持（`priority`）。
- 客户端检查仅用于界面提示，真正权益校验在服务端。

详见 [docs/OPEN_CORE.md](docs/OPEN_CORE.md)。

## 文档

- [Function.md](Function.md)：功能说明、免费/付费边界与已知限制
- [docs/GITHUB_RELEASE.md](docs/GITHUB_RELEASE.md)：官方构建与发布
- [CONTRIBUTING.md](CONTRIBUTING.md)、[SECURITY.md](SECURITY.md)、[TRADEMARK.md](TRADEMARK.md)、[CLA.md](CLA.md)

## 声明

本项目不是 Mojang Studios 或 Microsoft 的官方产品。Minecraft 是 Mojang Studios 的商标。
