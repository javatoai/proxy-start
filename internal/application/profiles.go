package application

import (
	"context"
	"errors"
	"fmt"
	"proxy-start/internal/domain"
	"sync"
	"time"
)

type profileService struct {
	mu          sync.Mutex
	repository  *settingsRepository
	credentials CredentialStore
	probe       ProxyProbe
}

func (s *profileService) save(ctx context.Context, p domain.ProxyProfile, password string) error {
	_, err := s.saveResult(ctx, p, password)
	return err
}

// saveResult persists a profile and returns the canonical profile, including
// the generated ID for new profiles. Keeping ID assignment and persistence in
// one operation prevents the UI from having to rediscover a saved profile by
// display fields such as name or host.
func (s *profileService) saveResult(ctx context.Context, p domain.ProxyProfile, password string) (domain.ProxyProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := p.Validate(); err != nil {
		return domain.ProxyProfile{}, err
	}
	if p.ID == "" {
		p.ID = newID()
	}
	old, oldErr := s.repository.profile(p.ID)
	oldExists := oldErr == nil
	if oldErr != nil && !errors.Is(oldErr, ErrNotFound) {
		return domain.ProxyProfile{}, oldErr
	}
	if p.CredentialRef == "" {
		p.CredentialRef = old.CredentialRef
	}
	oldCredentialRef := ""
	if oldExists {
		oldCredentialRef = old.CredentialRef
	}
	// When a replacement writes the same deterministic key, read the old
	// value first so a settings-store failure can restore it. A missing old
	// credential is intentionally treated as an empty value and remains
	// idempotent.
	oldSecret := ""
	oldSecretExists := false
	if password != "" {
		// Reuse an existing reference when replacing a password. This avoids
		// orphaning a custom credential key and makes rollback deterministic.
		if oldCredentialRef != "" {
			p.CredentialRef = oldCredentialRef
		} else if p.CredentialRef == "" {
			p.CredentialRef = "proxy-start/" + p.ID
		}
		if oldCredentialRef == p.CredentialRef {
			secret, readErr := s.credentials.Read(ctx, oldCredentialRef)
			if readErr != nil && !errors.Is(readErr, ErrCredentialNotFound) {
				return domain.ProxyProfile{}, fmt.Errorf("读取旧代理密码失败：%w", readErr)
			}
			if readErr == nil {
				oldSecret, oldSecretExists = secret, true
			}
		}
		if err := s.credentials.Write(ctx, p.CredentialRef, password); err != nil {
			writeErr := fmt.Errorf("保存代理密码失败：%w", err)
			if restoreErr := s.restoreCredential(p.CredentialRef, oldSecret, oldSecretExists); restoreErr != nil {
				writeErr = errors.Join(writeErr, fmt.Errorf("还原代理凭据失败：%w", restoreErr))
			}
			return domain.ProxyProfile{}, writeErr
		}
	}
	if err := s.repository.change(ctx, func(settings *domain.Settings) error {
		for i, existing := range settings.Profiles {
			if existing.ID == p.ID {
				settings.Profiles[i] = p
				return nil
			}
		}
		settings.Profiles = append(settings.Profiles, p)
		if settings.DefaultProxyID == "" {
			settings.DefaultProxyID = p.ID
		}
		return nil
	}); err != nil {
		if password != "" {
			if restoreErr := s.restoreCredential(p.CredentialRef, oldSecret, oldSecretExists); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("还原代理凭据失败：%w", restoreErr))
			}
		}
		return domain.ProxyProfile{}, err
	}
	return p, nil
}

