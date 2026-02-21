package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"persodl-cross/internal/core"
	"persodl-cross/internal/jobs"
	"persodl-cross/internal/services"
	"persodl-cross/internal/xuuid"
)

type Server struct {
	coordinator       *jobs.Coordinator
	artworkThumbnails *services.ArtworkThumbnailService
	indexHTML         []byte
	mux               *http.ServeMux
}

func NewServer(coordinator *jobs.Coordinator, artworkThumbnails *services.ArtworkThumbnailService, baseDir string) (*Server, error) {
	indexPath := filepath.Join(baseDir, "web", "index.html")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	s := &Server{
		coordinator:       coordinator,
		artworkThumbnails: artworkThumbnails,
		indexHTML:         data,
		mux:               http.NewServeMux(),
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.mux.ServeHTTP(w, r)
	})
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/", s.handleRoot)
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/settings", s.handleSettings)
	s.mux.HandleFunc("/api/diagnostics", s.handleDiagnostics)
	s.mux.HandleFunc("/api/dependencies/install", s.handleInstallDependencies)
	s.mux.HandleFunc("/api/dependencies/install-progress", s.handleDependencyInstallProgress)
	s.mux.HandleFunc("/api/translation/languages", s.handleTranslationLanguages)
	s.mux.HandleFunc("/api/translation/languages/install", s.handleInstallTranslationLanguage)
	s.mux.HandleFunc("/api/system/select-directory", s.handleSelectDirectory)
	s.mux.HandleFunc("/api/whisper/models", s.handleWhisperModels)
	s.mux.HandleFunc("/api/whisper/models/install-progress", s.handleWhisperModelInstallProgress)
	s.mux.HandleFunc("/api/whisper/models/install", s.handleInstallWhisperModel)
	s.mux.HandleFunc("/api/whisper/models/uninstall", s.handleUninstallWhisperModel)
	s.mux.HandleFunc("/api/qobuz/search-artists", s.handleQobuzArtistSearch)
	s.mux.HandleFunc("/api/qobuz/artist-catalog", s.handleQobuzArtistCatalog)
	s.mux.HandleFunc("/api/qobuz/album-tracks", s.handleQobuzAlbumTracks)
	s.mux.HandleFunc("/api/rss/episodes", s.handleRSSEpisodes)
	s.mux.HandleFunc("/api/youtube/catalog", s.handleYouTubeCatalog)
	s.mux.HandleFunc("/api/youtube/dates", s.handleYouTubeDates)
	s.mux.HandleFunc("/api/artwork", s.handleArtwork)
	s.mux.HandleFunc("/api/rss/artwork", s.handleArtwork)
	s.mux.HandleFunc("/api/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/jobs/", s.handleJobByID)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.indexHTML)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	writeJSON(w, http.StatusOK, core.HealthResponseDTO{Status: "ok", Time: time.Now().UTC()})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	writeJSON(w, http.StatusOK, s.coordinator.Dashboard())
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.coordinator.CurrentSettings())
	case http.MethodPut:
		var payload core.UpdateSettingsAPIRequest
		if err := decodeJSON(r, &payload); err != nil {
			errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.coordinator.UpdateSettings(payload))
	default:
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
	}
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	writeJSON(w, http.StatusOK, s.coordinator.Diagnostics(r.Context()))
}

func (s *Server) handleInstallDependencies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.DependencyInstallRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	resp, err := s.coordinator.InstallDependencies(r.Context(), payload)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDependencyInstallProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	writeJSON(w, http.StatusOK, s.coordinator.DependencyInstallProgress(r.Context()))
}

