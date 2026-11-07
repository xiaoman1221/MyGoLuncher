# releases/

TeamCity 的三个商用构建会把对应平台的产物写入这里，并提交回 `master`：

```text
releases/
├── latest-windows.json
├── latest-macos.json
├── latest-linux.json
└── <版本>/
    ├── windows/
    │   ├── MyGoLuncher-<版本>-windows-<arch>.exe
    │   ├── SHA256SUMS.txt
    │   └── build-info.json
    ├── macos/
    │   ├── MyGoLuncher-<版本>-macos-<arch>.zip
    │   ├── SHA256SUMS.txt
    │   └── build-info.json
    └── linux/
        ├── MyGoLuncher-<版本>-linux-<arch>.tar.gz
        ├── SHA256SUMS.txt
        └── build-info.json
```

- `latest-<平台>.json` 指向该平台最近一次构建。
- `SHA256SUMS.txt` 使用 `sha256sum -c` 校验；Linux 可额外生成 `.asc` GPG 签名。
- `build-info.json` 记录版本、平台、架构、提交、构建号、大小、哈希、UPX 与签名状态。
- 这些文件由 CI 自动生成，不需要手动编辑；本地 `build/bin/` 与 `frontend/dist/` 仍被忽略。
- 仓库会随每次发布增长。如需控制体积，建议只保留最近几个版本，或改用外部制品仓库。
