package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"persodl-cross/internal/core"
	"persodl-cross/internal/sys"
	"persodl-cross/internal/util"
)

var dependencyTools = []string{"yt-dlp", "ffmpeg", "qobuz-dl", "whisper-cli", "argostranslate"}

const dependencyInstallProgressLogsLimit = 96 * 1024

type DiagnosticsService struct {
	runner *sys.Runner

	installMu sync.Mutex

	progressMu      sync.RWMutex
	installProgress core.DependencyInstallProgressResponse
	installLogsTail string
}

func NewDiagnosticsService(r *sys.Runner) *DiagnosticsService {
	return &DiagnosticsService{
		runner: r,
		installProgress: core.DependencyInstallProgressResponse{
			Active: false,
			Stage:  "idle",
		},
	}
}

func (s *DiagnosticsService) CollectReport(ctx context.Context) core.WebDiagnosticsReport {
	pm := detectPackageManager()
	brew := inspectBinary(ctx, s.runner, "brew", []string{"--version"})
	ytDlp := inspectBinary(ctx, s.runner, "yt-dlp", []string{"--version"})
	ffmpeg := inspectBinary(ctx, s.runner, "ffmpeg", []string{"-version"})
	whisper := inspectBinary(ctx, s.runner, "whisper-cli", []string{"--version"})
	qobuz := inspectBinary(ctx, s.runner, "qobuz-dl", []string{"--help"})
	argos := inspectArgosTranslate(ctx, s.runner)

	tools := []core.WebBinaryDiagnostic{ytDlp, ffmpeg, whisper, qobuz, argos}
	outdated := s.detectOutdatedTools(ctx, pm)
	for i := range tools {
		if tools[i].Available && outdated[tools[i].Name] {
			tools[i].NeedsUpdate = true
		}
	}

	if runtime.GOOS != "darwin" {
		brew.Notes = "Homebrew non utilise sur cette plateforme"
	}

	return core.WebDiagnosticsReport{
		CollectedAt:    time.Now().UTC(),
		Platform:       runtime.GOOS,
		PackageManager: pm.Name,
		Brew:           brew,
		Tools:          tools,
	}
}

func (s *DiagnosticsService) InstallProgress() core.DependencyInstallProgressResponse {
	s.progressMu.RLock()
	defer s.progressMu.RUnlock()

	progress := s.installProgress
	if strings.TrimSpace(progress.Stage) == "" {
		progress.Stage = "idle"
	}
	progress.Logs = s.installLogsTail
	return progress
}

func (s *DiagnosticsService) resetInstallProgress() {
	now := time.Now().UTC()
	s.progressMu.Lock()
	s.installLogsTail = ""
	s.installProgress = core.DependencyInstallProgressResponse{
		Active:    true,
		Stage:     "preparing",
		Message:   "Preparation de l'installation...",
		StartedAt: now,
		UpdatedAt: now,
	}
	s.progressMu.Unlock()
}

func (s *DiagnosticsService) setInstallProgress(active bool, stage, tool, action, command, message string) {
	now := time.Now().UTC()

	s.progressMu.Lock()
	defer s.progressMu.Unlock()

	s.installProgress.Active = active
	if strings.TrimSpace(stage) != "" {
		s.installProgress.Stage = strings.TrimSpace(stage)
	}
	if strings.TrimSpace(s.installProgress.Stage) == "" {
		s.installProgress.Stage = "idle"
	}
	s.installProgress.Tool = strings.TrimSpace(tool)
	s.installProgress.Action = strings.TrimSpace(action)
	s.installProgress.Command = strings.TrimSpace(command)
	s.installProgress.Message = strings.TrimSpace(message)
	if s.installProgress.StartedAt.IsZero() {
		s.installProgress.StartedAt = now
	}
	s.installProgress.UpdatedAt = now
	s.installProgress.Logs = s.installLogsTail
}

func (s *DiagnosticsService) appendInstallProgressLog(chunk string) {
	if chunk == "" {
		return
	}
	now := time.Now().UTC()
	s.progressMu.Lock()
	s.installLogsTail = appendLogTail(s.installLogsTail, chunk, dependencyInstallProgressLogsLimit)
	s.installProgress.Logs = s.installLogsTail
	if s.installProgress.StartedAt.IsZero() {
		s.installProgress.StartedAt = now
	}
	s.installProgress.UpdatedAt = now
	s.progressMu.Unlock()
}

