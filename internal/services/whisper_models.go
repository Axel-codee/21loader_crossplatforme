package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"persodl-cross/internal/core"
	"persodl-cross/internal/util"
)

type whisperModelSpec struct {
	ID                string
	Name              string
	FileName          string
	DownloadURL       string
	ApproximateSizeMB int
}

var whisperModelCatalog = []whisperModelSpec{
	{ID: "tiny", Name: "Tiny (multilang)", FileName: "ggml-tiny.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin", ApproximateSizeMB: 75},
	{ID: "tiny.en", Name: "Tiny English", FileName: "ggml-tiny.en.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin", ApproximateSizeMB: 75},
	{ID: "base", Name: "Base (multilang)", FileName: "ggml-base.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin", ApproximateSizeMB: 142},
	{ID: "base.en", Name: "Base English", FileName: "ggml-base.en.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin", ApproximateSizeMB: 142},
	{ID: "small", Name: "Small (multilang)", FileName: "ggml-small.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin", ApproximateSizeMB: 466},
	{ID: "small.en", Name: "Small English", FileName: "ggml-small.en.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.en.bin", ApproximateSizeMB: 466},
	{ID: "medium", Name: "Medium (multilang)", FileName: "ggml-medium.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin", ApproximateSizeMB: 1500},
	{ID: "medium.en", Name: "Medium English", FileName: "ggml-medium.en.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.en.bin", ApproximateSizeMB: 1500},
	{ID: "large-v1", Name: "Large v1", FileName: "ggml-large-v1.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v1.bin", ApproximateSizeMB: 2900},
	{ID: "large-v2", Name: "Large v2", FileName: "ggml-large-v2.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v2.bin", ApproximateSizeMB: 2900},
	{ID: "large-v3", Name: "Large v3", FileName: "ggml-large-v3.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin", ApproximateSizeMB: 2900},
	{ID: "large-v3-turbo", Name: "Large v3 turbo", FileName: "ggml-large-v3-turbo.bin", DownloadURL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin", ApproximateSizeMB: 1700},
}

type WhisperModelService struct {
	httpClient *http.Client
	mu         sync.Mutex
	progressMu sync.RWMutex
	progress   map[string]core.WhisperModelInstallProgressResponse
}

func NewWhisperModelService() *WhisperModelService {
	return &WhisperModelService{
		httpClient: &http.Client{},
		progress:   map[string]core.WhisperModelInstallProgressResponse{},
	}
}

func (s *WhisperModelService) ListModels(selectedPath string) (core.WhisperModelsResponse, error) {
	modelDir, err := whisperModelDirectory()
	if err != nil {
		return core.WhisperModelsResponse{}, err
	}
	_ = os.MkdirAll(modelDir, 0o755)

	selectedPath = strings.TrimSpace(selectedPath)
	searchDirs := util.WhisperModelSearchDirs(selectedPath, "")
	knownModels := make([]core.WhisperModelInfoDTO, 0, len(whisperModelCatalog))
	for _, spec := range whisperModelCatalog {
		info := modelInfoFromSpec(spec)
		managedPath := filepath.Join(modelDir, spec.FileName)
		if managedBytes, ok := existingFileSize(managedPath); ok {
			info.Installed = true
			info.ManagedByApp = true
			info.InstalledPath = managedPath
			info.InstalledBytes = managedBytes
		} else if installedPath, installedBytes, ok := installedWhisperModelPath(spec, selectedPath, searchDirs); ok {
			info.Installed = true
			info.InstalledPath = installedPath
			info.InstalledBytes = installedBytes
		}
		knownModels = append(knownModels, info)
	}

	return core.WhisperModelsResponse{
		ModelDirectory:    modelDir,
		SelectedModelPath: selectedPath,
		SelectedModelID:   modelIDFromPath(selectedPath),
		Models:            knownModels,
	}, nil
}

func (s *WhisperModelService) InstallProgress(modelID string) core.WhisperModelInstallProgressResponse {
	id := strings.TrimSpace(modelID)
	if id == "" {
		return core.WhisperModelInstallProgressResponse{
			ModelID:         "",
			Active:          false,
			Stage:           "idle",
			ProgressPercent: 0,
		}
	}

	s.progressMu.RLock()
	progress, ok := s.progress[id]
	s.progressMu.RUnlock()
	if ok {
		return progress
	}

	return core.WhisperModelInstallProgressResponse{
		ModelID:         id,
		Active:          false,
		Stage:           "idle",
		ProgressPercent: 0,
	}
}

func (s *WhisperModelService) setInstallProgress(progress core.WhisperModelInstallProgressResponse) {
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

func progressPercentFromBytes(downloadedBytes, totalBytes int64, stage string) int {
	if totalBytes > 0 {
		percent := int((downloadedBytes * 100) / totalBytes)
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		if stage != "completed" && percent > 99 {
			percent = 99
		}
		return percent
	}
	if downloadedBytes > 0 {
		return 1
	}
	return 0
}

func (s *WhisperModelService) InstallModel(ctx context.Context, modelID string) (core.WhisperModelInfoDTO, string, error) {
	spec, ok := whisperModelSpecByID(modelID)
	if !ok {
		return core.WhisperModelInfoDTO{}, "", fmt.Errorf("modele inconnu: %s", strings.TrimSpace(modelID))
	}
	progress := core.WhisperModelInstallProgressResponse{
		ModelID:         spec.ID,
		Active:          true,
		Stage:           "preparing",
		Message:         "Préparation de l'installation...",
		ProgressPercent: 0,
	}
	s.setInstallProgress(progress)

	modelDir, err := whisperModelDirectory()
	if err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.WhisperModelInfoDTO{}, "", err
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.WhisperModelInfoDTO{}, "", err
	}

	finalPath := filepath.Join(modelDir, spec.FileName)
	if size, ok := existingFileSize(finalPath); ok {
		info := modelInfoFromSpec(spec)
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
		return info, "Modele deja installe.", nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if size, ok := existingFileSize(finalPath); ok {
		info := modelInfoFromSpec(spec)
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
		return info, "Modele deja installe.", nil
	}

	tmpFile, err := os.CreateTemp(modelDir, spec.FileName+".*.download")
	if err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.WhisperModelInfoDTO{}, "", err
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
		return core.WhisperModelInfoDTO{}, "", err
	}
	req.Header.Set("User-Agent", "PersoDL/whisper-model-installer")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.WhisperModelInfoDTO{}, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = fmt.Sprintf("Téléchargement échoué (HTTP %d)", resp.StatusCode)
		s.setInstallProgress(progress)
		return core.WhisperModelInfoDTO{}, "", fmt.Errorf("telechargement echoue (HTTP %d)", resp.StatusCode)
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
	counter := &progressWriter{
		onWrite: func(written int64) {
			if written == lastReported {
				return
			}
			if written-lastReported < 8*1024*1024 {
				return
			}
			lastReported = written
			progress.DownloadedBytes = written
			progress.ProgressPercent = progressPercentFromBytes(written, totalBytes, progress.Stage)
			s.setInstallProgress(progress)
		},
	}

	written, err := io.CopyBuffer(io.MultiWriter(tmpFile, counter), resp.Body, make([]byte, 128*1024))
	if err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		progress.DownloadedBytes = written
		progress.TotalBytes = totalBytes
		progress.ProgressPercent = progressPercentFromBytes(written, totalBytes, progress.Stage)
		s.setInstallProgress(progress)
		return core.WhisperModelInfoDTO{}, "", err
	}
	if written <= 0 {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = "Fichier modèle vide"
		progress.DownloadedBytes = 0
		progress.TotalBytes = totalBytes
		progress.ProgressPercent = 0
		s.setInstallProgress(progress)
		return core.WhisperModelInfoDTO{}, "", fmt.Errorf("fichier modele vide")
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
		return core.WhisperModelInfoDTO{}, "", err
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		progress.Active = false
		progress.Stage = "failed"
		progress.Message = err.Error()
		s.setInstallProgress(progress)
		return core.WhisperModelInfoDTO{}, "", err
	}
	success = true

	info := modelInfoFromSpec(spec)
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

	return info, "Modele installe.", nil
}

