# macOS

支持 Codex 桌面端启动与独立 Terminal 代理控制台，界面固定浅色。

Codex 从 `/Applications`、用户 Applications 和 Spotlight 索引发现，必须匹配 `com.openai.codex` 及真实应用包可执行入口。每次启动重新解析位置。打开启动器和点击圆圈刷新仅观察；只有显式点击停止 / 重启时才按身份校验后的进程执行终止，重启先检查代理配置。

本机 HTTP 端口默认 7897。Codex 使用 Chromium 代理参数及子进程代理变量；Terminal 使用短生命周期、私有权限的 `.command` 脚本注入新 zsh 环境，脚本读入后自行删除。没有 Keychain、强制代理或驱动依赖。

现有进程通过真实路径、PID 和创建时间识别。启动参数来自 KERN_PROCARGS2，只解析 argv，不读取环境变量。无法读取时显示未知；未发现显式参数不代表实际未使用代理。

在 macOS 使用 `bash scripts/pack-macos.sh` 或 `scripts/pack-macos.ps1` 生成一个通用 DMG，同时支持 Apple Silicon / Intel。DMG 内只有 `Proxy Start.app` 与 Applications 入口；脚本检查双架构、临时签名、磁盘映像完整性及挂载后内容。Windows 可以交叉编译 Go 主程序，但不能生成经过挂载验证的 DMG，Release 由 GitHub macOS 构建机完成。

本地临时签名不等于 Developer ID 签名或公证；真实 Mac GUI、Terminal 和 Codex 代理流量仍需实机验证。