func appendLogTail(existing, chunk string, limit int) string {
	combined := existing + chunk
	if limit <= 0 || len(combined) <= limit {
		return combined
	}
	return combined[len(combined)-limit:]
}

func inspectBinary(ctx context.Context, runner *sys.Runner, name string, versionArgs []string) core.WebBinaryDiagnostic {
	which, resolvedName, err := util.ResolveToolExecutable(name)
	if err != nil {
		return core.WebBinaryDiagnostic{Name: name, Available: false, Notes: "Binaire introuvable"}
	}
	output, err := runner.Run(ctx, sys.RunOptions{
		Executable:    which,
		Args:          versionArgs,
		CaptureOutput: true,
	})
	line := firstNonEmptyLine(output)
	note := ""
	if err != nil {
		note = "Version non disponible"
	}
	if resolvedName != "" && resolvedName != name {
		if note == "" {
			note = "Alias detecte: " + resolvedName
		} else {
			note += " | Alias detecte: " + resolvedName
		}
	}
	return core.WebBinaryDiagnostic{
		Name:      name,
		Path:      which,
		Version:   line,
		Available: true,
		Notes:     note,
	}
}

type pythonCommandCandidate struct {
	Exec       string
	PrefixArgs []string
}

func pythonCommandCandidates() []pythonCommandCandidate {
	if runtime.GOOS == "windows" {
		return uniquePythonCommandCandidates([]pythonCommandCandidate{
			{Exec: "py", PrefixArgs: []string{"-3.13"}},
			{Exec: "py", PrefixArgs: []string{"-3.12"}},
			{Exec: "py", PrefixArgs: []string{"-3.11"}},
			{Exec: "py", PrefixArgs: []string{"-3"}},
			{Exec: "py"},
			{Exec: "python"},
			{Exec: "python3"},
		})
	}
	candidates := []pythonCommandCandidate{}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			pythonCommandCandidate{Exec: "/opt/homebrew/opt/python@3.13/bin/python3.13"},
			pythonCommandCandidate{Exec: "/opt/homebrew/opt/python@3.12/bin/python3.12"},
			pythonCommandCandidate{Exec: "/usr/local/opt/python@3.13/bin/python3.13"},
			pythonCommandCandidate{Exec: "/usr/local/opt/python@3.12/bin/python3.12"},
		)
	}
	candidates = append(candidates,
		pythonCommandCandidate{Exec: "python3.13"},
		pythonCommandCandidate{Exec: "python3.12"},
		pythonCommandCandidate{Exec: "python3.11"},
		pythonCommandCandidate{Exec: "python3"},
		pythonCommandCandidate{Exec: "python"},
	)
	return uniquePythonCommandCandidates(candidates)
}

func uniquePythonCommandCandidates(candidates []pythonCommandCandidate) []pythonCommandCandidate {
	out := make([]pythonCommandCandidate, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		execName := strings.TrimSpace(candidate.Exec)
		if execName == "" {
			continue
		}
		key := execName + "\x00" + strings.Join(candidate.PrefixArgs, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, pythonCommandCandidate{
			Exec:       execName,
			PrefixArgs: append([]string{}, candidate.PrefixArgs...),
		})
	}
	return out
}

func inspectArgosTranslate(ctx context.Context, runner *sys.Runner) core.WebBinaryDiagnostic {
	const moduleName = "argostranslate"
	probeScript := "import argostranslate; import argostranslate.package as _p; import argostranslate.translate as _t; print(getattr(argostranslate, '__version__', 'installe'))"

	pythonDetected := false
	moduleMissing := false
	moduleError := ""

	for _, candidate := range argosPythonProbeCandidates() {
		output, version, note, available, pyFound, missing, errText := probeArgosWithPython(ctx, runner, candidate, probeScript)
		_ = output
		if pyFound {
			pythonDetected = true
		}
		if available {
			return core.WebBinaryDiagnostic{
				Name:      moduleName,
				Path:      resolvedProbePath(candidate),
				Version:   version,
				Available: true,
				Notes:     note,
			}
		}
		if missing {
			moduleMissing = true
			continue
		}
		if strings.TrimSpace(errText) != "" {
			moduleError = strings.TrimSpace(errText)
		}
	}

	if !pythonDetected {
		return core.WebBinaryDiagnostic{
			Name:      moduleName,
			Available: false,
			Notes:     "Python introuvable (python3/python)",
		}
	}
	if moduleMissing {
		return core.WebBinaryDiagnostic{
			Name:      moduleName,
			Available: false,
			Notes:     "Module Python introuvable. Utilise le bouton Installer pour creer le venv PersoDL et installer argostranslate.",
		}
	}
	if moduleError != "" {
		return core.WebBinaryDiagnostic{
			Name:      moduleName,
			Available: false,
			Notes:     "Runtime Argos incompatible: " + lastNonEmptyLine(moduleError),
		}
	}
	return core.WebBinaryDiagnostic{
		Name:      moduleName,
		Available: false,
		Notes:     "Module Python introuvable. Utilise le bouton Installer pour creer le venv PersoDL et installer argostranslate.",
	}
}

