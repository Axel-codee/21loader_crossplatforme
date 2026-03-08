package core

import (
	"time"

	"21loader-cross/internal/xuuid"
)

type JobSourceKind string

type JobContentType string

type JobStatus string

type JobStep string

type CollisionDecision string

const (
	SourceYouTube JobSourceKind = "youtube"
	SourceRSS     JobSourceKind = "rss"
	SourceQobuz   JobSourceKind = "qobuz"
)

const (
	ContentVideo JobContentType = "video"
	ContentAudio JobContentType = "audio"
	ContentMusic JobContentType = "music"
)

const (
	StatusQueued    JobStatus = "queued"
	StatusRunning   JobStatus = "running"
	StatusPaused    JobStatus = "paused"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

const (
	StepDownload      JobStep = "download"
	StepLyrics        JobStep = "lyrics"
	StepTranscription JobStep = "transcription"
	StepMuxing        JobStep = "muxing"
	StepOrganization  JobStep = "organization"
)

var AllSteps = []JobStep{
	StepDownload,
	StepLyrics,
	StepTranscription,
	StepMuxing,
	StepOrganization,
}

const (
	CollisionOverwrite          CollisionDecision = "overwrite"
	CollisionRename             CollisionDecision = "rename"
	CollisionComplete           CollisionDecision = "complete"
	CollisionFetchMissingLyrics CollisionDecision = "fetchMissingLyrics"
	CollisionCancel             CollisionDecision = "cancel"
)

type WebSettings struct {
	WhisperModelPath            string `json:"whisperModelPath"`
	UseFirefoxCookies           bool   `json:"useFirefoxCookies"`
	KeepTemporaryFilesOnFailure bool   `json:"keepTemporaryFilesOnFailure"`
	QobuzEmail                  string `json:"qobuzEmail"`
	QobuzPassword               string `json:"qobuzPassword"`
	DefaultOutputRoot           string `json:"defaultOutputRoot"`
}

type RSSEpisodeAPIInput struct {
	Title           string `json:"title"`
	PublicationDate string `json:"publicationDate"`
	MediaURL        string `json:"mediaURL"`
	PodcastTitle    string `json:"podcastTitle"`
	ArtworkURL      string `json:"artworkURL"`
}

type RSSFeedEpisodesAPIRequest struct {
	FeedURL string `json:"feedURL"`
}

type RSSEpisodeDTO struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	PublicationDate *time.Time `json:"publicationDate"`
	MediaURL        string     `json:"mediaURL,omitempty"`
	FallbackLink    string     `json:"fallbackLink,omitempty"`
	ArtworkURL      string     `json:"artworkURL,omitempty"`
}

type RSSFeedEpisodesAPIResponse struct {
	PodcastTitle      string          `json:"podcastTitle"`
	PodcastArtworkURL string          `json:"podcastArtworkURL,omitempty"`
	Episodes          []RSSEpisodeDTO `json:"episodes"`
}

type YouTubeCatalogAPIRequest struct {
	URL               string `json:"url"`
	UseFirefoxCookies *bool  `json:"useFirefoxCookies"`
}

type YouTubeVideoDTO struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	WebpageURL      string     `json:"webpageURL"`
	UploadDate      *time.Time `json:"uploadDate"`
	DurationSeconds *int       `json:"durationSeconds,omitempty"`
	Position        int        `json:"position"`
}

type YouTubeCatalogAPIResponse struct {
	SourceTitle string            `json:"sourceTitle"`
	Videos      []YouTubeVideoDTO `json:"videos"`
}

type YouTubeDatesAPIRequest struct {
	VideoIDs          []string `json:"videoIDs"`
	UseFirefoxCookies *bool    `json:"useFirefoxCookies"`
}

type YouTubeDatesAPIResponse struct {
	DatesByVideoID     map[string]time.Time `json:"datesByVideoID"`
	DurationsByVideoID map[string]int       `json:"durationsByVideoID"`
}

type SelectDirectoryRequest struct {
	CurrentPath string `json:"currentPath"`
}

