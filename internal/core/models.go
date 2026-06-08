package core

import (
	"time"

	"21loader-cross/internal/xuuid"
)

type JobSourceKind string

type JobContentType string

type JobStatus string

type JobStep string

type DiarizationProvider string

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

const (
	DiarizationProviderNone        DiarizationProvider = "none"
	DiarizationProviderTinydiarize DiarizationProvider = "tinydiarize"
	DiarizationProviderPyannote    DiarizationProvider = "pyannote"
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
	WhisperModelPath             string               `json:"whisperModelPath"`
	WhisperVADEnabled            bool                 `json:"whisperVADEnabled"`
	WhisperVADModelPath          string               `json:"whisperVADModelPath"`
	WhisperVADThreshold          float64              `json:"whisperVADThreshold"`
	WhisperVADMinSpeechDuration  int                  `json:"whisperVADMinSpeechDuration"`
	WhisperVADMinSilenceDuration int                  `json:"whisperVADMinSilenceDuration"`
	WhisperVADSpeechPad          int                  `json:"whisperVADSpeechPad"`
	WhisperMaxSegmentLength      int                  `json:"whisperMaxSegmentLength"`
	WhisperSplitOnWord           bool                 `json:"whisperSplitOnWord"`
	WhisperInitialPrompt         string               `json:"whisperInitialPrompt"`
	WhisperCarryInitialPrompt    bool                 `json:"whisperCarryInitialPrompt"`
	WhisperOutputJSONFull        bool                 `json:"whisperOutputJSONFull"`
	DiarizationProvider          DiarizationProvider  `json:"diarizationProvider"`
	WhisperTinydiarizeEnabled    bool                 `json:"whisperTinydiarizeEnabled"`
	WhisperTinydiarizeModelPath  string               `json:"whisperTinydiarizeModelPath"`
	WhisperTinydiarizeOutputTXT  bool                 `json:"whisperTinydiarizeOutputTXT"`
	WhisperTinydiarizeOutputSRT  bool                 `json:"whisperTinydiarizeOutputSRT"`
	PyannoteHuggingFaceToken     string               `json:"pyannoteHuggingFaceToken"`
	PyannoteLocalPipelinePath    string               `json:"pyannoteLocalPipelinePath"`
	PyannoteOutputTXT            bool                 `json:"pyannoteOutputTXT"`
	PyannoteOutputSRT            bool                 `json:"pyannoteOutputSRT"`
	UseFirefoxCookies            bool                 `json:"useFirefoxCookies"`
	YouTubeAudioFormat           string               `json:"youtubeAudioFormat"`
	KeepTemporaryFilesOnFailure  bool                 `json:"keepTemporaryFilesOnFailure"`
	QobuzEmail                   string               `json:"qobuzEmail"`
	QobuzPassword                string               `json:"qobuzPassword"`
	QobuzUseUserAuthToken        bool                 `json:"qobuzUseUserAuthToken"`
	QobuzUserAuthToken           string               `json:"qobuzUserAuthToken"`
	DefaultOutputRoot            string               `json:"defaultOutputRoot"`
	FavoriteRSSPodcasts          []FavoriteRSSPodcast `json:"favoriteRSSPodcasts,omitempty"`
}

type FavoriteRSSPodcast struct {
	FeedURL                   string `json:"feedURL"`
	PodcastTitle              string `json:"podcastTitle"`
	PodcastArtworkURL         string `json:"podcastArtworkURL,omitempty"`
	WhisperInitialPrompt      string `json:"whisperInitialPrompt,omitempty"`
	WhisperCarryInitialPrompt bool   `json:"whisperCarryInitialPrompt"`
}