func probeArgosWithPython(ctx context.Context, runner *sys.Runner, candidate pythonCommandCandidate, probeScript string) (rawOutput, version, note string, available bool, pythonFound bool, moduleMissing bool, errText string) {
	if strings.Contains(candidate.Exec, string(filepath.Separator)) {
		info, err := os.Stat(candidate.Exec)
		if err != nil || info.IsDir() {
			return "", "", "", false, false, false, ""
		}
		pythonFound = true
	} else {
		if _, lookErr := exec.LookPath(candidate.Exec); lookErr != nil {
			return "", "", "", false, false, false, ""
		}
		pythonFound = true
	}

	args := append(append([]string{}, candidate.PrefixArgs...), "-c", probeScript)
	output, runErr := runner.Run(ctx, sys.RunOptions{
		Executable:    candidate.Exec,
		Args:          args,
		CaptureOutput: true,
	})
	if runErr == nil {
		version = firstNonEmptyLine(output)
		if version == "" {
			version = "installe"
		}
		note = probeCandidateLabel(candidate)
		return output, version, note, true, pythonFound, false, ""
	}

	combined := strings.ToLower(strings.TrimSpace(output))
	if strings.Contains(combined, "no module named") || strings.Contains(combined, "modulenotfounderror") {
		return output, "", "", false, pythonFound, true, ""
	}
	return output, "", "", false, pythonFound, false, strings.TrimSpace(output)
}

func argosPythonProbeCandidates() []pythonCommandCandidate {
	candidates := make([]pythonCommandCandidate, 0, 8)
	for _, venvPython := range util.ArgosVenvPythonCandidates("") {
		if strings.TrimSpace(venvPython) == "" {
			continue
		}
		candidates = append(candidates, pythonCommandCandidate{Exec: venvPython})
	}
	candidates = append(candidates, pythonCommandCandidates()...)
	return candidates
}

func probeCandidateLabel(candidate pythonCommandCandidate) string {
	label := strings.TrimSpace(candidate.Exec)
	if len(candidate.PrefixArgs) > 0 {
		label = label + " " + strings.Join(candidate.PrefixArgs, " ")
	}
	return "Runtime detecte: " + label
}

func resolvedProbePath(candidate pythonCommandCandidate) string {
	if strings.Contains(candidate.Exec, string(filepath.Separator)) {
		return candidate.Exec
	}
	return lookupExecutablePath(candidate.Exec)
}

