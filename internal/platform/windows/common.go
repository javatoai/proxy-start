package windows

import (
	"path/filepath"
	"strings"
)

// splitWindowsCommandLine converts the command-line argument string exposed by
// a Windows shortcut into the same argument boundaries used by the Windows C
// runtime. It deliberately does not perform environment expansion.
func splitWindowsCommandLine(line string) []string {
	var args []string
	for i := 0; i < len(line); {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}

		var b strings.Builder
		inQuotes := false
		started := false
		for i < len(line) {
			c := line[i]
			switch c {
			case ' ', '\t':
				if !inQuotes {
					i++
					goto argumentDone
				}
				b.WriteByte(c)
				started = true
				i++
			case '\\':
				// Backslashes only acquire special meaning immediately before a
				// quote. Keep a run until the next byte so the CRT's 2n/2n+1
				// rule can be applied exactly.
				start := i
				for i < len(line) && line[i] == '\\' {
					i++
				}
				run := i - start
				if i < len(line) && line[i] == '"' {
					for j := 0; j < run/2; j++ {
						b.WriteByte('\\')
					}
					if run%2 == 1 {
						b.WriteByte('"')
						i++
					} else {
						// An even run leaves the quote syntactic.
						inQuotes = !inQuotes
						i++
						started = true
					}
				} else {
					for j := 0; j < run; j++ {
						b.WriteByte('\\')
					}
					started = true
				}
			case '"':
				// A doubled quote inside a quoted region is a literal quote.
				if inQuotes && i+1 < len(line) && line[i+1] == '"' {
					b.WriteByte('"')
					i += 2
				} else {
					inQuotes = !inQuotes
					i++
					started = true
				}
			default:
				b.WriteByte(c)
				started = true
				i++
			}
		}
	argumentDone:
		if started || b.Len() > 0 {
			args = append(args, b.String())
		}
	}
	return args
}

func canonicalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func samePath(a, b string) bool {
	a, b = canonicalPath(a), canonicalPath(b)
	return a != "" && b != "" && strings.EqualFold(a, b)
}
