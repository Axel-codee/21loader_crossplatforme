//go:build darwin

package updater

import (
	"os"
	"strings"
	"testing"
)

func TestMacUpdateScriptStopsOldServerBeforeCopy(t *testing.T) {
	scriptPath, err := writeMacUpdateScript("/tmp/21loader-test.dmg", "/Applications/21loader.app")
	if err != nil {
		t.Fatalf("writeMacUpdateScript failed: %v", err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script failed: %v", err)
	}
	script := string(data)

	stopIndex := strings.Index(script, "21loader-server")
	copyIndex := strings.Index(script, "ditto \"$APP_SOURCE\" \"$TARGET\"")
	if stopIndex < 0 {
		t.Fatalf("expected script to stop 21loader-server, got:\n%s", script)
	}
	if copyIndex < 0 {
		t.Fatalf("expected script to copy app with ditto, got:\n%s", script)
	}
	if stopIndex > copyIndex {
		t.Fatalf("expected server stop before app copy, got:\n%s", script)
	}
}
