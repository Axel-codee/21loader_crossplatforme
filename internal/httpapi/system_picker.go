package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"persodl-cross/internal/core"
)

func (s *Server) handleSelectDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.SelectDirectoryRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	resp, err := openNativeDirectoryDialog(r.Context(), payload.CurrentPath)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func openNativeDirectoryDialog(ctx context.Context, currentPath string) (core.SelectDirectoryResponse, error) {
	selectedPath, cancelled, err := pickDirectoryWithNativeDialog(ctx, currentPath)
	if err != nil {
		return core.SelectDirectoryResponse{}, err
	}
	return core.SelectDirectoryResponse{
		Path:      selectedPath,
		Cancelled: cancelled,
	}, nil
}

func pickDirectoryWithNativeDialog(ctx context.Context, currentPath string) (string, bool, error) {
	switch runtime.GOOS {
	case "darwin":
		return pickDirectoryDarwin(ctx, currentPath)
	case "windows":
		return pickDirectoryWindows(ctx, currentPath)
	case "linux":
		return pickDirectoryLinux(ctx, currentPath)
	default:
		return "", false, fmt.Errorf("selection native de dossier non supportee sur %s", runtime.GOOS)
	}
}

func pickDirectoryDarwin(ctx context.Context, currentPath string) (string, bool, error) {
	trimmed := strings.TrimSpace(currentPath)
	if trimmed != "" {
		clean := filepath.Clean(trimmed)
		out, err := runCommand(ctx, "osascript",
			"-e", fmt.Sprintf("set defaultFolder to POSIX file %s", strconv.Quote(clean)),
			"-e", `set pickedFolder to choose folder with prompt "Selectionner le dossier de sortie" default location defaultFolder`,
			"-e", `POSIX path of pickedFolder`,
		)
		if err == nil {
			if path := normalizeSelectedPath(out); path != "" {
				return path, false, nil
			}
			return "", true, nil
		}
		if isCommandCancelled(err, out) {
			return "", true, nil
		}
		// fallback sans dossier initial quand le chemin passe n'existe pas
	}

	out, err := runCommand(ctx, "osascript",
		"-e", `set pickedFolder to choose folder with prompt "Selectionner le dossier de sortie"`,
		"-e", `POSIX path of pickedFolder`,
	)
	if err != nil {
		if isCommandCancelled(err, out) {
			return "", true, nil
		}
		return "", false, fmt.Errorf("impossible d'ouvrir le selecteur macOS: %s", commandErrorDetail(err, out))
	}
	if path := normalizeSelectedPath(out); path != "" {
		return path, false, nil
	}
	return "", true, nil
}

func pickDirectoryWindows(ctx context.Context, currentPath string) (string, bool, error) {
	script := []string{
		`Add-Type -AssemblyName System.Windows.Forms`,
		`$dialog = New-Object System.Windows.Forms.FolderBrowserDialog`,
		`$dialog.Description = 'Selectionner le dossier de sortie'`,
		`$dialog.ShowNewFolderButton = $true`,
	}
	if trimmed := strings.TrimSpace(currentPath); trimmed != "" {
		script = append(script, fmt.Sprintf("$dialog.SelectedPath = %s", singleQuotedPowerShell(filepath.Clean(trimmed))))
	}
	script = append(script,
		`if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $dialog.SelectedPath }`,
	)

	out, err := runCommand(ctx, "powershell",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command", strings.Join(script, "; "),
	)
	if err != nil {
		if isCommandCancelled(err, out) {
			return "", true, nil
		}
		return "", false, fmt.Errorf("impossible d'ouvrir le selecteur Windows: %s", commandErrorDetail(err, out))
	}
	if path := normalizeSelectedPath(out); path != "" {
		return path, false, nil
	}
	return "", true, nil
}

func pickDirectoryLinux(ctx context.Context, currentPath string) (string, bool, error) {
	var lastErr error

	if _, err := exec.LookPath("zenity"); err == nil {
		out, runErr := runCommand(ctx, "zenity", "--file-selection", "--directory", "--title=Selection du dossier de sortie")
		if runErr == nil {
			if path := normalizeSelectedPath(out); path != "" {
				return path, false, nil
			}
			return "", true, nil
		}
		if isCommandCancelled(runErr, out) {
			return "", true, nil
		}
		lastErr = fmt.Errorf("zenity: %s", commandErrorDetail(runErr, out))
	}

	if _, err := exec.LookPath("kdialog"); err == nil {
		args := []string{"--getexistingdirectory"}
		if trimmed := strings.TrimSpace(currentPath); trimmed != "" {
			args = append(args, filepath.Clean(trimmed))
		}
		out, runErr := runCommand(ctx, "kdialog", args...)
		if runErr == nil {
			if path := normalizeSelectedPath(out); path != "" {
				return path, false, nil
			}
			return "", true, nil
		}
		if isCommandCancelled(runErr, out) {
			return "", true, nil
		}
		lastErr = fmt.Errorf("kdialog: %s", commandErrorDetail(runErr, out))
	}

	if lastErr != nil {
		return "", false, fmt.Errorf("impossible d'ouvrir le selecteur Linux: %w", lastErr)
	}
	return "", false, fmt.Errorf("aucun selecteur natif disponible (installe zenity ou kdialog)")
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func normalizeSelectedPath(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	v = strings.Trim(v, "\"")
	lines := strings.Split(v, "\n")
	if len(lines) == 0 {
		return ""
	}
	cleaned := strings.TrimSpace(lines[len(lines)-1])
	if cleaned == "" {
		return ""
	}
	return filepath.Clean(cleaned)
}

func isCommandCancelled(err error, output string) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status := exitErr.ExitCode()
	lower := strings.ToLower(strings.TrimSpace(output + "\n" + err.Error()))
	if strings.Contains(lower, "user canceled") || strings.Contains(lower, "user cancelled") || strings.Contains(lower, "was canceled") || strings.Contains(lower, "was cancelled") || strings.Contains(lower, "annule") {
		return true
	}
	if status == 130 {
		return true
	}
	if status == 1 && strings.TrimSpace(output) == "" {
		return true
	}
	return false
}

func commandErrorDetail(err error, output string) string {
	text := strings.TrimSpace(output)
	if text != "" {
		return text
	}
	return err.Error()
}

func singleQuotedPowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
