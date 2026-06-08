package util

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const extraPathEnv = "LOADER21_EXTRA_PATH"

func EnsureRuntimeSearchPath() {
	current := os.Getenv("PATH")
	expanded := ExpandedRuntimeSearchPath(current)
	if expanded != current {
		_ = os.Setenv("PATH", expanded)
	}
}

func ExpandedRuntimeSearchPath(current string) string {
	entries := splitPathList(current)
	entries = append(entries, splitPathList(os.Getenv(extraPathEnv))...)
	entries = append(entries, RuntimeSearchDirs()...)
	return strings.Join(uniquePathEntries(entries), string(os.PathListSeparator))
}

func RuntimeSearchDirs() []string {
	dirs := []string{}
	if binDir := Loader21BinDir(); binDir != "" {
		dirs = append(dirs, binDir)
	}

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "Library", "Python", "3.13", "bin"),
			filepath.Join(home, "Library", "Python", "3.12", "bin"),
			filepath.Join(home, "Library", "Python", "3.11", "bin"),
		)
	}

	if runtime.GOOS == "darwin" {
		dirs = append(dirs,
			"/opt/homebrew/bin",
			"/opt/homebrew/sbin",
			"/usr/local/bin",
			"/usr/local/sbin",
		)
	}

	dirs = append(dirs,
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
	)
	return uniquePathEntries(dirs)
}

func splitPathList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(value, string(os.PathListSeparator))
}

func uniquePathEntries(entries []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, raw := range entries {
		entry := filepath.Clean(strings.TrimSpace(raw))
		if entry == "." || entry == "" {
			continue
		}
		key := entry
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, entry)
	}
	return out
}
