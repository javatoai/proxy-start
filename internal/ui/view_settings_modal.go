package ui

import nativeui "github.com/egoist/mygo/ui"

var settingsIcon = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="m9.5 2-.6 2.6-1.8 1-2.5-.8-2.5 4.4L4 11v2l-1.9 1.8 2.5 4.4 2.5-.8 1.8 1 .6 2.6h5l.6-2.6 1.8-1 2.5.8 2.5-4.4L20 13v-2l1.9-1.8-2.5-4.4-2.5.8-1.8-1-.6-2.6Z"/><circle cx="12" cy="12" r="3"/></svg>`))

func (m *model) settingsDialog(c *nativeui.Context) {
	t := c.Theme()
	width, _ := c.Size()
	nativeui.Modal(c, &m.settingsOpen, func() {
		nativeui.Column(c).Width(min(float32(420), width-48)).Padding(24).Gap(16).Children(func() {
			nativeui.Text(c, "设置").FontSize(20).FontWeight(600)
			nativeui.Column(c).FillWidth().Gap(8).Children(func() {
				nativeui.Text(c, "代理端口").FontSize(13).FontWeight(600)
				nativeui.Row(c).FillWidth().Gap(10).AlignItems(nativeui.Center).Children(func() {
					nativeui.Text(c, "127.0.0.1 :").FontSize(14).TextColor(t.TextMuted)
					nativeui.TextInput(c.Key("settings-port"), &m.portDraft).Grow(1).MinWidth(0).Height(36).Label("代理端口").Placeholder("7897").Disabled(m.busy)
				})
				if m.settingsError != "" {
					nativeui.Text(c, m.settingsError).FontSize(12).TextColor(t.Danger).Wrap()
				}
			})
			if nativeui.Button(c, "打开代理控制台").FillWidth().Height(36).Disabled(m.busy).Clicked() {
				m.applySettings(m.desktop.OpenProxyConsole)
			}
			nativeui.Row(c).FillWidth().Justify(nativeui.End).Gap(8).Children(func() {
				if nativeui.Button(c, "取消").Height(36).Disabled(m.busy).Clicked() {
					m.settingsOpen = false
				}
				if nativeui.PrimaryButton(c, "保存").Label("保存设置").Height(36).Disabled(m.busy).Clicked() {
					m.saveSettings()
				}
			})
		})
	}).Padding(0).Radius(12)
}
