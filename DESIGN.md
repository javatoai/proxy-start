---
name: Proxy Start Native Application Manager
description: A quiet, native Windows and macOS desktop console for starting explicitly selected applications through honest proxy modes.
colors:
  brand-blue: "#2563EB"
  accent-blue: "#2C6DE8"
  accent-blue-hover: "#2563D9"
  accent-blue-pressed: "#1D4ED8"
  accent-blue-dark: "#2C6DE8"
  accent-blue-dark-hover: "#2563D9"
  accent-blue-dark-pressed: "#1D4ED8"
  light-background: "#FFFFFF"
  light-surface: "#F3F5F7"
  light-surface-hover: "#E9EDF2"
  light-surface-pressed: "#DEE5ED"
  light-border: "#DCE1E8"
  light-text: "#202938"
  light-muted: "#667085"
  dark-background: "#171D26"
  dark-surface: "#232B37"
  dark-surface-hover: "#2C3746"
  dark-surface-pressed: "#364255"
  dark-border: "#394453"
  dark-text: "#EDF2F7"
  dark-muted: "#A8B3C2"
  sidebar-light: "#EDF2F6"
  selection-light: "rgba(52, 120, 246, 0.23)"
  selection-dark: "rgba(74, 140, 255, 0.34)"
  focus-light: "rgba(52, 120, 246, 0.52)"
  focus-dark: "rgba(104, 160, 255, 0.62)"
  success: "#177B46"
  success-dark: "#63D69A"
  warning: "#A85A08"
  warning-dark: "#F4B75E"
  danger: "#C53B3F"
  danger-dark: "#FF8A8F"
  mode-native-light: "#245AC2"
  mode-native-dark: "#8CB7FF"
typography:
  display:
    fontFamily: "Microsoft YaHei UI"
    fontSize: "24px"
    fontWeight: 600
  headline:
    fontFamily: "Microsoft YaHei UI"
    fontSize: "18px"
    fontWeight: 600
  title:
    fontFamily: "Microsoft YaHei UI"
    fontSize: "24px"
    fontWeight: 600
  body:
    fontFamily: "Microsoft YaHei UI"
    fontSize: "14px"
    fontWeight: 400
  secondary:
    fontFamily: "Microsoft YaHei UI"
    fontSize: "12px"
    fontWeight: 400
  label:
    fontFamily: "Microsoft YaHei UI"
    fontSize: "13px"
    fontWeight: 600
rounded:
  control: "8px"
  detail: "6px"
  container: "8px"
  dialog: "12px"
spacing:
  xxs: "4px"
  xs: "8px"
  sm: "12px"
  md: "16px"
  lg: "24px"
  xl: "28px"
components:
  button-primary:
    backgroundColor: "{colors.accent-blue}"
    textColor: "#FFFFFF"
    typography: "{typography.body}"
    rounded: "{rounded.control}"
    padding: "0 14px"
    height: "36px"
  button-secondary:
    backgroundColor: "{colors.light-surface}"
    textColor: "{colors.light-text}"
    typography: "{typography.body}"
    rounded: "{rounded.control}"
    padding: "0 12px"
    height: "36px"
  input:
    backgroundColor: "{colors.light-surface}"
    textColor: "{colors.light-text}"
    typography: "{typography.body}"
    rounded: "{rounded.control}"
    padding: "0 11px"
    height: "36px"
  nav-active:
    backgroundColor: "{colors.accent-blue}"
    textColor: "#FFFFFF"
    typography: "{typography.body}"
    rounded: "{rounded.control}"
    padding: "0 12px"
    height: "40px"
  table-row:
    backgroundColor: "{colors.light-background}"
    textColor: "{colors.light-text}"
    typography: "{typography.body}"
    rounded: "0px"
    padding: "8px 12px"
    height: "56px"
  brand-mark:
    backgroundColor: "{colors.brand-blue}"
    rounded: "{rounded.container}"
    size: "32px"
  operation-toast:
    backgroundColor: "{colors.light-background}"
    textColor: "{colors.light-text}"
    typography: "{typography.secondary}"
    rounded: "{rounded.control}"
    maxWidth: "420px"
    padding: "10px 14px"
  installed-picker:
    backgroundColor: "{colors.light-background}"
    textColor: "{colors.light-text}"
    rounded: "12px"
    width: "680px"
    height: "580px"
    padding: "24px"
