package jobs

import (
	"strings"
	"testing"

	"21loader-cross/internal/core"
)

func TestBuildJobAllowsMusicTranscriptionWhenRequested(t *testing.T) {
	enableTranscription := true
	enableLyrics := false
	c := &Coordinator{}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:            "https://www.youtube.com/watch?v=86aHZNYEUjw",
		SourceKind:          "youtube",
		ContentType:         "music",
		EnableTranscription: &enableTranscription,
		EnableLyrics:        &enableLyrics,
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if !built.Request.EnableTranscription {
		t.Fatalf("music job should keep transcription enabled when explicitly requested")
	}
	if built.Request.EnableLyrics {
		t.Fatalf("lyrics should remain disabled when explicitly unchecked")
	}
}

func TestBuildJobKeepsCustomLyricsSearchOnlyForYouTubeMusic(t *testing.T) {
	enableLyrics := true
	useCustomLyricsSearch := true
	c := &Coordinator{}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:              "https://www.youtube.com/watch?v=86aHZNYEUjw",
		SourceKind:            "youtube",
		ContentType:           "music",
		EnableLyrics:          &enableLyrics,
		UseCustomLyricsSearch: &useCustomLyricsSearch,
		LyricsSearchTitle:     "Mon titre",
		LyricsSearchArtist:    "",
		LyricsSearchAlbum:     "",
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if !built.Request.UseCustomLyricsSearch {
		t.Fatalf("expected custom lyrics search to stay enabled for youtube music")
	}
	if built.Request.LyricsSearchTitle != "Mon titre" {
		t.Fatalf("unexpected custom lyrics title: %q", built.Request.LyricsSearchTitle)
	}
	if built.Request.LyricsSearchArtist != "" || built.Request.LyricsSearchAlbum != "" {
		t.Fatalf("artist and album should stay optional, got artist=%q album=%q", built.Request.LyricsSearchArtist, built.Request.LyricsSearchAlbum)
	}
}

func TestBuildJobKeepsManualLyricsSelectionForYouTubeMusic(t *testing.T) {
	enableLyrics := true
	useManualLyricsSelection := true
	c := &Coordinator{}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:                 "https://www.youtube.com/watch?v=86aHZNYEUjw",
		SourceKind:               "youtube",
		ContentType:              "music",
		EnableLyrics:             &enableLyrics,
		UseManualLyricsSelection: &useManualLyricsSelection,
		ManualLyricsTrackName:    "Titre choisi",
		ManualLyricsArtistName:   "",
		ManualLyricsAlbumName:    "",
		ManualLyricsSynced:       "[00:00.00] Ligne choisie",
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if !built.Request.UseManualLyricsSelection {
		t.Fatalf("expected manual lyrics selection to stay enabled for youtube music")
	}
	if built.Request.ManualLyricsTrackName != "Titre choisi" {
		t.Fatalf("unexpected manual lyrics track name: %q", built.Request.ManualLyricsTrackName)
	}
	if built.Request.ManualLyricsArtistName != "" || built.Request.ManualLyricsAlbumName != "" {
		t.Fatalf("manual artist/album should stay optional, got artist=%q album=%q", built.Request.ManualLyricsArtistName, built.Request.ManualLyricsAlbumName)
	}
	if built.Request.ManualLyricsSynced == "" {
		t.Fatalf("expected synced manual lyrics payload to be kept")
	}
}

