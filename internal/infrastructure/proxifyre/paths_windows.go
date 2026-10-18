//go:build windows

package proxifyre

import (
	"os"
	"path/filepath"
)

func defaultEnginePath() string {
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	return filepath.Join(programFiles, "ProxiFyre", "ProxiFyre.exe")
}
