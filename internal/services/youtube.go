package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"21loader-cross/internal/core"
	"21loader-cross/internal/sys"
	"21loader-cross/internal/util"
)

type YouTubeService struct {
	runner *sys.Runner
	mu     sync.RWMutex
	cache  map[string]cachedYouTubeMetadata

	resolveDatesSemaphore chan struct{}
}

func NewYouTubeService(r *sys.Runner) *YouTubeService {
	resolveDatesConcurrency := resolveDatesConcurrencyLimit()
	return &YouTubeService{
		runner:                r,
		cache:                 map[string]cachedYouTubeMetadata{},
		resolveDatesSemaphore: make(chan struct{}, resolveDatesConcurrency),
	}
}

const youtubeMetadataCacheTTL = 24 * time.Hour

type cachedYouTubeMetadata struct {
	UploadDate      *time.Time
	DurationSeconds *int
	CachedAt        time.Time
}

func resolveDatesConcurrencyLimit() int {
	if raw := strings.TrimSpace(os.Getenv("LOADER21_YT_DATES_CONCURRENCY")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > 8 {
				return 8
			}
			return parsed
		}
	}

	cpus := runtime.NumCPU()
	switch {
	case cpus <= 2:
		return 1
	case cpus <= 4:
		return 2
	case cpus <= 8:
		return 3
	default:
		return 4
	}
}

type ytFlatEntry struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	WebpageURL string `json:"webpage_url"`
	Channel    string `json:"channel"`
	Uploader   string `json:"uploader"`
	Playlist   string `json:"playlist_title"`
}

func parseUploadDate(v string) *time.Time {
	v = strings.TrimSpace(v)
	if len(v) != 8 {
		return nil
	}
	t, err := time.Parse("20060102", v)
	if err != nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func parseDurationSeconds(value string) (int, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "na" || value == "none" {
		return 0, false
	}
	if strings.Contains(value, ":") {
		parts := strings.Split(value, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return 0, false
		}
		total := 0
		for _, raw := range parts {
			part := strings.TrimSpace(raw)
			if part == "" {
				return 0, false
			}
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 {
				return 0, false
			}
			total = total*60 + n
		}
		return total, true
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds < 0 {
		return 0, false
	}
	return int(seconds), true
}

func anyToInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case json.Number:
		if asInt, err := x.Int64(); err == nil {
			return int(asInt), true
		}
		if asFloat, err := x.Float64(); err == nil {
			return int(asFloat), true
		}
	case string:
		if parsed, ok := parseDurationSeconds(x); ok {
			return parsed, true
		}
	}
	return 0, false
}

func extractUploadDate(entry map[string]any) *time.Time {
	if dt := parseUploadDate(anyToString(entry["upload_date"])); dt != nil {
		return dt
	}
	if ts, ok := anyToInt(entry["timestamp"]); ok && ts > 0 {
		t := time.Unix(int64(ts), 0).UTC()
		return &t
	}
	return nil
}

func extractDurationSeconds(entry map[string]any) *int {
	if seconds, ok := anyToInt(entry["duration"]); ok && seconds >= 0 {
		return &seconds
	}
	if seconds, ok := anyToInt(entry["duration_seconds"]); ok && seconds >= 0 {
		return &seconds
	}
	if seconds, ok := parseDurationSeconds(anyToString(entry["duration_string"])); ok {
		return &seconds
	}
	return nil
}

func parseResolvedMetadataLine(line string) (*time.Time, *int, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil, false
	}

	var resolvedDate *time.Time
	var resolvedDuration *int

	fields := strings.SplitN(line, "\t", 2)
	if len(fields) == 2 {
		if dt := parseUploadDate(fields[0]); dt != nil {
			resolvedDate = dt
		}
		if seconds, ok := parseDurationSeconds(fields[1]); ok {
			sec := seconds
			resolvedDuration = &sec
		}
		if resolvedDate != nil || resolvedDuration != nil {
			return resolvedDate, resolvedDuration, true
		}
	}

	if dt := parseUploadDate(line); dt != nil {
		return dt, nil, true
	}
	if seconds, ok := parseDurationSeconds(line); ok {
		sec := seconds
		return nil, &sec, true
	}
	return nil, nil, false
}

func cloneTimePtr(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	c := v.UTC()
	return &c
}

func cloneIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func (s *YouTubeService) readCachedMetadata(videoID string) (*time.Time, *int, bool) {
	s.mu.RLock()
	entry, ok := s.cache[videoID]
	s.mu.RUnlock()
	if !ok {
		return nil, nil, false
	}
	if time.Since(entry.CachedAt) > youtubeMetadataCacheTTL {
		s.mu.Lock()
		if freshEntry, freshOK := s.cache[videoID]; freshOK && time.Since(freshEntry.CachedAt) > youtubeMetadataCacheTTL {
			delete(s.cache, videoID)
		}
		s.mu.Unlock()
		return nil, nil, false
	}
	return cloneTimePtr(entry.UploadDate), cloneIntPtr(entry.DurationSeconds), true
}

