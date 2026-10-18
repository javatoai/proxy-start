package ui

import (
	"strings"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
)

func (m *model) profileEditorFields(c *nativeui.Context) {
	t := c.Theme()
	if !m.profileNew && m.profileID == "" && len(m.snapshot.Settings.Profiles) > 0 {
		m.profileID = m.snapshot.Settings.Profiles[0].ID
	}
	if !m.profileNew && m.profileID != "" && m.profileDraft.id != m.profileID {
		for _, profile := range m.snapshot.Settings.Profiles {
			if profile.ID == m.profileID {
				m.loadProfileDraft(profile)
				break
			}
		}
	}
	if !m.profileNew && m.profileDraft.name == "" {
		nativeui.Column(c).Gap(6).Children(func() {
			nativeui.Text(c, "选择一个代理").FontSize(18).FontWeight(600)
			nativeui.Text(c, "编辑代理地址、协议和凭据").FontSize(13).TextColor(t.TextMuted)
		})
		return
	}
	nativeui.Column(c).Gap(16).Children(func() {
		nativeui.Text(c, "基本信息").FontSize(13).FontWeight(600).TextColor(t.TextMuted)
		nativeui.Column(c).Gap(12).Children(func() {
			nativeui.Column(c).Gap(6).Children(func() {
				nativeui.Text(c, "名称").FontSize(13).FontWeight(600)
				nativeui.TextInput(c.Key("profile-name"), &m.profileDraft.name).FillWidth().Height(36).Label("名称").Placeholder("例如：本机 Clash")
			})
			nativeui.Column(c).Gap(6).Children(func() {
				nativeui.Text(c, "协议").FontSize(13).FontWeight(600)
				protocol := protocolOptionLabel(m.profileDraft.protocol)
				if nativeui.Select(c.Key("profile-protocol"), &protocol, []string{"自动检测", "HTTP", "SOCKS5"}).FillWidth().Height(36).Label("协议").Changed() {
					m.profileDraft.protocol = protocolOptionValue(protocol)
				}
			})
			nativeui.Row(c).Gap(8).Wrap().Children(func() {
				nativeui.Column(c).Grow(1).MinWidth(140).Gap(6).Children(func() {
					nativeui.Text(c, "地址").FontSize(13).FontWeight(600)
					nativeui.TextInput(c.Key("profile-host"), &m.profileDraft.host).FillWidth().Height(36).Label("地址").Placeholder("127.0.0.1 或 proxy.example.com")
				})
				nativeui.Column(c).Width(100).Gap(6).Children(func() {
					nativeui.Text(c, "端口").FontSize(13).FontWeight(600)
					nativeui.TextInput(c.Key("profile-port"), &m.profileDraft.port).FillWidth().Height(36).Label("端口").Placeholder("7897")
				})
			})
		})
		authKey := "profile-auth-" + m.profileID
		authExpanded := m.profileAuthExpanded()
		authLabel := "展开认证（可选）"
		if authExpanded {
			authLabel = "收起认证（可选）"
		}
		authToggle := nativeui.ButtonBase(c.Key(authKey)).FillWidth().Height(32).Padding(0, 4).Radius(6).Label(authLabel)
		if authToggle.Hovered() {
			authToggle.Background(t.SurfaceHover)
		}
		authToggle.Children(func() {
			nativeui.Row(c).FillWidth().Gap(8).AlignItems(nativeui.Center).Children(func() {
				nativeui.Text(c, "认证（可选）").FontSize(13).FontWeight(600).TextColor(t.TextMuted)
				nativeui.Spacer(c).Grow(1)
				action := "展开"
				if authExpanded {
					action = "收起"
				}
				nativeui.Text(c, action).FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
		})
		if authToggle.Clicked() {
			m.details[authKey] = !authExpanded
			authExpanded = !authExpanded
		}
		if authExpanded {
			nativeui.Column(c).Gap(12).Margin(4, 0, 0, 0).Children(func() {
				nativeui.Column(c).Gap(6).Children(func() {
					nativeui.Text(c, "用户名").FontSize(13).FontWeight(600)
					nativeui.TextInput(c.Key("profile-user"), &m.profileDraft.username).FillWidth().Height(36).Label("用户名").Placeholder("可选")
				})
				nativeui.Column(c).Gap(6).Children(func() {
					nativeui.Text(c, "密码").FontSize(13).FontWeight(600)
					nativeui.TextInput(c.Key("profile-pass"), &m.profileDraft.password).FillWidth().Height(36).Label("密码").Password().Placeholder("留空则保留已保存密码")
					nativeui.Text(c, "密码保存在系统凭据存储中，不写入配置文件").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
			})
		}
	})
	if !m.profileNew {
		probeKey := "probe:" + m.profileID
		if m.busy[probeKey] {
			m.probeCard(c, "正在测试代理连接…", "正在验证 TCP 连接", t.Accent, "", false)
		} else if probe, ok := m.probes[m.profileID]; ok {
			m.probeCard(c, probeCardStatus(probe), probeCardSummary(probe), probeResultColor(t, probe), probe.Detail, true)
		}
	}
}

// profileEditorActions stays outside the scrolling form so the primary save
// action remains reachable when optional authentication fields expand.
func (m *model) profileEditorActions(c *nativeui.Context) {
	t := c.Theme()
	nativeui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if nativeui.PrimaryButton(c, "保存代理").Height(36).Label("保存代理配置").Disabled(m.busy["profile-save"]).Clicked() {
			m.saveProfile()
		}
		if !m.profileNew {
			if nativeui.Button(c, "设为默认").Height(36).Label("设为默认代理").Disabled(m.profileID == m.snapshot.Settings.DefaultProxyID || m.busy["profile-default"]).Clicked() {
				m.setDefaultProxy(m.profileID)
			}
			if nativeui.Button(c, "测试连接").Height(36).Label("测试代理连接").Disabled(m.busy["probe:"+m.profileID]).Clicked() {
				m.testProfile(m.profileID)
			}
			if profile, ok := m.profileFor(m.profileID); ok && profile.CredentialRef != "" {
				if nativeui.Button(c, "清除认证").Height(36).Label("清除已保存认证").Disabled(m.busy["profile-credential-clear"]).Clicked() {
					m.clearCredential(m.profileID)
				}
			}
			if nativeui.Button(c, "删除").Height(36).Label("删除代理").TextColor(t.Danger).Disabled(m.busy["profile-delete"]).Clicked() {
				m.deleteProfile(m.profileID)
			}
		}
	})
}

func (m *model) profileAuthExpanded() bool {
	key := "profile-auth-" + m.profileID
	if expanded, ok := m.details[key]; ok {
		return expanded
	}
	if strings.TrimSpace(m.profileDraft.username) != "" || strings.TrimSpace(m.profileDraft.password) != "" {
		return true
	}
	profile, ok := m.profileFor(m.profileID)
	return ok && profile.CredentialRef != ""
}

func (m *model) probeCard(c *nativeui.Context, status, summary string, color nativeui.Color, detail string, expandable bool) {
	probeDetailsKey := "probe-status-" + m.profileID
	t := c.Theme()
	nativeui.Box(c).Margin(12, 0, 0, 0).Padding(10, 12).Radius(8).Border(1, t.Border).Background(t.Surface).Children(func() {
		nativeui.Row(c).Gap(8).AlignItems(nativeui.Center).Children(func() {
			nativeui.Box(c).Size(8, 8).Radius(4).Background(color)
			nativeui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				nativeui.Text(c, status).FontSize(13).FontWeight(600).TextColor(t.Text).SingleLine()
				nativeui.Text(c, summary).FontSize(12).TextColor(t.TextMuted).Wrap()
			})
			if expandable && detail != "" {
				detailLabel := "查看详情"
				if m.details[probeDetailsKey] {
					detailLabel = "收起详情"
				}
				if nativeui.Button(c, detailLabel).Height(30).Label(detailLabel).Clicked() {
					m.details[probeDetailsKey] = !m.details[probeDetailsKey]
				}
			}
		})
		if expandable && m.details[probeDetailsKey] && detail != "" {
			nativeui.Box(c).Margin(8, 0, 0, 0).Padding(9, 10).Radius(6).Background(t.Background).Border(1, t.Border).Children(func() {
				nativeui.Text(c, detail).FontSize(12).TextColor(t.TextMuted).Wrap()
			})
		}
	})
}

