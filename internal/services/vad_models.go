package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"21loader-cross/internal/core"
	"21loader-cross/internal/util"
)

type vadModelSpec struct {
	ID                string
	Name              string
	FileName          string
	DownloadURL       string
	ApproximateSizeMB int
}

var vadModelCatalog = []vadModelSpec{
	{ID: "silero-v5.1.2", Name: "Silero VAD v5.1.2", FileName: "ggml-silero-v5.1.2.bin", DownloadURL: "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin", ApproximateSizeMB: 1},
	{ID: "silero-v6.2.0", Name: "Silero VAD v6.2.0", FileName: "ggml-silero-v6.2.0.bin", DownloadURL: "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v6.2.0.bin", ApproximateSizeMB: 1},
}

type VADModelService struct {
	httpClient *http.Client
	mu         sync.Mutex
	progressMu sync.RWMutex
	progress   map[string]core.VADModelInstallProgressResponse
}

func NewVADModelService() *VADModelService {
	return &VADModelService{
		httpClient: &http.Client{},
		progress:   map[string]core.VADModelInstallProgressResponse{},
	}
}

func (s *VADModelService) ListModels(selectedPath string) (core.VADModelsResponse, error) {
	modelDir, err := vadModelDirectory()
	if err != nil {
		return core.VADModelsResponse{}, err
	}
	_ = os.MkdirAll(modelDir, 0o755)

	selectedPath = strings.TrimSpace(selectedPath)
	searchDirs := util.VADModelSearchDirs(selectedPath)
	knownModels := make([]core.VADModelInfoDTO, 0, len(vadModelCatalog))
	for _, spec := range vadModelCatalog {
		info := vadModelInfoFromSpec(spec)
		managedPath := filepath.Join(modelDir, spec.FileName)
		if managedBytes, ok := existingFileSize(managedPath); ok {
			info.Installed = true
			info.ManagedByApp = true
			info.InstalledPath = managedPath
			info.InstalledBytes = managedBytes
		} else if installedPath, installedBytes, ok := installedVADModelPath(spec, selectedPath, searchDirs); ok {
			info.Installed = true
			info.InstalledPath = installedPath
			info.InstalledBytes = installedBytes
		}
		knownModels = append(knownModels, info)
	}

	return core.VADModelsResponse{
		ModelDirectory:    modelDir,
		SelectedModelPath: selectedPath,
		SelectedModelID:   vadModelIDFromPath(selectedPath),
		Models:            knownModels,
	}, nil
}

func (s *VADModelService) InstallProgress(modelID string) core.VADModelInstallProgressResponse {
	id := strings.TrimSpace(modelID)
	if id == "" {
		return core.VADModelInstallProgressResponse{ModelID: "", Active: false, Stage: "idle", ProgressPercent: 0}
	}

	s.progressMu.RLock()
	progress, ok := s.progress[id]
	s.progressMu.RUnlock()
	if ok {
		return progress
	}
	return core.VADModelInstallProgressResponse{ModelID: id, Active: false, Stage: "idle", ProgressPercent: 0}
}

func (s *VADModelService) setInstallProgress(progress core.VADModelInstallProgressResponse) {
	progress.ModelID = strings.TrimSpace(progress.ModelID)
	if progress.ModelID == "" {
		return
	}
	if progress.Stage == "" {
		progress.Stage = "idle"
	}
	if progress.ProgressPercent < 0 {
		progress.ProgressPercent = 0
	}
	if progress.ProgressPercent > 100 {
		progress.ProgressPercent = 100
	}
	progress.UpdatedAt = time.Now().UTC()

	s.progressMu.Lock()
	s.progress[progress.ModelID] = progress
	s.progressMu.Unlock()
}

