# 架构与扩展

## 依赖方向

```text
cmd/proxy-start（组装入口）
  ├─ ui → application → domain
  ├─ adapters/native → application 的小接口与 domain
  ├─ infrastructure → application 的小接口与 domain
  └─ platform/windows、platform/darwin、platform/console → application 的小接口与 domain
```

领域与应用层不导入 MyGO、Windows API、代理引擎或 JSON 存储实现。UI 只调用 `application.Desktop` 的用例门面；文件访问、进程操作、凭据和提权均位于实现层。所有依赖在入口显式注入，没有全局可变服务注册表。

## 职责划分

| 模块 | 单一职责 |
| --- | --- |
| domain | 程序身份、代理配置、不可变绕过策略、启动方案和运行状态等平台无关概念 |
| application/catalog | 解析与管理启动项，不执行程序 |
| application/profiles | 代理配置、凭据引用和连接检测 |
| application/strategy | 用不可变的原生适配器集合选择启动策略 |
| application/launcher | 每个程序的启动、结束、重启及状态变化 |
| application/runtime | 串行维护共享强制代理会话，提交完整配置 |
| adapters/native | 识别已知软件并生成启动方案，不进行系统调用 |
| platform/windows | 快捷方式/AppX 解析、环境继承、进程身份与进程树操作 |
| platform/darwin | .app 清单与图标解析、libproc 进程身份、独立会话启动及有界停止 |
| platform/console | 实现 `application.ConsoleLauncher` 的平台 shell 适配；只负责新 shell 的环境边界，不修改父进程或系统全局环境 |
| application/platform | 从组装入口提供不可变的平台能力快照，供界面展示 |
| infrastructure/proxifyre | 强制代理配置、受限管理员辅助进程和服务控制 |
| infrastructure/unsupportedproxy | 平台缺少强制代理时拒绝应用规则，不启动无代理的目标程序 |
| infrastructure/httpbridge | SOCKS5 CONNECT 到 HTTP CONNECT 的协议转换，不接触应用列表 |
| infrastructure/storage、credentials、netprobe | 非秘密配置持久化、系统凭据管理、代理协议与连接探测 |

接口定义在使用方，不为每个结构体机械建立接口。`Desktop` 只转发用例与汇总快照，业务逻辑由各服务承担。

## 启动流程

1. 重新解析原始路径或包标识，避免快捷方式目标或 MSIX 安装目录更新后继续使用旧路径。
2. 查找对应的现有进程；普通“启动”不自动结束已有程序。
3. 读取代理、凭据和 `BypassPolicy`，对所需协议执行连接检测。
4. 由策略选择器查找适配器，并同时检查协议、认证和绕过策略是否支持。
5. 原生方式生成 `LaunchSpec` 并交给平台执行；强制方式先提交显式路径规则，再启动目标程序。
6. 记录实际 PID、创建时间和路径。进程启动成功不自动升级为“实际连接已验证”。

## 代理控制台用例

代理设置表格中的“打开代理控制台”调用 `Desktop.OpenProxyConsole`。它先读取选中的代理配置和凭据；协议为自动检测时，按 HTTP 已验证、SOCKS5 已验证的顺序选择，未得到已验证结果就返回错误，不把协议猜测当成成功。随后调用注入的 `application.ConsoleLauncher`：

```go
type ConsoleLauncher interface {
    Open(context.Context, ConsoleSpec) error
}
```

`ConsoleSpec` 只在运行时携带配置生成的环境映射和要清除的变量。HTTP、HTTPS、ALL_PROXY 以及对应小写变量都会写入同一代理 URL；HTTP / SOCKS5 的用户名和密码由 URL 编码后再组成 URL。非空 `BypassPolicy` 会以逗号分隔写入 `NO_PROXY` 与 `no_proxy`，覆盖子 shell 继承的值；空策略才会清除这两个继承变量。父进程环境、系统代理、Clash 配置、VPN/TUN 和全局路由不变。规则只在下一次启动、重启或打开新控制台时读取，已有进程保持原环境。打开控制台本身只表示 shell 创建请求成功，不表示目标软件流量已经实际连通。

领域层把编辑器输入收敛为不可变的值对象：

```go
type BypassPolicy struct { /* validated, private domains */ }

func NewBypassPolicy([]string) (BypassPolicy, error)
func ParseBypassPolicy(string) (BypassPolicy, error) // newline or comma separated
func (BypassPolicy) Domains() []string               // defensive copy
func (BypassPolicy) Empty() bool
```

每个值是规范化后的 ASCII 主机名或域名（含 Punycode），大小写归一化并去重；`example.com` 表示根域及其子域。URL、端口、通配符和 IP/CIDR 在领域层拒绝。配置保存 `BypassDomains`，旧 JSON 缺少该字段时按空策略处理。

Windows 的实现以 `CREATE_NEW_CONSOLE` 启动独立 PowerShell，并只把环境交给该进程及其子进程。macOS 的实现通过 Terminal.app 执行权限为 0700 的私有 `.command` 文件；脚本先自删再导出变量并进入干净 zsh。启动器最多等待 10 秒确认脚本被读取，失败、取消或超时立即清理临时文件。该路径目前尚未在 Mac 实机运行，也未完成 Developer ID 签名或公证。

同一应用操作使用独立锁；共享引擎配置使用运行时管理器的锁。外部 IO 有取消和超时；目标程序本身不使用随操作上下文取消的 `exec.CommandContext`，也不加入客户端退出时自动杀死的 Job。

