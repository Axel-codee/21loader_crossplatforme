package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"21loader-cross/internal/core"
	"21loader-cross/internal/sys"
	"21loader-cross/internal/util"
)

const argosJSONBeginMarker = "LOADER21_ARGOS_JSON_BEGIN"
const argosJSONEndMarker = "LOADER21_ARGOS_JSON_END"

type TranslationLanguageService struct {
	runner  *sys.Runner
	baseDir string
}

type argosLanguageScriptResponse struct {
	OK      bool                                    `json:"ok"`
	Message string                                  `json:"message"`
	Catalog core.TranslationLanguageCatalogResponse `json:"catalog"`
}

func NewTranslationLanguageService(r *sys.Runner, baseDir string) *TranslationLanguageService {
	return &TranslationLanguageService{
		runner:  r,
		baseDir: strings.TrimSpace(baseDir),
	}
}

func (s *TranslationLanguageService) Catalog(ctx context.Context) (core.TranslationLanguageCatalogResponse, error) {
	var payload argosLanguageScriptResponse
	if err := s.runScript(ctx, []string{"--action", "list"}, &payload); err != nil {
		return core.TranslationLanguageCatalogResponse{
			RuntimeAvailable: false,
			RuntimeMessage:   err.Error(),
			Languages:        []core.TranslationLanguageInfoDTO{},
			Pairs:            []core.TranslationLanguagePairDTO{},
		}, nil
	}

	catalog := payload.Catalog
	if catalog.Languages == nil {
		catalog.Languages = []core.TranslationLanguageInfoDTO{}
	}
	if catalog.Pairs == nil {
		catalog.Pairs = []core.TranslationLanguagePairDTO{}
	}
	return catalog, nil
}

func (s *TranslationLanguageService) InstallPair(ctx context.Context, sourceCode, targetCode string) (core.TranslationLanguageInstallResponse, error) {
	source := normalizeArgosLanguageCode(sourceCode)
	target := normalizeArgosLanguageCode(targetCode)
	if source == "" || target == "" {
		return core.TranslationLanguageInstallResponse{}, fmt.Errorf("les codes langue source/cible sont requis")
	}
	if source == target {
		return core.TranslationLanguageInstallResponse{}, fmt.Errorf("langues source/cible identiques")
	}

	var payload argosLanguageScriptResponse
	if err := s.runScript(ctx, []string{
		"--action", "install",
		"--from-code", source,
		"--to-code", target,
	}, &payload); err != nil {
		return core.TranslationLanguageInstallResponse{}, err
	}

	if !payload.OK {
		msg := strings.TrimSpace(payload.Message)
		if msg == "" {
			msg = fmt.Sprintf("installation Argos %s->%s echouee", source, target)
		}
		return core.TranslationLanguageInstallResponse{}, fmt.Errorf(msg)
	}

	catalog := payload.Catalog
	if catalog.Languages == nil {
		catalog.Languages = []core.TranslationLanguageInfoDTO{}
	}
	if catalog.Pairs == nil {
		catalog.Pairs = []core.TranslationLanguagePairDTO{}
	}

	message := strings.TrimSpace(payload.Message)
	if message == "" {
		message = fmt.Sprintf("Paquet Argos %s->%s installe.", source, target)
	}
	return core.TranslationLanguageInstallResponse{
		OK:      true,
		Message: message,
		Catalog: catalog,
	}, nil
}

func (s *TranslationLanguageService) runScript(ctx context.Context, scriptArgs []string, dst *argosLanguageScriptResponse) error {
	if dst == nil {
		return fmt.Errorf("destination reponse invalide")
	}

	pythonExec, err := s.resolveArgosPythonExecutable(ctx)
	if err != nil {
		return err
	}

	scriptPath := filepath.Join(s.baseDir, "assets", "scripts", "argos_languages.py")
	if info, statErr := os.Stat(scriptPath); statErr != nil || info.IsDir() {
		return fmt.Errorf("script Argos introuvable: %s", scriptPath)
	}

	args := append([]string{scriptPath}, scriptArgs...)
	output, runErr := s.runner.Run(ctx, sys.RunOptions{
		Executable:    pythonExec,
		Args:          args,
		CaptureOutput: true,
	})
	if runErr != nil {
		detail := lastNonEmptyLineText(output)
		if strings.TrimSpace(detail) != "" {
			return fmt.Errorf("%s", detail)
		}
		return fmt.Errorf("commande Argos echouee: %w", runErr)
	}

	payloadJSON, extractErr := extractArgosJSONPayload(output)
	if extractErr != nil {
		return extractErr
	}
	if unmarshalErr := json.Unmarshal([]byte(payloadJSON), dst); unmarshalErr != nil {
		return fmt.Errorf("reponse Argos invalide: %w", unmarshalErr)
	}
	return nil
}

func extractArgosJSONPayload(raw string) (string, error) {
	start := strings.LastIndex(raw, argosJSONBeginMarker)
	if start < 0 {
		return "", fmt.Errorf("reponse Argos invalide: marqueur debut manquant")
	}
	tail := raw[start+len(argosJSONBeginMarker):]
	end := strings.Index(tail, argosJSONEndMarker)
	if end < 0 {
		return "", fmt.Errorf("reponse Argos invalide: marqueur fin manquant")
	}
	payload := strings.TrimSpace(tail[:end])
	if payload == "" {
		return "", fmt.Errorf("reponse Argos vide")
	}
	return payload, nil
}

func (s *TranslationLanguageService) resolveArgosPythonExecutable(ctx context.Context) (string, error) {
	candidates := make([]string, 0, 8)
	candidates = append(candidates, util.ArgosVenvPythonCandidates("")...)
	candidates = append(candidates, "python3.13", "python3.12", "python3.11", "python3", "python")

	var lastProbeError string
	for _, raw := range candidates {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}

		resolved := candidate
		if !strings.Contains(candidate, string(os.PathSeparator)) {
			path, err := exec.LookPath(candidate)
			if err != nil {
				continue
			}
			resolved = path
		} else {
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
		}

		output, err := s.runner.Run(ctx, sys.RunOptions{
			Executable:    resolved,
			Args:          []string{"-c", "import argostranslate.package as _p; import argostranslate.translate as _t"},
			CaptureOutput: true,
		})
		if err == nil {
			return resolved, nil
		}
		if line := strings.TrimSpace(lastNonEmptyLineText(output)); line != "" {
			lastProbeError = line
		}
	}

	if lastProbeError != "" {
		return "", fmt.Errorf("runtime Argos indisponible: %s", lastProbeError)
	}
	return "", fmt.Errorf("runtime Argos indisponible: installe/maj argostranslate via Diagnostics")
}

func normalizeArgosLanguageCode(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return normalized
}

func lastNonEmptyLineText(v string) string {
	lines := strings.Split(v, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}