func lookupExecutablePath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func firstNonEmptyLine(v string) string {
	for _, line := range strings.Split(v, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func lastNonEmptyLine(v string) string {
	lines := strings.Split(v, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func (s *DiagnosticsService) InstallDependencies(ctx context.Context, tools []string) (core.DependencyInstallResponse, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()
	s.resetInstallProgress()

	if len(tools) == 0 {
		tools = dependencyTools
	}
	pm := detectPackageManager()
	outdated := s.detectOutdatedTools(ctx, pm)
	logBuilder := strings.Builder{}
	results := make([]core.DependencyInstallResult, 0, len(tools))
	s.setInstallProgress(true, "running", "", "", "", "Verification des dependances...")

	for _, tool := range tools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}

		if s.toolAvailable(ctx, tool) {
			if outdated[tool] {
				s.setInstallProgress(true, "running", tool, "update", "", "Mise a jour en cours...")
				updated, msg, logChunk := s.updateTool(ctx, pm, tool)
				appendInstallLog(&logBuilder, logChunk)
				results = append(results, core.DependencyInstallResult{Name: tool, Installed: updated, Message: msg})
			} else {
				s.setInstallProgress(true, "running", tool, "check", "", "Aucune action requise.")
				results = append(results, core.DependencyInstallResult{Name: tool, Installed: true, Message: "A jour"})
			}
			continue
		}

		s.setInstallProgress(true, "running", tool, "install", "", "Installation en cours...")
		installed, msg, logChunk := s.installTool(ctx, pm, tool)
		appendInstallLog(&logBuilder, logChunk)
		results = append(results, core.DependencyInstallResult{Name: tool, Installed: installed, Message: msg})
	}

	ok := true
	for _, r := range results {
		if !r.Installed {
			ok = false
			break
		}
	}
	if ok {
		s.setInstallProgress(false, "completed", "", "", "", "Installation terminee.")
	} else {
		s.setInstallProgress(false, "failed", "", "", "", "Installation terminee avec erreur(s).")
	}
	return core.DependencyInstallResponse{OK: ok, PackageManager: pm.Name, Results: results, Logs: logBuilder.String()}, nil
}

func appendInstallLog(out *strings.Builder, chunk string) {
	if chunk == "" {
		return
	}
	out.WriteString(chunk)
	if !strings.HasSuffix(chunk, "\n") {
		out.WriteString("\n")
	}
}

func (s *DiagnosticsService) toolAvailable(ctx context.Context, tool string) bool {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "argostranslate":
		return inspectArgosTranslate(ctx, s.runner).Available
	default:
		_, _, err := util.ResolveToolExecutable(tool)
		return err == nil
	}
}

func (s *DiagnosticsService) installTool(ctx context.Context, pm packageManager, tool string) (bool, string, string) {
	commands := commandsForTool(pm, tool)
	if len(commands) == 0 {
		return false, "Aucune commande d'installation disponible", ""
	}
	return s.runCommandSequence(
		ctx,
		tool,
		"install",
		commands,
		"Installe",
		"Installation automatique echouee (utilise les commandes manuelles)",
	)
}

func (s *DiagnosticsService) updateTool(ctx context.Context, pm packageManager, tool string) (bool, string, string) {
	commands := updateCommandsForTool(pm, tool)
	if len(commands) == 0 {
		commands = commandsForTool(pm, tool)
	}
	if len(commands) == 0 {
		return false, "Aucune commande de mise a jour disponible", ""
	}
	return s.runCommandSequence(
		ctx,
		tool,
		"update",
		commands,
		"Mis a jour",
		"Mise a jour automatique echouee (utilise les commandes manuelles)",
	)
}

func (s *DiagnosticsService) runCommandSequence(ctx context.Context, tool string, action string, commands []commandSpec, successMessage string, failureMessage string) (bool, string, string) {
	logs := strings.Builder{}
	var logsMu sync.Mutex
	appendLogChunk := func(chunk string) {
		if chunk == "" {
			return
		}
		logsMu.Lock()
		logs.WriteString(chunk)
		logsMu.Unlock()
		s.appendInstallProgressLog(chunk)
	}
	logText := func() string {
		logsMu.Lock()
		defer logsMu.Unlock()
		return logs.String()
	}

	for _, cmd := range commands {
		commandText := cmd.Exec
		if len(cmd.Args) > 0 {
			commandText += " " + strings.Join(cmd.Args, " ")
		}
		appendLogChunk("$ " + commandText + "\n")
		s.setInstallProgress(true, "running", tool, action, commandText, "Execution de la commande...")

		_, err := s.runner.Run(ctx, sys.RunOptions{
			Executable: cmd.Exec,
			Args:       cmd.Args,
			OnOutput:   appendLogChunk,
		})
		if err != nil {
			appendLogChunk(fmt.Sprintf("[erreur] %v\n", err))
			continue
		}
		if s.toolAvailable(ctx, tool) {
			s.setInstallProgress(true, "running", tool, action, commandText, successMessage)
			return true, successMessage, logText()
		}
	}

	return false, failureMessage, logText()
}

func (s *DiagnosticsService) detectOutdatedTools(ctx context.Context, pm packageManager) map[string]bool {
	outdatedTools := map[string]bool{}
	outdatedPackages := s.collectOutdatedPackages(ctx, pm)
	outdatedPythonPackages := s.collectOutdatedPythonPackages(ctx)
	if len(outdatedPackages) == 0 {
		if outdatedPythonPackages["argostranslate"] {
			outdatedTools["argostranslate"] = true
		}
		return outdatedTools
	}

	for _, tool := range dependencyTools {
		for _, key := range packageKeysForTool(pm.Name, tool) {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if normalizedKey != "" && outdatedPackages[normalizedKey] {
				outdatedTools[tool] = true
				break
			}
		}
	}
	if outdatedPythonPackages["argostranslate"] {
		outdatedTools["argostranslate"] = true
	}
	return outdatedTools
}

func (s *DiagnosticsService) collectOutdatedPythonPackages(ctx context.Context) map[string]bool {
	out := map[string]bool{}

	candidates := make([]pythonCommandCandidate, 0, 8)
	for _, venvPython := range util.ArgosVenvPythonCandidates("") {
		if strings.TrimSpace(venvPython) == "" {
			continue
		}
		candidates = append(candidates, pythonCommandCandidate{Exec: venvPython})
	}
	candidates = append(candidates, pythonCommandCandidates()...)

	for _, candidate := range candidates {
		output, available, err := runPythonPipOutdated(ctx, s.runner, candidate)
		if !available || err != nil {
			continue
		}
		payload := strings.TrimSpace(output)
		if payload == "" {
			return out
		}
		var outdated []struct {
			Name string `json:"name"`
		}
		if parseErr := json.Unmarshal([]byte(payload), &outdated); parseErr != nil {
			continue
		}
		for _, pkg := range outdated {
			name := strings.ToLower(strings.TrimSpace(pkg.Name))
			if name != "" {
				out[name] = true
			}
		}
		return out
	}

	return out
}

func runPythonPipOutdated(ctx context.Context, runner *sys.Runner, candidate pythonCommandCandidate) (string, bool, error) {
	if strings.Contains(candidate.Exec, string(filepath.Separator)) {
		info, err := os.Stat(candidate.Exec)
		if err != nil || info.IsDir() {
			return "", false, err
		}
	} else {
		if _, err := exec.LookPath(candidate.Exec); err != nil {
			return "", false, err
		}
	}

	args := append(append([]string{}, candidate.PrefixArgs...), "-m", "pip", "list", "--outdated", "--format=json")
	output, err := runner.Run(ctx, sys.RunOptions{
		Executable:    candidate.Exec,
		Args:          args,
		CaptureOutput: true,
	})
	return output, true, err
}

func (s *DiagnosticsService) collectOutdatedPackages(ctx context.Context, pm packageManager) map[string]bool {
	out := map[string]bool{}
	runAndSplit := func(execName string, args ...string) []string {
		output, _ := s.runner.Run(ctx, sys.RunOptions{Executable: execName, Args: args, CaptureOutput: true})
		if output == "" {
			return nil
		}
		return strings.Split(output, "\n")
	}

	switch pm.Name {
	case "brew":
		for _, line := range runAndSplit("brew", "outdated", "--formula", "--quiet") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			name := strings.ToLower(strings.Fields(line)[0])
			out[name] = true
		}
	case "choco":
		for _, line := range runAndSplit("choco", "outdated", "--limit-output") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "Chocolatey") {
				continue
			}
			parts := strings.Split(line, "|")
			name := strings.TrimSpace(parts[0])
			if name != "" {
				out[strings.ToLower(name)] = true
			}
		}
	case "scoop":
		for _, line := range runAndSplit("scoop", "status") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "Name") || strings.HasPrefix(trimmed, "-") {
				continue
			}
			fields := strings.Fields(trimmed)
			if len(fields) == 0 {
				continue
			}
			name := strings.ToLower(fields[0])
			if name != "scoop" {
				out[name] = true
			}
		}
	case "winget":
		output, _ := s.runner.Run(ctx, sys.RunOptions{
			Executable:    "winget",
			Args:          []string{"upgrade", "--accept-source-agreements"},
			CaptureOutput: true,
		})
		lower := strings.ToLower(output)
		for _, id := range []string{
			"yt-dlp.yt-dlp",
			"gyan.ffmpeg",
			"ggml-org.whisper.cpp",
			"ggerganov.whisper.cpp",
		} {
			if strings.Contains(lower, id) {
				out[id] = true
			}
		}
	}

	return out
}

