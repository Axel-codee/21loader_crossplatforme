package jobs

import (
	"path/filepath"
	"strings"
)

const (
	whisperFullJSONSuffix       = ".whisper-full.json"
	tinydiarizeJSONSuffix       = ".whisper-tdrz.json"
	tinydiarizeTranscriptSuffix = ".whisper-tdrz.txt"
	tinydiarizeSubtitleSuffix   = ".whisper-tdrz.srt"
	pyannoteJSONSuffix          = ".pyannote.json"
	pyannoteTranscriptSuffix    = ".pyannote.txt"
	pyannoteSubtitleSuffix      = ".pyannote.srt"
)

func artifactPathForMedia(mediaPath, suffix string) string {
	mediaPath = strings.TrimSpace(mediaPath)
	suffix = strings.TrimSpace(suffix)
	if mediaPath == "" || suffix == "" {
		return ""
	}
	base := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	return base + suffix
}

func whisperFullJSONPathForMedia(mediaPath string) string {
	return artifactPathForMedia(mediaPath, whisperFullJSONSuffix)
}

func tinydiarizeJSONPathForMedia(mediaPath string) string {
	return artifactPathForMedia(mediaPath, tinydiarizeJSONSuffix)
}

func tinydiarizeTranscriptPathForMedia(mediaPath string) string {
	return artifactPathForMedia(mediaPath, tinydiarizeTranscriptSuffix)
}

func tinydiarizeSubtitlePathForMedia(mediaPath string) string {
	return artifactPathForMedia(mediaPath, tinydiarizeSubtitleSuffix)
}

func pyannoteJSONPathForMedia(mediaPath string) string {
	return artifactPathForMedia(mediaPath, pyannoteJSONSuffix)
}

func pyannoteTranscriptPathForMedia(mediaPath string) string {
	return artifactPathForMedia(mediaPath, pyannoteTranscriptSuffix)
}

func pyannoteSubtitlePathForMedia(mediaPath string) string {
	return artifactPathForMedia(mediaPath, pyannoteSubtitleSuffix)
}
