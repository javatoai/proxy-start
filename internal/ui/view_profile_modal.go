package ui

import (
	"fmt"

	nativeui "github.com/egoist/mygo/ui"
)

func (m *model) profileModal(c *nativeui.Context) {
	open := m.proxyModalOpen
	t := c.Theme()
	width, height := c.Size()
	modalWidth := min(float32(620), width-48)
	modalHeight := min(float32(700), height-48)
	nativeui.Modal(c, &open, func() {
		if !open {
			m.proxyModalOpen = false
			return
		}
		nativeui.Column(c).Width(modalWidth).MaxHeight(modalHeight).MinHeight(0).Padding(24).Gap(16).Radius(12).Background(t.Background).Border(1, t.Border).Children(func() {
			nativeui.Row(c).AlignItems(nativeui.Center).Gap(12).Children(func() {
				nativeui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					title := "编辑代理"
					if m.profileNew {
						title = "新增代理"
					}
					nativeui.Text(c, title).FontSize(20).FontWeight(600)
					nativeui.Text(c, "保存后可从列表打开独立代理控制台").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				if nativeui.Button(c, "关闭").Height(36).Label("关闭代理编辑").Clicked() {
					m.proxyModalOpen = false
				}
			})
			nativeui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				nativeui.Column(c).FillWidth().Padding(4, 0, 4, 0).Children(func() { m.profileEditorFields(c) })
			})
			nativeui.Divider(c)
			nativeui.Box(c).FillWidth().Padding(8, 0, 0, 0).Children(func() { m.profileEditorActions(c) })
		})
	})
}

func (m *model) deleteProfileModal(c *nativeui.Context) {
	open := m.deleteModalOpen
	t := c.Theme()
	windowWidth, _ := c.Size()
	profile, _ := m.profileFor(m.deleteProfileID)
	name := profile.Name
	if name == "" {
		name = "此代理"
	}
	nativeui.Modal(c, &open, func() {
		if !open {
			m.deleteModalOpen = false
			return
		}
		nativeui.Column(c).Width(min(float32(480), windowWidth-48)).Padding(24).Gap(16).Radius(12).Background(t.Background).Border(1, t.Border).Children(func() {
			nativeui.Text(c, "删除代理").FontSize(20).FontWeight(600)
			nativeui.Text(c, fmt.Sprintf("确定删除“%s”吗？删除后无法恢复；仍在使用此代理时需先更换配置。", name)).FontSize(13).TextColor(t.TextMuted).Wrap()
			nativeui.Row(c).Justify(nativeui.End).Gap(8).Children(func() {
				if nativeui.Button(c, "取消").Height(36).Label("取消删除代理").Clicked() {
					m.deleteModalOpen = false
					m.proxyModalOpen = m.deleteReturnToEditor
					m.deleteReturnToEditor = false
				}
				confirm := nativeui.Button(c, "删除").Height(36).Label("确认删除代理").TextColor(t.Danger).Disabled(m.busy["profile-delete"])
				if confirm.Clicked() {
					m.confirmDeleteProfile()
				}
			})
		})
	})
}
