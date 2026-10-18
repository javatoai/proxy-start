package proxifyre

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"proxy-start/internal/domain"
)

type helperTransport interface {
	Apply(context.Context, helperApplyRequest) (helperResponse, error)
	Shutdown(context.Context) (helperResponse, error)
	Close() error
}

type helperResponse struct {
	OK     bool                 `json:"ok"`
	Code   string               `json:"code,omitempty"`
	Error  string               `json:"error,omitempty"`
	Detail string               `json:"detail,omitempty"`
	Status domain.BackendStatus `json:"status,omitempty"`
}

type helperEnvelope struct {
	Op       string              `json:"op"`
	Token    string              `json:"token,omitempty"`
	Apply    *helperApplyRequest `json:"apply,omitempty"`
	Response *helperResponse     `json:"response,omitempty"`
}

func helperError(result helperResponse) error {
	message := strings.TrimSpace(result.Error)
	if message == "" {
		message = strings.TrimSpace(result.Detail)
	}
	if message == "" {
		message = "ProxiFyre helper rejected the request"
	}
	var kind error
	switch strings.ToLower(result.Code) {
	case "component_missing":
		kind = ErrComponentMissing
	case "privilege_required", "access_denied":
		kind = ErrPrivilegeRequired
	case "configuration_conflict":
		kind = ErrConfigurationConflict
	case "invalid_session":
		kind = ErrInvalidSession
	default:
		kind = ErrHelperUnavailable
	}
	return fmt.Errorf("%w: %s", kind, message)
}

func marshalEnvelope(value helperEnvelope) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