func (s *YouTubeService) writeCachedMetadata(videoID string, uploadDate *time.Time, durationSeconds *int) {
	if videoID == "" {
		return
	}
	normalizedDate := cloneTimePtr(uploadDate)
	normalizedDuration := cloneIntPtr(durationSeconds)

	s.mu.Lock()
	defer s.mu.Unlock()

	if current, ok := s.cache[videoID]; ok {
		if normalizedDate == nil {
			normalizedDate = cloneTimePtr(current.UploadDate)
		}
		if normalizedDuration == nil {
			normalizedDuration = cloneIntPtr(current.DurationSeconds)
		}
	}
	if normalizedDate == nil && normalizedDuration == nil {
		return
	}

	s.cache[videoID] = cachedYouTubeMetadata{
		UploadDate:      normalizedDate,
		DurationSeconds: normalizedDuration,
		CachedAt:        time.Now().UTC(),
	}
}

func (s *YouTubeService) FetchCatalog(ctx context.Context, url string, useFirefoxCookies bool) (core.YouTubeCatalogAPIResponse, error) {
	if !util.LooksLikeYouTubeURL(url) {
		return core.YouTubeCatalogAPIResponse{}, fmt.Errorf("URL YouTube invalide")
	}
	if !util.LooksLikeYouTubeCollectionURL(url) {
		return core.YouTubeCatalogAPIResponse{}, fmt.Errorf("utilise une URL de chaine ou playlist YouTube")
	}
	args := []string{
		"--flat-playlist",
		"--ignore-errors",
		"--no-warnings",
		"--retries", "1",
		"--extractor-retries", "1",
		"--socket-timeout", "10",
		"--dump-single-json",
		url,
	}
	if useFirefoxCookies {
		args = append([]string{"--cookies-from-browser", "firefox"}, args...)
	}
	ytDlpExec, _, err := util.ResolveToolExecutable("yt-dlp")
	if err != nil {
		return core.YouTubeCatalogAPIResponse{}, err
	}
	output, err := s.runner.Run(ctx, sys.RunOptions{
		Executable:    ytDlpExec,
		Args:          args,
		CaptureOutput: true,
	})
	if err != nil {
		return core.YouTubeCatalogAPIResponse{}, err
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return core.YouTubeCatalogAPIResponse{}, fmt.Errorf("impossible de lire la collection YouTube")
	}

	entriesRaw, _ := payload["entries"].([]any)
	videos := make([]core.YouTubeVideoDTO, 0, len(entriesRaw))
	seen := map[string]bool{}
	sourceTitle := strings.TrimSpace(anyToString(payload["title"]))
	if sourceTitle == "" {
		sourceTitle = strings.TrimSpace(anyToString(payload["uploader"]))
	}
	if sourceTitle == "" {
		sourceTitle = strings.TrimSpace(anyToString(payload["channel"]))
	}
	if sourceTitle == "" {
		sourceTitle = "Source YouTube"
	}

	for i, raw := range entriesRaw {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entry := ytFlatEntry{
			ID:         strings.TrimSpace(anyToString(m["id"])),
			Title:      strings.TrimSpace(anyToString(m["title"])),
			WebpageURL: strings.TrimSpace(anyToString(m["url"])),
			Channel:    strings.TrimSpace(anyToString(m["channel"])),
			Uploader:   strings.TrimSpace(anyToString(m["uploader"])),
			Playlist:   strings.TrimSpace(anyToString(m["playlist_title"])),
		}
		if !isLikelyYouTubeVideoID(entry.ID) || seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		if entry.Title == "" {
			entry.Title = "Video " + entry.ID
		}
		web := entry.WebpageURL
		if web == "" {
			web = "https://www.youtube.com/watch?v=" + entry.ID
		}
		if sourceTitle == "Source YouTube" {
			if entry.Playlist != "" {
				sourceTitle = entry.Playlist
			} else if entry.Channel != "" {
				sourceTitle = entry.Channel
			} else if entry.Uploader != "" {
				sourceTitle = entry.Uploader
			}
		}
		uploadDate := extractUploadDate(m)
		durationSeconds := extractDurationSeconds(m)
		s.writeCachedMetadata(entry.ID, uploadDate, durationSeconds)

		videos = append(videos, core.YouTubeVideoDTO{
			ID:              entry.ID,
			Title:           entry.Title,
			WebpageURL:      web,
			UploadDate:      uploadDate,
			DurationSeconds: durationSeconds,
			Position:        i,
		})
	}

	if len(videos) == 0 {
		return core.YouTubeCatalogAPIResponse{}, fmt.Errorf("aucune video n'a ete trouvee pour cette URL")
	}

	sort.Slice(videos, func(i, j int) bool {
		li, lj := videos[i], videos[j]
		if li.UploadDate != nil && lj.UploadDate != nil {
			if !li.UploadDate.Equal(*lj.UploadDate) {
				return li.UploadDate.After(*lj.UploadDate)
			}
			return li.Position < lj.Position
		}
		if li.UploadDate != nil {
			return true
		}
		if lj.UploadDate != nil {
			return false
		}
		return li.Position < lj.Position
	})

	return core.YouTubeCatalogAPIResponse{SourceTitle: sourceTitle, Videos: videos}, nil
}

