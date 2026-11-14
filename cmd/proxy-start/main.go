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
	"proxy-start/internal/application"
	"proxy-start/internal/infrastructure/storage"
	"proxy-start/internal/ui"
)

func main() {
	if err := run(); err != nil {
		log.Printf("Proxy Start: %v", err)
		diagnostic := false
		for _, arg := range os.Args[1:] {
			if strings.HasPrefix(arg, "--") {
				diagnostic = true
			}
		}
		if !diagnostic {
			showStartupError(err.Error())
		}
		os.Exit(1)
	}
}

func run() error {
	if handled, err := runPackagedMode(os.Args[1:]); handled {
		return err
	}
	config := flag.String("config", "", "override settings path")
	licenses := flag.Bool("licenses", false, "print bundled third-party license notices")
	check := flag.Bool("check", false, "print non-secret Codex and proxy launch configuration")
	diagnose := flag.Bool("diagnose-proxy", false, "check whether the saved local proxy port is listening")
	console := flag.Bool("open-proxy-console", false, "open an independent proxy console")
	smoke := flag.Bool("ui-smoke", false, "preview the native window without launching Codex")
	duration := flag.Duration("ui-smoke-duration", 5*time.Second, "preview lifetime (1s to 2m)")
	size := flag.String("ui-smoke-size", "460x420", "preview window size")
	capture := flag.String("ui-smoke-capture", "", "save a native preview frame as PNG")
	flag.Parse()
	if *licenses {
		_, err := fmt.Fprint(os.Stdout, buildinfo.ThirdPartyNotices())
		return err
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if *config == "" {
		*config = filepath.Join(configRoot, "Proxy Start", "settings.json")
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	logDir := filepath.Join(cacheRoot, "Proxy Start", "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return err
	}
	output, err := os.OpenFile(filepath.Join(logDir, "proxy-start.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	log.SetOutput(io.MultiWriter(output, os.Stderr))
	services, err := newPlatformServices("")
	if err != nil {
		return err
	}
	ctx := context.Background()
	desktop, err := application.NewDesktop(ctx, application.Dependencies{
		Store: storage.New(*config), Resolver: services.resolver, Processes: services.processes, Console: services.console, Platform: services.capabilities,
	})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := desktop.Shutdown(cleanup); err != nil {
			log.Printf("退出：%v", err)
		}
	}()
	if *check {
		task, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := desktop.WaitReady(task); err != nil {
			return err
		}
		s := desktop.Snapshot()
		return json.NewEncoder(os.Stdout).Encode(struct {
			Version, Platform, SettingsPath           string
			ProxyPort                                 int
			CodexPath, State, ScanError, RuntimeError string
			Proxy                                     any
		}{buildinfo.Version(), services.target, *config, s.Settings.ProxyPort, s.Identity.Executable, s.State, s.ScanError, s.RuntimeError, s.Proxy})
	}
	if *diagnose {
		return application.CheckLocalPort(ctx, desktop.Snapshot().Settings.ProxyPort)
	}
	if *console {
		task, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return desktop.OpenProxyConsole(task)
	}
	options := ui.Options{}
	if *smoke {
		if *duration < time.Second || *duration > 2*time.Minute {
			return fmt.Errorf("预览时长范围为 1s 到 2m")
		}
		w, h, ok := strings.Cut(*size, "x")
		width, werr := strconv.Atoi(w)
		height, herr := strconv.Atoi(h)
		if !ok || werr != nil || herr != nil || width < 380 || width > 4096 || height < 360 || height > 2160 {
			return fmt.Errorf("预览尺寸范围为 380x360 到 4096x2160")
		}
		options.AutoQuitAfter, options.WindowWidth, options.WindowHeight = *duration, width, height
		previewCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		readyErr := desktop.WaitReady(previewCtx)
		cancel()
		if readyErr != nil {
			return readyErr
		}
		if *capture != "" {
			path := *capture
			options.OnReady = func(window *mygo.Window) {
				time.AfterFunc(1500*time.Millisecond, func() {
					data, err := window.CapturePage()
					if err == nil {
						err = os.WriteFile(path, data, 0600)
					}
					if err != nil {
						log.Printf("预览截图：%v", err)
					}
				})
			}
		}
	}
	return ui.RunWithOptions(desktop, options)
}
