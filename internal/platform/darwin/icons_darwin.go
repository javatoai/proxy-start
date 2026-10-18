//go:build darwin

package darwin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

type IconProvider struct {
	commandTimeout time.Duration
}

var _ application.AppIconProvider = (*IconProvider)(nil)

func NewIconProvider() *IconProvider { return &IconProvider{commandTimeout: 5 * time.Second} }

func (p *IconProvider) Icon(ctx context.Context, identity domain.AppIdentity) ([]byte, error) {
	bundle := enclosingBundle(identity.Executable)
	if bundle == "" {
		return nil, errors.New("应用不是 macOS .app 包，无法读取图标")
	}
	plist, err := (&Resolver{commandTimeout: p.commandTimeout}).readPlist(ctx, filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		return nil, err
	}
	iconName := firstString(plist, "CFBundleIconFile")
	if iconName == "" {
		if values, ok := plist["CFBundleIconFiles"].([]any); ok && len(values) > 0 {
			iconName, _ = values[0].(string)
		} else if value, ok := plist["CFBundleIconFiles"].(string); ok {
			// Older plist files and the small parser's compatibility path may
			// expose a single icon entry as text.
			iconName = strings.TrimSpace(value)
		}
	}
	if iconName == "" {
		iconName = "AppIcon.icns"
	}
	if !strings.HasSuffix(strings.ToLower(iconName), ".icns") {
		iconName += ".icns"
	}
	iconPath := filepath.Join(bundle, "Contents", "Resources", filepath.Base(iconName))
	if _, err := os.Stat(iconPath); err != nil {
		return nil, fmt.Errorf("应用图标不存在: %w", err)
	}
	temp, err := os.CreateTemp("", "proxy-start-icon-*.png")
	if err != nil {
		return nil, err
	}
	outputPath := temp.Name()
	_ = temp.Close()
	defer os.Remove(outputPath)
	commandCtx := ctx
	if p.commandTimeout > 0 {
		var cancel context.CancelFunc
		commandCtx, cancel = context.WithTimeout(ctx, p.commandTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(commandCtx, "/usr/bin/sips", "-z", "32", "32", "-s", "format", "png", iconPath, "--out", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		if commandCtx.Err() != nil {
			return nil, commandCtx.Err()
		}
		return nil, fmt.Errorf("转换应用图标失败: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("读取转换后的应用图标失败: %w", err)
	}
	if err := verifyIconPNG(data); err != nil {
		return nil, fmt.Errorf("应用图标 PNG 无效: %w", err)
	}
	return data, nil
}
