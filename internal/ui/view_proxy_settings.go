package ui

import (
	"fmt"
	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
)

const proxyTableMaxWidth float32 = 1060

func (m *model) proxySettingsPage(c *nativeui.Context) {
	t := c.Theme()
	nativeui.Column(c).Fill().Background(t.Background).Children(func() {
		nativeui.Row(c).Height(88).Padding(20, 24, 16, 24).AlignItems(nativeui.Center).Children(func() {
			nativeui.Column(c).Grow(1).Gap(4).Children(func() {
				nativeui.Text(c, "代理设置").FontSize(24).FontWeight(600)
				nativeui.Text(c, "管理代理配置，按需打开带代理环境的控制台").FontSize(12).TextColor(t.TextMuted)
			})
			if nativeui.PrimaryButton(c, "新增代理").Height(36).Label("新增代理").Clicked() {
				m.openNewProfileModal()
			}
		})
		nativeui.Divider(c)
		nativeui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			nativeui.Column(c).FillWidth().MaxWidth(proxyTableMaxWidth).Padding(24, 24, 36, 24).Gap(14).Children(func() {
				nativeui.Row(c).AlignItems(nativeui.End).Children(func() {
					nativeui.Column(c).Grow(1).Gap(3).Children(func() {
						nativeui.Text(c, "代理列表").FontSize(17).FontWeight(600)
						nativeui.Text(c, fmt.Sprintf("%d 个代理配置", len(m.snapshot.Settings.Profiles))).FontSize(12).TextColor(t.TextMuted)
					})
				})
				m.proxyTable(c)
			})
		})
	})
}

func (m *model) proxyTable(c *nativeui.Context) {
	t := c.Theme()
	if len(m.snapshot.Settings.Profiles) == 0 {
		nativeui.Box(c).FillWidth().Padding(28, 20).Border(1, t.Border).Radius(8).Background(t.Surface).Children(func() {
			nativeui.Text(c, "还没有代理配置").FontSize(14).FontWeight(600)
			nativeui.Text(c, "点击右上角“新增代理”添加本机或远程代理。").FontSize(12).TextColor(t.TextMuted).Wrap()
		})
		return
	}
	width, _ := c.Size()
	if width < 920 {
		m.compactProxyTable(c)
		return
	}
	nativeui.Column(c).FillWidth().Border(1, t.Border).Radius(8).Children(func() {
		nativeui.Row(c).FillWidth().Height(38).Padding(0, 14).AlignItems(nativeui.Center).Background(t.Surface).Children(func() {
			nativeui.Text(c, "名称").Width(232).Padding(0, 12, 0, 0).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
			nativeui.Text(c, "地址").Grow(1).MinWidth(150).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
			nativeui.Text(c, "协议").Width(100).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
			nativeui.Text(c, "操作").Width(270).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
		})
		for index, profile := range m.snapshot.Settings.Profiles {
			profile := profile
			row := nativeui.Row(c).FillWidth().MinHeight(68).Padding(10, 14).AlignItems(nativeui.Center)
			if index%2 == 1 {
				row.Background(t.Surface.Alpha(0.35))
			}
			row.Children(func() {
				nativeui.Row(c).Width(232).MinWidth(120).Padding(0, 12, 0, 0).Gap(7).AlignItems(nativeui.Center).Children(func() {
					nativeui.Text(c, profile.Name).FontSize(14).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Tooltip(profile.Name)
					if profile.ID == m.snapshot.Settings.DefaultProxyID {
						nativeui.Badge(c, "默认")
					}
				})
				nativeui.Column(c).Grow(1).MinWidth(150).Gap(3).Children(func() {
					nativeui.Text(c, profile.Address()).FontSize(13).TextColor(t.TextMuted).SingleLine().Tooltip(profile.Address())
					nativeui.Text(c, bypassProfileSummary(profile)).FontSize(12).TextColor(t.TextMuted).SingleLine().Tooltip(bypassProfileSummary(profile))
				})
				nativeui.Text(c, protocolLabel(profile.Protocol)).Width(100).FontSize(13).TextColor(t.TextMuted).SingleLine()
				nativeui.Row(c).Width(270).Gap(6).AlignItems(nativeui.Center).Children(func() {
					console := nativeui.PrimaryButton(c, "打开控制台").Height(34).Padding(0, 11).Label("打开代理控制台").Disabled(m.busy["profile-console:"+profile.ID])
					if console.Clicked() {
						m.openProxyConsole(profile.ID)
					}
					if nativeui.Button(c, "编辑").Height(34).Padding(0, 11).Label("编辑代理 " + profile.Name).Disabled(m.busy["profile-save"]).Clicked() {
						m.openEditProfileModal(profile)
					}
					if nativeui.Button(c, "删除").Height(34).Padding(0, 11).Label("删除代理 " + profile.Name).TextColor(t.Danger).Disabled(m.busy["profile-delete"]).Clicked() {
						m.deleteProfile(profile.ID)
					}
				})
			})
		}
	})
}