func (s *YouTubeService) ResolveVideoTitle(ctx context.Context, url string, useFirefoxCookies bool) (string, error) {
	if !util.LooksLikeYouTubeURL(url) {
		return "", fmt.Errorf("URL YouTube invalide")
	}
	args := []string{
		"--quiet",
		"--no-warnings",
		"--retries", "1",
		"--extractor-retries", "1",
		"--socket-timeout", "10",
		"--skip-download",
		"--no-playlist",
		"--print", "%(title)s",
		url,
	}
	if useFirefoxCookies {
		args = append([]string{"--cookies-from-browser", "firefox"}, args...)
	}
	ytDlpExec, _, err := util.ResolveToolExecutable("yt-dlp")
	if err != nil {
		return "", err
	}
	output, err := s.runner.Run(ctx, sys.RunOptions{
		Executable:    ytDlpExec,
		Args:          args,
		CaptureOutput: true,
	})
	if err != nil {
		return "", err
	}
	title := firstPrintedYouTubeTitle(output)
	if title == "" {
		return "", fmt.Errorf("titre YouTube introuvable")
	}
	return title, nil
}

func (s *YouTubeService) ResolveDates(ctx context.Context, videoIDs []string, useFirefoxCookies bool) (map[string]time.Time, map[string]int) {
	datesByVideoID := map[string]time.Time{}
	durationsByVideoID := map[string]int{}
	seen := map[string]bool{}
	urls := make([]string, 0, len(videoIDs))
	for _, rawID := range videoIDs {
		id := strings.TrimSpace(rawID)
		if !isLikelyYouTubeVideoID(id) || seen[id] {
			continue
		}
		seen[id] = true

		cachedDate, cachedDuration, hasCache := s.readCachedMetadata(id)
		if hasCache {
			if cachedDate != nil {
				datesByVideoID[id] = *cachedDate
			}
			if cachedDuration != nil {
				durationsByVideoID[id] = *cachedDuration
			}
			if cachedDate != nil && cachedDuration != nil {
				continue
			}
		}

		urls = append(urls, "https://www.youtube.com/watch?v="+id)
	}
	if len(urls) == 0 {
		return datesByVideoID, durationsByVideoID
	}

	args := []string{
		"--ignore-errors",
		"--no-warnings",
		"--retries", "1",
		"--extractor-retries", "1",
		"--socket-timeout", "10",
		"--extractor-args", "youtube:player_client=web",
		"--skip-download",
		"--no-playlist",
		"--print", "%(id)s\t%(upload_date)s\t%(duration)s",
	}
	args = append(args, urls...)
	if useFirefoxCookies {
		args = append([]string{"--cookies-from-browser", "firefox"}, args...)
	}

	select {
	case s.resolveDatesSemaphore <- struct{}{}:
	case <-ctx.Done():
		return datesByVideoID, durationsByVideoID
	}
	defer func() {
		<-s.resolveDatesSemaphore
	}()

	ytDlpExec, _, err := util.ResolveToolExecutable("yt-dlp")
	if err != nil {
		return datesByVideoID, durationsByVideoID
	}
	data, err := s.runner.Run(ctx, sys.RunOptions{Executable: ytDlpExec, Args: args, CaptureOutput: true})
	if err != nil {
		return datesByVideoID, durationsByVideoID
	}

	for _, rawLine := range strings.Split(data, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		if !isLikelyYouTubeVideoID(id) {
			continue
		}
		var entryDate *time.Time
		if dt := parseUploadDate(parts[1]); dt != nil {
			datesByVideoID[id] = *dt
			entryDate = dt
		}
		var entryDuration *int
		if len(parts) >= 3 {
			if duration, ok := parseDurationSeconds(parts[2]); ok {
				durationsByVideoID[id] = duration
				d := duration
				entryDuration = &d
			}
		}
		s.writeCachedMetadata(id, entryDate, entryDuration)
	}

	return datesByVideoID, durationsByVideoID
}

func firstPrintedYouTubeTitle(output string) string {
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "warning:") {
			continue
		}
		return line
	}
	return ""
}

func isLikelyYouTubeVideoID(value string) bool {
	if len(value) < 8 || len(value) > 16 {
		return false
	}
	ok := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString
	return ok(value)
}

func anyToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return fmt.Sprintf("%.0f", x)
	default:
		return ""
	}
}
