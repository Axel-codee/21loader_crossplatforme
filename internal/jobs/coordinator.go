package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"21loader-cross/internal/core"
	"21loader-cross/internal/services"
	"21loader-cross/internal/util"
	"21loader-cross/internal/xuuid"
)

type JobExecutionOptions struct {
	StandardCollision           core.CollisionDecision
	QobuzExistingAlbumCollision core.CollisionDecision
}

type builtJob struct {
	Request     core.JobRequest
	Options     JobExecutionOptions
	DisplayName string
}

type Coordinator struct {
	mu sync.Mutex

	paths util.AppPaths

	settings     core.WebSettings
	jobs         []core.JobRecord
	activeJobID  xuuid.UUID
	activeHas    bool
	activeCancel context.CancelFunc

	optionsByJobID     map[xuuid.UUID]JobExecutionOptions
	displayNameByJobID map[xuuid.UUID]string

	runner      *Runner
	diagnostics *services.DiagnosticsService
	translation *services.TranslationLanguageService
	whisper     *services.WhisperModelService
	qobuz       *services.QobuzService
	rss         *services.RSSService
	youtube     *services.YouTubeService

	maxLogCharacters int
}

var qobuzTmpTrackIndexRe = regexp.MustCompile(`\.(\d+)\.tmp\b`)
var lyricsSummaryRe = regexp.MustCompile(`\[lyrics\]\s+Termine:\s+(\d+)\s+genere\(s\),\s+(\d+)\s+deja present\(s\),\s+(\d+)\s+erreur\(s\)\.`)
var lyricsGeneratedLineRe = regexp.MustCompile(`(?m)^\[lyrics\]\s+(Sous-titres synchronises generes\.|Lyrics texte generes\.)\s*$`)
var lyricsAlreadyPresentLineRe = regexp.MustCompile(`(?m)^\[lyrics\]\s+Deja present, piste ignoree\.\s*$`)
var lyricsFailureLineRe = regexp.MustCompile(`(?m)^\[lyrics\]\s+Echec\s+.*$`)

func NewCoordinator(paths util.AppPaths, runner *Runner, diagnostics *services.DiagnosticsService, translation *services.TranslationLanguageService, whisper *services.WhisperModelService, qobuz *services.QobuzService, rss *services.RSSService, youtube *services.YouTubeService) (*Coordinator, error) {
	if err := paths.Ensure(); err != nil {
		return nil, err
	}
	c := &Coordinator{
		paths:              paths,
		runner:             runner,
		diagnostics:        diagnostics,
		translation:        translation,
		whisper:            whisper,
		qobuz:              qobuz,
		rss:                rss,
		youtube:            youtube,
		optionsByJobID:     map[xuuid.UUID]JobExecutionOptions{},
		displayNameByJobID: map[xuuid.UUID]string{},
		maxLogCharacters:   160000,
	}
	c.settings = c.loadSettings()
	return c, nil
}

func (c *Coordinator) Dashboard() core.DashboardResponseDTO {
	c.mu.Lock()
	defer c.mu.Unlock()
	jobs := c.sortedJobsLocked()
	active := ""
	if c.activeHas {
		active = c.activeJobID.String()
	}
	return core.DashboardResponseDTO{
		ServerTime:  time.Now().UTC(),
		ActiveJobID: active,
		Settings:    c.settings,
		Jobs:        c.mapDTOsLocked(jobs),
	}
}

func (c *Coordinator) ListJobs() []core.JobSummaryDTO {
	c.mu.Lock()
	defer c.mu.Unlock()
	jobs := c.sortedJobsLocked()
	return c.mapDTOsLocked(jobs)
}

func (c *Coordinator) Job(id xuuid.UUID) *core.JobSummaryDTO {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.jobs {
		if c.jobs[i].ID == id {
			d := c.dtoLocked(&c.jobs[i])
			return &d
		}
	}
	return nil
}

func (c *Coordinator) Logs(id xuuid.UUID) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.jobs {
		if c.jobs[i].ID == id {
			return c.jobs[i].Logs, true
		}
	}
	return "", false
}

func (c *Coordinator) CurrentSettings() core.WebSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.settings
}

func (c *Coordinator) Diagnostics(ctx context.Context) core.WebDiagnosticsReport {
	return c.diagnostics.CollectReport(ctx)
}

func (c *Coordinator) InstallDependencies(ctx context.Context, payload core.DependencyInstallRequest) (core.DependencyInstallResponse, error) {
	return c.diagnostics.InstallDependencies(ctx, payload.Tools)
}

func (c *Coordinator) DependencyInstallProgress(ctx context.Context) core.DependencyInstallProgressResponse {
	_ = ctx
	return c.diagnostics.InstallProgress()
}

func (c *Coordinator) TranslationLanguages(ctx context.Context) (core.TranslationLanguageCatalogResponse, error) {
	if c.translation == nil {
		return core.TranslationLanguageCatalogResponse{
			RuntimeAvailable: false,
			RuntimeMessage:   "service Argos langues indisponible",
			Languages:        []core.TranslationLanguageInfoDTO{},
			Pairs:            []core.TranslationLanguagePairDTO{},
		}, nil
	}
	return c.translation.Catalog(ctx)
}

func (c *Coordinator) InstallTranslationLanguage(ctx context.Context, payload core.TranslationLanguageInstallRequest) (core.TranslationLanguageInstallResponse, error) {
	if c.translation == nil {
		return core.TranslationLanguageInstallResponse{}, fmt.Errorf("service Argos langues indisponible")
	}
	return c.translation.InstallPair(ctx, payload.SourceCode, payload.TargetCode)
}

func (c *Coordinator) WhisperModels(ctx context.Context) (core.WhisperModelsResponse, error) {
	c.mu.Lock()
	selectedPath := strings.TrimSpace(c.settings.WhisperModelPath)
	c.mu.Unlock()

	if c.whisper == nil {
		return core.WhisperModelsResponse{}, fmt.Errorf("service Whisper indisponible")
	}
	_ = ctx
	return c.whisper.ListModels(selectedPath)
}

func (c *Coordinator) WhisperModelInstallProgress(ctx context.Context, payload core.WhisperModelInstallProgressRequest) (core.WhisperModelInstallProgressResponse, error) {
	_ = ctx
	if c.whisper == nil {
		return core.WhisperModelInstallProgressResponse{}, fmt.Errorf("service Whisper indisponible")
	}
	return c.whisper.InstallProgress(payload.ModelID), nil
}