type SelectDirectoryResponse struct {
	Path      string `json:"path,omitempty"`
	Cancelled bool   `json:"cancelled"`
}

type SelectFileRequest struct {
	CurrentPath string   `json:"currentPath"`
	Title       string   `json:"title,omitempty"`
	Filters     []string `json:"filters,omitempty"`
}

type SelectFileResponse struct {
	Path      string `json:"path,omitempty"`
	Cancelled bool   `json:"cancelled"`
}

type CreateJobAPIRequest struct {
	InputURL                  string              `json:"inputURL"`
	SourceKind                string              `json:"sourceKind"`
	ContentType               string              `json:"contentType"`
	OutputRootPath            string              `json:"outputRootPath"`
	CustomName                string              `json:"customName"`
	DisplayName               string              `json:"displayName"`
	TranscriptionLanguage     string              `json:"transcriptionLanguage"`
	EnableTranscription       *bool               `json:"enableTranscription"`
	EnableTranslation         *bool               `json:"enableTranslation"`
	TranslationSourceLanguage string              `json:"translationSourceLanguage"`
	TranslationTargetLanguage string              `json:"translationTargetLanguage"`
	EnableLyrics              *bool               `json:"enableLyrics"`
	WhisperModelPath          string              `json:"whisperModelPath"`
	YtDlpExtraArguments       string              `json:"ytDlpExtraArguments"`
	WhisperExtraArguments     string              `json:"whisperExtraArguments"`
	FfmpegExtraArguments      string              `json:"ffmpegExtraArguments"`
	QobuzExtraArguments       string              `json:"qobuzExtraArguments"`
	UseFirefoxCookies         *bool               `json:"useFirefoxCookies"`
	QobuzEmail                string              `json:"qobuzEmail"`
	QobuzPassword             string              `json:"qobuzPassword"`
	QobuzArtistName           string              `json:"qobuzArtistName"`
	QobuzPlaylistName         string              `json:"qobuzPlaylistName"`
	CollisionPolicy           string              `json:"collisionPolicy"`
	QobuzExistingAlbumPolicy  string              `json:"qobuzExistingAlbumPolicy"`
	RSSEpisode                *RSSEpisodeAPIInput `json:"rssEpisode"`
}

type UpdateSettingsAPIRequest struct {
	WhisperModelPath            *string `json:"whisperModelPath"`
	UseFirefoxCookies           *bool   `json:"useFirefoxCookies"`
	KeepTemporaryFilesOnFailure *bool   `json:"keepTemporaryFilesOnFailure"`
	QobuzEmail                  *string `json:"qobuzEmail"`
	QobuzPassword               *string `json:"qobuzPassword"`
	DefaultOutputRoot           *string `json:"defaultOutputRoot"`
}

type QobuzArtistCatalogAPIRequest struct {
	ArtistURL     string `json:"artistURL"`
	QobuzEmail    string `json:"qobuzEmail"`
	QobuzPassword string `json:"qobuzPassword"`
}

type QobuzArtistSearchAPIRequest struct {
	Query         string `json:"query"`
	Limit         int    `json:"limit"`
	QobuzEmail    string `json:"qobuzEmail"`
	QobuzPassword string `json:"qobuzPassword"`
}

type QobuzAlbumTracksAPIRequest struct {
	AlbumID       string `json:"albumID"`
	QobuzEmail    string `json:"qobuzEmail"`
	QobuzPassword string `json:"qobuzPassword"`
}

type QobuzPlaylistCatalogAPIRequest struct {
	PlaylistURL   string `json:"playlistURL"`
	QobuzEmail    string `json:"qobuzEmail"`
	QobuzPassword string `json:"qobuzPassword"`
}

type QobuzAlbumDTO struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	ArtistName       string     `json:"artistName"`
	WebpageURL       string     `json:"webpageURL"`
	ReleaseDate      *time.Time `json:"releaseDate"`
	TrackCount       *int       `json:"trackCount"`
	ReleaseKindLabel string     `json:"releaseKindLabel"`
	IsHiRes          bool       `json:"isHiRes"`
	ArtworkURL       string     `json:"artworkURL,omitempty"`
}