func packageKeysForTool(packageManagerName string, tool string) []string {
	switch packageManagerName {
	case "brew":
		switch tool {
		case "yt-dlp", "ffmpeg", "qobuz-dl":
			return []string{tool}
		case "whisper-cli":
			return []string{"whisper-cpp"}
		}
	case "winget":
		switch tool {
		case "yt-dlp":
			return []string{"yt-dlp.yt-dlp"}
		case "ffmpeg":
			return []string{"gyan.ffmpeg"}
		case "whisper-cli":
			return []string{"ggml-org.whisper.cpp", "ggerganov.whisper.cpp"}
		}
	case "choco", "scoop":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []string{tool}
		}
	}
	return nil
}

type packageManager struct {
	Name string
}

type commandSpec struct {
	Exec string
	Args []string
}

func detectPackageManager() packageManager {
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("brew"); err == nil {
			return packageManager{Name: "brew"}
		}
		return packageManager{Name: "none"}
	}
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("winget"); err == nil {
			return packageManager{Name: "winget"}
		}
		if _, err := exec.LookPath("choco"); err == nil {
			return packageManager{Name: "choco"}
		}
		if _, err := exec.LookPath("scoop"); err == nil {
			return packageManager{Name: "scoop"}
		}
		return packageManager{Name: "none"}
	}
	for _, cand := range []string{"apt-get", "dnf", "pacman"} {
		if _, err := exec.LookPath(cand); err == nil {
			return packageManager{Name: cand}
		}
	}
	return packageManager{Name: "none"}
}

