//go:build !windows

package proxifyre

import (
	"context"
	"proxy-start/internal/domain"
)

func initialStatus(Options) domain.BackendStatus {
	return domain.BackendStatus{Detail: "强制代理后端只支持 Windows"}
}

func statusFor(context.Context, Options) domain.BackendStatus {
	return initialStatus(Options{})
}
