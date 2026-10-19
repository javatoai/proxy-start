package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"proxy-start/internal/domain"
)

func (m *model) openProgramDialog() {
	go func() {
		paths, err := mygo.Dialog.Open(m.programDialogOptions())
		if err != nil {
			m.win.Update(func() { m.setStatus("选择程序失败："+err.Error(), noticeError) })
			return
		}
		// Dialog.Open returns on its worker, while addPath starts an operation
		// and updates model state. Marshal both the state mutation and the
		// operation kickoff back onto MyGO's UI thread.
		m.win.Update(func() {
			for _, path := range paths {
				m.addPath(path)
			}
		})
	}()
}

func (m *model) programDialogOptions() mygo.OpenDialogOptions {
	platform := m.platformCapabilities()
	extensions := normalizedAppExtensions(platform.AppExtensions)
	if len(extensions) == 0 {
		extensions = []string{"exe", "lnk"}
	}
	filterName := "程序和快捷方式"
	if len(extensions) == 1 && extensions[0] == "app" {
		filterName = "应用程序"
	}
	options := mygo.OpenDialogOptions{
		Parent:      m.win,
		Title:       "选择要代理启动的程序",
		ButtonLabel: "选择程序",
		Message:     platform.DragDescription,
		Multiple:    true,
		Filters: []mygo.FileFilter{
			{Name: filterName, Extensions: extensions},
			{Name: "所有文件", Extensions: []string{"*"}},
		},
	}
	if strings.EqualFold(strings.TrimSpace(platform.Name), "macOS") && containsString(extensions, "app") {
		// MyGO maps Directory to NSOpenPanel's canChooseDirectories. Keep it
		// false so .app is selected as a package file, while disabling
		// TreatPackagesAsDirectories prevents browsing into Contents/.
		options.Directory = false
		options.TreatPackagesAsDirectories = false
	}
	return options
}

func normalizedAppExtensions(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "."))
		if value == "" || value == "*" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func (m *model) addPath(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	m.run("add:"+path, "正在添加程序…", func(ctx context.Context) error {
		_, err := m.desktop.AddApp(ctx, path)
		return err
	})
}

func (m *model) addInstalled(identity domain.AppIdentity) {
	if m.installedAlreadyAdded(identity) {
		return
	}
	input := installedInput(identity)
	if input == "" {
		m.setStatus("该应用没有可解析的启动身份", noticeError)
		return
	}
	m.addPath(input)
}

func (m *model) openProxyConsole(id string) {
	m.run("profile-console:"+id, "正在打开代理控制台…", func(ctx context.Context) error {
		return m.desktop.OpenProxyConsole(ctx, id)
	})
}

func (m *model) removeApp(id string) {
	m.run("remove:"+id, "正在移除程序…", func(ctx context.Context) error { return m.desktop.RemoveApp(ctx, id) })
	delete(m.drafts, id)
	delete(m.details, id)
	delete(m.icons, id)
	delete(m.iconLoading, id)
	delete(m.iconAttempts, id)
}

func (m *model) saveApp(entry domain.AppEntry, draft *appDraft) {
	copy := entry
	copy.Name = strings.TrimSpace(draft.name)
	copy.ProxyID = strings.TrimSpace(draft.proxyID)
	copy.RelatedExecutables = splitCSV(draft.related)
	m.run("save:"+entry.ID, "正在保存程序设置…", func(ctx context.Context) error { return m.desktop.SaveApp(ctx, copy) })
}

func (m *model) startApp(id string) {
	m.run("start:"+id, "正在启动程序…", func(ctx context.Context) error { return m.desktop.Start(ctx, id) })
}

func (m *model) stopApp(id string) {
	m.run("stop:"+id, "正在结束程序…", func(ctx context.Context) error { return m.desktop.Stop(ctx, id) })
}

func (m *model) restartApp(id string) {
	m.run("restart:"+id, "正在重启程序…", func(ctx context.Context) error { return m.desktop.Restart(ctx, id) })
}

func (m *model) setTheme(theme string) {
	m.run("theme", "正在保存主题…", func(ctx context.Context) error { return m.desktop.SetTheme(ctx, theme) })
}

func (m *model) setHideSystemApps(hide bool) {
	m.run("hide-system-apps", "正在保存应用列表设置…", func(ctx context.Context) error {
		return m.desktop.SetHideSystemApps(ctx, hide)
	})
}

func (m *model) installBackend() {
	m.run("backend-install", "正在安装强制代理组件…", func(ctx context.Context) error { return m.desktop.InstallBackend(ctx) })
}

func (m *model) loadProfileDraft(profile domain.ProxyProfile) {
	m.profileDraft = profileDraft{
		id:            profile.ID,
		name:          profile.Name,
		protocol:      string(profile.Protocol),
		host:          profile.Host,
		port:          strconv.Itoa(profile.Port),
		username:      profile.Username,
		bypassDomains: strings.Join(profile.BypassDomains, "\n"),
	}
	m.profileNew = false
}

func (m *model) openNewProfileModal() {
	if m.busy["profile-save"] || m.busy["profile-delete"] {
		return
	}
	m.profileNew = true
	m.profileID = ""
	m.profileDraft = profileDraft{protocol: string(domain.ProtocolAuto), port: "7897"}
	m.profileEditToken++
	m.proxyModalOpen = true
	m.deleteModalOpen = false
}

