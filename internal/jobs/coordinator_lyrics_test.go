package jobs

import (
	"testing"
	"time"

	"21loader-cross/internal/core"
	"21loader-cross/internal/util"
	"21loader-cross/internal/xuuid"
)

func TestExtractLyricsFoundSummary(t *testing.T) {
	chunk := "[lyrics] Recherche 10/11: 10. All I Ask\n" +
		"[lyrics] Sous-titres synchronises generes.\n" +
		"[lyrics] Termine: 10 genere(s), 1 deja present(s), 0 erreur(s).\n"

	found, total, failed, ok := extractLyricsFoundSummary(chunk)
	if !ok {
		t.Fatalf("expected extractLyricsFoundSummary to match")
	}
	if found != 11 || total != 11 {
		t.Fatalf("unexpected summary values: found=%d total=%d", found, total)
	}
	if failed != 0 {
		t.Fatalf("unexpected summary failed count: got=%d want=0", failed)
	}
}

func TestExtractLyricsFoundSummaryNoMatch(t *testing.T) {
	found, total, failed, ok := extractLyricsFoundSummary("[lyrics] Recherche 1/11: 01. Hello\n")
	if ok {
		t.Fatalf("expected no match, got found=%d total=%d failed=%d", found, total, failed)
	}
}

func TestAppendLogLockedKeepsLyricsTotalFromTrackCount(t *testing.T) {
	jobID := xuuid.New()
	rec := core.NewJobRecord(core.JobRequest{
		ID:         jobID,
		CreatedAt:  time.Now().UTC(),
		SourceKind: core.SourceQobuz,
	})
	rec.LyricsTracksTotal = 21
	c := &Coordinator{
		paths:              util.AppPaths{LogsDirectory: t.TempDir()},
		jobs:               []core.JobRecord{rec},
		maxLogCharacters:   160000,
		displayNameByJobID: map[xuuid.UUID]string{},
	}

	c.appendLogLocked(jobID, "[lyrics] Termine: 9 genere(s), 0 deja present(s), 0 erreur(s).\n")

	if got := c.jobs[0].LyricsFound; got != 9 {
		t.Fatalf("unexpected lyrics found count: got=%d want=9", got)
	}
	if got := c.jobs[0].LyricsFoundTotal; got != 21 {
		t.Fatalf("unexpected lyrics total count: got=%d want=21", got)
	}
	if got := c.jobs[0].LyricsFailed; got != 0 {
		t.Fatalf("unexpected lyrics failed count: got=%d want=0", got)
	}
}

func TestAppendLogLockedUpdatesLyricsCountersInRealTime(t *testing.T) {
	jobID := xuuid.New()
	rec := core.NewJobRecord(core.JobRequest{
		ID:         jobID,
		CreatedAt:  time.Now().UTC(),
		SourceKind: core.SourceQobuz,
	})
	rec.LyricsTracksTotal = 5
	c := &Coordinator{
		paths:              util.AppPaths{LogsDirectory: t.TempDir()},
		jobs:               []core.JobRecord{rec},
		maxLogCharacters:   160000,
		displayNameByJobID: map[xuuid.UUID]string{},
	}

	c.appendLogLocked(jobID, "[lyrics] Sous-titres synchronises generes.\n")
	c.appendLogLocked(jobID, "[lyrics] Deja present, piste ignoree.\n")
	c.appendLogLocked(jobID, "[lyrics] Echec Track A: timeout\n")

	if got := c.jobs[0].LyricsFound; got != 2 {
		t.Fatalf("unexpected realtime lyrics found count: got=%d want=2", got)
	}
	if got := c.jobs[0].LyricsFailed; got != 1 {
		t.Fatalf("unexpected realtime lyrics failed count: got=%d want=1", got)
	}
	if got := c.jobs[0].LyricsFoundTotal; got != 5 {
		t.Fatalf("unexpected realtime lyrics total count: got=%d want=5", got)
	}
}

func TestCountLyricsProgressLines(t *testing.T) {
	chunk := "[lyrics] Sous-titres synchronises generes.\n" +
		"[lyrics] Lyrics texte generes.\n" +
		"[lyrics] Deja present, piste ignoree.\n" +
		"[lyrics] Echec Song 1: timeout\n" +
		"[lyrics] Echec Song 2: not found\n"
	if got := countLyricsGeneratedTracks(chunk); got != 2 {
		t.Fatalf("unexpected generated count: got=%d want=2", got)
	}
	if got := countLyricsAlreadyPresentTracks(chunk); got != 1 {
		t.Fatalf("unexpected already-present count: got=%d want=1", got)
	}
	if got := countLyricsFailedTracks(chunk); got != 2 {
		t.Fatalf("unexpected failed count: got=%d want=2", got)
	}
}
