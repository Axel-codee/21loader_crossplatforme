package jobs

import (
	"context"
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

	"persodl-cross/internal/core"
	"persodl-cross/internal/sys"
	"persodl-cross/internal/util"
)

type Runner struct {
	processRunner *sys.Runner
	organizer     *Organizer
	httpClient    *http.Client
	paths         util.AppPaths
	argosScript   string
}

type RunCallbacks struct {
	OnStep         func(core.JobStep)
	OnStepProgress func(float64)
	OnStepCount    func(int, int)
	OnLog          func(string)
	OnDisplayName  func(string)
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
	MediaPath       string
	SubtitlePath    string
	TranscriptPath  string
	MetadataPath    string
	Title           string
	SourceName      string
	PublicationDate *time.Time
}

type translationVariantArtifacts struct {
	SourceLanguage           string
	TargetLanguage           string
	OriginalSubtitlePath     string
	OriginalTranscriptPath   string
	TranslatedSubtitlePath   string
	TranslatedTranscriptPath string
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
		processRunner: proc,
		organizer:     organizer,
		httpClient:    &http.Client{Timeout: 25 * time.Second},
		paths:         paths,
		argosScript:   filepath.Join(baseDir, "assets", "scripts", "argos_translate_file.py"),
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
	} else if job.SourceKind == core.SourceQobuz && opt.QobuzExistingAlbumCollision == core.CollisionFetchMissingLyrics {
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
		r.fetchLyricsFromLRCLIBWithHints(ctx, artifact.MediaPath, artifact.Title, artifact.SourceName, cb)
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
			if subtitleFile != "" || transcriptFile != "" {
				if cb.OnLog != nil {
					cb.OnLog("[transcription] Mode completer: transcription existante detectee, etape ignoree.\n")
				}
			} else {
				subtitleFile, transcriptFile, err = r.transcribe(ctx, artifact.MediaPath, workspace, job, cb)
				if err != nil {
					return core.JobResult{}, err
				}
			}
		} else {
			subtitleFile, transcriptFile, err = r.transcribe(ctx, artifact.MediaPath, workspace, job, cb)
			if err != nil {
				return core.JobResult{}, err
			}
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
			if job.ContentType == core.ContentMusic {
				cb.OnLog("[transcription] Etape ignoree (musique).\n")
			} else {
				cb.OnLog("[transcription] Etape ignoree (desactivee).\n")
			}
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
		SourceKind:            job.SourceKind,
		SourceName:            artifact.SourceName,
		Title:                 artifact.Title,
		PublicationDate:       artifact.PublicationDate,
		OriginalInputURL:      job.InputURL,
		MediaPath:             artifact.MediaPath,
		IsMediaDirectory:      artifact.IsDirectory,
		SubtitleFile:          subtitleFile,
		TranscriptFile:        transcriptFile,
		ArtworkFile:           artifact.ArtworkPath,
		CustomName:            job.CustomName,
		OutputRoot:            outputRoot,
		TranscriptionLanguage: job.TranscriptionLanguage,
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
		return r.downloadWithYtDlp(ctx, job.InputURL, workspace, job.ContentType, job.UseFirefoxCookies, job.YtDlpExtraArguments, "", "", nil, cb)
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
		artifact, err = r.downloadWithYtDlp(ctx, selection.MediaURL, workspace, core.ContentAudio, job.UseFirefoxCookies, job.YtDlpExtraArguments, selection.PodcastTitle, selection.Title, selection.PublicationDate, cb)
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

func (r *Runner) downloadWithYtDlp(
	ctx context.Context,
	sourceURL, workspace string,
	mode core.JobContentType,
	useFirefoxCookies bool,
	extraArguments, forcedSourceName, forcedTitle string,
	forcedDate *time.Time,
	cb RunCallbacks,
) (downloadArtifact, error) {
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
		baseArgs = append(baseArgs, "-f", "bestaudio/b")
	}
	extraArgsList := util.ParseArgumentString(extraArguments)
	cookiesConfiguredInArgs := hasYtDlpCookieArgs(extraArgsList)
	args := append([]string{}, baseArgs...)
	if useFirefoxCookies {
		args = append(args, "--cookies-from-browser", "firefox")
	}
	args = append(args, extraArgsList...)
	args = append(args, "--no-quiet", "--progress", "--newline", sourceURL)
	if cb.OnLog != nil {
		cb.OnLog("[download] Demarrage du telechargement YouTube...\n")
	}

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
				return downloadArtifact{}, fmt.Errorf("%w.%s", adapted, attemptDetails)
			}
			return downloadArtifact{}, adapted
		}
	}
	if err != nil {
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
	return r.processRunner.Run(ctx, sys.RunOptions{
		Executable: "yt-dlp",
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
	if !ok || rt != util.QobuzAlbum {
		if rt == util.QobuzArtist {
			return downloadArtifact{}, fmt.Errorf("les URL artiste Qobuz necessitent de selectionner des albums dans l'ecran Nouveau job")
		}
		return downloadArtifact{}, fmt.Errorf("URL Qobuz invalide ou non supportee")
	}
	if err := r.ensureQobuzConfigured(ctx, job.QobuzEmail, job.QobuzPassword, cb); err != nil {
		return downloadArtifact{}, err
	}

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

	_, err := r.processRunner.Run(ctx, sys.RunOptions{
		Executable: "qobuz-dl",
		Args:       args,
		WorkingDir: workspace,
		OnOutput: func(line string) {
			if cb.OnLog != nil {
				cb.OnLog(line)
			}
			if cb.OnStepProgress != nil {
				if pct := parsePercentProgress(line); pct >= 0 {
					cb.OnStepProgress(minFloat(0.95, pct/100.0))
				}
			}
		},
		CaptureOutput: false,
	})
	if err != nil {
		return downloadArtifact{}, err
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0.99)
	}

	albumDir := discoverLatestDirectory(downloadRoot)
	if albumDir == "" {
		return downloadArtifact{}, fmt.Errorf("telechargement Qobuz termine mais dossier d'album introuvable")
	}
	meta := r.readQobuzFolderMetadata(albumDir, artistOverride)
	return downloadArtifact{MediaPath: albumDir, Title: meta.albumTitle, SourceName: meta.artistName, IsDirectory: true}, nil
}

func (r *Runner) ensureQobuzConfigured(ctx context.Context, email, password string, cb RunCallbacks) error {
	configFile := qobuzConfigPath()
	if _, err := os.Stat(configFile); err == nil {
		return nil
	}
	email = strings.TrimSpace(email)
	password = strings.TrimSpace(password)
	if email == "" || password == "" {
		return fmt.Errorf("qobuz-dl n'est pas configure. Renseigne email/mot de passe Qobuz dans Reglages")
	}
	if cb.OnLog != nil {
		cb.OnLog("[qobuz] Initialisation de la configuration qobuz-dl...\n")
	}
	stdin := email + "\n" + password + "\n\n27\n"
	_, err := r.processRunner.Run(ctx, sys.RunOptions{
		Executable:    "qobuz-dl",
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
	if _, err := os.Stat(configFile); err != nil {
		return fmt.Errorf("impossible d'initialiser la configuration qobuz-dl")
	}
	return nil
}

func qobuzConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "qobuz-dl", "config.ini")
}

func (r *Runner) transcribe(ctx context.Context, mediaPath, workspace string, job core.JobRequest, cb RunCallbacks) (string, string, error) {
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
		return "", "", err
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0.15)
	}

	whisperExec := "whisper-cli"
	if resolved, _, resolveErr := util.ResolveToolExecutable("whisper-cli"); resolveErr == nil {
		whisperExec = resolved
	}

	whisperModelPath := resolveWhisperModelPath(job.WhisperModelPath, whisperExec)
	if whisperModelPath == "" {
		return "", "", fmt.Errorf("chemin du modele Whisper invalide. Configure-le dans Reglages")
	}
	if cb.OnLog != nil && whisperModelPath != strings.TrimSpace(job.WhisperModelPath) {
		cb.OnLog("[transcription] Modele Whisper detecte: " + whisperModelPath + "\n")
	}

	outputBase := filepath.Join(workspace, "transcription")
	whisperArgs := []string{"-m", whisperModelPath, "-f", wav, "-osrt", "-otxt", "-of", outputBase}
	lang := strings.TrimSpace(job.TranscriptionLanguage)
	if lang == "" {
		lang = "auto"
	}
	whisperArgs = append(whisperArgs, "-l", lang)
	whisperArgs = append(whisperArgs, util.ParseArgumentString(job.WhisperExtraArguments)...)
	if cb.OnLog != nil && whisperExec != "whisper-cli" {
		cb.OnLog("[transcription] Executable Whisper detecte: " + whisperExec + "\n")
	}

	_, err = r.processRunner.Run(ctx, sys.RunOptions{
		Executable:    whisperExec,
		Args:          whisperArgs,
		WorkingDir:    workspace,
		CaptureOutput: false,
		OnOutput: func(line string) {
			if cb.OnLog != nil {
				cb.OnLog(line)
			}
			if cb.OnStepProgress != nil {
				if pct := parsePercentProgress(line); pct >= 0 {
					cb.OnStepProgress(minFloat(0.9, 0.15+(pct/100.0)*0.75))
				}
			}
		},
	})
	if err != nil {
		return "", "", err
	}
	subtitle := filepath.Join(workspace, "transcription.srt")
	transcript := filepath.Join(workspace, "transcription.txt")
	if _, err := os.Stat(subtitle); err != nil {
		return "", "", fmt.Errorf("Whisper n'a pas genere les fichiers .txt/.srt attendus")
	}
	if _, err := os.Stat(transcript); err != nil {
		return "", "", fmt.Errorf("Whisper n'a pas genere les fichiers .txt/.srt attendus")
	}
	if cb.OnStepProgress != nil {
		cb.OnStepProgress(0.9)
	}
	return subtitle, transcript, nil
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
	return job.ContentType != core.ContentMusic && job.EnableTranscription
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
			MediaPath:       mediaPath,
			SubtitlePath:    firstExistingFile(strings.TrimSpace(meta.SubtitlePath), sidecarPathForMedia(mediaPath, "", ".srt")),
			TranscriptPath:  firstExistingFile(strings.TrimSpace(meta.TranscriptPath), sidecarPathForMedia(mediaPath, "", ".txt")),
			MetadataPath:    path,
			Title:           strings.TrimSpace(meta.Title),
			SourceName:      strings.TrimSpace(meta.SourceName),
			PublicationDate: publicationDate,
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
			MediaPath:       mediaPath,
			SubtitlePath:    firstExistingFile(strings.TrimSpace(meta.SubtitlePath), sidecarPathForMedia(mediaPath, "", ".srt")),
			TranscriptPath:  firstExistingFile(strings.TrimSpace(meta.TranscriptPath), sidecarPathForMedia(mediaPath, "", ".txt")),
			MetadataPath:    path,
			Title:           strings.TrimSpace(meta.Title),
			SourceName:      strings.TrimSpace(meta.SourceName),
			PublicationDate: publicationDate,
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
	r.fetchLyricsFromLRCLIBWithHints(ctx, albumDir, "", "", cb)
}

