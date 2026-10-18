package proxifyre

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DependencyManifest struct {
	Name    string
	Version string
	URL     string
	SHA256  string
}

// OfficialDependency is the pinned x64 setup published by ProxiFyre. The
// client never executes a downloaded file until this digest has been checked.
func OfficialDependency() DependencyManifest {
	return DependencyManifest{
		Name:    "ProxiFyre-2.6.4-win-x64-setup.exe",
		Version: "2.6.4",
		URL:     "https://github.com/wiresock/proxifyre/releases/download/v2.6.4/ProxiFyre-2.6.4-win-x64-setup.exe",
		SHA256:  "afaf3ac8a2069f9653adb1b4a63b31d1edd8aa109ccdc367cf0d4462e78f5027",
	}
}

func downloadPinned(ctx context.Context, spec DependencyManifest) (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(cacheRoot, "ProxyStart", "downloads")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, spec.Name)
	if _, err := os.Stat(path); err == nil {
		if err := verifySHA256(ctx, path, spec.SHA256); err == nil {
			return path, nil
		}
		_ = os.Remove(path)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download pinned ProxiFyre setup: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download pinned ProxiFyre setup: HTTP %s", response.Status)
	}
	temp := path + ".download"
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(file, response.Body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(temp)
		return "", copyErr
	}
	if syncErr != nil {
		_ = os.Remove(temp)
		return "", syncErr
	}
	if closeErr != nil {
		_ = os.Remove(temp)
		return "", closeErr
	}
	if err := verifySHA256(ctx, temp, spec.SHA256); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	return path, nil
}

func verifySHA256(ctx context.Context, path, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if len(expected) != sha256.Size*2 {
		return fmt.Errorf("installer checksum pin is malformed: %q", expected)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read installer for checksum: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.CopyBuffer(hash, file, make([]byte, 256*1024)); err != nil {
		return fmt.Errorf("hash installer: %w", err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("installer checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}