func commandsForTool(pm packageManager, tool string) []commandSpec {
	switch pm.Name {
	case "brew":
		switch tool {
		case "yt-dlp", "ffmpeg", "qobuz-dl":
			return []commandSpec{{Exec: "brew", Args: []string{"install", tool}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "brew", Args: []string{"install", "whisper-cpp"}}}
		case "argostranslate":
			return argosManagedInstallCommands([]commandSpec{
				{Exec: "brew", Args: []string{"install", "python@3.12"}},
				{Exec: "brew", Args: []string{"install", "python"}},
			})
		}
	case "winget":
		switch tool {
		case "yt-dlp":
			return []commandSpec{{Exec: "winget", Args: []string{"install", "--id", "yt-dlp.yt-dlp", "-e"}}}
		case "ffmpeg":
			return []commandSpec{{Exec: "winget", Args: []string{"install", "--id", "Gyan.FFmpeg", "-e"}}}
		case "qobuz-dl":
			return []commandSpec{
				{Exec: "winget", Args: []string{"install", "--id", "Python.Python.3.12", "-e"}},
				{Exec: "py", Args: []string{"-m", "pip", "install", "--user", "pipx"}},
				{Exec: "py", Args: []string{"-m", "pipx", "install", "qobuz-dl"}},
			}
		case "whisper-cli":
			return []commandSpec{
				{Exec: "winget", Args: []string{"install", "--id", "ggml-org.whisper.cpp", "-e"}},
				{Exec: "winget", Args: []string{"install", "--id", "ggerganov.whisper.cpp", "-e"}},
				{Exec: "powershell", Args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", whisperWindowsInstallScript()}},
			}
		case "argostranslate":
			return argosManagedInstallCommands([]commandSpec{
				{Exec: "winget", Args: []string{"install", "--id", "Python.Python.3.12", "-e"}},
			})
		}
	case "choco":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "choco", Args: []string{"install", "-y", tool}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "powershell", Args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", whisperWindowsInstallScript()}}}
		case "argostranslate":
			return argosManagedInstallCommands([]commandSpec{
				{Exec: "choco", Args: []string{"install", "-y", "python"}},
			})
		}
	case "scoop":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "scoop", Args: []string{"install", tool}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "powershell", Args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", whisperWindowsInstallScript()}}}
		case "argostranslate":
			return argosManagedInstallCommands([]commandSpec{
				{Exec: "scoop", Args: []string{"install", "python"}},
			})
		}
	case "apt-get":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "sudo", Args: []string{"apt-get", "update"}}, {Exec: "sudo", Args: []string{"apt-get", "install", "-y", tool}}}
		case "qobuz-dl":
			return []commandSpec{{Exec: "sudo", Args: []string{"apt-get", "install", "-y", "python3-pip", "pipx"}}, {Exec: "pipx", Args: []string{"install", "qobuz-dl"}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "sudo", Args: []string{"apt-get", "install", "-y", "whisper-cpp"}}}
		case "argostranslate":
			return argosManagedInstallCommands([]commandSpec{
				{Exec: "sudo", Args: []string{"apt-get", "install", "-y", "python3", "python3-venv"}},
			})
		}
	case "dnf":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "sudo", Args: []string{"dnf", "install", "-y", tool}}}
		case "qobuz-dl":
			return []commandSpec{{Exec: "sudo", Args: []string{"dnf", "install", "-y", "python3-pip", "pipx"}}, {Exec: "pipx", Args: []string{"install", "qobuz-dl"}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "sudo", Args: []string{"dnf", "install", "-y", "whisper-cpp"}}}
		case "argostranslate":
			return argosManagedInstallCommands([]commandSpec{
				{Exec: "sudo", Args: []string{"dnf", "install", "-y", "python3"}},
			})
		}
	case "pacman":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "sudo", Args: []string{"pacman", "-Sy", "--noconfirm", tool}}}
		case "qobuz-dl":
			return []commandSpec{{Exec: "sudo", Args: []string{"pacman", "-Sy", "--noconfirm", "python-pipx"}}, {Exec: "pipx", Args: []string{"install", "qobuz-dl"}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "sudo", Args: []string{"pacman", "-Sy", "--noconfirm", "whisper-cpp"}}}
		case "argostranslate":
			return argosManagedInstallCommands([]commandSpec{
				{Exec: "sudo", Args: []string{"pacman", "-Sy", "--noconfirm", "python"}},
			})
		}
	}
	if tool == "argostranslate" {
		return argosManagedInstallCommands(nil)
	}
	return nil
}