// compactProxyTable keeps the three row actions together when the window is
// narrow. The information and actions use separate lines so no fixed table
// column can clip the content at the minimum window size.
func (m *model) compactProxyTable(c *nativeui.Context) {
	t := c.Theme()
	nativeui.Column(c).FillWidth().Border(1, t.Border).Radius(8).Children(func() {
		for index, profile := range m.snapshot.Settings.Profiles {
			profile := profile
			row := nativeui.Column(c).FillWidth().Padding(12, 14).Gap(10)
			if index%2 == 1 {
				row.Background(t.Surface.Alpha(0.35))
			}
			row.Children(func() {
				nativeui.Row(c).FillWidth().Gap(8).AlignItems(nativeui.Center).Children(func() {
					nativeui.Text(c, profile.Name).FontSize(14).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Tooltip(profile.Name)
					if profile.ID == m.snapshot.Settings.DefaultProxyID {
						nativeui.Badge(c, "默认")
					}
				})
				nativeui.Row(c).FillWidth().Gap(8).AlignItems(nativeui.Center).Children(func() {
					nativeui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						nativeui.Text(c, profile.Address()).FontSize(13).TextColor(t.TextMuted).SingleLine().Tooltip(profile.Address())
						nativeui.Text(c, bypassProfileSummary(profile)).FontSize(12).TextColor(t.TextMuted).SingleLine().Tooltip(bypassProfileSummary(profile))
					})
					nativeui.Text(c, protocolLabel(profile.Protocol)).FontSize(12).TextColor(t.TextMuted).SingleLine()
				})
				nativeui.Row(c).FillWidth().Gap(6).AlignItems(nativeui.Center).Children(func() {
					console := nativeui.PrimaryButton(c, "打开控制台").Width(140).Height(34).Padding(0, 10).Label("打开代理控制台").Disabled(m.busy["profile-console:"+profile.ID])
					if console.Clicked() {
						m.openProxyConsole(profile.ID)
					}
					if nativeui.Button(c, "编辑").Width(76).Height(34).Padding(0, 10).Label("编辑代理 " + profile.Name).Disabled(m.busy["profile-save"]).Clicked() {
						m.openEditProfileModal(profile)
					}
					if nativeui.Button(c, "删除").Width(76).Height(34).Padding(0, 10).Label("删除代理 " + profile.Name).TextColor(t.Danger).Disabled(m.busy["profile-delete"]).Clicked() {
						m.deleteProfile(profile.ID)
					}
				})
			})
			if index < len(m.snapshot.Settings.Profiles)-1 {
				nativeui.Divider(c)
			}
		}
	})
}

func bypassProfileSummary(profile domain.ProxyProfile) string {
	return fmt.Sprintf("绕过 %d 个域名", len(profile.BypassDomains))
}
