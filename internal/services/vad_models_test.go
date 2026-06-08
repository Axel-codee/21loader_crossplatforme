package services

import (
	"path/filepath"
	"testing"
)

func TestVADModelIDFromPath(t *testing.T) {
	path := filepath.Join("/tmp", "ggml-silero-v6.2.0.bin")
	if got := vadModelIDFromPath(path); got != "silero-v6.2.0" {
		t.Fatalf("unexpected VAD model id: %q", got)
	}
}

func TestVADModelSpecByID(t *testing.T) {
	spec, ok := vadModelSpecByID("silero-v5.1.2")
	if !ok {
		t.Fatalf("expected silero-v5.1.2 catalog entry")
	}
	if spec.FileName != "ggml-silero-v5.1.2.bin" {
		t.Fatalf("unexpected VAD model filename: %q", spec.FileName)
	}
}