func (c *Coordinator) InstallWhisperModel(ctx context.Context, payload core.WhisperModelInstallRequest) (core.WhisperModelInstallResponse, error) {
	if c.whisper == nil {
		return core.WhisperModelInstallResponse{}, fmt.Errorf("service Whisper indisponible")
	}
	model, message, err := c.whisper.InstallModel(ctx, payload.ModelID)
	if err != nil {
		return core.WhisperModelInstallResponse{}, err
	}
	return core.WhisperModelInstallResponse{
		OK:      true,
		Message: message,
		Model:   model,
	}, nil
}

func (c *Coordinator) UninstallWhisperModel(ctx context.Context, payload core.WhisperModelUninstallRequest) (core.WhisperModelUninstallResponse, error) {
	if c.whisper == nil {
		return core.WhisperModelUninstallResponse{}, fmt.Errorf("service Whisper indisponible")
	}

	model, message, removedPath, err := c.whisper.UninstallModel(ctx, payload.ModelID)
	if err != nil {
		return core.WhisperModelUninstallResponse{}, err
	}

	clearedDefaultSelection := false
	if strings.TrimSpace(removedPath) != "" {
		c.mu.Lock()
		if sameFilePath(c.settings.WhisperModelPath, removedPath) {
			c.settings.WhisperModelPath = ""
			c.persistSettingsLocked()
			clearedDefaultSelection = true
		}
		c.mu.Unlock()
	}

	return core.WhisperModelUninstallResponse{
		OK:                      true,
		Message:                 message,
		Model:                   model,
		RemovedPath:             removedPath,
		ClearedDefaultSelection: clearedDefaultSelection,
	}, nil
}

func (c *Coordinator) FetchQobuzArtistCatalog(ctx context.Context, payload core.QobuzArtistCatalogAPIRequest) (core.QobuzArtistCatalogAPIResponse, error) {
	artistURL := strings.TrimSpace(payload.ArtistURL)
	if artistURL == "" {
		return core.QobuzArtistCatalogAPIResponse{}, fmt.Errorf("le champ artistURL est requis")
	}
	c.mu.Lock()
	email := fallbackTrimmed(payload.QobuzEmail, c.settings.QobuzEmail)
	password := fallbackRaw(payload.QobuzPassword, c.settings.QobuzPassword)
	c.mu.Unlock()
	return c.qobuz.FetchArtistCatalog(ctx, artistURL, email, password)
}

func (c *Coordinator) SearchQobuzArtists(ctx context.Context, payload core.QobuzArtistSearchAPIRequest) (core.QobuzArtistSearchAPIResponse, error) {
	query := strings.TrimSpace(payload.Query)
	if query == "" {
		return core.QobuzArtistSearchAPIResponse{}, fmt.Errorf("le champ query est requis")
	}
	limit := payload.Limit
	if limit <= 0 {
		limit = 12
	}
	c.mu.Lock()
	email := fallbackTrimmed(payload.QobuzEmail, c.settings.QobuzEmail)
	password := fallbackRaw(payload.QobuzPassword, c.settings.QobuzPassword)
	c.mu.Unlock()
	return c.qobuz.SearchArtists(ctx, query, limit, email, password)
}

func (c *Coordinator) FetchQobuzAlbumTracks(ctx context.Context, payload core.QobuzAlbumTracksAPIRequest) (core.QobuzAlbumTracksAPIResponse, error) {
	albumID := strings.TrimSpace(payload.AlbumID)
	if albumID == "" {
		return core.QobuzAlbumTracksAPIResponse{}, fmt.Errorf("le champ albumID est requis")
	}
	c.mu.Lock()
	email := fallbackTrimmed(payload.QobuzEmail, c.settings.QobuzEmail)
	password := fallbackRaw(payload.QobuzPassword, c.settings.QobuzPassword)
	c.mu.Unlock()
	return c.qobuz.FetchAlbumTracks(ctx, albumID, email, password)
}

func (c *Coordinator) FetchQobuzPlaylistCatalog(ctx context.Context, payload core.QobuzPlaylistCatalogAPIRequest) (core.QobuzPlaylistCatalogAPIResponse, error) {
	playlistURL := strings.TrimSpace(payload.PlaylistURL)
	if playlistURL == "" {
		return core.QobuzPlaylistCatalogAPIResponse{}, fmt.Errorf("le champ playlistURL est requis")
	}
	c.mu.Lock()
	email := fallbackTrimmed(payload.QobuzEmail, c.settings.QobuzEmail)
	password := fallbackRaw(payload.QobuzPassword, c.settings.QobuzPassword)
	c.mu.Unlock()
	return c.qobuz.FetchPlaylistCatalog(ctx, playlistURL, email, password)
}

func (c *Coordinator) FetchRSSEpisodes(ctx context.Context, payload core.RSSFeedEpisodesAPIRequest) (core.RSSFeedEpisodesAPIResponse, error) {
	feedURL := strings.TrimSpace(payload.FeedURL)
	if feedURL == "" {
		return core.RSSFeedEpisodesAPIResponse{}, fmt.Errorf("le champ feedURL est requis")
	}
	return c.rss.FetchFeed(ctx, feedURL)
}

func (c *Coordinator) FetchYouTubeCatalog(ctx context.Context, payload core.YouTubeCatalogAPIRequest) (core.YouTubeCatalogAPIResponse, error) {
	url := strings.TrimSpace(payload.URL)
	if url == "" {
		return core.YouTubeCatalogAPIResponse{}, fmt.Errorf("le champ url est requis")
	}
	c.mu.Lock()
	useFirefox := c.settings.UseFirefoxCookies
	c.mu.Unlock()
	if payload.UseFirefoxCookies != nil {
		useFirefox = *payload.UseFirefoxCookies
	}
	return c.youtube.FetchCatalog(ctx, url, useFirefox)
}

func (c *Coordinator) FetchYouTubeDates(ctx context.Context, payload core.YouTubeDatesAPIRequest) core.YouTubeDatesAPIResponse {
	c.mu.Lock()
	useFirefox := c.settings.UseFirefoxCookies
	c.mu.Unlock()
	if payload.UseFirefoxCookies != nil {
		useFirefox = *payload.UseFirefoxCookies
	}
	dates, durations := c.youtube.ResolveDates(ctx, payload.VideoIDs, useFirefox)
	return core.YouTubeDatesAPIResponse{
		DatesByVideoID:     dates,
		DurationsByVideoID: durations,
	}
}

