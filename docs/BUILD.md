# 构建与运行

构建目标为 Windows amd64，以及 macOS arm64 / amd64。本地默认使用项目目录内的 Go 1.27.2，也可用 `-GoExe` 指定已安装的 Go；CI 使用 setup-go，不修改本机全局 `PATH`。Windows 构建流程见下文，macOS 使用 `scripts/pack-macos.ps1`，安装及实机验证说明见 [MACOS.md](MACOS.md)。Linux 暂不支持。

## 准备项目工具链

在项目根目录执行以下 PowerShell 命令。下载地址和摘要来自 Go 官方下载 JSON（`https://go.dev/dl/?mode=json&include=all`）：

```powershell
New-Item -ItemType Directory -Force .tools | Out-Null
Invoke-WebRequest `
  -Uri 'https://go.dev/dl/go1.27.2.windows-amd64.zip' `
  -OutFile '.tools/go1.27.2.windows-amd64.zip' `
  -UseBasicParsing
$actual = (Get-FileHash -Algorithm SHA256 '.tools/go1.27.2.windows-amd64.zip').Hash.ToLowerInvariant()
if ($actual -ne '1314008898bd40df77af4b014f777f08873dbdfbcd3d92308728ee03304fe04f') {
  throw "Go SDK SHA-256 校验失败：$actual"
}
Expand-Archive '.tools/go1.27.2.windows-amd64.zip' -DestinationPath '.tools' -Force
& '.tools/go/bin/go.exe' version
```

如果直连下载超时，可在 `Invoke-WebRequest` 中增加 `-Proxy 'http://127.0.0.1:7897'` 后重试。不要在未通过 SHA-256 校验时解压或运行 SDK。`.tools/` 已加入忽略规则，SDK 不会被提交到仓库。

## 构建

```powershell
.\scripts\build.ps1
```

脚本优先使用 `.tools/go/bin/go.exe`，也支持 `-GoExe go` 或绝对路径，覆盖可能冲突的 `GOROOT`、`GO111MODULE`、`GOTOOLCHAIN`、`GOOS`、`GOARCH` 和 `CGO_ENABLED`，并从项目根目录运行命令；先运行普通的 `go test ./...`（不启用 `-race`，也不把它当作完整发布验收），然后生成：

- `dist/proxy-start.exe`：客户端进程
- `dist/proxy-start-helper.exe`：需要提升权限时使用的辅助进程

只在排查构建入口、暂时跳过测试时使用：

```powershell
.\scripts\build.ps1 -SkipTests
```

默认使用 `-s -w` 去除调试符号；需要本地调试时使用 `-KeepSymbols`。输出目录可用 `-OutputDir <path>` 指定。

发布构建使用 Windows GUI 子系统（不会额外弹出控制台窗口）：

```powershell
.\scripts\build.ps1 -GUI
```

`-GUI` 只改变链接子系统，不改变客户端的诊断参数或辅助进程协议。

构建顺序固定为先生成 `proxy-start-helper.exe`，再计算其 SHA-256 并通过 `-X main.helperSHA256=...` 写入主程序。客户端启动 helper 前会校验这个摘要；即使使用控制台调试构建，也会保留同一完整性绑定。

## 运行

```powershell
.\scripts\run.ps1
```

目标文件不存在时，`run.ps1` 会先执行完整构建。明确要求重新构建时使用 `-Build`；运行辅助进程使用 `-Helper`。传给客户端的参数可直接写在脚本后面，例如：

```powershell
.\scripts\run.ps1 -Build -- --ui-smoke
```

`--ui-smoke` 是否启用由客户端入口决定，脚本只负责转发参数。

## 通过诊断入口打开默认代理控制台

无需打开 GUI 时，可让客户端读取当前保存的默认代理并调用同一个 `Desktop.OpenProxyConsole` 用例：

```powershell
.\dist\proxy-start.exe --open-proxy-console
```

该命令只请求打开独立的 PowerShell（Windows）或 Terminal（macOS）；它不会修改父进程、系统代理或全局路由，也不等于目标软件流量已经连通。需要指定隔离配置时可同时传入 `--config <配置文件绝对路径>`。

真实 HTTP / SOCKS5 请求及不可达代理对照的可重复测试见 [控制台验证记录](CONSOLE-VERIFICATION.md)。发布版本由根目录 `VERSION` 提供，诊断输出和 UI 共享该值；标签和 macOS 包版本也必须与之匹配。

## 原生界面预览

预览模式独立于正在运行的普通客户端，不读取或覆盖保存的窗口尺寸，默认五秒后自行退出。可用 `--ui-smoke-duration 2m` 延长检查时间，允许范围为一秒至两分钟。使用独立的测试配置即可检查页面和截图，不会启动或结束配置中的目标软件：

```powershell
.\dist\ui-polish-dev\proxy-start.exe --ui-smoke `
  --config <测试配置的绝对路径> `
  --ui-smoke-page settings --ui-smoke-size 1632x971 `
  --ui-smoke-capture <已有目录中的截图绝对路径.png>
```

