<h1 align="center"><img src="assets/proxy-start.svg" width="40" alt=""> Proxy Start</h1>

一个轻量的 Codex 桌面端代理启动器。当前源码仅提供 **Codex 启停管理** 和 **打开代理控制台**，支持 Windows 与 macOS，使用 Go / MyGO 原生界面，固定浅色。

打开后自动扫描本机 Codex，首页中央圆圈直接显示“扫描中、未找到、未启动、启动中、已启动、状态待确认”。圆圈用于刷新状态，下方提供启动 / 重启和停止按钮。未启动时停止按钮置灰；打开软件或刷新不会改变现有进程，只有点击停止或重启才会结束它。

## 使用

1. 启动 Proxy Start，等待 Codex 扫描完成。
2. 点击右上角齿轮，在设置弹窗中修改并保存代理端口，默认 **7897**。地址固定为 `127.0.0.1`，两项操作共用该端口。
3. 点击圆圈刷新状态；用下方按钮启动 / 重启或停止 Codex。重启前检查代理端口，检查失败保留现有进程。
4. 点击右上角齿轮，再点击“打开代理控制台”，按弹窗中的端口打开 PowerShell（Windows）或 Terminal（macOS）。即使未安装 Codex，也可使用控制台。

启动前只检查本机端口是否可连接，不以端口监听或进程启动作为实际网络连通的证明。修改端口只影响下一次启动或新控制台。只有读取代理环境变量的命令行程序才会使用控制台代理。

## Codex 代理诊断（`--check`）

- **显式代理**：从主进程启动参数识别到代理地址。
- **未发现显式代理参数**：不能据此断言未使用代理，还可能通过环境变量、系统代理或 TUN 联网。
- **代理状态未知**：进程参数不可读或多个实例证据不一致。

Windows 按 `OpenAI.Codex` 安装包身份定位主入口；macOS 校验 `com.openai.codex` 应用包标识，并检查常见安装目录及 Spotlight 索引。扫描和状态查询不会启动或结束目标进程。

## 配置与边界

配置位于系统用户配置目录下的 `Proxy Start/settings.json`。新版格式仅包含 `version: 2` 和 `proxyPort`。旧配置读取时只在内存中迁移兼容的本机 HTTP 代理端口；首次保存前创建 `settings.v1-backup-*.json` 原文备份。旧主题、其它应用、认证及域名绕过配置不再生效；备份失败时保留旧文件。

关闭窗口收进托盘；退出启动器不会结束 Codex 或已打开的控制台。不修改系统代理、Clash 配置、TUN 或全局环境，不需要 helper、服务或驱动。不再支持其它桌面软件、多代理、SOCKS5 专用端口或代理认证。

Windows x64 发布为单个 EXE；macOS 发布为支持 Apple Silicon 和 Intel 的通用 DMG，打开后将应用拖入 Applications。发布文件不附带文档、脚本或独立资源；第三方许可内嵌到程序，可通过 `--licenses` 查看。

DMG 在 macOS 构建机生成、挂载和验证，使用本地临时签名，未完成 Developer ID 签名或公证。实机 GUI、Terminal 和真实 Codex 流量仍需 Mac 验证。

## 构建与验证

使用 Go 1.27.2，运行 `scripts/build.ps1 -GUI`。详细说明见 [构建文档](docs/BUILD.md)、[架构说明](docs/ARCHITECTURE.md)、[验证记录](docs/VERIFICATION.md) 与 [macOS 说明](docs/MACOS.md)。
