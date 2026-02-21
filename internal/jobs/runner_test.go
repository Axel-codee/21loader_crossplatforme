package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"persodl-cross/internal/core"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newLRCLIBMockClient(t *testing.T, responses map[string]lrclibPayload, requested *[]string) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Helper()
			if req.URL.Host != "lrclib.net" {
				t.Fatalf("unexpected host: %s", req.URL.Host)
			}
			track := strings.TrimSpace(req.URL.Query().Get("track_name"))
			*requested = append(*requested, track)
			payload, ok := responses[track]
			if !ok {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("[]")),
				}, nil
			}
			item := map[string]string{}
			if payload.plainLyrics != "" {
				item["plainLyrics"] = payload.plainLyrics
			}
			if payload.syncedLyrics != "" {
				item["syncedLyrics"] = payload.syncedLyrics
			}
			body, err := json.Marshal([]map[string]string{item})
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		}),
	}
}

func TestFetchLyricsFromLRCLIBProcessesAllAlbumTracks(t *testing.T) {
	albumDir := t.TempDir()
	disc2Dir := filepath.Join(albumDir, "Disc 2")
	if err := os.MkdirAll(disc2Dir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	for _, rel := range []string{
		"01 First Song.flac",
		"02 Second Song.flac",
		filepath.Join("Disc 2", "03 Third Song.flac"),
	} {
		if err := os.WriteFile(filepath.Join(albumDir, rel), []byte("audio"), 0o644); err != nil {
			t.Fatalf("write audio file failed: %v", err)
		}
	}

	responses := map[string]lrclibPayload{
		"01 First Song":  {syncedLyrics: "[00:00.00] First"},
		"02 Second Song": {plainLyrics: "Second line"},
		"03 Third Song":  {syncedLyrics: "[00:00.00] Third"},
	}
	var requested []string
	r := &Runner{httpClient: newLRCLIBMockClient(t, responses, &requested)}

	var logs strings.Builder
	var progress []float64
	r.fetchLyricsFromLRCLIB(context.Background(), albumDir, RunCallbacks{
		OnLog: func(line string) {
			logs.WriteString(line)
		},
		OnStepProgress: func(v float64) {
			progress = append(progress, v)
		},
	})

	for _, want := range []string{"01 First Song", "02 Second Song", "03 Third Song"} {
		if !containsString(requested, want) {
			t.Fatalf("missing lrclib request for %q, got %v", want, requested)
		}
	}
	if len(progress) != 3 {
		t.Fatalf("expected 3 progress updates, got %d", len(progress))
	}
	if !strings.Contains(logs.String(), "Termine: 3 genere(s), 0 deja present(s), 0 erreur(s).") {
		t.Fatalf("unexpected logs: %s", logs.String())
	}

	assertFileContains(t, filepath.Join(albumDir, "01 First Song.lrc"), "[00:00.00] First")
	assertFileContains(t, filepath.Join(albumDir, "02 Second Song.lyrics.txt"), "Second line")
	assertFileContains(t, filepath.Join(disc2Dir, "03 Third Song.lrc"), "[00:00.00] Third")
}

func TestFetchLyricsFromLRCLIBSkipsExistingTrackButContinuesAlbum(t *testing.T) {
	albumDir := t.TempDir()
	track1Base := filepath.Join(albumDir, "01 Existing")
	track2Base := filepath.Join(albumDir, "02 Needs Lyrics")
	if err := os.WriteFile(track1Base+".flac", []byte("audio"), 0o644); err != nil {
		t.Fatalf("write track1 failed: %v", err)
	}
	if err := os.WriteFile(track2Base+".flac", []byte("audio"), 0o644); err != nil {
		t.Fatalf("write track2 failed: %v", err)
	}
	if err := os.WriteFile(track1Base+".lrc", []byte("[00:00.00] Existing"), 0o644); err != nil {
		t.Fatalf("write existing lrc failed: %v", err)
	}

	responses := map[string]lrclibPayload{
		"02 Needs Lyrics": {plainLyrics: "Generated for second track"},
	}
	var requested []string
	r := &Runner{httpClient: newLRCLIBMockClient(t, responses, &requested)}

	var logs strings.Builder
	r.fetchLyricsFromLRCLIB(context.Background(), albumDir, RunCallbacks{
		OnLog: func(line string) {
			logs.WriteString(line)
		},
	})

	if containsString(requested, "01 Existing") {
		t.Fatalf("track with existing lyric file should not be queried: %v", requested)
	}
	if !containsString(requested, "02 Needs Lyrics") {
		t.Fatalf("second track was not queried: %v", requested)
	}
	if !strings.Contains(logs.String(), "Termine: 1 genere(s), 1 deja present(s), 0 erreur(s).") {
		t.Fatalf("unexpected logs: %s", logs.String())
	}
	assertFileContains(t, track2Base+".lyrics.txt", "Generated for second track")
}

func TestFetchLRCLIBRetriesTransientTimeout(t *testing.T) {
	attempts := 0
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: context.DeadlineExceeded}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`[{"syncedLyrics":"[00:00.00] Retry worked"}]`)),
			}, nil
		}),
	}

	payload, err := fetchLRCLIB(context.Background(), client, "02 Retry Song")
	if err != nil {
		t.Fatalf("fetchLRCLIB returned unexpected error: %v", err)
	}
	if payload.syncedLyrics != "[00:00.00] Retry worked" {
		t.Fatalf("unexpected synced lyrics: %q", payload.syncedLyrics)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestFetchLRCLIBDoesNotRetryOnHTTP404(t *testing.T) {
	attempts := 0
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`[]`)),
			}, nil
		}),
	}

	_, err := fetchLRCLIB(context.Background(), client, "Unknown Song")
	if err == nil {
		t.Fatalf("expected an error for 404 response")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt for 404 response, got %d", attempts)
	}
}

