package jobs

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"21loader-cross/internal/core"
	"21loader-cross/internal/sys"
	"21loader-cross/internal/util"
)

const qobuzUserAuthTokenEnv = "LOADER21_QOBUZ_USER_AUTH_TOKEN"
const qobuzEmailEnv = "LOADER21_QOBUZ_EMAIL"

type Runner struct {
	processRunner         *sys.Runner
	organizer             *Organizer
	httpClient            *http.Client
	paths                 util.AppPaths
	argosScript           string
	pyannoteScript        string
	qobuzCLIWrapperScript string
}

type RunCallbacks struct {
	OnStep              func(core.JobStep)
	OnStepProgress      func(float64)
	OnStepCount         func(int, int)
	OnLog               func(string)
	OnStepReused        func(core.JobStep)
	OnTranslationReused func()
	OnDisplayName       func(string)
}

type RunOptions struct {
	StandardCollision           core.CollisionDecision
	QobuzExistingAlbumCollision core.CollisionDecision
	KeepTemporaryFilesOnFailure bool
}

type downloadArtifact struct {
	MediaPath       string
	Title           string
	SourceName      string
	PublicationDate *time.Time
	IsDirectory     bool
	ArtworkPath     string
}

type existingOutput struct {
	MediaPath                 string
	SubtitlePath              string
	TranscriptPath            string
	JSONPath                  string
	TinydiarizeJSONPath       string
	TinydiarizeTranscriptPath string
	TinydiarizeSubtitlePath   string
	PyannoteJSONPath          string
	PyannoteTranscriptPath    string
	PyannoteSubtitlePath      string
	MetadataPath              string
	Title                     string
	SourceName                string
	PublicationDate           *time.Time
}

type translationVariantArtifacts struct {
	SourceLanguage           string
	TargetLanguage           string
	OriginalSubtitlePath     string
	OriginalTranscriptPath   string
	TranslatedSubtitlePath   string
	TranslatedTranscriptPath string
}

type transcriptionArtifacts struct {
	SubtitlePath              string
	TranscriptPath            string
	JSONPath                  string
	InternalWhisperJSONPath   string
	TinydiarizeJSONPath       string
	TinydiarizeTranscriptPath string
	TinydiarizeSubtitlePath   string
	PyannoteJSONPath          string
	PyannoteTranscriptPath    string
	PyannoteSubtitlePath      string
}

func (a translationVariantArtifacts) hasAny() bool {
	return strings.TrimSpace(a.OriginalSubtitlePath) != "" ||
		strings.TrimSpace(a.OriginalTranscriptPath) != "" ||
		strings.TrimSpace(a.TranslatedSubtitlePath) != "" ||
		strings.TrimSpace(a.TranslatedTranscriptPath) != ""
}

func NewRunner(proc *sys.Runner, organizer *Organizer, paths util.AppPaths, baseDir string) *Runner {
	baseDir = strings.TrimSpace(baseDir)
	return &Runner{
		processRunner:         proc,
		organizer:             organizer,
		httpClient:            &http.Client{Timeout: 25 * time.Second},
		paths:                 paths,
		argosScript:           filepath.Join(baseDir, "assets", "scripts", "argos_translate_file.py"),
		pyannoteScript:        filepath.Join(baseDir, "assets", "scripts", "pyannote_diarize.py"),
		qobuzCLIWrapperScript: filepath.Join(baseDir, "assets", "scripts", "qobuz_cli_wrapper.py"),
	}
}

func (r *Runner) Cancel() {
	r.processRunner.CancelCurrentProcess()
}

func (r *Runner) Pause() bool {
	return r.processRunner.PauseCurrentProcess()
}

func (r *Runner) Resume() bool {
	return r.processRunner.ResumeCurrentProcess()
}

func (r *Runner) SearchLRCLIBCandidates(ctx context.Context, track, artistHint, albumHint string, limit int) (core.LRCLIBSearchAPIResponse, error) {
	track = strings.TrimSpace(track)
	if track == "" {
		return core.LRCLIBSearchAPIResponse{}, fmt.Errorf("le champ trackName est requis")
	}
	if limit <= 0 {
		limit = 8
	}
	candidates, err := searchLRCLIBCandidates(ctx, r.httpClient, track, artistHint, albumHint)
	if err != nil {
		return core.LRCLIBSearchAPIResponse{}, err
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	results := make([]core.LRCLIBSearchResultDTO, 0, len(candidates))
	for idx, candidate := range candidates {
		results = append(results, core.LRCLIBSearchResultDTO{
			ID:           fmt.Sprintf("lrclib-%d", idx+1),
			TrackName:    strings.TrimSpace(candidate.trackName),
			ArtistName:   strings.TrimSpace(candidate.artistName),
			AlbumName:    strings.TrimSpace(candidate.albumName),
			PlainLyrics:  candidate.payload.plainLyrics,
			SyncedLyrics: candidate.payload.syncedLyrics,
			Preview:      previewLRCLIBText(candidate.payload),
			HasSynced:    strings.TrimSpace(candidate.payload.syncedLyrics) != "",
			Score:        candidate.score,
		})
	}
	return core.LRCLIBSearchAPIResponse{Results: results}, nil
}

func (r *Runner) Run(ctx context.Context, job core.JobRequest, opt RunOptions, cb RunCallbacks) (core.JobResult, error) {
	workspace := r.paths.WorkspaceDirectory(job.ID.String())
	_ = os.RemoveAll(workspace)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return core.JobResult{}, err
	}
	completed := false
	defer func() {
		if completed || !opt.KeepTemporaryFilesOnFailure {
			_ = os.RemoveAll(workspace)
		}
	}()

	outputRoot := strings.TrimSpace(job.OutputRootPath)
	if outputRoot == "" || outputRoot == "/" {
		return core.JobResult{}, fmt.Errorf("le dossier de sortie est invalide")
	}

	useCompletion := opt.StandardCollision == core.CollisionComplete
	reusedOutput := existingOutput{}
	if useCompletion {
		reusedOutput = r.findExistingOutputForCompletion(job, outputRoot)
	}
	reusingExistingOutput := strings.TrimSpace(reusedOutput.MediaPath) != ""

	if cb.OnStep != nil {
		cb.OnStep(core.StepDownload)
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0)
	}

	var artifact downloadArtifact
	var err error
	if reusingExistingOutput {
		if cb.OnStepReused != nil {
			cb.OnStepReused(core.StepDownload)
		}
		if cb.OnLog != nil {
			cb.OnLog("[download] Mode completer: media existant detecte, telechargement ignore.\n")
		}
		title := strings.TrimSpace(reusedOutput.Title)
		if title == "" {
			title = strings.TrimSpace(strings.TrimSuffix(filepath.Base(reusedOutput.MediaPath), filepath.Ext(reusedOutput.MediaPath)))
		}
		sourceName := strings.TrimSpace(reusedOutput.SourceName)
		if sourceName == "" {
			sourceName = "Source inconnue"
		}
		artifact = downloadArtifact{
			MediaPath:       reusedOutput.MediaPath,
			Title:           title,
			SourceName:      sourceName,
			PublicationDate: reusedOutput.PublicationDate,
			IsDirectory:     false,
		}
	} else if job.SourceKind == core.SourceQobuz &&
		opt.QobuzExistingAlbumCollision == core.CollisionFetchMissingLyrics &&
		isQobuzAlbumURL(job.InputURL) {
		if existing := r.findExistingQobuzAlbumDirectory(job.InputURL, outputRoot); existing != "" {
			if cb.OnLog != nil {
				cb.OnLog("[qobuz] Telechargement ignore: album existant reutilise.\n")
			}
			metadata := r.readQobuzFolderMetadata(existing, job.QobuzArtistName)
			artifact = downloadArtifact{MediaPath: existing, Title: metadata.albumTitle, SourceName: metadata.artistName, IsDirectory: true}
		} else {
			artifact, err = r.download(ctx, job, outputRoot, workspace, cb)
		}
	} else {
		artifact, err = r.download(ctx, job, outputRoot, workspace, cb)
	}
	if err != nil {
		return core.JobResult{}, err
	}
	if cb.OnDisplayName != nil {
		cb.OnDisplayName(strings.TrimSpace(artifact.Title))
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(1)
	}

	if cb.OnStep != nil {
		cb.OnStep(core.StepLyrics)
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0)
	}
	if cb.OnStepCount != nil {
		cb.OnStepCount(0, 0)
	}
	if shouldFetchLyrics(job, artifact) {
		if err := r.fetchLyricsForJob(ctx, job, artifact, cb); err != nil {
			return core.JobResult{}, err
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(1)
		}
	} else {
		if cb.OnLog != nil {
			if job.ContentType == core.ContentMusic && !job.EnableLyrics {
				cb.OnLog("[lyrics] Etape ignoree (desactivee).\n")
			} else {
				cb.OnLog("[lyrics] Etape ignoree (non applicable).\n")
			}
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(1)
		}
	}

	if cb.OnStep != nil {
		cb.OnStep(core.StepTranscription)
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0)
	}

	subtitleFile := ""
	transcriptFile := ""
	transcriptionOutput := transcriptionArtifacts{}
	var translationArtifacts translationVariantArtifacts
	sourceLanguage := normalizeLanguageCode(job.TranslationSourceLanguage, "en")
	targetLanguage := normalizeLanguageCode(job.TranslationTargetLanguage, "fr")
	targetTag := languageFileTag(targetLanguage)
	if shouldTranscribe(job) {
		if reusingExistingOutput {
			preferredLanguages := []string{sourceLanguage, normalizeLanguageCode(job.TranscriptionLanguage, "")}
			subtitleFile = findPreferredSidecarForCompletion(
				artifact.MediaPath,
				".srt",
				preferredLanguages,
				reusedOutput.SubtitlePath,
			)
			transcriptFile = findPreferredSidecarForCompletion(
				artifact.MediaPath,
				".txt",
				preferredLanguages,
				reusedOutput.TranscriptPath,
			)
			transcriptionOutput.JSONPath = firstExistingFile(reusedOutput.JSONPath, whisperFullJSONPathForMedia(artifact.MediaPath))
			transcriptionOutput.InternalWhisperJSONPath = transcriptionOutput.JSONPath
			transcriptionOutput.TinydiarizeJSONPath = firstExistingFile(reusedOutput.TinydiarizeJSONPath, tinydiarizeJSONPathForMedia(artifact.MediaPath))
			transcriptionOutput.TinydiarizeTranscriptPath = firstExistingFile(reusedOutput.TinydiarizeTranscriptPath, tinydiarizeTranscriptPathForMedia(artifact.MediaPath))
			transcriptionOutput.TinydiarizeSubtitlePath = firstExistingFile(reusedOutput.TinydiarizeSubtitlePath, tinydiarizeSubtitlePathForMedia(artifact.MediaPath))
			transcriptionOutput.PyannoteJSONPath = firstExistingFile(reusedOutput.PyannoteJSONPath, pyannoteJSONPathForMedia(artifact.MediaPath))
			transcriptionOutput.PyannoteTranscriptPath = firstExistingFile(reusedOutput.PyannoteTranscriptPath, pyannoteTranscriptPathForMedia(artifact.MediaPath))
			transcriptionOutput.PyannoteSubtitlePath = firstExistingFile(reusedOutput.PyannoteSubtitlePath, pyannoteSubtitlePathForMedia(artifact.MediaPath))
			if canReuseTranscriptionOutput(job, subtitleFile, transcriptFile, transcriptionOutput) {
				transcriptionOutput.SubtitlePath = subtitleFile
				transcriptionOutput.TranscriptPath = transcriptFile
				if cb.OnStepReused != nil {
					cb.OnStepReused(core.StepTranscription)
				}
				if cb.OnLog != nil {
					cb.OnLog("[transcription] Mode completer: transcription existante detectee, etape ignoree.\n")
				}
			} else {
				transcriptionOutput, err = r.transcribe(ctx, artifact.MediaPath, workspace, job, cb)
				if err != nil {
					return core.JobResult{}, err
				}
				subtitleFile = transcriptionOutput.SubtitlePath
				transcriptFile = transcriptionOutput.TranscriptPath
			}
		} else {
			transcriptionOutput, err = r.transcribe(ctx, artifact.MediaPath, workspace, job, cb)
			if err != nil {
				return core.JobResult{}, err
			}
			subtitleFile = transcriptionOutput.SubtitlePath
			transcriptFile = transcriptionOutput.TranscriptPath
		}

		subtitleForTranslation := subtitleFile
		transcriptForTranslation := transcriptFile
		if reusingExistingOutput && job.EnableTranslation {
			if strings.TrimSpace(subtitleForTranslation) != "" {
				if translatedSubtitle := firstExistingFile(sidecarPathForMedia(artifact.MediaPath, targetTag, ".srt")); translatedSubtitle != "" {
					subtitleFile = translatedSubtitle
					subtitleForTranslation = ""
				}
			}
			if strings.TrimSpace(transcriptForTranslation) != "" {
				if translatedTranscript := firstExistingFile(sidecarPathForMedia(artifact.MediaPath, targetTag, ".txt")); translatedTranscript != "" {
					transcriptFile = translatedTranscript
					transcriptForTranslation = ""
				}
			}
			if strings.TrimSpace(subtitleForTranslation) == "" && strings.TrimSpace(transcriptForTranslation) == "" {
				if cb.OnTranslationReused != nil {
					cb.OnTranslationReused()
				}
				if cb.OnLog != nil {
					cb.OnLog("[translation] Mode completer: traductions deja presentes, etape ignoree.\n")
				}
			}
		}

		if shouldTranslate(job, subtitleForTranslation, transcriptForTranslation) {
			translatedSubtitleFile, translatedTranscriptFile, translateErr := r.translateTranscription(
				ctx,
				subtitleForTranslation,
				transcriptForTranslation,
				workspace,
				job,
				cb,
			)
			if translateErr != nil {
				return core.JobResult{}, translateErr
			}
			translationArtifacts.SourceLanguage = sourceLanguage
			translationArtifacts.TargetLanguage = targetLanguage
			if strings.TrimSpace(subtitleForTranslation) != "" {
				subtitleFile = translatedSubtitleFile
				translationArtifacts.OriginalSubtitlePath = subtitleForTranslation
				translationArtifacts.TranslatedSubtitlePath = translatedSubtitleFile
			}
			if strings.TrimSpace(transcriptForTranslation) != "" {
				transcriptFile = translatedTranscriptFile
				translationArtifacts.OriginalTranscriptPath = transcriptForTranslation
				translationArtifacts.TranslatedTranscriptPath = translatedTranscriptFile
			}
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(1)
		}
	} else {
		if cb.OnLog != nil {
			cb.OnLog("[transcription] Etape ignoree (desactivee).\n")
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(1)
		}
	}

	if cb.OnStep != nil {
		cb.OnStep(core.StepMuxing)
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0)
	}
	if job.SourceKind == core.SourceYouTube && job.ContentType == core.ContentVideo && subtitleFile != "" {
		if reusingExistingOutput {
			if cb.OnStepReused != nil {
				cb.OnStepReused(core.StepMuxing)
			}
			if cb.OnLog != nil {
				cb.OnLog("[muxing] Mode completer: media deja present, remux ignore.\n")
			}
		} else {
			muxed, muxErr := r.muxSubtitles(ctx, artifact.MediaPath, subtitleFile, workspace, job, translationArtifacts, cb)
			if muxErr != nil {
				return core.JobResult{}, muxErr
			}
			artifact.MediaPath = muxed
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(1)
		}
	} else {
		if cb.OnLog != nil {
			if job.SourceKind == core.SourceYouTube && job.ContentType == core.ContentVideo && !shouldTranscribe(job) {
				cb.OnLog("[muxing] Etape ignoree (transcription desactivee).\n")
			} else {
				cb.OnLog("[muxing] Etape ignoree (non applicable).\n")
			}
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(1)
		}
	}

	if cb.OnStep != nil {
		cb.OnStep(core.StepOrganization)
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0)
	}

	result, err := r.organizer.Organize(OrganizationPayload{
		SourceKind:                job.SourceKind,
		SourceName:                artifact.SourceName,
		Title:                     artifact.Title,
		PublicationDate:           artifact.PublicationDate,
		OriginalInputURL:          job.InputURL,
		MediaPath:                 artifact.MediaPath,
		IsMediaDirectory:          artifact.IsDirectory,
		SubtitleFile:              subtitleFile,
		TranscriptFile:            transcriptFile,
		JSONFile:                  transcriptionOutput.JSONPath,
		TinydiarizeJSONFile:       transcriptionOutput.TinydiarizeJSONPath,
		TinydiarizeTranscriptFile: transcriptionOutput.TinydiarizeTranscriptPath,
		TinydiarizeSubtitleFile:   transcriptionOutput.TinydiarizeSubtitlePath,
		PyannoteJSONFile:          transcriptionOutput.PyannoteJSONPath,
		PyannoteTranscriptFile:    transcriptionOutput.PyannoteTranscriptPath,
		PyannoteSubtitleFile:      transcriptionOutput.PyannoteSubtitlePath,
		ArtworkFile:               artifact.ArtworkPath,
		CustomName:                job.CustomName,
		OutputRoot:                outputRoot,
		TranscriptionLanguage:     job.TranscriptionLanguage,
	}, opt.StandardCollision)
	if err != nil {
		return core.JobResult{}, err
	}
	if err := r.preserveTranslationVariants(result, translationArtifacts, cb); err != nil {
		return core.JobResult{}, err
	}

	if cb.OnStepProgress != nil {
		cb.OnStepProgress(1)
	}

	completed = true
	return result, nil
}

func (r *Runner) download(ctx context.Context, job core.JobRequest, outputRoot, workspace string, cb RunCallbacks) (downloadArtifact, error) {
	switch job.SourceKind {
	case core.SourceYouTube:
		return r.downloadWithYtDlp(ctx, job.InputURL, workspace, job.ContentType, job.UseFirefoxCookies, job.YtDlpExtraArguments, job.YtDlpEmbedMetadata, job.YtDlpEmbedThumbnail, job.YouTubeAudioFormat, job.YouTubeAudioPreferences, "", "", nil, cb)
	case core.SourceRSS:
		if job.SelectedRSSEpisode == nil {
			return downloadArtifact{}, fmt.Errorf("aucun episode RSS selectionne")
		}
		return r.downloadRSS(ctx, *job.SelectedRSSEpisode, workspace, job, cb)
	case core.SourceQobuz:
		return r.downloadQobuzAlbum(ctx, job, outputRoot, workspace, cb)
	default:
		return downloadArtifact{}, fmt.Errorf("source non supportee")
	}
}

func (r *Runner) downloadRSS(ctx context.Context, selection core.RSSEpisodeSelection, workspace string, job core.JobRequest, cb RunCallbacks) (downloadArtifact, error) {
	mediaURL := strings.TrimSpace(selection.MediaURL)
	if mediaURL == "" {
		return downloadArtifact{}, fmt.Errorf("URL media RSS invalide")
	}
	u, err := url.Parse(mediaURL)
	if err != nil {
		return downloadArtifact{}, fmt.Errorf("URL media RSS invalide")
	}
	artifact := downloadArtifact{}
	if isDirectAudioURL(u) {
		if cb.OnLog != nil {
			cb.OnLog("[rss] Telechargement direct via HTTP: " + u.String() + "\n")
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(0.1)
		}
		tmp, ext, err := r.downloadToFile(ctx, u, workspace, util.SanitizePathComponent(selection.Title, 100))
		if err != nil {
			return downloadArtifact{}, err
		}
		if ext == "" {
			ext = "mp3"
		}
		final := filepath.Join(workspace, util.SanitizePathComponent(selection.Title, 100)+"."+ext)
		_ = os.Remove(final)
		if err := os.Rename(tmp, final); err != nil {
			return downloadArtifact{}, err
		}
		artifact = downloadArtifact{
			MediaPath:       final,
			Title:           selection.Title,
			SourceName:      selection.PodcastTitle,
			PublicationDate: selection.PublicationDate,
			IsDirectory:     false,
		}
	} else {
		if cb.OnLog != nil {
			cb.OnLog("[rss] URL non directe, fallback via yt-dlp\n")
		}
		artifact, err = r.downloadWithYtDlp(ctx, selection.MediaURL, workspace, core.ContentAudio, job.UseFirefoxCookies, job.YtDlpExtraArguments, job.YtDlpEmbedMetadata, job.YtDlpEmbedThumbnail, job.YouTubeAudioFormat, job.YouTubeAudioPreferences, selection.PodcastTitle, selection.Title, selection.PublicationDate, cb)
		if err != nil {
			return downloadArtifact{}, err
		}
	}

	if strings.TrimSpace(selection.ArtworkURL) != "" {
		if art, err := r.downloadArtwork(ctx, selection.ArtworkURL, workspace, selection.Title, cb); err == nil {
			artifact.ArtworkPath = art
			if media, err := r.embedArtwork(ctx, artifact.MediaPath, art, workspace, cb); err == nil {
				artifact.MediaPath = media
			}
		}
	}

	if cb.OnStepProgress != nil {
		cb.OnStepProgress(1)
	}
	return artifact, nil
}

