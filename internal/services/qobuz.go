package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"persodl-cross/internal/core"
	"persodl-cross/internal/sys"
	"persodl-cross/internal/util"
)

const qobuzJSONMarker = "__PERSODL_QOBUZ_JSON__"

type QobuzService struct {
	runner       *sys.Runner
	artistScript string
	searchScript string
	tracksScript string
}

func NewQobuzService(r *sys.Runner, baseDir string) *QobuzService {
	return &QobuzService{
		runner:       r,
		artistScript: filepath.Join(baseDir, "assets", "scripts", "qobuz_artist_catalog.py"),
		searchScript: filepath.Join(baseDir, "assets", "scripts", "qobuz_artist_search.py"),
		tracksScript: filepath.Join(baseDir, "assets", "scripts", "qobuz_album_tracks.py"),
	}
}

type qobuzArtistScriptOutput struct {
	ArtistName string `json:"artist_name"`
	Albums     []struct {
		ID               string `json:"id"`
		Title            string `json:"title"`
		ArtistName       string `json:"artist_name"`
		URL              string `json:"url"`
		ReleaseTimestamp *int64 `json:"release_timestamp"`
		TracksCount      *int   `json:"tracks_count"`
		ReleaseKind      string `json:"release_kind"`
		IsHiRes          bool   `json:"is_hires"`
		CoverURL         string `json:"cover_url"`
	} `json:"albums"`
}

type qobuzTracksScriptOutput struct {
	Tracks []struct {
		ID              string `json:"id"`
		TrackNumber     *int   `json:"track_number"`
		Title           string `json:"title"`
		DurationSeconds *int   `json:"duration_seconds"`
	} `json:"tracks"`
}

type qobuzArtistSearchScriptOutput struct {
	Artists []struct {
		ID                     string   `json:"id"`
		Name                   string   `json:"name"`
		URL                    string   `json:"url"`
		AlbumsCount            *int     `json:"albums_count"`
		CatalogAlbumsCount     *int     `json:"catalog_albums_count"`
		ImageURL               string   `json:"image_url"`
		Slug                   string   `json:"slug"`
		Country                string   `json:"country"`
		Genres                 []string `json:"genres"`
		Biography              string   `json:"biography"`
		LatestReleaseTitle     string   `json:"latest_release_title"`
		LatestReleaseTimestamp *int64   `json:"latest_release_timestamp"`
	} `json:"artists"`
}

func (s *QobuzService) SearchArtists(ctx context.Context, query string, limit int, qobuzEmail, qobuzPassword string) (core.QobuzArtistSearchAPIResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return core.QobuzArtistSearchAPIResponse{}, fmt.Errorf("recherche artiste Qobuz invalide")
	}
	if limit <= 0 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}
	if err := s.ensureConfigured(ctx, qobuzEmail, qobuzPassword); err != nil {
		return core.QobuzArtistSearchAPIResponse{}, err
	}
	python, err := resolveQobuzPythonRuntime()
	if err != nil {
		return core.QobuzArtistSearchAPIResponse{}, err
	}
	output, err := s.runner.Run(ctx, sys.RunOptions{
		Executable:    python,
		Args:          []string{s.searchScript, query, strconv.Itoa(limit)},
		CaptureOutput: true,
	})
	if err != nil {
		return core.QobuzArtistSearchAPIResponse{}, fmt.Errorf("la recherche d'artistes Qobuz a echoue: %w", err)
	}
	jsonPayload, ok := extractQobuzJSON(output)
	if !ok {
		return core.QobuzArtistSearchAPIResponse{}, fmt.Errorf("impossible de lire les resultats de recherche Qobuz")
	}
	var parsed qobuzArtistSearchScriptOutput
	if err := json.Unmarshal([]byte(jsonPayload), &parsed); err != nil {
		return core.QobuzArtistSearchAPIResponse{}, fmt.Errorf("impossible de lire les resultats de recherche Qobuz")
	}
	artists := make([]core.QobuzArtistSearchResultDTO, 0, len(parsed.Artists))
	seen := map[string]bool{}
	for _, item := range parsed.Artists {
		id := strings.TrimSpace(item.ID)
		url := strings.TrimSpace(item.URL)
		if id == "" || url == "" || seen[id] {
			continue
		}
		seen[id] = true
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = "Artiste " + id
		}
		var latestReleaseDate *time.Time
		if item.LatestReleaseTimestamp != nil && *item.LatestReleaseTimestamp > 0 {
			t := time.Unix(*item.LatestReleaseTimestamp, 0).UTC()
			latestReleaseDate = &t
		}
		genres := make([]string, 0, len(item.Genres))
		genreSeen := map[string]bool{}
		for _, rawGenre := range item.Genres {
			genre := strings.TrimSpace(rawGenre)
			key := strings.ToLower(genre)
			if genre == "" || genreSeen[key] {
				continue
			}
			genreSeen[key] = true
			genres = append(genres, genre)
		}
		artists = append(artists, core.QobuzArtistSearchResultDTO{
			ID:                 id,
			Name:               name,
			WebpageURL:         url,
			AlbumsCount:        item.AlbumsCount,
			CatalogAlbumsCount: item.CatalogAlbumsCount,
			ArtworkURL:         strings.TrimSpace(item.ImageURL),
			Slug:               strings.TrimSpace(item.Slug),
			Country:            strings.TrimSpace(item.Country),
			Genres:             genres,
			Biography:          strings.TrimSpace(item.Biography),
			LatestReleaseTitle: strings.TrimSpace(item.LatestReleaseTitle),
			LatestReleaseDate:  latestReleaseDate,
		})
	}
	if len(artists) == 0 {
		return core.QobuzArtistSearchAPIResponse{}, fmt.Errorf("aucun artiste Qobuz trouve pour cette recherche")
	}
	return core.QobuzArtistSearchAPIResponse{Artists: artists}, nil
}