func probeCardStatus(probe domain.ProbeResult) string {
	if probeFailed(probe) {
		return "连接未通过验证"
	}
	return probeStatusLabel(probe)
}

func probeCardSummary(probe domain.ProbeResult) string {
	if probeFailed(probe) {
		return "请检查地址、端口或认证信息"
	}
	return probeProtocolSummary(probe)
}

func probeResultColor(t *nativeui.Theme, probe domain.ProbeResult) nativeui.Color {
	if probeFailed(probe) {
		return t.Danger
	}
	return probeColor(t, probe)
}

func probeFailed(probe domain.ProbeResult) bool {
	return probe.CheckedAt.Unix() > 0 && strings.TrimSpace(probe.Detail) != "" &&
		!probe.HTTP && !probe.SOCKS5 && !probe.TCPVerified && !probe.HTTPVerified &&
		!probe.SOCKS5Verified && !probe.UDPVerified
}

func probeStatusLabel(probe domain.ProbeResult) string {
	if probe.TCPVerified || probe.HTTPVerified || probe.SOCKS5Verified {
		return "代理连接已验证"
	}
	if probe.UDPVerified {
		return "部分验证"
	}
	if probe.HTTP || probe.SOCKS5 {
		return "已识别代理协议"
	}
	return "连接未验证"
}

