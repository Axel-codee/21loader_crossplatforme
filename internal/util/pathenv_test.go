package util

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestExpandedRuntimeSearchPathKeepsExistingEntriesAndAddsCommonDirs(t *testing.T) {
	t.Setenv(extraPathEnv, strings.Join([]string{"/custom/bin", "/custom/bin"}, string(os.PathListSeparator)))

	path := ExpandedRuntimeSearchPath(strings.Join([]string{"/existing/bin", "/usr/bin"}, string(os.PathListSeparator)))
	parts := strings.Split(path, string(os.PathListSeparator))

	if len(parts) < 2 || parts[0] != "/existing/bin" || parts[1] != "/usr/bin" {
		t.Fatalf("existing path order was not preserved: %q", path)
	}
	if countPathEntry(parts, "/custom/bin") != 1 {
		t.Fatalf("expected custom path once, got: %q", path)
	}
	if countPathEntry(parts, "/bin") != 1 {
		t.Fatalf("expected system fallback path once, got: %q", path)
	}
}

func TestRuntimeSearchDirsIncludesMacOSHomebrewDirs(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-only search path expectation")
	}

	dirs := RuntimeSearchDirs()
	for _, want := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		if countPathEntry(dirs, want) != 1 {
			t.Fatalf("expected %s once, got: %q", want, dirs)
		}
	}
}

func countPathEntry(parts []string, want string) int {
	count := 0
	for _, part := range parts {
		if part == want {
			count++
		}
	}
	return count
}
