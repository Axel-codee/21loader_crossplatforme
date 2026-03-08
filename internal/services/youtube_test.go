package services

import "testing"

func TestParseResolvedMetadataLine(t *testing.T) {
	t.Run("parses date and duration tuple", func(t *testing.T) {
		date, duration, ok := parseResolvedMetadataLine("20240211\t367")
		if !ok {
			t.Fatalf("expected parsing success")
		}
		if date == nil {
			t.Fatalf("expected upload date to be parsed")
		}
		if got := date.Format("2006-01-02"); got != "2024-02-11" {
			t.Fatalf("unexpected upload date: %s", got)
		}
		if duration == nil || *duration != 367 {
			t.Fatalf("unexpected duration: %+v", duration)
		}
	})

	t.Run("ignores warning noise", func(t *testing.T) {
		date, duration, ok := parseResolvedMetadataLine("WARNING: temporary issue")
		if ok || date != nil || duration != nil {
			t.Fatalf("warning lines must be ignored")
		}
	})

	t.Run("parses standalone duration", func(t *testing.T) {
		date, duration, ok := parseResolvedMetadataLine("1:02:03")
		if !ok {
			t.Fatalf("expected parsing success")
		}
		if date != nil {
			t.Fatalf("date must be nil for duration-only line")
		}
		if duration == nil || *duration != 3723 {
			t.Fatalf("unexpected parsed duration: %+v", duration)
		}
	})

	t.Run("handles NA values", func(t *testing.T) {
		date, duration, ok := parseResolvedMetadataLine("NA\tNA")
		if ok || date != nil || duration != nil {
			t.Fatalf("NA tuple must be ignored")
		}
	})
}

func TestResolveDatesConcurrencyLimitFromEnv(t *testing.T) {
	t.Setenv("LOADER21_YT_DATES_CONCURRENCY", "2")
	if got := resolveDatesConcurrencyLimit(); got != 2 {
		t.Fatalf("unexpected concurrency limit: got=%d want=2", got)
	}

	t.Setenv("LOADER21_YT_DATES_CONCURRENCY", "999")
	if got := resolveDatesConcurrencyLimit(); got != 8 {
		t.Fatalf("unexpected capped concurrency limit: got=%d want=8", got)
	}
}

func TestFirstPrintedYouTubeTitle(t *testing.T) {
	t.Run("returns first plain non-empty line", func(t *testing.T) {
		got := firstPrintedYouTubeTitle("\nVideo Title\n")
		if got != "Video Title" {
			t.Fatalf("unexpected title: %q", got)
		}
	})

	t.Run("skips warning and error prefixes", func(t *testing.T) {
		got := firstPrintedYouTubeTitle("WARNING: temporary issue\nERROR: something broke\nFinal Title")
		if got != "Final Title" {
			t.Fatalf("unexpected title after warnings/errors: %q", got)
		}
	})

	t.Run("returns empty when no valid lines", func(t *testing.T) {
		got := firstPrintedYouTubeTitle(" \nWARNING: nope\nERROR: nope\n")
		if got != "" {
			t.Fatalf("expected empty title, got %q", got)
		}
	})
}
