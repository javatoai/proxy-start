package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/transfer"
	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
)

func (m *model) launchPage(c *nativeui.Context) {
	t := c.Theme()
	nativeui.Column(c).Fill().Background(t.Background).Children(func() {
		m.launchHeader(c)
		m.launchToolbar(c)
		width, _ := c.Size()
		if entry, ok := m.appByID(m.selectedAppID); ok {
			if width >= 1040 {
				nativeui.Row(c).Grow(1).MinHeight(0).FillHeight().Children(func() {
					m.appTable(c)
					m.appInspector(c, entry)
				})
			} else {
				// At the minimum window width the inspector becomes a separate
				// detail view. This keeps the table usable and gives the user a
				// reliable return path instead of stacking two cramped panes.
				m.appInspector(c, entry)
			}
		} else {
			m.appTable(c)
		}
	})
}

func (m *model) launchHeader(c *nativeui.Context) {
	t := c.Theme()
	running := m.runningAppCount()
	width, _ := c.Size()
	nativeui.Row(c).Height(88).Padding(20, 24, 16, 24).Gap(16).AlignItems(nativeui.Center).Children(func() {
		nativeui.Column(c).Grow(1).MinWidth(100).Gap(5).Children(func() {
			nativeui.Text(c, "应用").FontSize(24).FontWeight(600)
			nativeui.Text(c, fmt.Sprintf("%d 个应用 · %d 个运行中", len(m.snapshot.Settings.Apps), running)).FontSize(12).TextColor(t.TextMuted)
		})
		if width >= 1040 {
			searchWidth := float32(248)
			if width >= 1440 {
				searchWidth = 300
			}
			m.searchApplications(c, searchWidth)
			m.applicationFilter(c)
		} else {
			m.launchActions(c, true)
		}
	})
}

func (m *model) launchToolbar(c *nativeui.Context) {
	t := c.Theme()
	width, _ := c.Size()
	nativeui.Row(c).Padding(0, 24, 18, 24).Gap(12).AlignItems(nativeui.Center).Children(func() {
		if width >= 1040 {
			m.launchActions(c, false)
			nativeui.Spacer(c).Grow(1)
			nativeui.Text(c, "只为已添加的程序应用代理规则").FontSize(12).TextColor(t.TextMuted).SingleLine()
		} else {
			m.searchApplications(c, 0)
			m.applicationFilter(c)
		}
	})
}

func (m *model) launchActions(c *nativeui.Context, compact bool) {
	if nativeui.PrimaryButton(c, "添加程序").Height(36).Padding(0, 14).Label("添加程序").Clicked() {
		m.openProgramDialog()
	}
	installedLabel := "添加已安装应用"
	if compact {
		installedLabel = "已安装应用"
	}
	if nativeui.Button(c, installedLabel).Height(36).Padding(0, 12).Label("添加已安装应用").Clicked() {
		m.loadInstalled()
	}
}

func (m *model) searchApplications(c *nativeui.Context, width float32) {
	t := c.Theme()
	field := nativeui.Box(c).Height(36).Padding(0, 11).Radius(8).Border(1, t.Border).Background(t.Surface)
	if width > 0 {
		field.Width(width)
	} else {
		field.Grow(1).MinWidth(120)
	}
	var input nativeui.Element
	field.Children(func() {
		nativeui.Row(c).Fill().Gap(8).AlignItems(nativeui.Center).Children(func() {
			nativeui.Icon(c, searchIcon).Size(16, 16).TextColor(t.TextMuted)
			input = nativeui.TextInputBase(c.Key("app-search"), &m.appSearch).Label("搜索应用").Placeholder("搜索应用").Grow(1).MinWidth(0).Height(34).Padding(0).FocusRing(false)
			if m.appSearch != "" {
				clear := nativeui.ButtonBase(c).Size(22, 24).Radius(5).Label("清空搜索").TextColor(t.TextMuted)
				if clear.Hovered() {
					clear.Background(t.SurfaceHover)
				}
				clear.Children(func() { nativeui.Icon(c, clearIcon).Size(13, 13) })
				if clear.Clicked() {
					m.appSearch = ""
					input.Focus()
				}
			}
		})
	})
	if input.Focused() {
		field.Border(1, t.Accent)
	}
}

func (m *model) applicationFilter(c *nativeui.Context) {
	values := [...]string{"all", "running", "stopped"}
	selected := 0
	for index, value := range values {
		if m.appFilter == value {
			selected = index
		}
	}
	if nativeui.Segmented(c.Key("app-filter"), &selected, "全部", "运行中", "未运行").Height(36).Label("应用运行筛选").Changed() {
		m.appFilter = values[selected]
	}
}

