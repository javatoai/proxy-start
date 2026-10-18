# 第三方组件与许可

Proxy Start 将第三方组件与主程序分开管理。发布包中的第三方安装程序只在用户明确选择安装时运行；构建与打包脚本不会自动安装驱动、服务或其他系统组件。

## ProxiFyre 2.6.4

- 项目与源代码：https://github.com/wiresock/proxifyre
- 版本发布页：https://github.com/wiresock/proxifyre/releases/tag/v2.6.4
- 许可：AGPL-3.0（上游仓库声明）：https://github.com/wiresock/proxifyre/blob/main/LICENSE
- 发布资产：`ProxiFyre-2.6.4-win-x64-setup.exe`
- 资产地址：https://github.com/wiresock/proxifyre/releases/download/v2.6.4/ProxiFyre-2.6.4-win-x64-setup.exe
- SHA-256：`afaf3ac8a2069f9653adb1b4a63b31d1edd8aa109ccdc367cf0d4462e78f5027`

Proxy Start 只分发上方官方 x64 安装程序，不修改其中的内容。安装程序及其包含的 NDISAPI/Windows Packet Filter 组件仍由 ProxiFyre 上游许可和版权声明约束。需要源代码、完整许可文本或对应版本的合规材料时，应以 ProxiFyre 上游仓库和发布页为准。

## MyGo v0.5.5

- 项目与源代码：https://github.com/egoist/mygo/tree/v0.5.5
- 许可：MIT（上游许可文件）：https://github.com/egoist/mygo/blob/v0.5.5/LICENSE
- 版本：`github.com/egoist/mygo v0.5.5`

## Go 模块依赖

Proxy Start 的 Go 模块依赖记录在 `go.mod`/`go.sum` 中。对应项目和许可信息可从各自官方仓库获得：

- `github.com/ebitengine/purego v0.11.1` — https://github.com/ebitengine/purego
- `github.com/go-text/typesetting v0.3.5` — https://github.com/go-text/typesetting
- `golang.org/x/image v0.46.0` — https://go.dev/x/image
- `golang.org/x/text v0.43.0` — https://go.dev/x/text

这些模块的许可文件随其源代码发布；发布包不会把 Go SDK（`.tools/`）或模块缓存打进 Windows 分发包。版本校验以 `go.sum` 为准。