func updateCommandsForTool(pm packageManager, tool string) []commandSpec {
	if tool == "argostranslate" {
		return argosManagedInstallCommands(nil)
	}

	switch pm.Name {
	case "brew":
		switch tool {
		case "yt-dlp", "ffmpeg", "qobuz-dl":
			return []commandSpec{{Exec: "brew", Args: []string{"upgrade", tool}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "brew", Args: []string{"upgrade", "whisper-cpp"}}}
		}
	case "winget":
		switch tool {
		case "yt-dlp":
			return []commandSpec{{Exec: "winget", Args: []string{"upgrade", "--id", "yt-dlp.yt-dlp", "-e", "--accept-source-agreements", "--accept-package-agreements"}}}
		case "ffmpeg":
			return []commandSpec{{Exec: "winget", Args: []string{"upgrade", "--id", "Gyan.FFmpeg", "-e", "--accept-source-agreements", "--accept-package-agreements"}}}
		case "qobuz-dl":
			return []commandSpec{{Exec: "py", Args: []string{"-m", "pipx", "upgrade", "qobuz-dl"}}}
		case "whisper-cli":
			return []commandSpec{
				{Exec: "winget", Args: []string{"upgrade", "--id", "ggml-org.whisper.cpp", "-e", "--accept-source-agreements", "--accept-package-agreements"}},
				{Exec: "winget", Args: []string{"upgrade", "--id", "ggerganov.whisper.cpp", "-e", "--accept-source-agreements", "--accept-package-agreements"}},
				{Exec: "powershell", Args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", whisperWindowsInstallScript()}},
			}
		}
	case "choco":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "choco", Args: []string{"upgrade", "-y", tool}}}
		case "qobuz-dl":
			return []commandSpec{{Exec: "py", Args: []string{"-m", "pipx", "upgrade", "qobuz-dl"}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "powershell", Args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", whisperWindowsInstallScript()}}}
		}
	case "scoop":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "scoop", Args: []string{"update", tool}}}
		case "qobuz-dl":
			return []commandSpec{{Exec: "py", Args: []string{"-m", "pipx", "upgrade", "qobuz-dl"}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "powershell", Args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", whisperWindowsInstallScript()}}}
		}
	case "apt-get":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "sudo", Args: []string{"apt-get", "update"}}, {Exec: "sudo", Args: []string{"apt-get", "install", "--only-upgrade", "-y", tool}}}
		case "qobuz-dl":
			return []commandSpec{{Exec: "pipx", Args: []string{"upgrade", "qobuz-dl"}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "sudo", Args: []string{"apt-get", "update"}}, {Exec: "sudo", Args: []string{"apt-get", "install", "--only-upgrade", "-y", "whisper-cpp"}}}
		}
	case "dnf":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "sudo", Args: []string{"dnf", "upgrade", "-y", tool}}}
		case "qobuz-dl":
			return []commandSpec{{Exec: "pipx", Args: []string{"upgrade", "qobuz-dl"}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "sudo", Args: []string{"dnf", "upgrade", "-y", "whisper-cpp"}}}
		}
	case "pacman":
		switch tool {
		case "yt-dlp", "ffmpeg":
			return []commandSpec{{Exec: "sudo", Args: []string{"pacman", "-Sy", "--noconfirm", tool}}}
		case "qobuz-dl":
			return []commandSpec{{Exec: "pipx", Args: []string{"upgrade", "qobuz-dl"}}}
		case "whisper-cli":
			return []commandSpec{{Exec: "sudo", Args: []string{"pacman", "-Sy", "--noconfirm", "whisper-cpp"}}}
		}
	}
	return nil
}