func (m *model) appTable(c *nativeui.Context) {
	t := c.Theme()
	opts := transfer.DropOptions{Formats: []transfer.Format{transfer.FileList, transfer.URIList, transfer.Text}, Operations: transfer.Copy}
	table := nativeui.Column(c).Grow(1).MinHeight(0).FillWidth().FillHeight().Border(1, t.Border).Radius(8).Background(t.Background)
	if _, over := nativeui.DataDragOver(table, opts); over {
		table.Border(2, t.Accent).Background(t.Selection)
	}
	table.Children(func() {
		m.appTableHeader(c)
		nativeui.Divider(c)
		apps := m.filteredApps()
		if len(apps) == 0 {
			nativeui.Column(c).Grow(1).Center().Gap(6).Children(func() {
				if len(m.snapshot.Settings.Apps) == 0 {
					nativeui.Text(c, "还没有添加应用").FontSize(16).FontWeight(600)
					nativeui.Text(c, "点击“添加程序”，或将 exe / 快捷方式拖到此表格").FontSize(12).TextColor(t.TextMuted)
				} else {
					nativeui.Text(c, "没有匹配的应用").FontSize(15).FontWeight(600)
					nativeui.Text(c, "试试其他搜索词或筛选条件").FontSize(12).TextColor(t.TextMuted)
				}
			})
			return
		}
		nativeui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			for index, entry := range apps {
				m.appRow(c, entry, index)
			}
		})
	})
	if drop, ok := nativeui.DropData(table, opts); ok {
		if paths, err := drop.Data.Files(); err == nil {
			for _, path := range paths {
				m.addPath(path)
			}
		} else if text, err := drop.Data.Read(transfer.Text); err == nil {
			for _, path := range strings.Fields(string(text)) {
				m.addPath(path)
			}
		}
	}
}

func (m *model) appTableHeader(c *nativeui.Context) {
	t := c.Theme()
	metrics := m.tableMetrics(c)
	nativeui.Row(c).Height(36).Padding(0, 12).Gap(metrics.gap).AlignItems(nativeui.Center).Background(t.Surface).Children(func() {
		nativeui.Box(c).Width(metrics.iconWidth)
		nativeui.Text(c, "名称").Grow(1).MinWidth(metrics.nameWidth).FontWeight(600)
		nativeui.Text(c, "代理").Width(metrics.proxyWidth).FontWeight(600)
		if metrics.showMode {
			nativeui.Text(c, "启动方式").Width(metrics.modeWidth).FontWeight(600)
		}
		nativeui.Text(c, "状态").Width(metrics.statusWidth).FontWeight(600)
		nativeui.Text(c, "操作").Width(metrics.actionsWidth).FontWeight(600)
	})
}

func (m *model) appRow(c *nativeui.Context, entry domain.AppEntry, index int) {
	t := c.Theme()
	metrics := m.tableMetrics(c)
	draft := m.appDraft(entry)
	runtime := m.snapshot.Runtimes[entry.ID]
	selected := entry.ID == m.selectedAppID
	rowColor := t.Background
	if index%2 == 1 {
		rowColor = t.Surface
	}
	if selected {
		rowColor = t.Selection
	}
	row := nativeui.Box(c.Key("app-row-"+entry.ID)).FillWidth().MinHeight(56).Padding(8, 12).Background(rowColor).Label("选择应用 " + draft.name)
	row.Children(func() {
		nativeui.Row(c).Gap(metrics.gap).AlignItems(nativeui.Center).Children(func() {
			nativeui.Column(c).Width(metrics.iconWidth).AlignItems(nativeui.Center).Children(func() {
				m.appIcon(c, entry)
			})
			nativeui.Column(c).Grow(1).MinWidth(metrics.nameWidth).Gap(2).Children(func() {
				nativeui.Text(c, firstNonEmpty(draft.name, entry.Identity.Name, "未命名应用")).FontSize(14).FontWeight(500).SingleLine()
				if detail := firstNonEmpty(runtime.Error, runtime.Detail); detail != "" {
					color := t.TextMuted
					if runtime.Error != "" || (runtime.Mode == domain.ModeForced && runtime.State == domain.StateFailed) {
						color = t.Danger
					}
					nativeui.Text(c, detail).FontSize(12).TextColor(color).SingleLine()
				}
			})
			m.proxySelectWidth(c, draft, metrics.proxyWidth)
			if metrics.showMode {
				nativeui.Text(c, launchModeLabel(runtime.Mode)).Width(metrics.modeWidth).FontSize(12).TextColor(modeColor(t, runtime.Mode)).SingleLine()
			}
			nativeui.Text(c, runtimeLabel(runtime)).Width(metrics.statusWidth).FontSize(12).TextColor(runtimeColor(t, runtime)).SingleLine()
			m.appActions(c, entry, runtime, metrics.actionsWidth)
		})
	})
	if row.Clicked() {
		m.selectedAppID = entry.ID
	}
	if index < len(m.filteredApps())-1 {
		nativeui.Box(c).Height(1).FillWidth().Background(t.Border)
	}
}

