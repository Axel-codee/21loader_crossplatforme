package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"21loader-cross/internal/core"
	"21loader-cross/internal/util"
)

func TestNormalizeFavoriteRSSPodcastsTrimsDeduplicatesAndSorts(t *testing.T) {
	input := []core.FavoriteRSSPodcast{
		{
			FeedURL:                   " https://Feeds.360.audion.fm/wdHHhlNbFMLhqQi-Xdw3a#fragment ",
			PodcastTitle:              "  Le podcast B  ",
			PodcastArtworkURL:         " https://cdn.example.com/b.jpg ",
			WhisperInitialPrompt:      "  intro podcast  ",
			WhisperCarryInitialPrompt: true,
		},
		{
			FeedURL:      "",
			PodcastTitle: "Ignore",
		},
		{
			FeedURL:      "https://feeds.360.audion.fm/wdHHhlNbFMLhqQi-Xdw3a",
			PodcastTitle: "Doublon a supprimer",
		},
		{
			FeedURL:      "https://example.com/feed.xml",
			PodcastTitle: "",
		},
		{
			FeedURL:      " https://example.com/alpha.xml ",
			PodcastTitle: "Alpha",
		},
	}

	normalized := normalizeFavoriteRSSPodcasts(input)
	if len(normalized) != 3 {
		t.Fatalf("unexpected favorites count: %d", len(normalized))
	}

	if normalized[0].PodcastTitle != "Alpha" {
		t.Fatalf("unexpected first podcast title: %q", normalized[0].PodcastTitle)
	}
	if normalized[0].FeedURL != "https://example.com/alpha.xml" {
		t.Fatalf("unexpected first feed url: %q", normalized[0].FeedURL)
	}

	if normalized[1].PodcastTitle != "https://example.com/feed.xml" {
		t.Fatalf("expected empty title to fallback to feed url, got %q", normalized[1].PodcastTitle)
	}

	if normalized[2].FeedURL != "https://feeds.360.audion.fm/wdHHhlNbFMLhqQi-Xdw3a" {
		t.Fatalf("unexpected normalized feed url: %q", normalized[2].FeedURL)
	}
	if normalized[2].PodcastTitle != "Le podcast B" {
		t.Fatalf("unexpected normalized title: %q", normalized[2].PodcastTitle)
	}
	if normalized[2].PodcastArtworkURL != "https://cdn.example.com/b.jpg" {
		t.Fatalf("unexpected normalized artwork url: %q", normalized[2].PodcastArtworkURL)
	}
	if normalized[2].WhisperInitialPrompt != "intro podcast" {
		t.Fatalf("unexpected normalized whisper prompt: %q", normalized[2].WhisperInitialPrompt)
	}
	if !normalized[2].WhisperCarryInitialPrompt {
		t.Fatalf("expected carry initial prompt to be preserved")
	}
}

func TestUpdateSettingsNormalizesFavoriteRSSPodcasts(t *testing.T) {
	c := &Coordinator{}
	whisperVADEnabled := true
	whisperOutputJSONFull := true
	podcasts := []core.FavoriteRSSPodcast{
		{
			FeedURL:              " https://Feeds.360.audion.fm/wdHHhlNbFMLhqQi-Xdw3a#fragment ",
			PodcastTitle:         "  Mon podcast  ",
			WhisperInitialPrompt: "  prompt podcast  ",
		},
		{
			FeedURL:      "https://feeds.360.audion.fm/wdHHhlNbFMLhqQi-Xdw3a",
			PodcastTitle: "Doublon",
		},
	}

	saved, err := c.UpdateSettings(core.UpdateSettingsAPIRequest{
		WhisperVADEnabled:     &whisperVADEnabled,
		WhisperOutputJSONFull: &whisperOutputJSONFull,
		FavoriteRSSPodcasts:   &podcasts,
	})
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}

	if len(saved.FavoriteRSSPodcasts) != 1 {
		t.Fatalf("unexpected favorites count after update: %d", len(saved.FavoriteRSSPodcasts))
	}
	if saved.FavoriteRSSPodcasts[0].FeedURL != "https://feeds.360.audion.fm/wdHHhlNbFMLhqQi-Xdw3a" {
		t.Fatalf("unexpected saved feed url: %q", saved.FavoriteRSSPodcasts[0].FeedURL)
	}
	if saved.FavoriteRSSPodcasts[0].PodcastTitle != "Mon podcast" {
		t.Fatalf("unexpected saved podcast title: %q", saved.FavoriteRSSPodcasts[0].PodcastTitle)
	}
	if saved.FavoriteRSSPodcasts[0].WhisperInitialPrompt != "prompt podcast" {
		t.Fatalf("unexpected saved podcast prompt: %q", saved.FavoriteRSSPodcasts[0].WhisperInitialPrompt)
	}
	if !saved.WhisperVADEnabled {
		t.Fatalf("expected whisper VAD setting to be persisted")
	}
	if !saved.WhisperOutputJSONFull {
		t.Fatalf("expected whisper JSON full setting to be persisted")
	}
}

