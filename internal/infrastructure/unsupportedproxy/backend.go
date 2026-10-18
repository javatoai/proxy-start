// Package unsupportedproxy provides a backend that rejects interception on
// platforms where a forced-proxy implementation has not been delivered.
package unsupportedproxy

import (
	"context"
	"errors"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

type Backend struct{ reason string }

var _ application.ForcedBackend = (*Backend)(nil)

func New(reason string) *Backend { return &Backend{reason: reason} }

func (b *Backend) Apply(ctx context.Context, sessions []domain.ForcedSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(sessions) == 0 {
		return nil
	}
	return errors.New(b.reason)
}

func (b *Backend) Install(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New(b.reason)
}

func (*Backend) Shutdown(context.Context) error { return nil }

func (b *Backend) Status(context.Context) domain.BackendStatus {
	return domain.BackendStatus{Detail: b.reason}
}