func (m *model) openEditProfileModal(profile domain.ProxyProfile) {
	if m.busy["profile-save"] || m.busy["profile-delete"] {
		return
	}
	m.profileID = profile.ID
	m.loadProfileDraft(profile)
	m.profileEditToken++
	m.proxyModalOpen = true
	m.deleteModalOpen = false
}

func (m *model) saveProfile() {
	port, err := strconv.Atoi(strings.TrimSpace(m.profileDraft.port))
	if err != nil {
		m.setStatus("端口必须是数字", noticeError)
		return
	}
	bypass, err := domain.ParseBypassPolicy(m.profileDraft.bypassDomains)
	if err != nil {
		m.profileDraft.bypassError = err.Error()
		if m.win != nil {
			m.win.Invalidate()
		}
		return
	}
	m.profileDraft.bypassError = ""
	profile := domain.ProxyProfile{
		ID:            m.profileDraft.id,
		Name:          strings.TrimSpace(m.profileDraft.name),
		Protocol:      domain.Protocol(m.profileDraft.protocol),
		Host:          strings.TrimSpace(m.profileDraft.host),
		Port:          port,
		Username:      strings.TrimSpace(m.profileDraft.username),
		BypassDomains: bypass.Domains(),
	}
	password := m.profileDraft.password
	isNew := m.profileNew
	editToken := m.profileEditToken
	m.runAfter("profile-save", "正在保存代理配置…", func(ctx context.Context) error {
		saved, err := m.desktop.SaveProfileResult(ctx, profile, password)
		if err != nil {
			return err
		}
		profile = saved
		return nil
	}, func() {
		if editToken != m.profileEditToken {
			return
		}
		delete(m.probes, profile.ID)
		m.profileID = profile.ID
		m.profileNew = false
		m.loadProfileDraft(profile)
		m.proxyModalOpen = false
		if isNew {
			m.profileDraft.password = ""
		}
	})
}

func (m *model) setDefaultProxy(id string) {
	m.run("profile-default", "正在设置默认代理…", func(ctx context.Context) error { return m.desktop.SetDefaultProxy(ctx, id) })
}

func (m *model) deleteProfile(id string) {
	if id == "" || m.busy["profile-delete"] {
		return
	}
	m.deleteProfileID = id
	m.deleteSessionToken++
	m.deleteReturnToEditor = m.proxyModalOpen
	m.proxyModalOpen = false
	m.deleteModalOpen = true
}

func (m *model) confirmDeleteProfile() {
	id := m.deleteProfileID
	deleteToken := m.deleteSessionToken
	if id == "" || m.busy["profile-delete"] {
		return
	}
	m.runAfter("profile-delete", "正在删除代理…", func(ctx context.Context) error {
		return m.desktop.DeleteProfile(ctx, id)
	}, func() {
		if !m.deleteSessionCurrent(deleteToken, id) {
			return
		}
		m.deleteModalOpen = false
		m.deleteProfileID = ""
		m.deleteReturnToEditor = false
		m.profileID = ""
		m.profileDraft = profileDraft{}
		m.profileNew = false
	})
}

func (m *model) deleteSessionCurrent(token uint64, id string) bool {
	return token != 0 && token == m.deleteSessionToken && id != "" && id == m.deleteProfileID
}

func (m *model) clearCredential(id string) {
	m.run("profile-credential-clear", "正在清除已保存认证…", func(ctx context.Context) error {
		return m.desktop.ClearCredential(ctx, id)
	})
}

func (m *model) testProfile(id string) {
	m.busy["probe:"+id] = true
	m.setStatus("正在测试代理连接…", noticePending)
	m.win.Invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		result, err := m.desktop.TestProfile(ctx, id)
		cancel()
		m.win.Update(func() {
			delete(m.busy, "probe:"+id)
			if result.CheckedAt.IsZero() {
				result.CheckedAt = time.Now()
			}
			if err != nil && result.Detail == "" {
				result.Detail = err.Error()
			}
			// Failed re-tests replace previous successes as well. The editor
			// must never show an older success as the latest test result.
			m.probes[id] = result
			if err != nil {
				m.setStatus("代理测试失败："+err.Error(), noticeError)
				return
			}
			// Keep the detailed probe result in the editor and announce the
			// completed operation through the shared floating notification.
			m.setStatus("代理连接测试完成", noticeSuccess)
		})
	}()
}

func (m *model) run(key, pending string, fn func(context.Context) error) {
	m.runAfter(key, pending, fn, nil)
}

func (m *model) enqueueUpdate(update func()) {
	if m.postUpdate != nil {
		m.postUpdate(update)
		return
	}
	if m.win != nil {
		m.win.Update(update)
	}
}

func (m *model) runAfter(key, pending string, fn func(context.Context) error, onSuccess func()) {
	if m.busy[key] {
		return
	}
	m.busy[key] = true
	m.setStatus(pending, noticePending)
	if m.win != nil {
		m.win.Invalidate()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		err := fn(ctx)
		cancel()
		update := func() {
			delete(m.busy, key)
			if err != nil {
				m.setStatus("操作失败："+err.Error(), noticeError)
				return
			}
			m.snapshot = m.desktop.Snapshot()
			if onSuccess != nil {
				onSuccess()
			}
			m.setStatus(completedMessage(key), noticeSuccess)
		}
		m.enqueueUpdate(update)
	}()
}
