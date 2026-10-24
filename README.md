# MyMinecraftGoLuncher（MyGoLuncher）

一个使用 **Go + Wails + React + TypeScript** 构建的 Minecraft 桌面启动器。

界面保持简洁，同时提供版本设置、资源中心、整合包与 Java 运行时管理。支持黑夜 / 白天两种主题，白天模式以粉色为主色。

![黑夜模式](docs/screenshot-home-dark.png)

![白天模式](docs/screenshot-home-light.png)

![English](docs/screenshot-home-en.png)

## 功能

- **添加本地版本**：扫描本机官方启动器、HMCL、PCL2、Prism Launcher / MultiMC，也可以手动选择 `.minecraft`、`versions` 目录或单个版本文件夹。导入时复制存档、模组、资源包与配置，并准备游戏运行环境。
- **下载官方版本**：从 Mojang 元数据获取正式版、快照和远古版本，下载客户端、运行库、本地库与资源文件。
- **多下载源**：官方源与 BMCLAPI，安装前测速并自动回退，所有文件仍做哈希校验。
- **版本设置**：在左侧「版本设置」中统一管理已安装版本，可修改名称、内存与 Java，查看游戏目录。
- **版本隔离**：可在设置中开启或关闭。开启时每个版本拥有独立的 `mods/`、`shaderpacks/`、`resourcepacks/`、`saves/` 与日志目录；关闭后所有版本共用 `.minecraft`。
- **加载器**：Fabric、Quilt、Forge、NeoForge。
- **资源中心**：从 Modrinth 与 CurseForge 搜索并安装模组、光影、资源包、数据包与整合包；可在设置中填写 CurseForge API Key。
  - 内置常见中文别名（如“钠”“小地图”“苹果皮”），无需 AI 也可搜索。
  - 可选接入 AI 搜索：默认使用 **DeepSeek**（`deepseek-chat`），也支持 **Anthropic Claude**；在「设置 → AI 搜索」中选择提供商并填写对应 API Key。
- **整合包与服务器包**：
  - 导入 `.mrpack`、CurseForge `.zip`、Prism / MultiMC 或通用 `.zip`。
  - 一键导出 `.mrpack` 或完整备份 `.zip`。
  - 一键导出服务器包 `.zip`：包含加载器运行时或安装器、`mods/`、`config/`、`defaultconfigs/`、`scripts/`、`kubejs/`、`datapacks/`、`server.properties`、启动脚本与 EULA 模板。
- **账户**：
  - Microsoft 正版登录（设备码 OAuth）。
  - 外置登录，内置 **LittleSkin**（`https://littleskin.cn/api/yggdrasil`）与 **YggdrasilSkinServer**（`https://yss.1v.fit`），也兼容其他 Authlib-Injector 服务器。
  - **YSS 一键授权**：选择 YggdrasilSkinServer 后可使用 OAuth2 设备码流程，在系统浏览器中登录并确认，无需输入站点密码。需要在 YSS 管理端启用「启动器 OAuth2」。
  - 离线账户，可添加、重命名和切换。
- **Java**：自动扫描本机 Java，按版本元数据匹配；缺失时从 Adoptium 下载 Temurin JRE。
- **游戏控制台**：实时日志、错误高亮、复制日志、结束游戏进程。
- **启动动画**：点击启动后显示旋转方块与进度动画；检测到游戏窗口出现后动画结束。
- **游戏语言同步**：启动游戏前将 Minecraft 语言设置为启动器当前界面语言（`zh_cn` / `zh_tw` / `en_us` / `ja_jp` / `ko_kr`），写入实例的 `options.txt`。
- **启动后隐藏**：检测到游戏窗口真正出现后才隐藏启动器；游戏窗口关闭（进程退出）后自动恢复启动器窗口。可在设置中关闭。
- **主题**：黑夜模式（荧光绿）/ 白天模式（粉色），主题保存在本机。
- **多语言**：简体中文、繁體中文、English、日本語、한국어。默认跟随系统语言，无法识别时回退简体中文；可在设置中关闭自动跟随并固定启动语言。
- **内置窗口控制**：隐藏系统标题栏，顶部栏可拖动窗口，并提供最小化、最大化 / 还原和关闭按钮。
- **首页“你知道吗”**：默认从 HitokotoGo 获取一句“一言”，地址为 `https://hitokotogo.yhdzz.cn/v2`，可在设置中更换。接口文档：<https://hitokotogo.yhdzz.cn/docs>。