func (c *Coordinator) UpdateSettings(payload core.UpdateSettingsAPIRequest) core.WebSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	if payload.WhisperModelPath != nil {
		c.settings.WhisperModelPath = strings.TrimSpace(*payload.WhisperModelPath)
	}
	if payload.UseFirefoxCookies != nil {
		c.settings.UseFirefoxCookies = *payload.UseFirefoxCookies
	}
	if payload.KeepTemporaryFilesOnFailure != nil {
		c.settings.KeepTemporaryFilesOnFailure = *payload.KeepTemporaryFilesOnFailure
	}
	if payload.QobuzEmail != nil {
		c.settings.QobuzEmail = strings.TrimSpace(*payload.QobuzEmail)
	}
	if payload.QobuzPassword != nil {
		c.settings.QobuzPassword = *payload.QobuzPassword
	}
	if payload.DefaultOutputRoot != nil {
		if root := strings.TrimSpace(*payload.DefaultOutputRoot); root != "" {
			c.settings.DefaultOutputRoot = root
		}
	}
	c.persistSettingsLocked()
	return c.settings
}

func (c *Coordinator) Enqueue(ctx context.Context, payload core.CreateJobAPIRequest) (core.JobSummaryDTO, error) {
	built, err := c.buildJob(payload)
	if err != nil {
		return core.JobSummaryDTO{}, err
	}
	built.DisplayName = c.resolveEnqueueDisplayName(ctx, built)

	c.mu.Lock()
	c.optionsByJobID[built.Request.ID] = built.Options
	record := core.NewJobRecord(built.Request)
	c.jobs = append(c.jobs, record)
	if strings.TrimSpace(built.DisplayName) != "" {
		c.displayNameByJobID[built.Request.ID] = strings.TrimSpace(built.DisplayName)
	}
	c.appendLogLocked(built.Request.ID, "[job] Ajoute depuis l'interface web.\n")
	summary := c.dtoLocked(&c.jobs[len(c.jobs)-1])
	c.mu.Unlock()

	c.startNextJobIfNeeded()
	return summary, nil
}

func (c *Coordinator) resolveEnqueueDisplayName(ctx context.Context, built builtJob) string {
	displayName := strings.TrimSpace(built.DisplayName)
	if displayName != "" {
		return displayName
	}
	if built.Request.SourceKind != core.SourceYouTube {
		return ""
	}
	if c.youtube == nil {
		return ""
	}
	if !util.LooksLikeYouTubeURL(built.Request.InputURL) {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	title, err := c.youtube.ResolveVideoTitle(resolveCtx, built.Request.InputURL, built.Request.UseFirefoxCookies)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(title)
}

func (c *Coordinator) Cancel(jobID xuuid.UUID) core.ActionResponseDTO {
	c.mu.Lock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: false, Message: "Job introuvable."}
	}

	if c.activeHas && c.activeJobID == jobID {
		c.runner.Cancel()
		if c.activeCancel != nil {
			c.activeCancel()
		}
		now := time.Now().UTC()
		rec := &c.jobs[idx]
		c.accumulateCurrentStepElapsedLocked(rec, now)
		rec.Status = core.StatusCancelled
		rec.EndedAt = &now
		rec.CurrentStep = nil
		rec.CurrentStepStartedAt = nil
		c.finishTranslationLocked(rec, now, "cancelled")
		rec.CurrentStepProgress = 0
		rec.ErrorMessage = "Annule"
		c.appendLogLocked(jobID, "[job] Annulation demandee.\n")
		d := c.dtoLocked(rec)
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: true, Message: "Annulation demandee.", Job: &d}
	}

	rec := &c.jobs[idx]
	if rec.Status == core.StatusQueued {
		now := time.Now().UTC()
		c.accumulateCurrentStepElapsedLocked(rec, now)
		rec.Status = core.StatusCancelled
		rec.EndedAt = &now
		rec.CurrentStep = nil
		rec.CurrentStepStartedAt = nil
		c.finishTranslationLocked(rec, now, "cancelled")
		rec.CurrentStepProgress = 0
		rec.ErrorMessage = "Annule"
		c.appendLogLocked(jobID, "[job] Annule avant execution.\n")
		d := c.dtoLocked(rec)
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: true, Message: "Job annule.", Job: &d}
	}
	msg := "Ce job ne peut pas etre annule dans son etat actuel."
	d := c.dtoLocked(rec)
	c.mu.Unlock()
	return core.ActionResponseDTO{OK: false, Message: msg, Job: &d}
}

func (c *Coordinator) Pause(jobID xuuid.UUID) core.ActionResponseDTO {
	c.mu.Lock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: false, Message: "Job introuvable."}
	}
	rec := &c.jobs[idx]
	if !(c.activeHas && c.activeJobID == jobID) {
		d := c.dtoLocked(rec)
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: false, Message: "Seul le job actif peut etre mis en pause.", Job: &d}
	}
	if rec.Status != core.StatusRunning {
		d := c.dtoLocked(rec)
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: false, Message: "Le job n'est pas en cours.", Job: &d}
	}
	c.mu.Unlock()

	if c.runner.Pause() {
		c.mu.Lock()
		rec := &c.jobs[idx]
		rec.Status = core.StatusPaused
		rec.IsPauseRequested = true
		c.appendLogLocked(jobID, "[job] Pause demandee.\n")
		d := c.dtoLocked(rec)
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: true, Message: "Pause demandee.", Job: &d}
	}

	c.mu.Lock()
	d := c.dtoLocked(&c.jobs[idx])
	c.mu.Unlock()
	return core.ActionResponseDTO{OK: false, Message: "Pause impossible sur l'etape en cours.", Job: &d}
}

func (c *Coordinator) Resume(jobID xuuid.UUID) core.ActionResponseDTO {
	c.mu.Lock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: false, Message: "Job introuvable."}
	}
	rec := &c.jobs[idx]
	if !(c.activeHas && c.activeJobID == jobID) {
		d := c.dtoLocked(rec)
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: false, Message: "Seul le job actif peut etre repris.", Job: &d}
	}
	if rec.Status != core.StatusPaused {
		d := c.dtoLocked(rec)
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: false, Message: "Le job n'est pas en pause.", Job: &d}
	}
	c.mu.Unlock()

	if c.runner.Resume() {
		c.mu.Lock()
		rec := &c.jobs[idx]
		rec.Status = core.StatusRunning
		rec.IsPauseRequested = false
		c.appendLogLocked(jobID, "[job] Reprise demandee.\n")
		d := c.dtoLocked(rec)
		c.mu.Unlock()
		return core.ActionResponseDTO{OK: true, Message: "Reprise demandee.", Job: &d}
	}

	c.mu.Lock()
	d := c.dtoLocked(&c.jobs[idx])
	c.mu.Unlock()
	return core.ActionResponseDTO{OK: false, Message: "Reprise impossible sur l'etape en cours.", Job: &d}
}

