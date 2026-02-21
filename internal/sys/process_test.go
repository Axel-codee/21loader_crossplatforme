package sys

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const runnerHelperFlag = "--runner-helper"

func TestRunnerHelperProcess(t *testing.T) {
	helperIndex := -1
	for i, arg := range os.Args {
		if arg == runnerHelperFlag {
			helperIndex = i
			break
		}
	}
	if helperIndex == -1 {
		return
	}
	if helperIndex+1 >= len(os.Args) {
		os.Exit(2)
	}

	switch os.Args[helperIndex+1] {
	case "long-line":
		_, _ = os.Stdout.WriteString(strings.Repeat("x", 200*1024))
		os.Exit(0)
	case "carriage-progress":
		_, _ = os.Stdout.WriteString("0%\r")
		_, _ = os.Stdout.WriteString("50%\r")
		_, _ = os.Stdout.WriteString("100%")
		os.Exit(0)
	case "chunk-progress":
		for i, chunk := range []string{"10%", "50%", "100%"} {
			_, _ = os.Stdout.WriteString(chunk)
			if i < 2 {
				time.Sleep(120 * time.Millisecond)
			}
		}
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func TestRunnerRunCapturesLongSingleLineOutput(t *testing.T) {
	runner := &Runner{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := runner.Run(ctx, RunOptions{
		Executable: os.Args[0],
		Args: []string{
			"-test.run=TestRunnerHelperProcess",
			"--",
			runnerHelperFlag,
			"long-line",
		},
		CaptureOutput: true,
	})
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}
	if len(output) != 200*1024 {
		t.Fatalf("unexpected output size: got=%d want=%d", len(output), 200*1024)
	}
	if strings.Trim(output, "x") != "" {
		t.Fatalf("output contains unexpected characters")
	}
}

func TestRunnerRunStreamsCarriageReturnProgress(t *testing.T) {
	runner := &Runner{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var chunks []string
	output, err := runner.Run(ctx, RunOptions{
		Executable: os.Args[0],
		Args: []string{
			"-test.run=TestRunnerHelperProcess",
			"--",
			runnerHelperFlag,
			"carriage-progress",
		},
		OnOutput: func(line string) {
			chunks = append(chunks, strings.TrimSpace(line))
		},
		CaptureOutput: true,
	})
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}
	if !contains(chunks, "0%") || !contains(chunks, "50%") || !contains(chunks, "100%") {
		t.Fatalf("expected incremental chunks, got: %v", chunks)
	}
	if !strings.Contains(output, "0%") || !strings.Contains(output, "50%") || !strings.Contains(output, "100%") {
		t.Fatalf("expected captured output to contain all progress values, got: %q", output)
	}
}

func TestRunnerRunStreamsChunkProgressWithoutSeparators(t *testing.T) {
	runner := &Runner{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunkCount := 0
	output, err := runner.Run(ctx, RunOptions{
		Executable: os.Args[0],
		Args: []string{
			"-test.run=TestRunnerHelperProcess",
			"--",
			runnerHelperFlag,
			"chunk-progress",
		},
		OnOutput: func(line string) {
			if strings.TrimSpace(line) != "" {
				chunkCount++
			}
		},
		CaptureOutput: true,
	})
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}
	if chunkCount < 2 {
		t.Fatalf("expected at least 2 streamed chunks, got %s (%d)", strconv.Quote(output), chunkCount)
	}
	if !strings.Contains(output, "10%") || !strings.Contains(output, "50%") || !strings.Contains(output, "100%") {
		t.Fatalf("expected captured output to contain all progress values, got: %q", output)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
