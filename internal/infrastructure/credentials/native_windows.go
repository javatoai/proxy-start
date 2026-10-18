//go:build windows

package credentials

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	errorNotFound           = syscall.Errno(1168) // ERROR_NOT_FOUND
	maxCredentialBlobSize   = 512
)

var (
	advapi32   = syscall.NewLazyDLL("Advapi32.dll")
	credRead   = advapi32.NewProc("CredReadW")
	credWrite  = advapi32.NewProc("CredWriteW")
	credDelete = advapi32.NewProc("CredDeleteW")
	credFree   = advapi32.NewProc("CredFree")
)

// credential mirrors the Windows CREDENTIALW structure for generic
// credentials. It is intentionally private; callers only see string values.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func readNative(key string) (string, error) {
	target, err := syscall.UTF16PtrFromString(key)
	if err != nil {
		return "", fmt.Errorf("encode credential key: %w", err)
	}
	var pointer *credential
	result, _, callErr := credRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&pointer)))
	if result == 0 {
		if errno, ok := callErr.(syscall.Errno); ok && errno == errorNotFound {
			return "", ErrNotFound
		}
		if callErr == syscall.Errno(0) {
			callErr = syscall.EINVAL
		}
		return "", fmt.Errorf("CredReadW: %w", callErr)
	}
	if pointer == nil {
		return "", errors.New("CredReadW returned a nil credential")
	}
	defer credFree.Call(uintptr(unsafe.Pointer(pointer)))
	if pointer.CredentialBlobSize > maxCredentialBlobSize {
		return "", errors.New("credential blob is larger than the supported limit")
	}
	if pointer.CredentialBlobSize == 0 || pointer.CredentialBlob == nil {
		return "", nil
	}
	bytes := unsafe.Slice(pointer.CredentialBlob, pointer.CredentialBlobSize)
	copyBytes := append([]byte(nil), bytes...)
	defer clearBytes(copyBytes)
	return string(copyBytes), nil
}

func writeNative(key, value string) error {
	target, err := syscall.UTF16PtrFromString(key)
	if err != nil {
		return fmt.Errorf("encode credential key: %w", err)
	}
	blob := []byte(value)
	if len(blob) > maxCredentialBlobSize {
		return fmt.Errorf("credential value exceeds Windows limit of %d bytes", maxCredentialBlobSize)
	}
	var blobPointer *byte
	if len(blob) > 0 {
		blobPointer = &blob[0]
	}
	credentialValue := credential{
		Type:               credTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     blobPointer,
		Persist:            credPersistLocalMachine,
	}
	result, _, callErr := credWrite.Call(uintptr(unsafe.Pointer(&credentialValue)), 0)
	clearBytes(blob)
	if result == 0 {
		if callErr == syscall.Errno(0) {
			callErr = syscall.EINVAL
		}
		return fmt.Errorf("CredWriteW: %w", callErr)
	}
	return nil
}

func deleteNative(key string) error {
	target, err := syscall.UTF16PtrFromString(key)
	if err != nil {
		return fmt.Errorf("encode credential key: %w", err)
	}
	result, _, callErr := credDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0)
	if result == 0 {
		if errno, ok := callErr.(syscall.Errno); ok && errno == errorNotFound {
			return ErrNotFound
		}
		if callErr == syscall.Errno(0) {
			callErr = syscall.EINVAL
		}
		return fmt.Errorf("CredDeleteW: %w", callErr)
	}
	return nil
}

func clearBytes(bytes []byte) {
	for index := range bytes {
		bytes[index] = 0
	}
}