---

# Design System: Proxy Start Native Application Manager

## Overview

**Creative North Star: "The Quiet Operations Console"**

Proxy Start is a native Windows and macOS utility used while the user is thinking about another application. The interface is calm, legible, and operational: a pale blue-gray rail establishes place, while the working surface gives the application list and its real runtime state the full visual priority. The rail contains exactly 应用, 代理设置, and 系统设置. The supplied application-manager reference informs the hierarchy, compact rows, sidebar material, and restrained table rhythm; the product remains a native proxy tool with concise Chinese copy and native controls.

The system uses one blue interactive voice over theme-derived surfaces. Light mode keeps the main work surface white and the rail pale blue-gray; dark mode uses charcoal-blue surfaces without inverting status semantics. Color is reserved for selection, primary actions, focus, the shared brand mark, and honest runtime/probe states. Depth comes from surface changes and fine borders rather than decoration or floating cards.

**Key Characteristics:**
- Native desktop posture with a 214px work rail and a spacious working surface; use Windows UI fonts on Windows and the native system font on macOS.
- Compact, scanable application table with stable semantic columns and real application icons.
- One blue action voice; green, amber, and red only carry runtime or probe meaning.
- Light, dark, and system themes share the same structure and spacing rhythm.
- No gradients, fake connectivity indicators, invented usage statistics, or decorative dashboards.

## Colors

The palette is a cool neutral foundation with a product-blue mark and one stable blue interaction voice. Light and dark values are paired by role so navigation, controls, selection, and state labels remain legible across theme changes.

### Primary

- **Brand Blue** (`{colors.brand-blue}`): The geometric Proxy Start mark rendered in the sidebar, window icon, tray icon, and installed shortcut asset.
- **Product Blue** (`{colors.accent-blue}`): Primary actions, active navigation, and selected controls in both themes; the same accent is kept across light and dark surfaces for reliable contrast with white labels.
- **Product Blue Hover / Pressed** (`{colors.accent-blue-hover}` / `{colors.accent-blue-pressed}`): Immediate interaction states for product-blue controls.
- **Night Product Blue** (`{colors.accent-blue-dark}`): The dark-theme name for the same product accent; `{colors.accent-blue-dark-hover}` and `{colors.accent-blue-dark-pressed}` provide its interaction states.
- **Focus Blue** (`{colors.focus-light}` / `{colors.focus-dark}`): Visible keyboard focus treatment in the corresponding theme.

### Tertiary

- **Success Green** (`{colors.success}` / `{colors.success-dark}`): A real applied, ready, or verified runtime/probe result.
- **Attention Amber** (`{colors.warning}` / `{colors.warning-dark}`): Starting, stopping, pending, forced-mode, or attention-needed state.
- **Failure Red** (`{colors.danger}`): A failed operation or error detail. The dark theme inherits the native theme's danger value rather than declaring a separate product token.
- **Native Mode Blue** (`{colors.mode-native-light}` / `{colors.mode-native-dark}`): The readable text color for the native-proxy mode label on light and dark neutral surfaces; it is intentionally separate from the blue action background.

### Neutral

- **Light Canvas / Control Surface** (`{colors.light-background}` / `{colors.light-surface}`): The light launch/settings work surface and quiet control faces.
- **Light Hover / Pressed** (`{colors.light-surface-hover}` / `{colors.light-surface-pressed}`): Native control feedback.
- **Light Border / Ink / Muted Ink** (`{colors.light-border}` / `{colors.light-text}` / `{colors.light-muted}`): One-pixel structure, primary copy, and secondary copy.
- **Pale Blue-Gray Rail** (`{colors.sidebar-light}`): The light-theme navigation rail, separate from the working canvas without becoming a floating card.
- **Dark Canvas / Surface** (`{colors.dark-background}` / `{colors.dark-surface}`): The dark-theme work surface and control face.
- **Dark Hover / Pressed** (`{colors.dark-surface-hover}` / `{colors.dark-surface-pressed}`): Dark control states.
- **Dark Border / Ink / Muted Ink** (`{colors.dark-border}` / `{colors.dark-text}` / `{colors.dark-muted}`): Dark-theme structure and text hierarchy.

### Named Rules

