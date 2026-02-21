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

// PersoDLBinDir is an app-managed binary directory used for local installs.
func PersoDLBinDir() string {
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

	binDir := PersoDLBinDir()
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
		return []string{name + ".exe", name}
	}
	return []string{name}
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