func buildYtDlpBaseArgs(workspace string, mode core.JobContentType, audioFormat string, embedMetadata, embedThumbnail bool) []string {
	return buildYtDlpBaseArgsForAudioPreference(workspace, mode, ytDlpAudioPreference{Mode: "convert", Format: resolvedYtDlpConversionFormat(audioFormat)}, embedMetadata, embedThumbnail)
}

type ytDlpAudioPreference struct {
	Mode   string
	Format string
}

func buildYtDlpBaseArgsForAudioPreference(workspace string, mode core.JobContentType, preference ytDlpAudioPreference, embedMetadata, embedThumbnail bool) []string {
	baseArgs := []string{
		"--no-playlist",
		"--newline",
		"--write-info-json",
		"--print", "after_move:filepath",
		"-o", filepath.Join(workspace, "%(title)s [%(id)s].%(ext)s"),
	}
	if mode == core.ContentVideo {
		baseArgs = append(baseArgs, "-f", "bv*+ba/b", "--merge-output-format", "mkv")
	} else {
		switch preference.Mode {
		case "native":
			baseArgs = append(baseArgs, "-f", ytDlpNativeFormatSelector(preference.Format))
		default:
			baseArgs = append(baseArgs, "-f", "bestaudio/b", "--extract-audio", "--audio-format", resolvedYtDlpConversionFormat(preference.Format))
		}
	}
	if embedMetadata {
		baseArgs = append(baseArgs, "--embed-metadata")
	}
	if embedThumbnail && ytDlpThumbnailEmbeddingSupported(mode, ytDlpAudioPreferenceOutputFormat(preference)) {
		baseArgs = append(baseArgs, "--write-thumbnail", "--convert-thumbnails", "jpg", "--embed-thumbnail")
	}
	return baseArgs
}

func resolvedYtDlpConversionFormat(audioFormat string) string {
	switch strings.ToLower(strings.TrimSpace(audioFormat)) {
	case "m4a", "opus", "flac", "wav", "aac":
		return strings.ToLower(strings.TrimSpace(audioFormat))
	default:
		return "mp3"
	}
}

func ytDlpNativeFormatSelector(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "m4a":
		return "bestaudio[ext=m4a]"
	case "webm":
		return "bestaudio[ext=webm]"
	default:
		return "bestaudio/b"
	}
}

func ytDlpAudioPreferenceOutputFormat(preference ytDlpAudioPreference) string {
	if preference.Mode == "native" {
		return strings.ToLower(strings.TrimSpace(preference.Format))
	}
	return resolvedYtDlpConversionFormat(preference.Format)
}

func resolveYtDlpAudioPreferences(audioFormat string, raw []string) []ytDlpAudioPreference {
	fallback := resolvedYtDlpConversionFormat(audioFormat)
	if len(raw) == 0 {
		return []ytDlpAudioPreference{{Mode: "convert", Format: fallback}}
	}
	seen := map[string]bool{}
	out := make([]ytDlpAudioPreference, 0, len(raw))
	for _, item := range raw {
		preference, ok := parseYtDlpAudioPreference(item, fallback)
		if !ok {
			continue
		}
		key := preference.Mode + ":" + preference.Format
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, preference)
	}
	if len(out) == 0 {
		out = append(out, ytDlpAudioPreference{Mode: "convert", Format: fallback})
	}
	return out
}

func parseYtDlpAudioPreference(raw, fallbackFormat string) (ytDlpAudioPreference, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "default" {
		return ytDlpAudioPreference{Mode: "convert", Format: fallbackFormat}, true
	}
	if strings.Contains(value, ":") {
		parts := strings.SplitN(value, ":", 2)
		mode := strings.TrimSpace(parts[0])
		format := strings.TrimSpace(parts[1])
		switch mode {
		case "native":
			switch format {
			case "m4a", "webm", "best":
				return ytDlpAudioPreference{Mode: mode, Format: format}, true
			}
		case "convert":
			return ytDlpAudioPreference{Mode: mode, Format: resolvedYtDlpConversionFormat(format)}, true
		}
		return ytDlpAudioPreference{}, false
	}
	return ytDlpAudioPreference{Mode: "convert", Format: resolvedYtDlpConversionFormat(value)}, true
}

func ytDlpAudioPreferenceLabel(preference ytDlpAudioPreference) string {
	format := strings.ToUpper(strings.TrimSpace(preference.Format))
	if preference.Mode == "native" {
		if preference.Format == "best" {
			return "meilleur audio natif"
		}
		return format + " natif"
	}
	return format + " converti"
}

func ytDlpThumbnailEmbeddingSupported(mode core.JobContentType, audioFormat string) bool {
	if mode == core.ContentVideo {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(audioFormat)) {
	case "mp3", "m4a", "opus", "flac":
		return true
	default:
		return false
	}
}

func (r *Runner) downloadWithYtDlp(
	ctx context.Context,
	sourceURL, workspace string,
	mode core.JobContentType,
	useFirefoxCookies bool,
	extraArguments string,
	embedMetadata, embedThumbnail bool,
	audioFormat string,
	audioPreferences []string,
	forcedSourceName, forcedTitle string,
	forcedDate *time.Time,
	cb RunCallbacks,
) (downloadArtifact, error) {
	extraArgsList := util.ParseArgumentString(extraArguments)
	cookiesConfiguredInArgs := hasYtDlpCookieArgs(extraArgsList)
	preferences := []ytDlpAudioPreference{{Mode: "convert", Format: resolvedYtDlpConversionFormat(audioFormat)}}
	if mode != core.ContentVideo {
		preferences = resolveYtDlpAudioPreferences(audioFormat, audioPreferences)
	}
	if cb.OnLog != nil {
		cb.OnLog("[download] Demarrage du telechargement YouTube...\n")
		if mode != core.ContentVideo {
			labels := make([]string, 0, len(preferences))
			for _, preference := range preferences {
				labels = append(labels, ytDlpAudioPreferenceLabel(preference))
			}
			cb.OnLog("[download] Priorites audio: " + strings.Join(labels, " > ") + "\n")
		}
		if embedMetadata {
			cb.OnLog("[download] Integration des metadonnees yt-dlp activee.\n")
		}
	}

	output := ""
	var err error
	for idx, preference := range preferences {
		baseArgs := buildYtDlpBaseArgsForAudioPreference(workspace, mode, preference, embedMetadata, embedThumbnail)
		if cb.OnLog != nil && mode != core.ContentVideo {
			label := ytDlpAudioPreferenceLabel(preference)
			cb.OnLog("[download] Essai audio " + strconv.Itoa(idx+1) + "/" + strconv.Itoa(len(preferences)) + ": " + label + "\n")
			outputFormat := ytDlpAudioPreferenceOutputFormat(preference)
			if embedThumbnail {
				if ytDlpThumbnailEmbeddingSupported(mode, outputFormat) {
					cb.OnLog("[download] Integration de la miniature yt-dlp activee pour " + label + ".\n")
				} else {
					cb.OnLog("[download] Integration de la miniature ignoree pour " + label + " (support lecteur/conteneur limite).\n")
				}
			}
		}
		output, err = r.runYtDlpDownloadWithCookieRetry(ctx, workspace, baseArgs, extraArgsList, sourceURL, useFirefoxCookies, cookiesConfiguredInArgs, cb)
		if err == nil {
			break
		}
		if idx+1 < len(preferences) && shouldTryNextYtDlpAudioPreference(err, output, preference) {
			if cb.OnLog != nil {
				cb.OnLog("[download] Priorite audio indisponible ou conversion echouee, essai de la priorite suivante.\n")
			}
			continue
		}
		if looksLikeYouTubeBotCheckError(err, output) {
			return downloadArtifact{}, buildYouTubeBotCheckError(err, output, useFirefoxCookies, cookiesConfiguredInArgs)
		}
		return downloadArtifact{}, err
	}
	if cb.OnLog != nil {
		cb.OnLog("[download] Telechargement termine, finalisation...\n")
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0.99)
	}

	var printedPaths []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/") {
			if _, err := os.Stat(line); err == nil {
				printedPaths = append(printedPaths, line)
			}
		}
	}

	media := ""
	if len(printedPaths) > 0 {
		media = printedPaths[len(printedPaths)-1]
	}
	if media == "" {
		media = discoverDownloadedMedia(workspace)
	}
	if media == "" {
		return downloadArtifact{}, fmt.Errorf("telechargement termine mais fichier media introuvable")
	}

	infoPath := discoverInfoJSON(workspace)
	parsed := parseYtInfo(infoPath)
	title := forcedTitle
	if strings.TrimSpace(title) == "" {
		title = parsed.title
	}
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSuffix(filepath.Base(media), filepath.Ext(media))
	}
	source := forcedSourceName
	if strings.TrimSpace(source) == "" {
		source = parsed.sourceName
	}
	if strings.TrimSpace(source) == "" {
		source = "Source inconnue"
	}
	pub := forcedDate
	if pub == nil {
		pub = parsed.date
	}

	return downloadArtifact{MediaPath: media, Title: title, SourceName: source, PublicationDate: pub, IsDirectory: false}, nil
}

func (r *Runner) runYtDlpDownload(ctx context.Context, workspace string, args []string, cb RunCallbacks) (string, error) {
	ytDlpExec, _, err := util.ResolveToolExecutable("yt-dlp")
	if err != nil {
		return "", err
	}
	return r.processRunner.Run(ctx, sys.RunOptions{
		Executable: ytDlpExec,
		Args:       args,
		WorkingDir: workspace,
		OnOutput: func(line string) {
			if cb.OnLog != nil {
				cb.OnLog(line)
			}
			if cb.OnStepProgress != nil {
				if pct := parsePercentProgress(line); pct >= 0 {
					cb.OnStepProgress(minFloat(1, pct/100.0))
				}
			}
		},
		CaptureOutput: true,
	})
}

func (r *Runner) runYtDlpDownloadWithCookieRetry(
	ctx context.Context,
	workspace string,
	baseArgs []string,
	extraArgsList []string,
	sourceURL string,
	useFirefoxCookies bool,
	cookiesConfiguredInArgs bool,
	cb RunCallbacks,
) (string, error) {
	args := append([]string{}, baseArgs...)
	if useFirefoxCookies {
		args = append(args, "--cookies-from-browser", "firefox")
	}
	args = append(args, extraArgsList...)
	args = append(args, "--no-quiet", "--progress", "--newline", sourceURL)
	output, err := r.runYtDlpDownload(ctx, workspace, args, cb)
	lastYouTubeBotErr := error(nil)
	if looksLikeYouTubeBotCheckError(err, output) {
		lastYouTubeBotErr = err
	}
	if shouldRetryWithBrowserCookies(err, output, useFirefoxCookies, cookiesConfiguredInArgs) {
		if cb.OnLog != nil {
			cb.OnLog("[download] Verification anti-bot detectee. Tentatives automatiques avec cookies navigateur...\n")
		}
		hadExtractedCookies := false
		triedCookieSpecs := []string{}
	retryLoop:
		for _, browser := range preferredYtDlpCookieBrowsers() {
			if browser != "firefox" && !browserCookieStoreLikelyAvailable(browser) {
				if cb.OnLog != nil {
					cb.OnLog("[download] Navigateur " + browser + " ignore (profil/cookies introuvables).\n")
				}
				continue
			}
			for _, cookieSpec := range ytDlpCookieSpecsForBrowser(browser) {
				triedCookieSpecs = append(triedCookieSpecs, cookieSpec)
				if cb.OnLog != nil {
					cb.OnLog("[download] Nouvelle tentative avec --cookies-from-browser " + cookieSpec + "\n")
				}
				retryArgs := append([]string{}, baseArgs...)
				retryArgs = append(retryArgs, "--cookies-from-browser", cookieSpec)
				retryArgs = append(retryArgs, extraArgsList...)
				retryArgs = append(retryArgs, "--no-quiet", "--progress", "--newline", sourceURL)
				retryOutput, retryErr := r.runYtDlpDownload(ctx, workspace, retryArgs, cb)
				output = retryOutput
				err = retryErr
				if looksLikeCookiesWereExtracted(output) {
					hadExtractedCookies = true
				}
				if looksLikeYouTubeBotCheckError(err, output) {
					lastYouTubeBotErr = err
				}
				if err == nil {
					break retryLoop
				}
				if !looksLikeYouTubeBotCheckError(err, output) && !looksLikeYtDlpCookieSetupError(err, output) {
					break retryLoop
				}
			}
		}
		if err != nil && lastYouTubeBotErr != nil && (looksLikeYouTubeBotCheckError(err, output) || looksLikeYtDlpCookieSetupError(err, output)) {
			attemptDetails := ""
			if len(triedCookieSpecs) > 0 {
				attemptDetails = " Tentatives cookies: " + strings.Join(triedCookieSpecs, ", ") + "."
			}
			adapted := buildYouTubeBotCheckError(lastYouTubeBotErr, output, useFirefoxCookies || hadExtractedCookies, cookiesConfiguredInArgs || hadExtractedCookies)
			if attemptDetails != "" {
				return output, fmt.Errorf("%w.%s", adapted, attemptDetails)
			}
			return output, adapted
		}
	}
	return output, err
}

func shouldTryNextYtDlpAudioPreference(err error, output string, preference ytDlpAudioPreference) bool {
	if err == nil || looksLikeYouTubeBotCheckError(err, output) {
		return false
	}
	if looksLikeYtDlpFormatUnavailable(err, output) {
		return true
	}
	return preference.Mode == "convert" && looksLikeYtDlpPostprocessingError(err, output)
}

func looksLikeYtDlpFormatUnavailable(err error, output string) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(output + "\n" + err.Error()))
	return strings.Contains(text, "requested format is not available") ||
		strings.Contains(text, "no video formats found") ||
		strings.Contains(text, "no such format")
}

func looksLikeYtDlpPostprocessingError(err error, output string) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(output + "\n" + err.Error()))
	return strings.Contains(text, "postprocess") ||
		strings.Contains(text, "post-process") ||
		strings.Contains(text, "ffmpeg") ||
		strings.Contains(text, "conversion failed") ||
		strings.Contains(text, "failed to convert")
}

func shouldRetryWithBrowserCookies(err error, output string, useFirefoxCookies bool, cookiesConfiguredInArgs bool) bool {
	if err == nil {
		return false
	}
	if useFirefoxCookies || cookiesConfiguredInArgs {
		return false
	}
	return looksLikeYouTubeBotCheckError(err, output)
}

func hasYtDlpCookieArgs(args []string) bool {
	for _, arg := range args {
		v := strings.TrimSpace(strings.ToLower(arg))
		if v == "--cookies" || v == "--cookies-from-browser" {
			return true
		}
		if strings.HasPrefix(v, "--cookies=") || strings.HasPrefix(v, "--cookies-from-browser=") {
			return true
		}
	}
	return false
}

func looksLikeYouTubeBotCheckError(err error, output string) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(output + "\n" + err.Error()))
	if text == "" {
		return false
	}
	return strings.Contains(text, "sign in to confirm you're not a bot") ||
		strings.Contains(text, "sign in to confirm you’re not a bot") ||
		strings.Contains(text, "use --cookies-from-browser or --cookies")
}

func buildYouTubeBotCheckError(baseErr error, output string, useFirefoxCookies bool, cookiesConfiguredInArgs bool) error {
	if baseErr == nil {
		return nil
	}
	usesCookies := useFirefoxCookies || cookiesConfiguredInArgs || looksLikeCookiesWereExtracted(output)
	if usesCookies {
		return fmt.Errorf(
			"%w. Des cookies navigateur ont ete fournis, mais YouTube bloque encore l'acces (verification anti-bot). Ouvre YouTube dans ce meme profil navigateur, valide le challenge anti-bot, puis relance. Tu peux aussi exporter un cookies.txt et l'utiliser via --cookies <fichier>. Conseil: essaie de changer d'IP via un partage de connexion mobile ou un VPN",
			baseErr,
		)
	}
	return fmt.Errorf(
		"%w. YouTube a demande une verification anti-bot. Active \"Utiliser cookies Firefox\" dans Reglages ou ajoute --cookies-from-browser <navigateur[:profil]> (ou --cookies <fichier>) dans les arguments yt-dlp. Conseil: essaie de changer d'IP via un partage de connexion mobile ou un VPN",
		baseErr,
	)
}

func looksLikeYtDlpCookieSetupError(err error, output string) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(output + "\n" + err.Error()))
	if text == "" {
		return false
	}
	return strings.Contains(text, "cookies-from-browser") ||
		strings.Contains(text, "cookie") && strings.Contains(text, "browser")
}

func preferredYtDlpCookieBrowsers() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"firefox", "chrome", "brave", "chromium", "edge"}
	case "windows":
		return []string{"firefox", "chrome", "edge", "brave", "chromium"}
	default:
		return []string{"firefox", "chrome", "chromium", "brave", "edge"}
	}
}

func ytDlpCookieSpecsForBrowser(browser string) []string {
	browser = strings.ToLower(strings.TrimSpace(browser))
	if browser == "" {
		return nil
	}
	if browser != "firefox" {
		return []string{browser}
	}
	specs := []string{"firefox"}
	for _, profile := range discoverFirefoxProfileNames() {
		profile = strings.TrimSpace(profile)
		if profile == "" {
			continue
		}
		specs = append(specs, "firefox:"+profile)
	}
	return dedupeStrings(specs)
}

func discoverFirefoxProfileNames() []string {
	root := firefoxProfilesRootDir()
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	profiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		profiles = append(profiles, name)
	}
	sort.SliceStable(profiles, func(i, j int) bool {
		li := strings.ToLower(profiles[i])
		lj := strings.ToLower(profiles[j])
		pi := 2
		pj := 2
		if strings.Contains(li, "default-release") {
			pi = 0
		} else if strings.Contains(li, "default") {
			pi = 1
		}
		if strings.Contains(lj, "default-release") {
			pj = 0
		} else if strings.Contains(lj, "default") {
			pj = 1
		}
		if pi != pj {
			return pi < pj
		}
		return li < lj
	})
	if len(profiles) > 5 {
		profiles = profiles[:5]
	}
	return profiles
}

func firefoxProfilesRootDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles")
	case "windows":
		appData := strings.TrimSpace(os.Getenv("APPDATA"))
		if appData != "" {
			return filepath.Join(appData, "Mozilla", "Firefox", "Profiles")
		}
		return filepath.Join(home, "AppData", "Roaming", "Mozilla", "Firefox", "Profiles")
	default:
		return filepath.Join(home, ".mozilla", "firefox")
	}
}

func browserCookieStoreLikelyAvailable(browser string) bool {
	browser = strings.ToLower(strings.TrimSpace(browser))
	if browser == "" || browser == "firefox" {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}
	var root string
	switch runtime.GOOS {
	case "darwin":
		switch browser {
		case "chrome":
			root = filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
		case "brave":
			root = filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser")
		case "chromium":
			root = filepath.Join(home, "Library", "Application Support", "Chromium")
		case "edge":
			root = filepath.Join(home, "Library", "Application Support", "Microsoft Edge")
		}
	case "windows":
		local := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		switch browser {
		case "chrome":
			root = filepath.Join(local, "Google", "Chrome", "User Data")
		case "brave":
			root = filepath.Join(local, "BraveSoftware", "Brave-Browser", "User Data")
		case "chromium":
			root = filepath.Join(local, "Chromium", "User Data")
		case "edge":
			root = filepath.Join(local, "Microsoft", "Edge", "User Data")
		}
	default:
		switch browser {
		case "chrome":
			root = filepath.Join(home, ".config", "google-chrome")
		case "brave":
			root = filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser")
		case "chromium":
			root = filepath.Join(home, ".config", "chromium")
		case "edge":
			root = filepath.Join(home, ".config", "microsoft-edge")
		}
	}
	if root == "" {
		return true
	}
	info, statErr := os.Stat(root)
	return statErr == nil && info.IsDir()
}

