# Proxy Start 品牌资源

[`proxy-start.svg`](proxy-start.svg) 是品牌图标的唯一源文件：纯蓝色圆角底、几何 P 字母，以及负形的启动三角。图形完全使用路径构成，不依赖字体、外网图片或 AI 生成。

客户端侧栏直接读取嵌入的 SVG；窗口和托盘使用同一 SVG 导出的 32px PNG。所有 PNG 均由 MyGO 软件渲染器导出，保留透明边角，并在 PNG 元数据中记录 SVG 来源。重新生成：

```powershell
go generate ./assets
```

`proxy-start.ico` 包含 16、24、32、48、64、128、256px 条目，供 Windows EXE 与安装快捷方式使用。独立 PNG 提供 16、32、64、128、256、512、1024px；macOS 打包据此生成高分辨率 ICNS。
