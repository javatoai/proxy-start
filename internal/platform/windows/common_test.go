package windows

import (
	"reflect"
	"testing"
)

func TestSplitWindowsCommandLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{name: "quoted path", line: `--profile "C:\Program Files\Codex"`, want: []string{"--profile", `C:\Program Files\Codex`}},
		{name: "escaped quote", line: `"a\\\"b" c`, want: []string{`a\"b`, "c"}},
		{name: "empty argument", line: `"" x`, want: []string{"", "x"}},
		{name: "trailing slash", line: `"C:\tmp\\"`, want: []string{`C:\tmp\`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitWindowsCommandLine(tt.line); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitWindowsCommandLine(%q) = %#v, want %#v", tt.line, got, tt.want)
			}
		})
	}
}
