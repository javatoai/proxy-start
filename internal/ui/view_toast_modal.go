package ui

import nativeui "github.com/egoist/mygo/ui"

func (m *model) toastDetailModal(c *nativeui.Context) {
	open := m.toastDetailOpen
	t := c.Theme()
	width, height := c.Size()
	modalWidth := min(float32(620), width-48)
	modalHeight := min(float32(520), height-48)
	nativeui.Modal(c, &open, func() {
		if !open {
			m.toastDetailOpen = false
			return
		}
		nativeui.Column(c).Width(modalWidth).MaxHeight(modalHeight).MinHeight(0).Padding(24).Gap(16).Radius(12).Background(t.Background).Border(1, t.Border).Children(func() {
			nativeui.Row(c).AlignItems(nativeui.Center).Gap(12).Children(func() {
				nativeui.Text(c, "操作详情").FontSize(20).FontWeight(600).Grow(1)
				if nativeui.Button(c, "关闭").Height(36).Label("关闭操作详情").Clicked() {
					m.toastDetailOpen = false
				}
			})
			nativeui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				nativeui.Box(c).FillWidth().Padding(12, 14).Radius(8).Border(1, t.Border).Background(t.Surface).Children(func() {
					nativeui.Text(c, m.toastDetail).FontSize(13).TextColor(t.Text).Wrap()
				})
			})
		})
	})
}