type RSSEpisodeAPIInput struct {
	Title           string `json:"title"`
	PublicationDate string `json:"publicationDate"`
	MediaURL        string `json:"mediaURL"`
	FeedURL         string `json:"feedURL,omitempty"`
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

type LRCLIBSearchAPIRequest struct {
	TrackName  string `json:"trackName"`
	ArtistName string `json:"artistName"`
	AlbumName  string `json:"albumName"`
	Limit      int    `json:"limit"`
}

type LRCLIBSearchResultDTO struct {
	ID           string `json:"id"`
	TrackName    string `json:"trackName"`
	ArtistName   string `json:"artistName"`
	AlbumName    string `json:"albumName,omitempty"`
	PlainLyrics  string `json:"plainLyrics,omitempty"`
	SyncedLyrics string `json:"syncedLyrics,omitempty"`
	Preview      string `json:"preview,omitempty"`
	HasSynced    bool   `json:"hasSynced"`
	Score        int    `json:"score"`
}

type LRCLIBSearchAPIResponse struct {
	Results []LRCLIBSearchResultDTO `json:"results"`
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

type ManualLyricsSelectionInput struct {
	TargetTrackName  string `json:"targetTrackName"`
	TargetArtistName string `json:"targetArtistName"`
	TargetAlbumName  string `json:"targetAlbumName"`
	TrackName        string `json:"trackName"`
	ArtistName       string `json:"artistName"`
	AlbumName        string `json:"albumName"`
	PlainLyrics      string `json:"plainLyrics"`
	SyncedLyrics     string `json:"syncedLyrics"`
}

type CreateJobAPIRequest struct {
	InputURL                     string                       `json:"inputURL"`
	SourceKind                   string                       `json:"sourceKind"`
	ContentType                  string                       `json:"contentType"`
	OutputRootPath               string                       `json:"outputRootPath"`
	CustomName                   string                       `json:"customName"`
	DisplayName                  string                       `json:"displayName"`
	TranscriptionLanguage        string                       `json:"transcriptionLanguage"`
	EnableTranscription          *bool                        `json:"enableTranscription"`
	EnableTranslation            *bool                        `json:"enableTranslation"`
	TranslationSourceLanguage    string                       `json:"translationSourceLanguage"`
	TranslationTargetLanguage    string                       `json:"translationTargetLanguage"`
	EnableLyrics                 *bool                        `json:"enableLyrics"`
	UseCustomLyricsSearch        *bool                        `json:"useCustomLyricsSearch"`
	LyricsSearchTitle            string                       `json:"lyricsSearchTitle"`
	LyricsSearchArtist           string                       `json:"lyricsSearchArtist"`
	LyricsSearchAlbum            string                       `json:"lyricsSearchAlbum"`
	UseManualLyricsSelection     *bool                        `json:"useManualLyricsSelection"`
	ManualLyricsTrackName        string                       `json:"manualLyricsTrackName"`
	ManualLyricsArtistName       string                       `json:"manualLyricsArtistName"`
	ManualLyricsAlbumName        string                       `json:"manualLyricsAlbumName"`
	ManualLyricsPlain            string                       `json:"manualLyricsPlain"`
	ManualLyricsSynced           string                       `json:"manualLyricsSynced"`
	ManualLyricsSelections       []ManualLyricsSelectionInput `json:"manualLyricsSelections"`
	WhisperModelPath             string                       `json:"whisperModelPath"`
	WhisperVADEnabled            *bool                        `json:"whisperVADEnabled"`
	WhisperVADModelPath          string                       `json:"whisperVADModelPath"`
	WhisperVADThreshold          *float64                     `json:"whisperVADThreshold"`
	WhisperVADMinSpeechDuration  *int                         `json:"whisperVADMinSpeechDuration"`
	WhisperVADMinSilenceDuration *int                         `json:"whisperVADMinSilenceDuration"`
	WhisperVADSpeechPad          *int                         `json:"whisperVADSpeechPad"`
	WhisperMaxSegmentLength      *int                         `json:"whisperMaxSegmentLength"`
	WhisperSplitOnWord           *bool                        `json:"whisperSplitOnWord"`
	WhisperPromptEnabled         *bool                        `json:"whisperPromptEnabled"`
	WhisperInitialPrompt         string                       `json:"whisperInitialPrompt"`
	WhisperCarryInitialPrompt    *bool                        `json:"whisperCarryInitialPrompt"`
	WhisperOutputJSONFull        *bool                        `json:"whisperOutputJSONFull"`
	DiarizationProvider          string                       `json:"diarizationProvider"`
	WhisperTinydiarizeEnabled    *bool                        `json:"whisperTinydiarizeEnabled"`
	WhisperTinydiarizeModelPath  string                       `json:"whisperTinydiarizeModelPath"`
	WhisperTinydiarizeOutputTXT  *bool                        `json:"whisperTinydiarizeOutputTXT"`
	WhisperTinydiarizeOutputSRT  *bool                        `json:"whisperTinydiarizeOutputSRT"`
	PyannoteHuggingFaceToken     string                       `json:"pyannoteHuggingFaceToken"`
	PyannoteLocalPipelinePath    string                       `json:"pyannoteLocalPipelinePath"`
	PyannoteOutputTXT            *bool                        `json:"pyannoteOutputTXT"`
	PyannoteOutputSRT            *bool                        `json:"pyannoteOutputSRT"`
	YtDlpExtraArguments          string                       `json:"ytDlpExtraArguments"`
	YouTubeAudioFormat           string                       `json:"youtubeAudioFormat"`
	WhisperExtraArguments        string                       `json:"whisperExtraArguments"`
	FfmpegExtraArguments         string                       `json:"ffmpegExtraArguments"`
	QobuzExtraArguments          string                       `json:"qobuzExtraArguments"`
	UseFirefoxCookies            *bool                        `json:"useFirefoxCookies"`
	QobuzEmail                   string                       `json:"qobuzEmail"`
	QobuzPassword                string                       `json:"qobuzPassword"`
	QobuzUseUserAuthToken        *bool                        `json:"qobuzUseUserAuthToken"`
	QobuzUserAuthToken           string                       `json:"qobuzUserAuthToken"`
	QobuzArtistName              string                       `json:"qobuzArtistName"`
	QobuzPlaylistName            string                       `json:"qobuzPlaylistName"`
	CollisionPolicy              string                       `json:"collisionPolicy"`
	QobuzExistingAlbumPolicy     string                       `json:"qobuzExistingAlbumPolicy"`
	RSSEpisode                   *RSSEpisodeAPIInput          `json:"rssEpisode"`
}

type UpdateSettingsAPIRequest struct {
	WhisperModelPath             *string               `json:"whisperModelPath"`
	WhisperVADEnabled            *bool                 `json:"whisperVADEnabled"`
	WhisperVADModelPath          *string               `json:"whisperVADModelPath"`
	WhisperVADThreshold          *float64              `json:"whisperVADThreshold"`
	WhisperVADMinSpeechDuration  *int                  `json:"whisperVADMinSpeechDuration"`
	WhisperVADMinSilenceDuration *int                  `json:"whisperVADMinSilenceDuration"`
	WhisperVADSpeechPad          *int                  `json:"whisperVADSpeechPad"`
	WhisperMaxSegmentLength      *int                  `json:"whisperMaxSegmentLength"`
	WhisperSplitOnWord           *bool                 `json:"whisperSplitOnWord"`
	WhisperInitialPrompt         *string               `json:"whisperInitialPrompt"`
	WhisperCarryInitialPrompt    *bool                 `json:"whisperCarryInitialPrompt"`
	WhisperOutputJSONFull        *bool                 `json:"whisperOutputJSONFull"`
	DiarizationProvider          *string               `json:"diarizationProvider"`
	WhisperTinydiarizeEnabled    *bool                 `json:"whisperTinydiarizeEnabled"`
	WhisperTinydiarizeModelPath  *string               `json:"whisperTinydiarizeModelPath"`
	WhisperTinydiarizeOutputTXT  *bool                 `json:"whisperTinydiarizeOutputTXT"`
	WhisperTinydiarizeOutputSRT  *bool                 `json:"whisperTinydiarizeOutputSRT"`
	PyannoteHuggingFaceToken     *string               `json:"pyannoteHuggingFaceToken"`
	PyannoteLocalPipelinePath    *string               `json:"pyannoteLocalPipelinePath"`
	PyannoteOutputTXT            *bool                 `json:"pyannoteOutputTXT"`
	PyannoteOutputSRT            *bool                 `json:"pyannoteOutputSRT"`
	UseFirefoxCookies            *bool                 `json:"useFirefoxCookies"`
	YouTubeAudioFormat           *string               `json:"youtubeAudioFormat"`
	KeepTemporaryFilesOnFailure  *bool                 `json:"keepTemporaryFilesOnFailure"`
	QobuzEmail                   *string               `json:"qobuzEmail"`
	QobuzPassword                *string               `json:"qobuzPassword"`
	QobuzUseUserAuthToken        *bool                 `json:"qobuzUseUserAuthToken"`
	QobuzUserAuthToken           *string               `json:"qobuzUserAuthToken"`
	DefaultOutputRoot            *string               `json:"defaultOutputRoot"`
	FavoriteRSSPodcasts          *[]FavoriteRSSPodcast `json:"favoriteRSSPodcasts"`
}

type QobuzArtistCatalogAPIRequest struct {
	ArtistURL             string `json:"artistURL"`
	QobuzEmail            string `json:"qobuzEmail"`
	QobuzPassword         string `json:"qobuzPassword"`
	QobuzUseUserAuthToken *bool  `json:"qobuzUseUserAuthToken"`
	QobuzUserAuthToken    string `json:"qobuzUserAuthToken"`
}

type QobuzArtistSearchAPIRequest struct {
	Query                 string `json:"query"`
	Limit                 int    `json:"limit"`
	QobuzEmail            string `json:"qobuzEmail"`
	QobuzPassword         string `json:"qobuzPassword"`
	QobuzUseUserAuthToken *bool  `json:"qobuzUseUserAuthToken"`
	QobuzUserAuthToken    string `json:"qobuzUserAuthToken"`
}

type QobuzAlbumTracksAPIRequest struct {
	AlbumID               string `json:"albumID"`
	QobuzEmail            string `json:"qobuzEmail"`
	QobuzPassword         string `json:"qobuzPassword"`
	QobuzUseUserAuthToken *bool  `json:"qobuzUseUserAuthToken"`
	QobuzUserAuthToken    string `json:"qobuzUserAuthToken"`
}

type QobuzPlaylistCatalogAPIRequest struct {
	PlaylistURL           string `json:"playlistURL"`
	QobuzEmail            string `json:"qobuzEmail"`
	QobuzPassword         string `json:"qobuzPassword"`
	QobuzUseUserAuthToken *bool  `json:"qobuzUseUserAuthToken"`
	QobuzUserAuthToken    string `json:"qobuzUserAuthToken"`
}

type QobuzCredentialsCheckAPIRequest struct {
	QobuzEmail            string `json:"qobuzEmail"`
	QobuzPassword         string `json:"qobuzPassword"`
	QobuzUseUserAuthToken *bool  `json:"qobuzUseUserAuthToken"`
	QobuzUserAuthToken    string `json:"qobuzUserAuthToken"`
}

type QobuzCredentialsCheckAPIResponse struct {
	OK                     bool   `json:"ok"`
	Message                string `json:"message"`
	Email                  string `json:"email,omitempty"`
	MembershipLabel        string `json:"membershipLabel,omitempty"`
	AuthMode               string `json:"authMode,omitempty"`
	RefreshedUserAuthToken string `json:"-"`
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
	MediaPath                 string `json:"mediaPath"`
	SubtitlePath              string `json:"subtitlePath,omitempty"`
	TranscriptPath            string `json:"transcriptPath,omitempty"`
	JSONPath                  string `json:"jsonPath,omitempty"`
	TinydiarizeJSONPath       string `json:"tinydiarizeJSONPath,omitempty"`
	TinydiarizeTranscriptPath string `json:"tinydiarizeTranscriptPath,omitempty"`
	TinydiarizeSubtitlePath   string `json:"tinydiarizeSubtitlePath,omitempty"`
	PyannoteJSONPath          string `json:"pyannoteJSONPath,omitempty"`
	PyannoteTranscriptPath    string `json:"pyannoteTranscriptPath,omitempty"`
	PyannoteSubtitlePath      string `json:"pyannoteSubtitlePath,omitempty"`
	MetadataPath              string `json:"metadataPath"`
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
	ReusedSteps                 []string      `json:"reusedSteps,omitempty"`
	StartedAt                   *time.Time    `json:"startedAt"`
	EndedAt                     *time.Time    `json:"endedAt"`
	CurrentStepStartedAt        *time.Time    `json:"currentStepStartedAt,omitempty"`
	TotalElapsedSeconds         int64         `json:"totalElapsedSeconds,omitempty"`
	ActiveStepElapsedSeconds    int64         `json:"activeStepElapsedSeconds,omitempty"`
	DownloadElapsedSeconds      int64         `json:"downloadElapsedSeconds,omitempty"`
	LyricsElapsedSeconds        int64         `json:"lyricsElapsedSeconds,omitempty"`
	TranscriptionElapsedSeconds int64         `json:"transcriptionElapsedSeconds,omitempty"`
	TranslationStatus           string        `json:"translationStatus,omitempty"`
	TranslationReused           bool          `json:"translationReused,omitempty"`
	TranslationElapsedSeconds   int64         `json:"translationElapsedSeconds,omitempty"`
	ErrorMessage                string        `json:"errorMessage,omitempty"`
	IsPauseRequested            bool          `json:"isPauseRequested"`
	Result                      *JobResultDTO `json:"result,omitempty"`
	LogsSize                    int           `json:"logsSize"`
	QobuzTracksDone             int           `json:"qobuzTracksDone,omitempty"`
	QobuzTracksTotal            int           `json:"qobuzTracksTotal,omitempty"`
	QobuzTracksUnavailable      int           `json:"qobuzTracksUnavailable,omitempty"`
	LyricsTracksDone            int           `json:"lyricsTracksDone,omitempty"`
	LyricsTracksTotal           int           `json:"lyricsTracksTotal,omitempty"`
	LyricsFound                 int           `json:"lyricsFound,omitempty"`
	LyricsFoundTotal            int           `json:"lyricsFoundTotal,omitempty"`
	LyricsFailed                int           `json:"lyricsFailed,omitempty"`
}

type DashboardResponseDTO struct {
	ServerTime  time.Time       `json:"serverTime"`
	Version     string          `json:"version,omitempty"`
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
	Status  string    `json:"status"`
	Time    time.Time `json:"time"`
	Version string    `json:"version,omitempty"`
	PID     int       `json:"pid,omitempty"`
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
	State       string `json:"state,omitempty"`
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

type PyannoteAccessCheckRequest struct {
	Token             string `json:"token"`
	LocalPipelinePath string `json:"localPipelinePath"`
}

type PyannoteAccessCheckResponse struct {
	OK         bool                `json:"ok"`
	Message    string              `json:"message"`
	Diagnostic WebBinaryDiagnostic `json:"diagnostic"`
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

type VADModelInfoDTO struct {
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

type VADModelsResponse struct {
	ModelDirectory    string            `json:"modelDirectory"`
	SelectedModelPath string            `json:"selectedModelPath,omitempty"`
	SelectedModelID   string            `json:"selectedModelID,omitempty"`
	Models            []VADModelInfoDTO `json:"models"`
}

type VADModelInstallRequest struct {
	ModelID string `json:"modelID"`
}

type VADModelInstallProgressRequest struct {
	ModelID string `json:"modelID"`
}

type VADModelInstallProgressResponse struct {
	ModelID         string    `json:"modelID"`
	Active          bool      `json:"active"`
	Stage           string    `json:"stage"`
	Message         string    `json:"message,omitempty"`
	DownloadedBytes int64     `json:"downloadedBytes,omitempty"`
	TotalBytes      int64     `json:"totalBytes,omitempty"`
	ProgressPercent int       `json:"progressPercent"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
}

type VADModelInstallResponse struct {
	OK      bool            `json:"ok"`
	Message string          `json:"message"`
	Model   VADModelInfoDTO `json:"model"`
}

type VADModelUninstallRequest struct {
	ModelID string `json:"modelID"`
}

type VADModelUninstallResponse struct {
	OK                      bool            `json:"ok"`
	Message                 string          `json:"message"`
	Model                   VADModelInfoDTO `json:"model"`
	RemovedPath             string          `json:"removedPath,omitempty"`
	ClearedDefaultSelection bool            `json:"clearedDefaultSelection,omitempty"`
}

type RSSEpisodeSelection struct {
	Title           string
	PublicationDate *time.Time
	MediaURL        string
	FeedURL         string
	PodcastTitle    string
	ArtworkURL      string
}

type ManualLyricsSelection struct {
	TargetTrackName  string
	TargetArtistName string
	TargetAlbumName  string
	TrackName        string
	ArtistName       string
	AlbumName        string
	PlainLyrics      string
	SyncedLyrics     string
}

type JobRequest struct {
	ID                           xuuid.UUID
	CreatedAt                    time.Time
	SourceKind                   JobSourceKind
	ContentType                  JobContentType
	InputURL                     string
	SelectedRSSEpisode           *RSSEpisodeSelection
	TranscriptionLanguage        string
	EnableTranscription          bool
	EnableTranslation            bool
	TranslationSourceLanguage    string
	TranslationTargetLanguage    string
	EnableLyrics                 bool
	UseCustomLyricsSearch        bool
	LyricsSearchTitle            string
	LyricsSearchArtist           string
	LyricsSearchAlbum            string
	UseManualLyricsSelection     bool
	ManualLyricsTrackName        string
	ManualLyricsArtistName       string
	ManualLyricsAlbumName        string
	ManualLyricsPlain            string
	ManualLyricsSynced           string
	ManualLyricsSelections       []ManualLyricsSelection
	WhisperModelPath             string
	WhisperVADEnabled            bool
	WhisperVADModelPath          string
	WhisperVADThreshold          float64
	WhisperVADMinSpeechDuration  int
	WhisperVADMinSilenceDuration int
	WhisperVADSpeechPad          int
	WhisperMaxSegmentLength      int
	WhisperSplitOnWord           bool
	WhisperPromptEnabled         bool
	WhisperInitialPrompt         string
	WhisperCarryInitialPrompt    bool
	WhisperOutputJSONFull        bool
	DiarizationProvider          DiarizationProvider
	WhisperTinydiarizeEnabled    bool
	WhisperTinydiarizeModelPath  string
	WhisperTinydiarizeOutputTXT  bool
	WhisperTinydiarizeOutputSRT  bool
	PyannoteHuggingFaceToken     string
	PyannoteLocalPipelinePath    string
	PyannoteOutputTXT            bool
	PyannoteOutputSRT            bool
	YtDlpExtraArguments          string
	YouTubeAudioFormat           string
	WhisperExtraArguments        string
	FfmpegExtraArguments         string
	QobuzExtraArguments          string
	OutputRootPath               string
	CustomName                   string
	UseFirefoxCookies            bool
	QobuzEmail                   string
	QobuzPassword                string
	QobuzUseUserAuthToken        bool
	QobuzUserAuthToken           string
	QobuzArtistName              string
	QobuzPlaylistName            string
}

type JobResult struct {
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
}

type JobRecord struct {
	ID                     xuuid.UUID
	Request                JobRequest
	Status                 JobStatus
	CurrentStep            *JobStep
	CurrentStepStartedAt   *time.Time
	StepElapsed            map[JobStep]time.Duration
	TranslationStatus      string
	TranslationStartedAt   *time.Time
	TranslationEndedAt     *time.Time
	CurrentStepProgress    float64
	CompletedSteps         map[JobStep]bool
	ReusedSteps            map[JobStep]bool
	StartedAt              *time.Time
	EndedAt                *time.Time
	ErrorMessage           string
	Result                 *JobResult
	Logs                   string
	IsPauseRequested       bool
	QobuzTracksDone        int
	QobuzTracksTotal       int
	QobuzTracksUnavailable int
	LyricsTracksDone       int
	LyricsTracksTotal      int
	LyricsFound            int
	LyricsFoundTotal       int
	LyricsFailed           int
	TranslationReused      bool
}

func NewJobRecord(req JobRequest) JobRecord {
	return JobRecord{
		ID:             req.ID,
		Request:        req,
		Status:         StatusQueued,
		CompletedSteps: map[JobStep]bool{},
		ReusedSteps:    map[JobStep]bool{},
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
