//go:build darwin

package credentials

import (
	"errors"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestKeychainStatusMapping(t *testing.T) {
	if err := keychainStatus("find", errSecSuccess); err != nil {
		t.Fatalf("success mapped to %v", err)
	}
	if !errors.Is(keychainStatus("find", errSecItemNotFound), ErrNotFound) {
		t.Fatal("errSecItemNotFound must map to ErrNotFound")
	}
	if !errors.Is(keychainStatus("add", errSecDuplicateItem), errKeychainDuplicate) {
		t.Fatal("errSecDuplicateItem must map to duplicate sentinel")
	}
	err := keychainStatus("find", -25293)
	var statusErr *keychainStatusError
	if !errors.As(err, &statusErr) || statusErr.status != -25293 {
		t.Fatalf("unexpected status error: %v", err)
	}
	if got := err.Error(); got != "keychain find failed (OSStatus -25293)" {
		t.Fatalf("status error leaked or changed format: %q", got)
	}
}

func TestKeychainInteractionRestoresPreviousState(t *testing.T) {
	var states []uint8
	api := &keychainAPI{
		getInteraction: func(state *uint8) int32 {
			*state = 0
			return errSecSuccess
		},
		interaction: func(state uint8) int32 {
			states = append(states, state)
			return errSecSuccess
		},
	}
	if err := withKeychainInteraction(api, func(*keychainAPI) error { return errors.New("operation failed") }); err == nil || err.Error() != "operation failed" {
		t.Fatalf("operation error was not preserved: %v", err)
	}
	if len(states) != 2 || states[0] != 0 || states[1] != 0 {
		t.Fatalf("previous interaction state was not restored: %v", states)
	}
}

func TestKeychainInteractionRestoresAfterPanic(t *testing.T) {
	var states []uint8
	api := &keychainAPI{
		getInteraction: func(state *uint8) int32 {
			*state = 1
			return errSecSuccess
		},
		interaction: func(state uint8) int32 {
			states = append(states, state)
			return errSecSuccess
		},
	}
	func() {
		defer func() { _ = recover() }()
		_ = withKeychainInteraction(api, func(*keychainAPI) error { panic("test panic") })
	}()
	if len(states) != 2 || states[0] != 0 || states[1] != 1 {
		t.Fatalf("panic path did not restore previous interaction state: %v", states)
	}
}

// This test is opt-in because it creates a short-lived generic password in
// the current user's login keychain. It is intended for a real macOS runner,
// never for Windows cross-compilation or unattended CI.
func TestKeychainIntegration(t *testing.T) {
	if os.Getenv("PROXY_START_MACOS_KEYCHAIN_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_MACOS_KEYCHAIN_INTEGRATION=1 on a disposable macOS account")
	}
	key := "integration-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	defer func() { _ = deleteNative(key) }()
	if err := writeNative(key, "secret-one"); err != nil {
		t.Fatalf("write keychain item: %v", err)
	}
	got, err := readNative(key)
	if err != nil || got != "secret-one" {
		t.Fatalf("read keychain item: got %q, err %v", got, err)
	}
	if err := writeNative(key, "secret-two"); err != nil {
		t.Fatalf("update keychain item: %v", err)
	}
	got, err = readNative(key)
	if err != nil || got != "secret-two" {
		t.Fatalf("read updated keychain item: got %q, err %v", got, err)
	}
	if err := deleteNative(key); err != nil {
		t.Fatalf("delete keychain item: %v", err)
	}
	if _, err := readNative(key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted item should be missing, got %v", err)
	}
}