func looksLikeCookiesWereExtracted(output string) bool {
	text := strings.ToLower(strings.TrimSpace(output))
	if text == "" {
		return false
	}
	return strings.Contains(text, "extracting cookies from") && strings.Contains(text, "extracted") && strings.Contains(text, "cookies from")
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func (r *Runner) downloadQobuzAlbum(ctx context.Context, job core.JobRequest, outputRoot, workspace string, cb RunCallbacks) (downloadArtifact, error) {
	rt, ok := util.QobuzResourceTypeFromURL(job.InputURL)
	if !ok {
		return downloadArtifact{}, fmt.Errorf("URL Qobuz invalide ou non supportee")
	}
	if rt == util.QobuzArtist {
		return downloadArtifact{}, fmt.Errorf("les URL artiste Qobuz necessitent de selectionner des albums dans l'ecran Nouveau job")
	}
	if rt != util.QobuzAlbum && rt != util.QobuzPlaylist {
		return downloadArtifact{}, fmt.Errorf("URL Qobuz invalide ou non supportee")
	}
	if err := r.ensureQobuzConfigured(ctx, job.QobuzEmail, job.QobuzPassword, job.QobuzUserAuthToken, job.QobuzUseUserAuthToken, cb); err != nil {
		return downloadArtifact{}, err
	}
	if rt == util.QobuzPlaylist {
		return r.downloadQobuzPlaylistResource(ctx, job, outputRoot, workspace, cb)
	}
	return r.downloadQobuzAlbumResource(ctx, job, outputRoot, workspace, cb)
}

func (r *Runner) downloadQobuzAlbumResource(ctx context.Context, job core.JobRequest, outputRoot, workspace string, cb RunCallbacks) (downloadArtifact, error) {
	artistOverride := strings.TrimSpace(job.QobuzArtistName)
	downloadRoot := filepath.Join(outputRoot, "qobuz", util.SanitizePathComponent(defaultIfEmpty(artistOverride, "Artiste inconnu"), 120))
	if err := os.MkdirAll(downloadRoot, 0o755); err != nil {
		return downloadArtifact{}, err
	}
	if cb.OnLog != nil {
		cb.OnLog("[qobuz] Telechargement direct dans: " + downloadRoot + "\n")
	}

	args := []string{"dl", "-q", "27", "--embed-art", "--og-cover", "--no-db", "-d", downloadRoot}
	args = append(args, util.ParseArgumentString(job.QobuzExtraArguments)...)
	args = append(args, job.InputURL)

	runResult, err := r.runQobuzDownloadCommand(ctx, args, workspace, job.QobuzEmail, job.QobuzPassword, job.QobuzUserAuthToken, job.QobuzUseUserAuthToken, cb)
	if err != nil {
		return downloadArtifact{}, err
	}

	albumDir := strings.TrimSpace(runResult.Directory)
	if albumDir == "" {
		albumDir = discoverQobuzDirectoryByDownloadLabel(downloadRoot, runResult.Label)
	}
	if albumDir == "" {
		albumDir = discoverLatestDirectory(downloadRoot)
	}
	if albumDir == "" {
		albumDir = r.findExistingQobuzAlbumDirectory(job.InputURL, outputRoot)
	}
	if albumDir == "" {
		return downloadArtifact{}, fmt.Errorf("telechargement Qobuz termine mais dossier d'album introuvable")
	}
	if cb.OnLog != nil {
		cb.OnLog("[qobuz] Dossier album detecte: " + albumDir + "\n")
	}
	meta := r.readQobuzFolderMetadata(albumDir, artistOverride)
	return downloadArtifact{MediaPath: albumDir, Title: meta.albumTitle, SourceName: meta.artistName, IsDirectory: true}, nil
}

func (r *Runner) downloadQobuzPlaylistResource(ctx context.Context, job core.JobRequest, outputRoot, workspace string, cb RunCallbacks) (downloadArtifact, error) {
	playlistOverride := strings.TrimSpace(job.QobuzPlaylistName)
	downloadRoot := filepath.Join(outputRoot, "qobuz", "Playlists")
	if err := os.MkdirAll(downloadRoot, 0o755); err != nil {
		return downloadArtifact{}, err
	}
	if cb.OnLog != nil {
		cb.OnLog("[qobuz] Telechargement playlist dans: " + downloadRoot + "\n")
	}

	args := []string{"dl", "-q", "27", "--embed-art", "--og-cover", "--no-db", "-d", downloadRoot}
	args = append(args, util.ParseArgumentString(job.QobuzExtraArguments)...)
	args = append(args, job.InputURL)
	runResult, err := r.runQobuzDownloadCommand(ctx, args, workspace, job.QobuzEmail, job.QobuzPassword, job.QobuzUserAuthToken, job.QobuzUseUserAuthToken, cb)
	if err != nil {
		return downloadArtifact{}, err
	}

	playlistDir := strings.TrimSpace(runResult.Directory)
	if playlistDir == "" {
		playlistDir = discoverQobuzDirectoryByDownloadLabel(downloadRoot, runResult.Label)
	}
	if playlistDir == "" {
		playlistDir = discoverLatestDirectory(downloadRoot)
	}
	if playlistDir == "" {
		playlistDir = r.findExistingQobuzAlbumDirectory(job.InputURL, outputRoot)
	}
	if playlistDir == "" {
		return downloadArtifact{}, fmt.Errorf("telechargement Qobuz termine mais dossier de playlist introuvable")
	}

	if playlistOverride != "" {
		sanitized := util.SanitizePathComponent(playlistOverride, 140)
		if sanitized != "" {
			targetDir := filepath.Join(downloadRoot, sanitized)
			if !samePath(playlistDir, targetDir) {
				if _, err := os.Stat(targetDir); err == nil {
					targetDir = filepath.Join(downloadRoot, uniqueDirectoryName(downloadRoot, sanitized))
				}
				if err := moveReplacing(playlistDir, targetDir); err == nil {
					playlistDir = targetDir
				} else if cb.OnLog != nil {
					cb.OnLog("[qobuz] Renommage dossier playlist ignore: " + err.Error() + "\n")
				}
			}
		}
	}

	title := strings.TrimSpace(playlistOverride)
	if title == "" {
		title = strings.TrimSpace(filepath.Base(playlistDir))
	}
	if title == "" {
		title = "Playlist"
	}
	if cb.OnLog != nil {
		cb.OnLog("[qobuz] Dossier playlist detecte: " + playlistDir + "\n")
	}
	return downloadArtifact{MediaPath: playlistDir, Title: title, SourceName: "Playlists", IsDirectory: true}, nil
}

type qobuzDownloadRunResult struct {
	Label     string
	Directory string
}

func (r *Runner) runQobuzDownloadCommand(ctx context.Context, args []string, workspace, qobuzEmail, qobuzPassword, qobuzUserAuthToken string, useUserAuthToken bool, cb RunCallbacks) (qobuzDownloadRunResult, error) {
	qobuzExec := "qobuz-dl"
	if resolved, _, err := util.ResolveToolExecutable("qobuz-dl"); err == nil {
		qobuzExec = resolved
	}
	const maxRetryableDownloadAttempts = 3
	result := qobuzDownloadRunResult{}
	bestProgress := 0.0
	currentArgs := append([]string{}, args...)
	ogCoverFallbackUsed := false
	passwordMode := "raw"
	commandEnv, err := qobuzCommandEnvironment(workspace, qobuzEmail, qobuzPassword, qobuzUserAuthToken, passwordMode, useUserAuthToken)
	if err != nil {
		return qobuzDownloadRunResult{}, err
	}
	authFallbackUsed := false
	executable := qobuzExec
	buildCommandArgs := func(downloadArgs []string) ([]string, error) {
		wrapperScript := strings.TrimSpace(r.qobuzCLIWrapperScript)
		if wrapperScript != "" {
			if info, statErr := os.Stat(wrapperScript); statErr == nil && !info.IsDir() {
				python, buildErr := resolveQobuzPythonRuntimeForRunner()
				if buildErr == nil {
					executable = python.Exec
					return append(append(append([]string{}, python.PrefixArgs...), wrapperScript), downloadArgs...), nil
				}
				if useUserAuthToken {
					return nil, buildErr
				}
			}
		}
		if useUserAuthToken {
			python, buildErr := resolveQobuzPythonRuntimeForRunner()
			if buildErr != nil {
				return nil, buildErr
			}
			executable = python.Exec
			return append(append(append([]string{}, python.PrefixArgs...), r.qobuzCLIWrapperScript), downloadArgs...), nil
		}
		executable = qobuzExec
		return append([]string{}, downloadArgs...), nil
	}
	commandArgs, err := buildCommandArgs(currentArgs)
	if err != nil {
		return qobuzDownloadRunResult{}, err
	}

	retryableDownloadAttempts := 0
	for attempt := 1; ; attempt++ {
		attemptLabel := ""
		attemptDirectory := ""
		attemptOutputTail := ""

		_, err := r.processRunner.Run(ctx, sys.RunOptions{
			Executable:  executable,
			Args:        commandArgs,
			WorkingDir:  workspace,
			Environment: commandEnv,
			OnOutput: func(line string) {
				if attemptLabel == "" {
					attemptLabel = parseQobuzDownloadingLabel(line)
				}
				if attemptDirectory == "" {
					attemptDirectory = parseQobuzDirectoryFromProgressLine(line)
				}
				attemptOutputTail = appendOutputTail(attemptOutputTail, line, 32*1024)
				if cb.OnLog != nil {
					cb.OnLog(line)
				}
				if cb.OnStepProgress != nil {
					if pct := parsePercentProgress(line); pct >= 0 {
						progress := minFloat(0.95, pct/100.0)
						if progress > bestProgress {
							bestProgress = progress
							cb.OnStepProgress(progress)
						}
					}
				}
			},
			CaptureOutput: false,
		})
		if result.Label == "" && attemptLabel != "" {
			result.Label = attemptLabel
		}
		if result.Directory == "" && attemptDirectory != "" {
			result.Directory = attemptDirectory
		}

		if !authFallbackUsed && !useUserAuthToken && detectQobuzAuthenticationFailure(attemptOutputTail) && strings.TrimSpace(qobuzEmail) != "" && strings.TrimSpace(qobuzPassword) != "" && passwordMode == "raw" {
			if ctx.Err() != nil {
				return qobuzDownloadRunResult{}, ctx.Err()
			}
			commandEnv, err = qobuzCommandEnvironment(workspace, qobuzEmail, qobuzPassword, "", "md5", useUserAuthToken)
			if err != nil {
				return qobuzDownloadRunResult{}, err
			}
			passwordMode = "md5"
			authFallbackUsed = true
			if cb.OnLog != nil {
				cb.OnLog(fmt.Sprintf("[qobuz] Authentification refusee avec le mot de passe brut. Reprise automatique avec mot de passe MD5, tentative %d...\n", attempt+1))
			}
			continue
		}

		if !ogCoverFallbackUsed && qobuzArgsContainOGCover(currentArgs) && detectQobuzOGCoverTooLargeError(attemptOutputTail) {
			if ctx.Err() != nil {
				return qobuzDownloadRunResult{}, ctx.Err()
			}
			currentArgs = qobuzArgsWithoutOGCover(currentArgs)
			commandArgs, err = buildCommandArgs(currentArgs)
			if err != nil {
				return qobuzDownloadRunResult{}, err
			}
			ogCoverFallbackUsed = true
			if cb.OnLog != nil {
				cb.OnLog(fmt.Sprintf("[qobuz] Cover trop volumineuse pour l'embed detectee. Reprise automatique sans --og-cover, tentative %d...\n", attempt+1))
			}
			continue
		}

		if retryReason := detectQobuzRetryableDownloadError(attemptOutputTail); retryReason != "" {
			if ctx.Err() != nil {
				return qobuzDownloadRunResult{}, ctx.Err()
			}
			if retryableDownloadAttempts >= maxRetryableDownloadAttempts-1 {
				return qobuzDownloadRunResult{}, fmt.Errorf("telechargement Qobuz interrompu apres %d tentatives (%s)", retryableDownloadAttempts+1, retryReason)
			}
			retryableDownloadAttempts++
			if cb.OnLog != nil {
				cb.OnLog(fmt.Sprintf("[qobuz] Erreur transitoire detectee (%s). Reprise automatique du telechargement, tentative %d...\n", retryReason, attempt+1))
			}
			if err := waitForRetryDelay(ctx, 1500*time.Millisecond); err != nil {
				return qobuzDownloadRunResult{}, err
			}
			continue
		}

		if err != nil {
			return qobuzDownloadRunResult{}, err
		}
		if cb.OnStepProgress != nil && bestProgress < 0.99 {
			cb.OnStepProgress(0.99)
		}
		return result, nil
	}
}

func (r *Runner) ensureQobuzConfigured(ctx context.Context, email, password, userAuthToken string, useUserAuthToken bool, cb RunCallbacks) error {
	if useUserAuthToken {
		if strings.TrimSpace(userAuthToken) == "" {
			return fmt.Errorf("renseigne un token de session Qobuz pour activer le contournement")
		}
		if _, _, err := util.ResolveToolExecutable("qobuz-dl"); err != nil {
			return fmt.Errorf("qobuz-dl introuvable. Installe-le depuis Systeme > Diagnostics")
		}
		return nil
	}
	if existing := qobuzExistingConfigPath(); existing != "" {
		return nil
	}
	email = strings.TrimSpace(email)
	if email == "" || strings.TrimSpace(password) == "" {
		return fmt.Errorf("qobuz-dl n'est pas configure. Renseigne email/mot de passe Qobuz dans Reglages")
	}
	qobuzExec, _, err := util.ResolveToolExecutable("qobuz-dl")
	if err != nil {
		return fmt.Errorf("qobuz-dl introuvable. Installe-le depuis Systeme > Diagnostics")
	}
	if cb.OnLog != nil {
		cb.OnLog("[qobuz] Initialisation de la configuration qobuz-dl...\n")
	}
	stdin := email + "\n" + password + "\n\n27\n"
	_, err = r.processRunner.Run(ctx, sys.RunOptions{
		Executable:    qobuzExec,
		Args:          []string{"-r"},
		StandardInput: stdin,
		CaptureOutput: false,
		OnOutput: func(line string) {
			if cb.OnLog != nil {
				cb.OnLog(line)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("impossible d'initialiser la configuration qobuz-dl")
	}
	if existing := qobuzExistingConfigPath(); existing == "" {
		return fmt.Errorf("impossible d'initialiser la configuration qobuz-dl")
	}
	return nil
}

func qobuzConfigPath() string {
	if existing := qobuzExistingConfigPath(); existing != "" {
		return existing
	}
	candidates := qobuzConfigPathCandidates()
	if len(candidates) > 0 {
		return candidates[0]
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "qobuz-dl", "config.ini")
}

func qobuzCommandEnvironment(workspace, email, password, userAuthToken, passwordMode string, useUserAuthToken bool) (map[string]string, error) {
	if useUserAuthToken {
		trimmedToken := strings.TrimSpace(userAuthToken)
		if trimmedToken == "" {
			return nil, fmt.Errorf("renseigne un token de session Qobuz pour activer le contournement")
		}
		env := map[string]string{
			qobuzUserAuthTokenEnv: trimmedToken,
		}
		if trimmedEmail := strings.TrimSpace(email); trimmedEmail != "" {
			env[qobuzEmailEnv] = trimmedEmail
		}
		if runtime.GOOS == "windows" {
			appData := filepath.Join(workspace, "qobuz-appdata")
			if err := os.MkdirAll(appData, 0o755); err != nil {
				return nil, err
			}
			env["APPDATA"] = appData
			return env, nil
		}
		tempHome := filepath.Join(workspace, "qobuz-home")
		if err := os.MkdirAll(tempHome, 0o755); err != nil {
			return nil, err
		}
		env["HOME"] = tempHome
		return env, nil
	}

	email = strings.TrimSpace(email)
	if email == "" || strings.TrimSpace(password) == "" {
		return nil, nil
	}

	data, err := os.ReadFile(qobuzConfigPath())
	if err != nil {
		return nil, fmt.Errorf("impossible de lire la configuration qobuz-dl")
	}

	configData := overrideQobuzConfigCredentials(string(data), email, qobuzPasswordValueForMode(password, passwordMode))

	if runtime.GOOS == "windows" {
		appData := filepath.Join(workspace, "qobuz-appdata")
		configDir := filepath.Join(appData, "qobuz-dl")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(configDir, "config.ini"), []byte(configData), 0o600); err != nil {
			return nil, err
		}
		return map[string]string{"APPDATA": appData}, nil
	}

	tempHome := filepath.Join(workspace, "qobuz-home")
	configDir := filepath.Join(tempHome, ".config", "qobuz-dl")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.ini"), []byte(configData), 0o600); err != nil {
		return nil, err
	}
	return map[string]string{"HOME": tempHome}, nil
}

func overrideQobuzConfigCredentials(configData, email, passwordMD5 string) string {
	lines := strings.Split(configData, "\n")
	emailFound := false
	passwordFound := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "email"):
			lines[i] = "email = " + email
			emailFound = true
		case strings.HasPrefix(lower, "password"):
			lines[i] = "password = " + passwordMD5
			passwordFound = true
		}
	}

	if !emailFound {
		lines = append(lines, "email = "+email)
	}
	if !passwordFound {
		lines = append(lines, "password = "+passwordMD5)
	}
	return strings.Join(lines, "\n")
}

func qobuzPasswordValueForMode(password, mode string) string {
	if mode == "md5" {
		sum := md5.Sum([]byte(password))
		return hex.EncodeToString(sum[:])
	}
	return password
}

func qobuzExistingConfigPath() string {
	for _, candidate := range qobuzConfigPathCandidates() {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func qobuzConfigPathCandidates() []string {
	candidates := make([]string, 0, 3)
	if runtime.GOOS == "windows" {
		if cfg := strings.TrimSpace(os.Getenv("APPDATA")); cfg != "" {
			candidates = append(candidates, filepath.Join(cfg, "qobuz-dl", "config.ini"))
		}
	}
	if home, _ := os.UserHomeDir(); strings.TrimSpace(home) != "" {
		candidates = append(candidates, filepath.Join(home, ".config", "qobuz-dl", "config.ini"))
	}
	if len(candidates) == 0 {
		candidates = append(candidates, filepath.Join(".config", "qobuz-dl", "config.ini"))
	}
	return dedupeStrings(candidates)
}

type qobuzPythonCommandCandidate struct {
	Exec       string
	PrefixArgs []string
}

func resolveQobuzPythonRuntimeForRunner() (qobuzPythonCommandCandidate, error) {
	for _, candidate := range qobuzPythonProbeCandidatesForRunner() {
		if qobuzPythonCandidateSupportsModuleForRunner(candidate) {
			return candidate, nil
		}
	}
	return qobuzPythonCommandCandidate{}, fmt.Errorf("impossible de trouver le runtime Python de qobuz-dl")
}

func qobuzPythonProbeCandidatesForRunner() []qobuzPythonCommandCandidate {
	candidates := make([]qobuzPythonCommandCandidate, 0, 16)
	if qobuzPath, _, err := util.ResolveToolExecutable("qobuz-dl"); err == nil {
		for _, probeFile := range qobuzRuntimeProbeFilesForRunner(qobuzPath) {
			if resolved := qobuzPythonFromShebangForRunner(probeFile); strings.TrimSpace(resolved) != "" {
				candidates = append(candidates, qobuzPythonCommandCandidate{Exec: resolved})
			}
		}
	}
	for _, candidate := range qobuzFallbackPythonCandidatesForRunner() {
		candidates = append(candidates, candidate)
	}
	return uniqueQobuzPythonCandidatesForRunner(candidates)
}

func qobuzRuntimeProbeFilesForRunner(entrypoint string) []string {
	out := []string{entrypoint}
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(entrypoint), ".exe") {
		base := strings.TrimSuffix(entrypoint, filepath.Ext(entrypoint))
		out = append(out, base+"-script.py")
		out = append(out, base+".py")
	}
	return out
}

func qobuzPythonFromShebangForRunner(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	if !s.Scan() {
		return ""
	}
	line := strings.TrimSpace(s.Text())
	if !strings.HasPrefix(line, "#!") {
		return ""
	}

	interpreter := strings.TrimSpace(strings.TrimPrefix(line, "#!"))
	if interpreter == "" {
		return ""
	}
	parts := strings.Fields(interpreter)
	if len(parts) == 0 {
		return ""
	}
	candidate := strings.Trim(parts[0], "\"'")
	if strings.EqualFold(filepath.Base(candidate), "env") && len(parts) >= 2 {
		candidate = strings.Trim(parts[1], "\"'")
	}
	return resolvePythonCandidatePathForRunner(candidate)
}

func resolvePythonCandidatePathForRunner(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}

	looksLikePath := filepath.IsAbs(candidate) ||
		strings.Contains(candidate, string(filepath.Separator)) ||
		(runtime.GOOS == "windows" && strings.Contains(candidate, "/"))

	if looksLikePath {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			return ""
		}
		return candidate
	}

	resolved, err := exec.LookPath(candidate)
	if err != nil {
		return ""
	}
	return resolved
}

