package util

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ToolBinaryCandidates returns executable names accepted for a logical tool.
func ToolBinaryCandidates(tool string) []string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "whisper-cli":
		return []string{"whisper-cli", "whisper-cpp"}
	default:
		return []string{strings.TrimSpace(tool)}
	}
}

// Loader21BinDir is an app-managed binary directory used for local installs.
func Loader21BinDir() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cfg, AppName, "bin")
}

// ResolveToolExecutable resolves a logical tool to an executable path.
func ResolveToolExecutable(tool string) (path string, resolvedName string, err error) {
	return resolveExecutableCandidates(ToolBinaryCandidates(tool))
}

func resolveExecutableCandidates(candidates []string) (path string, resolvedName string, err error) {
	ordered := uniqueNonEmpty(candidates)
	for _, candidate := range ordered {
		if p, lookErr := exec.LookPath(candidate); lookErr == nil {
			return p, candidate, nil
		}
	}

	binDir := Loader21BinDir()
	if binDir != "" {
		for _, candidate := range ordered {
			for _, fileName := range candidateFileNames(candidate) {
				p := filepath.Join(binDir, fileName)
				if isExecutableFile(p) {
					return p, candidate, nil
				}
			}
		}
	}

	if runtime.GOOS == "darwin" {
		for _, candidate := range ordered {
			for _, dir := range RuntimeSearchDirs() {
				for _, fileName := range candidateFileNames(candidate) {
					p := filepath.Join(dir, fileName)
					if isExecutableFile(p) {
						return p, candidate, nil
					}
				}
			}
		}
	}

	// Windows users frequently install qobuz-dl with pipx without adding
	// pipx app directories to PATH. Probe common user-level locations.
	if runtime.GOOS == "windows" {
		for _, candidate := range ordered {
			for _, dir := range windowsToolSearchDirs(candidate) {
				for _, fileName := range candidateFileNames(candidate) {
					p := filepath.Join(dir, fileName)
					if isExecutableFile(p) {
						return p, candidate, nil
					}
				}
			}
		}
	}

	return "", "", errors.New("binaire introuvable")
}

func uniqueNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	return out
}

func candidateFileNames(name string) []string {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		return []string{name + ".exe", name + ".cmd", name + ".bat", name}
	}
	return []string{name}
}

func windowsToolSearchDirs(tool string) []string {
	out := make([]string, 0, 6)

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		out = append(out, filepath.Join(home, ".local", "bin"))
		trimmedTool := strings.TrimSpace(tool)
		if trimmedTool != "" {
			out = append(out, filepath.Join(home, "pipx", "venvs", trimmedTool, "Scripts"))
		}
	}

	if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
		out = append(out, filepath.Join(localAppData, "Programs", "Python", "Scripts"))
	}
	if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
		out = append(out, filepath.Join(appData, "Python", "Scripts"))
	}

	return uniqueNonEmpty(out)
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}
