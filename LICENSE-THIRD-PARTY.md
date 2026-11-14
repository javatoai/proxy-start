# 第三方组件与许可

Proxy Start 使用下列 Go 模块，不再分发第三方代理引擎、服务或驱动安装程序。

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

实际分发的许可全文保存在根目录 `THIRD-PARTY-NOTICES.txt`，包含上述模块和 Go 标准库许可，并通过 go:embed 编入 Windows / macOS 程序。运行 `--licenses` 可读取，分发包无需外置许可文件。