type QobuzArtistCatalogAPIResponse struct {
	ArtistName string          `json:"artistName"`
	Albums     []QobuzAlbumDTO `json:"albums"`
}

type QobuzArtistSearchResultDTO struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	WebpageURL         string     `json:"webpageURL"`
	AlbumsCount        *int       `json:"albumsCount,omitempty"`
	CatalogAlbumsCount *int       `json:"catalogAlbumsCount,omitempty"`
	ArtworkURL         string     `json:"artworkURL,omitempty"`
	Slug               string     `json:"slug,omitempty"`
	Country            string     `json:"country,omitempty"`
	Genres             []string   `json:"genres,omitempty"`
	Biography          string     `json:"biography,omitempty"`
	LatestReleaseTitle string     `json:"latestReleaseTitle,omitempty"`
	LatestReleaseDate  *time.Time `json:"latestReleaseDate,omitempty"`
}

type QobuzArtistSearchAPIResponse struct {
	Artists []QobuzArtistSearchResultDTO `json:"artists"`
}

type QobuzTrackDTO struct {
	ID              string `json:"id"`
	TrackNumber     *int   `json:"trackNumber"`
	Title           string `json:"title"`
	DurationSeconds *int   `json:"durationSeconds"`
}

type QobuzAlbumTracksAPIResponse struct {
	AlbumID string          `json:"albumID"`
	Tracks  []QobuzTrackDTO `json:"tracks"`
}

type QobuzPlaylistTrackDTO struct {
	ID               string `json:"id"`
	Position         *int   `json:"position,omitempty"`
	Title            string `json:"title"`
	DurationSeconds  *int   `json:"durationSeconds,omitempty"`
	ArtistID         string `json:"artistID,omitempty"`
	ArtistName       string `json:"artistName"`
	ArtistWebpageURL string `json:"artistWebpageURL,omitempty"`
	AlbumID          string `json:"albumID,omitempty"`
	AlbumTitle       string `json:"albumTitle,omitempty"`
	AlbumWebpageURL  string `json:"albumWebpageURL,omitempty"`
}

type QobuzPlaylistArtistDTO struct {
	ID               string `json:"id,omitempty"`
	Name             string `json:"name"`
	WebpageURL       string `json:"webpageURL,omitempty"`
	TracksInPlaylist int    `json:"tracksInPlaylist"`
	AlbumsInPlaylist int    `json:"albumsInPlaylist"`
}

type QobuzPlaylistCatalogAPIResponse struct {
	PlaylistID   string                   `json:"playlistID"`
	PlaylistName string                   `json:"playlistName"`
	WebpageURL   string                   `json:"webpageURL"`
	TracksCount  int                      `json:"tracksCount"`
	Tracks       []QobuzPlaylistTrackDTO  `json:"tracks"`
	Albums       []QobuzAlbumDTO          `json:"albums"`
	Artists      []QobuzPlaylistArtistDTO `json:"artists"`
}

type JobResultDTO struct {
	MediaPath      string `json:"mediaPath"`
	SubtitlePath   string `json:"subtitlePath,omitempty"`
	TranscriptPath string `json:"transcriptPath,omitempty"`
	MetadataPath   string `json:"metadataPath"`
}

