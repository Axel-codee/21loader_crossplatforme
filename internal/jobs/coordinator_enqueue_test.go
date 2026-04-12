package jobs

import (
	"context"
	"strings"
	"testing"
	"time"

	"21loader-cross/internal/core"
	"21loader-cross/internal/xuuid"
)

func TestEnqueueRejectsEquivalentCompleteJobAlreadyPresent(t *testing.T) {
	jobID := xuuid.New()
	createdAt := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	c := &Coordinator{
		activeHas: true,
		jobs: []core.JobRecord{
			{
				ID: jobID,
				Request: core.JobRequest{
					ID:                        jobID,
					CreatedAt:                 createdAt,
					SourceKind:                core.SourceQobuz,
					ContentType:               core.ContentMusic,
					InputURL:                  "https://play.qobuz.com/album/123456",
					OutputRootPath:            "/tmp/output",
					EnableLyrics:              true,
					TranscriptionLanguage:     "auto",
					TranslationSourceLanguage: "en",
					TranslationTargetLanguage: "fr",
					QobuzArtistName:           "Artiste Demo",
				},
				Status:         core.StatusCompleted,
				CompletedSteps: map[core.JobStep]bool{},
				ReusedSteps:    map[core.JobStep]bool{},
				StepElapsed:    map[core.JobStep]time.Duration{},
			},
		},
		optionsByJobID: map[xuuid.UUID]JobExecutionOptions{
			jobID: {StandardCollision: core.CollisionComplete},
		},
		displayNameByJobID: map[xuuid.UUID]string{
			jobID: "Album Demo",
		},
	}

	_, err := c.Enqueue(context.Background(), core.CreateJobAPIRequest{
		InputURL:          "https://play.qobuz.com/album/123456",
		SourceKind:        "qobuz",
		ContentType:       "music",
		OutputRootPath:    "/tmp/output",
		CollisionPolicy:   "complete",
		EnableLyrics:      boolPtr(true),
		DisplayName:       "Album Demo",
		QobuzArtistName:   "Artiste Demo",
		QobuzPlaylistName: "",
	})
	if err == nil {
		t.Fatalf("expected duplicate enqueue to be rejected")
	}
	if !strings.Contains(err.Error(), "job identique") {
		t.Fatalf("unexpected duplicate error: %v", err)
	}
}

func TestEnqueueAllowsSameInputWhenCollisionIsRename(t *testing.T) {
	jobID := xuuid.New()
	createdAt := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	c := &Coordinator{
		activeHas: true,
		jobs: []core.JobRecord{
			{
				ID: jobID,
				Request: core.JobRequest{
					ID:             jobID,
					CreatedAt:      createdAt,
					SourceKind:     core.SourceQobuz,
					ContentType:    core.ContentMusic,
					InputURL:       "https://play.qobuz.com/album/123456",
					OutputRootPath: "/tmp/output",
				},
				Status:         core.StatusCompleted,
				CompletedSteps: map[core.JobStep]bool{},
				ReusedSteps:    map[core.JobStep]bool{},
				StepElapsed:    map[core.JobStep]time.Duration{},
			},
		},
		optionsByJobID:     map[xuuid.UUID]JobExecutionOptions{jobID: {StandardCollision: core.CollisionComplete}},
		displayNameByJobID: map[xuuid.UUID]string{},
	}

	created, err := c.Enqueue(context.Background(), core.CreateJobAPIRequest{
		InputURL:        "https://play.qobuz.com/album/123456",
		SourceKind:      "qobuz",
		ContentType:     "music",
		OutputRootPath:  "/tmp/output",
		CollisionPolicy: "rename",
	})
	if err != nil {
		t.Fatalf("expected rename collision to allow a new job, got error: %v", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		t.Fatalf("expected a created job summary with an id")
	}
	if len(c.jobs) != 2 {
		t.Fatalf("expected second job to be enqueued, got %d jobs", len(c.jobs))
	}
}

func boolPtr(value bool) *bool {
	return &value
}
