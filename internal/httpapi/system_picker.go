package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"21loader-cross/internal/core"
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

func (s *Server) handleSelectFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.SelectFileRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	resp, err := openNativeFileDialog(r.Context(), payload.CurrentPath, payload.Title, payload.Filters)
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

func openNativeFileDialog(ctx context.Context, currentPath, title string, filters []string) (core.SelectFileResponse, error) {
	selectedPath, cancelled, err := pickFileWithNativeDialog(ctx, currentPath, title, filters)
	if err != nil {
		return core.SelectFileResponse{}, err
	}
	return core.SelectFileResponse{
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

func pickFileWithNativeDialog(ctx context.Context, currentPath, title string, filters []string) (string, bool, error) {
	switch runtime.GOOS {
	case "darwin":
		return pickFileDarwin(ctx, currentPath, title)
	case "windows":
		return pickFileWindows(ctx, currentPath, title, filters)
	case "linux":
		return pickFileLinux(ctx, currentPath, title, filters)
	default:
		return "", false, fmt.Errorf("selection native de fichier non supportee sur %s", runtime.GOOS)
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

func pickFileDarwin(ctx context.Context, currentPath, title string) (string, bool, error) {
	prompt := strings.TrimSpace(title)
	if prompt == "" {
		prompt = "Selectionner un fichier"
	}
	trimmed := strings.TrimSpace(currentPath)
	if trimmed != "" {
		clean := filepath.Clean(trimmed)
		defaultLocation := clean
		if info, err := os.Stat(clean); err == nil && !info.IsDir() {
			defaultLocation = filepath.Dir(clean)
		}
		out, err := runCommand(ctx, "osascript",
			"-e", fmt.Sprintf("set defaultFolder to POSIX file %s", strconv.Quote(defaultLocation)),
			"-e", fmt.Sprintf("set pickedFile to choose file with prompt %s default location defaultFolder", strconv.Quote(prompt)),
			"-e", `POSIX path of pickedFile`,
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
	}

	out, err := runCommand(ctx, "osascript",
		"-e", fmt.Sprintf("set pickedFile to choose file with prompt %s", strconv.Quote(prompt)),
		"-e", `POSIX path of pickedFile`,
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

	out, err := runWindowsPowerShellDialog(ctx, strings.Join(script, "; "))
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

func pickFileWindows(ctx context.Context, currentPath, title string, filters []string) (string, bool, error) {
	script := []string{
		`Add-Type -AssemblyName System.Windows.Forms`,
		`$dialog = New-Object System.Windows.Forms.OpenFileDialog`,
		`$dialog.CheckFileExists = $true`,
		`$dialog.Multiselect = $false`,
	}
	if prompt := strings.TrimSpace(title); prompt != "" {
		script = append(script, fmt.Sprintf("$dialog.Title = %s", singleQuotedPowerShell(prompt)))
	} else {
		script = append(script, `$dialog.Title = 'Selectionner un fichier'`)
	}
	if filter := windowsFileDialogFilter(filters); filter != "" {
		script = append(script, fmt.Sprintf("$dialog.Filter = %s", singleQuotedPowerShell(filter)))
	}
	if trimmed := strings.TrimSpace(currentPath); trimmed != "" {
		clean := filepath.Clean(trimmed)
		if info, err := os.Stat(clean); err == nil {
			if info.IsDir() {
				script = append(script, fmt.Sprintf("$dialog.InitialDirectory = %s", singleQuotedPowerShell(clean)))
			} else {
				script = append(script,
					fmt.Sprintf("$dialog.InitialDirectory = %s", singleQuotedPowerShell(filepath.Dir(clean))),
					fmt.Sprintf("$dialog.FileName = %s", singleQuotedPowerShell(filepath.Base(clean))),
				)
			}
		}
	}
	script = append(script,
		`if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $dialog.FileName }`,
	)

	out, err := runWindowsPowerShellDialog(ctx, strings.Join(script, "; "))
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

func pickFileLinux(ctx context.Context, currentPath, title string, _ []string) (string, bool, error) {
	var lastErr error
	prompt := strings.TrimSpace(title)
	if prompt == "" {
		prompt = "Selectionner un fichier"
	}

	if _, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--title=" + prompt}
		if trimmed := strings.TrimSpace(currentPath); trimmed != "" {
			args = append(args, "--filename="+filepath.Clean(trimmed))
		}
		out, runErr := runCommand(ctx, "zenity", args...)
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
		args := []string{"--getopenfilename"}
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

func windowsFileDialogFilter(filters []string) string {
	normalized := make([]string, 0, len(filters))
	for _, raw := range filters {
		v := strings.TrimSpace(strings.ToLower(raw))
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, ".") {
			v = "*" + v
		}
		if !strings.ContainsAny(v, "*?") {
			if strings.HasPrefix(v, "*.") {
				// already ok
			} else if strings.HasPrefix(v, ".") {
				v = "*" + v
			} else {
				v = "*." + strings.TrimPrefix(v, ".")
			}
		}
		normalized = append(normalized, v)
	}
	if len(normalized) == 0 {
		return "Tous les fichiers (*.*)|*.*"
	}
	pattern := strings.Join(normalized, ";")
	return fmt.Sprintf("Fichiers compatibles (%s)|%s|Tous les fichiers (*.*)|*.*", pattern, pattern)
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runWindowsPowerShellDialog(ctx context.Context, script string) (string, error) {
	exe := resolveWindowsPowerShellExecutable()
	args := []string{
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-STA",
		"-Command", script,
	}
	out, err := runCommand(ctx, exe, args...)
	if err == nil {
		return out, nil
	}

	// Fallback for environments where -STA is unsupported by the selected shell.
	fallbackArgs := []string{
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command", script,
	}
	return runCommand(ctx, exe, fallbackArgs...)
}

func resolveWindowsPowerShellExecutable() string {
	candidates := []string{}
	if windir := strings.TrimSpace(os.Getenv("WINDIR")); windir != "" {
		candidates = append(candidates, filepath.Join(windir, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"))
	}
	candidates = append(candidates, "powershell.exe", "powershell", "pwsh.exe", "pwsh")

	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if strings.Contains(candidate, string(filepath.Separator)) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			continue
		}
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return "powershell"
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