func (m *model) appInspector(c *nativeui.Context, entry domain.AppEntry) {
	t := c.Theme()
	width, _ := c.Size()
	draft := m.appDraft(entry)
	runtime := m.snapshot.Runtimes[entry.ID]
	panel := nativeui.Column(c).MinHeight(0).Padding(18, 18).Gap(12).Background(t.Surface)
	if width >= 1040 {
		panel.Width(300).FillHeight()
	} else {
		panel.FillWidth().Grow(1)
	}
	panel.Children(func() {
		nativeui.Row(c).AlignItems(nativeui.Center).Children(func() {
			nativeui.Column(c).Grow(1).Gap(2).Children(func() {
				nativeui.Text(c, "应用详情").FontSize(16).FontWeight(600)
				nativeui.Text(c, firstNonEmpty(draft.name, entry.Identity.Name, "未命名应用")).FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
			closeLabel := "关闭"
			accessibilityLabel := "关闭应用详情"
			if width < 1040 {
				closeLabel = "返回"
				accessibilityLabel = "返回应用列表"
			}
			if nativeui.Button(c, closeLabel).Height(36).Label(accessibilityLabel).Clicked() {
				m.selectedAppID = ""
			}
		})
		nativeui.Divider(c)
		// Keep long paths and error details scrollable, with actions pinned
		// below the body so they stay reachable at the minimum window size.
		nativeui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			nativeui.Column(c).FillWidth().Gap(14).Children(func() {
				nativeui.Field(c, "显示名称", func() {
					nativeui.TextInput(c.Key("inspector-name"), &draft.name).Height(36).Placeholder("应用名称")
				})
				nativeui.Field(c, "使用代理", func() {
					m.proxySelect(c, draft)
				})
				nativeui.Field(c, "关联程序", func() {
					nativeui.TextInput(c.Key("inspector-related"), &draft.related).Height(36).Placeholder("可选，逗号分隔 exe 路径")
				}).Description("强制代理模式会同时覆盖这些关联程序")
				nativeui.Text(c, "程序路径").FontSize(12).FontWeight(600).TextColor(t.TextMuted)
				nativeui.Text(c, entry.Identity.Executable).FontSize(12).TextColor(t.TextMuted).Wrap()
				if len(entry.RelatedExecutables) > 0 {
					for _, path := range entry.RelatedExecutables {
						nativeui.Text(c, path).FontSize(12).TextColor(t.TextMuted).Wrap()
					}
				}
				if detail := firstNonEmpty(runtime.Error, runtime.Detail); detail != "" {
					color := t.TextMuted
					if runtime.Error != "" {
						color = t.Danger
					}
					nativeui.Box(c).Padding(9).Radius(6).Background(t.Background).Children(func() {
						nativeui.Text(c, detail).FontSize(12).TextColor(color).Wrap()
					})
				}
			})
		})
		nativeui.Row(c).Height(36).Gap(8).Children(func() {
			if nativeui.PrimaryButton(c, "保存").Height(36).Label("保存程序设置").Disabled(m.busy["save:"+entry.ID]).Clicked() {
				m.saveApp(entry, draft)
			}
			if nativeui.Button(c, "移除").Height(36).Label("移除程序").TextColor(t.Danger).Disabled(m.busy["remove:"+entry.ID]).Clicked() {
				m.removeApp(entry.ID)
				m.selectedAppID = ""
			}
		})
	})
}

func (m *model) appDraft(entry domain.AppEntry) *appDraft {
	if draft := m.drafts[entry.ID]; draft != nil {
		return draft
	}
	draft := &appDraft{name: entry.Name, proxyID: entry.ProxyID, related: strings.Join(entry.RelatedExecutables, ", ")}
	if draft.name == "" {
		draft.name = entry.Identity.Name
	}
	m.drafts[entry.ID] = draft
	return draft
}

func (m *model) proxySelect(c *nativeui.Context, draft *appDraft) {
	m.proxySelectWidth(c, draft, 0)
}

