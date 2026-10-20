# 平台范围与后续实现

当前实现 Windows 与 macOS 原生客户端。Windows 原生适配 Codex 和 Store 版 Claude Desktop，另有按程序强制代理后端。macOS 原生适配 Codex，提供 arm64 / amd64 交叉编译包，尚未实机验收，不包含强制代理。Linux 暂不支持，也不生成 Linux 产物。

## macOS

现有实现解析 `.app` 的 bundle ID、可执行路径与友好名称，使用 macOS 进程身份与 Keychain，通过 Codex 原生启动计划传入代理环境和参数。安装与待执行的实机检查见 [MACOS.md](MACOS.md)。

真正按应用透明转发仍是后续工作，优先验证 Apple `NETransparentProxyProvider`。MyGO 保留界面，Swift/Objective-C 系统扩展处理 TCP/UDP flow，再按来源应用身份选择转发或放行；本次没有实现或宣称该能力可用。

需要验证 Developer ID、Network Extension entitlement、签名与公证、首次扩展批准，以及签名应用/未签名应用/辅助进程的身份解析。不要把依赖 MDM 的 per-app VPN 当成任意用户拖入应用的等价方案。

参考：[NETransparentProxyProvider](https://developer.apple.com/documentation/networkextension/netransparentproxyprovider)、[Network Extension 部署](https://developer.apple.com/documentation/technotes/tn3134-network-extension-provider-deployment)。

## Linux

根据当前产品范围暂不开发。核心领域与应用接口继续保持平台独立，保留后续扩展可能，不等于已经提供 Linux 客户端。
