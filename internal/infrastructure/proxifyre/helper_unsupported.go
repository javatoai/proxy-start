//go:build !windows

package proxifyre

import (
	"context"
	"fmt"
)

func (b *Backend) ensureHelper(context.Context) (helperTransport, error) {
	b.stateMu.RLock()
	current := b.helper
	b.stateMu.RUnlock()
	if current != nil {
		return current, nil
	}
	return nil, fmt.Errorf("%w: forced interception is available in the Windows build only", ErrComponentMissing)
}

func launchInstaller(context.Context, string) error {
	return fmt.Errorf("%w: ProxiFyre installation is available in the Windows build only", ErrComponentMissing)
}