func (s *WhisperModelService) UninstallModel(ctx context.Context, modelID string) (core.WhisperModelInfoDTO, string, string, error) {
	_ = ctx
	spec, ok := whisperModelSpecByID(modelID)
	if !ok {
		return core.WhisperModelInfoDTO{}, "", "", fmt.Errorf("modele inconnu: %s", strings.TrimSpace(modelID))
	}

	modelDir, err := whisperModelDirectory()
	if err != nil {
		return core.WhisperModelInfoDTO{}, "", "", err
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return core.WhisperModelInfoDTO{}, "", "", err
	}

	managedPath := filepath.Join(modelDir, spec.FileName)
	removedPath := ""
	removed := false

	s.mu.Lock()
	removed, err = removeFileIfExists(managedPath)
	if err != nil {
		s.mu.Unlock()
		return core.WhisperModelInfoDTO{}, "", "", err
	}
	if removed {
		removedPath = managedPath
	}
	s.mu.Unlock()

	info := modelInfoFromSpec(spec)
	if size, ok := existingFileSize(managedPath); ok {
		info.Installed = true
		info.ManagedByApp = true
		info.InstalledPath = managedPath
		info.InstalledBytes = size
	} else if installedPath, installedBytes, ok := installedWhisperModelPath(spec, "", util.WhisperModelSearchDirs("", "")); ok {
		info.Installed = true
		info.InstalledPath = installedPath
		info.InstalledBytes = installedBytes
		info.ManagedByApp = samePath(installedPath, managedPath)
	}

	if removed {
		return info, "Modele supprime.", removedPath, nil
	}
	return info, "Aucun modele applicatif a supprimer.", "", nil
}

