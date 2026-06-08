package jobs

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

const unknownSpeakerLabel = "SPEAKER_UNKNOWN"

type whisperSegment struct {
	Start float64
	End   float64
	Text  string
}

type pyannoteSegment struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker string  `json:"speaker"`
}

type pyannoteDiarizationJSON struct {
	Pipeline          string            `json:"pipeline"`
	Duration          float64           `json:"duration"`
	SpeakerLabels     []string          `json:"speaker_labels"`
	Segments          []pyannoteSegment `json:"segments"`
	ExclusiveSegments []pyannoteSegment `json:"exclusive_segments"`
}

type diarizedWhisperSegment struct {
	Start   float64
	End     float64
	Text    string
	Speaker string
}

func mergeWhisperAndPyannoteJSON(whisperJSONPath, pyannoteJSONPath string) ([]diarizedWhisperSegment, error) {
	whisperSegments, err := readWhisperSegments(whisperJSONPath)
	if err != nil {
		return nil, err
	}
	diarization, err := readPyannoteDiarization(pyannoteJSONPath)
	if err != nil {
		return nil, err
	}
	sourceSegments := diarization.ExclusiveSegments
	if len(sourceSegments) == 0 {
		sourceSegments = diarization.Segments
	}
	out := make([]diarizedWhisperSegment, 0, len(whisperSegments))
	for _, segment := range whisperSegments {
		if strings.TrimSpace(segment.Text) == "" {
			continue
		}
		out = append(out, diarizedWhisperSegment{
			Start:   segment.Start,
			End:     segment.End,
			Text:    strings.TrimSpace(segment.Text),
			Speaker: dominantSpeakerForInterval(segment.Start, segment.End, sourceSegments),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("aucun segment Whisper exploitable dans %s", whisperJSONPath)
	}
	return out, nil
}

func writeAnnotatedTranscript(path string, segments []diarizedWhisperSegment) error {
	lines := make([]string, 0, len(segments))
	for _, segment := range segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" {
			continue
		}
		lines = append(lines, segment.Speaker+": "+text)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func writeAnnotatedSRT(path string, segments []diarizedWhisperSegment) error {
	var b strings.Builder
	index := 1
	for _, segment := range segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" {
			continue
		}
		b.WriteString(strconv.Itoa(index))
		b.WriteString("\n")
		b.WriteString(formatSRTTimestamp(segment.Start))
		b.WriteString(" --> ")
		b.WriteString(formatSRTTimestamp(segment.End))
		b.WriteString("\n")
		b.WriteString(segment.Speaker)
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n\n")
		index++
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func readPyannoteDiarization(path string) (pyannoteDiarizationJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pyannoteDiarizationJSON{}, err
	}
	var payload pyannoteDiarizationJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return pyannoteDiarizationJSON{}, fmt.Errorf("json pyannote invalide: %w", err)
	}
	return payload, nil
}

func readWhisperSegments(path string) ([]whisperSegment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("json Whisper invalide: %w", err)
	}
	segments := extractWhisperSegments(payload)
	if len(segments) == 0 {
		return nil, fmt.Errorf("aucun segment Whisper exploitable dans %s", path)
	}
	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].Start == segments[j].Start {
			return segments[i].End < segments[j].End
		}
		return segments[i].Start < segments[j].Start
	})
	return segments, nil
}

func extractWhisperSegments(payload any) []whisperSegment {
	for _, candidate := range whisperSegmentCollections(payload) {
		segments := decodeWhisperSegmentCollection(candidate)
		if len(segments) > 0 {
			return segments
		}
	}
	return nil
}

func whisperSegmentCollections(payload any) []any {
	collections := []any{}
	if m, ok := payload.(map[string]any); ok {
		for _, key := range []string{"transcription", "segments"} {
			if value, exists := m[key]; exists {
				collections = append(collections, value)
			}
		}
		if result, ok := m["result"].(map[string]any); ok {
			for _, key := range []string{"transcription", "segments"} {
				if value, exists := result[key]; exists {
					collections = append(collections, value)
				}
			}
		}
	}
	return collections
}

func decodeWhisperSegmentCollection(collection any) []whisperSegment {
	entries, ok := collection.([]any)
	if !ok {
		return nil
	}
	out := make([]whisperSegment, 0, len(entries))
	for _, entry := range entries {
		segment, ok := decodeWhisperSegment(entry)
		if ok {
			out = append(out, segment)
		}
	}
	return out
}

