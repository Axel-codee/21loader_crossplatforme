package core

import "testing"

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