func TestUpdateSettingsNormalizesYouTubeAudioFormat(t *testing.T) {
	c := &Coordinator{}
	format := " OPUS "

	saved, err := c.UpdateSettings(core.UpdateSettingsAPIRequest{
		YouTubeAudioFormat: &format,
	})
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}
	if saved.YouTubeAudioFormat != "opus" {
		t.Fatalf("expected normalized opus format, got %q", saved.YouTubeAudioFormat)
	}
}

func TestUpdateSettingsNormalizesYouTubeAudioPreferences(t *testing.T) {
	c := &Coordinator{}
	preferences := []string{" native:M4A ", "convert:M4A", "convert:MP3", "convert:MP3"}

	saved, err := c.UpdateSettings(core.UpdateSettingsAPIRequest{
		YouTubeAudioPreferences: &preferences,
	})
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}
	expected := []string{"native:m4a", "convert:m4a", "convert:mp3"}
	if strings.Join(saved.YouTubeAudioPreferences, ",") != strings.Join(expected, ",") {
		t.Fatalf("expected normalized preferences %v, got %v", expected, saved.YouTubeAudioPreferences)
	}
}

func TestUpdateSettingsRejectsInvalidYouTubeAudioPreference(t *testing.T) {
	c := &Coordinator{}
	preferences := []string{"native:mp3"}

	_, err := c.UpdateSettings(core.UpdateSettingsAPIRequest{
		YouTubeAudioPreferences: &preferences,
	})
	if err == nil || !strings.Contains(err.Error(), "youtubeAudioPreferences invalide") {
		t.Fatalf("expected invalid preference error, got %v", err)
	}
}

func TestUpdateSettingsPersistsYtDlpEmbeddingOptions(t *testing.T) {
	c := &Coordinator{}
	embedMetadata := false
	embedThumbnail := false

	saved, err := c.UpdateSettings(core.UpdateSettingsAPIRequest{
		YtDlpEmbedMetadata:  &embedMetadata,
		YtDlpEmbedThumbnail: &embedThumbnail,
	})
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}
	if saved.YtDlpEmbedMetadata || saved.YtDlpEmbedThumbnail {
		t.Fatalf("expected embedding options disabled, got metadata=%v thumbnail=%v", saved.YtDlpEmbedMetadata, saved.YtDlpEmbedThumbnail)
	}
}

func TestUpdateSettingsRejectsInvalidYouTubeAudioFormat(t *testing.T) {
	c := &Coordinator{}
	format := "webm"

	_, err := c.UpdateSettings(core.UpdateSettingsAPIRequest{
		YouTubeAudioFormat: &format,
	})
	if err == nil || !strings.Contains(err.Error(), "youtubeAudioFormat invalide") {
		t.Fatalf("expected invalid format error, got %v", err)
	}
}

func TestLoadSettingsMigratesLegacyTinydiarizeToProvider(t *testing.T) {
	tmp := t.TempDir()
	settingsPath := filepath.Join(tmp, "web-settings.json")
	legacyJSON := `{"whisperTinydiarizeEnabled":true}`
	if err := os.WriteFile(settingsPath, []byte(legacyJSON), 0o644); err != nil {
		t.Fatalf("write settings failed: %v", err)
	}

	c := &Coordinator{paths: util.AppPaths{WebSettingsFile: settingsPath}}
	loaded := c.loadSettings()
	if loaded.DiarizationProvider != core.DiarizationProviderTinydiarize {
		t.Fatalf("expected migrated provider tinydiarize, got %q", loaded.DiarizationProvider)
	}
	if !loaded.WhisperTinydiarizeEnabled {
		t.Fatalf("expected legacy tinydiarize flag to remain true after migration")
	}
	if !loaded.PyannoteOutputTXT || !loaded.PyannoteOutputSRT {
		t.Fatalf("expected pyannote output defaults to be enabled, got txt=%v srt=%v", loaded.PyannoteOutputTXT, loaded.PyannoteOutputSRT)
	}
	if loaded.YouTubeAudioFormat != "mp3" {
		t.Fatalf("expected youtube audio format default mp3, got %q", loaded.YouTubeAudioFormat)
	}
	if strings.Join(loaded.YouTubeAudioPreferences, ",") != "convert:mp3" {
		t.Fatalf("expected migrated youtube audio preferences convert:mp3, got %v", loaded.YouTubeAudioPreferences)
	}
	if !loaded.YtDlpEmbedMetadata || !loaded.YtDlpEmbedThumbnail {
		t.Fatalf("expected yt-dlp embedding defaults enabled, got metadata=%v thumbnail=%v", loaded.YtDlpEmbedMetadata, loaded.YtDlpEmbedThumbnail)
	}
}