func whisperModelDirectory() (string, error) {
	binDir := util.PersoDLBinDir()
	if strings.TrimSpace(binDir) == "" {
		return "", fmt.Errorf("dossier applicatif introuvable")
	}
	return filepath.Join(binDir, "models"), nil
}

func whisperModelSpecByID(modelID string) (whisperModelSpec, bool) {
	needle := strings.ToLower(strings.TrimSpace(modelID))
	for _, spec := range whisperModelCatalog {
		if strings.EqualFold(spec.ID, needle) {
			return spec, true
		}
	}
	return whisperModelSpec{}, false
}

func modelInfoFromSpec(spec whisperModelSpec) core.WhisperModelInfoDTO {
	return core.WhisperModelInfoDTO{
		ID:                spec.ID,
		Name:              spec.Name,
		FileName:          spec.FileName,
		DownloadURL:       spec.DownloadURL,
		ApproximateSizeMB: spec.ApproximateSizeMB,
		Installed:         false,
		ManagedByApp:      false,
	}
}

func installedWhisperModelPath(spec whisperModelSpec, selectedPath string, searchDirs []string) (string, int64, bool) {
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

func modelIDFromPath(modelPath string) string {
	modelPath = strings.TrimSpace(modelPath)
	if modelPath == "" {
		return ""
	}
	base := strings.ToLower(strings.TrimSpace(filepath.Base(modelPath)))
	for _, spec := range whisperModelCatalog {
		if strings.EqualFold(spec.FileName, base) {
			return spec.ID
		}
	}
	return ""
}

func existingFileSize(path string) (int64, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0, false
	}
	if info.Size() <= 0 {
		return 0, false
	}
	return info.Size(), true
}

func removeFileIfExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("chemin modele invalide (dossier): %s", path)
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type progressWriter struct {
	onWrite func(written int64)
	written int64
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	if n <= 0 {
		return 0, nil
	}
	w.written += int64(n)
	if w.onWrite != nil {
		w.onWrite(w.written)
	}
	return n, nil
}

func maxInt64(left, right int64) int64 {
	if left >= right {
		return left
	}
	return right
}

func samePath(left, right string) bool {
	l := strings.TrimSpace(left)
	r := strings.TrimSpace(right)
	if l == "" || r == "" {
		return false
	}
	l = filepath.Clean(l)
	r = filepath.Clean(r)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(l, r)
	}
	return l == r
}
