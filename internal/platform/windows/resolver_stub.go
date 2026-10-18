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

var _ application.AppResolver = (*Resolver)(nil)

func NewResolver() *Resolver { return &Resolver{} }

func (*Resolver) Resolve(context.Context, string) (domain.AppIdentity, error) {
	return domain.AppIdentity{}, errors.New("Windows 应用解析器只能在 Windows 上使用")
}

func (*Resolver) Installed(context.Context) ([]domain.AppIdentity, error) {
	return nil, errors.New("Windows 应用解析器只能在 Windows 上使用")
}