func (s *Server) handleTranslationLanguages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	resp, err := s.coordinator.TranslationLanguages(r.Context())
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleInstallTranslationLanguage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.TranslationLanguageInstallRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	resp, err := s.coordinator.InstallTranslationLanguage(r.Context(), payload)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWhisperModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	resp, err := s.coordinator.WhisperModels(r.Context())
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWhisperModelInstallProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	modelID := strings.TrimSpace(r.URL.Query().Get("modelID"))
	resp, err := s.coordinator.WhisperModelInstallProgress(r.Context(), core.WhisperModelInstallProgressRequest{
		ModelID: modelID,
	})
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleInstallWhisperModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.WhisperModelInstallRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	resp, err := s.coordinator.InstallWhisperModel(r.Context(), payload)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUninstallWhisperModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.WhisperModelUninstallRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	resp, err := s.coordinator.UninstallWhisperModel(r.Context(), payload)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleQobuzArtistCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.QobuzArtistCatalogAPIRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	catalog, err := s.coordinator.FetchQobuzArtistCatalog(r.Context(), payload)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) handleQobuzArtistSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.QobuzArtistSearchAPIRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	artists, err := s.coordinator.SearchQobuzArtists(r.Context(), payload)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, artists)
}

func (s *Server) handleQobuzAlbumTracks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.QobuzAlbumTracksAPIRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	tracks, err := s.coordinator.FetchQobuzAlbumTracks(r.Context(), payload)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) handleRSSEpisodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.RSSFeedEpisodesAPIRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	out, err := s.coordinator.FetchRSSEpisodes(r.Context(), payload)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleYouTubeCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.YouTubeCatalogAPIRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	out, err := s.coordinator.FetchYouTubeCatalog(r.Context(), payload)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleYouTubeDates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	var payload core.YouTubeDatesAPIRequest
	if err := decodeJSON(r, &payload); err != nil {
		errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.coordinator.FetchYouTubeDates(r.Context(), payload))
}

func (s *Server) handleArtwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
		return
	}
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		errorJSON(w, http.StatusBadRequest, "Parametre url manquant.")
		return
	}
	size := 96
	if parsed, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("size"))); err == nil && parsed > 0 {
		size = parsed
	}
	if s.artworkThumbnails == nil {
		errorJSON(w, http.StatusBadRequest, "Service de miniatures indisponible.")
		return
	}

	thumbnail, err := s.artworkThumbnails.ThumbnailData(r.Context(), rawURL, size)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	contentType := strings.TrimSpace(thumbnail.ContentType)
	if contentType == "" {
		contentType = "image/jpeg"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
	_, _ = w.Write(thumbnail.Data)
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.coordinator.ListJobs())
	case http.MethodPost:
		var payload core.CreateJobAPIRequest
		if err := decodeJSON(r, &payload); err != nil {
			errorJSON(w, http.StatusBadRequest, "JSON invalide: "+err.Error())
			return
		}
		created, err := s.coordinator.Enqueue(r.Context(), payload)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		errorJSON(w, http.StatusMethodNotAllowed, "Methode non autorisee")
	}
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 || strings.TrimSpace(parts[0]) == "" {
		errorJSON(w, http.StatusNotFound, "Route introuvable.")
		return
	}
	id, err := xuuid.Parse(parts[0])
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "Identifiant de job invalide.")
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		if job := s.coordinator.Job(id); job != nil {
			writeJSON(w, http.StatusOK, job)
			return
		}
		errorJSON(w, http.StatusNotFound, "Job introuvable.")
		return
	}
	if len(parts) == 2 && parts[1] == "logs" && r.Method == http.MethodGet {
		if logs, ok := s.coordinator.Logs(id); ok {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(logs))
			return
		}
		errorJSON(w, http.StatusNotFound, "Job introuvable.")
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "cancel":
			writeJSON(w, http.StatusOK, s.coordinator.Cancel(id))
			return
		case "pause":
			writeJSON(w, http.StatusOK, s.coordinator.Pause(id))
			return
		case "resume":
			writeJSON(w, http.StatusOK, s.coordinator.Resume(id))
			return
		}
	}
	errorJSON(w, http.StatusNotFound, "Route introuvable.")
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return fmt.Errorf("corps JSON requis")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 4*1024*1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}

func errorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, core.ErrorResponseDTO{Error: message})
}
