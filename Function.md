# 功能说明

MyGoLuncher 的功能清单、免费/付费边界与已知限制。快速开始见 [README.md](README.md)。

## 版本管理

- 扫描官方启动器、HMCL、PCL2、Prism Launcher / MultiMC、Modrinth App、CurseForge、ATLauncher、GDLauncher 与 FTB App，也可手动选择 `.minecraft`、`versions` 或实例目录，支持多选批量导入。
- 「自定义导入」可填写版本名、Minecraft 版本、加载器与来源目录；缺少版本 JSON 时从文件夹名推断。
- 导入时复制存档、模组、资源包、光影、`kubejs`、`scripts`、`datapacks` 与配置，并准备运行环境。
- 版本隔离：每个版本使用 `versions/<版本名>/` 下的独立目录；关闭后所有版本共用 `.minecraft`。
- 支持修改名称、内存与 Java，修复缺失或损坏的本体/运行库/资源/加载器文件，启停模组、资源包与光影。
- 模组更新：按 Modrinth SHA-1 或 CurseForge 指纹识别，替换前下载校验并检测冲突；旧文件保留为 `.jar.old`。

## 下载与镜像

- 从 Mojang 下载正式版、快照与远古版本，包含客户端、运行库、本地库与资源文件。
- 下载 Minecraft 与修复版本文件免费，不需要激活。
- 内置官方源、BMCLAPI、清华 Adoptium 与自定义镜像；安装前测速并自动回退，所有文件校验哈希。
- 资源下载支持 MCIM 等 Modrinth / CurseForge 镜像，可在设置或安装时切换。

## 加载器与 Java

- 支持 Fabric、Quilt、Forge、NeoForge，自动列出与当前 Minecraft 版本兼容的加载器版本。
- Forge/NeoForge 优先执行官方安装器；旧版安装器失败时自动解包 `install_profile` 与 Maven 运行库进行兼容安装。
- 自动扫描本机 Java 并按版本元数据匹配；缺失时从 Adoptium 下载 Temurin JRE。
- 加载器元数据、Maven 与安装器按当前网络选择镜像。

## 资源中心

- 搜索并安装 Modrinth / CurseForge 的模组、光影、资源包、数据包与整合包。
- 中文搜索内置模组别名与类型词，兼容繁体、全角字符与中英混合查询，精确匹配优先；未命中时可使用 AI 搜索或英文名。
- 安装前扫描 Fabric / Quilt / Forge / NeoForge 元数据，检测重复文件、`breaks` 声明与不兼容依赖；确认风险后可继续。
- 自动安装必填前置依赖，并报告依赖数量；不自动处理可选依赖。
- AI 搜索支持 DeepSeek（默认）、Anthropic Claude 与 OrcaRouter；OrcaRouter 支持 PKCE 一键授权。

## 整合包与服务器包

- 导入 `.mrpack`、CurseForge `.zip`、Prism / MultiMC 或通用 `.zip`。
- 导出 `.mrpack`，可被 Modrinth App、Prism 等导入；导出时排除日志、存档与本机选项。
- 导出 Prism / MultiMC 布局的完整备份 `.zip`。
- 导出服务器包 `.zip`：包含加载器运行时或安装器、`mods/`、`plugins/`、`config/`、脚本与 `server.properties`，并剔除仅客户端模组。
- 服务器包不会自动接受 Minecraft EULA。

## 服务器管理（付费）

- 绑定多个 MCSManager 面板与实例，增量同步 `mods/`、`config/`、脚本、资源包与光影等目录。
- 同步前在服务器创建 `.mgl-backups/` 快照，保留最近 5 份，可一键恢复。
- 支持手动勾选文件、可选删除远端多余文件，以及远程启动、停止、重启。
- 同步按文件大小判断差异，不校验哈希，不支持断点续传；回滚会覆盖远端目录。

## 局域网联机（付费）

- 自动发现同网段使用本启动器的玩家，以及「对局域网开放」的世界和局域网服务器，显示版本、MOTD 与在线玩家。
- 「扫描局域网」并发探测本机私有 IPv4 `/24` 网段的 `25565` 端口，只把返回 Minecraft Server List Ping 响应的地址判定为服务器。
- 选择本地版本即可直连：1.20+ 使用 `--quickPlayMultiplayer`，旧版本使用 `--server/--port`；加入前校验版本。
- 版本共享：比较 Minecraft/加载器版本与文件差异，确认后按 SHA-256 增量同步；可共享整个版本并创建新的隔离实例。
- 联机工具：群组/私聊聊天、WebRTC 语音（最多 8 人）、最大 512 MB 的文件传输，文件保存在 `lan-inbox`。

