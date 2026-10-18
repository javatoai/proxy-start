# 发布说明

本项目的发布目标是 Windows x64 与 macOS arm64 / amd64。发布流程参考 `javatoai/silverwing` 的原生构建流水线：标签触发前先检查版本，分别构建平台产物，收集合并后的包并校验清单，再创建 GitHub Release。工作流负责可重复构建和资产整理，不替代目标机器上的功能验收。

## 版本与标签

0.1.10 的发布标签应为 `v0.1.10`，并与根目录 `VERSION`、产品入口和 macOS `Info.plist` 中的版本保持一致。Go 程序直接嵌入 `VERSION`，无需分别维护界面与诊断版本。新版本发布前应检查：

- 根目录 `VERSION` 与发布标签；
- `scripts/pack.ps1` 与 `scripts/pack-macos.ps1` 的输出目录及默认版本；
- README、`PRODUCT.md`、`docs/MACOS.md` 和本文件中的发布目标；
- 发布包内的 `checksums.sha256` 与外层清单。

配置文件格式版本独立于客户端版本。发布构建不得覆盖用户的 `%APPDATA%\Proxy Start` 或 macOS `~/Library/Application Support/Proxy Start` 配置。

## GitHub Actions 约定

项目使用 `.github/workflows/release.yml`，结构参考 Silverwing 当前的 `Native builds and releases`：

1. `version` 作业在 Ubuntu runner 检查当前产品版本；只有 `refs/tags/v<版本>` 才进入发布分支。
2. `build` 作业使用 Windows runner 构建 Windows x64；使用 macOS runner 构建 Apple Silicon 与 Intel `.app`。每个平台先运行测试，再上传对应压缩包和清单。
3. `release` 作业仅在目标仓库的版本标签上运行，下载本次构建产物，检查文件集合、架构、ZIP 权限和 SHA-256 后创建 GitHub Release。
4. 失败或普通分支构建只保留 CI artifact，不发布正式 Release。发布 job 才拥有 `contents: write` 权限。

标签发布前检查工作流条件、仓库名称、默认分支和 secrets；不要把本地 GitHub CLI 登录状态或个人凭据写入仓库。Release 资产应包含平台包、对应校验文件以及与源码版本一致的文档。

## Windows 构建

在 Windows PowerShell 中执行：

```powershell
.\scripts\pack.ps1 `
  -OutputDir dist\release-0.1.10 `
  -Proxy http://127.0.0.1:7897
```

脚本会运行 Go 测试、构建 GUI 客户端和管理员辅助进程、校验 helper 摘要、获取固定版本的 ProxiFyre 安装包，并生成：

- `ProxyStart-windows-amd64.zip`：可运行客户端、安装/卸载脚本、组件安装包和文档；
- `ProxyStart-source.zip`：不含 `.tools`、缓存和既有发布目录的源码包；
- `checksums.sha256`：两个 ZIP 的 SHA-256。

本地发布目录已存在时，必须显式使用 `-Force`。脚本不会运行 ProxiFyre 安装器，不会安装 NDISRD/Windows Packet Filter 驱动，也不会修改系统代理。

## macOS 构建

Windows 上可以交叉编译两个架构：

```powershell
.\scripts\pack-macos.ps1 `
  -OutputDir dist\release-0.1.10-macos `
  -Version 0.1.10
```

产物为 `ProxyStart-macos-arm64.zip` 和 `ProxyStart-macos-amd64.zip`。脚本检查 Mach-O 架构、`MH_EXECUTE` 文件类型、`.app` 目录布局、ZIP 内 Unix 权限和 SHA-256。它不完成 Developer ID 签名或 Apple 公证。

macOS 当前只实现 Codex 原生代理路径和通过 Terminal 注入代理环境变量的控制台；按程序强制代理尚未实现。Apple Silicon / Intel 的源码与交叉编译包存在，但没有 Mac 实机 GUI、Codex 网络、控制台、Keychain、签名或公证验收。CI 在 macOS runner 上通过单元测试，也不能作为这些实机结论的替代。

正式分发前应在对应 Mac 上重新打包并完成自己的签名、公证和首次启动检查：

```sh
codesign --deep --force --options runtime --sign "Developer ID Application: YOUR NAME (TEAMID)" "Proxy Start.app"
codesign --verify --deep --strict --verbose=2 "Proxy Start.app"
ditto -c -k --keepParent "Proxy Start.app" ProxyStart-macos-signed.zip
xcrun notarytool submit ProxyStart-macos-signed.zip --keychain-profile YOUR_PROFILE --wait
xcrun stapler staple "Proxy Start.app"
```

证书、私钥和 `notarytool` profile 不进入仓库或 CI 日志。

## 发布前验证

发布构建通过后，仍需单独记录以下结果：

- `go test ./...` 与 `go vet ./...`；
- Windows 包的 `--check`、配置哈希不变和 `--open-proxy-console` 环境注入；
- 使用新 PowerShell 的 `curl.exe` 做真实代理请求检查，并明确区分“环境已注入”和“目标软件流量已验证”；
- ProxiFyre 安装、服务/驱动状态与至少一个未读取代理环境变量的目标程序的真实流量；
- Mac 实机的 `.app` 启动、Codex 原生代理、Keychain、Terminal 控制台、签名、公证和退出行为。

上述项目的结果写入 [验证记录](VERIFICATION.md)。没有实机证据时，Release 说明应保留“未验收”字样，不能把交叉编译或 CI 通过写成 macOS 已完全支持。
