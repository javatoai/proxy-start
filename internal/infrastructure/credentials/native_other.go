//go:build !windows && !darwin

package credentials

func readNative(string) (string, error) { return "", ErrUnsupported }

func writeNative(string, string) error { return ErrUnsupported }

func deleteNative(string) error { return ErrUnsupported }
