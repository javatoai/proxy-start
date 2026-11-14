//go:build !windows

package windows

import (
	"context"
	"errors"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

// Resolver is retained on non-Windows builds so the rest of the application
// can be compiled and tested without platform conditionals at every call site.
type Resolver struct{}

var _ application.CodexResolver = (*Resolver)(nil)

func NewResolver() *Resolver { return &Resolver{} }

func (*Resolver) Discover(context.Context) (domain.AppIdentity, error) {
	return domain.AppIdentity{}, errors.New("Codex 扫描器只能在 Windows 上使用")
}
