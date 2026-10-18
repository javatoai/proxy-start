# macOS 交付说明

Proxy Start 0.1.10 的 macOS 交付目标是 Apple Silicon（arm64）和 Intel（amd64）。打包脚本从 Windows 交叉编译两个原生 Go 可执行文件，分别放入 `Proxy Start.app`，再生成 ZIP 和 SHA-256 清单。

```powershell
.\scripts\pack-macos.ps1 -OutputDir dist\release-0.1.10-macos -Version 0.1.10
```

如需覆盖同名的 arm64/amd64 包和校验文件，显式添加 `-Force`；脚本不会删除输出目录里的其他文件。本地快速构建可以添加 `-SkipTests`。脚本要求 Go 1.27.x，优先使用项目内 `.tools/go/bin/go.exe`，也可以通过 `-GoExe` 指定可执行文件。MyGO v0.5.5 的默认最低 macOS 版本来自其本地源码 `cmd/mygo/config.go`：`12.0`，因此包内 `Info.plist` 写入 `LSMinimumSystemVersion=12.0`。

脚本会读取交叉编译产物的 Mach-O 头，确认它是对应架构的 `MH_EXECUTE`；创建 ZIP 后还会读取外部属性，确认可执行文件为 `100755`、`Info.plist` 为 `100644`，并拒绝 ZIP 内的 Windows 反斜杠路径。

生成的文件位于 `dist/release-0.1.10-macos/`，包括：

- `ProxyStart-macos-arm64.zip`：`Proxy Start.app` 和包内 `checksums.sha256`；
- `ProxyStart-macos-amd64.zip`：Intel Mac 对应的同结构应用包；
- `checksums.sha256`：两个 ZIP 的发布校验清单。

解压后将 `Proxy Start.app` 拖入“应用程序”文件夹，再双击启动。0.1.10 ZIP 是可运行的未签名包，未经过 Apple 公证，也尚未在 Mac 实机运行或验收。首次打开时，macOS 可能需要在“系统设置 → 隐私与安全性”中允许打开；这不是代码签名完成的证明。代理控制台通过私有临时脚本向 Terminal 传递环境变量，Mac 实机运行、签名和公证仍待完成。

macOS 当前支持 Codex 原生代理启动、`.app` 身份解析、Keychain 凭据和代理控制台环境注入；不提供按程序强制代理。对不支持的协议、认证或强制代理请求会返回明确错误，不会静默直连。Apple Silicon / Intel 交叉编译成功、CI 单元测试通过或 Windows 上的 ZIP 检查，都不能代替 Mac 实机 GUI、Codex 网络、Terminal 控制台和 Keychain 验收。

正式分发前请在对应的 macOS 机器上重新验证，并使用 Apple Developer 证书完成签名与公证。典型流程如下：

```sh
xattr -dr com.apple.quarantine "Proxy Start.app"
codesign --deep --force --options runtime --sign "Developer ID Application: YOUR NAME (TEAMID)" "Proxy Start.app"
codesign --verify --deep --strict --verbose=2 "Proxy Start.app"
ditto -c -k --keepParent "Proxy Start.app" ProxyStart-macos-signed.zip
xcrun notarytool submit ProxyStart-macos-signed.zip --keychain-profile YOUR_PROFILE --wait
xcrun stapler staple "Proxy Start.app"
```

上面的证书名称、团队 ID 和 notarytool profile 必须替换为发行者自己的配置；本仓库不保存证书、私钥或公证凭据。强制代理组件目前只在 Windows 路线中实现，macOS 包不宣称包含该能力。Linux 暂不支持。