## 账户

- Microsoft 正版登录（设备码 OAuth），启动前自动刷新令牌。
- 外置登录：内置 LittleSkin 与 YggdrasilSkinServer，兼容其他 Authlib-Injector 服务器。
- YSS 一键授权：OAuth2 设备码流程，需要在站点启用「启动器 OAuth2」。
- 离线账户：可添加、重命名与切换，每个名称生成标准离线 UUID。

## 桌面与系统集成

- 后台任务：实时日志、进度、取消；已下载的部分会保留。
- 游戏控制台：实时日志、错误高亮、结束进程。
- 启动动画、检测到游戏窗口后隐藏启动器、游戏退出后恢复。
- 启动前把游戏语言同步为启动器语言（`zh_cn`、`zh_tw`、`en_us`、`ja_jp`、`ko_kr`）。
- 退出时终止由启动器启动的游戏；Windows 使用 `KILL_ON_JOB_CLOSE` Job Object 兜底。
- 关闭行为可设为询问、最小化到托盘或直接退出；没有游戏运行时不会弹出确认，直接关闭。
- 深浅主题（黑夜荧光绿 / 白天粉色），五种界面语言。
- 页面文字可选择、复制；输入框支持全选、剪切、粘贴与右键菜单。

## 授权与更新

### 免费与付费

| 类型 | 内容 |
| --- | --- |
| 免费 | 启动游戏、版本管理与导入、下载/修复 Minecraft、Java 管理、基础模组/资源安装、本地备份 |
| `server` | MCSM 服务器管理、备份与同步 |
| `lan` | 局域网发现、版本共享、聊天/语音/文件传输 |
| `ai` | AI 搜索与 OrcaRouter 授权 |
| `cloud` / `cdn` / `hosting` / `priority` | 预留或运营权益 |

`*`、`all`、`pro`、`commercial` 可作为全量权益。客户端检查只用于界面提示，真正的权益校验在服务端。

### 授权数据

- 激活入口位于「关于 → 授权与订阅」，需要激活码、绑定邮箱与购买登记的客户名称。
- Windows 写入 `HKCU\Software\MyGoLuncher\Secrets` 并由 DPAPI 加密；其他平台使用 AES-256-GCM 文件与 0600 密钥文件。
- 启动时在线校验；网络失败且上次成功校验在 7 天内时进入离线宽限期。
- 每个激活码/机器可试用 3 次，每次 7 天，总计最多 21 天。

### 更新

- 官方构建通过 LicensingGo 的 `GET /api/v1/update` 查询 OTA 清单，校验 URL、SHA256 与大小。
- 启动时自动检查更新：`force` 全局阻断并要求更新，`mandatory` 显示一次普通提示，`optional` 仅在关于页显示。
- 前端热更新解压到 `MGL/web`，旧版本保留为 `MGL/web.old` 可回滚。
- 完整更新使用新版本 exe 作为独立更新助手进程，等待旧进程退出后替换、校验 SHA256 并重启，旧文件保留为 `.bak`。
- 社区构建没有产品 Key 与授权服务地址，付费入口显示 PRO 提示。

## 已知限制

- 界面多语言已覆盖；下载、安装与登录的详细错误和后台任务标题仍由后端返回中文。
- 关闭版本隔离后所有版本共用模组、存档与配置，删除版本不会删除共享目录。
- CurseForge 内容需要自备 API Key；未填写时仅显示 Modrinth 结果。
- 冲突检测依赖模组元数据；无法读取元数据的旧 jar 会被忽略。
- 模组更新只能识别 Modrinth / CurseForge 上未被修改的文件，回滚需手动把 `.jar.old` 改回 `.jar`。
- AI 搜索需要自备 API Key，未配置或失败时回退到本地别名匹配。
- 第三方镜像可能限速或暂停；Forge/NeoForge 安装器启动后的部分下载不受启动器控制。
- 局域网发现仅支持 IPv4 与 UDP 组播；扫描只覆盖本机私有 `/24` 网段。
- 版本同步不会删除本地多余文件；完整版本安装要求开启版本隔离。
- 内置 Microsoft OAuth Client ID 可能受白名单限制；如报 `Invalid app registration`，请使用外置或离线登录。
- 自签名发布证书不会获得 Windows SmartScreen / macOS Gatekeeper 的公共信任。
- 本项目不是 Mojang Studios 或 Microsoft 的官方产品。
