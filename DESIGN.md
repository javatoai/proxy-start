---
name: Proxy Start Codex Launcher
description: A compact, light native launcher with immediate Codex status and one shared proxy port.
colors:
  brand-blue: "#2563EB"
  accent-blue: "#2C6DE8"
  accent-blue-hover: "#2563D9"
  accent-blue-pressed: "#1D4ED8"
  light-background: "#FFFFFF"
  light-surface: "#F3F5F7"
  light-surface-hover: "#E9EDF2"
  light-surface-pressed: "#DEE5ED"
  light-border: "#DCE1E8"
  light-text: "#202938"
  light-muted: "#667085"
  success: "#177B46"
  danger: "#C53B3F"
typography:
  title:
    fontFamily: "Microsoft YaHei UI"
    fontSize: "22px"
    fontWeight: 600
  body:
    fontFamily: "Microsoft YaHei UI"
    fontSize: "14px"
    fontWeight: 400
components:
  status-circle:
    size: "160px"
    rounded: "80px"
  port-input:
    height: "38px"
    width: "112px"
    rounded: "8px"
  action:
    height: "38px"
    rounded: "8px"
---

# Proxy Start 界面规范

使用场景为 Operate：用户进入后立即确认 Codex 桌面端的运行状态，然后按需要启动或打开控制台。固定浅色，不随系统切换，不提供主题设置。

## 布局与层级

默认窗口 460×420，最小 380×360；使用新的窗口状态键，避免旧版大窗口尺寸覆盖紧凑默认值。页边留白 20px，右上角为 32px 齿轮，页面中央为状态圆圈和两按钮操作行。

圆圈通常为 160px，较矮的原生内容区缩为 144px。运行中为浅绿，未启动为浅蓝，未知为中性灰。圆圈始终用于刷新状态，提示和辅助技术标签明确写“刷新状态”。不再显示单独的重新扫描按钮。

圆圈下方两个 116×38 按钮，间距 12px。主按钮按状态显示启动或重启；停止按钮在未启动时置灰。操作中禁用重复动作并显示启动中 / 重启中 / 停止中。

刷新时使用沿圆周旋转、弧长缓慢变化的 3px 弧线，180ms 淡入淡出，文字不旋转；手动刷新至少保留 650ms 的可见过程。动画只在活动期间绘制并遵循系统减少动态效果设置，结束后不持续请求帧。

齿轮打开约 420px 的设置弹窗，包含固定本机地址、代理端口输入、打开代理控制台和取消 / 保存。控制台动作先保存当前输入端口，再启动独立控制台；失败时保留弹窗并显示错误。取消及关闭丢弃草稿；保存先校验端口，再异步持久化，成功才关闭并采用新值。失败保留弹窗和原配置，错误在弹窗内显示。弹窗采用单层白色面板，不嵌套卡片。

## 字体与状态

Windows 使用 Microsoft YaHei UI / Segoe UI；macOS 使用原生系统字体。正文 14px，说明 12px，状态 13px。使用同一蓝色交互语义，保留原 SVG 品牌资产。圆角 6–8px，分隔边框不叠加装饰阴影。

扫描中、未找到、未启动、已启动和状态待确认都有明确文案。异步操作期间禁用重复动作，错误在正文中持续显示。代理观察保留在诊断接口中，不在主页展示。

## 验证

使用 MyGO 原生渲染检查默认与最小尺寸、扫描和失败状态、设置弹窗的保存 / 取消 / 校验与按钮可见性。以系统深色作为输入验证页面仍为浅色。UI 状态测试不启动或结束用户应用。
