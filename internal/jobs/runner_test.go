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
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"21loader-cross/internal/core"
	"21loader-cross/internal/sys"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

const qobuzRunnerHelperFlag = "--qobuz-runner-helper"

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
			if track == "" {
				track = strings.TrimSpace(req.URL.Query().Get("q"))
			}
			artist := strings.TrimSpace(req.URL.Query().Get("artist_name"))
			lookup := track
			if artist != "" {
				lookup = artist + "::" + track
			}
			*requested = append(*requested, lookup)
			payload, ok := responses[lookup]
			if !ok {
				payload, ok = responses[track]
			}
			if !ok {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("[]")),
				}, nil
			}
			item := map[string]string{}
			if track != "" {
				item["trackName"] = track
			}
			if artist != "" {
				item["artistName"] = artist
			}
			if album := strings.TrimSpace(req.URL.Query().Get("album_name")); album != "" {
				item["albumName"] = album
			}
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

func TestFetchLyricsFromLRCLIBNormalizesYouTubeStyleTitle(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "GIMS & La Mano 1.9 - PARISIENNE (Clip officiel) [7CGKeID7nRc].webm")
	if err := os.WriteFile(mediaPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write webm failed: %v", err)
	}

	responses := map[string]lrclibPayload{
		"GIMS & La Mano 1.9::PARISIENNE": {syncedLyrics: "[00:00.00] Parisienne"},
	}
	var requested []string
	r := &Runner{httpClient: newLRCLIBMockClient(t, responses, &requested)}

	r.fetchLyricsFromLRCLIB(context.Background(), mediaPath, RunCallbacks{})

	assertFileContains(t, filepath.Join(root, "GIMS & La Mano 1.9 - PARISIENNE (Clip officiel) [7CGKeID7nRc].lrc"), "[00:00.00] Parisienne")
	if !containsString(requested, "GIMS & La Mano 1.9::PARISIENNE") {
		t.Fatalf("expected normalized artist/title query, got %v", requested)
	}
}

func TestBuildLRCLIBSearchQueriesIncludesCleanedTitle(t *testing.T) {
	queries := buildLRCLIBSearchQueries("GIMS & La Mano 1.9 - PARISIENNE (Clip officiel) [7CGKeID7nRc]", "", "")
	if len(queries) == 0 {
		t.Fatalf("expected at least one query")
	}
	found := false
	for _, query := range queries {
		if query.trackName == "PARISIENNE" && query.artistName == "GIMS & La Mano 1.9" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cleaned artist/title query, got %+v", queries)
	}
}

func TestBuildLRCLIBSearchQueriesStripsLeadingTrackNumber(t *testing.T) {
	queries := buildLRCLIBSearchQueries("05. Check da Crou", "", "")
	if len(queries) == 0 {
		t.Fatalf("expected queries for numbered track")
	}
	found := false
	for _, query := range queries {
		if strings.EqualFold(strings.TrimSpace(query.trackName), "Check da Crou") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected stripped track name query, got %+v", queries)
	}
}

func TestQobuzCommandEnvironmentUsesTokenWorkaroundHome(t *testing.T) {
	workspace := t.TempDir()

	env, err := qobuzCommandEnvironment(workspace, " user@example.com ", "", " session.token.value ", "raw", true)
	if err != nil {
		t.Fatalf("qobuzCommandEnvironment returned error: %v", err)
	}
	if got, want := env[qobuzUserAuthTokenEnv], "session.token.value"; got != want {
		t.Fatalf("unexpected token env: got=%q want=%q", got, want)
	}
	if got, want := env[qobuzEmailEnv], "user@example.com"; got != want {
		t.Fatalf("unexpected email env: got=%q want=%q", got, want)
	}
	if runtime.GOOS == "windows" {
		if got := strings.TrimSpace(env["APPDATA"]); got == "" {
			t.Fatalf("expected APPDATA override, got %q", got)
		}
	} else if got := strings.TrimSpace(env["HOME"]); got == "" {
		t.Fatalf("expected HOME override, got %q", got)
	}
}

func TestQobuzCommandEnvironmentIgnoresTokenWhenModeDisabled(t *testing.T) {
	env, err := qobuzCommandEnvironment(t.TempDir(), "", "", " session.token.value ", "raw", false)
	if err != nil {
		t.Fatalf("qobuzCommandEnvironment returned error: %v", err)
	}
	if env != nil {
		t.Fatalf("expected no token environment when workaround is disabled, got %#v", env)
	}
}

func TestBuildLRCLIBSearchQueriesSanitizesQobuzAlbumHint(t *testing.T) {
	queries := buildLRCLIBSearchQueries("04. LEAVING THE CITY", "Rilès", "THE 25TH HOUR (2025) [24B-48kHz]")
	if len(queries) == 0 {
		t.Fatalf("expected at least one query")
	}

	for _, query := range queries {
		if query.albumName == "" {
			continue
		}
		if query.albumName != "THE 25TH HOUR" {
			t.Fatalf("unexpected album hint in query: %+v", query)
		}
	}
}

