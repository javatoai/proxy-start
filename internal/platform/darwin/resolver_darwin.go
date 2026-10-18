//go:build darwin

// Package darwin contains the macOS adapters. It intentionally deals in
// ordinary .app bundles and returns only platform-neutral domain values.
package darwin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

type Resolver struct {
	commandTimeout time.Duration
}

var _ application.AppResolver = (*Resolver)(nil)

func NewResolver() *Resolver { return &Resolver{commandTimeout: 8 * time.Second} }

func (r *Resolver) Resolve(ctx context.Context, input string) (domain.AppIdentity, error) {
	if err := ctx.Err(); err != nil {
		return domain.AppIdentity{}, err
	}
	path := strings.TrimSpace(input)
	if path == "" {
		return domain.AppIdentity{}, errors.New("应用路径不能为空")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return domain.AppIdentity{}, fmt.Errorf("解析应用路径失败: %w", err)
	}
	path = filepath.Clean(path)
	if !strings.HasSuffix(strings.ToLower(path), ".app") {
		path = enclosingBundle(path)
	}
	if path == "" {
		return domain.AppIdentity{}, errors.New("macOS 首期仅支持 .app 应用包")
	}
	return r.resolveBundle(ctx, path)
}

func (r *Resolver) Installed(ctx context.Context) ([]domain.AppIdentity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	roots := []string{"/Applications", "/System/Applications", "/System/Library/CoreServices"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, filepath.Join(home, "Applications"))
	}
	seen := make(map[string]struct{})
	result := make([]domain.AppIdentity, 0)
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("读取应用目录 %s: %w", root, err)
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".app") {
				continue
			}
			identity, err := r.resolveBundle(ctx, filepath.Join(root, entry.Name()))
			if err != nil {
				continue
			}
			key := strings.ToLower(identity.AUMID)
			if key == "" {
				key = strings.ToLower(identity.Executable)
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, identity)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		// Keep the supported first-party app easy to find in the picker.
		ci, cj := strings.EqualFold(result[i].PackageName, "com.openai.codex"), strings.EqualFold(result[j].PackageName, "com.openai.codex")
		if ci != cj {
			return ci
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func (r *Resolver) resolveBundle(ctx context.Context, bundle string) (domain.AppIdentity, error) {
	bundle = filepath.Clean(bundle)
	info, err := os.Stat(bundle)
	if err != nil {
		return domain.AppIdentity{}, fmt.Errorf("无法读取应用包 %q: %w", bundle, err)
	}
	if !info.IsDir() || !strings.HasSuffix(strings.ToLower(bundle), ".app") {
		return domain.AppIdentity{}, errors.New("应用路径必须是 .app 应用包")
	}
	plist, err := r.readPlist(ctx, filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		return domain.AppIdentity{}, err
	}
	executableName := firstString(plist, "CFBundleExecutable")
	executable, ok := bundleExecutablePath(bundle, executableName)
	if !ok {
		return domain.AppIdentity{}, errors.New("应用包的 CFBundleExecutable 无效或越出 Contents/MacOS")
	}
	execInfo, err := os.Stat(executable)
	if err != nil || execInfo.IsDir() || execInfo.Mode()&0111 == 0 {
		return domain.AppIdentity{}, fmt.Errorf("应用包缺少可执行文件: %s", executable)
	}
	displayName := firstString(plist, "CFBundleDisplayName", "CFBundleName")
	if displayName == "" {
		displayName = strings.TrimSuffix(filepath.Base(bundle), filepath.Ext(bundle))
	}
	bundleID := firstString(plist, "CFBundleIdentifier")
	publisher := firstString(plist, "CFBundleGetInfoString", "NSHumanReadableCopyright")
	system := isSystemBundle(bundle, bundleID, publisher)
	return domain.AppIdentity{
		Name:             displayName,
		Executable:       executable,
		WorkingDirectory: bundle,
		Publisher:        publisher,
		SystemApp:        system,
		PackageName:      bundleID,
		PackageFamily:    bundleID,
		ApplicationID:    executableName,
		AUMID:            bundleIdentity(bundleID),
	}, nil
}

func bundleIdentity(bundleID string) string {
	if strings.TrimSpace(bundleID) == "" {
		return ""
	}
	return "bundle:" + strings.TrimSpace(bundleID)
}

func isSystemBundle(path, bundleID, publisher string) bool {
	lowerPath := strings.ToLower(filepath.Clean(path))
	lowerID := strings.ToLower(strings.TrimSpace(bundleID))
	lowerPublisher := strings.ToLower(publisher)
	return strings.HasPrefix(lowerPath, "/system/") || strings.HasPrefix(lowerID, "com.apple.") ||
		strings.Contains(lowerID, "microsoft") || strings.Contains(lowerPublisher, "apple") || strings.Contains(lowerPublisher, "microsoft")
}

func (r *Resolver) readPlist(ctx context.Context, path string) (map[string]any, error) {
	commandCtx := ctx
	if r.commandTimeout > 0 {
		var cancel context.CancelFunc
		commandCtx, cancel = context.WithTimeout(ctx, r.commandTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(commandCtx, "/usr/bin/plutil", "-convert", "xml1", "-o", "-", "--", path)
	output, err := cmd.Output()
	if err != nil {
		if commandCtx.Err() != nil {
			return nil, commandCtx.Err()
		}
		return nil, fmt.Errorf("读取 Info.plist 失败: %w", err)
	}
	value, err := parsePlistXML(output)
	if err != nil {
		return nil, fmt.Errorf("解析 Info.plist 失败: %w", err)
	}
	return value, nil
}