**The One Blue Voice Rule.** Use product blue for action, selection, focus, and the active rail destination. Do not turn every informational label into accent-colored decoration.

**The Honest State Rule.** Green means an actual applied, ready, or verified result; amber means pending, forced, or attention-needed; red means failure. A launched process alone never earns a verified state.

## Typography

**Display Font:** Microsoft YaHei UI

**Body Font:** Microsoft YaHei UI

**Label/Mono Font:** Microsoft YaHei UI; executable paths remain regular UI text and wrap when needed.

**Character:** The single native UI family keeps Chinese and Latin labels cohesive. The current font diagnostic confirms that Microsoft YaHei UI is the rendered family: 400 and 500 resolve to the same regular face, while 600 and 700 resolve to the same bold face; the documented system does not introduce a fallback family.

### Hierarchy

- **Display** (600, `24px`): Page titles such as 应用, 代理设置, and 系统设置.
- **Headline / Brand** (600, `18px`): The Proxy Start lockup and editor section titles.
- **Title** (600, `24px`): The canonical page-heading token used by 应用, 代理设置, and 系统设置; profile-list headings use the smaller 18px/600 headline treatment, while the app inspector uses a compact 16px heading where space is constrained.
- **Body** (400, `14px`): The root native theme size for controls, navigation labels, and application names. Application names may request 500, which resolves to the same regular face.
- **Secondary** (400, `12px`): Counts, descriptions, helper text, paths, and runtime detail.
- **Label** (600, `13px`): Settings field labels, status headings, and compact section labels. Navigation uses 14px/500.

### Named Rules

**The Weight Before Size Rule.** Establish hierarchy with weight and spacing before introducing a larger type size; keep dense tables readable without oversized labels.

## Layout

The adaptive native window opens at `1180×760` logical pixels and has a minimum useful size of `820×560`. The 214px rail is fixed; the work surface grows into the remaining width. The rail uses `24px` top, `16px` horizontal, and `20px` bottom padding with a 20px vertical gap. Main page headers are 88px high, content insets are 24px, and the recurring rhythm is 4/8/12/16/24/28px.

The launch surface follows the reference manager: a 24px/600 page title and count line, a 36px control row, then a bordered application table that grows and scrolls. At widths of 1040px and above, the header carries search and the segmented filter while the toolbar carries the actions and quiet helper copy. Below 1040px, the actions move into the header and search/filter controls occupy the toolbar. Search is 248px wide at the default window and 300px at widths of 1440px and above; it remains flexible with a 120px minimum in the compact arrangement.

The table uses a 36px header and 56px rows with 8px/12px row padding. The normal grid is icon 40px, Name minimum 140px, Proxy 166px, Mode 88px, Status 116px, Actions 142px, with 10px gaps. Below 920px, Mode is dropped and the compact grid uses icon 32px, Name minimum 112px, Proxy 144px, Status 92px, Actions 124px, and 6px gaps. When a selected application has a fixed 300px inspector at 1040px and above, the table uses icon 32px, Name minimum 100px, Proxy 126px, Status 84px, Actions 124px, with no Mode column and 6px gaps.

The selected application's inspector is a 300px right column at or above 1040px. Its long paths and runtime details live in an internal scroll body; the 36px save/remove action row remains reachable at the bottom of the panel. Below 1040px, the inspector becomes a separate full-width detail view with a 36px 返回 action and the same scrollable body/action-bar split.

代理设置是独立的表格页，内容最大宽度为 1060px。页头保留 24px 内容留白和“新增代理”主按钮；表格列固定为 名称 / 地址 / 协议 / 默认标记 / 操作，每行的操作依次提供打开代理控制台、编辑和删除。新增 / 编辑使用原生模态表单，字段包含名称、地址、协议、认证信息和默认选项，并提供测试动作；删除必须先经过确认模态。表格和模态的输入控件为 36px 高，操作在窄窗口中自然换行。

系统设置是单列、最大宽度 `760px` 的垂直滚动页，按顺序呈现外观主题、应用列表中的隐藏系统软件和强制代理组件。分隔线和 24px 内容留白保持各段边界，不再把应用列表偏好或组件状态放在代理编辑器的底部行。