func (c *Coordinator) startNextJobIfNeeded() {
	c.mu.Lock()
	if c.activeHas {
		c.mu.Unlock()
		return
	}
	next := -1
	for i := range c.jobs {
		if c.jobs[i].Status == core.StatusQueued {
			next = i
			break
		}
	}
	if next < 0 {
		c.mu.Unlock()
		return
	}

	rec := &c.jobs[next]
	now := time.Now().UTC()
	rec.Status = core.StatusRunning
	rec.StartedAt = &now
	rec.CurrentStep = nil
	rec.CurrentStepStartedAt = nil
	rec.StepElapsed = map[core.JobStep]time.Duration{}
	rec.TranslationStatus = ""
	rec.TranslationStartedAt = nil
	rec.TranslationEndedAt = nil
	rec.CurrentStepProgress = 0
	rec.CompletedSteps = map[core.JobStep]bool{}
	rec.IsPauseRequested = false

	jobID := rec.ID
	request := rec.Request
	options := c.optionsByJobID[jobID]
	ctx, cancel := context.WithCancel(context.Background())
	c.activeJobID = jobID
	c.activeHas = true
	c.activeCancel = cancel
	c.mu.Unlock()

	go c.execute(ctx, request, options)
}

func (c *Coordinator) execute(ctx context.Context, request core.JobRequest, options JobExecutionOptions) {
	jobID := request.ID
	c.preloadQobuzTrackTotal(ctx, request)
	result, err := c.runner.Run(ctx, request, RunOptions{
		StandardCollision:           options.StandardCollision,
		QobuzExistingAlbumCollision: options.QobuzExistingAlbumCollision,
		KeepTemporaryFilesOnFailure: c.readKeepTempOption(),
	}, RunCallbacks{
		OnStep: func(step core.JobStep) {
			c.updateStep(jobID, step)
		},
		OnStepProgress: func(progress float64) {
			c.updateStepProgress(jobID, progress)
		},
		OnStepCount: func(done, total int) {
			c.updateLyricsTrackProgress(jobID, done, total)
		},
		OnLog: func(chunk string) {
			c.appendLog(jobID, chunk)
		},
		OnDisplayName: func(name string) {
			c.setRuntimeDisplayName(jobID, name)
		},
	})

	if err != nil {
		if ctx.Err() == context.Canceled {
			c.markCancelled(jobID)
		} else {
			c.markFailed(jobID, err)
		}
	} else {
		c.markCompleted(jobID, result)
	}

	c.mu.Lock()
	if c.activeHas && c.activeJobID == jobID {
		c.activeHas = false
		c.activeCancel = nil
	}
	c.mu.Unlock()
	c.startNextJobIfNeeded()
}

func (c *Coordinator) preloadQobuzTrackTotal(ctx context.Context, request core.JobRequest) {
	if request.SourceKind != core.SourceQobuz {
		return
	}
	rt, ok := util.QobuzResourceTypeFromURL(request.InputURL)
	if !ok {
		return
	}
	metaCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	total := 0
	switch rt {
	case util.QobuzAlbum:
		albumID, idOK := util.QobuzResourceIdentifier(request.InputURL)
		if !idOK || strings.TrimSpace(albumID) == "" {
			return
		}
		resp, err := c.qobuz.FetchAlbumTracks(metaCtx, albumID, request.QobuzEmail, request.QobuzPassword)
		if err != nil {
			return
		}
		total = len(resp.Tracks)
	case util.QobuzPlaylist:
		resp, err := c.qobuz.FetchPlaylistCatalog(metaCtx, request.InputURL, request.QobuzEmail, request.QobuzPassword)
		if err != nil {
			return
		}
		if resp.TracksCount > 0 {
			total = resp.TracksCount
		} else {
			total = len(resp.Tracks)
		}
	default:
		return
	}
	if total <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOfLocked(request.ID)
	if idx < 0 {
		return
	}
	rec := &c.jobs[idx]
	rec.QobuzTracksTotal = total
	if rec.QobuzTracksDone > total {
		rec.QobuzTracksDone = total
	}
}

func (c *Coordinator) readKeepTempOption() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.settings.KeepTemporaryFilesOnFailure
}

func (c *Coordinator) updateStep(jobID xuuid.UUID, step core.JobStep) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		return
	}
	rec := &c.jobs[idx]
	if rec.Status != core.StatusRunning && rec.Status != core.StatusPaused {
		return
	}
	if rec.Status == core.StatusPaused {
		rec.Status = core.StatusRunning
		rec.IsPauseRequested = false
	}
	now := time.Now().UTC()
	if rec.CurrentStep != nil && *rec.CurrentStep == step {
		return
	}
	if rec.CurrentStep != nil && *rec.CurrentStep != step {
		c.accumulateCurrentStepElapsedLocked(rec, now)
		if *rec.CurrentStep == core.StepTranscription {
			c.finishTranslationLocked(rec, now, "completed")
		}
		rec.CompletedSteps[*rec.CurrentStep] = true
	}
	rec.CurrentStep = &step
	rec.CurrentStepStartedAt = &now
	rec.CurrentStepProgress = 0
}

func (c *Coordinator) updateStepProgress(jobID xuuid.UUID, progress float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		return
	}
	rec := &c.jobs[idx]
	if rec.Status != core.StatusRunning && rec.Status != core.StatusPaused {
		return
	}
	if rec.CurrentStep == nil {
		return
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	if progress > rec.CurrentStepProgress {
		rec.CurrentStepProgress = progress
	}
}

func (c *Coordinator) updateLyricsTrackProgress(jobID xuuid.UUID, done, total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		return
	}
	rec := &c.jobs[idx]
	if rec.Status != core.StatusRunning && rec.Status != core.StatusPaused {
		return
	}
	if rec.CurrentStep == nil || *rec.CurrentStep != core.StepLyrics {
		return
	}
	if total < 0 {
		total = 0
	}
	if done < 0 {
		done = 0
	}
	if total > 0 && done > total {
		done = total
	}
	rec.LyricsTracksTotal = total
	rec.LyricsTracksDone = done
	if total > rec.LyricsFoundTotal {
		rec.LyricsFoundTotal = total
	}
	if rec.LyricsFound > rec.LyricsFoundTotal {
		rec.LyricsFound = rec.LyricsFoundTotal
	}
}