func qobuzFallbackPythonCandidatesForRunner() []qobuzPythonCommandCandidate {
	if runtime.GOOS == "windows" {
		return uniqueQobuzPythonCandidatesForRunner([]qobuzPythonCommandCandidate{
			{Exec: "py", PrefixArgs: []string{"-3.13"}},
			{Exec: "py", PrefixArgs: []string{"-3.12"}},
			{Exec: "py", PrefixArgs: []string{"-3.11"}},
			{Exec: "py", PrefixArgs: []string{"-3"}},
			{Exec: "py"},
			{Exec: "python"},
			{Exec: "python3"},
		})
	}
	candidates := []qobuzPythonCommandCandidate{}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			qobuzPythonCommandCandidate{Exec: "/opt/homebrew/opt/python@3.13/bin/python3.13"},
			qobuzPythonCommandCandidate{Exec: "/opt/homebrew/opt/python@3.12/bin/python3.12"},
			qobuzPythonCommandCandidate{Exec: "/usr/local/opt/python@3.13/bin/python3.13"},
			qobuzPythonCommandCandidate{Exec: "/usr/local/opt/python@3.12/bin/python3.12"},
		)
	}
	candidates = append(candidates,
		qobuzPythonCommandCandidate{Exec: "python3.13"},
		qobuzPythonCommandCandidate{Exec: "python3.12"},
		qobuzPythonCommandCandidate{Exec: "python3.11"},
		qobuzPythonCommandCandidate{Exec: "python3"},
		qobuzPythonCommandCandidate{Exec: "python"},
	)
	return uniqueQobuzPythonCandidatesForRunner(candidates)
}

func uniqueQobuzPythonCandidatesForRunner(candidates []qobuzPythonCommandCandidate) []qobuzPythonCommandCandidate {
	out := make([]qobuzPythonCommandCandidate, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		execName := strings.TrimSpace(candidate.Exec)
		if execName == "" {
			continue
		}
		key := execName + "\x00" + strings.Join(candidate.PrefixArgs, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, qobuzPythonCommandCandidate{
			Exec:       execName,
			PrefixArgs: append([]string{}, candidate.PrefixArgs...),
		})
	}
	return out
}

func qobuzPythonCandidateSupportsModuleForRunner(candidate qobuzPythonCommandCandidate) bool {
	execName := strings.TrimSpace(candidate.Exec)
	if execName == "" {
		return false
	}

	if strings.Contains(execName, string(filepath.Separator)) || (runtime.GOOS == "windows" && strings.Contains(execName, "/")) {
		info, err := os.Stat(execName)
		if err != nil || info.IsDir() {
			return false
		}
	} else {
		if _, err := exec.LookPath(execName); err != nil {
			return false
		}
	}

	args := append(append([]string{}, candidate.PrefixArgs...), "-c", "import qobuz_dl.qopy")
	cmd := exec.Command(execName, args...)
	return cmd.Run() == nil
}

type whisperInvocationOptions struct {
	OutputBase         string
	GenerateSubtitle   bool
	GenerateTranscript bool
	GenerateJSONFull   bool
	Tinydiarize        bool
}

type whisperInvocationArtifacts struct {
	SubtitlePath   string
	TranscriptPath string
	JSONPath       string
}

func (r *Runner) transcribe(ctx context.Context, mediaPath, workspace string, job core.JobRequest, cb RunCallbacks) (transcriptionArtifacts, error) {
	wav := filepath.Join(workspace, "audio_for_whisper.wav")
	extractArgs := []string{"-y", "-nostdin", "-i", mediaPath, "-vn", "-ac", "1", "-ar", "16000"}
	extractArgs = append(extractArgs, util.ParseArgumentString(job.FfmpegExtraArguments)...)
	extractArgs = append(extractArgs, wav)
	_, err := r.processRunner.Run(ctx, sys.RunOptions{
		Executable:    "ffmpeg",
		Args:          extractArgs,
		WorkingDir:    workspace,
		CaptureOutput: false,
		OnOutput: func(line string) {
			if cb.OnLog != nil {
				cb.OnLog(line)
			}
			if cb.OnStepProgress != nil {
				if pct := parseFfmpegTimeProgress(line); pct >= 0 {
					cb.OnStepProgress(minFloat(0.15, pct*0.15))
				}
			}
		},
	})
	if err != nil {
		return transcriptionArtifacts{}, err
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0.15)
	}

	whisperExec := "whisper-cli"
	if resolved, _, resolveErr := util.ResolveToolExecutable("whisper-cli"); resolveErr == nil {
		whisperExec = resolved
	}
	diarizationProvider := resolvedJobDiarizationProvider(job)

	whisperModelPath := resolveWhisperModelPath(job.WhisperModelPath, whisperExec)
	if whisperModelPath == "" {
		return transcriptionArtifacts{}, fmt.Errorf("chemin du modele Whisper invalide. Configure-le dans Reglages")
	}
	tinydiarizeWhisperModelPath := whisperModelPath
	if strings.TrimSpace(job.WhisperTinydiarizeModelPath) != "" {
		tinydiarizeWhisperModelPath = resolveWhisperModelPath(job.WhisperTinydiarizeModelPath, whisperExec)
		if tinydiarizeWhisperModelPath == "" {
			return transcriptionArtifacts{}, fmt.Errorf("chemin du modele Whisper tinydiarize invalide. Configure un modele compatible `*-tdrz` dans Reglages ou dans le job")
		}
	}
	vadModelPath := ""
	if job.WhisperVADEnabled {
		vadModelPath = resolveVADModelPath(job.WhisperVADModelPath)
	}
	if cb.OnLog != nil && whisperModelPath != strings.TrimSpace(job.WhisperModelPath) {
		cb.OnLog("[transcription] Modele Whisper detecte: " + whisperModelPath + "\n")
	}
	if job.WhisperVADEnabled && strings.TrimSpace(vadModelPath) == "" {
		return transcriptionArtifacts{}, fmt.Errorf("VAD activé sans modèle VAD valide. Installe ou sélectionne un modèle VAD dans Réglages moteur ou dans le job")
	}
	if diarizationProvider == core.DiarizationProviderTinydiarize && !whisperModelSupportsTinydiarize(tinydiarizeWhisperModelPath) {
		return transcriptionArtifacts{}, fmt.Errorf("tinydiarize exige un modèle Whisper compatible `*-tdrz` (ex: `ggml-small.en-tdrz.bin`)")
	}
	if cb.OnLog != nil && vadModelPath != "" && vadModelPath != strings.TrimSpace(job.WhisperVADModelPath) {
		cb.OnLog("[transcription] Modele VAD detecte: " + vadModelPath + "\n")
	}
	if cb.OnLog != nil && diarizationProvider == core.DiarizationProviderTinydiarize {
		cb.OnLog("[transcription] Modele tinydiarize utilise: " + tinydiarizeWhisperModelPath + "\n")
	}
	if cb.OnLog != nil && whisperExec != "whisper-cli" {
		cb.OnLog("[transcription] Executable Whisper detecte: " + whisperExec + "\n")
	}

	requiresWhisperJSON := job.WhisperOutputJSONFull || diarizationProvider == core.DiarizationProviderPyannote
	primaryPlan := whisperInvocationOptions{
		OutputBase:         filepath.Join(workspace, "transcription"),
		GenerateSubtitle:   true,
		GenerateTranscript: true,
		GenerateJSONFull:   requiresWhisperJSON,
	}
	primaryArtifacts, err := r.runWhisperInvocation(ctx, whisperExec, whisperModelPath, vadModelPath, wav, workspace, job, primaryPlan, 0.15, 0.75, cb)
	if err != nil {
		return transcriptionArtifacts{}, err
	}
	out := transcriptionArtifacts{
		SubtitlePath:            primaryArtifacts.SubtitlePath,
		TranscriptPath:          primaryArtifacts.TranscriptPath,
		InternalWhisperJSONPath: primaryArtifacts.JSONPath,
	}
	if job.WhisperOutputJSONFull {
		out.JSONPath = primaryArtifacts.JSONPath
	}

	progressBase := 0.9
	switch diarizationProvider {
	case core.DiarizationProviderTinydiarize:
		tinydiarizePlan := whisperInvocationOptions{
			OutputBase:         filepath.Join(workspace, "transcription.tdrz"),
			GenerateSubtitle:   job.WhisperTinydiarizeOutputSRT,
			GenerateTranscript: job.WhisperTinydiarizeOutputTXT,
			GenerateJSONFull:   true,
			Tinydiarize:        true,
		}
		tinydiarizeArtifacts, runErr := r.runWhisperInvocation(ctx, whisperExec, tinydiarizeWhisperModelPath, vadModelPath, wav, workspace, job, tinydiarizePlan, 0.9, 0.09, cb)
		if runErr != nil {
			return transcriptionArtifacts{}, runErr
		}
		out.TinydiarizeJSONPath = tinydiarizeArtifacts.JSONPath
		out.TinydiarizeTranscriptPath = tinydiarizeArtifacts.TranscriptPath
		out.TinydiarizeSubtitlePath = tinydiarizeArtifacts.SubtitlePath
		progressBase = 0.99
	case core.DiarizationProviderPyannote:
		pyannoteJSONPath := filepath.Join(workspace, "transcription.pyannote.json")
		if err := r.runPyannoteDiarization(ctx, wav, pyannoteJSONPath, workspace, job, 0.9, 0.05, cb); err != nil {
			return transcriptionArtifacts{}, err
		}
		out.PyannoteJSONPath = pyannoteJSONPath
		if job.PyannoteOutputTXT || job.PyannoteOutputSRT {
			mergedSegments, mergeErr := mergeWhisperAndPyannoteJSON(out.InternalWhisperJSONPath, pyannoteJSONPath)
			if mergeErr != nil {
				return transcriptionArtifacts{}, mergeErr
			}
			if job.PyannoteOutputTXT {
				out.PyannoteTranscriptPath = filepath.Join(workspace, "transcription.pyannote.txt")
				if err := writeAnnotatedTranscript(out.PyannoteTranscriptPath, mergedSegments); err != nil {
					return transcriptionArtifacts{}, err
				}
			}
			if job.PyannoteOutputSRT {
				out.PyannoteSubtitlePath = filepath.Join(workspace, "transcription.pyannote.srt")
				if err := writeAnnotatedSRT(out.PyannoteSubtitlePath, mergedSegments); err != nil {
					return transcriptionArtifacts{}, err
				}
			}
		}
		progressBase = 0.99
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(progressBase)
	}
	return out, nil
}

func (r *Runner) runWhisperInvocation(ctx context.Context, whisperExec, whisperModelPath, vadModelPath, wavPath, workspace string, job core.JobRequest, options whisperInvocationOptions, progressBase, progressSpan float64, cb RunCallbacks) (whisperInvocationArtifacts, error) {
	args, artifacts, err := buildWhisperArgs(job, wavPath, whisperModelPath, vadModelPath, options)
	if err != nil {
		return whisperInvocationArtifacts{}, err
	}
	_, err = r.processRunner.Run(ctx, sys.RunOptions{
		Executable:    whisperExec,
		Args:          args,
		WorkingDir:    workspace,
		CaptureOutput: false,
		OnOutput: func(line string) {
			if cb.OnLog != nil {
				cb.OnLog(line)
			}
			if cb.OnStepProgress != nil {
				if pct := parsePercentProgress(line); pct >= 0 {
					cb.OnStepProgress(minFloat(progressBase+progressSpan, progressBase+(pct/100.0)*progressSpan))
				}
			}
		},
	})
	if err != nil {
		return whisperInvocationArtifacts{}, err
	}
	if options.GenerateSubtitle {
		if !fileExists(artifacts.SubtitlePath) {
			return whisperInvocationArtifacts{}, fmt.Errorf("Whisper n'a pas genere le fichier .srt attendu")
		}
	}
	if options.GenerateTranscript {
		if !fileExists(artifacts.TranscriptPath) {
			return whisperInvocationArtifacts{}, fmt.Errorf("Whisper n'a pas genere le fichier .txt attendu")
		}
	}
	if options.GenerateJSONFull {
		if !fileExists(artifacts.JSONPath) {
			return whisperInvocationArtifacts{}, fmt.Errorf("Whisper n'a pas genere le fichier JSON attendu")
		}
	}
	return artifacts, nil
}

func buildWhisperArgs(job core.JobRequest, wavPath, whisperModelPath, vadModelPath string, options whisperInvocationOptions) ([]string, whisperInvocationArtifacts, error) {
	if strings.TrimSpace(whisperModelPath) == "" {
		return nil, whisperInvocationArtifacts{}, fmt.Errorf("chemin du modele Whisper invalide. Configure-le dans Reglages")
	}
	if strings.TrimSpace(wavPath) == "" {
		return nil, whisperInvocationArtifacts{}, fmt.Errorf("fichier audio Whisper introuvable")
	}
	if strings.TrimSpace(options.OutputBase) == "" {
		return nil, whisperInvocationArtifacts{}, fmt.Errorf("base de sortie Whisper invalide")
	}
	if job.WhisperVADEnabled && strings.TrimSpace(vadModelPath) == "" {
		return nil, whisperInvocationArtifacts{}, fmt.Errorf("VAD activé sans modèle VAD valide. Installe ou sélectionne un modèle VAD dans Réglages moteur ou dans le job")
	}
	if options.Tinydiarize && !whisperModelSupportsTinydiarize(whisperModelPath) {
		return nil, whisperInvocationArtifacts{}, fmt.Errorf("tinydiarize exige un modèle Whisper compatible `*-tdrz` (ex: `ggml-small.en-tdrz.bin`)")
	}
	if job.WhisperVADThreshold < 0 {
		return nil, whisperInvocationArtifacts{}, fmt.Errorf("le seuil VAD doit être positif")
	}
	if job.WhisperVADMinSpeechDuration < 0 || job.WhisperVADMinSilenceDuration < 0 || job.WhisperVADSpeechPad < 0 {
		return nil, whisperInvocationArtifacts{}, fmt.Errorf("les durées VAD doivent être positives")
	}
	if job.WhisperMaxSegmentLength < 0 {
		return nil, whisperInvocationArtifacts{}, fmt.Errorf("la longueur maximale de segment doit être positive")
	}

	args := []string{"-m", whisperModelPath, "-f", wavPath, "-of", options.OutputBase}
	if options.GenerateSubtitle {
		args = append(args, "-osrt")
	}
	if options.GenerateTranscript {
		args = append(args, "-otxt")
	}
	if options.GenerateJSONFull {
		args = append(args, "-ojf")
	}
	if options.Tinydiarize {
		args = append(args, "-tdrz")
	}
	lang := strings.TrimSpace(job.TranscriptionLanguage)
	if lang == "" {
		lang = "auto"
	}
	args = append(args, "-l", lang)
	if job.WhisperVADEnabled {
		args = append(args, "--vad", "--vad-model", vadModelPath)
		if job.WhisperVADThreshold > 0 {
			args = append(args, "--vad-threshold", strconv.FormatFloat(job.WhisperVADThreshold, 'f', -1, 64))
		}
		if job.WhisperVADMinSpeechDuration > 0 {
			args = append(args, "--vad-min-speech-duration-ms", strconv.Itoa(job.WhisperVADMinSpeechDuration))
		}
		if job.WhisperVADMinSilenceDuration > 0 {
			args = append(args, "--vad-min-silence-duration-ms", strconv.Itoa(job.WhisperVADMinSilenceDuration))
		}
		if job.WhisperVADSpeechPad > 0 {
			args = append(args, "--vad-speech-pad-ms", strconv.Itoa(job.WhisperVADSpeechPad))
		}
	}
	if job.WhisperMaxSegmentLength > 0 {
		args = append(args, "-ml", strconv.Itoa(job.WhisperMaxSegmentLength))
	}
	if job.WhisperSplitOnWord {
		args = append(args, "-sow")
	}
	if strings.TrimSpace(job.WhisperInitialPrompt) != "" {
		args = append(args, "--prompt", strings.TrimSpace(job.WhisperInitialPrompt))
	}
	if job.WhisperCarryInitialPrompt {
		args = append(args, "--carry-initial-prompt")
	}
	args = append(args, util.ParseArgumentString(job.WhisperExtraArguments)...)

	artifacts := whisperInvocationArtifacts{}
	if options.GenerateSubtitle {
		artifacts.SubtitlePath = options.OutputBase + ".srt"
	}
	if options.GenerateTranscript {
		artifacts.TranscriptPath = options.OutputBase + ".txt"
	}
	if options.GenerateJSONFull {
		artifacts.JSONPath = options.OutputBase + ".json"
	}
	return args, artifacts, nil
}

func (r *Runner) translateTranscription(ctx context.Context, subtitlePath, transcriptPath, workspace string, job core.JobRequest, cb RunCallbacks) (string, string, error) {
	sourceLanguage := normalizeLanguageCode(job.TranslationSourceLanguage, "en")
	targetLanguage := normalizeLanguageCode(job.TranslationTargetLanguage, "fr")
	if sourceLanguage == targetLanguage {
		if cb.OnLog != nil {
			cb.OnLog("[translation] Etape ignoree (langues source/cible identiques).\n")
		}
		return subtitlePath, transcriptPath, nil
	}
	if strings.TrimSpace(subtitlePath) == "" && strings.TrimSpace(transcriptPath) == "" {
		if cb.OnLog != nil {
			cb.OnLog("[translation] Etape ignoree (aucun fichier a traduire).\n")
		}
		return subtitlePath, transcriptPath, nil
	}

	pythonExec, err := r.resolveArgosPythonExecutable(ctx)
	if err != nil {
		return "", "", err
	}
	scriptPath := strings.TrimSpace(r.argosScript)
	if scriptPath == "" {
		scriptPath = filepath.Join("assets", "scripts", "argos_translate_file.py")
	}
	if _, err := os.Stat(scriptPath); err != nil {
		return "", "", fmt.Errorf("script Argos introuvable: %s", scriptPath)
	}

	targetTag := languageFileTag(targetLanguage)
	translatedSubtitle := subtitlePath
	translatedTranscript := transcriptPath
	translatedSubtitlePath := filepath.Join(workspace, "transcription."+targetTag+".srt")
	translatedTranscriptPath := filepath.Join(workspace, "transcription."+targetTag+".txt")

	if cb.OnLog != nil {
		cb.OnLog(fmt.Sprintf("[translation] Traduction Argos %s -> %s\n", sourceLanguage, targetLanguage))
	}
	taskCount := 0
	if strings.TrimSpace(subtitlePath) != "" {
		taskCount++
	}
	if strings.TrimSpace(transcriptPath) != "" {
		taskCount++
	}
	progressBase := 0.93
	progressSpan := 0.06
	taskSpan := progressSpan
	if taskCount > 0 {
		taskSpan = progressSpan / float64(taskCount)
	}
	taskIndex := 0
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(progressBase)
	}

	didTranslateOne := false
	if strings.TrimSpace(subtitlePath) != "" {
		if cb.OnLog != nil {
			cb.OnLog("[translation] Traduction du fichier SRT...\n")
		}
		fileProgressBase := progressBase + float64(taskIndex)*taskSpan
		if err := r.translateFileWithArgos(ctx, pythonExec, scriptPath, subtitlePath, translatedSubtitlePath, sourceLanguage, targetLanguage, "srt", fileProgressBase, taskSpan, workspace, cb); err != nil {
			return "", "", err
		}
		translatedSubtitle = translatedSubtitlePath
		didTranslateOne = true
		taskIndex++
	}

	if strings.TrimSpace(transcriptPath) != "" {
		if cb.OnLog != nil {
			cb.OnLog("[translation] Traduction du fichier TXT...\n")
		}
		fileProgressBase := progressBase + float64(taskIndex)*taskSpan
		if err := r.translateFileWithArgos(ctx, pythonExec, scriptPath, transcriptPath, translatedTranscriptPath, sourceLanguage, targetLanguage, "txt", fileProgressBase, taskSpan, workspace, cb); err != nil {
			return "", "", err
		}
		translatedTranscript = translatedTranscriptPath
		didTranslateOne = true
		taskIndex++
	}
	if didTranslateOne && cb.OnStepProgress != nil {
		cb.OnStepProgress(progressBase + progressSpan)
	}

	return translatedSubtitle, translatedTranscript, nil
}

