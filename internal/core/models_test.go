package core

import (
	"testing"
	"time"
)

func TestProgressPercentYouTubeDownloadUsesStepProgress(t *testing.T) {
	step := StepDownload
	rec := JobRecord{
		Status:              StatusRunning,
		Request:             JobRequest{SourceKind: SourceYouTube},
		CurrentStep:         &step,
		CurrentStepProgress: 0.427,
		CompletedSteps:      map[JobStep]bool{},
	}
	if got := rec.ProgressPercent(); got != 43 {
		t.Fatalf("unexpected progress percent: got=%d want=43", got)
	}
}

func TestProgressPercentNonYouTubeKeepsGlobalWeighting(t *testing.T) {
	step := StepDownload
	rec := JobRecord{
		Status:              StatusRunning,
		Request:             JobRequest{SourceKind: SourceRSS},
		CurrentStep:         &step,
		CurrentStepProgress: 0.5,
		CompletedSteps:      map[JobStep]bool{},
	}
	// 5 steps total -> 0.5/5 = 10%
	if got := rec.ProgressPercent(); got != 10 {
		t.Fatalf("unexpected weighted progress percent: got=%d want=10", got)
	}
}

func TestTotalElapsedUsesNowForRunningJob(t *testing.T) {
	start := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	now := start.Add(95 * time.Second)
	rec := JobRecord{StartedAt: &start}

	if got := rec.TotalElapsed(now); got != 95*time.Second {
		t.Fatalf("unexpected total elapsed duration: got=%v want=%v", got, 95*time.Second)
	}
}

func TestTotalElapsedUsesEndedAtForFinishedJob(t *testing.T) {
	start := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	end := start.Add(3*time.Minute + 12*time.Second)
	now := end.Add(2 * time.Hour)
	rec := JobRecord{StartedAt: &start, EndedAt: &end}

	if got := rec.TotalElapsed(now); got != 3*time.Minute+12*time.Second {
		t.Fatalf("unexpected finished total elapsed duration: got=%v want=%v", got, 3*time.Minute+12*time.Second)
	}
}

func TestActiveStepElapsedRequiresActiveStep(t *testing.T) {
	start := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	now := start.Add(20 * time.Second)
	rec := JobRecord{StartedAt: &start}

	if got := rec.ActiveStepElapsed(now); got != 0 {
		t.Fatalf("unexpected active step elapsed without current step: got=%v want=0", got)
	}
}

func TestActiveStepElapsedUsesCurrentStepStart(t *testing.T) {
	start := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	stepStart := start.Add(37 * time.Second)
	now := stepStart.Add(44 * time.Second)
	step := StepTranscription
	rec := JobRecord{
		StartedAt:            &start,
		CurrentStep:          &step,
		CurrentStepStartedAt: &stepStart,
	}

	if got := rec.ActiveStepElapsed(now); got != 44*time.Second {
		t.Fatalf("unexpected active step elapsed duration: got=%v want=%v", got, 44*time.Second)
	}
}