func (c *Coordinator) markCompleted(jobID xuuid.UUID, result core.JobResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		return
	}
	rec := &c.jobs[idx]
	if rec.Status == core.StatusCancelled {
		return
	}
	now := time.Now().UTC()
	c.accumulateCurrentStepElapsedLocked(rec, now)
	c.finishTranslationLocked(rec, now, "completed")
	rec.Status = core.StatusCompleted
	if rec.CurrentStep != nil {
		rec.CompletedSteps[*rec.CurrentStep] = true
	}
	rec.CurrentStep = nil
	rec.CurrentStepStartedAt = nil
	rec.CurrentStepProgress = 1
	rec.EndedAt = &now
	rec.ErrorMessage = ""
	rec.Result = &result
	c.appendLogLocked(jobID, "[job] Termine avec succes.\n")
}

func (c *Coordinator) markCancelled(jobID xuuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		return
	}
	rec := &c.jobs[idx]
	now := time.Now().UTC()
	c.accumulateCurrentStepElapsedLocked(rec, now)
	c.finishTranslationLocked(rec, now, "cancelled")
	rec.Status = core.StatusCancelled
	rec.CurrentStep = nil
	rec.CurrentStepStartedAt = nil
	rec.CurrentStepProgress = 0
	rec.EndedAt = &now
	rec.ErrorMessage = "Annule"
	c.appendLogLocked(jobID, "[job] Annule.\n")
}

func (c *Coordinator) markFailed(jobID xuuid.UUID, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		return
	}
	rec := &c.jobs[idx]
	if rec.Status == core.StatusCancelled {
		return
	}
	now := time.Now().UTC()
	c.accumulateCurrentStepElapsedLocked(rec, now)
	c.finishTranslationLocked(rec, now, "failed")
	rec.Status = core.StatusFailed
	rec.CurrentStep = nil
	rec.CurrentStepStartedAt = nil
	rec.CurrentStepProgress = 0
	rec.EndedAt = &now
	rec.ErrorMessage = err.Error()
	c.appendLogLocked(jobID, "[erreur] "+err.Error()+"\n")
}

func (c *Coordinator) appendLog(jobID xuuid.UUID, chunk string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.appendLogLocked(jobID, chunk)
}

func (c *Coordinator) appendLogLocked(jobID xuuid.UUID, chunk string) {
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		return
	}
	rec := &c.jobs[idx]
	now := time.Now().UTC()
	rec.Logs += chunk
	if rec.Request.SourceKind == core.SourceQobuz && rec.QobuzTracksTotal > 0 {
		done := extractQobuzTrackDone(chunk)
		if done > rec.QobuzTracksDone {
			rec.QobuzTracksDone = done
			if rec.QobuzTracksDone > rec.QobuzTracksTotal {
				rec.QobuzTracksDone = rec.QobuzTracksTotal
			}
		}
	}
	updateLyricsProgressFromLogChunk(rec, chunk)
	updateTranslationProgressFromLogChunk(rec, chunk, now)
	if len(rec.Logs) > c.maxLogCharacters {
		rec.Logs = rec.Logs[len(rec.Logs)-c.maxLogCharacters:]
	}
	logFile := c.paths.LogFile(jobID.String())
	_ = os.MkdirAll(filepath.Dir(logFile), 0o755)
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		_, _ = f.WriteString(chunk)
		_ = f.Close()
	}
}

func (c *Coordinator) accumulateCurrentStepElapsedLocked(rec *core.JobRecord, now time.Time) {
	if rec == nil || rec.CurrentStep == nil || rec.CurrentStepStartedAt == nil {
		return
	}
	if now.Before(*rec.CurrentStepStartedAt) {
		return
	}
	if rec.StepElapsed == nil {
		rec.StepElapsed = map[core.JobStep]time.Duration{}
	}
	rec.StepElapsed[*rec.CurrentStep] += now.Sub(*rec.CurrentStepStartedAt)
}

func (c *Coordinator) finishTranslationLocked(rec *core.JobRecord, now time.Time, fallbackStatus string) {
	if rec == nil {
		return
	}
	if rec.TranslationStartedAt == nil {
		return
	}
	if rec.TranslationEndedAt == nil {
		rec.TranslationEndedAt = &now
	}
	if strings.TrimSpace(rec.TranslationStatus) == "" {
		rec.TranslationStatus = fallbackStatus
		return
	}
	if rec.TranslationStatus == "running" || rec.TranslationStatus == "pending" {
		rec.TranslationStatus = fallbackStatus
	}
}

func updateLyricsProgressFromLogChunk(rec *core.JobRecord, chunk string) {
	if rec == nil {
		return
	}
	generated := countLyricsGeneratedTracks(chunk)
	alreadyPresent := countLyricsAlreadyPresentTracks(chunk)
	failed := countLyricsFailedTracks(chunk)
	if generated > 0 || alreadyPresent > 0 || failed > 0 {
		rec.LyricsFound += generated + alreadyPresent
		rec.LyricsFailed += failed
		if rec.LyricsTracksTotal > rec.LyricsFoundTotal {
			rec.LyricsFoundTotal = rec.LyricsTracksTotal
		}
		progressTotal := rec.LyricsFound + rec.LyricsFailed
		if progressTotal > rec.LyricsFoundTotal {
			rec.LyricsFoundTotal = progressTotal
		}
		if rec.LyricsFound > rec.LyricsFoundTotal {
			rec.LyricsFound = rec.LyricsFoundTotal
		}
	}
	if found, total, failedCount, ok := extractLyricsFoundSummary(chunk); ok {
		rec.LyricsFound = found
		rec.LyricsFoundTotal = total
		rec.LyricsFailed = failedCount
		if rec.LyricsTracksTotal > rec.LyricsFoundTotal {
			rec.LyricsFoundTotal = rec.LyricsTracksTotal
		}
		if rec.LyricsFound > rec.LyricsFoundTotal {
			rec.LyricsFound = rec.LyricsFoundTotal
		}
		if rec.LyricsFailed < 0 {
			rec.LyricsFailed = 0
		}
	}
}