func (r *Runner) translateFileWithArgos(ctx context.Context, pythonExec, scriptPath, inputPath, outputPath, sourceLanguage, targetLanguage, format string, progressBase, progressSpan float64, workspace string, cb RunCallbacks) error {
	args := []string{
		scriptPath,
		"--input", inputPath,
		"--output", outputPath,
		"--format", format,
		"--from-code", sourceLanguage,
		"--to-code", targetLanguage,
	}
	lastProgress := progressBase
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(progressBase)
	}
	_, err := r.processRunner.Run(ctx, sys.RunOptions{
		Executable:    pythonExec,
		Args:          args,
		WorkingDir:    workspace,
		CaptureOutput: false,
		OnOutput: func(line string) {
			if cb.OnLog != nil {
				cb.OnLog(line)
			}
			if cb.OnStepProgress != nil {
				if pct := parseArgosProgressPercent(line); pct >= 0 {
					value := progressBase + (pct/100.0)*progressSpan
					if value > lastProgress {
						lastProgress = value
						cb.OnStepProgress(value)
					}
				}
			}
		},
	})
	if err != nil {
		return fmt.Errorf(
			"traduction Argos echouee (%s). Verifie l'installation de Python + argostranslate + paquet %s->%s: %w",
			format,
			sourceLanguage,
			targetLanguage,
			err,
		)
	}
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("traduction Argos incomplete: fichier %s manquant", filepath.Base(outputPath))
	}
	if cb.OnStepProgress != nil {
		finalProgress := progressBase + progressSpan
		if finalProgress > lastProgress {
			cb.OnStepProgress(finalProgress)
		}
	}
	return nil
}

func resolvePythonExecutable() (string, error) {
	for _, candidate := range []string{"python3", "python"} {
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("runtime Python introuvable (python3/python)")
}

func (r *Runner) resolvePyannotePythonExecutable(ctx context.Context) (string, error) {
	candidates := make([]string, 0, 8)
	candidates = append(candidates, util.PyannoteVenvPythonCandidates("")...)
	candidates = append(candidates, "python3.13", "python3.12", "python3.11", "python3", "python")

	var lastProbeError string
	for _, raw := range candidates {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}

		resolved := candidate
		if !strings.Contains(candidate, string(os.PathSeparator)) {
			path, err := exec.LookPath(candidate)
			if err != nil {
				continue
			}
			resolved = path
		} else {
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
		}

		output, err := r.processRunner.Run(ctx, sys.RunOptions{
			Executable:    resolved,
			Args:          []string{"-c", "import pyannote.audio"},
			CaptureOutput: true,
		})
		if err == nil {
			return resolved, nil
		}
		if line := strings.TrimSpace(output); line != "" {
			lastProbeError = firstNonEmptyLineValue(line)
		}
	}

	if strings.TrimSpace(lastProbeError) != "" {
		return "", fmt.Errorf("runtime pyannote indisponible: %s", lastProbeError)
	}
	return "", fmt.Errorf("runtime pyannote indisponible: installe/maj pyannote via Diagnostics")
}

func (r *Runner) runPyannoteDiarization(ctx context.Context, wavPath, outputJSONPath, workspace string, job core.JobRequest, progressBase, progressSpan float64, cb RunCallbacks) error {
	pythonExec, err := r.resolvePyannotePythonExecutable(ctx)
	if err != nil {
		return err
	}
	scriptPath := strings.TrimSpace(r.pyannoteScript)
	if scriptPath == "" {
		scriptPath = filepath.Join("assets", "scripts", "pyannote_diarize.py")
	}
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("script pyannote introuvable: %s", scriptPath)
	}
	args := []string{scriptPath, "--audio", wavPath, "--output-json", outputJSONPath}
	env := map[string]string{}
	pipelinePath := strings.TrimSpace(job.PyannoteLocalPipelinePath)
	if token := strings.TrimSpace(job.PyannoteHuggingFaceToken); token != "" && pipelinePath == "" {
		env["PYANNOTE_HF_TOKEN"] = token
	}
	if pipelinePath != "" {
		args = append(args, "--pipeline-path", pipelinePath)
	}
	if cb.OnLog != nil {
		cb.OnLog("[pyannote] Diarisation locale en cours...\n")
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(progressBase)
	}
	_, err = r.processRunner.Run(ctx, sys.RunOptions{
		Executable:    pythonExec,
		Args:          args,
		WorkingDir:    workspace,
		Environment:   env,
		CaptureOutput: false,
		OnOutput: func(line string) {
			if cb.OnLog != nil {
				cb.OnLog(line)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("diarisation pyannote echouee: %w", err)
	}
	if _, err := os.Stat(outputJSONPath); err != nil {
		return fmt.Errorf("diarisation pyannote incomplete: fichier JSON manquant")
	}
	if _, err := readPyannoteDiarization(outputJSONPath); err != nil {
		return fmt.Errorf("diarisation pyannote incomplete: %w", err)
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(progressBase + progressSpan)
	}
	return nil
}

func (r *Runner) resolveArgosPythonExecutable(ctx context.Context) (string, error) {
	candidates := make([]string, 0, 8)
	candidates = append(candidates, util.ArgosVenvPythonCandidates("")...)
	candidates = append(candidates, "python3.13", "python3.12", "python3.11", "python3", "python")

	var lastProbeError string
	for _, raw := range candidates {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}

		resolved := candidate
		if !strings.Contains(candidate, string(os.PathSeparator)) {
			path, err := exec.LookPath(candidate)
			if err != nil {
				continue
			}
			resolved = path
		} else {
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
		}

		output, err := r.processRunner.Run(ctx, sys.RunOptions{
			Executable:    resolved,
			Args:          []string{"-c", "import argostranslate.package as _p; import argostranslate.translate as _t"},
			CaptureOutput: true,
		})
		if err == nil {
			return resolved, nil
		}
		if line := strings.TrimSpace(output); line != "" {
			lastProbeError = firstNonEmptyLineValue(line)
		}
	}

	if strings.TrimSpace(lastProbeError) != "" {
		return "", fmt.Errorf("runtime Argos indisponible: %s", lastProbeError)
	}
	return "", fmt.Errorf("runtime Argos indisponible: installe/maj argostranslate via Diagnostics")
}

func firstNonEmptyLineValue(v string) string {
	for _, line := range strings.Split(v, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func languageFileTag(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "_", "-")
	if raw == "" {
		return "translated"
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "translated"
	}
	return b.String()
}

func resolveWhisperModelPath(configuredPath, whisperExecutable string) string {
	configuredPath = strings.TrimSpace(configuredPath)
	modelFiles := util.WhisperModelCandidateFiles()
	candidates := make([]string, 0, len(modelFiles)*8+2)
	if configuredPath != "" {
		candidates = append(candidates, configuredPath)
	}
	for _, dir := range util.WhisperModelSearchDirs(configuredPath, whisperExecutable) {
		for _, file := range modelFiles {
			candidates = append(candidates, filepath.Join(dir, file))
		}
	}
	return firstExistingWhisperModelPath(candidates)
}

func resolveVADModelPath(configuredPath string) string {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath != "" && fileExists(configuredPath) {
		return configuredPath
	}
	candidates := make([]string, 0, len(util.VADModelCandidateFiles())*4)
	if configuredPath != "" {
		candidates = append(candidates, configuredPath)
	}
	for _, dir := range util.VADModelSearchDirs(configuredPath) {
		for _, modelFile := range util.VADModelCandidateFiles() {
			candidates = append(candidates, filepath.Join(dir, modelFile))
		}
	}
	return firstExistingVADModelPath(candidates)
}

func firstExistingWhisperModelPath(paths []string) string {
	for _, candidate := range paths {
		path := strings.TrimSpace(candidate)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		return path
	}
	return ""
}

func firstExistingVADModelPath(paths []string) string {
	for _, p := range paths {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func whisperModelSupportsTinydiarize(modelPath string) bool {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(strings.TrimSpace(modelPath))))
	return strings.Contains(base, "tdrz")
}

type muxSubtitleTrack struct {
	Path     string
	Language string
	Default  bool
}

func (r *Runner) muxSubtitles(ctx context.Context, mediaPath, subtitlePath, workspace string, job core.JobRequest, artifacts translationVariantArtifacts, cb RunCallbacks) (string, error) {
	tracks := buildMuxSubtitleTracks(job, mediaPath, subtitlePath, artifacts)
	if len(tracks) == 0 {
		return "", fmt.Errorf("aucune piste de sous-titres disponible pour le mux")
	}

	defaultIndex := 0
	labels := make([]string, 0, len(tracks))
	args := []string{"-y", "-nostdin", "-i", mediaPath}
	for idx, track := range tracks {
		args = append(args, "-i", track.Path)
		label := subtitleLanguageLabel(track.Language)
		labels = append(labels, label)
		if track.Default {
			defaultIndex = idx
		}
	}

	// Keep existing media streams but replace subtitle tracks with normalized local tracks.
	args = append(args, "-map", "0", "-map", "-0:s")
	for idx := range tracks {
		args = append(args, "-map", fmt.Sprintf("%d:0", idx+1))
	}
	args = append(args, "-c", "copy", "-c:s", "srt")
	for idx, track := range tracks {
		streamIndex := strconv.Itoa(idx)
		args = append(args,
			"-metadata:s:s:"+streamIndex, "language="+ffmpegSubtitleLanguage(track.Language),
			"-metadata:s:s:"+streamIndex, "title="+subtitleLanguageLabel(track.Language),
			"-disposition:s:"+streamIndex, "0",
		)
	}
	args = append(args, "-disposition:s:"+strconv.Itoa(defaultIndex), "default")
	args = append(args, util.ParseArgumentString(job.FfmpegExtraArguments)...)

	muxed := filepath.Join(workspace, "video_muxed.mkv")
	args = append(args, muxed)
	if cb.OnLog != nil {
		cb.OnLog("[muxing] Sous-titres integres: " + strings.Join(labels, ", ") + "\n")
	}
	_, err := r.processRunner.Run(ctx, sys.RunOptions{
		Executable:    "ffmpeg",
		Args:          args,
		WorkingDir:    workspace,
		CaptureOutput: false,
		OnOutput: func(line string) {
			if cb.OnLog != nil {
				cb.OnLog(line)
			}
		},
	})
	if err != nil {
		return "", err
	}
	return muxed, nil
}

func buildMuxSubtitleTracks(job core.JobRequest, mediaPath, subtitlePath string, artifacts translationVariantArtifacts) []muxSubtitleTrack {
	mediaPath = strings.TrimSpace(mediaPath)
	selectedSubtitle := strings.TrimSpace(subtitlePath)
	if selectedSubtitle == "" {
		return nil
	}

	tracks := make([]muxSubtitleTrack, 0, 2)
	addTrack := func(path, language string, isDefault bool) {
		path = strings.TrimSpace(path)
		if path == "" || !fileExists(path) {
			return
		}
		for idx := range tracks {
			if samePath(tracks[idx].Path, path) {
				if isDefault {
					tracks[idx].Default = true
				}
				return
			}
		}
		tracks = append(tracks, muxSubtitleTrack{
			Path:     path,
			Language: normalizeSubtitleLanguage(language),
			Default:  isDefault,
		})
	}

	sourceLanguage := normalizeLanguageCode(job.TranslationSourceLanguage, "en")
	targetLanguage := normalizeLanguageCode(job.TranslationTargetLanguage, "fr")
	if strings.TrimSpace(artifacts.OriginalSubtitlePath) != "" {
		addTrack(artifacts.OriginalSubtitlePath, sourceLanguage, false)
	}
	if strings.TrimSpace(artifacts.TranslatedSubtitlePath) != "" {
		addTrack(artifacts.TranslatedSubtitlePath, targetLanguage, false)
	}

	if len(tracks) == 0 {
		addTrack(selectedSubtitle, detectSubtitleLanguage(job, mediaPath, selectedSubtitle, artifacts), true)
	} else {
		selectedIndex := -1
		for idx := range tracks {
			if samePath(tracks[idx].Path, selectedSubtitle) {
				selectedIndex = idx
				break
			}
		}
		if selectedIndex < 0 {
			addTrack(selectedSubtitle, detectSubtitleLanguage(job, mediaPath, selectedSubtitle, artifacts), true)
			for idx := range tracks {
				if samePath(tracks[idx].Path, selectedSubtitle) {
					selectedIndex = idx
					break
				}
			}
		}
		for idx := range tracks {
			tracks[idx].Default = false
		}
		if selectedIndex >= 0 {
			tracks[selectedIndex].Default = true
		} else {
			tracks[0].Default = true
		}
	}
	return tracks
}

func detectSubtitleLanguage(job core.JobRequest, mediaPath, subtitlePath string, artifacts translationVariantArtifacts) string {
	subtitlePath = strings.TrimSpace(subtitlePath)
	if subtitlePath == "" {
		return "und"
	}
	sourceLanguage := normalizeLanguageCode(job.TranslationSourceLanguage, "en")
	targetLanguage := normalizeLanguageCode(job.TranslationTargetLanguage, "fr")
	if samePath(subtitlePath, artifacts.OriginalSubtitlePath) {
		return sourceLanguage
	}
	if samePath(subtitlePath, artifacts.TranslatedSubtitlePath) {
		return targetLanguage
	}

	sourceTag := languageFileTag(sourceLanguage)
	targetTag := languageFileTag(targetLanguage)
	base := strings.ToLower(strings.TrimSpace(filepath.Base(subtitlePath)))
	if sourceTag != "" && strings.Contains(base, "."+strings.ToLower(sourceTag)+".") {
		return sourceLanguage
	}
	if targetTag != "" && strings.Contains(base, "."+strings.ToLower(targetTag)+".") {
		return targetLanguage
	}
	if mediaPath != "" {
		if sourceTag != "" && samePath(subtitlePath, sidecarPathForMedia(mediaPath, sourceTag, ".srt")) {
			return sourceLanguage
		}
		if targetTag != "" && samePath(subtitlePath, sidecarPathForMedia(mediaPath, targetTag, ".srt")) {
			return targetLanguage
		}
	}
	transcriptionLanguage := normalizeLanguageCode(job.TranscriptionLanguage, "und")
	if transcriptionLanguage == "und" && job.EnableTranslation {
		return sourceLanguage
	}
	return transcriptionLanguage
}

func normalizeSubtitleLanguage(raw string) string {
	normalized := normalizeLanguageCode(raw, "und")
	if normalized == "" {
		return "und"
	}
	return normalized
}

func subtitleLanguageLabel(raw string) string {
	tag := strings.ToLower(strings.TrimSpace(languageFileTag(raw)))
	if tag == "" || tag == "translated" {
		return "und"
	}
	return tag
}

func ffmpegSubtitleLanguage(raw string) string {
	label := subtitleLanguageLabel(raw)
	primary := label
	if parts := strings.Split(label, "-"); len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
		primary = strings.TrimSpace(parts[0])
	}
	if len(primary) == 3 {
		return strings.ToLower(primary)
	}
	if code, ok := subtitleLanguageISO6392[strings.ToLower(primary)]; ok {
		return code
	}
	return "und"
}

var subtitleLanguageISO6392 = map[string]string{
	"ar": "ara",
	"de": "deu",
	"en": "eng",
	"es": "spa",
	"fr": "fra",
	"hi": "hin",
	"it": "ita",
	"ja": "jpn",
	"ko": "kor",
	"pt": "por",
	"ru": "rus",
	"tr": "tur",
	"uk": "ukr",
	"vi": "vie",
	"zh": "zho",
}

func shouldTranscribe(job core.JobRequest) bool {
	return job.EnableTranscription
}

func canReuseTranscriptionOutput(job core.JobRequest, subtitleFile, transcriptFile string, artifacts transcriptionArtifacts) bool {
	if strings.TrimSpace(subtitleFile) == "" || strings.TrimSpace(transcriptFile) == "" {
		return false
	}
	provider := resolvedJobDiarizationProvider(job)
	if job.WhisperOutputJSONFull && strings.TrimSpace(artifacts.JSONPath) == "" {
		return false
	}
	switch provider {
	case core.DiarizationProviderTinydiarize:
		if strings.TrimSpace(artifacts.TinydiarizeJSONPath) == "" {
			return false
		}
		if job.WhisperTinydiarizeOutputTXT && strings.TrimSpace(artifacts.TinydiarizeTranscriptPath) == "" {
			return false
		}
		if job.WhisperTinydiarizeOutputSRT && strings.TrimSpace(artifacts.TinydiarizeSubtitlePath) == "" {
			return false
		}
	case core.DiarizationProviderPyannote:
		if strings.TrimSpace(artifacts.PyannoteJSONPath) == "" {
			return false
		}
		if job.PyannoteOutputTXT && strings.TrimSpace(artifacts.PyannoteTranscriptPath) == "" {
			return false
		}
		if job.PyannoteOutputSRT && strings.TrimSpace(artifacts.PyannoteSubtitlePath) == "" {
			return false
		}
	}
	return true
}

func shouldTranslate(job core.JobRequest, subtitleFile, transcriptFile string) bool {
	if !job.EnableTranslation || !shouldTranscribe(job) {
		return false
	}
	sourceLanguage := normalizeLanguageCode(job.TranslationSourceLanguage, "en")
	targetLanguage := normalizeLanguageCode(job.TranslationTargetLanguage, "fr")
	if sourceLanguage == targetLanguage {
		return false
	}
	return strings.TrimSpace(subtitleFile) != "" || strings.TrimSpace(transcriptFile) != ""
}

func shouldFetchLyrics(job core.JobRequest, artifact downloadArtifact) bool {
	return job.ContentType == core.ContentMusic && job.EnableLyrics && strings.TrimSpace(artifact.MediaPath) != ""
}

func resolveLyricsSearchHints(job core.JobRequest, artifact downloadArtifact) (string, string, string) {
	defaultTrackHint := strings.TrimSpace(artifact.Title)
	defaultArtistHint := strings.TrimSpace(artifact.SourceName)
	defaultAlbumHint := strings.TrimSpace(artifact.Title)

	if (job.SourceKind != core.SourceYouTube && job.SourceKind != core.SourceQobuz) || job.ContentType != core.ContentMusic || !job.UseCustomLyricsSearch {
		return defaultTrackHint, defaultArtistHint, defaultAlbumHint
	}

	trackHint := strings.TrimSpace(job.LyricsSearchTitle)
	if trackHint == "" {
		trackHint = defaultTrackHint
	}
	return trackHint, strings.TrimSpace(job.LyricsSearchArtist), strings.TrimSpace(job.LyricsSearchAlbum)
}

func shouldUseManualLyricsSelection(job core.JobRequest) bool {
	return (job.SourceKind == core.SourceYouTube || job.SourceKind == core.SourceQobuz) &&
		job.ContentType == core.ContentMusic &&
		job.UseManualLyricsSelection &&
		(strings.TrimSpace(job.ManualLyricsSynced) != "" || strings.TrimSpace(job.ManualLyricsPlain) != "")
}

func manualLyricsSelections(job core.JobRequest) []core.ManualLyricsSelection {
	selections := make([]core.ManualLyricsSelection, 0, len(job.ManualLyricsSelections)+1)
	for _, selection := range job.ManualLyricsSelections {
		entry := core.ManualLyricsSelection{
			TargetTrackName:  strings.TrimSpace(selection.TargetTrackName),
			TargetArtistName: strings.TrimSpace(selection.TargetArtistName),
			TargetAlbumName:  strings.TrimSpace(selection.TargetAlbumName),
			TrackName:        strings.TrimSpace(selection.TrackName),
			ArtistName:       strings.TrimSpace(selection.ArtistName),
			AlbumName:        strings.TrimSpace(selection.AlbumName),
			PlainLyrics:      strings.TrimSpace(selection.PlainLyrics),
			SyncedLyrics:     strings.TrimSpace(selection.SyncedLyrics),
		}
		if entry.PlainLyrics == "" && entry.SyncedLyrics == "" {
			continue
		}
		selections = append(selections, entry)
	}
	if len(selections) == 0 && shouldUseManualLyricsSelection(job) {
		selections = append(selections, core.ManualLyricsSelection{
			TrackName:    strings.TrimSpace(job.ManualLyricsTrackName),
			ArtistName:   strings.TrimSpace(job.ManualLyricsArtistName),
			AlbumName:    strings.TrimSpace(job.ManualLyricsAlbumName),
			PlainLyrics:  strings.TrimSpace(job.ManualLyricsPlain),
			SyncedLyrics: strings.TrimSpace(job.ManualLyricsSynced),
		})
	}
	return selections
}

func manualLyricsSelectionTargetCandidates(selection core.ManualLyricsSelection) []string {
	base := strings.TrimSpace(selection.TargetTrackName)
	if base == "" {
		base = strings.TrimSpace(selection.TrackName)
	}
	return normalizeLRCLIBTrackCandidates(base)
}

func manualLyricsSelectionMatchesTrack(selection core.ManualLyricsSelection, track string, totalAudioFiles int) bool {
	if totalAudioFiles == 1 && strings.TrimSpace(selection.TargetTrackName) == "" {
		return true
	}
	targetCandidates := manualLyricsSelectionTargetCandidates(selection)
	if len(targetCandidates) == 0 {
		return totalAudioFiles == 1
	}
	trackCandidates := normalizeLRCLIBTrackCandidates(track)
	if len(trackCandidates) == 0 {
		trackCandidates = []string{strings.TrimSpace(track)}
	}
	targets := map[string]struct{}{}
	for _, candidate := range targetCandidates {
		normalized := normalizeLRCLIBText(candidate)
		if normalized == "" {
			continue
		}
		targets[normalized] = struct{}{}
	}
	for _, candidate := range trackCandidates {
		if _, ok := targets[normalizeLRCLIBText(candidate)]; ok {
			return true
		}
	}
	return false
}

func writeManualLyricsSelectionFile(base string, selection core.ManualLyricsSelection) (string, error) {
	targetPath := base + ".lyrics.txt"
	payload := []byte(selection.PlainLyrics)
	generatedLabel := "[lyrics] Lyrics texte generes.\n"
	if strings.TrimSpace(selection.SyncedLyrics) != "" {
		targetPath = base + ".lrc"
		payload = []byte(selection.SyncedLyrics)
		generatedLabel = "[lyrics] Sous-titres synchronises generes.\n"
	}
	if err := os.WriteFile(targetPath, payload, 0o644); err != nil {
		return "", err
	}
	return generatedLabel, nil
}

func resolveLRCLIBSearchForAudioFile(track string, totalAudioFiles int, trackHint, artistHint, albumHint string) (string, string, string) {
	searchTrack := sanitizeLRCLIBTrackHint(track)
	if searchTrack == "" {
		searchTrack = track
	}
	searchArtist := sanitizeLRCLIBArtistHint(artistHint)
	searchAlbum := sanitizeLRCLIBAlbumHint(albumHint)
	if totalAudioFiles == 1 {
		if t := strings.TrimSpace(trackHint); t != "" {
			searchTrack = sanitizeLRCLIBTrackHint(t)
			if searchTrack == "" {
				searchTrack = t
			}
		}
	} else if searchArtist != "" && searchAlbum == "" {
		if a := strings.TrimSpace(trackHint); a != "" {
			// For legacy callers, trackHint may actually carry the album title for multi-track jobs.
			searchAlbum = sanitizeLRCLIBAlbumHint(a)
		}
	}
	return searchTrack, searchArtist, searchAlbum
}

func (r *Runner) fetchLyricsForJob(ctx context.Context, job core.JobRequest, artifact downloadArtifact, cb RunCallbacks) error {
	audioFiles := discoverAudioFiles(artifact.MediaPath)
	if len(audioFiles) == 0 {
		if cb.OnLog != nil {
			cb.OnLog("[lyrics] Aucun fichier audio trouve.\n")
		}
		return nil
	}

	trackHint, artistHint, albumHint := resolveLyricsSearchHints(job, artifact)
	selections := manualLyricsSelections(job)
	usedSelections := make([]bool, len(selections))

	if cb.OnStepCount != nil {
		cb.OnStepCount(0, len(audioFiles))
	}
	if cb.OnLog != nil {
		cb.OnLog(fmt.Sprintf("[lyrics] Recherche des sous-titres LRCLIB en cours (%d piste(s)).\n", len(audioFiles)))
	}

	generated, skipped, failed := 0, 0, 0
	for idx, file := range audioFiles {
		track := strings.TrimSpace(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
		if track == "" {
			track = fmt.Sprintf("Track %d", idx+1)
		}
		base := strings.TrimSuffix(file, filepath.Ext(file))
		if fileExists(base+".lrc") || fileExists(base+".lyrics.txt") {
			skipped++
			for selectionIdx, selection := range selections {
				if usedSelections[selectionIdx] {
					continue
				}
				if manualLyricsSelectionMatchesTrack(selection, track, len(audioFiles)) {
					usedSelections[selectionIdx] = true
					break
				}
			}
			if cb.OnLog != nil {
				cb.OnLog("[lyrics] Deja present, piste ignoree.\n")
			}
			if cb.OnStepProgress != nil {
				cb.OnStepProgress(float64(idx+1) / float64(len(audioFiles)))
			}
			if cb.OnStepCount != nil {
				cb.OnStepCount(idx+1, len(audioFiles))
			}
			continue
		}

		matchedSelection := -1
		for selectionIdx, selection := range selections {
			if usedSelections[selectionIdx] {
				continue
			}
			if manualLyricsSelectionMatchesTrack(selection, track, len(audioFiles)) {
				matchedSelection = selectionIdx
				usedSelections[selectionIdx] = true
				break
			}
		}
		if matchedSelection >= 0 {
			if cb.OnLog != nil {
				cb.OnLog(fmt.Sprintf("[lyrics] Selection LRCLIB manuelle detectee pour %q.\n", track))
			}
			label, err := writeManualLyricsSelectionFile(base, selections[matchedSelection])
			if err != nil {
				failed++
				if cb.OnLog != nil {
					cb.OnLog("[lyrics] Echec " + track + ": " + err.Error() + "\n")
				}
			} else {
				generated++
				if cb.OnLog != nil {
					cb.OnLog(label)
				}
			}
			if cb.OnStepProgress != nil {
				cb.OnStepProgress(float64(idx+1) / float64(len(audioFiles)))
			}
			if cb.OnStepCount != nil {
				cb.OnStepCount(idx+1, len(audioFiles))
			}
			continue
		}

		searchTrack, searchArtist, searchAlbum := resolveLRCLIBSearchForAudioFile(track, len(audioFiles), trackHint, artistHint, albumHint)
		if cb.OnLog != nil {
			cb.OnLog(fmt.Sprintf("[lyrics] Recherche %d/%d: track=%q, artist=%q, album=%q\n", idx+1, len(audioFiles), searchTrack, searchArtist, searchAlbum))
		}
		payload, err := fetchLRCLIB(ctx, r.httpClient, searchTrack, searchArtist, searchAlbum)
		if err != nil {
			failed++
			if cb.OnLog != nil {
				cb.OnLog("[lyrics] Echec " + track + ": " + err.Error() + "\n")
			}
			if cb.OnStepProgress != nil {
				cb.OnStepProgress(float64(idx+1) / float64(len(audioFiles)))
			}
			if cb.OnStepCount != nil {
				cb.OnStepCount(idx+1, len(audioFiles))
			}
			continue
		}
		if payload.syncedLyrics != "" {
			_ = os.WriteFile(base+".lrc", []byte(payload.syncedLyrics), 0o644)
			generated++
			if cb.OnLog != nil {
				cb.OnLog("[lyrics] Sous-titres synchronises generes.\n")
			}
		} else if payload.plainLyrics != "" {
			_ = os.WriteFile(base+".lyrics.txt", []byte(payload.plainLyrics), 0o644)
			generated++
			if cb.OnLog != nil {
				cb.OnLog("[lyrics] Lyrics texte generes.\n")
			}
		} else {
			if cb.OnLog != nil {
				detail := ""
				if searchArtist != "" {
					if searchAlbum != "" {
						detail = fmt.Sprintf(" (track=%q, artist=%q, album=%q)", searchTrack, searchArtist, searchAlbum)
					} else {
						detail = fmt.Sprintf(" (track=%q, artist=%q)", searchTrack, searchArtist)
					}
				} else if searchTrack != track {
					detail = fmt.Sprintf(" (track=%q)", searchTrack)
				}
				cb.OnLog("[lyrics] Aucun resultat LRCLIB pour cette piste." + detail + "\n")
			}
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(float64(idx+1) / float64(len(audioFiles)))
		}
		if cb.OnStepCount != nil {
			cb.OnStepCount(idx+1, len(audioFiles))
		}
	}
	if cb.OnLog != nil {
		for selectionIdx, selection := range selections {
			if usedSelections[selectionIdx] {
				continue
			}
			label := strings.TrimSpace(selection.TargetTrackName)
			if label == "" {
				label = strings.TrimSpace(selection.TrackName)
			}
			if label == "" {
				label = "selection LRCLIB"
			}
			cb.OnLog(fmt.Sprintf("[lyrics] Selection LRCLIB manuelle sans piste correspondante: %q.\n", label))
		}
		cb.OnLog(fmt.Sprintf("[lyrics] Termine: %d genere(s), %d deja present(s), %d erreur(s).\n", generated, skipped, failed))
	}
	return nil
}

func (r *Runner) applyManualLyricsSelection(job core.JobRequest, mediaPath string, cb RunCallbacks) (bool, error) {
	if !shouldUseManualLyricsSelection(job) {
		return false, nil
	}

	audioFiles := discoverAudioFiles(mediaPath)
	if len(audioFiles) != 1 {
		if cb.OnLog != nil {
			cb.OnLog("[lyrics] Selection LRCLIB manuelle ignoree (plusieurs pistes detectees). Retour au mode automatique.\n")
		}
		return false, nil
	}

	file := audioFiles[0]
	base := strings.TrimSuffix(file, filepath.Ext(file))
	if fileExists(base+".lrc") || fileExists(base+".lyrics.txt") {
		if cb.OnStepCount != nil {
			cb.OnStepCount(0, 1)
			cb.OnStepCount(1, 1)
		}
		if cb.OnLog != nil {
			cb.OnLog("[lyrics] Selection LRCLIB manuelle detectee.\n")
			cb.OnLog("[lyrics] Deja present, piste ignoree.\n")
			cb.OnLog("[lyrics] Termine: 0 genere(s), 1 deja present(s), 0 erreur(s).\n")
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(1)
		}
		return true, nil
	}

	targetPath := base + ".lyrics.txt"
	payload := []byte(job.ManualLyricsPlain)
	generatedLabel := "[lyrics] Lyrics texte generes.\n"
	if strings.TrimSpace(job.ManualLyricsSynced) != "" {
		targetPath = base + ".lrc"
		payload = []byte(job.ManualLyricsSynced)
		generatedLabel = "[lyrics] Sous-titres synchronises generes.\n"
	}
	if err := os.WriteFile(targetPath, payload, 0o644); err != nil {
		return false, err
	}
	if cb.OnStepCount != nil {
		cb.OnStepCount(0, 1)
		cb.OnStepCount(1, 1)
	}
	if cb.OnLog != nil {
		cb.OnLog("[lyrics] Selection LRCLIB manuelle detectee.\n")
		cb.OnLog(generatedLabel)
		cb.OnLog("[lyrics] Termine: 1 genere(s), 0 deja present(s), 0 erreur(s).\n")
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(1)
	}
	return true, nil
}

func discoverDownloadedMedia(workspace string) string {
	entries, _ := os.ReadDir(workspace)
	extOrder := map[string]int{".mkv": 1, ".mp4": 2, ".webm": 3, ".mov": 4, ".m4a": 5, ".mp3": 6, ".flac": 7, ".opus": 8, ".ogg": 9, ".wav": 10}
	best := ""
	bestScore := 9999
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		score, ok := extOrder[ext]
		if !ok {
			continue
		}
		if strings.HasSuffix(name, ".part") {
			continue
		}
		if score < bestScore {
			bestScore = score
			best = filepath.Join(workspace, name)
		}
	}
	return best
}

func discoverInfoJSON(workspace string) string {
	entries, _ := filepath.Glob(filepath.Join(workspace, "*.info.json"))
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool {
		li, _ := os.Stat(entries[i])
		lj, _ := os.Stat(entries[j])
		if li == nil || lj == nil {
			return entries[i] < entries[j]
		}
		return li.ModTime().After(lj.ModTime())
	})
	return entries[0]
}

