# MyMinecraftGoLuncher（MyGoLuncher）

一个使用 **Go + Wails + React + TypeScript** 构建的 Minecraft 桌面启动器。

界面只保留启动游戏所需的基本信息：版本、账户、内存与 Java。支持黑夜 / 白天两种主题，白天模式以粉色为主色。

![黑夜模式](docs/screenshot-home-dark.png)

![白天模式](docs/screenshot-home-light.png)

## 功能

- **添加本地版本**：扫描本机官方启动器、HMCL、PCL2、Prism Launcher / MultiMC，也可以手动选择 `.minecraft`、`versions` 目录或单个版本文件夹。导入时复制存档、模组、资源包与配置，并准备游戏运行环境。
- **下载官方版本**：从 Mojang 元数据获取正式版、快照和远古版本，下载客户端、运行库、本地库与资源文件。
- **多下载源**：官方源与 BMCLAPI，安装前测速并自动回退，所有文件仍做哈希校验。
- **独立版本目录**：每个版本拥有独立的 `mods/`、`shaderpacks/`、`resourcepacks/`、`saves/` 与日志目录。
- **加载器**：Fabric、Quilt、Forge、NeoForge。
- **账户**：
  - Microsoft 正版登录（设备码 OAuth）。
  - 外置登录，内置 **LittleSkin**（`https://littleskin.cn/api/yggdrasil`）与 **YggdrasilSkinServer**（`https://yss.1v.fit`），也兼容其他 Authlib-Injector 服务器。
  - 离线账户，可添加、重命名和切换。
- **Java**：自动扫描本机 Java，按版本元数据匹配；缺失时从 Adoptium 下载 Temurin JRE。
- **游戏控制台**：实时日志、错误高亮、复制日志、结束游戏进程。
- **主题**：黑夜模式（荧光绿）/ 白天模式（粉色），主题保存在本机。
- **首页“你知道吗”**：默认从 HitokotoGo 获取一句“一言”，地址为 `https://hitokotogo.yhdzz.cn/v2`，可在设置中更换。接口文档：<https://hitokotogo.yhdzz.cn/docs>。

## 技术栈

| 层 | 技术 |
| --- | --- |
| 桌面壳 | Wails v2.17 |
| 后端 | Go 1.25+ |
| 前端 | React 19 + TypeScript + Vite |
| 图标 | Lucide |
| 数据源 | Mojang Piston Meta、Adoptium API、Fabric / Quilt Meta、Forge / NeoForge 官方 Maven |

## 开发

### 依赖

- Go 1.25 或更高版本
- Node.js 20 或更高版本
- Wails CLI v2.17

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
```

### 运行

```bash
cd frontend
npm install
cd ..
wails dev
```

### 构建

```bash
wails build
```

构建产物位于 `build/bin/`。Windows 可执行文件名为 `MyMinecraftGoLuncher.exe`。

仅调试前端界面时：

```bash
cd frontend
npm run dev
```

浏览器中会启用演示模式，后端 API 由 `src/lib/mock.ts` 提供模拟数据。

### 测试

```bash
go test ./...
go vet ./...
cd frontend && npm run build
```

## 便携目录

所有数据都与可执行文件放在同一目录，移动整个文件夹即可迁移：

```text
<程序所在目录>/
├── MyMinecraftGoLuncher.exe
├── .minecraft/          # 游戏下载、版本与实例数据
└── MyGoLuncher/         # 启动器配置
    ├── accounts.json
    ├── instances.json
    ├── settings.json
    └── cache/
```

> 程序目录必须可写。若放在 `Program Files` 等受保护目录，启动器将无法创建上述目录。
>
> 可用环境变量 `MYGOLUNCHER_HOME` 覆盖基础目录。旧版本使用的 `MGL` 配置目录会在首次启动时自动迁移为 `MyGoLuncher`。

## 账户说明

### Microsoft 正版登录

- 使用设备码登录，无需内置客户端密钥。
- 启动前自动刷新过期的 Minecraft 访问令牌。
- 内置公共 OAuth Client ID 可能受 Minecraft Services 白名单限制；如返回 `Invalid app registration`，请使用外置或离线登录。

### 外置登录

- 默认提供 LittleSkin 与 YggdrasilSkinServer 两个预设。
- 密码不会保存，只在本次登录请求中使用。
- 启动时自动下载并注入 `authlib-injector`。

### 离线登录

- 可创建多个离线身份；每个名称生成标准离线 UUID。
- 离线账户无法加入开启正版验证的服务器。

## 已知限制

- 账户令牌保存在本机 `accounts.json` 中，请勿在共享电脑上保存私人账户。
- 外置服务器若要求两步验证或特殊登录流程，需要按其文档处理。
- 资源中心、整合包导入 / 导出等功能的后端代码仍然保留，但当前界面未提供入口。
- 本项目不是 Mojang Studios 或 Microsoft 的官方产品。Minecraft 是 Mojang Studios 的商标。
