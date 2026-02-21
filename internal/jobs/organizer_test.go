package jobs

import (
	"os"
	"path/filepath"
	"testing"

	"persodl-cross/internal/core"
)

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