func probeProtocolSummary(probe domain.ProbeResult) string {
	verified := make([]string, 0, 4)
	if probe.TCPVerified {
		verified = append(verified, "TCP")
	}
	if probe.HTTPVerified {
		verified = append(verified, "HTTP")
	}
	if probe.SOCKS5Verified {
		verified = append(verified, "SOCKS5")
	}
	if probe.UDPVerified {
		verified = append(verified, "UDP")
	}
	if len(verified) > 0 {
		return "已验证 " + strings.Join(verified, " · ")
	}
	identified := make([]string, 0, 2)
	if probe.HTTP {
		identified = append(identified, "HTTP")
	}
	if probe.SOCKS5 {
		identified = append(identified, "SOCKS5")
	}
	if len(identified) > 0 {
		return "已识别 " + strings.Join(identified, " · ") + "，连接未验证"
	}
	return "等待连接验证"
}

func (m *model) backendSection(c *nativeui.Context) {
	t := c.Theme()
	backend := m.snapshot.Backend
	platform := m.platformCapabilities()
	statusLabel, statusSummary, statusColor := backendPresentationForPlatform(backend, platform, t)
	nativeui.Column(c).Gap(10).Padding(0, 0, 4, 0).Children(func() {
		nativeui.Row(c).AlignItems(nativeui.Center).Gap(12).Wrap().Children(func() {
			nativeui.Column(c).Grow(1).MinWidth(180).Gap(4).Children(func() {
				nativeui.Row(c).Gap(7).AlignItems(nativeui.Center).Wrap().Children(func() {
					nativeui.Box(c).Size(8, 8).Radius(4).Background(statusColor)
					nativeui.Text(c, "强制代理组件").FontSize(15).FontWeight(600)
					nativeui.Text(c, statusLabel).FontSize(12).FontWeight(600).TextColor(statusColor)
				})
				nativeui.Text(c, statusSummary).FontSize(12).TextColor(t.TextMuted).Wrap()
				nativeui.Text(c, nativeAdapterSummary(platform)).FontSize(12).TextColor(t.Accent).SingleLine()
			})
			if backend.Detail != "" && platform.ForceProxySupported {
				detailLabel := "查看诊断"
				if m.details["backend-status"] {
					detailLabel = "收起诊断"
				}
				if nativeui.Button(c, detailLabel).Height(36).Label(detailLabel).Clicked() {
					m.details["backend-status"] = !m.details["backend-status"]
				}
			}
			installLabel := "安装组件"
			installDisabled := m.busy["backend-install"] || !platform.ForceProxyInstallable
			if !platform.ForceProxySupported {
				installLabel = "当前不可用"
			} else if !platform.ForceProxyInstallable {
				installLabel = "暂不可安装"
			} else if backend.Installed {
				installLabel = "重新检查组件"
			}
			if nativeui.Button(c, installLabel).Height(36).Label(installLabel).Disabled(installDisabled).Clicked() {
				m.installBackend()
			}
		})
		if m.details["backend-status"] && backend.Detail != "" {
			nativeui.Box(c).Padding(10).Radius(6).Background(t.Surface).Border(1, t.Border).Children(func() {
				nativeui.Text(c, backend.Detail).FontSize(12).TextColor(t.TextMuted).Wrap()
			})
		}
	})
}

func backendPresentation(backend domain.BackendStatus, t *nativeui.Theme) (label, summary string, color nativeui.Color) {
	switch {
	case backend.Running && backend.Ready:
		return "运行中", "组件已启动，可为启用强制代理的应用提供服务", t.Success
	case backend.Running:
		return "已安装，未就绪", "组件进程正在运行，但就绪检查尚未通过", t.Warning
	case backend.Ready:
		return "已就绪", "组件已安装并通过就绪检查", t.Success
	case backend.Installed:
		return "已安装，未就绪", "组件已安装，但就绪检查尚未通过", t.Warning
	case backend.Detail != "" && !backendDetailLooksMissing(backend.Detail):
		return "检查失败", "无法确认组件状态，请查看诊断信息", t.Danger
	default:
		return "未安装", "尚未安装强制代理组件", t.TextMuted
	}
}

func backendDetailLooksMissing(detail string) bool {
	detail = strings.ToLower(strings.TrimSpace(detail))
	for _, marker := range []string{"未检测到", "不存在", "未安装", "not found", "does not exist"} {
		if strings.Contains(detail, marker) {
			return true
		}
	}
	return false
}
