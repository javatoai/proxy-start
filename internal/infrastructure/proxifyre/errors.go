package proxifyre

import "errors"

// Sentinel errors let the UI present an actionable recovery action while the
// concrete error retains the failing path or component details.
var (
	ErrComponentMissing      = errors.New("proxifyre component is missing")
	ErrPrivilegeRequired     = errors.New("proxifyre operation requires administrator privileges")
	ErrConfigurationConflict = errors.New("existing proxifyre configuration is not managed by Proxy Start")
	ErrInvalidSession        = errors.New("forced proxy session is invalid")
	ErrHelperUnavailable     = errors.New("proxifyre elevated helper is unavailable")
)