type ytInfoParsed struct {
	title      string
	sourceName string
	date       *time.Time
}

func parseYtInfo(path string) ytInfoParsed {
	if strings.TrimSpace(path) == "" {
		return ytInfoParsed{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ytInfoParsed{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return ytInfoParsed{}
	}
	title := strings.TrimSpace(anyToString(m["title"]))
	source := strings.TrimSpace(anyToString(m["uploader"]))
	if source == "" {
		source = strings.TrimSpace(anyToString(m["channel"]))
	}
	if source == "" {
		source = strings.TrimSpace(anyToString(m["playlist_title"]))
	}
	var date *time.Time
	if raw := strings.TrimSpace(anyToString(m["upload_date"])); len(raw) == 8 {
		if parsed, err := time.Parse("20060102", raw); err == nil {
			u := parsed.UTC()
			date = &u
		}
	}
	return ytInfoParsed{title: title, sourceName: source, date: date}
}

func anyToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprintf("%.0f", x)
	default:
		return ""
	}
}

var percentRe = regexp.MustCompile(`([0-9]{1,3}(?:[.,][0-9]+)?)%`)
var argosProgressRe = regexp.MustCompile(`(?i)\[argos\][^\n\r]*?([0-9]{1,3})%`)

func parsePercentProgress(output string) float64 {
	matches := percentRe.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return -1
	}
	match := matches[len(matches)-1]
	var value float64
	_, err := fmt.Sscanf(strings.ReplaceAll(match[1], ",", "."), "%f", &value)
	if err != nil {
		return -1
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return value
}

func parseArgosProgressPercent(output string) float64 {
	matches := argosProgressRe.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return -1
	}
	match := matches[len(matches)-1]
	var value float64
	_, err := fmt.Sscanf(match[1], "%f", &value)
	if err != nil {
		return -1
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return value
}

var ffmpegTimeRe = regexp.MustCompile(`time=(\d{2}):(\d{2}):(\d{2}(?:\.\d+)?)`)

func parseFfmpegTimeProgress(output string) float64 {
	m := ffmpegTimeRe.FindStringSubmatch(output)
	if len(m) != 4 {
		return -1
	}
	var hh, mm, ss float64
	_, err := fmt.Sscanf(m[1], "%f", &hh)
	if err != nil {
		return -1
	}
	_, err = fmt.Sscanf(m[2], "%f", &mm)
	if err != nil {
		return -1
	}
	_, err = fmt.Sscanf(m[3], "%f", &ss)
	if err != nil {
		return -1
	}
	seconds := hh*3600 + mm*60 + ss
	if seconds <= 0 {
		return -1
	}
	// heuristic only
	return minFloat(1, seconds/1800.0)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func discoverLatestDirectory(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	list := []candidate{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, candidate{path: filepath.Join(root, e.Name()), mod: info.ModTime()})
	}
	if len(list) == 0 {
		return ""
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })
	return list[0].path
}

func parseQobuzDownloadingLabel(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	sep := strings.Index(trimmed, ":")
	if sep <= 0 {
		return ""
	}
	prefix := strings.ToLower(strings.TrimSpace(trimmed[:sep]))
	if prefix != "downloading" {
		return ""
	}
	label := strings.TrimSpace(trimmed[sep+1:])
	return label
}

func parseQobuzDirectoryFromProgressLine(line string) string {
	if !strings.Contains(line, ".tmp") || !strings.Contains(line, " /// ") {
		return ""
	}
	parts := strings.Split(line, " /// ")
	for idx := len(parts) - 1; idx >= 0; idx-- {
		segment := strings.TrimSpace(parts[idx])
		tmpStart := strings.LastIndex(segment, "/.")
		if tmpStart <= 0 || !strings.Contains(segment[tmpStart:], ".tmp") {
			continue
		}
		dir := strings.TrimSpace(segment[:tmpStart])
		if dir == "" {
			continue
		}
		if isQobuzDiscDirectory(filepath.Base(dir)) {
			dir = filepath.Dir(dir)
		}
		if dir != "" {
			return dir
		}
	}
	return ""
}

func isQobuzDiscDirectory(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasPrefix(name, "disc ") {
		return false
	}
	suffix := strings.TrimSpace(strings.TrimPrefix(name, "disc "))
	if suffix == "" {
		return false
	}
	_, err := strconv.Atoi(suffix)
	return err == nil
}

func appendOutputTail(current, chunk string, limit int) string {
	if limit <= 0 || chunk == "" {
		return current
	}
	current += chunk
	if len(current) <= limit {
		return current
	}
	return current[len(current)-limit:]
}

func detectQobuzRetryableDownloadError(output string) string {
	lower := strings.ToLower(strings.TrimSpace(output))
	if lower == "" {
		return ""
	}
	if strings.Contains(lower, "incompleteread") &&
		(strings.Contains(lower, "error getting release") || strings.Contains(lower, "connection broken")) {
		return "IncompleteRead"
	}
	return ""
}

func detectQobuzAuthenticationFailure(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "authenticationerror") ||
		strings.Contains(lower, "invalid credentials") ||
		strings.Contains(lower, "authentification qobuz refusee")
}

func detectQobuzOGCoverTooLargeError(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "downloaded cover size too large to embed") ||
		strings.Contains(lower, "turn off `og_cover` to avoid error") ||
		strings.Contains(lower, "turn off og_cover to avoid error")
}

func qobuzArgsContainOGCover(args []string) bool {
	for _, arg := range args {
		trimmed := strings.ToLower(strings.TrimSpace(arg))
		if trimmed == "--og-cover" || strings.HasPrefix(trimmed, "--og-cover=") {
			return true
		}
	}
	return false
}

func qobuzArgsWithoutOGCover(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		trimmed := strings.ToLower(strings.TrimSpace(arg))
		if trimmed == "--og-cover" || strings.HasPrefix(trimmed, "--og-cover=") {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

func waitForRetryDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func discoverQobuzDirectoryByDownloadLabel(root, label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	target := normalizeLRCLIBText(label)
	if target == "" {
		return ""
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	type candidate struct {
		path  string
		score int
		mod   time.Time
	}
	best := candidate{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(root, entry.Name())
		nameNorm := normalizeLRCLIBText(entry.Name())
		if nameNorm == "" {
			continue
		}
		score := 0
		if strings.Contains(nameNorm, target) {
			score = 2
		} else if strings.Contains(target, nameNorm) {
			score = 1
		}
		if score == 0 {
			continue
		}
		info, infoErr := entry.Info()
		mod := time.Time{}
		if infoErr == nil {
			mod = info.ModTime()
		}
		if score > best.score || (score == best.score && mod.After(best.mod)) {
			best = candidate{
				path:  dirPath,
				score: score,
				mod:   mod,
			}
		}
	}
	return best.path
}

func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

type qobuzFolderMeta struct {
	artistName string
	albumTitle string
}

func (r *Runner) readQobuzFolderMetadata(albumDir string, artistOverride string) qobuzFolderMeta {
	fallbackAlbum := util.SanitizePathComponent(filepath.Base(albumDir), 140)
	artist := strings.TrimSpace(artistOverride)
	if artist == "" {
		parts := strings.Split(filepath.Base(albumDir), " - ")
		if len(parts) >= 2 {
			artist = strings.TrimSpace(parts[0])
		}
	}
	if artist == "" {
		artist = "Artiste inconnu"
	}
	album := fallbackAlbum
	if parts := strings.Split(filepath.Base(albumDir), " - "); len(parts) >= 2 {
		album = strings.TrimSpace(parts[len(parts)-1])
	}
	if album == "" {
		album = fallbackAlbum
	}
	return qobuzFolderMeta{artistName: util.SanitizePathComponent(artist, 120), albumTitle: util.SanitizePathComponent(album, 140)}
}

func isDirectAudioURL(u *url.URL) bool {
	p := strings.ToLower(u.Path)
	return strings.HasSuffix(p, ".mp3") || strings.HasSuffix(p, ".m4a") || strings.HasSuffix(p, ".aac") || strings.HasSuffix(p, ".flac") || strings.HasSuffix(p, ".wav") || strings.HasSuffix(p, ".ogg") || strings.HasSuffix(p, ".opus")
}

func (r *Runner) downloadToFile(ctx context.Context, u *url.URL, workspace, base string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", "", err
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(u.Path)), ".")
	if ext == "" {
		ext = guessAudioExtFromContentType(resp.Header.Get("Content-Type"))
	}
	tmp := filepath.Join(workspace, base+".download")
	f, err := os.Create(tmp)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", "", err
	}
	return tmp, ext, nil
}

func guessAudioExtFromContentType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.Contains(ct, "audio/mpeg"):
		return "mp3"
	case strings.Contains(ct, "audio/mp4"):
		return "m4a"
	case strings.Contains(ct, "audio/x-flac") || strings.Contains(ct, "audio/flac"):
		return "flac"
	case strings.Contains(ct, "audio/wav"):
		return "wav"
	case strings.Contains(ct, "audio/ogg"):
		return "ogg"
	default:
		return "mp3"
	}
}