func (s *profileService) delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.repository.profile(id)
	if err != nil {
		return err
	}
	oldSecret, oldSecretExists, err := s.readCredential(ctx, old.CredentialRef)
	if err != nil {
		return err
	}
	credentialDeleted := false
	err = s.repository.change(ctx, func(settings *domain.Settings) error {
		if err := validateProfileDeletion(*settings, id); err != nil {
			return err
		}
		found := false
		for _, profile := range settings.Profiles {
			if profile.ID == id {
				found = true
				break
			}
		}
		if !found {
			return ErrNotFound
		}
		if err := s.deleteCredential(ctx, old.CredentialRef); err != nil {
			return fmt.Errorf("删除代理密码失败：%w", err)
		}
		credentialDeleted = old.CredentialRef != ""
		for i, p := range settings.Profiles {
			if p.ID == id {
				settings.Profiles = append(settings.Profiles[:i], settings.Profiles[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
	if err != nil && credentialDeleted {
		if restoreErr := s.restoreCredential(old.CredentialRef, oldSecret, oldSecretExists); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("还原代理凭据失败：%w", restoreErr))
		}
	}
	return err
}

func (s *profileService) clearCredential(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, err := s.repository.profile(id)
	if err != nil {
		return err
	}
	credentialRef := profile.CredentialRef
	oldSecret, oldSecretExists, err := s.readCredential(ctx, credentialRef)
	if err != nil {
		return err
	}
	if err := s.deleteCredential(ctx, credentialRef); err != nil {
		return fmt.Errorf("删除代理密码失败：%w", err)
	}
	if err := s.repository.change(ctx, func(settings *domain.Settings) error {
		for i := range settings.Profiles {
			if settings.Profiles[i].ID != id {
				continue
			}
			// Profile writes are serialized by this service. Keep the check as
			// an invariant in the durable transaction as well, so a future
			// caller cannot clear a credential that was replaced meanwhile.
			if settings.Profiles[i].CredentialRef != credentialRef {
				return fmt.Errorf("代理配置已变更，请重试")
			}
			settings.Profiles[i].CredentialRef = ""
			settings.Profiles[i].Username = ""
			return nil
		}
		return ErrNotFound
	}); err != nil {
		if restoreErr := s.restoreCredential(credentialRef, oldSecret, oldSecretExists); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("还原代理凭据失败：%w", restoreErr))
		}
		return err
	}
	return nil
}

func validateProfileDeletion(settings domain.Settings, id string) error {
	if len(settings.Profiles) <= 1 {
		return fmt.Errorf("请至少保留一个代理配置")
	}
	if settings.DefaultProxyID == id {
		return fmt.Errorf("请先将其他代理设为默认，再删除此代理")
	}
	for _, app := range settings.Apps {
		if app.ProxyID == id {
			return fmt.Errorf("启动项“%s”正在使用此代理，请先更换", app.Name)
		}
	}
	return nil
}

func (s *profileService) readCredential(ctx context.Context, ref string) (string, bool, error) {
	if ref == "" {
		return "", false, nil
	}
	secret, err := s.credentials.Read(ctx, ref)
	if errors.Is(err, ErrCredentialNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("读取代理密码失败：%w", err)
	}
	return secret, true, nil
}

func (s *profileService) deleteCredential(ctx context.Context, ref string) error {
	if ref == "" {
		return nil
	}
	err := s.credentials.Delete(ctx, ref)
	if errors.Is(err, ErrCredentialNotFound) {
		return nil
	}
	return err
}

// restoreCredential uses a short independent context so a cancelled settings
// operation cannot leave the credential half-updated. The secret never enters
// an error string or log.
func (s *profileService) restoreCredential(ref, secret string, existed bool) error {
	if ref == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if existed {
		return s.credentials.Write(ctx, ref, secret)
	}
	return s.deleteCredential(ctx, ref)
}

func (s *profileService) endpoint(ctx context.Context, p domain.ProxyProfile) (domain.ProxyEndpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Callers often resolve a profile immediately before this method. Refresh
	// by stable ID while holding the profile mutex so a concurrent edit cannot
	// combine old endpoint fields with a newly replaced credential.
	if p.ID != "" {
		latest, err := s.repository.profile(p.ID)
		if err != nil {
			return domain.ProxyEndpoint{}, err
		}
		p = latest
	}
	return s.endpointUnlocked(ctx, p)
}

func (s *profileService) endpointUnlocked(ctx context.Context, p domain.ProxyProfile) (domain.ProxyEndpoint, error) {
	endpoint := domain.ProxyEndpoint{Protocol: p.Protocol, Host: p.Host, Port: p.Port, Username: p.Username}
	if p.CredentialRef != "" {
		password, err := s.credentials.Read(ctx, p.CredentialRef)
		if err != nil {
			return endpoint, fmt.Errorf("读取代理密码失败：%w", err)
		}
		endpoint.Password = password
	}
	return endpoint, nil
}

func (s *profileService) test(ctx context.Context, id string) (domain.ProbeResult, error) {
	p, err := s.repository.profile(id)
	if err != nil {
		return domain.ProbeResult{}, err
	}
	e, err := s.endpoint(ctx, p)
	if err != nil {
		return domain.ProbeResult{}, err
	}
	return s.probe.Test(ctx, e)
}