先使用 `scripts/build.ps1 -GUI -OutputDir dist/ui-polish-dev` 构建此路径。页面可选 `launch`、`proxy-settings`、`system-settings`；旧别名 `settings` 仍保留并指向代理设置页。`--ui-smoke-selected` 打开首个配置应用的详情。截图来自运行中的 MyGO 内容渲染，不包含 Windows 标题栏；它不能证明原生标题栏、托盘或真实鼠标交互已经验收。

## 获取 ProxiFyre 依赖

强制代理模式依赖官方 ProxiFyre 2.6.4 x64 安装程序。依赖清单和 SHA-256 固定在 `internal/infrastructure/proxifyre/dependencies.json`。只下载并校验安装包：

```powershell
.\scripts\fetch-engine.ps1
```

产物位于 `dist/dependencies/ProxiFyre-2.6.4-win-x64-setup.exe`。下载脚本不会运行安装程序，也不会安装服务或 NDISRD/Windows Packet Filter 驱动；用户必须在“系统设置”的“强制代理组件”区域或资源管理器中明确启动已校验的安装包并完成管理员向导。

## 生成 Windows 发布包

```powershell
.\scripts\pack.ps1
```

脚本会先通过测试、使用 `-H=windowsgui` 构建客户端和辅助进程，下载并校验 ProxiFyre，生成两个压缩包：

- `dist/ProxyStart-windows-amd64.zip`：GUI 二进制、ProxiFyre 安装程序、依赖清单、`scripts/` 每用户安装/卸载入口、README、`docs/` 文档目录（含架构、验证和平台路线说明）、产品/设计说明、第三方许可和包内 `checksums.sha256`。
- `dist/ProxyStart-source.zip`：项目源码和文档，不包含 `.tools/`、缓存、构建产物或用户本地发布目录。

两个压缩包的 SHA-256 位于 `dist/checksums.sha256`。若要覆盖已有发布目录，明确使用 `-Force`；若只做本地快速打包并跳过测试，可使用 `-SkipTests`。打包流程不会执行第三方安装程序或驱动安装器。

## 每用户安装与卸载

解压 Windows 发布包后，双击 `scripts/install.cmd` 即可安装到 `%LOCALAPPDATA%\Programs\Proxy Start`，不需要管理员权限。安装脚本会写入专用所有权标记；若同名目录不是本安装脚本创建的目录，安装和卸载都会拒绝递归替换或删除。也可以在 PowerShell 中选择创建桌面快捷方式：

```powershell
.\scripts\install.ps1 -DesktopShortcut
```

安装会创建开始菜单快捷方式，并将 `proxy-start.exe`、helper、依赖清单和已校验的 ProxiFyre 安装包复制到用户目录。安装脚本不会启动客户端，不会安装、启动或停止 ProxiFyre 服务/驱动，也不会改动 `%APPDATA%\Proxy Start` 配置和 Windows 凭据。

从开始菜单选择“Uninstall Proxy Start”，或运行安装目录中的 `scripts\uninstall.cmd`。卸载前会检查客户端和 helper 是否仍在运行；若仍在托盘中，请先退出，脚本不会强制结束进程。卸载只删除安装目录和本工具创建的快捷方式，保留配置、凭据、ProxiFyre 服务/驱动以及目标软件。可先用 `powershell -File scripts\uninstall.ps1 -WhatIf` 预览路径，不会执行真实删除。
