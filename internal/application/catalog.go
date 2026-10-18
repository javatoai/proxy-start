package application

import (
	"context"
	"fmt"
	"path/filepath"
	"proxy-start/internal/domain"
	"strings"
)

type catalogService struct {
	repository *settingsRepository
	resolver   AppResolver
}

func (s *catalogService) add(ctx context.Context, input string) (domain.AppEntry, error) {
	identity, err := s.resolver.Resolve(ctx, input)
	if err != nil {
		return domain.AppEntry{}, err
	}
	entry := domain.AppEntry{ID: newID(), Name: identity.Name, Source: input, Identity: identity}
	err = s.repository.change(ctx, func(settings *domain.Settings) error {
		for _, existing := range settings.Apps {
			if identityEqual(existing.Identity, identity) {
				return fmt.Errorf("“%s”已经在启动列表中", existing.Name)
			}
		}
		settings.Apps = append(settings.Apps, entry)
		return nil
	})
	return entry, err
}

func (s *catalogService) save(ctx context.Context, entry domain.AppEntry) error {
	if strings.TrimSpace(entry.Name) == "" {
		return fmt.Errorf("请填写程序名称")
	}
	paths := make([]string, 0, len(entry.RelatedExecutables))
	for _, path := range entry.RelatedExecutables {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".exe") {
			return fmt.Errorf("关联程序必须是 exe 的完整路径")
		}
		paths = append(paths, filepath.Clean(path))
	}
	return s.repository.change(ctx, func(settings *domain.Settings) error {
		if entry.ProxyID != "" {
			profileExists := false
			for _, profile := range settings.Profiles {
				if profile.ID == entry.ProxyID {
					profileExists = true
					break
				}
			}
			if !profileExists {
				return ErrNotFound
			}
		}
		for i, existing := range settings.Apps {
			if existing.ID == entry.ID {
				// Identity and source are resolver-owned; editing labels cannot replace a launch target.
				existing.Name = strings.TrimSpace(entry.Name)
				existing.ProxyID = entry.ProxyID
				existing.RelatedExecutables = paths
				settings.Apps[i] = existing
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *catalogService) remove(ctx context.Context, id string) error {
	return s.repository.change(ctx, func(settings *domain.Settings) error {
		for i, entry := range settings.Apps {
			if entry.ID == id {
				settings.Apps = append(settings.Apps[:i], settings.Apps[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
}
