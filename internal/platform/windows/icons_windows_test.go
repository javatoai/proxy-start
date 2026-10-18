//go:build windows

package windows

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"proxy-start/internal/domain"
)

func TestIconProviderDecodesSystemExecutable(t *testing.T) {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		t.Skip("SystemRoot is not set")
	}
	executable := filepath.Join(systemRoot, "System32", "notepad.exe")
	if _, err := os.Stat(executable); err != nil {
		executable = filepath.Join(systemRoot, "System32", "cmd.exe")
	}
	if _, err := os.Stat(executable); err != nil {
		t.Skip("no system executable fixture available")
	}

	provider := NewIconProvider()
	data, err := provider.Icon(context.Background(), domain.AppIdentity{Executable: executable})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("icon PNG decode failed: %v", err)
	}
	if decoded.Bounds().Dx() != iconSize || decoded.Bounds().Dy() != iconSize {
		t.Fatalf("icon dimensions = %v, want %dx%d", decoded.Bounds(), iconSize, iconSize)
	}
	outputPath := os.Getenv("PROXY_START_ICON_OUTPUT")
	if outputPath == "" {
		outputPath = filepath.Join(t.TempDir(), "system-icon.png")
	}
	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("PNG output for visual inspection: %s", outputPath)

	// A second read exercises the mtime-keyed cache and must return identical
	// bytes without extracting the icon again.
	cached, err := provider.Icon(context.Background(), domain.AppIdentity{Executable: executable})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cached, data) {
		t.Fatal("cached icon differs from first extraction")
	}
}