func (s *QobuzService) FetchArtistCatalog(ctx context.Context, artistURL, qobuzEmail, qobuzPassword string) (core.QobuzArtistCatalogAPIResponse, error) {
	rt, ok := util.QobuzResourceTypeFromURL(artistURL)
	if !ok || rt != util.QobuzArtist {
		return core.QobuzArtistCatalogAPIResponse{}, fmt.Errorf("URL artiste Qobuz invalide")
	}
	artistID, ok := util.QobuzResourceIdentifier(artistURL)
	if !ok || artistID == "" {
		return core.QobuzArtistCatalogAPIResponse{}, fmt.Errorf("URL artiste Qobuz invalide")
	}
	if err := s.ensureConfigured(ctx, qobuzEmail, qobuzPassword); err != nil {
		return core.QobuzArtistCatalogAPIResponse{}, err
	}
	python, err := resolveQobuzPythonRuntime()
	if err != nil {
		return core.QobuzArtistCatalogAPIResponse{}, err
	}
	output, err := s.runner.Run(ctx, sys.RunOptions{
		Executable:    python,
		Args:          []string{s.artistScript, artistID},
		CaptureOutput: true,
	})
	if err != nil {
		return core.QobuzArtistCatalogAPIResponse{}, fmt.Errorf("la recuperation de la discographie Qobuz a echoue: %w", err)
	}
	jsonPayload, ok := extractQobuzJSON(output)
	if !ok {
		return core.QobuzArtistCatalogAPIResponse{}, fmt.Errorf("impossible de lire la discographie Qobuz")
	}
	var parsed qobuzArtistScriptOutput
	if err := json.Unmarshal([]byte(jsonPayload), &parsed); err != nil {
		return core.QobuzArtistCatalogAPIResponse{}, fmt.Errorf("impossible de lire la discographie Qobuz")
	}
	albums := make([]core.QobuzAlbumDTO, 0, len(parsed.Albums))
	for _, a := range parsed.Albums {
		if strings.TrimSpace(a.ID) == "" {
			continue
		}
		var releaseDate *time.Time
		if a.ReleaseTimestamp != nil {
			t := time.Unix(*a.ReleaseTimestamp, 0).UTC()
			releaseDate = &t
		}
		albums = append(albums, core.QobuzAlbumDTO{
			ID:               strings.TrimSpace(a.ID),
			Title:            strings.TrimSpace(a.Title),
			ArtistName:       strings.TrimSpace(a.ArtistName),
			WebpageURL:       strings.TrimSpace(a.URL),
			ReleaseDate:      releaseDate,
			TrackCount:       a.TracksCount,
			ReleaseKindLabel: strings.TrimSpace(a.ReleaseKind),
			IsHiRes:          a.IsHiRes,
			ArtworkURL:       strings.TrimSpace(a.CoverURL),
		})
	}
	if len(albums) == 0 {
		return core.QobuzArtistCatalogAPIResponse{}, fmt.Errorf("aucun album trouve pour cet artiste Qobuz")
	}
	artist := strings.TrimSpace(parsed.ArtistName)
	if artist == "" {
		artist = "Artiste inconnu"
	}
	return core.QobuzArtistCatalogAPIResponse{ArtistName: artist, Albums: albums}, nil
}

