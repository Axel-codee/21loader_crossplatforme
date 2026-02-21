package util

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// WhisperModelCandidateFiles lists known GGML model file names from whisper.cpp.
func WhisperModelCandidateFiles() []string {
	return []string{
		"ggml-tiny.bin",
		"ggml-tiny.en.bin",
		"ggml-base.bin",
		"ggml-base.en.bin",
		"ggml-small.bin",
		"ggml-small.en.bin",
		"ggml-medium.bin",
		"ggml-medium.en.bin",
		"ggml-large-v1.bin",
		"ggml-large-v2.bin",
		"ggml-large-v3.bin",
		"ggml-large-v3-turbo.bin",
	}
}

// WhisperModelSearchDirs returns likely directories containing Whisper models.
// configuredPath can be either a model file path or a directory path.
// whisperExecutable can be an absolute executable path (optional).
func WhisperModelSearchDirs(configuredPath, whisperExecutable string) []string {
	dirs := make([]string, 0, 24)
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath != "" {
		if info, err := os.Stat(configuredPath); err == nil {
			if info.IsDir() {
				dirs = append(dirs, configuredPath)
			} else {
				dirs = append(dirs, filepath.Dir(configuredPath))
			}
		} else if strings.HasSuffix(strings.ToLower(configuredPath), ".bin") {
			dirs = append(dirs, filepath.Dir(configuredPath))
		} else {
			dirs = append(dirs, configuredPath)
		}
	}

	if appBin := PersoDLBinDir(); appBin != "" {
		dirs = append(dirs, filepath.Join(appBin, "models"), appBin)
	}

	dirs = append(dirs, commonWhisperModelDirs()...)

	execPath := strings.TrimSpace(whisperExecutable)
	if execPath == "" || !strings.Contains(execPath, string(filepath.Separator)) {
		if resolved, _, err := ResolveToolExecutable("whisper-cli"); err == nil {
			execPath = resolved
		}
	}
	if execPath != "" && strings.Contains(execPath, string(filepath.Separator)) {
		execDir := filepath.Dir(execPath)
		root := filepath.Dir(execDir)
		dirs = append(dirs,
			filepath.Join(execDir, "models"),
			filepath.Join(root, "models"),
			filepath.Join(root, "share", "whisper", "models"),
			filepath.Join(root, "share", "whisper.cpp", "models"),
		)
	}

	if runtime.GOOS == "darwin" {
		for _, cellarBase := range []string{"/opt/homebrew/Cellar/whisper-cpp", "/usr/local/Cellar/whisper-cpp"} {
			matches, _ := filepath.Glob(filepath.Join(cellarBase, "*", "share", "whisper", "models"))
			dirs = append(dirs, matches...)
		}
	}

	return uniqueCleanPaths(dirs)
}

func commonWhisperModelDirs() []string {
	userHome, _ := os.UserHomeDir()
	userCacheDir, _ := os.UserCacheDir()
	userCacheCandidates := []string{}
	if strings.TrimSpace(userHome) != "" {
		userCacheCandidates = append(userCacheCandidates,
			filepath.Join(userHome, ".cache", "whisper"),
			filepath.Join(userHome, ".cache", "whisper", "models"),
		)
	}
	if strings.TrimSpace(userCacheDir) != "" {
		userCacheCandidates = append(userCacheCandidates,
			filepath.Join(userCacheDir, "whisper"),
			filepath.Join(userCacheDir, "whisper", "models"),
		)
	}

	switch runtime.GOOS {
	case "darwin":
		return append(userCacheCandidates,
			"/opt/homebrew/share/whisper/models",
			"/usr/local/share/whisper/models",
		)
	case "linux":
		return append(userCacheCandidates,
			"/usr/local/share/whisper/models",
			"/usr/share/whisper/models",
		)
	case "windows":
		out := append([]string{}, userCacheCandidates...)
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			out = append(out, filepath.Join(appData, "whisper", "models"))
		}
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			out = append(out, filepath.Join(localAppData, "whisper", "models"))
		}
		return out
	default:
		return userCacheCandidates
	}
}

func uniqueCleanPaths(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		v = filepath.Clean(v)
		key := v
		if runtime.GOOS == "windows" {
			key = strings.ToLower(v)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	return out
}
