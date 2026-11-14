package ui

import (
	"strings"

	nativeui "github.com/egoist/mygo/ui"
)

func compactError(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	runes := []rune(message)
	if len(runes) > 40 {
		return string(runes[:40]) + "…"
	}
	return message
}

func (m *model) errorDialog(c *nativeui.Context) {
	width, height := c.Size()
	t := c.Theme()
	nativeui.Modal(c, &m.errorOpen, func() {
		nativeui.Column(c).Width(min(float32(480), width-48)).Height(min(float32(300), height-64)).Padding(20).Gap(16).Children(func() {
			nativeui.Row(c).FillWidth().Gap(12).AlignItems(nativeui.Center).Children(func() {
				nativeui.Text(c, "错误详情").FontSize(18).FontWeight(600).Grow(1)
				if nativeui.Button(c, "关闭").Label("关闭错误详情").Height(32).Clicked() {
					m.errorOpen = false
				}
			})
			nativeui.Scroll(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
				nativeui.Text(c, m.message).FontSize(12).TextColor(t.Text).Wrap()
			})
		})
	}).Padding(0).Radius(12)
}