func updateTranslationProgressFromLogChunk(rec *core.JobRecord, chunk string, now time.Time) {
	if rec == nil {
		return
	}
	if strings.Contains(chunk, "[translation] Etape ignoree (") {
		if rec.TranslationStartedAt == nil {
			rec.TranslationStartedAt = &now
		}
		rec.TranslationEndedAt = &now
		rec.TranslationStatus = "skipped"
		return
	}
	if strings.Contains(chunk, "[translation] Traduction Argos ") || strings.Contains(chunk, "[translation] Traduction du fichier ") {
		if rec.TranslationStartedAt == nil {
			rec.TranslationStartedAt = &now
		}
		rec.TranslationEndedAt = nil
		rec.TranslationStatus = "running"
	}
	if strings.Contains(chunk, "[translation] Variantes source/traduite conservees.") {
		if rec.TranslationStartedAt == nil {
			rec.TranslationStartedAt = &now
		}
		rec.TranslationEndedAt = &now
		rec.TranslationStatus = "completed"
	}
}

func (c *Coordinator) setRuntimeDisplayName(jobID xuuid.UUID, name string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.indexOfLocked(jobID)
	if idx < 0 {
		return
	}
	rec := &c.jobs[idx]
	if rec.Request.SourceKind != core.SourceYouTube {
		return
	}
	if strings.TrimSpace(rec.Request.CustomName) != "" {
		return
	}
	if existing := strings.TrimSpace(c.displayNameByJobID[jobID]); existing != "" {
		return
	}
	c.displayNameByJobID[jobID] = trimmed
}

func extractQobuzTrackDone(chunk string) int {
	maxDone := 0
	matches := qobuzTmpTrackIndexRe.FindAllStringSubmatch(chunk, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value, err := strconv.Atoi(match[1])
		if err != nil || value < 0 {
			continue
		}
		done := value + 1
		if done > maxDone {
			maxDone = done
		}
	}
	return maxDone
}

func countLyricsGeneratedTracks(chunk string) int {
	return len(lyricsGeneratedLineRe.FindAllString(chunk, -1))
}

func countLyricsAlreadyPresentTracks(chunk string) int {
	return len(lyricsAlreadyPresentLineRe.FindAllString(chunk, -1))
}

func countLyricsFailedTracks(chunk string) int {
	return len(lyricsFailureLineRe.FindAllString(chunk, -1))
}

func extractLyricsFoundSummary(chunk string) (int, int, int, bool) {
	matches := lyricsSummaryRe.FindAllStringSubmatch(chunk, -1)
	if len(matches) == 0 {
		return 0, 0, 0, false
	}
	last := matches[len(matches)-1]
	if len(last) < 4 {
		return 0, 0, 0, false
	}
	generated, err := strconv.Atoi(last[1])
	if err != nil || generated < 0 {
		return 0, 0, 0, false
	}
	alreadyPresent, err := strconv.Atoi(last[2])
	if err != nil || alreadyPresent < 0 {
		return 0, 0, 0, false
	}
	errorsCount, err := strconv.Atoi(last[3])
	if err != nil || errorsCount < 0 {
		return 0, 0, 0, false
	}
	found := generated + alreadyPresent
	total := found + errorsCount
	return found, total, errorsCount, true
}

func (c *Coordinator) indexOfLocked(jobID xuuid.UUID) int {
	for i := range c.jobs {
		if c.jobs[i].ID == jobID {
			return i
		}
	}
	return -1
}

func (c *Coordinator) sortedJobsLocked() []core.JobRecord {
	out := make([]core.JobRecord, len(c.jobs))
	copy(out, c.jobs)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Request.CreatedAt.After(out[j].Request.CreatedAt)
	})
	return out
}

func (c *Coordinator) mapDTOsLocked(records []core.JobRecord) []core.JobSummaryDTO {
	now := time.Now().UTC()
	out := make([]core.JobSummaryDTO, 0, len(records))
	for i := range records {
		rec := records[i]
		out = append(out, c.dtoFromRecordLocked(rec, now))
	}
	return out
}

func (c *Coordinator) dtoLocked(record *core.JobRecord) core.JobSummaryDTO {
	return c.dtoFromRecordLocked(*record, time.Now().UTC())
}

func (c *Coordinator) dtoFromRecordLocked(record core.JobRecord, now time.Time) core.JobSummaryDTO {
	completed := []string{}
	for _, step := range core.AllSteps {
		if record.CompletedSteps[step] {
			completed = append(completed, string(step))
		}
	}
	var result *core.JobResultDTO
	if record.Result != nil {
		result = &core.JobResultDTO{MediaPath: record.Result.MediaPath, SubtitlePath: record.Result.SubtitlePath, TranscriptPath: record.Result.TranscriptPath, MetadataPath: record.Result.MetadataPath}
	}
	currentStep := ""
	if record.CurrentStep != nil {
		currentStep = string(*record.CurrentStep)
	}
	totalElapsedSeconds := int64(record.TotalElapsed(now) / time.Second)
	if totalElapsedSeconds < 0 {
		totalElapsedSeconds = 0
	}
	activeStepElapsedSeconds := int64(record.ActiveStepElapsed(now) / time.Second)
	if activeStepElapsedSeconds < 0 {
		activeStepElapsedSeconds = 0
	}
	downloadElapsedSeconds := int64(record.ElapsedForStep(core.StepDownload, now) / time.Second)
	if downloadElapsedSeconds < 0 {
		downloadElapsedSeconds = 0
	}
	lyricsElapsedSeconds := int64(record.ElapsedForStep(core.StepLyrics, now) / time.Second)
	if lyricsElapsedSeconds < 0 {
		lyricsElapsedSeconds = 0
	}
	transcriptionElapsedSeconds := int64(record.ElapsedForStep(core.StepTranscription, now) / time.Second)
	if transcriptionElapsedSeconds < 0 {
		transcriptionElapsedSeconds = 0
	}
	translationElapsedSeconds := int64(record.TranslationElapsed(now) / time.Second)
	if translationElapsedSeconds < 0 {
		translationElapsedSeconds = 0
	}
	return core.JobSummaryDTO{
		ID:                          record.ID.String(),
		CreatedAt:                   record.Request.CreatedAt,
		SourceKind:                  string(record.Request.SourceKind),
		ContentType:                 string(record.Request.ContentType),
		InputURL:                    record.Request.InputURL,
		OutputRootPath:              record.Request.OutputRootPath,
		CustomName:                  record.Request.CustomName,
		DisplayName:                 c.resolvedDisplayNameLocked(record),
		Status:                      string(record.Status),
		CurrentStep:                 currentStep,
		CurrentStepProgress:         record.CurrentStepProgress,
		ProgressFraction:            record.ProgressFraction(),
		ProgressPercent:             record.ProgressPercent(),
		CompletedSteps:              completed,
		StartedAt:                   record.StartedAt,
		EndedAt:                     record.EndedAt,
		CurrentStepStartedAt:        record.CurrentStepStartedAt,
		TotalElapsedSeconds:         totalElapsedSeconds,
		ActiveStepElapsedSeconds:    activeStepElapsedSeconds,
		DownloadElapsedSeconds:      downloadElapsedSeconds,
		LyricsElapsedSeconds:        lyricsElapsedSeconds,
		TranscriptionElapsedSeconds: transcriptionElapsedSeconds,
		TranslationStatus:           record.TranslationStatus,
		TranslationElapsedSeconds:   translationElapsedSeconds,
		ErrorMessage:                record.ErrorMessage,
		IsPauseRequested:            record.IsPauseRequested,
		Result:                      result,
		LogsSize:                    len(record.Logs),
		QobuzTracksDone:             record.QobuzTracksDone,
		QobuzTracksTotal:            record.QobuzTracksTotal,
		LyricsTracksDone:            record.LyricsTracksDone,
		LyricsTracksTotal:           record.LyricsTracksTotal,
		LyricsFound:                 record.LyricsFound,
		LyricsFoundTotal:            record.LyricsFoundTotal,
		LyricsFailed:                record.LyricsFailed,
	}
}

