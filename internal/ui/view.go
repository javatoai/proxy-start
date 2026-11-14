package ui

import (
	"github.com/egoist/mygo"
	nativeui "github.com/egoist/mygo/ui"
	"math"
	"time"
)

func (m *model) view(c *nativeui.Context) {
	m.applyTheme(c)
	if c.Shortcut(nativeui.Cmd, nativeui.KeyQ) {
		mygo.App.Quit()
		return
	}
	t := c.Theme()
	_, height := c.Size()
	diameter := float32(160)
	if height < 360 {
		diameter = 144
	}
	nativeui.Column(c).Fill().Background(t.Background).Font(m.uiFontFamily()).Children(func() {
		nativeui.Row(c).FillWidth().Height(44).Padding(6, 16).Justify(nativeui.End).AlignItems(nativeui.Center).Children(func() {
			gear := nativeui.ButtonBase(c.Key("settings")).Size(32, 32).Radius(8).Label("打开设置").Tooltip("设置").Disabled(m.busy)
			if gear.Hovered() {
				gear.Background(t.SurfaceHover)
			}
			gear.Children(func() { nativeui.Icon(c, settingsIcon).Size(20, 20).TextColor(t.TextMuted).Center() })
			if gear.Clicked() {
				m.openSettings()
			}
		})
		nativeui.Column(c).FillWidth().Grow(1).MinHeight(0).Padding(4, 20, 20, 20).Gap(20).AlignItems(nativeui.Center).Justify(nativeui.Center).Children(func() {
			m.codexCircle(c, diameter)
			nativeui.Row(c).Gap(12).Children(func() {
				label, action := "启动", "start"
				if m.snapshot.State == "running" {
					label, action = "重启", "restart"
				}
				if m.pendingAction == "start" {
					label = "启动中"
				} else if m.pendingAction == "restart" {
					label = "重启中"
				}
				if nativeui.PrimaryButton(c, label).Label(actionLabel(action)).Width(116).Height(38).Disabled(!m.canLaunch()).Clicked() {
					m.runLifecycle(action)
				}
				stopLabel := "停止"
				if m.pendingAction == "stop" {
					stopLabel = "停止中"
				}
				if nativeui.Button(c, stopLabel).Label("停止 Codex").Width(116).Height(38).Disabled(!m.canStop()).Clicked() {
					m.runLifecycle("stop")
				}
			})
			if m.snapshot.ScanError != "" && !m.snapshot.ScanNotFound {
				nativeui.Text(c, m.snapshot.ScanError).FontSize(12).TextColor(t.Danger).Wrap().MaxWidth(360)
			}
			if m.snapshot.RuntimeError != "" {
				nativeui.Text(c, m.snapshot.RuntimeError).FontSize(12).TextColor(t.Danger).Wrap().MaxWidth(360)
			}
			if m.failed && m.message != "" {
				nativeui.Row(c).FillWidth().MaxWidth(360).Gap(8).AlignItems(nativeui.Center).Children(func() {
					nativeui.Text(c, compactError(m.message)).Grow(1).MinWidth(0).FontSize(12).TextColor(t.Danger).Wrap()
					if nativeui.Button(c, "详情").Label("查看错误详情").Height(28).Clicked() {
						m.errorOpen = true
					}
				})
			}
		})
	})
	m.settingsDialog(c)
	m.errorDialog(c)
}

func actionLabel(action string) string {
	if action == "restart" {
		return "重启 Codex"
	}
	return "启动 Codex"
}

func (m *model) canLaunch() bool {
	return !m.busy && !m.snapshot.Scanning && (m.snapshot.State == "stopped" || m.snapshot.State == "running") && m.snapshot.ScanError == "" && m.snapshot.Identity.Executable != ""
}
func (m *model) canStop() bool {
	return !m.busy && !m.snapshot.Scanning && m.snapshot.State == "running"
}

func (m *model) codexCircle(c *nativeui.Context, diameter float32) {
	t := c.Theme()
	background, foreground, border := t.Surface, t.TextMuted, t.Border
	if m.snapshot.State == "running" {
		background, foreground, border = nativeui.Hex("#EDF7F0"), t.Success, nativeui.Hex("#C3E2CD")
	} else if m.snapshot.State == "stopped" {
		background, foreground, border = nativeui.Hex("#F0F5FF"), t.Accent, nativeui.Hex("#D7E3FB")
	}
	enabled := !m.busy && !m.snapshot.Scanning
	var circle nativeui.Element
	if enabled {
		circle = nativeui.ButtonBase(c.Key("codex-refresh"))
	} else {
		circle = nativeui.Box(c.Key("codex-refresh"))
	}
	circle.Size(diameter, diameter).Radius(diameter/2).Padding(0).Background(background).Border(1, border).Label("刷新 Codex 状态").Tooltip("刷新状态")
	if enabled && circle.Hovered() {
		circle.Background(nativeui.Hex("#E8EEF5"))
	}
	circle.Children(func() {
		nativeui.Column(c).Fill().Gap(8).AlignItems(nativeui.Center).Justify(nativeui.Center).Children(func() {
			nativeui.Text(c, "Codex").FontSize(17).TextColor(foreground)
			nativeui.Text(c, m.codexStatus()).FontSize(24).FontWeight(600).TextColor(foreground)
		})
	})
	active := m.pendingAction != "" || m.snapshot.Scanning
	target := float32(0)
	if active {
		target = 1
	}
	opacity := circle.Animate("activity", target, 180*time.Millisecond)
	if active && m.animationStart.IsZero() {
		m.animationStart = c.Now()
	}
	if opacity > 0.001 {
		epoch := m.animationStart
		reduced := c.Preferences().ReduceMotion
		color := t.Accent
		color.A = uint8(255 * opacity)
		circle.DrawOver(func(p *nativeui.Painter, r nativeui.Rect) {
			phase := p.Now().Sub(epoch).Seconds()
			if reduced {
				phase = 0
			} else {
				p.AnimationFrame()
			}
			drawRefreshRing(p, r, phase, color)
		})
	} else if !active {
		m.animationStart = time.Time{}
	}
	if enabled && circle.Clicked() {
		m.refreshCodex()
	}
}

func drawRefreshRing(p *nativeui.Painter, r nativeui.Rect, phase float64, color nativeui.Color) {
	cx, cy, radius := r.X+r.W/2, r.Y+r.H/2, r.W/2-3
	start := phase*2*math.Pi/1.05 - math.Pi/2
	span := math.Pi * (0.85 + 0.35*math.Sin(phase*2*math.Pi/1.6))
	var arc nativeui.Path
	for index := 0; index <= 56; index++ {
		angle := start + span*float64(index)/56
		x, y := cx+radius*float32(math.Cos(angle)), cy+radius*float32(math.Sin(angle))
		if index == 0 {
			arc.MoveTo(x, y)
		} else {
			arc.LineTo(x, y)
		}
	}
	p.StrokePath(&arc, 3, color)
}

func (m *model) codexStatus() string {
	switch m.pendingAction {
	case "refresh":
		return "刷新中"
	case "start":
		return "启动中"
	case "restart":
		return "重启中"
	case "stop":
		return "停止中"
	}
	if m.snapshot.Scanning {
		return "扫描中"
	}
	if m.snapshot.Identity.Executable == "" && m.snapshot.ScanNotFound {
		return "未找到"
	}
	switch m.snapshot.State {
	case "running":
		return "已启动"
	case "stopped":
		return "未启动"
	case "starting":
		return "启动中"
	case "stopping":
		return "停止中"
	default:
		return "状态待确认"
	}
}
