// Package storage persists the small v2 configuration, preserving v1 originals.
package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"proxy-start/internal/domain"
)

type Store struct {
	path       string
	mu         sync.Mutex
	legacy     []byte
	createTemp func(string, string) (*os.File, error)
}

func New(path string) *Store { return &Store{path: path} }

func (s *Store) temporary(dir, pattern string) (*os.File, error) {
	if s.createTemp != nil {
		return s.createTemp(dir, pattern)
	}
	return os.CreateTemp(dir, pattern)
}

func (s *Store) Load(ctx context.Context) (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Settings{}, err
	}
	if strings.TrimSpace(s.path) == "" {
		return domain.Settings{}, errors.New("配置路径为空")
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.DefaultSettings(), nil
	}
	if err != nil {
		return domain.Settings{}, err
	}
	var header struct{ Version int }
	if err := json.Unmarshal(data, &header); err != nil {
		return domain.Settings{}, fmt.Errorf("配置 JSON 无效：%w", err)
	}
	if header.Version == 1 {
		settings, err := migrate(data)
		if err == nil {
			s.legacy = append([]byte(nil), data...)
		}
		return settings, err
	}
	var decoded struct {
		Version   int    `json:"version"`
		ProxyPort int    `json:"proxyPort"`
		Theme     string `json:"theme,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return domain.Settings{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.Settings{}, errors.New("配置含多余 JSON 内容")
	}
	settings := domain.Settings{Version: decoded.Version, ProxyPort: decoded.ProxyPort}
	return settings, validate(settings)
}

func migrate(data []byte) (domain.Settings, error) {
	var old struct {
		DefaultProxyID string
		Theme          string
		Profiles       []struct {
			ID, Protocol, Host, Username, CredentialRef string
			Port                                        int
		}
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return domain.Settings{}, err
	}
	settings := domain.DefaultSettings()
	for _, profile := range old.Profiles {
		if profile.ID == old.DefaultProxyID && (profile.Protocol == "http" || profile.Protocol == "auto") &&
			(profile.Host == "127.0.0.1" || strings.EqualFold(profile.Host, "localhost")) &&
			profile.Username == "" && profile.CredentialRef == "" && domain.ValidatePort(profile.Port) == nil {
			settings.ProxyPort = profile.Port
			break
		}
	}
	return settings, nil
}

func (s *Store) Save(ctx context.Context, settings domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validate(settings); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(s.path) == "" {
		return errors.New("配置路径为空")
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if s.legacy != nil {
		original, err := os.ReadFile(s.path)
		if err != nil {
			return fmt.Errorf("读取待备份旧配置：%w", err)
		}
		if !bytes.Equal(original, s.legacy) {
			return errors.New("旧配置已被其它程序修改，请重新打开启动器")
		}
		backup, err := s.temporary(dir, "settings.v1-backup-*.json")
		if err != nil {
			return fmt.Errorf("备份旧配置失败：%w", err)
		}
		if err = writeAndSync(backup, original); err != nil {
			_ = os.Remove(backup.Name())
			return fmt.Errorf("备份旧配置失败：%w", err)
		}
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	temp, err := s.temporary(dir, ".proxy-start-settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := writeAndSync(temp, append(data, '\n')); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), s.path); err != nil {
		return err
	}
	s.legacy = nil
	return nil
}

func writeAndSync(file *os.File, data []byte) error {
	_, err := file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func validate(settings domain.Settings) error {
	if settings.Version != 2 {
		return fmt.Errorf("不支持的配置版本 %d", settings.Version)
	}
	if err := domain.ValidatePort(settings.ProxyPort); err != nil {
		return err
	}

	return nil
}