func (c *Coordinator) resolvedDisplayNameLocked(record core.JobRecord) string {
	if strings.TrimSpace(record.Request.CustomName) != "" {
		return strings.TrimSpace(record.Request.CustomName)
	}
	if mapped := strings.TrimSpace(c.displayNameByJobID[record.ID]); mapped != "" {
		return mapped
	}
	if record.Request.SelectedRSSEpisode != nil {
		if t := strings.TrimSpace(record.Request.SelectedRSSEpisode.Title); t != "" {
			return t
		}
	}
	if inferred := inferQobuzAlbumTitle(record.Request.InputURL); inferred != "" {
		return inferred
	}
	return ""
}

func inferQobuzAlbumTitle(inputURL string) string {
	rt, ok := util.QobuzResourceTypeFromURL(inputURL)
	if !ok || rt != util.QobuzAlbum {
		return ""
	}
	u, err := url.Parse(inputURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	idx := -1
	for i, p := range parts {
		if strings.ToLower(p) == "album" {
			idx = i
			break
		}
	}
	if idx < 0 || idx+1 >= len(parts) {
		return ""
	}
	tail := parts[idx+1:]
	if len(tail) < 2 {
		return ""
	}
	slug := strings.Join(tail[:len(tail)-1], " ")
	slug, _ = url.QueryUnescape(slug)
	slug = strings.ReplaceAll(slug, "-", " ")
	slug = strings.Join(strings.Fields(slug), " ")
	return strings.TrimSpace(slug)
}

func (c *Coordinator) buildJob(payload core.CreateJobAPIRequest) (builtJob, error) {
	inputURL := strings.TrimSpace(payload.InputURL)
	if inputURL == "" {
		return builtJob{}, fmt.Errorf("le champ inputURL est requis")
	}
	sourceKind, err := c.resolveSourceKind(inputURL, payload.SourceKind)
	if err != nil {
		return builtJob{}, err
	}
	contentType, err := c.resolveContentType(sourceKind, payload.ContentType)
	if err != nil {
		return builtJob{}, err
	}

	c.mu.Lock()
	settings := c.settings
	c.mu.Unlock()

	outputRoot := strings.TrimSpace(payload.OutputRootPath)
	if outputRoot == "" {
		outputRoot = strings.TrimSpace(settings.DefaultOutputRoot)
	}
	if outputRoot == "" {
		home, _ := os.UserHomeDir()
		outputRoot = home
	}
	whisperModelPath := strings.TrimSpace(payload.WhisperModelPath)
	if whisperModelPath == "" {
		whisperModelPath = settings.WhisperModelPath
	}
	transcriptionLanguage := strings.TrimSpace(payload.TranscriptionLanguage)
	if transcriptionLanguage == "" {
		transcriptionLanguage = "auto"
	}
	enableTranscription := contentType != core.ContentMusic
	if payload.EnableTranscription != nil {
		enableTranscription = *payload.EnableTranscription && contentType != core.ContentMusic
	}
	enableTranslation := false
	if payload.EnableTranslation != nil {
		enableTranslation = *payload.EnableTranslation
	}
	enableTranslation = enableTranslation && enableTranscription
	translationSourceLanguage := normalizeLanguageCode(payload.TranslationSourceLanguage, "en")
	translationTargetLanguage := normalizeLanguageCode(payload.TranslationTargetLanguage, "fr")
	enableLyrics := contentType == core.ContentMusic
	if payload.EnableLyrics != nil {
		enableLyrics = *payload.EnableLyrics && contentType == core.ContentMusic
	}
	useFirefoxCookies := settings.UseFirefoxCookies
	if payload.UseFirefoxCookies != nil {
		useFirefoxCookies = *payload.UseFirefoxCookies
	}
	qobuzEmail := fallbackTrimmed(payload.QobuzEmail, settings.QobuzEmail)
	qobuzPassword := fallbackRaw(payload.QobuzPassword, settings.QobuzPassword)

	var selectedRSS *core.RSSEpisodeSelection
	if sourceKind == core.SourceRSS {
		ep := payload.RSSEpisode
		if ep == nil {
			return builtJob{}, fmt.Errorf("pour un job RSS, renseigne rssEpisode { title, mediaURL, podcastTitle, publicationDate? }")
		}
		mediaURL := strings.TrimSpace(ep.MediaURL)
		title := strings.TrimSpace(ep.Title)
		podcastTitle := strings.TrimSpace(ep.PodcastTitle)
		if mediaURL == "" {
			return builtJob{}, fmt.Errorf("rssEpisode.mediaURL est requis")
		}
		if title == "" {
			return builtJob{}, fmt.Errorf("rssEpisode.title est requis")
		}
		if podcastTitle == "" {
			return builtJob{}, fmt.Errorf("rssEpisode.podcastTitle est requis")
		}
		selectedRSS = &core.RSSEpisodeSelection{Title: title, MediaURL: mediaURL, PodcastTitle: podcastTitle, ArtworkURL: strings.TrimSpace(ep.ArtworkURL)}
		if pub, ok := parseOptionalDate(ep.PublicationDate); ok {
			selectedRSS.PublicationDate = &pub
		}
	}

	collision, err := parseCollisionDecision(payload.CollisionPolicy, core.CollisionRename)
	if err != nil {
		return builtJob{}, err
	}
	qobuzCollision, err := parseCollisionDecision(payload.QobuzExistingAlbumPolicy, collision)
	if err != nil {
		return builtJob{}, err
	}

	req := core.JobRequest{
		ID:                        xuuid.New(),
		CreatedAt:                 time.Now().UTC(),
		SourceKind:                sourceKind,
		ContentType:               contentType,
		InputURL:                  inputURL,
		SelectedRSSEpisode:        selectedRSS,
		TranscriptionLanguage:     transcriptionLanguage,
		EnableTranscription:       enableTranscription,
		EnableTranslation:         enableTranslation,
		TranslationSourceLanguage: translationSourceLanguage,
		TranslationTargetLanguage: translationTargetLanguage,
		EnableLyrics:              enableLyrics,
		WhisperModelPath:          whisperModelPath,
		YtDlpExtraArguments:       strings.TrimSpace(payload.YtDlpExtraArguments),
		WhisperExtraArguments:     strings.TrimSpace(payload.WhisperExtraArguments),
		FfmpegExtraArguments:      strings.TrimSpace(payload.FfmpegExtraArguments),
		QobuzExtraArguments:       strings.TrimSpace(payload.QobuzExtraArguments),
		OutputRootPath:            outputRoot,
		CustomName:                strings.TrimSpace(payload.CustomName),
		UseFirefoxCookies:         useFirefoxCookies,
		QobuzEmail:                qobuzEmail,
		QobuzPassword:             qobuzPassword,
		QobuzArtistName:           strings.TrimSpace(payload.QobuzArtistName),
		QobuzPlaylistName:         strings.TrimSpace(payload.QobuzPlaylistName),
	}
	displayName := strings.TrimSpace(payload.DisplayName)
	if displayName == "" && selectedRSS != nil {
		displayName = strings.TrimSpace(selectedRSS.Title)
	}
	if displayName == "" {
		displayName = inferQobuzAlbumTitle(inputURL)
	}
	return builtJob{
		Request:     req,
		Options:     JobExecutionOptions{StandardCollision: collision, QobuzExistingAlbumCollision: qobuzCollision},
		DisplayName: displayName,
	}, nil
}

func (c *Coordinator) resolveSourceKind(inputURL, requested string) (core.JobSourceKind, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" || requested == "auto" {
		return c.inferSourceKind(inputURL)
	}
	switch requested {
	case string(core.SourceYouTube):
		return core.SourceYouTube, nil
	case string(core.SourceRSS):
		return core.SourceRSS, nil
	case string(core.SourceQobuz):
		return core.SourceQobuz, nil
	default:
		return "", fmt.Errorf("sourceKind invalide. Valeurs: auto, youtube, rss, qobuz")
	}
}