操作反馈使用窗口右上角的悬浮提示，不预留底部状态栏空间。成功提示约 4 秒后自动消失，错误提示可关闭并持续显示，进行中的操作显示为持续提示直到结果替换它。提示最大宽度为 420px，保留清晰的状态色和关闭按钮。

## Elevation & Depth

The system is flat by default and declares no shadow or motion vocabulary. Depth comes from contrast between canvas, control surface, selection fill, and one-pixel borders. The sidebar is a tonal rail, the table is a bordered working region, and the inspector/settings editor are quiet surfaces within the same window. A two-pixel accent border appears only during drag-over; modal surfaces use the native window treatment and a 12px container radius.

### Named Rules

**The Tonal Layer Rule.** Prefer a surface shift or a one-pixel border to a shadow. Use a stronger border only for drag-over, focus, or an explicit error state.

## Shapes

The native theme radius is 8px and is carried by controls, navigation rows, search/filter surfaces, profile items, and table/inspector containers. Detail/error surfaces use a quiet 6px radius; the installed-app modal uses 12px. Table rows remain square so the list reads as one continuous surface; selection uses a filled tint rather than a capsule. Borders are one pixel and theme-derived, with a two-pixel accent border during drag-over.

## Components

### Buttons

- **Shape:** 36px-high native controls with gently rounded 8px corners (`{rounded.control}`); the navigation row is 40px high.
- **Primary:** Product-blue fill with white text, 14px horizontal padding for the main action, and 12px for the adjacent installed-app action.
- **Hover / Focus:** Move to the theme's accent-hover value on hover, use the pressed value on activation, and keep the native focus ring visible with the theme focus color.
- **Secondary / Ghost / Tertiary:** Neutral surface with a one-pixel border for adjacent actions such as adding an installed app, stopping, restarting, testing, or deleting. Danger text is reserved for deletion and failure copy.

### Chips

- **Style:** Compact status badges use theme text and a quiet surface treatment; the default profile badge is a semantic label such as 默认 rather than a decorative pill.
- **State:** Selected filters and navigation use blue selection/active treatment. Runtime labels use success, warning, or danger only when the state is real.

### Cards / Containers

- **Corner Style:** 8px for the application table, inspector, profile items, and control surfaces; 12px for the installed-app modal; 6px for diagnostic/detail boxes.
- **Background:** Canvas for the main table; theme surface for controls, inspector, settings editor, and secondary panels.
- **Shadow Strategy:** Flat and border-led; see Elevation & Depth.
- **Border:** One-pixel theme border around the table, controls, inspector separators, settings split, and diagnostic boxes.
- **Internal Padding:** 8px/12px for table rows, 18px for the inspector, 20px/24px for settings body insets, 28px for the settings editor, and 24px for the modal.

### Inputs / Fields

- **Style:** Native text inputs and selects use a theme surface, one-pixel border, 8px radius, 36px height, and 11px horizontal breathing room for the search field.
- **Focus:** The search shell changes its border to the accent while focused; native fields retain a visible theme focus ring.
- **Error / Disabled:** Errors wrap in place and keep recovery actions visible. Busy operations disable only their duplicate action and retain its label.

### Navigation

The rail is 214px wide with the 32px geometric brand mark, an 18px/600 product name, a concise 12px subtitle, a one-pixel divider, and three 40px workspace destinations for 应用, 代理设置, and 系统设置. The active destination is a filled product-blue row with a 17px line icon, 10px icon/text gap, and white 14px/500 text; the inactive row stays quiet and uses the theme surface-hover on hover. The rail contains only the brand lockup and workspace navigation; no component status or tray explanation returns to its bottom edge.

### Application Table

The table is the signature component. Its header exposes 名称 / 代理 / 启动方式 / 状态 / 操作, and each row keeps the real app icon, name, proxy selector, mode, runtime label, and action cluster on one alignment line. Alternating neutral surface tones help scanning; selection uses the theme selection tint and exposes the inspector at wide widths. Search, filters, drag-over, and empty states remain part of the working surface rather than a permanent drop card.

### Application Inspector

The inspector is a quiet surface with a compact heading, three 36px fields, wrapped executable paths, and an internal scroll body. Save and remove actions stay in a 36px action bar below that body, so long paths and diagnostics do not hide the controls at the minimum window size. In the narrow detail view, the close action becomes 返回 and restores the application table.

### Proxy Settings

