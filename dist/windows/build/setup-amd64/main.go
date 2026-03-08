package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed payload.zip
var payload []byte

const (
	appName             = "21loader"
	launcherFileName    = "21loader.cmd"
	serverBinaryName    = "21loader-server.exe"
	startMenuFolderName = "21loader"
)

func main() {
	installDir := resolveInstallDir()
	if err := extractPayload(installDir); err != nil {
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		os.Exit(1)
	}

	if err := ensureShellIntegration(installDir); err != nil {
		fmt.Fprintf(os.Stderr, "Post-install setup warning: %v\n", err)
	}

	launcher := filepath.Join(installDir, launcherFileName)
	_ = exec.Command("cmd", "/C", "start", "", launcher).Start()
	fmt.Printf("Installed in: %s\n", installDir)
}

func resolveInstallDir() string {
	if custom := strings.TrimSpace(os.Getenv("LOADER21_INSTALL_DIR")); custom != "" {
		return filepath.Clean(custom)
	}
	local := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if local == "" {
		home, _ := os.UserHomeDir()
		local = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(local, "Programs", appName)
}

func extractPayload(installDir string) error {
	if strings.TrimSpace(installDir) == "" {
		return errors.New("empty install directory")
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return err
	}

	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return err
	}

	root := filepath.Clean(installDir)
	rootPrefix := root + string(os.PathSeparator)

	for _, file := range reader.File {
		name := strings.TrimSpace(file.Name)
		if name == "" {
			continue
		}

		targetPath := filepath.Clean(filepath.Join(root, name))
		if targetPath != root && !strings.HasPrefix(targetPath, rootPrefix) {
			return fmt.Errorf("invalid archive path: %s", name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}

		in, err := file.Open()
		if err != nil {
			return err
		}

		mode := os.FileMode(0o644)
		if file.Mode()&0o111 != 0 {
			mode = 0o755
		}
		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			_ = in.Close()
			return err
		}

		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		inCloseErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if inCloseErr != nil {
			return inCloseErr
		}
	}

	return nil
}

func ensureShellIntegration(installDir string) error {
	launcherPath := filepath.Join(installDir, launcherFileName)
	if _, err := os.Stat(launcherPath); err != nil {
		return fmt.Errorf("launcher not found: %w", err)
	}

	appData := strings.TrimSpace(os.Getenv("APPDATA"))
	if appData == "" {
		return fmt.Errorf("APPDATA unavailable, cannot create Start Menu entry")
	}

	startMenuPrograms := filepath.Join(
		appData,
		"Microsoft",
		"Windows",
		"Start Menu",
		"Programs",
		startMenuFolderName,
	)
	if err := os.MkdirAll(startMenuPrograms, 0o755); err != nil {
		return fmt.Errorf("cannot create Start Menu folder: %w", err)
	}

	iconPath := resolveIconPath(installDir, launcherPath)
	shortcutPath := filepath.Join(startMenuPrograms, appName+".lnk")
	if err := createWindowsShortcut(shortcutPath, launcherPath, installDir, iconPath, appName); err != nil {
		return err
	}
	return nil
}

func resolveIconPath(installDir, launcherPath string) string {
	candidates := []string{
		filepath.Join(installDir, "app", "assets", "windows", "21loader.ico"),
		filepath.Join(installDir, "app", serverBinaryName),
		launcherPath,
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return launcherPath
}

func createWindowsShortcut(shortcutPath, targetPath, workingDir, iconPath, description string) error {
	script := strings.Join([]string{
		"$ErrorActionPreference='Stop'",
		"$shell = New-Object -ComObject WScript.Shell",
		fmt.Sprintf("$shortcut = $shell.CreateShortcut(%s)", singleQuotedPowerShell(shortcutPath)),
		fmt.Sprintf("$shortcut.TargetPath = %s", singleQuotedPowerShell(targetPath)),
		fmt.Sprintf("$shortcut.WorkingDirectory = %s", singleQuotedPowerShell(workingDir)),
		fmt.Sprintf("$shortcut.Description = %s", singleQuotedPowerShell(description)),
		fmt.Sprintf("$shortcut.IconLocation = %s", singleQuotedPowerShell(iconPath+",0")),
		"$shortcut.Save()",
	}, "; ")

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("cannot create Start Menu shortcut: %s", detail)
	}
	return nil
}

func singleQuotedPowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
