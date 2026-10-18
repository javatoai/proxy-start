package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	buildinfo "proxy-start"
	"proxy-start/internal/adapters/native"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
	"proxy-start/internal/infrastructure/credentials"
	"proxy-start/internal/infrastructure/netprobe"
	"proxy-start/internal/infrastructure/storage"
	"proxy-start/internal/ui"
)

// Set by the release build after hashing the independently built helper.
var helperSHA256 string

func main() {
	if err := run(); err != nil {
		log.Printf("Proxy Start: %v", err)
		if !isDiagnosticInvocation() {
			showStartupError(err.Error())
		}
		os.Exit(1)
	}
}

func run() error {
	configFlag := flag.String("config", "", "override settings path (for diagnostics and isolated previews)")
	proxyDiagnostic := flag.Bool("diagnose-proxy", false, "test the saved default proxy without launching an application")
	proxyConsole := flag.Bool("open-proxy-console", false, "open an interactive console with the saved default proxy environment")
	check := flag.Bool("check", false, "print non-secret configuration and component status")
	resolve := flag.String("resolve", "", "resolve an executable, shortcut, or package reference")
	smoke := flag.Bool("ui-smoke", false, "open the native window and close it automatically after five seconds")
	smokeDuration := flag.Duration("ui-smoke-duration", 5*time.Second, "preview lifetime, from one second to two minutes")
	smokePage := flag.String("ui-smoke-page", "launch", "preview page: launch, proxy-settings, or system-settings (settings remains an alias)")
	smokeSize := flag.String("ui-smoke-size", "1180x760", "native preview window size")
	smokeSelected := flag.Bool("ui-smoke-selected", false, "open the first configured application's inspector in a preview")
	smokeCapture := flag.String("ui-smoke-capture", "", "save a native preview frame as PNG")
	flag.Parse()
	ctx := context.Background()
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	configPath := *configFlag
	if configPath == "" {
		configPath = filepath.Join(configRoot, "Proxy Start", "settings.json")
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	logDirectory := filepath.Join(cacheRoot, "Proxy Start", "logs")
	if err = os.MkdirAll(logDirectory, 0700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(logDirectory, "proxy-start.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(logFile, os.Stderr))
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	services, err := newPlatformServices(executable)
	if err != nil {
		return err
	}
	backend, resolver := services.backend, services.resolver
	if *resolve != "" {
		taskCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		identity, err := resolver.Resolve(taskCtx, *resolve)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(identity)
	}
	desktop, err := application.NewDesktop(ctx, application.Dependencies{Store: storage.New(configPath), Credentials: credentials.New(), Resolver: resolver, Processes: services.processes, Probe: netprobe.New(), Backend: backend, Icons: services.icons, Console: services.console, Adapters: native.Production(), Platform: services.capabilities})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := desktop.Shutdown(cleanup); err != nil {
			log.Printf("代理组件清理失败: %v", err)
		}
	}()
	if *proxyDiagnostic {
		taskCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := desktop.TestProfile(taskCtx, desktop.Snapshot().Settings.DefaultProxyID)
		if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
			return encodeErr
		}
		return err
	}
	if *check {
		snapshot := desktop.Snapshot()
		return json.NewEncoder(os.Stdout).Encode(struct {
			Version, Platform, SettingsPath string
			Profiles, Apps                  int
			Backend                         domain.BackendStatus
		}{buildinfo.Version(), services.target, configPath, len(snapshot.Settings.Profiles), len(snapshot.Settings.Apps), backend.Status(ctx)})
	}
	if *proxyConsole {
		taskCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return desktop.OpenProxyConsole(taskCtx, desktop.Snapshot().Settings.DefaultProxyID)
	}
	options := ui.Options{}
	if *smoke {
		if *smokeDuration < time.Second || *smokeDuration > 2*time.Minute {
			return fmt.Errorf("预览时长范围为 1s 到 2m")
		}
		options.AutoQuitAfter = *smokeDuration
		if *smokePage != "launch" && *smokePage != "settings" && *smokePage != "proxy-settings" && *smokePage != "system-settings" {
			return fmt.Errorf("预览页面只能是 launch、proxy-settings 或 system-settings（兼容 settings）")
		}
		options.InitialPage = *smokePage
		width, height, ok := strings.Cut(*smokeSize, "x")
		options.WindowWidth, err = strconv.Atoi(width)
		if !ok || err != nil {
			return fmt.Errorf("预览尺寸应为 1180x760 这样的格式")
		}
		options.WindowHeight, err = strconv.Atoi(height)
		if err != nil || options.WindowWidth < 820 || options.WindowWidth > 4096 || options.WindowHeight < 560 || options.WindowHeight > 2160 {
			return fmt.Errorf("预览尺寸范围为 820x560 到 4096x2160")
		}
		if *smokeSelected {
			if apps := desktop.Snapshot().Settings.Apps; len(apps) > 0 {
				options.InitialAppID = apps[0].ID
			}
		}
		if *smokeCapture != "" {
			capturePath := *smokeCapture
			options.OnReady = func(window *mygo.Window) {
				time.AfterFunc(800*time.Millisecond, func() {
					data, captureErr := window.CapturePage()
					if captureErr == nil {
						captureErr = os.WriteFile(capturePath, data, 0600)
					}
					if captureErr != nil {
						log.Printf("原生预览截图失败: %v", captureErr)
					}
				})
			}
		}
	}
	if err := ui.RunWithOptions(desktop, options); err != nil {
		return fmt.Errorf("界面启动失败：%w", err)
	}
	return nil
}

func isDiagnosticInvocation() bool {
	for _, argument := range os.Args[1:] {
		if strings.HasPrefix(argument, "--check") || strings.HasPrefix(argument, "--diagnose-proxy") || strings.HasPrefix(argument, "--resolve") || strings.HasPrefix(argument, "--ui-smoke") || strings.HasPrefix(argument, "--open-proxy-console") {
			return true
		}
	}
	return false
}