func TestFetchLyricsFromLRCLIBWithHintsUsesArtistForAlbumTracks(t *testing.T) {
	root := t.TempDir()
	track1 := filepath.Join(root, "01. Invasion.flac")
	track2 := filepath.Join(root, "05. Check da Crou.flac")
	if err := os.WriteFile(track1, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write track1 failed: %v", err)
	}
	if err := os.WriteFile(track2, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write track2 failed: %v", err)
	}

	responses := map[string]lrclibPayload{
		"Stupeflip::Check da Crou": {syncedLyrics: "[00:00.00] Crou"},
	}
	var requested []string
	r := &Runner{httpClient: newLRCLIBMockClient(t, responses, &requested)}

	r.fetchLyricsFromLRCLIBWithHints(context.Background(), root, "", "Stupeflip", "", RunCallbacks{})

	assertFileContains(t, filepath.Join(root, "05. Check da Crou.lrc"), "[00:00.00] Crou")
	if !containsString(requested, "Stupeflip::Check da Crou") {
		t.Fatalf("expected artist+track query for album track, got %v", requested)
	}
}

func TestFetchLyricsFromLRCLIBWithHintsIgnoresGenericPlaylistArtistHint(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"01. PRESSURE.flac", "02. Filler.flac"} {
		if err := os.WriteFile(filepath.Join(root, rel), []byte("audio"), 0o644); err != nil {
			t.Fatalf("write track failed: %v", err)
		}
	}

	responses := map[string]lrclibPayload{
		"PRESSURE": {syncedLyrics: "[00:00.00] Pressure"},
	}
	var requested []string
	r := &Runner{httpClient: newLRCLIBMockClient(t, responses, &requested)}

	r.fetchLyricsFromLRCLIBWithHints(context.Background(), root, "My Playlist", "Playlists", "", RunCallbacks{})

	assertFileContains(t, filepath.Join(root, "01. PRESSURE.lrc"), "[00:00.00] Pressure")
	if containsString(requested, "Playlists::PRESSURE") || containsString(requested, "Playlists::01. PRESSURE") {
		t.Fatalf("generic playlist artist hint should not be used as strict artist filter, got %v", requested)
	}
}

func TestNormalizeLRCLIBTextFoldsAccents(t *testing.T) {
	left := normalizeLRCLIBText("Gaëlle")
	right := normalizeLRCLIBText("Gaelle")
	if left == "" || right == "" || left != right {
		t.Fatalf("expected accent-insensitive normalization, left=%q right=%q", left, right)
	}
}

func TestPickBestLRCLIBPayloadPrefersSyncedMatchingArtist(t *testing.T) {
	items := []map[string]any{
		{
			"trackName":   "Gaëlle",
			"artistName":  "Other Artist",
			"plainLyrics": "wrong",
		},
		{
			"trackName":    "Gaelle",
			"artistName":   "Stupeflip",
			"syncedLyrics": "[00:00.00] correct",
		},
	}
	payload, _ := pickBestLRCLIBPayload(items, lrclibSearchQuery{
		trackName:  "Gaëlle",
		artistName: "Stupeflip",
	})
	if payload.syncedLyrics != "[00:00.00] correct" {
		t.Fatalf("expected synced lyrics from matching artist, got plain=%q synced=%q", payload.plainLyrics, payload.syncedLyrics)
	}
}

func TestFetchLyricsFromLRCLIBWithHintsUsesMetadataForSingleTrack(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "GIMS - 20250816 - GIMS & La Mano 1.9 - PARISIENNE (Clip officiel).webm")
	if err := os.WriteFile(mediaPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write webm failed: %v", err)
	}

	responses := map[string]lrclibPayload{
		"GIMS::GIMS & La Mano 1.9 - PARISIENNE (Clip officiel)": {syncedLyrics: "[00:00.00] From metadata"},
	}
	var requested []string
	r := &Runner{httpClient: newLRCLIBMockClient(t, responses, &requested)}

	r.fetchLyricsFromLRCLIBWithHints(context.Background(), mediaPath, "GIMS & La Mano 1.9 - PARISIENNE (Clip officiel)", "GIMS", "", RunCallbacks{})

	assertFileContains(t, filepath.Join(root, "GIMS - 20250816 - GIMS & La Mano 1.9 - PARISIENNE (Clip officiel).lrc"), "[00:00.00] From metadata")
	if !containsString(requested, "GIMS::GIMS & La Mano 1.9 - PARISIENNE (Clip officiel)") {
		t.Fatalf("expected metadata-assisted query, got %v", requested)
	}
}

