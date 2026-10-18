//go:build darwin

package credentials

// This file talks to the user's login keychain through Security.framework.
// Calling the framework directly keeps secrets out of argv, shell history and
// command output. The service is fixed to "proxy-start" and the account is
// the credential key, so all application secrets share one stable namespace.

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	securityFramework      = "/System/Library/Frameworks/Security.framework/Security"
	coreFoundation         = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	errSecSuccess          = int32(0)
	errSecDuplicateItem    = int32(-25299)
	errSecItemNotFound     = int32(-25300)
	maxKeychainSecretBytes = 1 << 20
)

var errKeychainDuplicate = errors.New("keychain item already exists")

type keychainStatusError struct {
	op     string
	status int32
}

func (e *keychainStatusError) Error() string {
	// Do not include key, service or secret data in this error. OSStatus is
	// stable and is enough to diagnose a failed framework operation.
	return fmt.Sprintf("keychain %s failed (OSStatus %d)", e.op, e.status)
}

func keychainStatus(op string, status int32) error {
	switch status {
	case errSecSuccess:
		return nil
	case errSecItemNotFound:
		return ErrNotFound
	case errSecDuplicateItem:
		return errKeychainDuplicate
	default:
		return &keychainStatusError{op: op, status: status}
	}
}

// These signatures use pointers rather than C structs. SecKeychain's generic
// password APIs only pass byte buffers and opaque references, which makes them
// a good fit for purego and avoids a cgo dependency in the desktop binary.
type secKeychainFindGenericPassword func(
	unsafe.Pointer, uint32, unsafe.Pointer, uint32, unsafe.Pointer,
	*uint32, *unsafe.Pointer, *unsafe.Pointer,
) int32
type secKeychainAddGenericPassword func(
	unsafe.Pointer, uint32, unsafe.Pointer, uint32, unsafe.Pointer,
	uint32, unsafe.Pointer, *unsafe.Pointer,
) int32
type secKeychainItemModifyAttributesAndData func(unsafe.Pointer, unsafe.Pointer, uint32, unsafe.Pointer) int32
type secKeychainItemFreeContent func(unsafe.Pointer, unsafe.Pointer) int32
type secKeychainItemDelete func(unsafe.Pointer) int32
type secKeychainSetUserInteractionAllowed func(uint8) int32
type secKeychainGetUserInteractionAllowed func(*uint8) int32
type cfRelease func(unsafe.Pointer)

type keychainAPI struct {
	find           secKeychainFindGenericPassword
	add            secKeychainAddGenericPassword
	modify         secKeychainItemModifyAttributesAndData
	free           secKeychainItemFreeContent
	delete         secKeychainItemDelete
	interaction    secKeychainSetUserInteractionAllowed
	getInteraction secKeychainGetUserInteractionAllowed
	release        cfRelease
}

var (
	keychainOnce  sync.Once
	keychainLib   *keychainAPI
	keychainErr   error
	interactionMu sync.Mutex
)

func loadKeychain() (*keychainAPI, error) {
	keychainOnce.Do(func() {
		security, err := purego.Dlopen(securityFramework, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			keychainErr = fmt.Errorf("load Security.framework: %w", err)
			return
		}
		core, err := purego.Dlopen(coreFoundation, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			keychainErr = fmt.Errorf("load CoreFoundation: %w", err)
			return
		}
		api := &keychainAPI{}
		defer func() {
			if recovered := recover(); recovered != nil {
				keychainErr = fmt.Errorf("load keychain symbols: %v", recovered)
			}
		}()
		purego.RegisterLibFunc(&api.find, security, "SecKeychainFindGenericPassword")
		purego.RegisterLibFunc(&api.add, security, "SecKeychainAddGenericPassword")
		purego.RegisterLibFunc(&api.modify, security, "SecKeychainItemModifyAttributesAndData")
		purego.RegisterLibFunc(&api.free, security, "SecKeychainItemFreeContent")
		purego.RegisterLibFunc(&api.delete, security, "SecKeychainItemDelete")
		purego.RegisterLibFunc(&api.interaction, security, "SecKeychainSetUserInteractionAllowed")
		purego.RegisterLibFunc(&api.getInteraction, security, "SecKeychainGetUserInteractionAllowed")
		purego.RegisterLibFunc(&api.release, core, "CFRelease")
		keychainLib = api
	})
	if keychainErr != nil {
		return nil, keychainErr
	}
	if keychainLib == nil {
		return nil, errors.New("keychain framework is unavailable")
	}
	return keychainLib, nil
}

// keychainCall temporarily disables Security.framework's UI interaction for
// this process. A locked keychain therefore fails with an OSStatus instead of
// leaving the app waiting on an unbounded authorization dialog. The mutex
// prevents another operation in this process from observing the temporary
// setting. Store already checks context before entering this synchronous
// framework call; Security.framework itself has no cancellation parameter, so
// an in-flight OS call cannot be forcibly interrupted.
func withKeychainInteraction(api *keychainAPI, fn func(*keychainAPI) error) (result error) {
	var previous uint8
	if err := keychainStatus("SecKeychainGetUserInteractionAllowed", api.getInteraction(&previous)); err != nil {
		return err
	}
	if err := keychainStatus("SecKeychainSetUserInteractionAllowed(false)", api.interaction(0)); err != nil {
		return err
	}
	// Restore the caller's prior setting even if the operation panics. The
	// panic is intentionally not recovered here; this defer only restores the
	// process-global Security.framework setting before the panic propagates.
	defer func() {
		restoreErr := keychainStatus("SecKeychainSetUserInteractionAllowed", api.interaction(previous))
		if result == nil {
			result = restoreErr
		}
	}()
	result = fn(api)
	return result
}