## 添加原生适配器

实现 `application.NativeAdapter`：

```go
type NativeAdapter interface {
    ID() string
    Match(domain.AppIdentity) bool
    Supports(domain.ProxyEndpoint, domain.BypassPolicy) bool
    Plan(domain.AppIdentity, domain.ProxyEndpoint, domain.BypassPolicy) (domain.LaunchSpec, error)
}
```

参考 `internal/adapters/native/adapters.go` 的 Codex 实现。生产名单集中在 `native.Production()`，首期仅注册 Codex。`Match` 使用真实软件身份；`Supports` 声明支持的协议、认证和绕过策略边界；`Plan` 是纯函数，复制参数与环境设置，不能更改传入对象或直接启动进程。适配器若不支持绕过，必须在 `Supports` 中显式拒绝非空策略，不能静默忽略规则。新增支持时修改该名单，不修改启动器内的条件分支。macOS bundle ID 与路径规则参考 [OpenAI 官方启动器](https://github.com/openai/codex/blob/main/codex-rs/cli/src/desktop_app/mac.rs)。

新增适配器至少测试身份误匹配、协议与认证边界、已有代理参数处理，以及输入不可变性。应用需要新的平台能力时，再扩展对应小接口，不给已有接口加入无关方法。

下面是一个可复制后再按实测安装路径修改的最小适配器示例。`ExampleTool` 是占位名称，不代表项目已经验证过某个真实软件；只有确认了真实安装根目录、可执行文件名和代理能力后，才应把它加入生产组装入口。

```go
package exampleadapter

import (
    "fmt"
    "strings"

    "proxy-start/internal/application"
    "proxy-start/internal/domain"
)

// ExampleHTTPToolAdapter is a deliberately narrow example: one verified
// executable path family and unauthenticated HTTP only.
type ExampleHTTPToolAdapter struct{}

func NewExampleHTTPTool() application.NativeAdapter {
    return &ExampleHTTPToolAdapter{}
}

func (ExampleHTTPToolAdapter) ID() string { return "example-http-tool" }

func (ExampleHTTPToolAdapter) Match(identity domain.AppIdentity) bool {
    path := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(identity.Executable), "/", `\`))
    for _, root := range []string{
        `c:\program files\examplevendor\exampletool\`,
        `c:\program files (x86)\examplevendor\exampletool\`,
    } {
        if strings.HasPrefix(path, root) && strings.HasSuffix(path, `\example-tool.exe`) {
            return true
        }
    }
    return false
}

func (ExampleHTTPToolAdapter) Supports(proxy domain.ProxyEndpoint, bypass domain.BypassPolicy) bool {
    host := strings.TrimSpace(proxy.Host)
    return bypass.Empty() &&
        proxy.Protocol == domain.ProtocolHTTP &&
        host != "" && !strings.ContainsAny(host, "/\\\r\n\t ") &&
        proxy.Port >= 1 && proxy.Port <= 65535 &&
        strings.TrimSpace(proxy.Username) == "" && strings.TrimSpace(proxy.Password) == ""
}

func (a ExampleHTTPToolAdapter) Plan(identity domain.AppIdentity, proxy domain.ProxyEndpoint, bypass domain.BypassPolicy) (domain.LaunchSpec, error) {
    if !a.Match(identity) {
        return domain.LaunchSpec{}, fmt.Errorf("example adapter does not match %q", identity.Executable)
    }
    if !a.Supports(proxy, bypass) {
        return domain.LaunchSpec{}, fmt.Errorf("example adapter requires an unauthenticated HTTP proxy and an empty bypass policy")
    }

    proxyURL := "http://" + proxy.Address()
    identityCopy := identity
    identityCopy.Arguments = append([]string(nil), identity.Arguments...)
    args := append([]string(nil), identity.Arguments...)
    args = append(args, "--proxy-server="+proxyURL)
    environment := map[string]string{
        "HTTP_PROXY":  proxyURL,
        "HTTPS_PROXY": proxyURL,
    }
    return domain.LaunchSpec{
        Identity:    identityCopy,
        Arguments:   args,
        Environment: environment,
    }, nil
}
```

把适配器注入组装入口，而不是在启动器里增加软件名称分支：

```go
desktop, err := application.NewDesktop(ctx, application.Dependencies{
    // ...store, resolver, process controller, probe and backend...
    // 仅在实际验证并决定扩大名单后加入示例适配器。
    Adapters: append(native.Production(), exampleadapter.NewExampleHTTPTool()),
})
```

示例的 `Match` 只依据已确认的路径前缀和可执行文件名，`Plan` 复制参数并创建新的环境映射，不执行任何系统命令。真实适配器还应按该软件的实际行为补充冲突代理参数清理和测试，然后再纳入入口注入列表。

## 退出与平台隔离

关闭窗口只隐藏。完整退出取消监视、释放强制代理和 HTTP 桥接，不调用目标程序的停止接口。管理员辅助进程只接受代理会话更新、状态握手和关闭，不接受任意命令执行。

Windows 和 macOS 分别通过带构建标签的组装文件注入 `AppResolver`、`ProcessController`、`CredentialStore` 和 `ForcedBackend`；核心用例及视图不散布系统类型判断。macOS 的强制代理后端返回明确不可用错误，仍会先于目标进程启动执行，保证不会将回退误当成成功。Linux 的入口直接返回未支持，不提供客户端产物。
