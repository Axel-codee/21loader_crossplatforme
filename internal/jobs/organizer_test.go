package jobs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"21loader-cross/internal/core"
)

func TestMoveReplacingFallsBackToCopyForCrossDeviceFile(t *testing.T) {
	sourceRoot := t.TempDir()
	src := filepath.Join(sourceRoot, "audio.webm")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write source file failed: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "target", "audio.webm")

	originalRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errors.New("cross-device link")}
	}
	t.Cleanup(func() {
		renamePath = originalRename
	})

	if err := moveReplacing(src, dst); err != nil {
		t.Fatalf("moveReplacing failed: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be removed after fallback copy, err=%v", err)
	}
	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read destination failed: %v", err)
	}
	if string(content) != "payload" {
		t.Fatalf("destination content mismatch: got=%q", string(content))
	}
}

func TestMoveReplacingFallsBackToCopyForCrossDeviceDirectory(t *testing.T) {
	sourceRoot := t.TempDir()
	src := filepath.Join(sourceRoot, "album")
	if err := os.MkdirAll(filepath.Join(src, "disc1"), 0o755); err != nil {
		t.Fatalf("mkdir source failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "disc1", "01.flac"), []byte("track"), 0o644); err != nil {
		t.Fatalf("write source track failed: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "target", "album")

	originalRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errors.New("cross-device link")}
	}
	t.Cleanup(func() {
		renamePath = originalRename
	})

	if err := moveReplacing(src, dst); err != nil {
		t.Fatalf("moveReplacing failed: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source directory should be removed after fallback copy, err=%v", err)
	}
	content, err := os.ReadFile(filepath.Join(dst, "disc1", "01.flac"))
	if err != nil {
		t.Fatalf("read destination track failed: %v", err)
	}
	if string(content) != "track" {
		t.Fatalf("destination content mismatch: got=%q", string(content))
	}
}

func TestOrganizeDirectoryCompleteMergesMissingFiles(t *testing.T) {
	outputRoot := t.TempDir()
	sourceDir := filepath.Join(t.TempDir(), "Album Demo")
	targetDir := filepath.Join(outputRoot, "qobuz", "Artiste Demo", "Album Demo")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source failed: %v", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target failed: %v", err)
	}

	existingTrack := filepath.Join(targetDir, "01 Existing.flac")
	missingTrackSource := filepath.Join(sourceDir, "02 Missing.flac")
	existingTrackSource := filepath.Join(sourceDir, "01 Existing.flac")
	if err := os.WriteFile(existingTrack, []byte("target-version"), 0o644); err != nil {
		t.Fatalf("write existing target track failed: %v", err)
	}
	if err := os.WriteFile(existingTrackSource, []byte("source-version"), 0o644); err != nil {
		t.Fatalf("write existing source track failed: %v", err)
	}
	if err := os.WriteFile(missingTrackSource, []byte("new-track"), 0o644); err != nil {
		t.Fatalf("write missing source track failed: %v", err)
	}

	organizer := NewOrganizer()
	result, err := organizer.Organize(OrganizationPayload{
		SourceKind:       core.SourceQobuz,
		SourceName:       "Artiste Demo",
		Title:            "Album Demo",
		OriginalInputURL: "https://play.qobuz.com/album/abc123",
		MediaPath:        sourceDir,
		IsMediaDirectory: true,
		OutputRoot:       outputRoot,
	}, core.CollisionComplete)
	if err != nil {
		t.Fatalf("organize failed: %v", err)
	}

	if !samePath(result.MediaPath, targetDir) {
		t.Fatalf("unexpected media path: got=%q want=%q", result.MediaPath, targetDir)
	}
	if _, statErr := os.Stat(filepath.Join(targetDir, "02 Missing.flac")); statErr != nil {
		t.Fatalf("missing track was not merged: %v", statErr)
	}
	content, readErr := os.ReadFile(existingTrack)
	if readErr != nil {
		t.Fatalf("read existing track failed: %v", readErr)
	}
	if string(content) != "target-version" {
		t.Fatalf("existing file should remain untouched, got=%q", string(content))
	}
	if _, statErr := os.Stat(sourceDir); !os.IsNotExist(statErr) {
		t.Fatalf("source directory should be cleaned after merge, err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(targetDir, "album.json")); statErr != nil {
		t.Fatalf("album metadata was not written: %v", statErr)
	}
}