type JobSummaryDTO struct {
	ID                          string        `json:"id"`
	CreatedAt                   time.Time     `json:"createdAt"`
	SourceKind                  string        `json:"sourceKind"`
	ContentType                 string        `json:"contentType"`
	InputURL                    string        `json:"inputURL"`
	OutputRootPath              string        `json:"outputRootPath"`
	CustomName                  string        `json:"customName"`
	DisplayName                 string        `json:"displayName"`
	Status                      string        `json:"status"`
	CurrentStep                 string        `json:"currentStep,omitempty"`
	CurrentStepProgress         float64       `json:"currentStepProgress"`
	ProgressFraction            float64       `json:"progressFraction"`
	ProgressPercent             int           `json:"progressPercent"`
	CompletedSteps              []string      `json:"completedSteps"`
	StartedAt                   *time.Time    `json:"startedAt"`
	EndedAt                     *time.Time    `json:"endedAt"`
	CurrentStepStartedAt        *time.Time    `json:"currentStepStartedAt,omitempty"`
	TotalElapsedSeconds         int64         `json:"totalElapsedSeconds,omitempty"`
	ActiveStepElapsedSeconds    int64         `json:"activeStepElapsedSeconds,omitempty"`
	DownloadElapsedSeconds      int64         `json:"downloadElapsedSeconds,omitempty"`
	LyricsElapsedSeconds        int64         `json:"lyricsElapsedSeconds,omitempty"`
	TranscriptionElapsedSeconds int64         `json:"transcriptionElapsedSeconds,omitempty"`
	TranslationStatus           string        `json:"translationStatus,omitempty"`
	TranslationElapsedSeconds   int64         `json:"translationElapsedSeconds,omitempty"`
	ErrorMessage                string        `json:"errorMessage,omitempty"`
	IsPauseRequested            bool          `json:"isPauseRequested"`
	Result                      *JobResultDTO `json:"result,omitempty"`
	LogsSize                    int           `json:"logsSize"`
	QobuzTracksDone             int           `json:"qobuzTracksDone,omitempty"`
	QobuzTracksTotal            int           `json:"qobuzTracksTotal,omitempty"`
	LyricsTracksDone            int           `json:"lyricsTracksDone,omitempty"`
	LyricsTracksTotal           int           `json:"lyricsTracksTotal,omitempty"`
	LyricsFound                 int           `json:"lyricsFound,omitempty"`
	LyricsFoundTotal            int           `json:"lyricsFoundTotal,omitempty"`
	LyricsFailed                int           `json:"lyricsFailed,omitempty"`
}

type DashboardResponseDTO struct {
	ServerTime  time.Time       `json:"serverTime"`
	ActiveJobID string          `json:"activeJobID,omitempty"`
	Settings    WebSettings     `json:"settings"`
	Jobs        []JobSummaryDTO `json:"jobs"`
}

type ActionResponseDTO struct {
	OK      bool           `json:"ok"`
	Message string         `json:"message"`
	Job     *JobSummaryDTO `json:"job,omitempty"`
}

type HealthResponseDTO struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

type ErrorResponseDTO struct {
	Error string `json:"error"`
}

type WebBinaryDiagnostic struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Version     string `json:"version,omitempty"`
	Available   bool   `json:"available"`
	NeedsUpdate bool   `json:"needsUpdate,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

type WebDiagnosticsReport struct {
	CollectedAt    time.Time             `json:"collectedAt"`
	Platform       string                `json:"platform"`
	PackageManager string                `json:"packageManager"`
	Brew           WebBinaryDiagnostic   `json:"brew"`
	Tools          []WebBinaryDiagnostic `json:"tools"`
}

type DependencyInstallRequest struct {
	Tools []string `json:"tools"`
}

type DependencyInstallResult struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Message   string `json:"message"`
}

type DependencyInstallResponse struct {
	OK             bool                      `json:"ok"`
	PackageManager string                    `json:"packageManager"`
	Results        []DependencyInstallResult `json:"results"`
	Logs           string                    `json:"logs"`
}

type DependencyInstallProgressResponse struct {
	Active    bool      `json:"active"`
	Stage     string    `json:"stage"`
	Tool      string    `json:"tool,omitempty"`
	Action    string    `json:"action,omitempty"`
	Command   string    `json:"command,omitempty"`
	Message   string    `json:"message,omitempty"`
	Logs      string    `json:"logs,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

type AppUpdateRequest struct {
	FilePath string `json:"filePath"`
}

type AppUpdateResponse struct {
	OK               bool   `json:"ok"`
	Message          string `json:"message"`
	RestartScheduled bool   `json:"restartScheduled,omitempty"`
}

type TranslationLanguageInfoDTO struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

type TranslationLanguagePairDTO struct {
	SourceCode string `json:"sourceCode"`
	TargetCode string `json:"targetCode"`
	Installed  bool   `json:"installed"`
}