func TestParsePercentProgressUsesLatestMatch(t *testing.T) {
	got := parsePercentProgress("[download] 0.0% of 10.00MiB\r[download] 42.7% of 10.00MiB\r")
	if math.Abs(got-42.7) > 0.001 {
		t.Fatalf("unexpected percent: got=%v want=42.7", got)
	}
}

func TestParsePercentProgressSupportsCommaDecimal(t *testing.T) {
	got := parsePercentProgress("[download] 12,5% of 10.00MiB")
	if math.Abs(got-12.5) > 0.001 {
		t.Fatalf("unexpected percent: got=%v want=12.5", got)
	}
}

func TestParseArgosProgressPercent(t *testing.T) {
	got := parseArgosProgressPercent("[argos] srt: 48% (120/250)\n")
	if math.Abs(got-48) > 0.001 {
		t.Fatalf("unexpected Argos percent: got=%v want=48", got)
	}
	if gotNone := parseArgosProgressPercent("ordinary warning line"); gotNone >= 0 {
		t.Fatalf("expected no Argos percent in unrelated logs, got=%v", gotNone)
	}
}

func TestHasYtDlpCookieArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "empty",
			args: nil,
			want: false,
		},
		{
			name: "cookies file",
			args: []string{"--cookies", "/tmp/cookies.txt"},
			want: true,
		},
		{
			name: "cookies browser equals",
			args: []string{"--cookies-from-browser=firefox"},
			want: true,
		},
		{
			name: "unrelated args",
			args: []string{"--retries", "1"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasYtDlpCookieArgs(tt.args); got != tt.want {
				t.Fatalf("unexpected cookie args detection: got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestLooksLikeYouTubeBotCheckError(t *testing.T) {
	err := errors.New("la commande a echoue (1): yt-dlp ...")
	output := "ERROR: [youtube] abc: Sign in to confirm you’re not a bot. Use --cookies-from-browser or --cookies"
	if !looksLikeYouTubeBotCheckError(err, output) {
		t.Fatalf("expected anti-bot detection to be true")
	}
	if looksLikeYouTubeBotCheckError(err, "ERROR: this video is unavailable") {
		t.Fatalf("unexpected anti-bot detection for unrelated error")
	}
	if looksLikeYouTubeBotCheckError(nil, output) {
		t.Fatalf("anti-bot detection must be false when err is nil")
	}
}

func TestShouldRetryWithBrowserCookies(t *testing.T) {
	err := errors.New("la commande a echoue (1): yt-dlp ...")
	output := "ERROR: Sign in to confirm you're not a bot"
	if !shouldRetryWithBrowserCookies(err, output, false, false) {
		t.Fatalf("expected automatic retry when cookies are not configured")
	}
	if shouldRetryWithBrowserCookies(err, output, true, false) {
		t.Fatalf("must not retry automatically when firefox cookies are already enabled")
	}
	if shouldRetryWithBrowserCookies(err, output, false, true) {
		t.Fatalf("must not retry automatically when explicit cookie args are already set")
	}
	if shouldRetryWithBrowserCookies(err, "ERROR: unrelated", false, false) {
		t.Fatalf("must not retry for unrelated failures")
	}
}

func TestPreferredYtDlpCookieBrowsersNotEmpty(t *testing.T) {
	browsers := preferredYtDlpCookieBrowsers()
	if len(browsers) == 0 {
		t.Fatalf("expected at least one preferred browser")
	}
	for _, browser := range browsers {
		if strings.TrimSpace(browser) == "" {
			t.Fatalf("browser list must not contain empty values")
		}
	}
}

func TestYtDlpCookieSpecsForBrowser(t *testing.T) {
	specs := ytDlpCookieSpecsForBrowser("chrome")
	if len(specs) != 1 || specs[0] != "chrome" {
		t.Fatalf("unexpected chrome specs: %v", specs)
	}
	firefoxSpecs := ytDlpCookieSpecsForBrowser("firefox")
	if len(firefoxSpecs) == 0 || firefoxSpecs[0] != "firefox" {
		t.Fatalf("unexpected firefox specs: %v", firefoxSpecs)
	}
}

func TestLooksLikeCookiesWereExtracted(t *testing.T) {
	logs := "Extracting cookies from firefox\nExtracted 46 cookies from firefox\n"
	if !looksLikeCookiesWereExtracted(logs) {
		t.Fatalf("expected cookie extraction detection")
	}
	if looksLikeCookiesWereExtracted("ERROR: could not find cookies database") {
		t.Fatalf("unexpected positive detection on failed extraction")
	}
}

func TestDedupeStrings(t *testing.T) {
	values := []string{" firefox ", "firefox", "", "chrome", "chrome"}
	got := dedupeStrings(values)
	if len(got) != 2 || got[0] != "firefox" || got[1] != "chrome" {
		t.Fatalf("unexpected dedupe result: %v", got)
	}
}

func TestShouldTranslateAllowsSingleTranscript(t *testing.T) {
	job := core.JobRequest{
		ContentType:               core.ContentVideo,
		EnableTranscription:       true,
		EnableTranslation:         true,
		TranslationSourceLanguage: "en",
		TranslationTargetLanguage: "fr",
	}
	if !shouldTranslate(job, "", "/tmp/transcription.txt") {
		t.Fatalf("expected translation to be enabled with a single transcript file")
	}
}

func TestShouldTranslateRejectsSameLanguage(t *testing.T) {
	job := core.JobRequest{
		ContentType:               core.ContentVideo,
		EnableTranscription:       true,
		EnableTranslation:         true,
		TranslationSourceLanguage: "en",
		TranslationTargetLanguage: "en",
	}
	if shouldTranslate(job, "/tmp/transcription.srt", "/tmp/transcription.txt") {
		t.Fatalf("expected translation to be disabled when source and target languages are identical")
	}
}

func TestYouTubeInputURLsMatchByVideoIdentifier(t *testing.T) {
	left := "https://www.youtube.com/watch?v=BaAqD5kOEBQ"
	right := "https://youtu.be/BaAqD5kOEBQ?t=42"
	if !youtubeInputURLsMatch(left, right) {
		t.Fatalf("expected URLs to match by shared video identifier")
	}
}

func TestBuildMuxSubtitleTracksWithTranslationVariants(t *testing.T) {
	tmp := t.TempDir()
	media := filepath.Join(tmp, "video.mkv")
	orig := filepath.Join(tmp, "video.en.srt")
	translated := filepath.Join(tmp, "video.fr.srt")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatalf("write media failed: %v", err)
	}
	if err := os.WriteFile(orig, []byte("1"), 0o644); err != nil {
		t.Fatalf("write original subtitle failed: %v", err)
	}
	if err := os.WriteFile(translated, []byte("2"), 0o644); err != nil {
		t.Fatalf("write translated subtitle failed: %v", err)
	}

	job := core.JobRequest{
		ContentType:               core.ContentVideo,
		EnableTranscription:       true,
		EnableTranslation:         true,
		TranslationSourceLanguage: "en",
		TranslationTargetLanguage: "fr",
	}
	tracks := buildMuxSubtitleTracks(job, media, translated, translationVariantArtifacts{
		SourceLanguage:         "en",
		TargetLanguage:         "fr",
		OriginalSubtitlePath:   orig,
		TranslatedSubtitlePath: translated,
	})
	if len(tracks) != 2 {
		t.Fatalf("expected 2 subtitle tracks, got %d", len(tracks))
	}
	if !samePath(tracks[0].Path, orig) {
		t.Fatalf("unexpected first track path: %s", tracks[0].Path)
	}
	if tracks[0].Language != "en" {
		t.Fatalf("unexpected first track language: %s", tracks[0].Language)
	}
	if !samePath(tracks[1].Path, translated) {
		t.Fatalf("unexpected second track path: %s", tracks[1].Path)
	}
	if tracks[1].Language != "fr" {
		t.Fatalf("unexpected second track language: %s", tracks[1].Language)
	}
	if !tracks[1].Default {
		t.Fatalf("translated subtitle should be default track")
	}
}

func TestFFmpegSubtitleLanguageMapping(t *testing.T) {
	if got := ffmpegSubtitleLanguage("en"); got != "eng" {
		t.Fatalf("unexpected en mapping: %s", got)
	}
	if got := ffmpegSubtitleLanguage("fr"); got != "fra" {
		t.Fatalf("unexpected fr mapping: %s", got)
	}
	if got := ffmpegSubtitleLanguage("xx"); got != "und" {
		t.Fatalf("unexpected fallback mapping: %s", got)
	}
}

func TestFindPreferredSidecarForCompletionUsesLanguageTaggedVariant(t *testing.T) {
	tmp := t.TempDir()
	media := filepath.Join(tmp, "video.mkv")
	untagged := filepath.Join(tmp, "video.txt")
	tagged := filepath.Join(tmp, "video.en.txt")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatalf("write media failed: %v", err)
	}
	if err := os.WriteFile(tagged, []byte("transcript"), 0o644); err != nil {
		t.Fatalf("write tagged transcript failed: %v", err)
	}
	got := findPreferredSidecarForCompletion(media, ".txt", []string{"en"}, untagged)
	if !samePath(got, tagged) {
		t.Fatalf("unexpected transcript path: got=%q want=%q", got, tagged)
	}
}

func TestFindPreferredSidecarForCompletionFallsBackToAnyTaggedVariant(t *testing.T) {
	tmp := t.TempDir()
	media := filepath.Join(tmp, "video.mkv")
	tagged := filepath.Join(tmp, "video.en.srt")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatalf("write media failed: %v", err)
	}
	if err := os.WriteFile(tagged, []byte("subtitle"), 0o644); err != nil {
		t.Fatalf("write tagged subtitle failed: %v", err)
	}
	got := findPreferredSidecarForCompletion(media, ".srt", nil)
	if !samePath(got, tagged) {
		t.Fatalf("unexpected subtitle path: got=%q want=%q", got, tagged)
	}
}