func TestOrganizeFileKeepsWhisperAdditionalArtifacts(t *testing.T) {
	outputRoot := t.TempDir()
	workspace := t.TempDir()
	mediaPath := filepath.Join(workspace, "episode.mp3")
	subtitlePath := filepath.Join(workspace, "episode.srt")
	transcriptPath := filepath.Join(workspace, "episode.txt")
	jsonPath := filepath.Join(workspace, "episode.whisper-full.json")
	tdrzJSONPath := filepath.Join(workspace, "episode.whisper-tdrz.json")
	tdrzTXTPath := filepath.Join(workspace, "episode.whisper-tdrz.txt")
	tdrzSRTPath := filepath.Join(workspace, "episode.whisper-tdrz.srt")
	pyannoteJSONPath := filepath.Join(workspace, "episode.pyannote.json")
	pyannoteTXTPath := filepath.Join(workspace, "episode.pyannote.txt")
	pyannoteSRTPath := filepath.Join(workspace, "episode.pyannote.srt")
	for _, path := range []string{mediaPath, subtitlePath, transcriptPath, jsonPath, tdrzJSONPath, tdrzTXTPath, tdrzSRTPath, pyannoteJSONPath, pyannoteTXTPath, pyannoteSRTPath} {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
			t.Fatalf("write artifact %s failed: %v", path, err)
		}
	}

	organizer := NewOrganizer()
	result, err := organizer.Organize(OrganizationPayload{
		SourceKind:                core.SourceRSS,
		SourceName:                "Podcast Demo",
		Title:                     "Episode 01",
		OriginalInputURL:          "https://cdn.example.com/episode-01.mp3",
		MediaPath:                 mediaPath,
		SubtitleFile:              subtitlePath,
		TranscriptFile:            transcriptPath,
		JSONFile:                  jsonPath,
		TinydiarizeJSONFile:       tdrzJSONPath,
		TinydiarizeTranscriptFile: tdrzTXTPath,
		TinydiarizeSubtitleFile:   tdrzSRTPath,
		PyannoteJSONFile:          pyannoteJSONPath,
		PyannoteTranscriptFile:    pyannoteTXTPath,
		PyannoteSubtitleFile:      pyannoteSRTPath,
		OutputRoot:                outputRoot,
	}, core.CollisionRename)
	if err != nil {
		t.Fatalf("organize failed: %v", err)
	}
	if result.JSONPath == "" || result.TinydiarizeJSONPath == "" || result.TinydiarizeTranscriptPath == "" || result.TinydiarizeSubtitlePath == "" || result.PyannoteJSONPath == "" || result.PyannoteTranscriptPath == "" || result.PyannoteSubtitlePath == "" {
		t.Fatalf("expected all whisper artifacts in job result, got=%+v", result)
	}
	for _, path := range []string{result.JSONPath, result.TinydiarizeJSONPath, result.TinydiarizeTranscriptPath, result.TinydiarizeSubtitlePath, result.PyannoteJSONPath, result.PyannoteTranscriptPath, result.PyannoteSubtitlePath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected organized artifact %s: %v", path, err)
		}
	}
	data, err := os.ReadFile(result.MetadataPath)
	if err != nil {
		t.Fatalf("read metadata failed: %v", err)
	}
	var meta MediaMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("unmarshal metadata failed: %v", err)
	}
	if meta.JSONPath == "" || meta.TinydiarizeJSONPath == "" || meta.TinydiarizeTranscriptPath == "" || meta.TinydiarizeSubtitlePath == "" || meta.PyannoteJSONPath == "" || meta.PyannoteTranscriptPath == "" || meta.PyannoteSubtitlePath == "" {
		t.Fatalf("expected metadata to expose whisper artifacts, got=%+v", meta)
	}
}
