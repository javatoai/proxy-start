//go:build windows

package proxifyre

import (
	"strings"
	"testing"
)

func TestHelperLaunchArgumentsRoundTripWindowsPaths(t *testing.T) {
	pipe := `\\.\pipe\proxy-start-abc 123`
	engine := `C:\Program Files\ProxiFyre\ProxiFyre.exe`
	service := `Proxy Start Service`
	args := helperLaunchArguments(pipe, "token", engine, service)
	got := parseWindowsCommandLine(args)
	want := []string{"--pipe", pipe, "--token", "token", "--engine", engine, "--service", service}
	if len(got) != len(want) {
		t.Fatalf("argument count: got %d want %d (%q)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argument %d: got %q want %q", i, got[i], want[i])
		}
	}
}

// parseWindowsCommandLine mirrors the CommandLineToArgvW quoting rules for
// the argument shapes emitted by helperLaunchArguments. Keeping this parser in
// the test avoids spawning an elevated process just to verify path escaping.
func parseWindowsCommandLine(command string) []string {
	var result []string
	var value strings.Builder
	inQuote := false
	backslashes := 0
	flush := func() {
		if value.Len() > 0 || len(result) == 0 {
			result = append(result, value.String())
			value.Reset()
		}
	}
	for i := 0; i < len(command); i++ {
		switch command[i] {
		case '\\':
			backslashes++
		case '"':
			if backslashes%2 == 0 {
				for n := 0; n < backslashes/2; n++ {
					value.WriteByte('\\')
				}
				inQuote = !inQuote
			} else {
				for n := 0; n < backslashes/2; n++ {
					value.WriteByte('\\')
				}
				value.WriteByte('"')
			}
			backslashes = 0
		case ' ', '\t':
			for n := 0; n < backslashes; n++ {
				value.WriteByte('\\')
			}
			backslashes = 0
			if !inQuote {
				flush()
			} else {
				value.WriteByte(command[i])
			}
		default:
			for n := 0; n < backslashes; n++ {
				value.WriteByte('\\')
			}
			backslashes = 0
			value.WriteByte(command[i])
		}
	}
	for n := 0; n < backslashes; n++ {
		value.WriteByte('\\')
	}
	flush()
	return result
}
