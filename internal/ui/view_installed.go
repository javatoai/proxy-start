package ui

import (
	"fmt"
	"strings"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
)

func (m *model) installedDialog(c *nativeui.Context) {
	t := c.Theme()
	p := &m.installed
	width, height := c.Size()
	dialogWidth := min(float32(680), width-48)
	dialogHeight := min(float32(580), height-48)
	open := p.open
	nativeui.Modal(c, &open, func() {
		if !open {
			m.closeInstalled()
			return
		}
		nativeui.Column(c).Width(dialogWidth).Height(dialogHeight).MinHeight(0).Padding(24).Gap(14).Radius(12).Background(t.Background).Border(1, t.Border).Children(func() {
			nativeui.Row(c).AlignItems(nativeui.Center).Gap(12).Children(func() {
				nativeui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					nativeui.Text(c, "添加已安装应用").FontSize(20).FontWeight(600)
				nativeui.Text(c, "选择后加入应用列表，点击启动时应用代理").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				if nativeui.Button(c, "关闭").Height(36).Label("关闭应用列表").Clicked() {
					m.closeInstalled()
				}
			})
			m.installedSearch(c)
			filtered := filteredInstalledApps(p.items, p.query)
			count := fmt.Sprintf("共 %d 个应用", len(p.items))
			if strings.TrimSpace(p.query) != "" {
				count = fmt.Sprintf("找到 %d 个 · 共 %d 个应用", len(filtered), len(p.items))
			}
			if p.loading {
				count = "正在读取已安装应用…"
			} else if !m.snapshot.Settings.ShowSystemApps {
				count += " · 系统软件已隐藏"
			}
			if p.err == "" {
				nativeui.Text(c, count).FontSize(12).TextColor(t.TextMuted)
			}
			switch {
			case p.loading:
				nativeui.Column(c).FillWidth().Grow(1).Gap(12).Children(func() {
					nativeui.Progress(c, -1).FillWidth()
				})
			case p.err != "":
				nativeui.Column(c).FillWidth().Grow(1).Gap(10).Children(func() {
					nativeui.Text(c, "无法读取应用列表").FontSize(14).FontWeight(600)
					nativeui.Text(c, p.err).FontSize(12).TextColor(t.TextMuted).Wrap()
					if nativeui.Button(c, "重试").Width(76).Height(36).Label("重试读取已安装应用").Clicked() {
						m.loadInstalled()
					}
				})
			case len(filtered) == 0:
				m.installedEmptyState(c, len(p.items) > 0)
			default:
				p.list.Key = func(i int) any { return installedIdentityKey(filtered[i]) }
				p.list.Label = func(i int) string { return filtered[i].Name }
				nativeui.List(c.Key("installed-list"), &p.list, len(filtered)).FillWidth().Grow(1).MinHeight(0).Rows(func(row nativeui.ListRow) {
					m.installedRow(row.Context, filtered[row.Index])
				})
			}
		})
	})
}

func (m *model) installedSearch(c *nativeui.Context) {
	t := c.Theme()
	field := nativeui.Box(c).FillWidth().Height(36).Padding(0, 11).Radius(8).Border(1, t.Border).Background(t.Surface)
	var input nativeui.Element
	field.Children(func() {
		nativeui.Row(c).Fill().Gap(8).AlignItems(nativeui.Center).Children(func() {
			nativeui.Icon(c, searchIcon).Size(16, 16).TextColor(t.TextMuted)
			query := m.installed.query
			input = nativeui.TextInputBase(c.Key("installed-search"), &query).Grow(1).MinWidth(0).FillHeight().Placeholder("搜索名称、路径或包名").Label("搜索已安装应用").AutoFocus()
			if input.Changed() {
				m.setInstalledSearch(query)
			}
			if m.installed.query != "" {
				clear := nativeui.ButtonBase(c.Key("clear-installed-search")).Size(24, 24).Radius(6).Label("清空已安装应用搜索").Tooltip("清空搜索")
				if clear.Hovered() {
					clear.Background(t.SurfaceHover)
				}
				clear.Children(func() { nativeui.Icon(c, clearIcon).Size(12, 12).TextColor(t.TextMuted).Center() })
				if clear.Clicked() {
					m.setInstalledSearch("")
				}
			}
		})
	})
	if input.Focused() {
		field.Border(1, t.Accent)
	}
}

func (m *model) installedEmptyState(c *nativeui.Context, searched bool) {
	t := c.Theme()
	title, hint := "没有发现可添加的应用", "可以关闭此窗口，手动添加程序或快捷方式"
	if searched {
		title, hint = "没有匹配的应用", "试试其他名称、路径或包名"
	}
	nativeui.Column(c).FillWidth().Grow(1).Gap(8).Padding(24, 0).Children(func() {
		nativeui.Text(c, title).FontSize(14).FontWeight(600)
		nativeui.Text(c, hint).FontSize(12).TextColor(t.TextMuted).Wrap()
		if searched {
			if nativeui.Button(c, "清空搜索").Width(104).Height(36).Label("清空无结果搜索").Clicked() {
				m.setInstalledSearch("")
			}
		}
	})
}

func (m *model) installedRow(c *nativeui.Context, identity domain.AppIdentity) {
	t := c.Theme()
	nativeui.Row(c).FillWidth().Height(60).Padding(10, 2).Gap(12).AlignItems(nativeui.Center).Children(func() {
		m.installedAppIcon(c, identity)
		nativeui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			nativeui.Text(c, identity.Name).FontSize(14).FontWeight(500).SingleLine().Tooltip(identity.Name)
			detail := installedIdentityDetail(identity)
			nativeui.Text(c, detail).FontSize(12).TextColor(t.TextMuted).SingleLine().Tooltip(detail)
		})
		added := m.installedAlreadyAdded(identity)
		adding := m.busy["add:"+installedInput(identity)]
		label := "添加"
		if added {
			label = "已添加"
		} else if adding {
			label = "添加中"
		}
		if nativeui.Button(c, label).Height(36).Width(80).Label(label + " " + identity.Name).Disabled(added || adding).Clicked() {
			m.addInstalled(identity)
		}
	})
}
