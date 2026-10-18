// Package console contains platform adapters for opening an interactive shell
// with a proxy environment scoped to that shell and its child processes.
package console

import (
	"os"
	"strings"
)

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func childEnvironment(overrides map[string]string, unset []string) []string {
	result := append([]string(nil), os.Environ()...)
	for key, value := range overrides {
		result = setEnvironmentValue(result, key, value)
	}
	for _, key := range unset {
		if strings.TrimSpace(key) == "" {
			continue
		}
		filtered := result[:0]
		for _, item := range result {
			index := strings.IndexByte(item, '=')
			if index < 0 || !strings.EqualFold(item[:index], key) {
				filtered = append(filtered, item)
			}
		}
		result = filtered
	}
	return result
}

func setEnvironmentValue(environment []string, key, value string) []string {
	for i, item := range environment {
		index := strings.IndexByte(item, '=')
		if index >= 0 && strings.EqualFold(item[:index], key) {
			environment[i] = key + "=" + value
			return environment
		}
	}
	return append(environment, key+"="+value)
}
