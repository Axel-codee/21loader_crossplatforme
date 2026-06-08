//go:build darwin

package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ApplyPackage(_ context.Context, packagePath string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(packagePath))
	if err != nil {
		return "", err
	}
	if strings.ToLower(filepath.Ext(abs)) != ".dmg" {
		return "", fmt.Errorf("format d'update macOS non supporte: %s", filepath.Ext(abs))
	}
	targetApp := resolveTargetAppPath()
	scriptPath, err := writeMacUpdateScript(abs, targetApp)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("/bin/sh", scriptPath)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("impossible de lancer l'update macOS: %w", err)
	}
	return "Mise a jour macOS lancee. Le .app sera copie puis relance.", nil
}

func resolveTargetAppPath() string {
	if custom := strings.TrimSpace(os.Getenv("LOADER21_APP_PATH")); custom != "" {
		return filepath.Clean(custom)
	}
	if exePath, err := os.Executable(); err == nil {
		clean := filepath.Clean(exePath)
		for dir := filepath.Dir(clean); dir != "/" && dir != "."; dir = filepath.Dir(dir) {
			if strings.HasSuffix(strings.ToLower(filepath.Base(dir)), ".app") {
				return dir
			}
		}
	}
	return "/Applications/21loader.app"
}

func writeMacUpdateScript(dmgPath, targetApp string) (string, error) {
	dir := filepath.Join(os.TempDir(), "21loader-update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	scriptPath := filepath.Join(dir, "apply-macos-update.sh")
	logPath := filepath.Join(dir, "apply-macos-update.log")
	mountPoint := filepath.Join(dir, "mnt")
	script := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		fmt.Sprintf("exec >>%s 2>&1", shellQuote(logPath)),
		"sleep 1",
		fmt.Sprintf("DMG=%s", shellQuote(dmgPath)),
		fmt.Sprintf("TARGET=%s", shellQuote(targetApp)),
		fmt.Sprintf("MOUNT=%s", shellQuote(mountPoint)),
		"rm -rf \"$MOUNT\"",
		"mkdir -p \"$MOUNT\"",
		"hdiutil attach \"$DMG\" -nobrowse -readonly -mountpoint \"$MOUNT\"",
		"cleanup() { hdiutil detach \"$MOUNT\" >/dev/null 2>&1 || true; }",
		"trap cleanup EXIT",
		"APP_SOURCE=$(find \"$MOUNT\" -maxdepth 1 -name '21loader.app' -type d -print -quit)",
		"if [[ -z \"$APP_SOURCE\" ]]; then echo '21loader.app introuvable dans le dmg'; exit 1; fi",
		"mkdir -p \"$(dirname \"$TARGET\")\"",
		"ditto \"$APP_SOURCE\" \"$TARGET\"",
		"cleanup",
		"open \"$TARGET\"",
		"echo 'Update macOS terminee.'",
		"",
	}, "\n")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