func (c *Coordinator) inferSourceKind(inputURL string) (core.JobSourceKind, error) {
	lower := strings.ToLower(inputURL)
	if strings.Contains(lower, "qobuz.com") {
		return core.SourceQobuz, nil
	}
	if strings.Contains(lower, "youtube.com") || strings.Contains(lower, "youtu.be") {
		return core.SourceYouTube, nil
	}
	if strings.Contains(lower, "/rss") || strings.Contains(lower, "feed") || strings.HasSuffix(lower, ".xml") {
		return core.SourceRSS, nil
	}
	return "", fmt.Errorf("impossible de deduire la source. Renseigne sourceKind explicitement (youtube, rss, qobuz)")
}

func (c *Coordinator) resolveContentType(source core.JobSourceKind, requested string) (core.JobContentType, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" || requested == "auto" {
		switch source {
		case core.SourceYouTube:
			return core.ContentVideo, nil
		case core.SourceRSS:
			return core.ContentAudio, nil
		case core.SourceQobuz:
			return core.ContentMusic, nil
		}
	}
	switch requested {
	case string(core.ContentVideo):
		return core.ContentVideo, nil
	case string(core.ContentAudio):
		return core.ContentAudio, nil
	case string(core.ContentMusic):
		return core.ContentMusic, nil
	default:
		return "", fmt.Errorf("contentType invalide. Valeurs: auto, video, audio, music")
	}
}

func parseCollisionDecision(raw string, fallback core.CollisionDecision) (core.CollisionDecision, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return fallback, nil
	}
	switch v {
	case "overwrite":
		return core.CollisionOverwrite, nil
	case "rename":
		return core.CollisionRename, nil
	case "complete":
		return core.CollisionComplete, nil
	case "fetchmissinglyrics":
		return core.CollisionFetchMissingLyrics, nil
	case "cancel":
		return core.CollisionCancel, nil
	default:
		return "", fmt.Errorf("politique de collision invalide. Valeurs: overwrite, rename, complete, fetchMissingLyrics, cancel")
	}
}

func parseOptionalDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

func (c *Coordinator) loadSettings() core.WebSettings {
	home, _ := os.UserHomeDir()
	fallback := core.WebSettings{
		WhisperModelPath:            "",
		UseFirefoxCookies:           false,
		KeepTemporaryFilesOnFailure: true,
		QobuzEmail:                  "",
		QobuzPassword:               "",
		DefaultOutputRoot:           home,
	}
	if data, err := os.ReadFile(c.paths.WebSettingsFile); err == nil {
		var s core.WebSettings
		if err := json.Unmarshal(data, &s); err == nil {
			if strings.TrimSpace(s.DefaultOutputRoot) == "" {
				s.DefaultOutputRoot = home
			}
			if !s.KeepTemporaryFilesOnFailure {
				// keep false if explicit value false in file. no-op.
			}
			return s
		}
	}
	return fallback
}

func (c *Coordinator) persistSettingsLocked() {
	_ = os.MkdirAll(filepath.Dir(c.paths.WebSettingsFile), 0o755)
	data, err := json.MarshalIndent(c.settings, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(c.paths.WebSettingsFile, data, 0o644)
}

func fallbackTrimmed(raw, fallback string) string {
	if strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(fallback)
}

func fallbackRaw(raw, fallback string) string {
	if strings.TrimSpace(raw) != "" {
		return raw
	}
	return fallback
}

func normalizeLanguageCode(raw, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if normalized == "" || normalized == "auto" {
		return fallback
	}
	return normalized
}

func sameFilePath(left, right string) bool {
	l := strings.TrimSpace(left)
	r := strings.TrimSpace(right)
	if l == "" || r == "" {
		return false
	}
	l = filepath.Clean(l)
	r = filepath.Clean(r)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(l, r)
	}
	return l == r
}