type TranslationLanguageCatalogResponse struct {
	RuntimeAvailable bool                         `json:"runtimeAvailable"`
	RuntimeMessage   string                       `json:"runtimeMessage,omitempty"`
	Languages        []TranslationLanguageInfoDTO `json:"languages"`
	Pairs            []TranslationLanguagePairDTO `json:"pairs"`
	Warnings         []string                     `json:"warnings,omitempty"`
}

type TranslationLanguageInstallRequest struct {
	SourceCode string `json:"sourceCode"`
	TargetCode string `json:"targetCode"`
}

type TranslationLanguageInstallResponse struct {
	OK      bool                               `json:"ok"`
	Message string                             `json:"message"`
	Catalog TranslationLanguageCatalogResponse `json:"catalog"`
}

type WhisperModelInfoDTO struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	FileName          string `json:"fileName"`
	DownloadURL       string `json:"downloadURL"`
	ApproximateSizeMB int    `json:"approximateSizeMB"`
	Installed         bool   `json:"installed"`
	ManagedByApp      bool   `json:"managedByApp"`
	InstalledPath     string `json:"installedPath,omitempty"`
	InstalledBytes    int64  `json:"installedBytes,omitempty"`
}

type WhisperModelsResponse struct {
	ModelDirectory    string                `json:"modelDirectory"`
	SelectedModelPath string                `json:"selectedModelPath,omitempty"`
	SelectedModelID   string                `json:"selectedModelID,omitempty"`
	Models            []WhisperModelInfoDTO `json:"models"`
}

type WhisperModelInstallRequest struct {
	ModelID string `json:"modelID"`
}

type WhisperModelInstallProgressRequest struct {
	ModelID string `json:"modelID"`
}