func decodeWhisperSegment(entry any) (whisperSegment, bool) {
	m, ok := entry.(map[string]any)
	if !ok {
		return whisperSegment{}, false
	}
	text := strings.TrimSpace(stringValue(m["text"]))
	if text == "" {
		return whisperSegment{}, false
	}

	start, end, ok := extractWhisperSegmentTimes(m)
	if !ok || end <= start {
		return whisperSegment{}, false
	}
	return whisperSegment{Start: start, End: end, Text: text}, true
}

func extractWhisperSegmentTimes(m map[string]any) (float64, float64, bool) {
	start, startOK := numericSecondsValue(m["start"])
	end, endOK := numericSecondsValue(m["end"])
	if startOK && endOK {
		return start, end, true
	}
	if timestamps, ok := m["timestamps"].(map[string]any); ok {
		start, startOK = flexibleTimeValue(timestamps["from"])
		end, endOK = flexibleTimeValue(timestamps["to"])
		if startOK && endOK {
			return start, end, true
		}
	}
	if offsets, ok := m["offsets"].(map[string]any); ok {
		start, startOK = millisecondsValue(offsets["from"])
		end, endOK = millisecondsValue(offsets["to"])
		if startOK && endOK {
			return start, end, true
		}
		start, startOK = millisecondsValue(offsets["from_ms"])
		end, endOK = millisecondsValue(offsets["to_ms"])
		if startOK && endOK {
			return start, end, true
		}
	}
	return 0, 0, false
}

func dominantSpeakerForInterval(start, end float64, segments []pyannoteSegment) string {
	if len(segments) == 0 {
		return unknownSpeakerLabel
	}
	overlaps := map[string]float64{}
	nearestSpeaker := unknownSpeakerLabel
	nearestDistance := -1.0
	for _, segment := range segments {
		speaker := strings.TrimSpace(segment.Speaker)
		if speaker == "" {
			speaker = unknownSpeakerLabel
		}
		overlap := minFloat(end, segment.End) - maxFloat(start, segment.Start)
		if overlap > 0 {
			overlaps[speaker] += overlap
		}
		distance := intervalDistance(start, end, segment.Start, segment.End)
		if nearestDistance < 0 || distance < nearestDistance {
			nearestDistance = distance
			nearestSpeaker = speaker
		}
	}
	bestSpeaker := ""
	bestOverlap := 0.0
	for speaker, overlap := range overlaps {
		if overlap > bestOverlap || (overlap == bestOverlap && (bestSpeaker == "" || speaker < bestSpeaker)) {
			bestSpeaker = speaker
			bestOverlap = overlap
		}
	}
	if bestSpeaker != "" {
		return bestSpeaker
	}
	return nearestSpeaker
}

func intervalDistance(startA, endA, startB, endB float64) float64 {
	if endB < startA {
		return startA - endB
	}
	if startB > endA {
		return startB - endA
	}
	return 0
}

func formatSRTTimestamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	totalMillis := int(seconds*1000 + 0.5)
	hours := totalMillis / 3600000
	totalMillis %= 3600000
	minutes := totalMillis / 60000
	totalMillis %= 60000
	secs := totalMillis / 1000
	millis := totalMillis % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secs, millis)
}

func flexibleTimeValue(value any) (float64, bool) {
	if numeric, ok := numericSecondsValue(value); ok {
		return numeric, true
	}
	text := strings.TrimSpace(stringValue(value))
	if text == "" {
		return 0, false
	}
	if numeric, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", "."), 64); err == nil {
		return numeric, true
	}
	parts := strings.Split(strings.ReplaceAll(text, ",", "."), ":")
	if len(parts) != 3 {
		return 0, false
	}
	hours, errHours := strconv.ParseFloat(parts[0], 64)
	minutes, errMinutes := strconv.ParseFloat(parts[1], 64)
	secs, errSeconds := strconv.ParseFloat(parts[2], 64)
	if errHours != nil || errMinutes != nil || errSeconds != nil {
		return 0, false
	}
	return hours*3600 + minutes*60 + secs, true
}

func millisecondsValue(value any) (float64, bool) {
	numeric, ok := numericSecondsValue(value)
	if !ok {
		return 0, false
	}
	return numeric / 1000.0, true
}

func numericSecondsValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