func argosManagedInstallCommands(prefix []commandSpec) []commandSpec {
	commands := make([]commandSpec, 0, len(prefix)+48)
	commands = append(commands, prefix...)

	venvDir := strings.TrimSpace(util.ArgosVenvDirectory())
	if venvDir == "" {
		return commands
	}

	for _, creator := range pythonCommandCandidates() {
		venvArgs := append(append([]string{}, creator.PrefixArgs...), "-m", "venv", "--clear", venvDir)
		commands = append(commands, commandSpec{Exec: creator.Exec, Args: venvArgs})

		for _, venvPython := range util.ArgosVenvPythonCandidates(venvDir) {
			pythonExec := strings.TrimSpace(venvPython)
			if pythonExec == "" {
				continue
			}
			commands = append(commands,
				commandSpec{Exec: pythonExec, Args: []string{"-m", "pip", "install", "--upgrade", "pip"}},
				commandSpec{Exec: pythonExec, Args: []string{"-m", "pip", "install", "--upgrade", "argostranslate"}},
			)
		}
	}

	if len(commands) == len(prefix) {
		for _, venvPython := range util.ArgosVenvPythonCandidates(venvDir) {
			pythonExec := strings.TrimSpace(venvPython)
			if pythonExec == "" {
				continue
			}
			commands = append(commands, commandSpec{Exec: pythonExec, Args: []string{"-m", "pip", "install", "--upgrade", "argostranslate"}})
		}
	}

	return uniqueCommandSpecs(commands)
}

func uniqueCommandSpecs(commands []commandSpec) []commandSpec {
	out := make([]commandSpec, 0, len(commands))
	seen := map[string]struct{}{}
	for _, command := range commands {
		execName := strings.TrimSpace(command.Exec)
		if execName == "" {
			continue
		}
		key := execName + "\x00" + strings.Join(command.Args, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out,
			commandSpec{Exec: execName, Args: append([]string{}, command.Args...)},
		)
	}
	return out
}

func whisperWindowsInstallScript() string {
	return "$ErrorActionPreference='Stop';" +
		"$release=Invoke-RestMethod -Headers @{ 'User-Agent'='PersoDL' } -Uri 'https://api.github.com/repos/ggml-org/whisper.cpp/releases/latest';" +
		"$asset=$release.assets | Where-Object { $_.name -match 'win' -and $_.name -match 'x64' -and $_.name -match 'zip' } | Select-Object -First 1;" +
		"if(-not $asset){ throw 'Aucun binaire Windows x64 whisper.cpp trouve'; };" +
		"$zipPath=Join-Path $env:TEMP 'persodl-whispercpp.zip';" +
		"$extractDir=Join-Path $env:TEMP 'persodl-whispercpp';" +
		"Remove-Item -Path $extractDir -Recurse -Force -ErrorAction SilentlyContinue;" +
		"Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $zipPath;" +
		"Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force;" +
		"$candidate=Get-ChildItem -Path $extractDir -Recurse -File | Where-Object { $_.Name -ieq 'whisper-cli.exe' -or $_.Name -ieq 'whisper-cpp.exe' } | Select-Object -First 1;" +
		"if(-not $candidate){ throw 'whisper-cli.exe introuvable dans l archive'; };" +
		"$binDir=Join-Path $env:APPDATA 'PersoDL\\bin';" +
		"New-Item -ItemType Directory -Path $binDir -Force | Out-Null;" +
		"$sourceDir=Split-Path -Parent $candidate.FullName;" +
		"Copy-Item -Path (Join-Path $sourceDir '*') -Destination $binDir -Recurse -Force;" +
		"if(-not (Test-Path (Join-Path $binDir 'whisper-cli.exe')) -and (Test-Path (Join-Path $binDir 'whisper-cpp.exe'))){ Copy-Item -Path (Join-Path $binDir 'whisper-cpp.exe') -Destination (Join-Path $binDir 'whisper-cli.exe') -Force; };" +
		"$modelDir=Join-Path $binDir 'models';" +
		"New-Item -ItemType Directory -Path $modelDir -Force | Out-Null;" +
		"$modelPath=Join-Path $modelDir 'ggml-base.bin';" +
		"if(-not (Test-Path $modelPath)){ Invoke-WebRequest -Uri 'https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin' -OutFile $modelPath; };" +
		"Write-Output ('whisper installe dans ' + $binDir)"
}
