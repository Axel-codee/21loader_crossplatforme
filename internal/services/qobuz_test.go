package services

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"21loader-cross/internal/sys"
)

func TestExtractQobuzError(t *testing.T) {
	output := "Logging...\n" + qobuzErrorMarker + "Impossible de joindre Qobuz. Verifie la connexion Internet puis reessaie.\n"

	got := extractQobuzError(output)
	want := "Impossible de joindre Qobuz. Verifie la connexion Internet puis reessaie."
	if got != want {
		t.Fatalf("extractQobuzError() = %q, want %q", got, want)
	}
}

func TestWrapQobuzProcessErrorPrefersScriptMessage(t *testing.T) {
	processErr := sys.ProcessError{Command: "python qobuz_artist_search.py toto 20", Status: 1}
	output := "Logging...\n" + qobuzErrorMarker + "Impossible de joindre Qobuz. Verifie la connexion Internet puis reessaie.\n"

	err := wrapQobuzProcessError("la recherche d'artistes Qobuz a echoue", output, processErr)
	if err == nil {
		t.Fatal("wrapQobuzProcessError() returned nil error")
	}

	got := err.Error()
	want := "la recherche d'artistes Qobuz a echoue: Impossible de joindre Qobuz. Verifie la connexion Internet puis reessaie."
	if got != want {
		t.Fatalf("wrapQobuzProcessError() = %q, want %q", got, want)
	}
}

func TestWrapQobuzProcessErrorFallsBackToOriginalError(t *testing.T) {
	processErr := errors.New("la commande a echoue (1): python qobuz_artist_search.py toto 20")

	err := wrapQobuzProcessError("la recherche d'artistes Qobuz a echoue", "Logging...\n", processErr)
	if err == nil {
		t.Fatal("wrapQobuzProcessError() returned nil error")
	}

	got := err.Error()
	want := "la recherche d'artistes Qobuz a echoue: la commande a echoue (1): python qobuz_artist_search.py toto 20"
	if got != want {
		t.Fatalf("wrapQobuzProcessError() = %q, want %q", got, want)
	}
}

func TestQobuzScriptEnvironment(t *testing.T) {
	env := qobuzScriptEnvironment(" user@example.com ", "secret-pass", " session.token.value ", true)
	if env == nil {
		t.Fatal("qobuzScriptEnvironment() returned nil")
	}
	if got, want := env[qobuzEmailEnv], "user@example.com"; got != want {
		t.Fatalf("unexpected email override: got=%q want=%q", got, want)
	}
	if got, want := env[qobuzPasswordRawEnv], "secret-pass"; got != want {
		t.Fatalf("unexpected raw password override: got=%q want=%q", got, want)
	}
	if got, want := env[qobuzPasswordMD5Env], "591fac3e56ffbdc6f310c1b646050c09"; got != want {
		t.Fatalf("unexpected password hash: got=%q want=%q", got, want)
	}
	if got, want := env[qobuzUserAuthTokenEnv], "session.token.value"; got != want {
		t.Fatalf("unexpected token override: got=%q want=%q", got, want)
	}
	if _, ok := env[qobuzDisableTokenAuthEnv]; ok {
		t.Fatalf("token auth should stay enabled when workaround is active")
	}
}

func TestQobuzScriptEnvironmentDisablesTokenWhenModeOff(t *testing.T) {
	env := qobuzScriptEnvironment(" user@example.com ", "secret-pass", " session.token.value ", false)
	if env == nil {
		t.Fatal("qobuzScriptEnvironment() returned nil")
	}
	if got, want := env[qobuzDisableTokenAuthEnv], "1"; got != want {
		t.Fatalf("unexpected token disable flag: got=%q want=%q", got, want)
	}
	if _, ok := env[qobuzUserAuthTokenEnv]; ok {
		t.Fatalf("token override should be absent when workaround is disabled")
	}
}

func TestQobuzScriptsUseSharedLoadClient(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	scriptNames := []string{
		"qobuz_artist_search.py",
		"qobuz_artist_catalog.py",
		"qobuz_album_tracks.py",
		"qobuz_playlist_catalog.py",
		"qobuz_auth_check.py",
	}

	for _, scriptName := range scriptNames {
		scriptPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "assets", "scripts", scriptName)
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("os.ReadFile(%q) error = %v", scriptPath, err)
		}

		text := string(content)
		if !strings.Contains(text, "from qobuz_common import load_client, run_with_qobuz_error_handling") {
			t.Fatalf("%s should import load_client from qobuz_common", scriptName)
		}
		if strings.Contains(text, "\ndef load_client(") {
			t.Fatalf("%s should not redefine load_client()", scriptName)
		}
	}
}
