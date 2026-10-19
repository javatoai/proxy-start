# 代理控制台真实请求验证

这份记录验证的是“打开代理控制台后，新 shell 是否真的把请求交给代理”，不是对用户已经打开的交互式 PowerShell 注入命令。验证在 Windows 上运行，测试不会写入 Proxy Start 配置，不修改系统代理，不安装驱动，也不会结束现有目标程序或用户终端。

## 测试方法

`internal/platform/console/launcher_windows_test.go` 的网络测试通过测试专用 seam 调用生产 `Launcher.Open`。生产默认仍然打开独立、可持续使用的 PowerShell；测试 seam 只把 PowerShell 替换成一个短生命周期的 `-Command`，运行一次：

```powershell
curl.exe -sS --connect-timeout 5 --max-time 20 -o NUL -w '%{http_code}' https://example.com/
```

测试在本机回环地址启动临时 HTTP CONNECT 记录转发器（`127.0.0.1:<临时端口>`）。它只接受来自测试 shell 的 CONNECT，将原始 CONNECT 请求转发到已存在的本机 HTTP 代理 `127.0.0.1:7897`，再把 CONNECT 建立后的 TLS 字节双向转发。这样可以同时观察：

1. PowerShell 子进程实际收到的 `HTTPS_PROXY`；
2. `curl.exe` 是否发出了 `CONNECT example.com:443`；
3. 经过本机 7897 上游后，TLS/HTTP 请求是否得到非 `000` 的 HTTP 状态码。

测试随后关闭一个临时监听端口，把同样的 curl 请求指向这个不可达代理。该请求必须失败，作为“没有因为环境变量失效而直连”的负向对照。

## 可重复命令

在仓库根目录执行：

```powershell
$goRoot = Join-Path $PWD '.tools\go'
$env:GOROOT = $goRoot
$env:GO111MODULE='on'
$env:GOTOOLCHAIN='local'
$env:PROXY_START_CONSOLE_NETWORK='1'
& "$env:GOROOT\bin\go.exe" test ./internal/platform/console -run TestWindowsConsoleProxyNetworkIntegration -count=1 -v
```

同时可运行不发起网络请求的子进程环境检查：

```powershell
$goRoot = Join-Path $PWD '.tools\go'
$env:GOROOT = $goRoot
$env:PROXY_START_CONSOLE_REAL='1'
& "$env:GOROOT\bin\go.exe" test ./internal/platform/console -run TestWindowsChildEnvironmentIntegration -count=1 -v
```

域名绕过的路由检查使用独立的测试入口，不读取用户配置。它创建一个本地直连服务和一个记录 HTTP 代理，为 `root.test` 注入 `NO_PROXY` / `no_proxy`，并让 `curl.exe` 分别请求根域、子域和未匹配域名；前两者应直连，未匹配域名应到记录代理。随后关闭代理监听，未匹配请求必须失败而不能回退直连。SOCKS5 分支复用同一规则，检查匹配域名仍可直连：

```powershell
$env:PROXY_START_CONSOLE_BYPASS_REAL = '1'
& "$env:GOROOT\bin\go.exe" test ./internal/platform/console -run TestWindowsConsoleProxyBypassRoutingIntegration -count=1 -v
```

## 2026-10-19 0.1.11 域名绕过验证

以下结果对应待发布的 0.1.11 域名绕过实现；发布流程仍需通过 CI 完成。Windows 路由测试按上面的命令执行，并额外以 `-count=3` 重复，均通过。测试不读取用户配置、不连接外网或本机 `7897`，不会使用 `--proxy` / `--noproxy`；`curl.exe -q --resolve` 只把测试域名映射到测试服务。

- 测试目标服务器累计记录 `direct=5`（包含一次由 SOCKS5 代理转发的请求，该计数不全是客户端直连），HTTP 未绕过的请求到记录代理 `http://through.test:<临时端口>/proxy`；关闭代理后的未绕过请求退出码为 `7`、HTTP 状态 `000`。
- HTTP 测试确认根域和子域直连，未匹配域名经过记录代理；SOCKS5 路径记录未匹配请求的 `CONNECT through.test:<临时端口>`，退出码 `0`、HTTP 状态 `200`。
- `TestWindowsChildEnvironmentIntegration` 与 `TestWindowsChildEnvironmentBypassIntegration` 两个真实子进程环境用例通过，验证非空 `NO_PROXY` / `no_proxy` 覆盖和空规则清理继承值。
- AppX/Codex 参数清理与顺序静态测试通过；Darwin 环境变量、临时脚本自删和参数测试已交叉编译验证。真实 Codex 网络请求和 Mac 实机 GUI / Terminal 验收仍未完成。

## 结果（2026-10-18，Windows）

网络测试通过：`TestWindowsConsoleProxyNetworkIntegration`。成功路径中，子 PowerShell 的 `HTTPS_PROXY` 是临时回环转发器地址，curl 退出码为 0、HTTP 状态为 `200`，记录转发器观察到严格的 `CONNECT example.com:443`；随后 CONNECT 被转发到实际本机上游 `127.0.0.1:7897` 并完成请求。测试日志只记录临时回环端口、`example.com:443`、退出码和 HTTP 状态。

负向对照也通过：关闭代理监听端口后，curl 返回连接失败退出码 7、HTTP 状态 `000`，且仍报告使用了该不可达 `HTTPS_PROXY`。这证明测试请求没有绕过代理直接访问 `example.com`。

同一测试还用 `BuildProxyEnvironment` 生成 `socks5h://127.0.0.1:7897` 环境变量，使用同一条不带 `--proxy` 的 curl 命令完成 HTTP 状态 `200` 的请求。该分支只证明 curl 的环境变量路径和本机混合代理的 SOCKS5 入口；它不等于任意目标程序都支持 SOCKS5 环境变量。

测试只输出回环端口、CONNECT 目标和状态码，不输出代理认证信息或公网地址，也不会操作用户已有的终端。

## 边界

- 这条证据覆盖 Windows PowerShell、HTTP CONNECT 代理和会读取环境变量的 `curl.exe`；它不等于 Codex 或其他目标程序已经完成业务流量验收。
- 绕过规则只在下一次启动、重启或打开新控制台时读取；空规则清除继承的 bypass，非空规则覆盖 `NO_PROXY` / `no_proxy`。它不改写 VPN、TUN 或系统路由。
- 该次网络验证使用当前本机 Clash/Mihomo HTTP 入口 `127.0.0.1:7897`。如果该入口停止，测试会按预期失败，而不会自动改写配置。
- 旧的 2026-10-18 记录没有把真实 SOCKS5 上游改造成网络测试入口；本页新增的 2026-10-19 工作区测试只验证本地 SOCKS5 路由和绕过行为，仍不等于真实目标程序或外网 Codex 流量验收。
- macOS 的 Terminal 启动器和临时脚本有 Darwin 交叉编译及清理测试；本机没有 macOS，尚未声称 macOS 实机网络请求、签名或公证已验证。