func (r *Runner) downloadArtwork(ctx context.Context, artworkURL, workspace, title string, cb RunCallbacks) (string, error) {
	u, err := url.Parse(strings.TrimSpace(artworkURL))
	if err != nil {
		return "", err
	}
	if cb.OnLog != nil {
		cb.OnLog("[rss] Telechargement illustration: " + u.String() + "\n")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(u.Path)), ".")
	if ext == "" {
		ext = guessImageExt(resp.Header.Get("Content-Type"))
	}
	base := util.SanitizePathComponent(title, 100)
	path := filepath.Join(workspace, base+".cover."+ext)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return path, nil
}

func guessImageExt(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.Contains(ct, "image/png"):
		return "png"
	case strings.Contains(ct, "image/webp"):
		return "webp"
	case strings.Contains(ct, "image/gif"):
		return "gif"
	default:
		return "jpg"
	}
}

func (r *Runner) embedArtwork(ctx context.Context, mediaPath, artworkPath, workspace string, cb RunCallbacks) (string, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(mediaPath), "."))
	if ext != "mp3" && ext != "m4a" && ext != "aac" && ext != "flac" && ext != "ogg" {
		if cb.OnLog != nil {
			cb.OnLog("[rss] Embed illustration ignore (format non compatible).\n")
		}
		return mediaPath, nil
	}
	out := filepath.Join(workspace, "audio_with_artwork."+ext)
	args := []string{"-y", "-nostdin", "-i", mediaPath, "-i", artworkPath, "-map", "0", "-map", "1", "-c", "copy", "-disposition:v:0", "attached_pic", out}
	_, err := r.processRunner.Run(ctx, sys.RunOptions{Executable: "ffmpeg", Args: args, WorkingDir: workspace, CaptureOutput: false, OnOutput: cb.OnLog})
	if err != nil {
		if cb.OnLog != nil {
			cb.OnLog("[rss] Embed illustration ignore: " + err.Error() + "\n")
		}
		return mediaPath, nil
	}
	return out, nil
}

func (r *Runner) findExistingQobuzAlbumDirectory(inputURL, outputRoot string) string {
	currentType, typeOK := util.QobuzResourceTypeFromURL(inputURL)
	if !typeOK {
		return ""
	}
	currentID, ok := util.QobuzResourceIdentifier(inputURL)
	if !ok || strings.TrimSpace(currentID) == "" {
		return ""
	}
	qobuzRoot := filepath.Join(outputRoot, "qobuz")
	if _, err := os.Stat(qobuzRoot); err != nil {
		return ""
	}
	found := ""
	_ = filepath.WalkDir(qobuzRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "album.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var meta map[string]any
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil
		}
		sourceKind := strings.ToLower(strings.TrimSpace(anyToString(meta["sourceKind"])))
		if sourceKind != string(core.SourceQobuz) {
			return nil
		}
		orig := strings.TrimSpace(anyToString(meta["originalInputURL"]))
		existingType, existingTypeOK := util.QobuzResourceTypeFromURL(orig)
		if !existingTypeOK || existingType != currentType {
			return nil
		}
		existingID, ok := util.QobuzResourceIdentifier(orig)
		if ok && existingID == currentID {
			found = filepath.Dir(path)
			return io.EOF
		}
		return nil
	})
	if found != "" {
		if _, err := os.Stat(found); err == nil {
			return found
		}
	}
	return ""
}

func isQobuzAlbumURL(inputURL string) bool {
	rt, ok := util.QobuzResourceTypeFromURL(inputURL)
	return ok && rt == util.QobuzAlbum
}

func (r *Runner) findExistingOutputForCompletion(job core.JobRequest, outputRoot string) existingOutput {
	switch job.SourceKind {
	case core.SourceYouTube:
		return r.findExistingYouTubeOutput(job.InputURL, outputRoot)
	case core.SourceRSS:
		return r.findExistingRSSOutput(job, outputRoot)
	default:
		return existingOutput{}
	}
}

func (r *Runner) findExistingYouTubeOutput(inputURL, outputRoot string) existingOutput {
	youtubeRoot := filepath.Join(outputRoot, "YouTube")
	if _, err := os.Stat(youtubeRoot); err != nil {
		return existingOutput{}
	}
	found := existingOutput{}
	_ = filepath.WalkDir(youtubeRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var meta MediaMetadata
		if unmarshalErr := json.Unmarshal(data, &meta); unmarshalErr != nil {
			return nil
		}
		if meta.SourceKind != core.SourceYouTube {
			return nil
		}
		if !youtubeInputURLsMatch(inputURL, meta.OriginalInputURL) {
			return nil
		}
		mediaPath := strings.TrimSpace(meta.MediaPath)
		if mediaPath == "" || !fileExists(mediaPath) {
			return nil
		}
		publicationDate := (*time.Time)(nil)
		if meta.PublicationDate != nil {
			value := meta.PublicationDate.UTC()
			publicationDate = &value
		}
		found = existingOutput{
			MediaPath:                 mediaPath,
			SubtitlePath:              firstExistingFile(strings.TrimSpace(meta.SubtitlePath), sidecarPathForMedia(mediaPath, "", ".srt")),
			TranscriptPath:            firstExistingFile(strings.TrimSpace(meta.TranscriptPath), sidecarPathForMedia(mediaPath, "", ".txt")),
			JSONPath:                  firstExistingFile(strings.TrimSpace(meta.JSONPath), whisperFullJSONPathForMedia(mediaPath)),
			TinydiarizeJSONPath:       firstExistingFile(strings.TrimSpace(meta.TinydiarizeJSONPath), tinydiarizeJSONPathForMedia(mediaPath)),
			TinydiarizeTranscriptPath: firstExistingFile(strings.TrimSpace(meta.TinydiarizeTranscriptPath), tinydiarizeTranscriptPathForMedia(mediaPath)),
			TinydiarizeSubtitlePath:   firstExistingFile(strings.TrimSpace(meta.TinydiarizeSubtitlePath), tinydiarizeSubtitlePathForMedia(mediaPath)),
			PyannoteJSONPath:          firstExistingFile(strings.TrimSpace(meta.PyannoteJSONPath), pyannoteJSONPathForMedia(mediaPath)),
			PyannoteTranscriptPath:    firstExistingFile(strings.TrimSpace(meta.PyannoteTranscriptPath), pyannoteTranscriptPathForMedia(mediaPath)),
			PyannoteSubtitlePath:      firstExistingFile(strings.TrimSpace(meta.PyannoteSubtitlePath), pyannoteSubtitlePathForMedia(mediaPath)),
			MetadataPath:              path,
			Title:                     strings.TrimSpace(meta.Title),
			SourceName:                strings.TrimSpace(meta.SourceName),
			PublicationDate:           publicationDate,
		}
		return io.EOF
	})
	return found
}

func (r *Runner) findExistingRSSOutput(job core.JobRequest, outputRoot string) existingOutput {
	selection := job.SelectedRSSEpisode
	if selection == nil {
		return existingOutput{}
	}
	rssRoot := filepath.Join(outputRoot, "RSS")
	if _, err := os.Stat(rssRoot); err != nil {
		return existingOutput{}
	}

	targetInputURL := normalizeComparableURL(job.InputURL)
	targetMediaURL := normalizeComparableURL(selection.MediaURL)
	targetTitle := normalizeComparableText(selection.Title)
	targetPodcast := normalizeComparableText(selection.PodcastTitle)
	hasTargetURL := targetInputURL != "" || targetMediaURL != ""
	found := existingOutput{}
	bestScore := 0

	_ = filepath.WalkDir(rssRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var meta MediaMetadata
		if unmarshalErr := json.Unmarshal(data, &meta); unmarshalErr != nil {
			return nil
		}
		if meta.SourceKind != core.SourceRSS {
			return nil
		}
		mediaPath := strings.TrimSpace(meta.MediaPath)
		if mediaPath == "" || !fileExists(mediaPath) {
			return nil
		}

		score := 0
		candidateInputURL := normalizeComparableURL(meta.OriginalInputURL)
		if hasTargetURL && candidateInputURL != targetMediaURL && candidateInputURL != targetInputURL {
			return nil
		}
		if targetMediaURL != "" && candidateInputURL == targetMediaURL {
			score += 4
		}
		if targetInputURL != "" && candidateInputURL == targetInputURL {
			score += 3
		}
		if targetTitle != "" && normalizeComparableText(meta.Title) == targetTitle {
			score += 2
		}
		if targetPodcast != "" && normalizeComparableText(meta.SourceName) == targetPodcast {
			score++
		}
		if score == 0 || score < bestScore {
			return nil
		}

		publicationDate := (*time.Time)(nil)
		if meta.PublicationDate != nil {
			value := meta.PublicationDate.UTC()
			publicationDate = &value
		}
		found = existingOutput{
			MediaPath:                 mediaPath,
			SubtitlePath:              firstExistingFile(strings.TrimSpace(meta.SubtitlePath), sidecarPathForMedia(mediaPath, "", ".srt")),
			TranscriptPath:            firstExistingFile(strings.TrimSpace(meta.TranscriptPath), sidecarPathForMedia(mediaPath, "", ".txt")),
			JSONPath:                  firstExistingFile(strings.TrimSpace(meta.JSONPath), whisperFullJSONPathForMedia(mediaPath)),
			TinydiarizeJSONPath:       firstExistingFile(strings.TrimSpace(meta.TinydiarizeJSONPath), tinydiarizeJSONPathForMedia(mediaPath)),
			TinydiarizeTranscriptPath: firstExistingFile(strings.TrimSpace(meta.TinydiarizeTranscriptPath), tinydiarizeTranscriptPathForMedia(mediaPath)),
			TinydiarizeSubtitlePath:   firstExistingFile(strings.TrimSpace(meta.TinydiarizeSubtitlePath), tinydiarizeSubtitlePathForMedia(mediaPath)),
			PyannoteJSONPath:          firstExistingFile(strings.TrimSpace(meta.PyannoteJSONPath), pyannoteJSONPathForMedia(mediaPath)),
			PyannoteTranscriptPath:    firstExistingFile(strings.TrimSpace(meta.PyannoteTranscriptPath), pyannoteTranscriptPathForMedia(mediaPath)),
			PyannoteSubtitlePath:      firstExistingFile(strings.TrimSpace(meta.PyannoteSubtitlePath), pyannoteSubtitlePathForMedia(mediaPath)),
			MetadataPath:              path,
			Title:                     strings.TrimSpace(meta.Title),
			SourceName:                strings.TrimSpace(meta.SourceName),
			PublicationDate:           publicationDate,
		}
		bestScore = score
		return nil
	})
	return found
}

func normalizeComparableText(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeComparableURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return strings.ToLower(strings.TrimSuffix(raw, "/"))
	}
	parsed.Fragment = ""
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	pathValue := strings.TrimSuffix(parsed.EscapedPath(), "/")
	if pathValue == "" {
		pathValue = "/"
	}
	query := parsed.Query()
	query.Del("utm_source")
	query.Del("utm_medium")
	query.Del("utm_campaign")
	encodedQuery := query.Encode()
	if encodedQuery == "" {
		return host + pathValue
	}
	return host + pathValue + "?" + encodedQuery
}

func youtubeInputURLsMatch(left, right string) bool {
	leftID := youtubeVideoIdentifier(left)
	rightID := youtubeVideoIdentifier(right)
	if leftID != "" && rightID != "" {
		return leftID == rightID
	}
	l := strings.TrimSpace(left)
	r := strings.TrimSpace(right)
	if l == "" || r == "" {
		return false
	}
	l = strings.TrimSuffix(l, "/")
	r = strings.TrimSuffix(r, "/")
	return strings.EqualFold(l, r)
}

func youtubeVideoIdentifier(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	pathValue := strings.Trim(parsed.Path, "/")
	if strings.Contains(host, "youtu.be") {
		if pathValue == "" {
			return ""
		}
		return strings.TrimSpace(strings.Split(pathValue, "/")[0])
	}
	if !strings.Contains(host, "youtube.com") {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(parsed.Path), "/watch") {
		return strings.TrimSpace(parsed.Query().Get("v"))
	}
	parts := strings.Split(pathValue, "/")
	if len(parts) >= 2 {
		switch strings.ToLower(strings.TrimSpace(parts[0])) {
		case "shorts", "embed", "live", "v":
			return strings.TrimSpace(parts[1])
		}
	}
	if queryID := strings.TrimSpace(parsed.Query().Get("v")); queryID != "" {
		return queryID
	}
	return ""
}

