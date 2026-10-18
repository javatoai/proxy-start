//go:build !windows

package windows

import (
	"context"
	"errors"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

type IconProvider struct{}

var _ application.AppIconProvider = (*IconProvider)(nil)

func NewIconProvider() *IconProvider { return &IconProvider{} }

func (*IconProvider) Icon(context.Context, domain.AppIdentity) ([]byte, error) {
	return nil, errors.New("Windows 图标提供器只能在 Windows 上使用")
}