type WhisperModelInstallProgressResponse struct {
	ModelID         string    `json:"modelID"`
	Active          bool      `json:"active"`
	Stage           string    `json:"stage"`
	Message         string    `json:"message,omitempty"`
	DownloadedBytes int64     `json:"downloadedBytes,omitempty"`
	TotalBytes      int64     `json:"totalBytes,omitempty"`
	ProgressPercent int       `json:"progressPercent"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
}

type WhisperModelInstallResponse struct {
	OK      bool                `json:"ok"`
	Message string              `json:"message"`
	Model   WhisperModelInfoDTO `json:"model"`
}

type WhisperModelUninstallRequest struct {
	ModelID string `json:"modelID"`
}

type WhisperModelUninstallResponse struct {
	OK                      bool                `json:"ok"`
	Message                 string              `json:"message"`
	Model                   WhisperModelInfoDTO `json:"model"`
	RemovedPath             string              `json:"removedPath,omitempty"`
	ClearedDefaultSelection bool                `json:"clearedDefaultSelection,omitempty"`
}

type RSSEpisodeSelection struct {
	Title           string
	PublicationDate *time.Time
	MediaURL        string
	PodcastTitle    string
	ArtworkURL      string
}

type JobRequest struct {
	ID                        xuuid.UUID
	CreatedAt                 time.Time
	SourceKind                JobSourceKind
	ContentType               JobContentType
	InputURL                  string
	SelectedRSSEpisode        *RSSEpisodeSelection
	TranscriptionLanguage     string
	EnableTranscription       bool
	EnableTranslation         bool
	TranslationSourceLanguage string
	TranslationTargetLanguage string
	EnableLyrics              bool
	WhisperModelPath          string
	YtDlpExtraArguments       string
	WhisperExtraArguments     string
	FfmpegExtraArguments      string
	QobuzExtraArguments       string
	OutputRootPath            string
	CustomName                string
	UseFirefoxCookies         bool
	QobuzEmail                string
	QobuzPassword             string
	QobuzArtistName           string
	QobuzPlaylistName         string
}

type JobResult struct {
	MediaPath      string
	SubtitlePath   string
	TranscriptPath string
	MetadataPath   string
}

type JobRecord struct {
	ID                   xuuid.UUID
	Request              JobRequest
	Status               JobStatus
	CurrentStep          *JobStep
	CurrentStepStartedAt *time.Time
	StepElapsed          map[JobStep]time.Duration
	TranslationStatus    string
	TranslationStartedAt *time.Time
	TranslationEndedAt   *time.Time
	CurrentStepProgress  float64
	CompletedSteps       map[JobStep]bool
	StartedAt            *time.Time
	EndedAt              *time.Time
	ErrorMessage         string
	Result               *JobResult
	Logs                 string
	IsPauseRequested     bool
	QobuzTracksDone      int
	QobuzTracksTotal     int
	LyricsTracksDone     int
	LyricsTracksTotal    int
	LyricsFound          int
	LyricsFoundTotal     int
	LyricsFailed         int
}

func NewJobRecord(req JobRequest) JobRecord {
	return JobRecord{
		ID:             req.ID,
		Request:        req,
		Status:         StatusQueued,
		CompletedSteps: map[JobStep]bool{},
		StepElapsed:    map[JobStep]time.Duration{},
	}
}

func (r *JobRecord) ProgressFraction() float64 {
	if r.Status == StatusCompleted {
		return 1.0
	}
	completed := float64(len(r.CompletedSteps))
	active := 0.0
	if r.CurrentStep != nil {
		if r.CurrentStepProgress < 0 {
			active = 0
		} else if r.CurrentStepProgress > 1 {
			active = 1
		} else {
			active = r.CurrentStepProgress
		}
	}
	total := float64(len(AllSteps))
	if total <= 0 {
		return 0
	}
	fraction := (completed + active) / total
	if fraction > 1 {
		return 1
	}
	if fraction < 0 {
		return 0
	}
	return fraction
}

func (r *JobRecord) ProgressPercent() int {
	if (r.Status == StatusRunning || r.Status == StatusPaused) &&
		r.Request.SourceKind == SourceYouTube &&
		r.CurrentStep != nil &&
		*r.CurrentStep == StepDownload {
		pct := int(r.CurrentStepProgress*100 + 0.5)
		if pct < 0 {
			return 0
		}
		if pct > 100 {
			return 100
		}
		return pct
	}
	return int(r.ProgressFraction()*100 + 0.5)
}

func (r *JobRecord) TotalElapsed(now time.Time) time.Duration {
	if r.StartedAt == nil {
		return 0
	}
	end := now
	if r.EndedAt != nil {
		end = *r.EndedAt
	}
	if end.Before(*r.StartedAt) {
		return 0
	}
	return end.Sub(*r.StartedAt)
}

func (r *JobRecord) ActiveStepElapsed(now time.Time) time.Duration {
	if r.CurrentStep == nil || r.CurrentStepStartedAt == nil {
		return 0
	}
	end := now
	if r.EndedAt != nil {
		end = *r.EndedAt
	}
	if end.Before(*r.CurrentStepStartedAt) {
		return 0
	}
	return end.Sub(*r.CurrentStepStartedAt)
}

func (r *JobRecord) ElapsedForStep(step JobStep, now time.Time) time.Duration {
	total := time.Duration(0)
	if r.StepElapsed != nil {
		total = r.StepElapsed[step]
	}
	if r.CurrentStep != nil && *r.CurrentStep == step && r.CurrentStepStartedAt != nil {
		end := now
		if r.EndedAt != nil {
			end = *r.EndedAt
		}
		if !end.Before(*r.CurrentStepStartedAt) {
			total += end.Sub(*r.CurrentStepStartedAt)
		}
	}
	if total < 0 {
		return 0
	}
	return total
}

func (r *JobRecord) TranslationElapsed(now time.Time) time.Duration {
	if r.TranslationStartedAt == nil {
		return 0
	}
	end := now
	if r.TranslationEndedAt != nil {
		end = *r.TranslationEndedAt
	}
	if r.EndedAt != nil && r.EndedAt.Before(end) {
		end = *r.EndedAt
	}
	if end.Before(*r.TranslationStartedAt) {
		return 0
	}
	return end.Sub(*r.TranslationStartedAt)
}
