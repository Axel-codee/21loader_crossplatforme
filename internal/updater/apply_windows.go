//go:build windows

package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func ApplyPackage(_ context.Context, packagePath string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(packagePath))
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(abs))
	switch ext {
	case ".exe":
		cmd := exec.Command("cmd", "/C", "start", "", abs)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("impossible de lancer l'installateur: %w", err)
		}
		return "Installateur lance. Ferme 21loader si une ancienne instance est encore ouverte.", nil
	case ".zip":
		installDir, launcherPath, err := resolveInstalledLayout()
		if err != nil {
			return "", err
		}
		script := windowsZipUpdateScript(abs, installDir, launcherPath)
		cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("impossible de lancer l'update zip: %w", err)
		}
		return "Mise a jour zip lancee. 21loader sera relance apres copie.", nil
	default:
		return "", fmt.Errorf("format d'update Windows non supporte: %s", ext)
	}
}

func resolveInstalledLayout() (string, string, error) {
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
		return "", "", fmt.Errorf("mise a jour zip indisponible: lance 21loader depuis son installation Windows")
	}
	return installDir, launcherPath, nil
}

func windowsZipUpdateScript(updatePackagePath, installDir, launcherPath string) string {
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

func singleQuotedPowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
