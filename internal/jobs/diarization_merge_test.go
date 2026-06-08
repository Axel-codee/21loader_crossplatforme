package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeWhisperAndPyannoteJSONUsesExclusiveSegments(t *testing.T) {
	tmp := t.TempDir()
	whisperPath := filepath.Join(tmp, "transcription.json")
	pyannotePath := filepath.Join(tmp, "transcription.pyannote.json")

	whisperJSON := `{
	  "transcription": [
	    {
	      "timestamps": {"from": "00:00:00,000", "to": "00:00:01,500"},
	      "text": "Bonjour a tous"
	    },
	    {
	      "timestamps": {"from": "00:00:01,500", "to": "00:00:03,000"},
	      "text": "Salut Axel"
	    }
	  ]
	}`
	pyannoteJSON := `{
	  "pipeline": "pyannote/speaker-diarization-community-1",
	  "duration": 3.0,
	  "speaker_labels": ["SPEAKER_00", "SPEAKER_01"],
	  "segments": [
	    {"start": 0.0, "end": 1.6, "speaker": "SPEAKER_00"},
	    {"start": 1.6, "end": 3.0, "speaker": "SPEAKER_01"}
	  ],
	  "exclusive_segments": [
	    {"start": 0.0, "end": 1.0, "speaker": "SPEAKER_00"},
	    {"start": 1.0, "end": 3.0, "speaker": "SPEAKER_01"}
	  ]
	}`

	if err := os.WriteFile(whisperPath, []byte(whisperJSON), 0o644); err != nil {
		t.Fatalf("write whisper json failed: %v", err)
	}
	if err := os.WriteFile(pyannotePath, []byte(pyannoteJSON), 0o644); err != nil {
		t.Fatalf("write pyannote json failed: %v", err)
	}

	segments, err := mergeWhisperAndPyannoteJSON(whisperPath, pyannotePath)
	if err != nil {
		t.Fatalf("mergeWhisperAndPyannoteJSON failed: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("unexpected merged segment count: %d", len(segments))
	}
	if segments[0].Speaker != "SPEAKER_00" {
		t.Fatalf("unexpected first speaker: %q", segments[0].Speaker)
	}
	if segments[1].Speaker != "SPEAKER_01" {
		t.Fatalf("unexpected second speaker: %q", segments[1].Speaker)
	}

	annotatedSRT := filepath.Join(tmp, "annotated.srt")
	if err := writeAnnotatedSRT(annotatedSRT, segments); err != nil {
		t.Fatalf("writeAnnotatedSRT failed: %v", err)
	}
	content, err := os.ReadFile(annotatedSRT)
	if err != nil {
		t.Fatalf("read annotated srt failed: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "SPEAKER_00: Bonjour a tous") || !strings.Contains(text, "SPEAKER_01: Salut Axel") {
		t.Fatalf("annotated srt missing speaker labels: %s", text)
	}
}

func TestReadWhisperSegmentsSupportsOffsetsFormat(t *testing.T) {
	tmp := t.TempDir()
	whisperPath := filepath.Join(tmp, "transcription.json")
	whisperJSON := `{
	  "result": {
	    "segments": [
	      {
	        "offsets": {"from": 0, "to": 2100},
	        "text": "Segment avec offsets"
	      }
	    ]
	  }
	}`
	if err := os.WriteFile(whisperPath, []byte(whisperJSON), 0o644); err != nil {
		t.Fatalf("write whisper json failed: %v", err)
	}

	segments, err := readWhisperSegments(whisperPath)
	if err != nil {
		t.Fatalf("readWhisperSegments failed: %v", err)
	}
	if len(segments) != 1 {
		t.Fatalf("unexpected segment count: %d", len(segments))
	}
	if segments[0].Start != 0 || segments[0].End != 2.1 {
		t.Fatalf("unexpected segment timing: %+v", segments[0])
	}
}