func TestBuildJobKeepsManualLyricsSelectionsForQobuzMusic(t *testing.T) {
	enableLyrics := true
	useManualLyricsSelection := true
	c := &Coordinator{}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:                 "https://play.qobuz.com/album/123456",
		SourceKind:               "qobuz",
		ContentType:              "music",
		EnableLyrics:             &enableLyrics,
		UseManualLyricsSelection: &useManualLyricsSelection,
		ManualLyricsSelections: []core.ManualLyricsSelectionInput{
			{
				TargetTrackName: "Intro",
				TargetAlbumName: "Album Demo",
				TrackName:       "Intro",
				ArtistName:      "Artiste Demo",
				SyncedLyrics:    "[00:00.00] Ligne choisie",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if !built.Request.UseManualLyricsSelection {
		t.Fatalf("expected manual lyrics selections to stay enabled for qobuz music")
	}
	if len(built.Request.ManualLyricsSelections) != 1 {
		t.Fatalf("unexpected manual lyrics selections count: %d", len(built.Request.ManualLyricsSelections))
	}
	if built.Request.ManualLyricsSelections[0].TargetTrackName != "Intro" {
		t.Fatalf("unexpected target track name: %q", built.Request.ManualLyricsSelections[0].TargetTrackName)
	}
	if built.Request.ManualLyricsSelections[0].SyncedLyrics == "" {
		t.Fatalf("expected synced lyrics payload to be kept")
	}
}

func TestBuildJobUsesQobuzTokenWorkaroundFromSettings(t *testing.T) {
	c := &Coordinator{
		settings: core.WebSettings{
			QobuzUseUserAuthToken: true,
			QobuzUserAuthToken:    "session.token.value",
		},
	}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:    "https://play.qobuz.com/album/123456",
		SourceKind:  "qobuz",
		ContentType: "music",
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if !built.Request.QobuzUseUserAuthToken {
		t.Fatalf("expected token workaround to be enabled from settings")
	}
	if built.Request.QobuzUserAuthToken != "session.token.value" {
		t.Fatalf("unexpected qobuz token: %q", built.Request.QobuzUserAuthToken)
	}
}

func TestBuildJobCanDisableQobuzTokenWorkaroundFromPayload(t *testing.T) {
	disableTokenMode := false
	c := &Coordinator{
		settings: core.WebSettings{
			QobuzUseUserAuthToken: true,
			QobuzUserAuthToken:    "session.token.value",
		},
	}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:              "https://play.qobuz.com/album/123456",
		SourceKind:            "qobuz",
		ContentType:           "music",
		QobuzUseUserAuthToken: &disableTokenMode,
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if built.Request.QobuzUseUserAuthToken {
		t.Fatalf("expected token workaround to be disabled by payload")
	}
}

func TestBuildJobUsesFavoriteRSSPromptBeforeGlobal(t *testing.T) {
	c := &Coordinator{
		settings: core.WebSettings{
			WhisperInitialPrompt:      "prompt global",
			WhisperCarryInitialPrompt: false,
			FavoriteRSSPodcasts: []core.FavoriteRSSPodcast{{
				FeedURL:                   "https://example.com/feed.xml",
				PodcastTitle:              "Podcast Demo",
				WhisperInitialPrompt:      "prompt podcast",
				WhisperCarryInitialPrompt: true,
			}},
		},
	}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:    "https://cdn.example.com/episode-01.mp3",
		SourceKind:  "rss",
		ContentType: "audio",
		RSSEpisode: &core.RSSEpisodeAPIInput{
			Title:        "Episode 01",
			MediaURL:     "https://cdn.example.com/episode-01.mp3",
			FeedURL:      "https://example.com/feed.xml",
			PodcastTitle: "Podcast Demo",
		},
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if built.Request.WhisperInitialPrompt != "prompt podcast" {
		t.Fatalf("unexpected prompt: %q", built.Request.WhisperInitialPrompt)
	}
	if !built.Request.WhisperCarryInitialPrompt {
		t.Fatalf("expected podcast carry flag to be used")
	}
}

func TestBuildJobUsesJobPromptBeforePodcastAndGlobal(t *testing.T) {
	carry := false
	c := &Coordinator{
		settings: core.WebSettings{
			WhisperInitialPrompt: "prompt global",
			FavoriteRSSPodcasts: []core.FavoriteRSSPodcast{{
				FeedURL:              "https://example.com/feed.xml",
				PodcastTitle:         "Podcast Demo",
				WhisperInitialPrompt: "prompt podcast",
			}},
		},
	}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:                  "https://cdn.example.com/episode-01.mp3",
		SourceKind:                "rss",
		ContentType:               "audio",
		WhisperInitialPrompt:      "prompt job",
		WhisperCarryInitialPrompt: &carry,
		RSSEpisode: &core.RSSEpisodeAPIInput{
			Title:        "Episode 01",
			MediaURL:     "https://cdn.example.com/episode-01.mp3",
			FeedURL:      "https://example.com/feed.xml",
			PodcastTitle: "Podcast Demo",
		},
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if built.Request.WhisperInitialPrompt != "prompt job" {
		t.Fatalf("unexpected prompt: %q", built.Request.WhisperInitialPrompt)
	}
	if built.Request.WhisperCarryInitialPrompt {
		t.Fatalf("expected job carry flag to override podcast/global")
	}
}

func TestBuildJobCanDisablePromptFallbackAtJobLevel(t *testing.T) {
	promptEnabled := false
	c := &Coordinator{
		settings: core.WebSettings{
			WhisperInitialPrompt: "prompt global",
			FavoriteRSSPodcasts: []core.FavoriteRSSPodcast{{
				FeedURL:              "https://example.com/feed.xml",
				PodcastTitle:         "Podcast Demo",
				WhisperInitialPrompt: "prompt podcast",
			}},
		},
	}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:             "https://cdn.example.com/episode-01.mp3",
		SourceKind:           "rss",
		ContentType:          "audio",
		WhisperPromptEnabled: &promptEnabled,
		RSSEpisode: &core.RSSEpisodeAPIInput{
			Title:        "Episode 01",
			MediaURL:     "https://cdn.example.com/episode-01.mp3",
			FeedURL:      "https://example.com/feed.xml",
			PodcastTitle: "Podcast Demo",
		},
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if built.Request.WhisperPromptEnabled {
		t.Fatalf("expected prompt to stay disabled")
	}
	if built.Request.WhisperInitialPrompt != "" {
		t.Fatalf("expected no prompt fallback when disabled, got %q", built.Request.WhisperInitialPrompt)
	}
	if built.Request.WhisperCarryInitialPrompt {
		t.Fatalf("expected carry prompt to be disabled too")
	}
}

func TestBuildJobMigratesLegacyTinydiarizeSettingToProvider(t *testing.T) {
	c := &Coordinator{
		settings: core.WebSettings{
			WhisperTinydiarizeEnabled:   true,
			WhisperTinydiarizeOutputTXT: true,
		},
	}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:    "https://www.youtube.com/watch?v=86aHZNYEUjw",
		SourceKind:  "youtube",
		ContentType: "audio",
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if built.Request.DiarizationProvider != core.DiarizationProviderTinydiarize {
		t.Fatalf("expected tinydiarize provider, got %q", built.Request.DiarizationProvider)
	}
	if !built.Request.WhisperTinydiarizeEnabled {
		t.Fatalf("expected legacy tinydiarize flag to stay in sync")
	}
}

func TestBuildJobAcceptsPyannoteProviderFromPayload(t *testing.T) {
	c := &Coordinator{
		settings: core.WebSettings{
			PyannoteOutputTXT: true,
			PyannoteOutputSRT: true,
		},
	}

	built, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:                 "https://www.youtube.com/watch?v=86aHZNYEUjw",
		SourceKind:               "youtube",
		ContentType:              "audio",
		DiarizationProvider:      "pyannote",
		PyannoteOutputTXT:        boolPtrBuild(true),
		PyannoteOutputSRT:        boolPtrBuild(false),
		PyannoteLocalPipelinePath: "/tmp/pyannote-community-1",
	})
	if err != nil {
		t.Fatalf("buildJob returned error: %v", err)
	}
	if built.Request.DiarizationProvider != core.DiarizationProviderPyannote {
		t.Fatalf("expected pyannote provider, got %q", built.Request.DiarizationProvider)
	}
	if built.Request.WhisperTinydiarizeEnabled {
		t.Fatalf("legacy tinydiarize flag should be false with pyannote")
	}
	if built.Request.PyannoteLocalPipelinePath != "/tmp/pyannote-community-1" {
		t.Fatalf("unexpected pyannote pipeline path: %q", built.Request.PyannoteLocalPipelinePath)
	}
	if !built.Request.PyannoteOutputTXT || built.Request.PyannoteOutputSRT {
		t.Fatalf("unexpected pyannote output toggles: txt=%v srt=%v", built.Request.PyannoteOutputTXT, built.Request.PyannoteOutputSRT)
	}
}

func TestBuildJobRejectsPyannoteWhenTranscriptionDisabled(t *testing.T) {
	disabled := false
	c := &Coordinator{}

	_, err := c.buildJob(core.CreateJobAPIRequest{
		InputURL:            "https://www.youtube.com/watch?v=86aHZNYEUjw",
		SourceKind:          "youtube",
		ContentType:         "music",
		EnableTranscription: &disabled,
		DiarizationProvider: "pyannote",
	})
	if err == nil || !strings.Contains(err.Error(), "diarisation exige une transcription") {
		t.Fatalf("expected explicit pyannote validation error, got=%v", err)
	}
}

func boolPtrBuild(v bool) *bool {
	return &v
}
