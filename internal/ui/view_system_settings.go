package ui

import nativeui "github.com/egoist/mygo/ui"

const systemSettingsMaxWidth float32 = 760

func (m *model) systemSettingsPage(c *nativeui.Context) {
	t := c.Theme()
	nativeui.Column(c).Fill().Background(t.Background).Children(func() {
		nativeui.Row(c).Height(88).Padding(20, 24, 16, 24).AlignItems(nativeui.Center).Children(func() {
			nativeui.Column(c).Gap(4).Children(func() {
				nativeui.Text(c, "系统设置").FontSize(24).FontWeight(600)
				nativeui.Text(c, "主题、应用列表与平台组件").FontSize(12).TextColor(t.TextMuted)
			})
		})
		nativeui.Divider(c)
		nativeui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			nativeui.Column(c).FillWidth().MaxWidth(systemSettingsMaxWidth).Padding(24, 24, 40, 24).Gap(26).Children(func() {
				m.appearanceSettings(c)
				nativeui.Divider(c)
				m.applicationListSettings(c)
				nativeui.Divider(c)
				m.backendSection(c)
			})
		})
	})
}

func (m *model) appearanceSettings(c *nativeui.Context) {
	t := c.Theme()
	nativeui.Column(c).FillWidth().Gap(10).Children(func() {
		nativeui.Text(c, "外观").FontSize(16).FontWeight(600)
		nativeui.Row(c).FillWidth().AlignItems(nativeui.Center).Gap(16).Children(func() {
			nativeui.Column(c).Grow(1).Gap(3).Children(func() {
				nativeui.Text(c, "主题").FontSize(14).FontWeight(600)
				nativeui.Text(c, "选择窗口的颜色主题，修改会立即生效。").FontSize(12).TextColor(t.TextMuted).Wrap()
			})
			theme := themeOptionLabel(m.snapshot.Settings.Theme)
			if nativeui.Select(c.Key("system-theme"), &theme, []string{"跟随系统", "浅色", "深色"}).Width(144).Height(36).Label("主题").Changed() {
				m.setTheme(themeOptionValue(theme))
			}
		})
	})
}

func (m *model) applicationListSettings(c *nativeui.Context) {
	t := c.Theme()
	platform := m.platformCapabilities()
	description := platform.SystemAppsDescription
	if description == "" {
		description = "隐藏微软软件及 Windows 系统组件"
	}
	nativeui.Column(c).FillWidth().Gap(10).Children(func() {
		nativeui.Text(c, "应用列表").FontSize(16).FontWeight(600)
		nativeui.Row(c).FillWidth().AlignItems(nativeui.Center).Gap(16).Children(func() {
			nativeui.Column(c).Grow(1).Gap(3).Children(func() {
				nativeui.Text(c, "隐藏系统软件").FontSize(14).FontWeight(600)
				nativeui.Text(c, description).FontSize(12).TextColor(t.TextMuted).Wrap()
			})
			hidden := !m.snapshot.Settings.ShowSystemApps
			if nativeui.Switch(c.Key("hide-system-apps"), &hidden).Label("隐藏系统软件").Disabled(m.busy["hide-system-apps"]).Changed() {
				m.setHideSystemApps(hidden)
			}
		})
	})
}