func keychainCall(fn func(*keychainAPI) error) (result error) {
	api, err := loadKeychain()
	if err != nil {
		return err
	}
	interactionMu.Lock()
	defer interactionMu.Unlock()
	return withKeychainInteraction(api, fn)
}

func nativeBytes(value string) (unsafe.Pointer, uint32, []byte, error) {
	bytes := []byte(value)
	if uint64(len(bytes)) > uint64(^uint32(0)) {
		return nil, 0, nil, errors.New("keychain field is too long")
	}
	if len(bytes) == 0 {
		return nil, 0, bytes, nil
	}
	return unsafe.Pointer(&bytes[0]), uint32(len(bytes)), bytes, nil
}

func findPassword(api *keychainAPI, key string) (item, data unsafe.Pointer, length uint32, err error) {
	service, serviceLength, serviceBytes, err := nativeBytes(serviceAccount)
	if err != nil {
		return nil, nil, 0, err
	}
	defer clearBytes(serviceBytes)
	account, accountLength, accountBytes, err := nativeBytes(key)
	if err != nil {
		return nil, nil, 0, err
	}
	defer clearBytes(accountBytes)
	status := api.find(nil, serviceLength, service, accountLength, account, &length, &data, &item)
	runtime.KeepAlive(serviceBytes)
	runtime.KeepAlive(accountBytes)
	if err := keychainStatus("SecKeychainFindGenericPassword", status); err != nil {
		return nil, nil, 0, err
	}
	if data == nil && length != 0 {
		_ = releaseItem(api, item, data)
		return nil, nil, 0, errors.New("keychain returned a nil password buffer")
	}
	if length > maxKeychainSecretBytes {
		_ = releaseItem(api, item, data)
		return nil, nil, 0, errors.New("keychain item exceeds the supported size")
	}
	return item, data, length, nil
}

func releaseItem(api *keychainAPI, item, data unsafe.Pointer) error {
	var first error
	if data != nil {
		if err := keychainStatus("SecKeychainItemFreeContent", api.free(nil, data)); err != nil {
			first = err
		}
	}
	if item != nil {
		api.release(item)
	}
	return first
}

func readNative(key string) (string, error) {
	var value string
	err := keychainCall(func(api *keychainAPI) error {
		item, data, length, err := findPassword(api, key)
		if err != nil {
			return err
		}
		defer func() { _ = releaseItem(api, item, data) }()
		secret := append([]byte(nil), unsafe.Slice((*byte)(data), int(length))...)
		value = string(secret)
		clearBytes(secret)
		return nil
	})
	return value, err
}

func writeNative(key, value string) error {
	return keychainCall(func(api *keychainAPI) error {
		item, data, _, err := findPassword(api, key)
		if err == nil {
			defer func() { _ = releaseItem(api, item, data) }()
			return modifyPassword(api, item, value)
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := addPassword(api, key, value); err != errKeychainDuplicate {
			return err
		}
		// A concurrent writer won the add race. Find it again and modify the
		// existing item; the old value remains intact if modification fails.
		item, data, _, err = findPassword(api, key)
		if err != nil {
			return err
		}
		defer func() { _ = releaseItem(api, item, data) }()
		return modifyPassword(api, item, value)
	})
}

func addPassword(api *keychainAPI, key, value string) error {
	service, serviceLength, serviceBytes, err := nativeBytes(serviceAccount)
	if err != nil {
		return err
	}
	defer clearBytes(serviceBytes)
	account, accountLength, accountBytes, err := nativeBytes(key)
	if err != nil {
		return err
	}
	defer clearBytes(accountBytes)
	secret, secretLength, secretBytes, err := nativeBytes(value)
	if err != nil {
		return err
	}
	defer clearBytes(secretBytes)
	var item unsafe.Pointer
	status := api.add(nil, serviceLength, service, accountLength, account, secretLength, secret, &item)
	runtime.KeepAlive(serviceBytes)
	runtime.KeepAlive(accountBytes)
	runtime.KeepAlive(secretBytes)
	if item != nil {
		api.release(item)
	}
	return keychainStatus("SecKeychainAddGenericPassword", status)
}

func modifyPassword(api *keychainAPI, item unsafe.Pointer, value string) error {
	secret, length, secretBytes, err := nativeBytes(value)
	if err != nil {
		return err
	}
	defer clearBytes(secretBytes)
	status := api.modify(item, nil, length, secret)
	runtime.KeepAlive(secretBytes)
	return keychainStatus("SecKeychainItemModifyAttributesAndData", status)
}

func deleteNative(key string) error {
	return keychainCall(func(api *keychainAPI) error {
		item, data, _, err := findPassword(api, key)
		if err != nil {
			return err
		}
		if data != nil {
			if err := keychainStatus("SecKeychainItemFreeContent", api.free(nil, data)); err != nil {
				api.release(item)
				return err
			}
		}
		if err := keychainStatus("SecKeychainItemDelete", api.delete(item)); err != nil {
			api.release(item)
			return err
		}
		api.release(item)
		return nil
	})
}

const serviceAccount = "proxy-start"

func clearBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