func (r *Runner) fetchLyricsFromLRCLIBWithHints(ctx context.Context, albumDir, titleHint, artistHint string, cb RunCallbacks) {
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
		if cb.OnLog != nil {
			cb.OnLog(fmt.Sprintf("[lyrics] Recherche %d/%d: %s\n", idx+1, len(audioFiles), track))
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
		searchTrack := track
		searchArtist := strings.TrimSpace(artistHint)
		searchAlbum := ""
		if len(audioFiles) == 1 {
			if t := strings.TrimSpace(titleHint); t != "" {
				searchTrack = t
			}
		} else if a := strings.TrimSpace(titleHint); a != "" {
			// For multi-track jobs, the title hint is usually the album title.
			searchAlbum = a
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
		return lrclibPayload{}, -1, nil
	}

	u := "https://lrclib.net/api/search?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return lrclibPayload{}, -1, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return lrclibPayload{}, -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return lrclibPayload{}, -1, lrclibHTTPError{statusCode: resp.StatusCode}
	}
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return lrclibPayload{}, -1, err
	}
	payload, score := pickBestLRCLIBPayload(items, scoreSearch)
	return payload, score, nil
}

func pickBestLRCLIBPayload(items []map[string]any, search lrclibSearchQuery) (lrclibPayload, int) {
	bestScore := -1
	bestPayload := lrclibPayload{}
	bestArtistScore := -1
	bestArtistPayload := lrclibPayload{}
	searchTrack := normalizeLRCLIBText(search.trackName)
	searchArtist := normalizeLRCLIBText(search.artistName)
	searchAlbum := normalizeLRCLIBText(search.albumName)
	searchQuery := normalizeLRCLIBText(search.query)

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

		score := 0
		if payload.syncedLyrics != "" {
			score += 50
		} else {
			score += 10
		}

		itemTrack := normalizeLRCLIBText(firstNonEmptyValue(item["trackName"], item["track_name"], item["name"]))
		itemArtist := normalizeLRCLIBText(firstNonEmptyValue(item["artistName"], item["artist_name"], item["artist"]))
		itemAlbum := normalizeLRCLIBText(firstNonEmptyValue(item["albumName"], item["album_name"], item["album"]))
		trackScore := partialMatchScore(searchTrack, itemTrack, 40, 20)
		artistScore := partialMatchScore(searchArtist, itemArtist, 25, 12)
		albumScore := partialMatchScore(searchAlbum, itemAlbum, 12, 6)
		score += trackScore + artistScore
		score += albumScore

		// Avoid selecting unrelated songs when a source artist is known.
		if searchArtist != "" && itemArtist != "" && artistScore == 0 {
			score -= 25
		}
		if searchArtist != "" && itemArtist == "" {
			score -= 20
		}
		if searchTrack != "" && itemTrack != "" && trackScore == 0 {
			score -= 15
		}
		if searchAlbum != "" && itemAlbum != "" && albumScore == 0 {
			score -= 6
		}
		if searchQuery != "" && (searchTrack == "" || searchArtist == "") {
			haystack := strings.TrimSpace(itemArtist + " " + itemTrack)
			if haystack == searchQuery {
				score += 8
			} else if strings.Contains(haystack, searchQuery) || strings.Contains(searchQuery, haystack) {
				score += 4
			}
		}

		if score > bestScore {
			bestScore = score
			bestPayload = payload
		}
		if searchArtist != "" && artistScore > 0 {
			if searchTrack != "" && trackScore == 0 {
				// Artist match without track overlap is too risky for plain-text lyrics.
				continue
			}
			if score > bestArtistScore {
				bestArtistScore = score
				bestArtistPayload = payload
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

func buildLRCLIBSearchQueries(track, artistHint, albumHint string) []lrclibSearchQuery {
	raw := strings.TrimSpace(track)
	if raw == "" {
		return nil
	}
	artistHint = strings.TrimSpace(artistHint)
	albumHint = strings.TrimSpace(albumHint)
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
