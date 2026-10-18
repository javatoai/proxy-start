// Package credentials stores proxy secrets outside settings JSON. Windows
// uses Credential Manager and macOS uses Keychain. Other platforms return an
// explicit unsupported error.
package credentials

import (
	"context"
	"errors"
	"strings"

	"proxy-start/internal/application"
)

var (
	// ErrUnsupported indicates that the current platform has no implementation.
	ErrUnsupported = errors.New("credential store is unsupported on this platform")
	// ErrNotFound indicates that the target credential does not exist.
	ErrNotFound = application.ErrCredentialNotFound
)

// Store implements application.CredentialStore using platform helpers.
type Store struct{}

// New creates a platform credential store.
func New() application.CredentialStore {
	return &Store{}
}

func (s *Store) Read(ctx context.Context, key string) (string, error) {
	if err := check(ctx, key); err != nil {
		return "", err
	}
	value, err := readNative(key)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *Store) Write(ctx context.Context, key, value string) error {
	if err := check(ctx, key); err != nil {
		return err
	}
	return writeNative(key, value)
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if err := check(ctx, key); err != nil {
		return err
	}
	return deleteNative(key)
}

func check(ctx context.Context, key string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(key) == "" {
		return errors.New("credential key is empty")
	}
	return nil
}
