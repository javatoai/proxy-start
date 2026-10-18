package darwin

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func enclosingBundle(path string) string {
	path = filepath.Clean(path)
	for current := path; current != "." && current != string(filepath.Separator); current = filepath.Dir(current) {
		if strings.HasSuffix(strings.ToLower(current), ".app") {
			return current
		}
	}
	return ""
}

func cleanExecutable(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

func sameExecutablePath(a, b string) bool {
	a, b = cleanExecutable(a), cleanExecutable(b)
	return a != "" && b != "" && strings.EqualFold(a, b)
}

func mergedEnvironment(overrides map[string]string, unset []string) []string {
	values := make(map[string]string)
	for _, item := range os.Environ() {
		if key, value, ok := strings.Cut(item, "="); ok {
			values[key] = value
		}
	}
	for _, key := range unset {
		delete(values, key)
	}
	for key, value := range overrides {
		values[key] = value
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

func localeEnvironment() []string {
	result := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if key, _, ok := strings.Cut(item, "="); ok && strings.EqualFold(key, "LC_ALL") {
			continue
		}
		result = append(result, item)
	}
	return append(result, "LC_ALL=C")
}

// bundleExecutablePath accepts only a leaf executable name under
// Contents/MacOS. A plist-controlled absolute path or traversal cannot escape
// the signed bundle's executable directory.
func bundleExecutablePath(bundle, executableName string) (string, bool) {
	name := strings.TrimSpace(executableName)
	if name == "" || filepath.IsAbs(name) || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return "", false
	}
	root := filepath.Join(bundle, "Contents", "MacOS")
	path := filepath.Join(root, name)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return path, true
}