func TestFindExistingRSSOutputMatchesSelectedEpisode(t *testing.T) {
	outputRoot := t.TempDir()
	episodeDir := filepath.Join(outputRoot, "RSS", "Podcast Demo", "Episode 01")
	if err := os.MkdirAll(episodeDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	mediaPath := filepath.Join(episodeDir, "Episode 01.mp3")
	subtitlePath := filepath.Join(episodeDir, "Episode 01.srt")
	transcriptPath := filepath.Join(episodeDir, "Episode 01.txt")
	metadataPath := filepath.Join(episodeDir, "Episode 01.json")
	for _, entry := range []struct {
		path string
		body string
	}{
		{path: mediaPath, body: "audio"},
		{path: subtitlePath, body: "subtitle"},
		{path: transcriptPath, body: "transcript"},
	} {
		if err := os.WriteFile(entry.path, []byte(entry.body), 0o644); err != nil {
			t.Fatalf("write %s failed: %v", entry.path, err)
		}
	}
	pub := time.Now().UTC()
	meta := MediaMetadata{
		Title:            "Episode 01",
		SourceName:       "Podcast Demo",
		SourceKind:       core.SourceRSS,
		OriginalInputURL: "https://cdn.example.com/podcast/episode-01.mp3",
		MediaPath:        mediaPath,
		SubtitlePath:     subtitlePath,
		TranscriptPath:   transcriptPath,
		PublicationDate:  &pub,
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata failed: %v", err)
	}
	if err := os.WriteFile(metadataPath, data, 0o644); err != nil {
		t.Fatalf("write metadata failed: %v", err)
	}

	r := &Runner{}
	job := core.JobRequest{
		SourceKind: core.SourceRSS,
		InputURL:   "https://example.com/feed.xml",
		SelectedRSSEpisode: &core.RSSEpisodeSelection{
			Title:        "Episode 01",
			PodcastTitle: "Podcast Demo",
			MediaURL:     "https://cdn.example.com/podcast/episode-01.mp3",
		},
	}

	got := r.findExistingRSSOutput(job, outputRoot)
	if !samePath(got.MediaPath, mediaPath) {
		t.Fatalf("unexpected media path: got=%q want=%q", got.MediaPath, mediaPath)
	}
	if !samePath(got.SubtitlePath, subtitlePath) {
		t.Fatalf("unexpected subtitle path: got=%q want=%q", got.SubtitlePath, subtitlePath)
	}
	if !samePath(got.TranscriptPath, transcriptPath) {
		t.Fatalf("unexpected transcript path: got=%q want=%q", got.TranscriptPath, transcriptPath)
	}
}

func TestShouldTranslateSupportsPodcastAudio(t *testing.T) {
	job := core.JobRequest{
		SourceKind:                core.SourceRSS,
		ContentType:               core.ContentAudio,
		EnableTranscription:       true,
		EnableTranslation:         true,
		TranslationSourceLanguage: "en",
		TranslationTargetLanguage: "fr",
	}
	if !shouldTranscribe(job) {
		t.Fatalf("expected podcasts to support transcription")
	}
	if !shouldTranslate(job, "/tmp/transcription.srt", "") {
		t.Fatalf("expected podcasts to support translation when transcript/subtitle exists")
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %q failed: %v", path, err)
	}
	if strings.TrimSpace(string(data)) != strings.TrimSpace(want) {
		t.Fatalf("unexpected content in %q: got %q want %q", path, string(data), want)
	}
}