代理设置以表格为主工作面，列为 名称、地址、协议、默认标记和操作。每行把“打开代理控制台”“编辑”“删除”保持在同一操作组；页头的“新增代理”打开同一套模态表单。新增 / 编辑模态支持认证信息、默认选项和测试；删除动作使用单独的确认模态，危险色只用于删除与错误。

### System Settings

系统设置使用单列 `760px` 最大宽度和垂直滚动，包含外观主题、应用列表隐藏系统软件和强制代理组件。每个设置组独立显示标题、说明和原生控件，设置页不包含代理表格的编辑栏，也不把组件状态压到页面底部。

### Connection Test Result

Connection tests use a semantic state dot, a short neutral-ink conclusion, and a separate protocol summary beneath the action group. Raw CONNECT replies and diagnostic errors remain behind 查看详情. Re-testing shows a pending state, and a failed test replaces any earlier success. Editing and saving a proxy clears its old result; recognition alone never earns a verified label. Optional authentication fields start collapsed when unused, open automatically for existing authentication, and retain their values when manually collapsed.

### Proxy Console

The “打开代理控制台” row action opens a new platform shell through the `application.ConsoleLauncher` use case. `Desktop.OpenProxyConsole` resolves the selected profile, prefers a verified HTTP result when protocol detection is automatic, then falls back to verified SOCKS5. The shell receives URL-encoded proxy values in `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY` and their lowercase counterparts; `NO_PROXY` and `no_proxy` are removed for that child shell. Only tools that read these environment variables are affected; a tool that ignores them or supplies its own proxy setting is outside this console contract. Windows uses a new PowerShell console, while macOS opens Terminal through a private temporary script that removes itself before handing control to zsh. This is a scoped environment action and never implies verified target connectivity.

### Installed Application Picker

The existing native modal uses a 680×580px content surface, clamped to leave 24px on each side of the host window at smaller sizes. A single 36px search field receives initial keyboard focus and filters names, paths and package identities without changing launch-page search. Count, loading, failure with retry, empty catalog and no-result recovery remain in the picker. The list virtualizes 60px rows with a fixed 32px real system icon, a flexible name/path column, and a fixed 80×36px action. Already-added launch entries show a disabled 已添加 action immediately, matched by stable package identity or executable path rather than display name. Unavailable icons use a centered neutral glyph. Visible rows and small overscan request icons asynchronously with at most four workers; cached images persist across picker sessions, while closing cancels outstanding work. Search resets the virtual list to its first result. Escape and the close action dismiss the picker.

Prefer Start-menu and manifest display names, with executable metadata as a fallback. Frameworks, resource packages and hidden background entries do not belong in a launchable application catalog. Hide Microsoft and Windows system software by default and disclose the active filter in the result count. The 应用列表 section in 系统设置 exposes a native 隐藏系统软件 switch with a short explanation; it remains reachable even when no proxy profiles exist. Persist this preference independently of picker query and runtime state. Third-party applications under WindowsApps remain visible.

## Do's and Don'ts

### Do:

- **Do** use the theme tokens for every surface, border, text, selection, focus, and state color so system, light, and dark modes remain coherent.
- **Do** keep the 214px rail, 24px content inset, 88px page header, and 4/8/12/16/24/28px spacing rhythm unless a responsive rule requires compression.
- **Do** preserve the stable table column grid and let the Name column absorb available width.
- **Do** keep long detail bodies scrollable while leaving their 36px action bar reachable.
- **Do** let proxy and system settings scroll within their own content columns, keep modal actions in one wrapping row, and place operation feedback in the top-right toast viewport.
- **Do** expose real operation progress, disabled duplicate actions, wrapped errors, and clear recovery actions.
- **Do** keep drag-and-drop discoverable through the table surface and a restrained accent border while dragging.

### Don't:

- **Don't** add gradients, glassmorphism, decorative shadows, fake connectivity indicators, or invented usage statistics.
- **Don't** use a blue label for a state merely because it looks active; reserve semantic colors for their documented meanings.
- **Don't** hide the selected-program inspector in repeated table rows or make it permanently consume narrow-window width.
- **Don't** claim proxy connectivity because a target process started; retain the distinction between applied, running, and verified.
- **Don't** replace native Windows controls with web-shaped controls, oversized cards, or a decorative empty drop zone.
