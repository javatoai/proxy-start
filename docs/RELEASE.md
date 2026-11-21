# 发布验证

版本统一从根目录 `VERSION` 读取。Git 标签由 `v` 加上 `VERSION` 中的版本号组成。

发布页只上传两个资产：

- ProxyStart-windows-amd64.exe：单个 x64 GUI 程序，图标和第三方许可内嵌，无外置资源、脚本、配置或安装器。
- ProxyStart-macos-universal.dmg：包含 Apple Silicon / Intel 通用应用和 Applications 拖入入口，不放文档或开发文件。

Windows 使用 scripts/pack.ps1，macOS 使用 scripts/pack-macos.sh（或 PowerShell 包装入口）。DMG 必须在 macOS 使用 lipo、codesign、hdiutil 构建；本地临时签名不等于 Developer ID 签名或公证。

CI 先运行 Windows / macOS 原生测试、静态检查和平台集成用例，再构建。Windows 核对 GUI PE、版本与默认端口；macOS 验证双架构、应用版本、签名、DMG 完整性及只读挂载后的精确文件集合，并从挂载卷执行 --check。检查第三方许可可通过 --licenses。

scripts/verify-release-assets.py 验证两文件集合、PE GUI 子系统和 DMG UDIF 尾标识，将 SHA-256 输出到 CI 日志，不向分发目录写入附加校验文件。Release 先创建为草稿，上传后通过 scripts/verify-github-release.py 逐文件核对远端大小和摘要，全部通过才公开。GitHub 自动提供的 Source code 下载入口不属于手工上传的两个安装文件。

发布标签固定到已提交、已推送的源码。真实 Codex 网络流量与 Mac GUI / Terminal 验收另行记录；不以 CI 通过替代用户实机验收。
