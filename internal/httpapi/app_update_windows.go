//go:build windows

package httpapi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"21loader-cross/internal/core"
)

func triggerLocalAppUpdate(filePath string) (core.AppUpdateResponse, error) {
	updatePackagePath, ext, err := normalizeUpdatePackagePath(filePath)
	if err != nil {
		return core.AppUpdateResponse{}, err
	}

	installDir, launcherPath, err := resolveInstalledAppLayout()
	if err != nil {
		return core.AppUpdateResponse{}, err
	}

	script := windowsUpdateScript(updatePackagePath, ext, installDir, launcherPath)
	if err := startDetachedPowerShell(script); err != nil {
		return core.AppUpdateResponse{}, fmt.Errorf("impossible de lancer la mise a jour: %w", err)
	}

	scheduleCurrentProcessExit(450 * time.Millisecond)

	message := "Mise a jour lancee. L'application va se fermer, appliquer l'update, puis se relancer."
	if ext == ".exe" {
		message = "Installateur lance. L'application va se fermer pour finaliser la mise a jour."
	}
	return core.AppUpdateResponse{
		OK:               true,
		Message:          message,
		RestartScheduled: true,
	}, nil
}

func normalizeUpdatePackagePath(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("fichier de mise a jour requis")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("chemin update invalide: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", fmt.Errorf("fichier introuvable: %w", err)
	}
	if info.IsDir() {
		return "", "", fmt.Errorf("le fichier de mise a jour doit etre un fichier (.exe ou .zip)")
	}
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(abs)))
	if ext != ".exe" && ext != ".zip" {
		return "", "", fmt.Errorf("format non supporte: utilise un .exe (setup) ou un .zip (package portable)")
	}
	return abs, ext, nil
}

func resolveInstalledAppLayout() (string, string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("impossible de determiner l'executable courant: %w", err)
	}
	exeDir := filepath.Dir(filepath.Clean(exePath))
	installDir := exeDir
	if strings.EqualFold(filepath.Base(exeDir), "app") {
		installDir = filepath.Dir(exeDir)
	}
	launcherPath := filepath.Join(installDir, "21loader.cmd")
	if info, statErr := os.Stat(launcherPath); statErr != nil || info.IsDir() {
		return "", "", fmt.Errorf("mise a jour locale indisponible: lance l'application depuis l'installation Windows")
	}
	return installDir, launcherPath, nil
}

func windowsUpdateScript(updatePackagePath, ext, installDir, launcherPath string) string {
	if ext == ".exe" {
		return strings.Join([]string{
			"$ErrorActionPreference='Stop'",
			"Start-Sleep -Milliseconds 1200",
			fmt.Sprintf("Start-Process -FilePath %s", singleQuotedPowerShell(updatePackagePath)),
		}, "; ")
	}

	return strings.Join([]string{
		"$ErrorActionPreference='Stop'",
		fmt.Sprintf("$pkg = %s", singleQuotedPowerShell(updatePackagePath)),
		fmt.Sprintf("$install = %s", singleQuotedPowerShell(installDir)),
		fmt.Sprintf("$launcher = %s", singleQuotedPowerShell(launcherPath)),
		"Start-Sleep -Milliseconds 1200",
		"$tmp = Join-Path $env:TEMP ('21loader-update-' + [Guid]::NewGuid().ToString())",
		"New-Item -ItemType Directory -Path $tmp -Force | Out-Null",
		"Expand-Archive -LiteralPath $pkg -DestinationPath $tmp -Force",
		"$dirs = Get-ChildItem -Path $tmp -Directory | Sort-Object Name",
		"$files = Get-ChildItem -Path $tmp -File",
		"if ($files.Count -eq 0 -and $dirs.Count -eq 1) { $payload = $dirs[0].FullName } else { $payload = $tmp }",
		"Copy-Item -Path (Join-Path $payload '*') -Destination $install -Recurse -Force",
		"Start-Process -FilePath $launcher",
	}, "; ")
}

func startDetachedPowerShell(script string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

func scheduleCurrentProcessExit(delay time.Duration) {
	if delay < 0 {
		delay = 0
	}
	go func() {
		time.Sleep(delay)
		os.Exit(0)
	}()
}