func TestResolveLyricsSearchHintsAllowsBlankArtistAndAlbumOverrides(t *testing.T) {
	job := core.JobRequest{
		SourceKind:            core.SourceYouTube,
		ContentType:           core.ContentMusic,
		UseCustomLyricsSearch: true,
		LyricsSearchTitle:     "Titre manuel",
		LyricsSearchArtist:    "",
		LyricsSearchAlbum:     "",
	}
	artifact := downloadArtifact{
		Title:      "Titre auto",
		SourceName: "Artiste auto",
	}

	trackHint, artistHint, albumHint := resolveLyricsSearchHints(job, artifact)

	if trackHint != "Titre manuel" {
		t.Fatalf("unexpected track hint: %q", trackHint)
	}
	if artistHint != "" {
		t.Fatalf("artist hint should stay empty, got %q", artistHint)
	}
	if albumHint != "" {
		t.Fatalf("album hint should stay empty, got %q", albumHint)
	}
}

func TestApplyManualLyricsSelectionWritesSyncedLyrics(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Chosen Song.webm")
	if err := os.WriteFile(mediaPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write webm failed: %v", err)
	}
	r := &Runner{}
	job := core.JobRequest{
		SourceKind:               core.SourceYouTube,
		ContentType:              core.ContentMusic,
		UseManualLyricsSelection: true,
		ManualLyricsTrackName:    "Chosen Song",
		ManualLyricsSynced:       "[00:00.00] Manual line",
	}

	handled, err := r.applyManualLyricsSelection(job, mediaPath, RunCallbacks{})
	if err != nil {
		t.Fatalf("applyManualLyricsSelection returned error: %v", err)
	}
	if !handled {
		t.Fatalf("expected manual lyrics selection to be handled")
	}
	assertFileContains(t, filepath.Join(root, "Chosen Song.lrc"), "[00:00.00] Manual line")
}

func TestFetchLyricsForJobAppliesManualSelectionsBeforeAutomaticFallback(t *testing.T) {
	root := t.TempDir()
	introPath := filepath.Join(root, "01. Intro.flac")
	outroPath := filepath.Join(root, "02. Outro.flac")
	if err := os.WriteFile(introPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write intro failed: %v", err)
	}
	if err := os.WriteFile(outroPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write outro failed: %v", err)
	}

	requested := []string{}
	responses := map[string]lrclibPayload{
		"Artist Demo::Outro": {syncedLyrics: "[00:00.00] Auto outro"},
	}
	r := &Runner{httpClient: newLRCLIBMockClient(t, responses, &requested)}
	job := core.JobRequest{
		SourceKind:   core.SourceQobuz,
		ContentType:  core.ContentMusic,
		EnableLyrics: true,
		ManualLyricsSelections: []core.ManualLyricsSelection{
			{
				TargetTrackName: "Intro",
				TrackName:       "Intro",
				ArtistName:      "Artist Demo",
				AlbumName:       "Album Demo",
				SyncedLyrics:    "[00:00.00] Manual intro",
			},
		},
	}
	artifact := downloadArtifact{
		MediaPath:   root,
		Title:       "Album Demo",
		SourceName:  "Artist Demo",
		IsDirectory: true,
	}

	if err := r.fetchLyricsForJob(context.Background(), job, artifact, RunCallbacks{}); err != nil {
		t.Fatalf("fetchLyricsForJob returned error: %v", err)
	}

	assertFileContains(t, filepath.Join(root, "01. Intro.lrc"), "[00:00.00] Manual intro")
	assertFileContains(t, filepath.Join(root, "02. Outro.lrc"), "[00:00.00] Auto outro")
	if len(requested) == 0 {
		t.Fatalf("expected LRCLIB fallback requests for the non-manual track")
	}
	if requested[0] != "Artist Demo::Outro" {
		t.Fatalf("unexpected LRCLIB requests: %#v", requested)
	}
	for _, value := range requested {
		if strings.Contains(strings.ToLower(value), "intro") {
			t.Fatalf("manual track should not trigger LRCLIB fallback, got %#v", requested)
		}
	}
}

