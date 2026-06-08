package util

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PyannoteVenvDirectory returns the app-managed virtual environment directory
// used for pyannote.audio.
func PyannoteVenvDirectory() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cfg, AppName, "pyannote-venv")
}

// PyannoteVenvPythonCandidates returns candidate python executables inside the
// app-managed pyannote virtual environment.
func PyannoteVenvPythonCandidates(venvDir string) []string {
	venvDir = strings.TrimSpace(venvDir)
	if venvDir == "" {
		venvDir = PyannoteVenvDirectory()
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
