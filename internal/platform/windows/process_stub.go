//go:build !windows

package windows

import (
	"context"
	"errors"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

// ProcessController is retained on non-Windows builds for interface-level
// tests and tooling; it fails closed rather than pretending process control is
// available.
type ProcessController struct{}

var _ application.ProcessController = (*ProcessController)(nil)

func NewProcessController() *ProcessController { return &ProcessController{} }

func unavailable() error { return errors.New("Windows 进程控制器只能在 Windows 上使用") }

func (*ProcessController) Find(context.Context, domain.AppIdentity) ([]domain.ProcessHandle, error) {
	return nil, unavailable()
}

func (*ProcessController) Start(context.Context, domain.LaunchSpec) (domain.ProcessHandle, error) {
	return domain.ProcessHandle{}, unavailable()
}

func (*ProcessController) Stop(context.Context, domain.AppIdentity, []domain.ProcessHandle) error {
	return unavailable()
}

func (*ProcessController) Alive(context.Context, domain.ProcessHandle) bool { return false }
