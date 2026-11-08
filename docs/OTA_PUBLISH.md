# OTA 自动发布

GitHub Actions 在三个平台构建完成后，会登录 LicensingGo、上传安装包，并更新产品的 OTA 清单。清单中的下载地址指向授权系统/CDN，**不依赖私有 GitHub Release 链接**。

流程定义在 `.github/workflows/release.yml` 的 `publish-ota` job，脚本为 `scripts/publish-ota.mjs`。

## 触发方式

手动运行 **Official release** 时：

| 输入 | 默认 | 说明 |
| --- | --- | --- |
| `version` | `1.4.1` | 发布版本，必须与仓库版本字段一致 |
| `publish` | `false` | 是否创建 GitHub Release |
| `update_ota` | `true` | 是否上传产物并更新 LicensingGo OTA |
| `ota_tag` | `optional` | 更新策略：`optional` / `mandatory` / `force` |

`update_ota` 与 `publish` 相互独立：即使不创建 GitHub Release，也可以只更新 OTA。

### 更新 TAG

| TAG | 客户端行为 |
| --- | --- |
| `force` | 强制更新：启动后全局弹窗，不可跳过，必须更新或退出 |
| `mandatory` | 非强制的重要更新：启动后普通提示，用户可稍后更新 |
| `optional` | 普通可选更新：不主动弹窗，可在「关于 → 更新」中查看 |

工作流中的 `ota_tag` 会写入 `MGL_OTA_TAG`。如果同时设置 `MGL_OTA_MANDATORY` 或 `MGL_OTA_FORCE`，显式布尔值优先于 TAG。

## 必需 Secrets

| Secret | 说明 |
| --- | --- |
| `MGL_PRODUCT_SERVER_URL` | LicensingGo 服务地址，HTTPS |
| `MGL_LICENSE_ADMIN_USERNAME` | 控制台管理员用户名 |
| `MGL_LICENSE_ADMIN_PASSWORD` | 控制台管理员密码 |

也可以用 `MGL_LICENSE_ADMIN_TOKEN` 直接提供控制台会话令牌；令牌优先于用户名密码。

管理员账号只需要能访问产品、上传 OTA 和保存 OTA 清单，建议使用专用账号。

## 可选 Variables

| Variable | 说明 |
| --- | --- |
| `MGL_PRODUCT_ID` | 产品 ID；为空时按 `MGL_PRODUCT_SLUG` 查找 |
| `MGL_PRODUCT_SLUG` | 产品 slug，默认 `MyGo` |
| `MGL_PRODUCT_NAME` | 产品名称，默认 `MyGoLuncher` |
| `MGL_OTA_TAG` | `force` / `mandatory` / `optional`，默认 `optional` |
| `MGL_OTA_MIN_VERSION` | 最低允许版本；为空时沿用旧清单 |
| `MGL_OTA_NOTES` | 更新说明；为空时沿用旧清单或自动生成 |
| `MGL_OTA_MANDATORY` | `true` / `false`，覆盖 TAG 的必填提示 |
| `MGL_OTA_FORCE` | `true` / `false`，覆盖 TAG 的强制更新 |
| `MGL_OTA_ALLOW_PARTIAL` | `true` 时允许缺少某个平台产物仍继续发布 |

## 产物映射

| 构建产物 | OTA 包 |
| --- | --- |
| `MyGoLuncher-<version>-windows-amd64.exe` | `app.windows.x64` |
| `MyGoLuncher-<version>-macos-universal.zip` | `app.macos.universal` |
| `MyGoLuncher-<version>-linux-amd64.tar.gz` | `app.linux.x64` |
| `MyGoLuncher-<version>-web.zip`（如果存在） | `web.any.any` |

- 每次都通过 `POST /api/v1/uploads/ota` 上传文件，服务端返回 URL、SHA-256 与大小。
- 上传后脚本会再次校验服务端返回的哈希和大小。
- 脚本保留旧清单中的 `web` 和未重新上传的平台包，只替换本次上传的 OS/架构。
- 最后调用 `PUT /api/v1/products/:id/ota` 保存清单。

## 前提条件

- LicensingGo 已配置 `uploads.storage`（local 或 S3）。
- `uploads.allowed_extensions` 包含 `.exe`、`.zip`、`.tar.gz`。
- `uploads.public_base_url` / S3 CDN 地址是 HTTPS，客户端才能下载。
- 产品已存在；如果不知道产品 ID，请设置 `MGL_PRODUCT_SLUG` 或 `MGL_PRODUCT_ID`。
- 管理员账号可以访问 `/api/v1/products` 与 `/api/v1/uploads/ota`。

## 回滚

在 LicensingGo 控制台恢复上一版 OTA 清单，或删除清单：

```http
DELETE /api/v1/products/:id/ota
```

上传过的文件不会自动删除，可保留用于回滚。