func TestSearchLRCLIBCandidatesReturnsSortedResults(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `[{"trackName":"Chosen Song","artistName":"Chosen Artist","albumName":"Chosen Album","syncedLyrics":"[00:00.00] synced line"},{"trackName":"Chosen Song","artistName":"Other Artist","plainLyrics":"plain line"}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}
	r := &Runner{httpClient: client}

	response, err := r.SearchLRCLIBCandidates(context.Background(), "Chosen Song", "Chosen Artist", "Chosen Album", 8)
	if err != nil {
		t.Fatalf("SearchLRCLIBCandidates returned error: %v", err)
	}
	if len(response.Results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(response.Results))
	}
	if !response.Results[0].HasSynced {
		t.Fatalf("expected synced result to be ranked first, got %+v", response.Results[0])
	}
	if strings.TrimSpace(response.Results[0].Preview) == "" {
		t.Fatalf("expected preview text on first result")
	}
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

	payload, err := fetchLRCLIB(context.Background(), client, "02 Retry Song", "", "")
	if err != nil {
		t.Fatalf("fetchLRCLIB returned unexpected error: %v", err)
	}
	if payload.syncedLyrics != "[00:00.00] Retry worked" {
		t.Fatalf("unexpected synced lyrics: %q", payload.syncedLyrics)
	}
	if attempts < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", attempts)
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

	_, err := fetchLRCLIB(context.Background(), client, "Unknown Song", "", "")
	if err == nil {
		t.Fatalf("expected an error for 404 response")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt for 404 response, got %d", attempts)
	}
}

func TestFetchLRCLIBPrefersArtistMatchedResultAcrossQueries(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			artist := strings.TrimSpace(req.URL.Query().Get("artist_name"))
			body := `[{"trackName":"Gaelle","artistName":"Other Artist","plainLyrics":"wrong plain"}]`
			if artist == "Stupeflip" {
				body = `[{"trackName":"Gaëlle","artistName":"Stupeflip","syncedLyrics":"[00:00.00] correct synced"}]`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	payload, err := fetchLRCLIB(context.Background(), client, "04. Gaëlle", "Stupeflip", "The Hypnoflip Invasion")
	if err != nil {
		t.Fatalf("fetchLRCLIB returned unexpected error: %v", err)
	}
	if payload.syncedLyrics != "[00:00.00] correct synced" {
		t.Fatalf("expected synced artist-matched result, got plain=%q synced=%q", payload.plainLyrics, payload.syncedLyrics)
	}
}

func TestFetchLRCLIBRejectsArtistMismatchAcrossQueries(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			artist := strings.TrimSpace(req.URL.Query().Get("artist_name"))
			body := `[]`
			if artist == "" {
				body = `[{"trackName":"Gaelle","artistName":"Other Artist","plainLyrics":"wrong plain"}]`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	payload, err := fetchLRCLIB(context.Background(), client, "04. Gaëlle", "Stupeflip", "The Hypnoflip Invasion")
	if err != nil {
		t.Fatalf("fetchLRCLIB returned unexpected error: %v", err)
	}
	if payload.syncedLyrics != "" || payload.plainLyrics != "" {
		t.Fatalf("expected no lyrics when artist does not match, got plain=%q synced=%q", payload.plainLyrics, payload.syncedLyrics)
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

func TestParseQobuzDownloadingLabel(t *testing.T) {
	if got := parseQobuzDownloadingLabel("Downloading: The Monroeville Sound"); got != "The Monroeville Sound" {
		t.Fatalf("unexpected parsed label: %q", got)
	}
	if got := parseQobuzDownloadingLabel("  downloading :  Album Demo  "); got != "Album Demo" {
		t.Fatalf("unexpected parsed label with spaces: %q", got)
	}
	if got := parseQobuzDownloadingLabel("Roadrunner was already downloaded"); got != "" {
		t.Fatalf("unexpected label for non-downloading line: %q", got)
	}
}

func TestParseQobuzDirectoryFromProgressLineUsesAlbumRoot(t *testing.T) {
	line := "0.00/44.7M /// /Users/test/qobuz/David Guetta/David Guetta - 7 (2018) [24B-44.1kHz]/Disc 2/.24.tmp353k/44.7M"
	got := parseQobuzDirectoryFromProgressLine(line)
	want := "/Users/test/qobuz/David Guetta/David Guetta - 7 (2018) [24B-44.1kHz]"
	if got != want {
		t.Fatalf("unexpected parsed qobuz directory: got=%q want=%q", got, want)
	}
}

func TestIsQobuzDiscDirectory(t *testing.T) {
	if !isQobuzDiscDirectory("Disc 2") {
		t.Fatalf("expected disc directory to be detected")
	}
	if isQobuzDiscDirectory("Disc bonus") {
		t.Fatalf("did not expect non-numeric disc directory to be detected")
	}
}

func TestDetectQobuzRetryableDownloadError(t *testing.T) {
	logOutput := "Error getting release: ('Connection broken: IncompleteRead(1 bytes read, 52476484 more expected)', IncompleteRead(1 bytes read, 52476484 more expected)). Skipping...\n"
	if got := detectQobuzRetryableDownloadError(logOutput); got != "IncompleteRead" {
		t.Fatalf("unexpected retry reason: %q", got)
	}
	if got := detectQobuzRetryableDownloadError("Error getting release: 404 not found"); got != "" {
		t.Fatalf("unexpected retry reason for non-retryable error: %q", got)
	}
}

func TestDetectQobuzOGCoverTooLargeError(t *testing.T) {
	logOutput := "Error embedding image: downloaded cover size too large to embed. turn off `og_cover` to avoid error\n"
	if !detectQobuzOGCoverTooLargeError(logOutput) {
		t.Fatalf("expected og cover embed failure to be detected")
	}
	if detectQobuzOGCoverTooLargeError("Error embedding image: some other tag failure") {
		t.Fatalf("unexpected og cover detection for unrelated embedding error")
	}
}

func TestQobuzArgsWithoutOGCover(t *testing.T) {
	args := []string{"dl", "--embed-art", "--og-cover", "--og-cover=true", "--no-db", "https://play.qobuz.com/album/123456"}
	got := qobuzArgsWithoutOGCover(args)
	if qobuzArgsContainOGCover(got) {
		t.Fatalf("expected og-cover args to be removed, got %v", got)
	}
	if len(got) != 4 {
		t.Fatalf("unexpected filtered args length: %d (%v)", len(got), got)
	}
}

func TestRunQobuzDownloadCommandRetriesAfterIncompleteRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrapper qobuz-dl de test non implemente sur Windows")
	}

	binDir := t.TempDir()
	stateFile := filepath.Join(binDir, "attempts.txt")
	writeQobuzTestWrapper(t, filepath.Join(binDir, "qobuz-dl"), "retry-once", stateFile)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r := &Runner{processRunner: &sys.Runner{}}
	var logs strings.Builder

	result, err := r.runQobuzDownloadCommand(context.Background(), []string{"dl", "https://play.qobuz.com/album/123456"}, binDir, "", "", "", false, RunCallbacks{
		OnLog: func(line string) {
			logs.WriteString(line)
		},
	})
	if err != nil {
		t.Fatalf("runQobuzDownloadCommand returned error: %v", err)
	}
	if result.Label != "Album Demo" {
		t.Fatalf("unexpected download label: %q", result.Label)
	}

	rawAttempts, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read attempts failed: %v", err)
	}
	if strings.TrimSpace(string(rawAttempts)) != "2" {
		t.Fatalf("expected 2 qobuz attempts, got %q", strings.TrimSpace(string(rawAttempts)))
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, "Erreur transitoire detectee (IncompleteRead). Reprise automatique du telechargement, tentative 2") {
		t.Fatalf("expected retry log in output, got: %s", logOutput)
	}
}

func TestRunQobuzDownloadCommandStopsAfterMaxRetryableAttempts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrapper qobuz-dl de test non implemente sur Windows")
	}

	binDir := t.TempDir()
	stateFile := filepath.Join(binDir, "attempts.txt")
	writeQobuzTestWrapper(t, filepath.Join(binDir, "qobuz-dl"), "retry-always", stateFile)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r := &Runner{processRunner: &sys.Runner{}}
	_, err := r.runQobuzDownloadCommand(context.Background(), []string{"dl", "https://play.qobuz.com/album/123456"}, binDir, "", "", "", false, RunCallbacks{})
	if err == nil {
		t.Fatal("expected retry exhaustion error")
	}
	if !strings.Contains(err.Error(), "telechargement Qobuz interrompu apres 3 tentatives") {
		t.Fatalf("unexpected error: %v", err)
	}

	rawAttempts, readErr := os.ReadFile(stateFile)
	if readErr != nil {
		t.Fatalf("read attempts failed: %v", readErr)
	}
	if strings.TrimSpace(string(rawAttempts)) != "3" {
		t.Fatalf("expected 3 qobuz attempts, got %q", strings.TrimSpace(string(rawAttempts)))
	}
}

func TestRunQobuzDownloadCommandRetriesWithoutOGCoverAfterCoverTooLargeError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrapper qobuz-dl de test non implemente sur Windows")
	}

	binDir := t.TempDir()
	stateFile := filepath.Join(binDir, "attempts.txt")
	writeQobuzTestWrapper(t, filepath.Join(binDir, "qobuz-dl"), "fallback-og-cover", stateFile)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r := &Runner{processRunner: &sys.Runner{}}
	var logs strings.Builder

	result, err := r.runQobuzDownloadCommand(context.Background(), []string{"dl", "--embed-art", "--og-cover", "https://play.qobuz.com/album/123456"}, binDir, "", "", "", false, RunCallbacks{
		OnLog: func(line string) {
			logs.WriteString(line)
		},
	})
	if err != nil {
		t.Fatalf("runQobuzDownloadCommand returned error: %v", err)
	}
	if result.Label != "Album Demo" {
		t.Fatalf("unexpected download label: %q", result.Label)
	}

	rawAttempts, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read attempts failed: %v", err)
	}
	if strings.TrimSpace(string(rawAttempts)) != "2" {
		t.Fatalf("expected 2 qobuz attempts, got %q", strings.TrimSpace(string(rawAttempts)))
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, "Cover trop volumineuse pour l'embed detectee. Reprise automatique sans --og-cover, tentative 2") {
		t.Fatalf("expected og-cover fallback log in output, got: %s", logOutput)
	}
}

func TestRunQobuzDownloadCommandCapturesDirectoryFromProgressLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrapper qobuz-dl de test non implemente sur Windows")
	}

	binDir := t.TempDir()
	stateFile := filepath.Join(binDir, "attempts.txt")
	writeQobuzTestWrapper(t, filepath.Join(binDir, "qobuz-dl"), "emit-dir", stateFile)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r := &Runner{processRunner: &sys.Runner{}}
	result, err := r.runQobuzDownloadCommand(context.Background(), []string{"dl", "https://play.qobuz.com/album/123456"}, binDir, "", "", "", false, RunCallbacks{})
	if err != nil {
		t.Fatalf("runQobuzDownloadCommand returned error: %v", err)
	}
	wantDir := "/tmp/qobuz/Artist Demo/Artist Demo - Album Demo (2026) [24B-44.1kHz]"
	if result.Directory != wantDir {
		t.Fatalf("unexpected download directory: got=%q want=%q", result.Directory, wantDir)
	}
}

func TestDiscoverQobuzDirectoryByDownloadLabelPrefersMatchingDirectory(t *testing.T) {
	root := t.TempDir()
	monroevilleDir := filepath.Join(root, "Rilès - The Monroeville Sound (2023) [16B-44.1kHz]")
	hourDir := filepath.Join(root, "Rilès - THE 25TH HOUR (2025) [24B-48kHz]")
	if err := os.MkdirAll(monroevilleDir, 0o755); err != nil {
		t.Fatalf("mkdir monroeville failed: %v", err)
	}
	if err := os.MkdirAll(hourDir, 0o755); err != nil {
		t.Fatalf("mkdir 25th hour failed: %v", err)
	}
	base := time.Now().UTC()
	if err := os.Chtimes(monroevilleDir, base.Add(-2*time.Hour), base.Add(-2*time.Hour)); err != nil {
		t.Fatalf("chtimes monroeville failed: %v", err)
	}
	if err := os.Chtimes(hourDir, base, base); err != nil {
		t.Fatalf("chtimes 25th hour failed: %v", err)
	}

	got := discoverQobuzDirectoryByDownloadLabel(root, "The Monroeville Sound")
	if !samePath(got, monroevilleDir) {
		t.Fatalf("unexpected directory selection: got=%q want=%q", got, monroevilleDir)
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

func TestBuildWhisperArgsIncludesAdvancedOptions(t *testing.T) {
	job := core.JobRequest{
		TranscriptionLanguage:        "fr",
		WhisperVADEnabled:            true,
		WhisperVADThreshold:          0.62,
		WhisperVADMinSpeechDuration:  320,
		WhisperVADMinSilenceDuration: 180,
		WhisperVADSpeechPad:          80,
		WhisperMaxSegmentLength:      42,
		WhisperSplitOnWord:           true,
		WhisperInitialPrompt:         "Podcast demo",
		WhisperCarryInitialPrompt:    true,
		WhisperExtraArguments:        "--print-progress",
	}
	args, artifacts, err := buildWhisperArgs(job, "/tmp/input.wav", "/tmp/ggml-small.en-tdrz.bin", "/tmp/ggml-silero-v6.2.0.bin", whisperInvocationOptions{
		OutputBase:         "/tmp/transcription.tdrz",
		GenerateTranscript: true,
		GenerateJSONFull:   true,
		Tinydiarize:        true,
	})
	if err != nil {
		t.Fatalf("buildWhisperArgs returned error: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"--vad",
		"--vad-model /tmp/ggml-silero-v6.2.0.bin",
		"--vad-threshold 0.62",
		"--vad-min-speech-duration-ms 320",
		"--vad-min-silence-duration-ms 180",
		"--vad-speech-pad-ms 80",
		"-ml 42",
		"-sow",
		"--prompt Podcast demo",
		"--carry-initial-prompt",
		"-ojf",
		"-tdrz",
		"--print-progress",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected args to contain %q, got=%q", expected, joined)
		}
	}
	if artifacts.JSONPath != "/tmp/transcription.tdrz.json" {
		t.Fatalf("unexpected json artifact path: %q", artifacts.JSONPath)
	}
	if artifacts.TranscriptPath != "/tmp/transcription.tdrz.txt" {
		t.Fatalf("unexpected transcript artifact path: %q", artifacts.TranscriptPath)
	}
}

func TestBuildWhisperArgsRejectsVADWithoutModel(t *testing.T) {
	_, _, err := buildWhisperArgs(core.JobRequest{WhisperVADEnabled: true}, "/tmp/input.wav", "/tmp/ggml-base.bin", "", whisperInvocationOptions{
		OutputBase:         "/tmp/transcription",
		GenerateSubtitle:   true,
		GenerateTranscript: true,
	})
	if err == nil || !strings.Contains(err.Error(), "VAD activé") {
		t.Fatalf("expected explicit VAD validation error, got=%v", err)
	}
}

func TestBuildWhisperArgsRejectsTinydiarizeWithoutCompatibleModel(t *testing.T) {
	_, _, err := buildWhisperArgs(core.JobRequest{}, "/tmp/input.wav", "/tmp/ggml-base.bin", "", whisperInvocationOptions{
		OutputBase:       "/tmp/transcription.tdrz",
		GenerateJSONFull: true,
		Tinydiarize:      true,
	})
	if err == nil || !strings.Contains(err.Error(), "*-tdrz") {
		t.Fatalf("expected explicit tinydiarize validation error, got=%v", err)
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

func TestFindExistingRSSOutputDoesNotReuseDifferentEpisodeFromSamePodcast(t *testing.T) {
	outputRoot := t.TempDir()
	episodeDir := filepath.Join(outputRoot, "RSS", "Podcast Demo", "Episode 01")
	if err := os.MkdirAll(episodeDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	mediaPath := filepath.Join(episodeDir, "Episode 01.mp3")
	subtitlePath := filepath.Join(episodeDir, "Episode 01.srt")
	transcriptPath := filepath.Join(episodeDir, "Episode 01.txt")
	metadataPath := filepath.Join(episodeDir, "Episode 01.json")
	for _, file := range []string{mediaPath, subtitlePath, transcriptPath} {
		if err := os.WriteFile(file, []byte("demo"), 0o644); err != nil {
			t.Fatalf("write artifact failed: %v", err)
		}
	}
	meta := MediaMetadata{
		Title:            "Episode 01",
		SourceName:       "Podcast Demo",
		SourceKind:       core.SourceRSS,
		OriginalInputURL: "https://cdn.example.com/podcast/episode-01.mp3",
		MediaPath:        mediaPath,
		SubtitlePath:     subtitlePath,
		TranscriptPath:   transcriptPath,
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
		InputURL:   "https://cdn.example.com/podcast/episode-02.mp3",
		SelectedRSSEpisode: &core.RSSEpisodeSelection{
			Title:        "Episode 02",
			PodcastTitle: "Podcast Demo",
			MediaURL:     "https://cdn.example.com/podcast/episode-02.mp3",
		},
	}

	got := r.findExistingRSSOutput(job, outputRoot)
	if got.MediaPath != "" || got.SubtitlePath != "" || got.TranscriptPath != "" {
		t.Fatalf("expected no reuse for a different RSS episode, got=%+v", got)
	}
}

func TestFindExistingQobuzAlbumDirectoryRequiresSameResourceType(t *testing.T) {
	outputRoot := t.TempDir()
	playlistDir := filepath.Join(outputRoot, "qobuz", "Playlists", "Playlist Demo")
	if err := os.MkdirAll(playlistDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	metadataPath := filepath.Join(playlistDir, "album.json")
	meta := MediaMetadata{
		Title:            "Playlist Demo",
		SourceName:       "Playlists",
		SourceKind:       core.SourceQobuz,
		OriginalInputURL: "https://play.qobuz.com/playlist/123456",
		MediaPath:        playlistDir,
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata failed: %v", err)
	}
	if err := os.WriteFile(metadataPath, data, 0o644); err != nil {
		t.Fatalf("write metadata failed: %v", err)
	}

	r := &Runner{}
	if got := r.findExistingQobuzAlbumDirectory("https://play.qobuz.com/album/123456", outputRoot); got != "" {
		t.Fatalf("expected no reuse across qobuz resource types, got=%q", got)
	}

	gotPlaylist := r.findExistingQobuzAlbumDirectory("https://play.qobuz.com/playlist/123456", outputRoot)
	if !samePath(gotPlaylist, playlistDir) {
		t.Fatalf("expected playlist reuse path: got=%q want=%q", gotPlaylist, playlistDir)
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

func TestShouldFetchLyricsSupportsYouTubeMusic(t *testing.T) {
	job := core.JobRequest{
		SourceKind:   core.SourceYouTube,
		ContentType:  core.ContentMusic,
		EnableLyrics: true,
	}
	artifact := downloadArtifact{
		MediaPath:   "/tmp/song.webm",
		IsDirectory: false,
	}
	if !shouldFetchLyrics(job, artifact) {
		t.Fatalf("expected lyrics fetch to be enabled for youtube music")
	}
}

func TestShouldTranscribeSupportsMusicWhenEnabled(t *testing.T) {
	job := core.JobRequest{
		SourceKind:          core.SourceYouTube,
		ContentType:         core.ContentMusic,
		EnableTranscription: true,
		EnableTranslation:   true,
	}
	if !shouldTranscribe(job) {
		t.Fatalf("expected music jobs to support transcription when enabled")
	}
	if !shouldTranslate(job, "/tmp/transcription.srt", "/tmp/transcription.txt") {
		t.Fatalf("expected translated subtitles/transcript to stay available for music jobs")
	}
}

func TestShouldFetchLyricsRequiresMusicAndEnabledFlag(t *testing.T) {
	job := core.JobRequest{
		SourceKind:   core.SourceQobuz,
		ContentType:  core.ContentMusic,
		EnableLyrics: false,
	}
	artifact := downloadArtifact{
		MediaPath:   "/tmp/album/01.flac",
		IsDirectory: false,
	}
	if shouldFetchLyrics(job, artifact) {
		t.Fatalf("lyrics fetch should be disabled when enableLyrics is false")
	}
	job.EnableLyrics = true
	job.ContentType = core.ContentAudio
	if shouldFetchLyrics(job, artifact) {
		t.Fatalf("lyrics fetch should only run for music content type")
	}
}

func TestDiscoverAudioFilesIncludesWebM(t *testing.T) {
	root := t.TempDir()
	webm := filepath.Join(root, "song.webm")
	if err := os.WriteFile(webm, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write webm failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatalf("write txt failed: %v", err)
	}
	files := discoverAudioFiles(root)
	if !containsString(files, webm) {
		t.Fatalf("expected webm file in discovered audio files, got %v", files)
	}
}

func TestFetchLyricsFromLRCLIBSupportsSingleWebMFilePath(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Single Track.webm")
	if err := os.WriteFile(mediaPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write webm failed: %v", err)
	}
	responses := map[string]lrclibPayload{
		"Single Track": {syncedLyrics: "[00:00.00] line"},
	}
	var requested []string
	r := &Runner{httpClient: newLRCLIBMockClient(t, responses, &requested)}

	r.fetchLyricsFromLRCLIB(context.Background(), mediaPath, RunCallbacks{})

	if !containsString(requested, "Single Track") {
		t.Fatalf("expected lrclib request for single webm track, got %v", requested)
	}
	assertFileContains(t, filepath.Join(root, "Single Track.lrc"), "[00:00.00] line")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestQobuzRunnerHelperProcess(t *testing.T) {
	helperIndex := -1
	for i, arg := range os.Args {
		if arg == qobuzRunnerHelperFlag {
			helperIndex = i
			break
		}
	}
	if helperIndex == -1 {
		return
	}
	if helperIndex+2 >= len(os.Args) {
		os.Exit(2)
	}

	mode := os.Args[helperIndex+1]
	stateFile := os.Args[helperIndex+2]
	attempt := readHelperAttempt(stateFile) + 1
	if err := os.WriteFile(stateFile, []byte(strconv.Itoa(attempt)), 0o644); err != nil {
		os.Exit(2)
	}
	helperArgs := os.Args[helperIndex+3:]

	switch mode {
	case "retry-once":
		_, _ = os.Stdout.WriteString("Downloading: Album Demo\n")
		if attempt == 1 {
			_, _ = os.Stdout.WriteString("Error getting release: ('Connection broken: IncompleteRead(1 bytes read, 52476484 more expected)', IncompleteRead(1 bytes read, 52476484 more expected)). Skipping...\n")
		}
		os.Exit(0)
	case "retry-always":
		_, _ = os.Stdout.WriteString("Downloading: Album Demo\n")
		_, _ = os.Stdout.WriteString("Error getting release: ('Connection broken: IncompleteRead(1 bytes read, 52476484 more expected)', IncompleteRead(1 bytes read, 52476484 more expected)). Skipping...\n")
		os.Exit(0)
	case "fallback-og-cover":
		_, _ = os.Stdout.WriteString("Downloading: Album Demo\n")
		if qobuzArgsContainOGCover(helperArgs) {
			_, _ = os.Stdout.WriteString("Error embedding image: downloaded cover size too large to embed. turn off `og_cover` to avoid error\n")
			os.Exit(1)
		}
		os.Exit(0)
	case "emit-dir":
		_, _ = os.Stdout.WriteString("Downloading: Album Demo\n")
		_, _ = os.Stdout.WriteString("0.00/44.7M /// /tmp/qobuz/Artist Demo/Artist Demo - Album Demo (2026) [24B-44.1kHz]/Disc 2/.24.tmp353k/44.7M\n")
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func writeQobuzTestWrapper(t *testing.T, path, mode, stateFile string) {
	t.Helper()
	content := "#!/bin/sh\nexec " + shellQuote(os.Args[0]) +
		" -test.run=TestQobuzRunnerHelperProcess -- " +
		shellQuote(qobuzRunnerHelperFlag) + " " +
		shellQuote(mode) + " " +
		shellQuote(stateFile) + ` "$@"` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write qobuz test wrapper failed: %v", err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func readHelperAttempt(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0
	}
	return value
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
