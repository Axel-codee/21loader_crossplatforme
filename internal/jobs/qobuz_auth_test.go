package jobs

import (
	"strings"
	"testing"
)

func TestOverrideQobuzConfigCredentials(t *testing.T) {
	input := strings.Join([]string{
		"[DEFAULT]",
		"email = old@example.com",
		"password = oldhash",
		"app_id = 123456",
		"secrets = abc,def",
		"",
	}, "\n")

	got := overrideQobuzConfigCredentials(input, "new@example.com", "newhash")
	if !strings.Contains(got, "email = new@example.com") {
		t.Fatalf("expected updated email in config, got: %q", got)
	}
	if !strings.Contains(got, "password = newhash") {
		t.Fatalf("expected updated password hash in config, got: %q", got)
	}
	if strings.Contains(got, "old@example.com") || strings.Contains(got, "oldhash") {
		t.Fatalf("stale credentials still present in config: %q", got)
	}
}
