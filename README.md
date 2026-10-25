# MyMinecraftGoLuncher（MyGoLuncher）

[![Powered by OrcaRouter](https://img.shields.io/badge/Powered_by-OrcaRouter-2563eb)](https://www.orcarouter.ai/ref/ref_00ee071bfd29fc8bb954)

一个使用 **Go + Wails + React + TypeScript** 构建的 Minecraft 桌面启动器。

界面保持简洁，同时提供版本设置、资源中心、整合包与 Java 运行时管理。支持黑夜 / 白天两种主题，白天模式以粉色为主色。

![黑夜模式](docs/screenshot-home-dark.png)

![白天模式](docs/screenshot-home-light.png)

![English](docs/screenshot-home-en.png)

## 功能

- **添加本地版本**：扫描本机官方启动器、HMCL、PCL2、Prism Launcher / MultiMC、Modrinth App、CurseForge、ATLauncher、GDLauncher 与 FTB App，也可以手动选择 `.minecraft`、`versions`、`instances` / `profiles` 目录或单个版本/实例文件夹。支持一次勾选多个版本批量导入；「自定义导入」可手动填写版本名称、Minecraft 版本、加载器与可选的来源目录，并支持递归搜索指定目录下的 `.minecraft` / `versions` 文件夹、自动识别版本并填入信息。缺少版本 JSON 的版本文件夹会从名称推断版本并给出警告；导入时复制存档、模组、资源包、`kubejs`、`scripts`、`datapacks` 与配置，并准备游戏运行环境。
- **下载官方版本**：从 Mojang 元数据获取正式版、快照和远古版本，下载客户端、运行库、本地库与资源文件。
- **多下载源**：官方源、BMCLAPI、清华 Adoptium 与自定义镜像，安装前按当前网络测速并自动回退，所有文件仍做哈希校验。游戏本体、Fabric/Quilt/Forge/NeoForge 的元数据与安装器、以及 Adoptium Java 归档都会优先尝试镜像；自定义 proxy 镜像也会参与 Java API/归档测速。整合包与资源下载另支持 MCIM 等 Modrinth / CurseForge 镜像，可在设置中配置默认源，也可在导入/安装时临时切换。
- **版本设置**：在左侧「版本设置」中统一管理已安装版本，可修改名称、内存与 Java，查看游戏目录。
- **版本隔离**：可在设置中开启或关闭。开启时每个版本在 `.minecraft/versions/<版本名>/` 中拥有独立的 `mods/`、`shaderpacks/`、`resourcepacks/`、`saves/` 与日志目录；关闭后所有版本共用 `.minecraft`。版本文件夹同时包含可被其他启动器识别的 `<版本名>.json` 和 `<版本名>.jar`。
- **加载器**：Fabric、Quilt、Forge、NeoForge。
- **资源中心**：从 Modrinth 与 CurseForge 搜索并安装模组、光影、资源包、数据包与整合包；可在设置中填写 CurseForge API Key。
  - 中文搜索：内置大量模组别名与类型词（如“钠”“小地图”“苹果皮”“机械动力”“农夫乐事”“物品管理器”），会同时用规范化英文词和原文搜索，并把项目名/短名精确匹配的结果排在兼容补丁和附属模组之前；不开启 AI 也能使用，未命中的关键词可使用 AI 搜索，或直接输入英文名。
  - 安装前会扫描已安装模组的 Fabric / Quilt / Forge / NeoForge 元数据，检测重复模组文件、`breaks` 声明和版本不兼容依赖；检测到冲突时取消安装并给出说明，确认风险后可继续。
  - 自动下载必填前置依赖（Modrinth `required` 与 CurseForge `RequiredDependency`），并在安装结果中提示依赖数量。
  - 模组更新：在「版本设置」点击「检查更新」，通过 Modrinth SHA-1 与 CurseForge 文件指纹识别已安装模组，查询兼容的最新版本并显示当前/最新版本；替换前会下载校验、复用冲突检测，并把旧 jar 原地改名为 `xxx.jar.old`（已有同名备份时使用 `.old.1`），不删除旧文件。回滚时关闭游戏，把当前新 jar 改名或移走，再把 `.old` 改回 `.jar` 即可。`.old`（以及手动使用的 `.dis`）不会参与模组扫描、局域网同步、服务器同步或整合包导出。
  - 可选接入 AI 搜索：默认使用 **DeepSeek**（`deepseek-chat`），也支持 **Anthropic Claude** 与 **OrcaRouter**（`orcarouter/auto`，Responses API）；在「设置 → AI 搜索」中选择提供商并填写 API Key。
  - OrcaRouter 支持 PKCE 一键授权：在系统浏览器中确认后自动获取并加密保存 API Key。
- **整合包与服务器包**：
  - 导入 `.mrpack`、CurseForge `.zip`、Prism / MultiMC 或通用 `.zip`。
  - 一键导出 `.mrpack` 或完整备份 `.zip`。`.mrpack` 可被 Modrinth App、Prism 等支持该格式的启动器导入；`.zip` 备份采用 Prism / MultiMC 实例布局，可直接导入对应启动器。导出 `.mrpack` 时会自动排除日志、存档和本机选项。
  - 一键导出服务器包 `.zip`：包含加载器运行时或安装器、`mods/`、`config/`、`defaultconfigs/`、`scripts/`、`kubejs/`、`datapacks/`、`server.properties`、启动脚本与 EULA 模板。导出时会根据 Fabric / Quilt / Forge / NeoForge 元数据自动剔除仅客户端模组，并把剔除记录写入 `README.txt`。
- **服务器管理（MCSM）**：添加多个 MCSManager 面板与实例，绑定本地版本后，把 `mods/`、`config/`、`defaultconfigs/`、`scripts/`、`kubejs/`、`datapacks/`、`resourcepacks/`、`shaderpacks/` 增量同步到服务器；支持远程启动、停止、重启、手动勾选要更新的文件，以及可选删除远端多余文件。每次同步前会在服务器上创建 `.mgl-backups/` 回滚快照，保留最近 5 份，可一键恢复。
- **局域网联机**：启动后自动发现同一局域网内使用本启动器的其他客户端（侧边栏显示数量），并自动发现「对局域网开放」的 Minecraft 世界和局域网服务器，显示版本、MOTD 与在线玩家。选择本地版本即可一键直连，也支持手动输入地址。
- **局域网版本同步**：可把本地版本的 `mods/`、`shaderpacks/`、`resourcepacks/`、`config/` 共享给同一局域网的其他客户端；对方在联机页选择本地版本后，启动器会先比较 Minecraft 版本、加载器版本以及新增/变更文件，确认差异后再增量同步，SHA-256 校验，只下载缺失或变化的文件。共享清单在确认后发生变化时必须重新确认。加载器版本不同时，确认后会同步更新本地加载器。
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
- **MyGO 彩蛋**：启动/加载文案会偶尔穿插「为什么要演奏春日影！」与「迷子でもいい、迷子でも進め。」，侧边栏 MYGO 标记悬停也可看到后一句；仅在界面文案中出现，不会写入日志或影响启动。
- **内置窗口控制**：隐藏系统标题栏，顶部栏可拖动窗口，并提供最小化、最大化 / 还原和关闭按钮。
- **内容保护**：全局禁止选择、复制、剪切、拖拽文字和图像，并拦截右键菜单与 `Ctrl/Cmd+C`、`Ctrl/Cmd+X`、`Ctrl/Cmd+A` 等快捷键；输入框仍可输入和粘贴，但不能选中或复制。设备码与日志复制按钮已移除，需要手动输入或记录。截图、系统级录屏与开发者工具无法由 WebView 应用层完全阻止。
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
├── .minecraft/          # 标准 Minecraft 游戏目录
│   ├── assets/          # 资源索引与 objects
│   ├── libraries/       # Maven 运行库
│   ├── versions/        # 每个版本/实例一个标准版本文件夹
│   │   └── <版本名>/
│   │       ├── <版本名>.json   # 合并后的完整版本档案
│   │       ├── <版本名>.jar    # 客户端主文件（硬链接或复制）
│   │       ├── mods/ config/ saves/ ...
│   │       └── natives/ logs/
│   └── launcher_profiles.json  # 自动生成的标准启动器档案
└── MGL/                 # 仅存放非敏感配置
    ├── instances.json
    ├── servers.json     # MCSM 服务器绑定（不含 API Key）
    ├── settings.json
    └── cache/
```

> 程序目录必须可写。若放在 `Program Files` 等受保护目录，启动器将无法创建上述目录。
>
> `.minecraft/launcher_profiles.json` 会自动为每个版本写入标准档案（`lastVersionId` 指向 `versions/<版本名>`，`gameDir` 指向同一文件夹），同时生成完整版本 JSON 和客户端 jar。因此把整个 `.minecraft` 文件夹交给其他用户后，PCL/HMCL/Prism/MultiMC 和读取该档案格式的启动器可以直接识别并启动这些版本。旧版 `instances/<id>` 目录会在启动时自动迁移到 `versions/<版本名>`；移动文件夹后如档案丢失，可在「设置 → 游戏目录」点击「生成兼容档案」重新生成。
>
> 本地版本支持多选批量导入；未被扫描器识别的版本可使用「自定义导入」，手动填写 Minecraft 版本、加载器与来源目录，或选择包含 `.minecraft` / `versions` 的父目录后搜索并识别版本。
>
> 可用环境变量 `MGL_HOME` 覆盖基础目录，`MYGOLUNCHER_HOME` 作为兼容别名。
>
> 旧版本使用过的 `MyGoLuncher` 临时配置目录会在首次启动时自动迁移回 `MGL`。

## 开源与商业授权

本项目采用 **AGPL-3.0**，并提供商业授权。客户端源码公开，商业服务端和官方发布密钥保持闭源。详见 [docs/OPEN_CORE.md](docs/OPEN_CORE.md)。

- **免费功能**：启动已安装的游戏、已安装版本管理、Java 管理、基础模组/资源安装、本地备份。
- **付费功能**：游戏版本下载（`versions`）、服务器管理（`server`）、局域网联机（`lan`）、AI 搜索（`ai`）、云同步/备份（`cloud`）、整合包 CDN（`cdn`）、服务器托管（`hosting`）、优先支持（`priority`）。
- 已安装版本的启动、设置修改、模组/资源安装、存档编辑不受 `versions` 限制；只有下载新的 Minecraft 游戏版本需要授权。
- 免费功能不再需要激活；只有付费 API 调用 `requireFeature`。客户端检查只是 UI 辅助，真正的权益校验在服务端。
- LicensingGo 激活码的 `features` 数组必须包含 `versions`、`server`、`lan`、`ai` 等对应值，付费功能才会解锁；`*` / `pro` 可作为全量权益。
- 社区构建不包含产品 API Key 和授权服务地址，付费入口显示 PRO 提示；官方构建通过私有 CI secret 注入产品配置。
- 官方构建的签名证书、OTA 私钥、发布密钥不进入公开仓库。

### 授权数据

- 用户输入激活码后按 LicensingGo 机器码规范计算 `lgmc1` 机器码并提交激活。
- 激活码、机器码、解绑密钥、到期时间、features 与最近校验时间通过 `secretstore` 保存。
- Windows 只写入当前用户注册表 `HKCU\Software\MyGoLuncher\Secrets` 并用 DPAPI 加密；其他平台使用 AES-256-GCM 文件 + 0600 密钥文件。不会写入 `MGL`。
- 启动时在线校验；网络失败时，若上次成功校验在 7 天内，进入离线宽限期，否则要求重新联网。

### 试用

- 每个激活码/机器可使用 **3 次**试用，每次 **7 天**，总计最多 21 天。
- 试用状态与正式授权分开加密保存，不需要激活码；官方构建可直接在授权弹层开始试用。
- 试用期间解锁全部付费功能；正式授权激活后会结束当前试用，剩余次数保留。
- 社区构建没有商业授权服务，不能开始试用。
- 当前试用次数保存在本机安全存储中；清空注册表/安全存储可以重置，因此它适合作为体验入口，不是强防盗措施。需要强约束时应把试用记录放到 LicensingGo 服务端。

### 官方构建

```powershell
$env:MGL_PRODUCT_API_KEY = "官方构建专用产品 Key"
$env:MGL_PRODUCT_SERVER_URL = "https://license.example.com"
$env:MGL_SIGN_PFX_PASSWORD = "PFX 密码"

.\scripts\build-release.ps1 `
  -PfxPath ".\certs\codesign.pfx" `
  -Pack
```

- 不设置 `MGL_PRODUCT_API_KEY` / `MGL_PRODUCT_SERVER_URL` 时生成社区构建，免费功能完整，付费功能不可用。
- `-LicenseServerUrl` 仍可作为 `MGL_PRODUCT_SERVER_URL` 的别名。
- 发布构建启用 `-trimpath`、去除符号表，并通过 `-X .../security.Enforce=true` 开启调试器检测。
- `-Pack` 使用 UPX 加壳；加壳后如需代码签名必须重新签名。
- 代码签名使用 `signtool` 和受信任的 Authenticode 证书；证书密码只从环境变量读取。
- 当前内置更新地址为空（OTA 暂定），设置页会显示未配置；确定地址后通过 `-UpdateManifestUrl` 注入。
- 如果检测到进程以 Windows 管理员或 Unix root 身份运行，界面会先显示风险提示；用户可以选择继续本次运行或直接退出。程序清单仍为 `asInvoker`，不会主动请求提权。

贡献前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)、[SECURITY.md](SECURITY.md)、[TRADEMARK.md](TRADEMARK.md) 与 [CLA.md](CLA.md)。

## 热更新与 OTA

更新清单和安装包使用 Ed25519 签名，客户端只内置公钥；发布私钥必须离线保管。默认构建没有配置更新地址，设置页会提示未配置。

- `CheckForUpdates` 请求清单 JSON，并读取响应头 `X-MGL-Signature`，或同目录的 `<manifest>.json.sig`（Base64 Ed25519 签名）。
- 清单示例：

```json
{
  "version": "1.1.0",
  "minVersion": "1.0.0",
  "notes": "修复若干问题",
  "mandatory": false,
  "web": {
    "version": "1.1.0",
    "url": "https://updates.example.com/web-1.1.0.zip",
    "sha256": "…",
    "size": 123456
  },
  "app": {
    "version": "1.1.0",
    "url": "https://updates.example.com/MyMinecraftGoLuncher-1.1.0.exe",
    "sha256": "…",
    "size": 12345678
  }
}
```

- 前端热更新：下载 `frontend/dist` 的 zip，校验后解压到 `MGL/web`，Wails 重新加载 WebView；旧版本保留为 `MGL/web.old`，可在设置页回滚。
- 完整 OTA：下载并校验新 exe，生成本地 PowerShell 替换脚本，退出后替换并重启；旧 exe 保留 `.bak`。
- 发布构建时注入清单地址：

```powershell
$env:MGL_PRODUCT_API_KEY = "官方构建专用产品 Key"
$env:MGL_PRODUCT_SERVER_URL = "https://license.example.com"

.\scripts\build-release.ps1 `
  -UpdateManifestUrl "https://updates.example.com/manifest.json" `
  -PfxPath ".\certs\codesign.pfx"
```

- 签名清单：

```powershell
node .\scripts\sign-update-manifest.mjs .\dist\manifest.json
```

- 更新服务器必须提供 HTTPS 和正确的 `Content-Type`；清单、签名和安装包均应做缓存控制。客户端会校验签名和 SHA256，但不会信任未签名清单。

## 安全存储

账户、Microsoft/Yggdrasil 令牌、外置登录令牌、CurseForge API Key、AI API Key 与 MCSM API Key 不再写入 `MGL`。

- **Windows**：加密后保存在当前用户的注册表
  `HKEY_CURRENT_USER\Software\MyGoLuncher\Secrets`，使用 Windows DPAPI（当前用户范围）保护。
- **macOS / Linux / BSD**：保存在用户配置目录下的 AES-256-GCM 加密文件，配套的 256 位随机密钥文件权限为 0600，目录权限为 0700；不需要用户设置或输入任何密码，也不会写入程序的 `MGL` 目录。当前方案不依赖桌面密钥环，避免在无桌面、SSH 或密钥环锁定环境中弹出密码框。
- 旧版 `accounts.json` 会自动迁移到加密存储并删除；`settings.json` 中的旧 API Key 也会迁移并清除。
- 每个 MCSM 服务器配置的 API Key 单独加密保存；`servers.json` 只包含面板地址、节点 ID、实例 UUID 等非敏感信息。
- `MGL` 仅保留语言、主题、内存、Java 路径、下载源、实例列表、服务器绑定、缓存等非敏感信息。

> 加密数据与当前操作系统用户绑定。更换 Windows 用户或迁移到其他电脑后，需要重新登录账户并重新填写 API Key。

## OrcaRouter AI 搜索

在「设置 → AI 搜索」中把提供商切换为 **OrcaRouter**，即可使用以下默认配置：

```toml
model_provider = "orcarouter"
model          = "orcarouter/auto"

[model_providers.orcarouter]
name     = "OrcaRouter"
base_url = "https://api.orcarouter.ai/v1"
wire_api = "responses"
env_key  = "ORCA_KEY"
```

- 点击「一键授权」会打开系统浏览器，使用 PKCE（S256）完成授权；启动器在 `127.0.0.1` 随机端口接收回调，严格校验 `state`，再用 `code` 与 `code_verifier` 换取 API Key，并保存到加密安全存储。
- 授权地址：`https://www.orcarouter.ai/auth`；换取 Key：`POST https://www.orcarouter.ai/api/v1/auth/keys`。推理接口仍使用 `https://api.orcarouter.ai/v1`。
- 也可以手动填写 API Key，或者设置环境变量 `ORCA_KEY`；手动填写的 Key 优先。
- 请求使用 Responses API（`/responses`），默认模型为 `orcarouter/auto`。

## 服务器管理（MCSM）

1. 在 MCSManager 面板中为账号生成 API Key，并确认已启用 API（面板设置中的 API Key 开关）。
2. 打开「服务器管理 → 添加服务器」，填写面板根地址（例如 `http://192.168.1.10:23333`）与 API Key。
3. 点击「测试并获取实例」，选择要绑定的节点实例；再选择本地版本、远端根目录与需要同步的目录。
4. 点击「同步」后会先创建回滚快照，再打开文件选择器。默认全选服务端可用文件；仅客户端模组会标记并自动排除，也可以只勾选本次要更新的文件。手动选择不会删除未选中的远端文件；选择「同步全部」时才按配置决定是否删除远端多余文件（默认关闭）。
5. 同步按文件大小判断差异，只上传缺失或大小变化的文件；可在「回滚」中查看最近 5 份快照并恢复。回滚前必须停止服务器，启动器也会自动保存当前状态作为安全点。
6. 可在同一页远程启动、停止或重启 MCSM 实例；修改模组后通常需要重启服务器才会生效。

> 管理员 API Key 通常没有单独绑定实例，启动器会按官方 API 依次读取当前用户的实例、管理员全局实例与实例详情，避免出现「MCSM 实例不存在，或当前 API Key 没有访问权限」的误报。

> API Key 只会用于访问面板 API，并加密保存在系统安全存储中；不会写入 `MGL/servers.json`，也不会下发给界面。回滚快照保存在服务器实例目录下的 `.mgl-backups/`，不会上传到第三方服务。

## 局域网联机

- 在 Minecraft 中「对局域网开放」后，世界会通过 `224.0.2.60:4445` 组播广播，启动器会自动发现并显示版本、MOTD 与在线玩家。
- 「扫描局域网」会并发探测本机各私有 IPv4 `/24` 网段的 `25565` 端口（最多 4 个网段），并且只把真正返回 Minecraft Server List Ping 状态响应的地址判定为服务器；仅端口开放不会被误报。
- 「启动器玩家」使用本启动器的轻量组播协议 `224.0.2.61:4446`。启动器启动后会自动监听并广播玩家名、版本与实例名，无需打开联机页即可发现其他客户端；侧边栏「联机」会显示发现的客户端数量。可在联机页关闭广播，关闭后仍可发现其他人。
- 选择本地版本后点击「加入」即可直连：
  - Minecraft 1.20+ 使用 `--quickPlayMultiplayer host:port`。
  - 1.20 以下使用 `--server host --port port`。
- 若发现不到房间或玩家，请允许启动器通过 Windows 防火墙，并确认双方处于同一局域网；VPN、多网卡与禁止组播的网络可能影响发现。

### 局域网版本同步

1. 在共享方打开「联机 → 局域网共享」，选择要共享的本地版本与目录（模组、光影、资源包、配置），开启开关。
2. 其他客户端会在「启动器玩家」列表中看到「同步版本」按钮，选择本地版本后会先读取对方清单，展示 Minecraft 版本、加载器版本、新增/变更/相同文件数量和预计下载大小。
3. 确认差异后才会开始同步；共享清单若在确认后发生变化，需要重新确认。MC 版本或加载器不一致时会阻止同步，避免把不同版本的模组写进同一实例。
4. 同步使用 SHA-256 校验，仅下载缺失或内容变化的文件，不会删除本地多余文件；加载器版本不同且已确认时，会一并更新本地加载器。
5. 目标版本需要先关闭游戏；首次同步或文件较多时请保持双方在局域网内。

> 共享服务使用随机端口和随机访问令牌，仅共享启动器选中的目录；API 令牌随局域网点对点广播，不会上传到公网。仍请只同步来源可信的局域网玩家文件。

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

## License

本项目以 **GNU Affero General Public License v3.0（AGPL-3.0）** 发布，见 [LICENSE](LICENSE)。

- 你可以自由使用、修改和分发，但修改版通过网络提供服务时需按 AGPL-3.0 提供对应源代码。
- 商业/闭源集成需要单独的商业授权，见 [docs/OPEN_CORE.md](docs/OPEN_CORE.md)。
- 名称、Logo、官方构建、签名证书和官方更新源不随 AGPL 授权，见 [TRADEMARK.md](TRADEMARK.md)。
- 贡献需遵守 [CLA.md](CLA.md) 与 DCO，见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 已知限制

- 界面已完成基础多语言适配；下载、安装与登录流程中的详细错误信息和后台任务标题仍由后端以中文返回。
- 账户令牌与 API Key 保存在 Windows 注册表的加密数据中；它们与当前 Windows 用户绑定，请勿在共享系统账户下保存私人账户。
- 外置服务器若要求两步验证或特殊登录流程，需要按其文档处理。
- YSS 一键授权依赖站点启用「启动器 OAuth2」；若站点未启用，启动器会给出提示，仍可使用用户名 / 密码外置登录。
- 关闭版本隔离后，所有版本共用 `.minecraft` 中的模组、存档与配置，删除版本时也不会删除共享游戏目录。
- 资源中心使用 CurseForge 内容时需要填写自己的 CurseForge API Key；未填写时仅显示 Modrinth 结果。
- 冲突检测基于已安装 jar 的 Fabric / Quilt / Forge / NeoForge 元数据和版本声明的 `incompatible` 依赖；无法读取元数据的旧 jar 会被忽略。自动前置依赖只处理必填依赖，不会自动安装可选依赖，也不会替代整合包作者指定的精确版本组合。
- 模组更新依赖 Modrinth 文件哈希和 CurseForge 指纹精确匹配；被修改过、重新打包或来源不在两个平台的 jar 无法自动识别。CurseForge 识别需要 API Key。更新只替换对应 jar，旧文件保留为 `.jar.old`，目前回滚需要手动把 `.old` 改回 `.jar`，不会自动处理其他模组之间的版本约束。
- AI 搜索需要自备 DeepSeek 或 Anthropic API Key，搜索关键词会发送到所选提供商的 API。未配置或调用失败时会回退到本地别名匹配。API Key 加密保存在系统安全存储中，不写入 `MGL`。
- MCIM、BMCLAPI、清华 Adoptium 等第三方镜像属于公益服务，可能限速或暂停；镜像失败时会自动回退官方源，文件哈希校验仍然生效。Forge/NeoForge 安装器启动后由安装器自身下载部分运行库，这部分不受启动器镜像测速控制；自定义 `proxy` 镜像可以覆盖 Adoptium API 和安装器之外的标准 Maven/HTTP 路径。
- MCSM 同步按文件大小判断变化，大小相同但内容不同的文件不会自动重传；暂不支持断点续传与哈希校验。手动选择文件时不会执行远端删除；「同步全部」时才会按配置删除多余文件。回滚会删除并覆盖绑定的远端目录，执行前请停止服务器并确认快照时间。
- 局域网发现仅支持 IPv4，依赖 UDP 组播；扫描只覆盖本机私有网段的 `/24`。启动器不负责托管局域网服务器，只发现并加入已有房间。
- 局域网版本同步允许确认后更新同 MC 版本、同加载器的加载器版本；MC 版本或加载器类型不一致时会阻止同步。同步不会删除本地多余文件，也不会改动未共享的目录。
- 导入使用自定义启动主类、OptiFine 等特殊组件的版本时，启动器会给出警告；这类运行库无法仅凭版本名称自动还原，可能需要改用自定义导入并手动补充。缺少版本 JSON 的版本会从文件夹名称推断 MC 与加载器版本。
- 导出的 `.mrpack` 会排除日志、存档、本机选项等用户数据；需要完整备份（含存档与设置）时请使用 Prism / MultiMC 备份 `.zip`。导出要求加载器版本已知，否则会拒绝生成不完整的整合包。
- 启动器玩家发现从启动器启动时自动运行；所有客户端都需要运行本启动器并处于同一局域网。关闭广播后不会主动出现在其他人的列表中，但仍能发现别人。广播内容只有玩家名、实例名、MC 版本与局域网 IP。
- 服务器包会按常见元数据（Fabric / Quilt 的 `environment`、Forge / NeoForge 的 `displayTest`、旧版 `clientSideOnly`）自动排除仅客户端模组；无法确认的模组会保守保留，必要时仍可在 `mods/` 中手动移除。Forge / NeoForge / Quilt 服务器包首次使用需运行 `install-server` 脚本。
- 服务器包不会自动接受 Minecraft EULA，请阅读后自行将 `eula.txt` 中的 `eula=false` 改为 `true`。
- 每次启动游戏都会把当前启动器语言写入 `options.txt`；如果之后在游戏内修改语言，下次通过启动器启动时会被重新同步。
- 内容保护是界面层的防误选/防复制措施，不是 DRM：无法阻止系统截图、录屏、屏幕阅读器、开发者工具或直接读取 WebView 数据。登录设备码需要手动输入，控制台日志不再提供复制按钮。
- 本项目不是 Mojang Studios 或 Microsoft 的官方产品。Minecraft 是 Mojang Studios 的商标。