func (s *VADModelService) InstallModel(ctx context.Context, modelID string) (core.VADModelInfoDTO, string, error) {
	spec, ok := vadModelSpecByID(modelID)
	if !ok {
		return core.VADModelInfoDTO{}, "", fmt.Errorf("modele VAD inconnu: %s", strings.TrimSpace(modelID))
	}
	progress := core.VADModelInstallProgressResponse{ModelID: spec.ID, Active: true, Stage: "preparing", Message: "Préparation de l'installation...", ProgressPercent: 0}
	s.setInstallProgress(progress)

	modelDir, err := vadModelDirectory()
	if err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", err
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", err
	}

	finalPath := filepath.Join(modelDir, spec.FileName)
	if size, ok := existingFileSize(finalPath); ok {
		info := vadModelInfoFromSpec(spec)
		info.Installed = true
		info.ManagedByApp = true
		info.InstalledPath = finalPath
		info.InstalledBytes = size
		progress.Active = false
		progress.Stage = "completed"
		progress.Message = "Modèle déjà installé."
		progress.DownloadedBytes = size
		progress.TotalBytes = size
		progress.ProgressPercent = 100
		s.setInstallProgress(progress)
		return info, "Modele VAD deja installe.", nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tmpFile, err := os.CreateTemp(modelDir, spec.FileName+".*.download")
	if err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", err
	}
	tmpPath := tmpFile.Name()
	success := false
	defer func() {
		_ = tmpFile.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.DownloadURL, nil)
	if err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", err
	}
	req.Header.Set("User-Agent", "21loader/vad-model-installer")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = fmt.Sprintf("Téléchargement échoué (HTTP %d)", resp.StatusCode)
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", fmt.Errorf("telechargement echoue (HTTP %d)", resp.StatusCode)
	}

	totalBytes := resp.ContentLength
	if totalBytes <= 0 && spec.ApproximateSizeMB > 0 {
		totalBytes = int64(spec.ApproximateSizeMB) * 1024 * 1024
	}
	progress.Stage = "downloading"
	progress.Message = "Téléchargement du modèle..."
	progress.TotalBytes = totalBytes
	progress.ProgressPercent = progressPercentFromBytes(0, totalBytes, progress.Stage)
	s.setInstallProgress(progress)

	var lastReported int64
	counter := &progressWriter{onWrite: func(written int64) {
		if written == lastReported || written-lastReported < 128*1024 {
			return
		}
		lastReported = written
		progress.DownloadedBytes = written
		progress.ProgressPercent = progressPercentFromBytes(written, totalBytes, progress.Stage)
		s.setInstallProgress(progress)
	}}

	written, err := io.CopyBuffer(io.MultiWriter(tmpFile, counter), resp.Body, make([]byte, 32*1024))
	if err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		progress.DownloadedBytes = written
		progress.TotalBytes = totalBytes
		progress.ProgressPercent = progressPercentFromBytes(written, totalBytes, progress.Stage)
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", err
	}
	if written <= 0 {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = "Fichier modèle vide"
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", fmt.Errorf("fichier modele VAD vide")
	}
	progress.DownloadedBytes = written
	progress.TotalBytes = maxInt64(totalBytes, written)
	progress.ProgressPercent = 99
	progress.Stage = "finalizing"
	progress.Message = "Finalisation de l'installation..."
	s.setInstallProgress(progress)
	if err := tmpFile.Close(); err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.VADModelInfoDTO{}, "", err
	}
	success = true

	info := vadModelInfoFromSpec(spec)
	info.Installed = true
	info.ManagedByApp = true
	info.InstalledPath = finalPath
	info.InstalledBytes = written

	progress.Active = false
	progress.Stage = "completed"
	progress.Message = "Modèle installé."
	progress.DownloadedBytes = written
	progress.TotalBytes = maxInt64(totalBytes, written)
	progress.ProgressPercent = 100
	s.setInstallProgress(progress)

	return info, "Modele VAD installe.", nil
}

func (s *VADModelService) UninstallModel(ctx context.Context, modelID string) (core.VADModelInfoDTO, string, string, error) {
	_ = ctx
	spec, ok := vadModelSpecByID(modelID)
	if !ok {
		return core.VADModelInfoDTO{}, "", "", fmt.Errorf("modele VAD inconnu: %s", strings.TrimSpace(modelID))
	}

	modelDir, err := vadModelDirectory()
	if err != nil {
		return core.VADModelInfoDTO{}, "", "", err
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return core.VADModelInfoDTO{}, "", "", err
	}

	managedPath := filepath.Join(modelDir, spec.FileName)
	removedPath := ""
	removed := false

	s.mu.Lock()
	removed, err = removeFileIfExists(managedPath)
	if err != nil {
		s.mu.Unlock()
		return core.VADModelInfoDTO{}, "", "", err
	}
	if removed {
		removedPath = managedPath
	}
	s.mu.Unlock()

	info := vadModelInfoFromSpec(spec)
	if size, ok := existingFileSize(managedPath); ok {
		info.Installed = true
		info.ManagedByApp = true
		info.InstalledPath = managedPath
		info.InstalledBytes = size
	} else if installedPath, installedBytes, ok := installedVADModelPath(spec, "", util.VADModelSearchDirs("")); ok {
		info.Installed = true
		info.InstalledPath = installedPath
		info.InstalledBytes = installedBytes
		info.ManagedByApp = samePath(installedPath, managedPath)
	}

	if removed {
		return info, "Modele VAD supprime.", removedPath, nil
	}
	return info, "Aucun modele VAD applicatif a supprimer.", "", nil
}

func vadModelDirectory() (string, error) {
	binDir := util.Loader21BinDir()
	if strings.TrimSpace(binDir) == "" {
		return "", fmt.Errorf("dossier applicatif introuvable")
	}
	return filepath.Join(binDir, "models", "vad"), nil
}

func vadModelSpecByID(modelID string) (vadModelSpec, bool) {
	needle := strings.ToLower(strings.TrimSpace(modelID))
	for _, spec := range vadModelCatalog {
		if strings.EqualFold(spec.ID, needle) {
			return spec, true
		}
	}
	return vadModelSpec{}, false
}

func vadModelInfoFromSpec(spec vadModelSpec) core.VADModelInfoDTO {
	return core.VADModelInfoDTO{
		ID:                spec.ID,
		Name:              spec.Name,
		FileName:          spec.FileName,
		DownloadURL:       spec.DownloadURL,
		ApproximateSizeMB: spec.ApproximateSizeMB,
	}
}

func installedVADModelPath(spec vadModelSpec, selectedPath string, searchDirs []string) (string, int64, bool) {
	candidates := make([]string, 0, len(searchDirs)+1)
	selectedPath = strings.TrimSpace(selectedPath)
	if selectedPath != "" && strings.EqualFold(filepath.Base(selectedPath), spec.FileName) {
		candidates = append(candidates, selectedPath)
	}
	for _, dir := range searchDirs {
		candidates = append(candidates, filepath.Join(dir, spec.FileName))
	}
	for _, candidate := range candidates {
		if size, ok := existingFileSize(candidate); ok {
			return candidate, size, true
		}
	}
	return "", 0, false
}

func vadModelIDFromPath(modelPath string) string {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(strings.TrimSpace(modelPath))))
	for _, spec := range vadModelCatalog {
		if strings.EqualFold(spec.FileName, base) {
			return spec.ID
		}
	}
	return ""
}