func (s *QobuzService) FetchAlbumTracks(ctx context.Context, albumID, qobuzEmail, qobuzPassword string) (core.QobuzAlbumTracksAPIResponse, error) {
	albumID = strings.TrimSpace(albumID)
	if albumID == "" {
		return core.QobuzAlbumTracksAPIResponse{}, fmt.Errorf("identifiant d'album Qobuz invalide")
	}
	if err := s.ensureConfigured(ctx, qobuzEmail, qobuzPassword); err != nil {
		return core.QobuzAlbumTracksAPIResponse{}, err
	}
	python, err := resolveQobuzPythonRuntime()
	if err != nil {
		return core.QobuzAlbumTracksAPIResponse{}, err
	}
	output, err := s.runner.Run(ctx, sys.RunOptions{
		Executable:    python,
		Args:          []string{s.tracksScript, albumID},
		CaptureOutput: true,
	})
	if err != nil {
		return core.QobuzAlbumTracksAPIResponse{}, fmt.Errorf("la recuperation des titres Qobuz a echoue: %w", err)
	}
	jsonPayload, ok := extractQobuzJSON(output)
	if !ok {
		return core.QobuzAlbumTracksAPIResponse{}, fmt.Errorf("impossible de lire la liste des titres de l'album Qobuz")
	}
	var parsed qobuzTracksScriptOutput
	if err := json.Unmarshal([]byte(jsonPayload), &parsed); err != nil {
		return core.QobuzAlbumTracksAPIResponse{}, fmt.Errorf("impossible de lire la liste des titres de l'album Qobuz")
	}
	tracks := make([]core.QobuzTrackDTO, 0, len(parsed.Tracks))
	for i, t := range parsed.Tracks {
		title := strings.TrimSpace(t.Title)
		if title == "" {
			title = fmt.Sprintf("Titre %d", i+1)
		}
		id := strings.TrimSpace(t.ID)
		if id == "" {
			id = fmt.Sprintf("%s-%d", albumID, i+1)
		}
		tracks = append(tracks, core.QobuzTrackDTO{
			ID:              id,
			TrackNumber:     t.TrackNumber,
			Title:           title,
			DurationSeconds: t.DurationSeconds,
		})
	}
	return core.QobuzAlbumTracksAPIResponse{AlbumID: albumID, Tracks: tracks}, nil
}

func (s *QobuzService) ensureConfigured(ctx context.Context, email, password string) error {
	cfgFile, err := qobuzConfigFilePath()
	if err == nil {
		if _, err := os.Stat(cfgFile); err == nil {
			return nil
		}
	}
	if strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
		return fmt.Errorf("qobuz-dl n'est pas configure. Renseigne email/mot de passe Qobuz dans Reglages")
	}
	stdin := strings.TrimSpace(email) + "\n" + strings.TrimSpace(password) + "\n\n27\n"
	_, err = s.runner.Run(ctx, sys.RunOptions{
		Executable:    "qobuz-dl",
		Args:          []string{"-r"},
		StandardInput: stdin,
		CaptureOutput: true,
	})
	if err != nil {
		return fmt.Errorf("impossible d'initialiser la configuration qobuz-dl")
	}
	if cfgFile != "" {
		if _, err := os.Stat(cfgFile); err != nil {
			return fmt.Errorf("impossible d'initialiser la configuration qobuz-dl")
		}
	}
	return nil
}

func qobuzConfigFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		cfg := os.Getenv("APPDATA")
		if cfg != "" {
			return filepath.Join(cfg, "qobuz-dl", "config.ini"), nil
		}
	}
	return filepath.Join(home, ".config", "qobuz-dl", "config.ini"), nil
}

func resolveQobuzPythonRuntime() (string, error) {
	path, err := exec.LookPath("qobuz-dl")
	if err != nil {
		return "", fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if !s.Scan() {
		return "", fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
	}
	line := strings.TrimSpace(s.Text())
	if !strings.HasPrefix(line, "#!") {
		return "", fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
	}
	interpreter := strings.TrimSpace(strings.TrimPrefix(line, "#!"))
	if interpreter == "" {
		return "", fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
	}
	parts := strings.Split(interpreter, " ")
	candidate := strings.TrimSpace(parts[0])
	if candidate == "" {
		return "", fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
	}
	if strings.HasSuffix(candidate, "env") && len(parts) >= 2 {
		candidate = strings.TrimSpace(parts[1])
	}
	if candidate == "" {
		return "", fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
	}
	if strings.Contains(candidate, string(os.PathSeparator)) {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	resolved, err := exec.LookPath(candidate)
	if err != nil {
		return "", fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
	}
	return resolved, nil
}

func extractQobuzJSON(output string) (string, bool) {
	if idx := strings.LastIndex(output, qobuzJSONMarker); idx >= 0 {
		payload := strings.TrimSpace(output[idx+len(qobuzJSONMarker):])
		if payload != "" {
			return payload, true
		}
	}
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start >= 0 && end > start {
		payload := strings.TrimSpace(output[start : end+1])
		if payload != "" {
			return payload, true
		}
	}
	return "", false
}