func firstExistingFile(paths ...string) string {
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func sidecarPathForMedia(mediaPath, languageTag, extension string) string {
	mediaPath = strings.TrimSpace(mediaPath)
	if mediaPath == "" {
		return ""
	}
	ext := strings.TrimSpace(extension)
	if ext == "" {
		ext = ".txt"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	base := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	tag := strings.TrimSpace(languageTag)
	if tag == "" {
		return base + ext
	}
	return base + "." + tag + ext
}

func findPreferredSidecarForCompletion(mediaPath, extension string, preferredLanguages []string, extraPaths ...string) string {
	candidates := make([]string, 0, len(preferredLanguages)+len(extraPaths)+6)
	for _, rawLanguage := range preferredLanguages {
		normalized := normalizeLanguageCode(rawLanguage, "")
		if normalized == "" {
			continue
		}
		tag := languageFileTag(normalized)
		if tag == "" || tag == "translated" {
			continue
		}
		candidates = append(candidates, sidecarPathForMedia(mediaPath, tag, extension))
	}
	candidates = append(candidates, extraPaths...)
	candidates = append(candidates, sidecarPathForMedia(mediaPath, "", extension))
	candidates = append(candidates, discoverTaggedSidecarsForMedia(mediaPath, extension)...)
	return firstExistingFile(dedupeStrings(candidates)...)
}

func discoverTaggedSidecarsForMedia(mediaPath, extension string) []string {
	mediaPath = strings.TrimSpace(mediaPath)
	if mediaPath == "" {
		return nil
	}
	ext := strings.TrimSpace(extension)
	if ext == "" {
		ext = ".txt"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	base := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	pattern := base + ".*" + ext
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	out := make([]string, 0, len(matches))
	for _, candidate := range matches {
		path := strings.TrimSpace(candidate)
		if path == "" || !fileExists(path) {
			continue
		}
		if samePath(path, base+ext) {
			continue
		}
		prefix := base + "."
		if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, ext) {
			continue
		}
		tag := strings.TrimSuffix(strings.TrimPrefix(path, prefix), ext)
		if strings.TrimSpace(tag) == "" {
			continue
		}
		out = append(out, path)
	}
	return out
}

func (r *Runner) preserveTranslationVariants(result core.JobResult, artifacts translationVariantArtifacts, cb RunCallbacks) error {
	if !artifacts.hasAny() {
		return nil
	}
	if strings.TrimSpace(artifacts.SourceLanguage) == "" || strings.TrimSpace(artifacts.TargetLanguage) == "" {
		return nil
	}
	if err := preserveLanguageVariant(result.SubtitlePath, artifacts.OriginalSubtitlePath, artifacts.TranslatedSubtitlePath, artifacts.SourceLanguage, artifacts.TargetLanguage, ".srt"); err != nil {
		return err
	}
	if err := preserveLanguageVariant(result.TranscriptPath, artifacts.OriginalTranscriptPath, artifacts.TranslatedTranscriptPath, artifacts.SourceLanguage, artifacts.TargetLanguage, ".txt"); err != nil {
		return err
	}
	if cb.OnLog != nil {
		cb.OnLog("[translation] Variantes source/traduite conservees.\n")
	}
	return nil
}

func preserveLanguageVariant(primaryPath, originalSource, translatedSource, sourceLanguage, targetLanguage, fallbackExtension string) error {
	primaryPath = strings.TrimSpace(primaryPath)
	if primaryPath == "" {
		return nil
	}
	sourceTag := languageFileTag(sourceLanguage)
	targetTag := languageFileTag(targetLanguage)
	if sourceTag == "" || targetTag == "" || sourceTag == targetTag {
		return nil
	}
	primaryExt := strings.TrimSpace(filepath.Ext(primaryPath))
	if primaryExt == "" {
		primaryExt = fallbackExtension
	}
	if primaryExt == "" {
		primaryExt = ".txt"
	}
	base := strings.TrimSuffix(primaryPath, filepath.Ext(primaryPath))
	sourceDestination := base + "." + sourceTag + primaryExt
	targetDestination := base + "." + targetTag + primaryExt

	originalInput := firstExistingFile(originalSource)
	if originalInput != "" && !samePath(originalInput, sourceDestination) {
		if err := copyFileReplacing(originalInput, sourceDestination); err != nil {
			return err
		}
	}
	translatedInput := firstExistingFile(translatedSource, primaryPath)
	if translatedInput != "" && !samePath(translatedInput, targetDestination) {
		if err := copyFileReplacing(translatedInput, targetDestination); err != nil {
			return err
		}
	}
	return nil
}

func copyFileReplacing(src, dst string) error {
	src = strings.TrimSpace(src)
	dst = strings.TrimSpace(dst)
	if src == "" || dst == "" || samePath(src, dst) {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func (r *Runner) fetchLyricsFromLRCLIB(ctx context.Context, albumDir string, cb RunCallbacks) {
	r.fetchLyricsFromLRCLIBWithHints(ctx, albumDir, "", "", "", cb)
}

func (r *Runner) fetchLyricsFromLRCLIBWithHints(ctx context.Context, albumDir, trackHint, artistHint, albumHint string, cb RunCallbacks) {
	audioFiles := discoverAudioFiles(albumDir)
	if len(audioFiles) == 0 {
		if cb.OnLog != nil {
			cb.OnLog("[lyrics] Aucun fichier audio trouve.\n")
		}
		return
	}
	if cb.OnStepCount != nil {
		cb.OnStepCount(0, len(audioFiles))
	}
	if cb.OnLog != nil {
		cb.OnLog(fmt.Sprintf("[lyrics] Recherche des sous-titres LRCLIB en cours (%d piste(s)).\n", len(audioFiles)))
	}
	generated, skipped, failed := 0, 0, 0
	for idx, file := range audioFiles {
		track := strings.TrimSpace(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
		if track == "" {
			track = fmt.Sprintf("Track %d", idx+1)
		}
		base := strings.TrimSuffix(file, filepath.Ext(file))
		if fileExists(base+".lrc") || fileExists(base+".lyrics.txt") {
			skipped++
			if cb.OnLog != nil {
				cb.OnLog("[lyrics] Deja present, piste ignoree.\n")
			}
			if cb.OnStepProgress != nil {
				cb.OnStepProgress(float64(idx+1) / float64(len(audioFiles)))
			}
			if cb.OnStepCount != nil {
				cb.OnStepCount(idx+1, len(audioFiles))
			}
			continue
		}
		searchTrack := sanitizeLRCLIBTrackHint(track)
		if searchTrack == "" {
			searchTrack = track
		}
		searchArtist := sanitizeLRCLIBArtistHint(artistHint)
		searchAlbum := sanitizeLRCLIBAlbumHint(albumHint)
		if len(audioFiles) == 1 {
			if t := strings.TrimSpace(trackHint); t != "" {
				searchTrack = sanitizeLRCLIBTrackHint(t)
				if searchTrack == "" {
					searchTrack = t
				}
			}
		} else if searchArtist != "" && searchAlbum == "" {
			if a := strings.TrimSpace(trackHint); a != "" {
				// For legacy callers, trackHint may actually carry the album title for multi-track jobs.
				searchAlbum = sanitizeLRCLIBAlbumHint(a)
			}
		}
		if cb.OnLog != nil {
			cb.OnLog(fmt.Sprintf("[lyrics] Recherche %d/%d: track=%q, artist=%q, album=%q\n", idx+1, len(audioFiles), searchTrack, searchArtist, searchAlbum))
		}
		payload, err := fetchLRCLIB(ctx, r.httpClient, searchTrack, searchArtist, searchAlbum)
		if err != nil {
			failed++
			if cb.OnLog != nil {
				cb.OnLog("[lyrics] Echec " + track + ": " + err.Error() + "\n")
			}
			if cb.OnStepProgress != nil {
				cb.OnStepProgress(float64(idx+1) / float64(len(audioFiles)))
			}
			if cb.OnStepCount != nil {
				cb.OnStepCount(idx+1, len(audioFiles))
			}
			continue
		}
		if payload.syncedLyrics != "" {
			_ = os.WriteFile(base+".lrc", []byte(payload.syncedLyrics), 0o644)
			generated++
			if cb.OnLog != nil {
				cb.OnLog("[lyrics] Sous-titres synchronises generes.\n")
			}
		} else if payload.plainLyrics != "" {
			_ = os.WriteFile(base+".lyrics.txt", []byte(payload.plainLyrics), 0o644)
			generated++
			if cb.OnLog != nil {
				cb.OnLog("[lyrics] Lyrics texte generes.\n")
			}
		} else {
			if cb.OnLog != nil {
				detail := ""
				if searchArtist != "" {
					if searchAlbum != "" {
						detail = fmt.Sprintf(" (track=%q, artist=%q, album=%q)", searchTrack, searchArtist, searchAlbum)
					} else {
						detail = fmt.Sprintf(" (track=%q, artist=%q)", searchTrack, searchArtist)
					}
				} else if searchTrack != track {
					detail = fmt.Sprintf(" (track=%q)", searchTrack)
				}
				cb.OnLog("[lyrics] Aucun resultat LRCLIB pour cette piste." + detail + "\n")
			}
		}
		if cb.OnStepProgress != nil {
			cb.OnStepProgress(float64(idx+1) / float64(len(audioFiles)))
		}
		if cb.OnStepCount != nil {
			cb.OnStepCount(idx+1, len(audioFiles))
		}
	}
	if cb.OnLog != nil {
		cb.OnLog(fmt.Sprintf("[lyrics] Termine: %d genere(s), %d deja present(s), %d erreur(s).\n", generated, skipped, failed))
	}
}

type lrclibPayload struct {
	plainLyrics  string
	syncedLyrics string
}

type lrclibHTTPError struct {
	statusCode int
}

func (e lrclibHTTPError) Error() string {
	return fmt.Sprintf("HTTP %d", e.statusCode)
}

type lrclibSearchQuery struct {
	trackName  string
	artistName string
	albumName  string
	query      string
}

type lrclibCandidate struct {
	trackName   string
	artistName  string
	albumName   string
	payload     lrclibPayload
	score       int
	trackScore  int
	artistScore int
	albumScore  int
}

func fetchLRCLIB(ctx context.Context, client *http.Client, track, artistHint, albumHint string) (lrclibPayload, error) {
	const maxAttempts = 3
	retryDelay := 350 * time.Millisecond
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		payload, err := fetchLRCLIBOnce(ctx, client, track, artistHint, albumHint)
		if err == nil {
			return payload, nil
		}
		lastErr = err
		if attempt == maxAttempts || !isRetryableLRCLIBError(err) {
			break
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lrclibPayload{}, ctx.Err()
		case <-timer.C:
		}
		if retryDelay < 2*time.Second {
			retryDelay *= 2
		}
	}
	if lastErr == nil {
		lastErr = errors.New("unexpected lrclib error")
	}
	return lrclibPayload{}, lastErr
}

func fetchLRCLIBOnce(ctx context.Context, client *http.Client, track, artistHint, albumHint string) (lrclibPayload, error) {
	queries := buildLRCLIBSearchQueries(track, artistHint, albumHint)
	if len(queries) == 0 {
		return lrclibPayload{}, nil
	}
	targetTrack := strings.TrimSpace(track)
	targetArtist := strings.TrimSpace(artistHint)
	targetAlbum := strings.TrimSpace(albumHint)
	bestScore := -1
	bestPayload := lrclibPayload{}
	for _, search := range queries {
		scoreSearch := search
		if targetTrack != "" && strings.TrimSpace(scoreSearch.trackName) == "" {
			scoreSearch.trackName = targetTrack
		}
		if targetArtist != "" {
			scoreSearch.artistName = targetArtist
		}
		if targetAlbum != "" && strings.TrimSpace(scoreSearch.albumName) == "" {
			scoreSearch.albumName = targetAlbum
		}
		payload, score, err := fetchLRCLIBSearch(ctx, client, search, scoreSearch)
		if err != nil {
			return lrclibPayload{}, err
		}
		if score > bestScore {
			bestScore = score
			bestPayload = payload
		}
	}
	if bestScore < 0 {
		return lrclibPayload{}, nil
	}
	return bestPayload, nil
}

func fetchLRCLIBSearch(ctx context.Context, client *http.Client, request lrclibSearchQuery, scoreSearch lrclibSearchQuery) (lrclibPayload, int, error) {
	candidates, err := fetchLRCLIBSearchCandidates(ctx, client, request, scoreSearch)
	if err != nil {
		return lrclibPayload{}, -1, err
	}
	payload, score := pickBestLRCLIBCandidate(candidates, scoreSearch)
	return payload, score, nil
}

func fetchLRCLIBSearchCandidates(ctx context.Context, client *http.Client, request lrclibSearchQuery, scoreSearch lrclibSearchQuery) ([]lrclibCandidate, error) {
	q := url.Values{}
	trackName := strings.TrimSpace(request.trackName)
	artistName := strings.TrimSpace(request.artistName)
	albumName := strings.TrimSpace(request.albumName)
	query := strings.TrimSpace(request.query)
	if trackName != "" {
		q.Set("track_name", trackName)
	}
	if artistName != "" {
		q.Set("artist_name", artistName)
	}
	if albumName != "" {
		q.Set("album_name", albumName)
	}
	if query != "" {
		q.Set("q", query)
	}
	if len(q) == 0 {
		return nil, nil
	}

	u := "https://lrclib.net/api/search?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, lrclibHTTPError{statusCode: resp.StatusCode}
	}
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	return decodeLRCLIBCandidates(items, scoreSearch), nil
}

func pickBestLRCLIBPayload(items []map[string]any, search lrclibSearchQuery) (lrclibPayload, int) {
	return pickBestLRCLIBCandidate(decodeLRCLIBCandidates(items, search), search)
}

func pickBestLRCLIBCandidate(candidates []lrclibCandidate, search lrclibSearchQuery) (lrclibPayload, int) {
	bestScore := -1
	bestPayload := lrclibPayload{}
	bestArtistScore := -1
	bestArtistPayload := lrclibPayload{}
	searchTrack := normalizeLRCLIBText(search.trackName)
	searchArtist := normalizeLRCLIBText(search.artistName)

	for _, candidate := range candidates {
		if candidate.score > bestScore {
			bestScore = candidate.score
			bestPayload = candidate.payload
		}
		if searchArtist != "" && candidate.artistScore > 0 {
			if searchTrack != "" && candidate.trackScore == 0 {
				// Artist match without track overlap is too risky for plain-text lyrics.
				continue
			}
			if candidate.score > bestArtistScore {
				bestArtistScore = candidate.score
				bestArtistPayload = candidate.payload
			}
		}
	}
	if searchArtist != "" {
		if bestArtistScore >= 0 {
			return bestArtistPayload, bestArtistScore
		}
		return lrclibPayload{}, -1
	}
	return bestPayload, bestScore
}

func decodeLRCLIBCandidates(items []map[string]any, search lrclibSearchQuery) []lrclibCandidate {
	searchTrack := normalizeLRCLIBText(search.trackName)
	searchArtist := normalizeLRCLIBText(search.artistName)
	searchAlbum := normalizeLRCLIBText(search.albumName)
	searchQuery := normalizeLRCLIBText(search.query)

	candidates := make([]lrclibCandidate, 0, len(items))
	for _, item := range items {
		payload := lrclibPayload{
			plainLyrics:  strings.TrimSpace(anyToString(item["plainLyrics"])),
			syncedLyrics: strings.TrimSpace(anyToString(item["syncedLyrics"])),
		}
		if payload.plainLyrics == "" {
			payload.plainLyrics = strings.TrimSpace(anyToString(item["plain_lyrics"]))
		}
		if payload.syncedLyrics == "" {
			payload.syncedLyrics = strings.TrimSpace(anyToString(item["synced_lyrics"]))
		}
		if payload.plainLyrics == "" && payload.syncedLyrics == "" {
			continue
		}

		itemTrack := strings.TrimSpace(firstNonEmptyValue(item["trackName"], item["track_name"], item["name"]))
		itemArtist := strings.TrimSpace(firstNonEmptyValue(item["artistName"], item["artist_name"], item["artist"]))
		itemAlbum := strings.TrimSpace(firstNonEmptyValue(item["albumName"], item["album_name"], item["album"]))
		normalizedTrack := normalizeLRCLIBText(itemTrack)
		normalizedArtist := normalizeLRCLIBText(itemArtist)
		normalizedAlbum := normalizeLRCLIBText(itemAlbum)

		score := 0
		if payload.syncedLyrics != "" {
			score += 50
		} else {
			score += 10
		}

		trackScore := partialMatchScore(searchTrack, normalizedTrack, 40, 20)
		artistScore := partialMatchScore(searchArtist, normalizedArtist, 25, 12)
		albumScore := partialMatchScore(searchAlbum, normalizedAlbum, 12, 6)
		score += trackScore + artistScore + albumScore

		// Avoid selecting unrelated songs when a source artist is known.
		if searchArtist != "" && normalizedArtist != "" && artistScore == 0 {
			score -= 25
		}
		if searchArtist != "" && normalizedArtist == "" {
			score -= 20
		}
		if searchTrack != "" && normalizedTrack != "" && trackScore == 0 {
			score -= 15
		}
		if searchAlbum != "" && normalizedAlbum != "" && albumScore == 0 {
			score -= 6
		}
		if searchQuery != "" && (searchTrack == "" || searchArtist == "") {
			haystack := strings.TrimSpace(normalizedArtist + " " + normalizedTrack)
			if haystack == searchQuery {
				score += 8
			} else if strings.Contains(haystack, searchQuery) || strings.Contains(searchQuery, haystack) {
				score += 4
			}
		}

		candidates = append(candidates, lrclibCandidate{
			trackName:   itemTrack,
			artistName:  itemArtist,
			albumName:   itemAlbum,
			payload:     payload,
			score:       score,
			trackScore:  trackScore,
			artistScore: artistScore,
			albumScore:  albumScore,
		})
	}
	return candidates
}

func searchLRCLIBCandidates(ctx context.Context, client *http.Client, track, artistHint, albumHint string) ([]lrclibCandidate, error) {
	queries := buildLRCLIBSearchQueries(track, artistHint, albumHint)
	if len(queries) == 0 {
		return nil, nil
	}
	targetTrack := strings.TrimSpace(track)
	targetArtist := strings.TrimSpace(artistHint)
	targetAlbum := strings.TrimSpace(albumHint)
	byKey := map[string]lrclibCandidate{}
	for _, search := range queries {
		scoreSearch := search
		if targetTrack != "" && strings.TrimSpace(scoreSearch.trackName) == "" {
			scoreSearch.trackName = targetTrack
		}
		if targetArtist != "" {
			scoreSearch.artistName = targetArtist
		}
		if targetAlbum != "" && strings.TrimSpace(scoreSearch.albumName) == "" {
			scoreSearch.albumName = targetAlbum
		}
		candidates, err := fetchLRCLIBSearchCandidates(ctx, client, search, scoreSearch)
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			key := lrclibCandidateKey(candidate)
			existing, ok := byKey[key]
			if !ok || candidate.score > existing.score {
				byKey[key] = candidate
			}
		}
	}

	results := make([]lrclibCandidate, 0, len(byKey))
	for _, candidate := range byKey {
		results = append(results, candidate)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		iHasSynced := strings.TrimSpace(results[i].payload.syncedLyrics) != ""
		jHasSynced := strings.TrimSpace(results[j].payload.syncedLyrics) != ""
		if iHasSynced != jHasSynced {
			return iHasSynced
		}
		if results[i].trackName != results[j].trackName {
			return strings.ToLower(results[i].trackName) < strings.ToLower(results[j].trackName)
		}
		if results[i].artistName != results[j].artistName {
			return strings.ToLower(results[i].artistName) < strings.ToLower(results[j].artistName)
		}
		return strings.ToLower(results[i].albumName) < strings.ToLower(results[j].albumName)
	})
	return results, nil
}

func lrclibCandidateKey(candidate lrclibCandidate) string {
	return strings.ToLower(strings.TrimSpace(candidate.trackName)) + "\x00" +
		strings.ToLower(strings.TrimSpace(candidate.artistName)) + "\x00" +
		strings.ToLower(strings.TrimSpace(candidate.albumName)) + "\x00" +
		candidate.payload.syncedLyrics + "\x00" +
		candidate.payload.plainLyrics
}

func partialMatchScore(expected, actual string, exactScore, partialScore int) int {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if expected == "" || actual == "" {
		return 0
	}
	if expected == actual {
		return exactScore
	}
	if strings.Contains(actual, expected) || strings.Contains(expected, actual) {
		return partialScore
	}
	return 0
}

func firstNonEmptyValue(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(anyToString(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func previewLRCLIBText(payload lrclibPayload) string {
	text := strings.TrimSpace(payload.syncedLyrics)
	if text == "" {
		text = strings.TrimSpace(payload.plainLyrics)
	}
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	preview := make([]string, 0, minInt(len(lines), 4))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		preview = append(preview, line)
		if len(preview) == 4 {
			break
		}
	}
	joined := strings.Join(preview, "\n")
	if joined == "" {
		joined = text
	}
	if len([]rune(joined)) > 280 {
		runes := []rune(joined)
		return strings.TrimSpace(string(runes[:280])) + "..."
	}
	return joined
}

func buildLRCLIBSearchQueries(track, artistHint, albumHint string) []lrclibSearchQuery {
	raw := strings.TrimSpace(track)
	if raw == "" {
		return nil
	}
	artistHint = strings.TrimSpace(artistHint)
	albumHint = sanitizeLRCLIBAlbumHint(albumHint)
	candidates := normalizeLRCLIBTrackCandidates(raw)
	queries := make([]lrclibSearchQuery, 0, 10)
	seen := map[string]bool{}
	addQuery := func(trackName, artistName, albumName, query string) {
		trackName = strings.TrimSpace(trackName)
		artistName = strings.TrimSpace(artistName)
		albumName = strings.TrimSpace(albumName)
		query = strings.TrimSpace(query)
		if trackName == "" && query == "" {
			return
		}
		key := strings.ToLower(trackName + "\x00" + artistName + "\x00" + albumName + "\x00" + query)
		if seen[key] {
			return
		}
		seen[key] = true
		queries = append(queries, lrclibSearchQuery{
			trackName:  trackName,
			artistName: artistName,
			albumName:  albumName,
			query:      query,
		})
	}
	if artistHint != "" {
		addQuery(raw, artistHint, albumHint, "")
		addQuery("", "", "", artistHint+" "+raw)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		addQuery(candidate, "", albumHint, "")
		if artistHint != "" {
			addQuery(candidate, artistHint, albumHint, "")
		}
		if artist, title, ok := splitArtistAndTitle(candidate); ok {
			addQuery(title, artist, albumHint, "")
			addQuery("", "", "", artist+" "+title)
			addQuery(title, "", albumHint, "")
		}
		addQuery("", "", "", candidate)
	}

	if len(queries) > 24 {
		return queries[:24]
	}
	return queries
}

var youtubeIDSuffixRe = regexp.MustCompile(`\s*\[[A-Za-z0-9_-]{6,}\]\s*$`)
var lrclibLeadingTrackNumberRe = regexp.MustCompile(`^\s*\d{1,3}\s*[\.\-:]\s+`)
var lrclibBracketChunkRe = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
var lrclibTitleNoiseRe = regexp.MustCompile(`(?i)\s*[\(\[][^)\]]*(clip officiel|clip|official|video|audio|lyrics?|paroles?|visualizer|remix|remaster|version|hd|4k)[^)\]]*[\)\]]`)
var lrclibTextTokenRe = regexp.MustCompile(`[^\p{L}\p{N}]+`)
var lrclibAlbumYearSuffixRe = regexp.MustCompile(`\s*\(\d{4}\).*$`)
var lrclibGenericArtistHints = map[string]bool{
	"artiste inconnu": true,
	"source inconnue": true,
	"unknown artist":  true,
	"unknown source":  true,
	"playlist":        true,
	"playlists":       true,
}

func sanitizeLRCLIBArtistHint(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	normalized := normalizeLRCLIBText(trimmed)
	if normalized == "" {
		return ""
	}
	if lrclibGenericArtistHints[normalized] {
		return ""
	}
	return trimmed
}

func sanitizeLRCLIBTrackHint(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	cleaned := strings.TrimSpace(stripLeadingTrackNumber(trimmed))
	if cleaned == "" {
		return trimmed
	}
	return cleaned
}

func sanitizeLRCLIBAlbumHint(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	cleaned := strings.TrimSpace(lrclibAlbumYearSuffixRe.ReplaceAllString(trimmed, ""))
	if cleaned == "" {
		return trimmed
	}
	return cleaned
}

func normalizeLRCLIBTrackCandidates(track string) []string {
	raw := strings.TrimSpace(track)
	if raw == "" {
		return nil
	}
	values := []string{}
	addValue := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		values = append(values, value)
		stripped := stripLeadingTrackNumber(value)
		if stripped != "" && !strings.EqualFold(stripped, value) {
			values = append(values, stripped)
		}
	}

	addValue(raw)

	withoutID := strings.TrimSpace(youtubeIDSuffixRe.ReplaceAllString(raw, ""))
	if withoutID != "" {
		addValue(withoutID)
	}
	withoutBracketed := strings.TrimSpace(lrclibBracketChunkRe.ReplaceAllString(withoutID, " "))
	withoutBracketed = strings.Join(strings.Fields(withoutBracketed), " ")
	if withoutBracketed != "" {
		addValue(withoutBracketed)
	}
	withoutNoise := strings.TrimSpace(lrclibTitleNoiseRe.ReplaceAllString(withoutBracketed, " "))
	withoutNoise = strings.Join(strings.Fields(withoutNoise), " ")
	if withoutNoise != "" {
		addValue(withoutNoise)
	}
	artist, title, ok := splitArtistAndTitle(withoutNoise)
	if !ok {
		artist, title, ok = splitArtistAndTitle(withoutBracketed)
	}
	if ok {
		addValue(title)
		addValue(artist + " - " + title)
	}

	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}

func stripLeadingTrackNumber(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	out := strings.TrimSpace(lrclibLeadingTrackNumberRe.ReplaceAllString(value, ""))
	if out == value {
		return value
	}
	return out
}

func splitArtistAndTitle(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	for _, separator := range []string{" - ", " – ", " — ", " | ", " : "} {
		parts := strings.SplitN(value, separator, 2)
		if len(parts) != 2 {
			continue
		}
		artist := strings.TrimSpace(parts[0])
		title := strings.TrimSpace(parts[1])
		if artist == "" || title == "" {
			continue
		}
		return artist, title, true
	}
	return "", "", false
}

func normalizeLRCLIBText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = foldLRCLIBAccents(value)
	value = lrclibTextTokenRe.ReplaceAllString(value, " ")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}

func foldLRCLIBAccents(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'à', 'á', 'â', 'ä', 'ã', 'å', 'ā', 'ă', 'ą':
			return 'a'
		case 'ç', 'ć', 'ĉ', 'ċ', 'č':
			return 'c'
		case 'ď', 'đ':
			return 'd'
		case 'è', 'é', 'ê', 'ë', 'ē', 'ĕ', 'ė', 'ę', 'ě':
			return 'e'
		case 'ĝ', 'ğ', 'ġ', 'ģ':
			return 'g'
		case 'ĥ', 'ħ':
			return 'h'
		case 'ì', 'í', 'î', 'ï', 'ĩ', 'ī', 'ĭ', 'į', 'ı':
			return 'i'
		case 'ĵ':
			return 'j'
		case 'ķ':
			return 'k'
		case 'ĺ', 'ļ', 'ľ', 'ŀ', 'ł':
			return 'l'
		case 'ñ', 'ń', 'ņ', 'ň', 'ŉ':
			return 'n'
		case 'ò', 'ó', 'ô', 'ö', 'õ', 'ø', 'ō', 'ŏ', 'ő':
			return 'o'
		case 'ŕ', 'ŗ', 'ř':
			return 'r'
		case 'ś', 'ŝ', 'ş', 'š':
			return 's'
		case 'ţ', 'ť', 'ŧ':
			return 't'
		case 'ù', 'ú', 'û', 'ü', 'ũ', 'ū', 'ŭ', 'ů', 'ű', 'ų':
			return 'u'
		case 'ŵ':
			return 'w'
		case 'ý', 'ÿ', 'ŷ':
			return 'y'
		case 'ź', 'ż', 'ž':
			return 'z'
		default:
			return r
		}
	}, value)
}

func isRetryableLRCLIBError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var httpErr lrclibHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.statusCode == http.StatusTooManyRequests || httpErr.statusCode >= http.StatusInternalServerError
	}
	return false
}

func discoverAudioFiles(dir string) []string {
	list := []string{}
	extSet := map[string]bool{".mp3": true, ".flac": true, ".m4a": true, ".aac": true, ".ogg": true, ".opus": true, ".wav": true, ".webm": true}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if extSet[ext] {
			list = append(list, path)
		}
		return nil
	})
	sort.Strings(list)
	return list
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
