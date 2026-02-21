package util

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ArgosVenvDirectory returns the app-managed virtual environment directory
// used for argostranslate.
func ArgosVenvDirectory() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cfg, AppName, "argostranslate-venv")
}

// ArgosVenvPythonCandidates returns candidate python executables inside the
// app-managed Argos virtual environment.
func ArgosVenvPythonCandidates(venvDir string) []string {
	venvDir = strings.TrimSpace(venvDir)
	if venvDir == "" {
		venvDir = ArgosVenvDirectory()
	}
	if venvDir == "" {
		return nil
	}

	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(venvDir, "Scripts", "python.exe"),
			filepath.Join(venvDir, "Scripts", "python"),
		}
	}

	return []string{
		filepath.Join(venvDir, "bin", "python3"),
		filepath.Join(venvDir, "bin", "python"),
	}
}
