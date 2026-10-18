//go:build !windows && !darwin

package console

import (
	"context"
	"errors"
	"proxy-start/internal/application"
)

type Launcher struct{}

var _ application.ConsoleLauncher = (*Launcher)(nil)

func NewLauncher() *Launcher { return &Launcher{} }

func (*Launcher) Open(context.Context, application.ConsoleSpec) error {
	return errors.New("当前平台不支持代理控制台")
}
