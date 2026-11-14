# 构建与运行

Windows 构建使用 Go 1.27.2；优先读取项目内 `.tools/go/bin/go.exe`，也可通过 `-GoExe` 指定。依赖以 `go.mod` 和 `go.sum` 为准。

```powershell
.\scripts\build.ps1 -GUI
.\scripts\run.ps1
```

构建先执行 `go test ./...`，再生成 `dist/proxy-start.exe`，注入品牌图标并检查 PE / Go 构建元数据。没有 helper 或第三方代理引擎。已独立完成测试后可加 `-SkipTests`；本地调试可用 `-KeepSymbols`。

```powershell
.\scripts\pack.ps1 -OutputDir dist/release-local
.\scripts\pack-macos.ps1 -OutputDir dist/release-local-macos -SkipTests
```

Windows 发布目录只有 `ProxyStart-windows-amd64.exe`；macOS 发布目录只有 `ProxyStart-macos-universal.dmg`。DMG 需要在 macOS 执行脚本，内部只有通用应用及 Applications 入口，校验包含双架构、临时签名和只读挂载。也可在 macOS 使用 `bash scripts/pack-macos.sh dist/release-local-macos`。发布页不另附校验、文档、脚本或源码 ZIP；第三方许可通过程序的 `--licenses` 查看。

macOS 尚未完成 Developer ID 签名、公证和完整实机验收；CI 构建不替代这些验收。打包脚本不会下载代理引擎或安装服务、驱动。

## 诊断与隔离预览

```powershell
.\dist\proxy-start.exe --check --config <隔离配置路径>
.\dist\proxy-start.exe --diagnose-proxy --config <隔离配置路径>
.\dist\proxy-start.exe --open-proxy-console
.\dist\proxy-start.exe --ui-smoke --ui-smoke-size 460x420 --ui-smoke-capture <截图绝对路径.png> --config <隔离配置路径>
```

`--check` 只读扫描并输出路径、运行状态、端口和脱敏代理证据。`--diagnose-proxy` 仅检查本机端口监听。控制台入口读取共用端口。预览尺寸范围 380×360 至 4096×2160，默认五秒退出，不读写正常窗口的尺寸状态，不启动或结束 Codex。旧多页面预览和任意软件解析参数已移除。

## 安装与卸载

Windows 发布文件是单个 EXE，下载后直接运行，不附带安装脚本。macOS 打开 DMG 后将应用拖入 Applications。

仓库中的 `scripts/install.cmd` / `scripts/uninstall.cmd` 仅供源码开发和本机安装使用，需要完整仓库和本地构建产物，不属于 Release 文件。它们仍校验目录所有权并拒绝在客户端运行时替换，不改变保存配置或用户目标进程；卸载支持 `-WhatIf`。

## 测试

```powershell
.\.tools\go\bin\go.exe test ./...
.\.tools\go\bin\go.exe vet ./...
$env:PROXY_START_WINDOWS_INTEGRATION='1'
.\.tools\go\bin\go.exe test ./internal/platform/windows -count=1 -v
python -m unittest discover -s scripts -p test_release_verification.py -v
```

Windows 控制台测试只使用独立的有界 PowerShell、临时文件和本机测试代理，包含不可达端口对照；进程生命周期测试只结束测试自己创建的子进程。安装扫描与现有 Codex 参数识别均为只读。