func (m *model) proxySelectWidth(c *nativeui.Context, draft *appDraft, width float32) {
	options := []string{"跟随默认代理"}
	profileIDs := make(map[string]string, len(m.snapshot.Settings.Profiles))
	for _, profile := range m.snapshot.Settings.Profiles {
		label := m.profileSelectLabel(profile)
		options = append(options, label)
		profileIDs[label] = profile.ID
	}
	selectedValue := "跟随默认代理"
	if draft.proxyID != "" {
		if profile, ok := m.profileFor(draft.proxyID); ok {
			selectedValue = m.profileSelectLabel(profile)
		} else {
			selectedValue = "已删除的代理"
			profileIDs[selectedValue] = draft.proxyID
			options = append(options, selectedValue)
		}
	}
	selected := nativeui.Select(c.Key("proxy"), &selectedValue, options).Height(36).Label("程序代理")
	if width > 0 {
		selected.Width(width)
	} else {
		selected.FillWidth()
	}
	if selected.Changed() {
		draft.proxyID = profileIDs[selectedValue]
		for _, entry := range m.snapshot.Settings.Apps {
			if m.appDraft(entry) == draft {
				m.saveApp(entry, draft)
				break
			}
		}
	}
}

func (m *model) profileSelectLabel(profile domain.ProxyProfile) string {
	label := profileOptionLabel(profile)
	for _, other := range m.snapshot.Settings.Profiles {
		if other.ID != profile.ID && profileOptionLabel(other) == label {
			return label + " · " + profile.Address()
		}
	}
	return label
}

func (m *model) appActions(c *nativeui.Context, entry domain.AppEntry, runtime domain.AppRuntime, width float32) {
	busy := m.appBusy(entry.ID)
	nativeui.Row(c).Width(width).Gap(6).Justify(nativeui.End).AlignItems(nativeui.Center).Children(func() {
		switch runtime.State {
		case domain.StateRunning, domain.StateStarting:
			if nativeui.Button(c, "结束").Label("结束 " + entry.Name).Disabled(busy).Clicked() {
				m.stopApp(entry.ID)
			}
			if nativeui.Button(c, "重启").Label("重启 " + entry.Name).Disabled(busy).Clicked() {
				m.restartApp(entry.ID)
			}
		default:
			if nativeui.PrimaryButton(c, "启动").Label("启动 " + entry.Name).Disabled(busy).Clicked() {
				m.startApp(entry.ID)
			}
		}
	})
}

type tableMetrics struct {
	iconWidth    float32
	nameWidth    float32
	proxyWidth   float32
	modeWidth    float32
	statusWidth  float32
	actionsWidth float32
	gap          float32
	showMode     bool
}

func (m *model) tableMetrics(c *nativeui.Context) tableMetrics {
	width, _ := c.Size()
	metrics := tableMetrics{iconWidth: 40, nameWidth: 140, proxyWidth: 166, modeWidth: 88, statusWidth: 116, actionsWidth: 142, gap: 10, showMode: width >= 920}
	if width < 920 {
		metrics.iconWidth = 32
		metrics.nameWidth = 112
		metrics.proxyWidth = 144
		metrics.statusWidth = 92
		metrics.actionsWidth = 124
		metrics.gap = 6
	}
	if m.selectedAppID != "" && width >= 1040 {
		// The inspector takes a fixed 300 DIP column. Keep the table's
		// optional mode column out of the reduced budget and let the name
		// column absorb the remaining width.
		metrics.iconWidth = 32
		metrics.nameWidth = 100
		metrics.proxyWidth = 126
		metrics.statusWidth = 84
		metrics.actionsWidth = 124
		metrics.gap = 6
		metrics.showMode = false
	}
	return metrics
}

func (m *model) appBusy(id string) bool {
	return m.busy["start:"+id] || m.busy["stop:"+id] || m.busy["restart:"+id] || m.busy["save:"+id] || m.busy["remove:"+id]
}

func (m *model) appByID(id string) (domain.AppEntry, bool) {
	for _, entry := range m.snapshot.Settings.Apps {
		if entry.ID == id {
			return entry, true
		}
	}
	return domain.AppEntry{}, false
}

func (m *model) filteredApps() []domain.AppEntry {
	query := strings.ToLower(strings.TrimSpace(m.appSearch))
	out := make([]domain.AppEntry, 0, len(m.snapshot.Settings.Apps))
	for _, entry := range m.snapshot.Settings.Apps {
		runtime := m.snapshot.Runtimes[entry.ID]
		if m.appFilter == "running" && runtime.State != domain.StateRunning && runtime.State != domain.StateStarting {
			continue
		}
		if m.appFilter == "stopped" && (runtime.State == domain.StateRunning || runtime.State == domain.StateStarting) {
			continue
		}
		if query != "" {
			name := strings.ToLower(firstNonEmpty(entry.Name, entry.Identity.Name))
			path := strings.ToLower(entry.Identity.Executable)
			if !strings.Contains(name, query) && !strings.Contains(path, query) {
				continue
			}
		}
		out = append(out, entry)
	}
	return out
}

func (m *model) runningAppCount() int {
	count := 0
	for _, entry := range m.snapshot.Settings.Apps {
		runtime := m.snapshot.Runtimes[entry.ID]
		if runtime.State == domain.StateRunning || runtime.State == domain.StateStarting {
			count++
		}
	}
	return count
}