## 技术栈

| 层 | 技术 |
| --- | --- |
| 桌面壳 | Wails v2.17 |
| 后端 | Go 1.25+ |
| 前端 | React 19 + TypeScript + Vite |
| 图标 | Lucide |
| 数据源 | Mojang Piston Meta、Adoptium API、Fabric / Quilt Meta、Forge / NeoForge 官方 Maven、Modrinth API、CurseForge API |

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
└── MGL/                 # 仅存放非敏感配置
    ├── instances.json
    ├── settings.json
    └── cache/
```

> 程序目录必须可写。若放在 `Program Files` 等受保护目录，启动器将无法创建上述目录。
>
> 可用环境变量 `MGL_HOME` 覆盖基础目录，`MYGOLUNCHER_HOME` 作为兼容别名。
>
> 旧版本使用过的 `MyGoLuncher` 临时配置目录会在首次启动时自动迁移回 `MGL`。

## 安全存储

账户、Microsoft/Yggdrasil 令牌、外置登录令牌、CurseForge API Key 与 AI API Key 不再写入 `MGL`。

- **Windows**：加密后保存在当前用户的注册表
  `HKEY_CURRENT_USER\Software\MyGoLuncher\Secrets`，使用 Windows DPAPI（当前用户范围）保护。
- **macOS / Linux**：保存在用户配置目录下的 AES-GCM 加密文件（密钥文件权限 0600），不会写入程序的 `MGL` 目录。
- 旧版 `accounts.json` 会自动迁移到加密存储并删除；`settings.json` 中的旧 API Key 也会迁移并清除。
- `MGL` 仅保留语言、主题、内存、Java 路径、下载源、实例列表、缓存等非敏感信息。

> 加密数据与当前操作系统用户绑定。更换 Windows 用户或迁移到其他电脑后，需要重新登录账户并重新填写 API Key。

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

- 界面已完成基础多语言适配；下载、安装与登录流程中的详细错误信息和后台任务标题仍由后端以中文返回。
- 账户令牌与 API Key 保存在 Windows 注册表的加密数据中；它们与当前 Windows 用户绑定，请勿在共享系统账户下保存私人账户。
- 外置服务器若要求两步验证或特殊登录流程，需要按其文档处理。
- YSS 一键授权依赖站点启用「启动器 OAuth2」；若站点未启用，启动器会给出提示，仍可使用用户名 / 密码外置登录。
- 关闭版本隔离后，所有版本共用 `.minecraft` 中的模组、存档与配置，删除版本时也不会删除共享游戏目录。
- 资源中心使用 CurseForge 内容时需要填写自己的 CurseForge API Key；未填写时仅显示 Modrinth 结果。
- AI 搜索需要自备 DeepSeek 或 Anthropic API Key，搜索关键词会发送到所选提供商的 API。未配置或调用失败时会回退到本地别名匹配。API Key 加密保存在系统安全存储中，不写入 `MGL`。
- 服务器包会包含实例中的全部模组；仅客户端模组可能需要手动从 `mods/` 中移除。Forge / NeoForge / Quilt 服务器包首次使用需运行 `install-server` 脚本。
- 服务器包不会自动接受 Minecraft EULA，请阅读后自行将 `eula.txt` 中的 `eula=false` 改为 `true`。
- 每次启动游戏都会把当前启动器语言写入 `options.txt`；如果之后在游戏内修改语言，下次通过启动器启动时会被重新同步。
- 本项目不是 Mojang Studios 或 Microsoft 的官方产品。Minecraft 是 Mojang Studios 的商标。
