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

	"21loader-cross/internal/core"
	"21loader-cross/internal/sys"
	"21loader-cross/internal/util"
)

const qobuzJSONMarker = "__LOADER21_QOBUZ_JSON__"

type QobuzService struct {
	runner         *sys.Runner
	artistScript   string
	searchScript   string
	tracksScript   string
	playlistScript string
}

func NewQobuzService(r *sys.Runner, baseDir string) *QobuzService {
	return &QobuzService{
		runner:         r,
		artistScript:   filepath.Join(baseDir, "assets", "scripts", "qobuz_artist_catalog.py"),
		searchScript:   filepath.Join(baseDir, "assets", "scripts", "qobuz_artist_search.py"),
		tracksScript:   filepath.Join(baseDir, "assets", "scripts", "qobuz_album_tracks.py"),
		playlistScript: filepath.Join(baseDir, "assets", "scripts", "qobuz_playlist_catalog.py"),
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

type qobuzPlaylistScriptOutput struct {
	PlaylistID   string `json:"playlist_id"`
	PlaylistName string `json:"playlist_name"`
	URL          string `json:"url"`
	TracksCount  *int   `json:"tracks_count"`
	Tracks       []struct {
		ID              string `json:"id"`
		Position        *int   `json:"position"`
		Title           string `json:"title"`
		DurationSeconds *int   `json:"duration_seconds"`
		ArtistID        string `json:"artist_id"`
		ArtistName      string `json:"artist_name"`
		ArtistURL       string `json:"artist_url"`
		AlbumID         string `json:"album_id"`
		AlbumTitle      string `json:"album_title"`
		AlbumURL        string `json:"album_url"`
	} `json:"tracks"`
	Albums []struct {
		ID               string `json:"id"`
		Title            string `json:"title"`
		ArtistID         string `json:"artist_id"`
		ArtistName       string `json:"artist_name"`
		URL              string `json:"url"`
		ReleaseTimestamp *int64 `json:"release_timestamp"`
		TracksCount      *int   `json:"tracks_count"`
		ReleaseKind      string `json:"release_kind"`
		IsHiRes          bool   `json:"is_hires"`
		CoverURL         string `json:"cover_url"`
		TracksInPlaylist *int   `json:"tracks_in_playlist"`
	} `json:"albums"`
	Artists []struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		URL              string `json:"url"`
		TracksInPlaylist *int   `json:"tracks_in_playlist"`
		AlbumsInPlaylist *int   `json:"albums_in_playlist"`
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
	args := append(append([]string{}, python.PrefixArgs...), s.searchScript, query, strconv.Itoa(limit))
	output, err := s.runner.Run(ctx, sys.RunOptions{
		Executable:    python.Exec,
		Args:          args,
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
	args := append(append([]string{}, python.PrefixArgs...), s.artistScript, artistID)
	output, err := s.runner.Run(ctx, sys.RunOptions{
		Executable:    python.Exec,
		Args:          args,
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
	args := append(append([]string{}, python.PrefixArgs...), s.tracksScript, albumID)
	output, err := s.runner.Run(ctx, sys.RunOptions{
		Executable:    python.Exec,
		Args:          args,
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

func (s *QobuzService) FetchPlaylistCatalog(ctx context.Context, playlistURL, qobuzEmail, qobuzPassword string) (core.QobuzPlaylistCatalogAPIResponse, error) {
	rt, ok := util.QobuzResourceTypeFromURL(playlistURL)
	if !ok || rt != util.QobuzPlaylist {
		return core.QobuzPlaylistCatalogAPIResponse{}, fmt.Errorf("URL playlist Qobuz invalide")
	}
	playlistID, ok := util.QobuzResourceIdentifier(playlistURL)
	if !ok || playlistID == "" {
		return core.QobuzPlaylistCatalogAPIResponse{}, fmt.Errorf("URL playlist Qobuz invalide")
	}
	if err := s.ensureConfigured(ctx, qobuzEmail, qobuzPassword); err != nil {
		return core.QobuzPlaylistCatalogAPIResponse{}, err
	}
	python, err := resolveQobuzPythonRuntime()
	if err != nil {
		return core.QobuzPlaylistCatalogAPIResponse{}, err
	}
	args := append(append([]string{}, python.PrefixArgs...), s.playlistScript, playlistID)
	output, err := s.runner.Run(ctx, sys.RunOptions{
		Executable:    python.Exec,
		Args:          args,
		CaptureOutput: true,
	})
	if err != nil {
		return core.QobuzPlaylistCatalogAPIResponse{}, fmt.Errorf("la recuperation de la playlist Qobuz a echoue: %w", err)
	}
	jsonPayload, ok := extractQobuzJSON(output)
	if !ok {
		return core.QobuzPlaylistCatalogAPIResponse{}, fmt.Errorf("impossible de lire la playlist Qobuz")
	}
	var parsed qobuzPlaylistScriptOutput
	if err := json.Unmarshal([]byte(jsonPayload), &parsed); err != nil {
		return core.QobuzPlaylistCatalogAPIResponse{}, fmt.Errorf("impossible de lire la playlist Qobuz")
	}

	tracks := make([]core.QobuzPlaylistTrackDTO, 0, len(parsed.Tracks))
	for i, t := range parsed.Tracks {
		id := strings.TrimSpace(t.ID)
		if id == "" {
			id = fmt.Sprintf("track-%d", i+1)
		}
		title := strings.TrimSpace(t.Title)
		if title == "" {
			title = fmt.Sprintf("Titre %d", i+1)
		}
		artistName := strings.TrimSpace(t.ArtistName)
		if artistName == "" {
			artistName = "Artiste inconnu"
		}
		tracks = append(tracks, core.QobuzPlaylistTrackDTO{
			ID:               id,
			Position:         t.Position,
			Title:            title,
			DurationSeconds:  t.DurationSeconds,
			ArtistID:         strings.TrimSpace(t.ArtistID),
			ArtistName:       artistName,
			ArtistWebpageURL: strings.TrimSpace(t.ArtistURL),
			AlbumID:          strings.TrimSpace(t.AlbumID),
			AlbumTitle:       strings.TrimSpace(t.AlbumTitle),
			AlbumWebpageURL:  strings.TrimSpace(t.AlbumURL),
		})
	}

	albums := make([]core.QobuzAlbumDTO, 0, len(parsed.Albums))
	for _, a := range parsed.Albums {
		id := strings.TrimSpace(a.ID)
		if id == "" {
			continue
		}
		var releaseDate *time.Time
		if a.ReleaseTimestamp != nil && *a.ReleaseTimestamp > 0 {
			t := time.Unix(*a.ReleaseTimestamp, 0).UTC()
			releaseDate = &t
		}
		title := strings.TrimSpace(a.Title)
		if title == "" {
			title = "Album " + id
		}
		artistName := strings.TrimSpace(a.ArtistName)
		if artistName == "" {
			artistName = "Artiste inconnu"
		}
		webpageURL := strings.TrimSpace(a.URL)
		if webpageURL == "" {
			webpageURL = "https://play.qobuz.com/album/" + id
		}
		releaseKind := strings.TrimSpace(a.ReleaseKind)
		if releaseKind == "" {
			releaseKind = "Release"
		}
		albums = append(albums, core.QobuzAlbumDTO{
			ID:               id,
			Title:            title,
			ArtistName:       artistName,
			WebpageURL:       webpageURL,
			ReleaseDate:      releaseDate,
			TrackCount:       a.TracksCount,
			ReleaseKindLabel: releaseKind,
			IsHiRes:          a.IsHiRes,
			ArtworkURL:       strings.TrimSpace(a.CoverURL),
		})
	}

	artists := make([]core.QobuzPlaylistArtistDTO, 0, len(parsed.Artists))
	for _, a := range parsed.Artists {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		tracksInPlaylist := 0
		if a.TracksInPlaylist != nil && *a.TracksInPlaylist > 0 {
			tracksInPlaylist = *a.TracksInPlaylist
		}
		albumsInPlaylist := 0
		if a.AlbumsInPlaylist != nil && *a.AlbumsInPlaylist > 0 {
			albumsInPlaylist = *a.AlbumsInPlaylist
		}
		artists = append(artists, core.QobuzPlaylistArtistDTO{
			ID:               strings.TrimSpace(a.ID),
			Name:             name,
			WebpageURL:       strings.TrimSpace(a.URL),
			TracksInPlaylist: tracksInPlaylist,
			AlbumsInPlaylist: albumsInPlaylist,
		})
	}

	playlistName := strings.TrimSpace(parsed.PlaylistName)
	if playlistName == "" {
		playlistName = "Playlist " + playlistID
	}
	webpageURL := strings.TrimSpace(parsed.URL)
	if webpageURL == "" {
		webpageURL = "https://play.qobuz.com/playlist/" + playlistID
	}
	tracksCount := len(tracks)
	if parsed.TracksCount != nil && *parsed.TracksCount >= 0 {
		tracksCount = *parsed.TracksCount
	}

	return core.QobuzPlaylistCatalogAPIResponse{
		PlaylistID:   playlistID,
		PlaylistName: playlistName,
		WebpageURL:   webpageURL,
		TracksCount:  tracksCount,
		Tracks:       tracks,
		Albums:       albums,
		Artists:      artists,
	}, nil
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
	qobuzExec, _, resolveErr := util.ResolveToolExecutable("qobuz-dl")
	if resolveErr != nil {
		return fmt.Errorf("qobuz-dl introuvable. Installe-le depuis Systeme > Diagnostics")
	}
	stdin := strings.TrimSpace(email) + "\n" + strings.TrimSpace(password) + "\n\n27\n"
	_, err = s.runner.Run(ctx, sys.RunOptions{
		Executable:    qobuzExec,
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
	candidates := make([]string, 0, 2)
	if runtime.GOOS == "windows" {
		if cfg := strings.TrimSpace(os.Getenv("APPDATA")); cfg != "" {
			candidates = append(candidates, filepath.Join(cfg, "qobuz-dl", "config.ini"))
		}
	}
	candidates = append(candidates, filepath.Join(home, ".config", "qobuz-dl", "config.ini"))

	for _, candidate := range candidates {
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	return filepath.Join(home, ".config", "qobuz-dl", "config.ini"), nil
}

func resolveQobuzPythonRuntime() (pythonCommandCandidate, error) {
	for _, candidate := range qobuzPythonProbeCandidates() {
		if qobuzPythonCandidateSupportsModule(candidate) {
			return candidate, nil
		}
	}
	return pythonCommandCandidate{}, fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
}

func qobuzPythonProbeCandidates() []pythonCommandCandidate {
	candidates := make([]pythonCommandCandidate, 0, 16)
	if qobuzPath, _, err := util.ResolveToolExecutable("qobuz-dl"); err == nil {
		for _, probeFile := range qobuzRuntimeProbeFiles(qobuzPath) {
			if resolved := qobuzPythonFromShebang(probeFile); strings.TrimSpace(resolved) != "" {
				candidates = append(candidates, pythonCommandCandidate{Exec: resolved})
			}
		}
	}
	candidates = append(candidates, pythonCommandCandidates()...)
	return uniquePythonCommandCandidates(candidates)
}

func qobuzRuntimeProbeFiles(entrypoint string) []string {
	out := []string{entrypoint}
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(entrypoint), ".exe") {
		base := strings.TrimSuffix(entrypoint, filepath.Ext(entrypoint))
		out = append(out, base+"-script.py")
		out = append(out, base+".py")
	}
	return out
}

func qobuzPythonFromShebang(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	if !s.Scan() {
		return ""
	}
	line := strings.TrimSpace(s.Text())
	if !strings.HasPrefix(line, "#!") {
		return ""
	}

	interpreter := strings.TrimSpace(strings.TrimPrefix(line, "#!"))
	if interpreter == "" {
		return ""
	}
	parts := strings.Fields(interpreter)
	if len(parts) == 0 {
		return ""
	}
	candidate := strings.Trim(parts[0], "\"'")
	if strings.EqualFold(filepath.Base(candidate), "env") && len(parts) >= 2 {
		candidate = strings.Trim(parts[1], "\"'")
	}
	return resolvePythonCandidatePath(candidate)
}

func resolvePythonCandidatePath(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}

	looksLikePath := filepath.IsAbs(candidate) ||
		strings.Contains(candidate, string(filepath.Separator)) ||
		(runtime.GOOS == "windows" && strings.Contains(candidate, "/"))

	if looksLikePath {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			return ""
		}
		return candidate
	}

	resolved, err := exec.LookPath(candidate)
	if err != nil {
		return ""
	}
	return resolved
}

func qobuzPythonCandidateSupportsModule(candidate pythonCommandCandidate) bool {
	execName := strings.TrimSpace(candidate.Exec)
	if execName == "" {
		return false
	}

	if strings.Contains(execName, string(filepath.Separator)) || (runtime.GOOS == "windows" && strings.Contains(execName, "/")) {
		info, err := os.Stat(execName)
		if err != nil || info.IsDir() {
			return false
		}
	} else {
		if _, err := exec.LookPath(execName); err != nil {
			return false
		}
	}

	args := append(append([]string{}, candidate.PrefixArgs...), "-c", "import qobuz_dl.qopy")
	cmd := exec.Command(execName, args...)
	return cmd.Run() == nil
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
