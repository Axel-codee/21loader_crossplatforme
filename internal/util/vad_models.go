package util

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func VADModelCandidateFiles() []string {
	return []string{
		"ggml-silero-v5.1.2.bin",
		"ggml-silero-v6.2.0.bin",
	}
}

func VADModelSearchDirs(configuredPath string) []string {
	dirs := make([]string, 0, 16)
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

	if appBin := Loader21BinDir(); appBin != "" {
		dirs = append(dirs,
			filepath.Join(appBin, "models", "vad"),
			filepath.Join(appBin, "vad-models"),
			filepath.Join(appBin, "models"),
		)
	}

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		dirs = append(dirs,
			filepath.Join(home, ".cache", "whisper-vad"),
			filepath.Join(home, ".cache", "whisper", "vad"),
		)
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
		dirs = append(dirs,
			filepath.Join(cacheDir, "whisper-vad"),
			filepath.Join(cacheDir, "whisper", "vad"),
		)
	}
	if runtime.GOOS == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			dirs = append(dirs, filepath.Join(appData, "whisper-vad"))
		}
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			dirs = append(dirs, filepath.Join(localAppData, "whisper-vad"))
		}
	}

	return uniqueCleanPaths(dirs)
}
