package jobs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"persodl-cross/internal/core"
	"persodl-cross/internal/util"
)

type MediaMetadata struct {
	Title                 string             `json:"title"`
	SourceName            string             `json:"sourceName"`
	SourceKind            core.JobSourceKind `json:"sourceKind"`
	PublicationDate       *time.Time         `json:"publicationDate,omitempty"`
	DownloadedAt          time.Time          `json:"downloadedAt"`
	OriginalInputURL      string             `json:"originalInputURL"`
	MediaPath             string             `json:"mediaPath"`
	SubtitlePath          string             `json:"subtitlePath,omitempty"`
	TranscriptPath        string             `json:"transcriptPath,omitempty"`
	ArtworkPath           string             `json:"artworkPath,omitempty"`
	TranscriptionLanguage string             `json:"transcriptionLanguage"`
}

type OrganizationPayload struct {
	SourceKind            core.JobSourceKind
	SourceName            string
	Title                 string
	PublicationDate       *time.Time
	OriginalInputURL      string
	MediaPath             string
	IsMediaDirectory      bool
	SubtitleFile          string
	TranscriptFile        string
	ArtworkFile           string
	CustomName            string
	OutputRoot            string
	TranscriptionLanguage string
}

type Organizer struct{}

var renamePath = os.Rename

func NewOrganizer() *Organizer {
	return &Organizer{}
}

func (o *Organizer) Organize(payload OrganizationPayload, collision core.CollisionDecision) (core.JobResult, error) {
	if strings.TrimSpace(payload.OutputRoot) == "" || payload.OutputRoot == "/" {
		return core.JobResult{}, fmt.Errorf("le dossier racine de medias est invalide")
	}
	if payload.IsMediaDirectory {
		return o.organizeDirectory(payload, collision)
	}
	return o.organizeFile(payload, collision)
}

func (o *Organizer) organizeFile(payload OrganizationPayload, collision core.CollisionDecision) (core.JobResult, error) {
	top := topFolderName(payload.SourceKind)
	sourceFolderName := util.SanitizePathComponent(payload.SourceName, 96)
	sourceDir := filepath.Join(payload.OutputRoot, top, sourceFolderName)
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return core.JobResult{}, err
	}
	itemFolderName := folderName(payload)
	itemDir := filepath.Join(sourceDir, itemFolderName)
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		return core.JobResult{}, err
	}
	preferredName := strings.TrimSpace(payload.CustomName)
	baseName := itemFolderName
	if preferredName != "" {
		baseName = util.SanitizePathComponent(preferredName, 140)
	}
	mediaExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(payload.MediaPath)), ".")
	if mediaExt == "" {
		mediaExt = "bin"
	}
	selectedBase := baseName
	mediaTarget := filepath.Join(itemDir, selectedBase+"."+mediaExt)

	if existsAnyTarget(itemDir, selectedBase, mediaExt) {
		switch collision {
		case core.CollisionOverwrite:
			if err := removeExistingTargets(itemDir, selectedBase, mediaExt); err != nil {
				return core.JobResult{}, err
			}
		case core.CollisionRename:
			selectedBase = uniqueName(itemDir, selectedBase, mediaExt)
			mediaTarget = filepath.Join(itemDir, selectedBase+"."+mediaExt)
		case core.CollisionComplete:
			// Keep existing files and only fill missing artifacts.
		default:
			return core.JobResult{}, fmt.Errorf("job annule suite a une collision de fichiers")
		}
	}

	subtitleTarget := filepath.Join(itemDir, selectedBase+".srt")
	transcriptTarget := filepath.Join(itemDir, selectedBase+".txt")
	metadataTarget := filepath.Join(itemDir, selectedBase+".json")
	artworkTarget := ""
	if strings.TrimSpace(payload.ArtworkFile) != "" {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(payload.ArtworkFile)), ".")
		if ext == "" {
			ext = "jpg"
		}
		artworkTarget = filepath.Join(itemDir, selectedBase+".cover."+ext)
	}

	if strings.TrimSpace(payload.MediaPath) != "" && !samePath(payload.MediaPath, mediaTarget) {
		if collision == core.CollisionComplete {
			if _, err := os.Stat(mediaTarget); os.IsNotExist(err) {
				if err := moveReplacing(payload.MediaPath, mediaTarget); err != nil {
					return core.JobResult{}, err
				}
			}
		} else {
			if err := moveReplacing(payload.MediaPath, mediaTarget); err != nil {
				return core.JobResult{}, err
			}
		}
	}
	if strings.TrimSpace(payload.SubtitleFile) != "" {
		if !samePath(payload.SubtitleFile, subtitleTarget) {
			if collision == core.CollisionComplete {
				if _, err := os.Stat(subtitleTarget); os.IsNotExist(err) {
					if err := moveReplacing(payload.SubtitleFile, subtitleTarget); err != nil {
						return core.JobResult{}, err
					}
				}
			} else {
				if err := moveReplacing(payload.SubtitleFile, subtitleTarget); err != nil {
					return core.JobResult{}, err
				}
			}
		}
	}
	if strings.TrimSpace(payload.TranscriptFile) != "" {
		if !samePath(payload.TranscriptFile, transcriptTarget) {
			if collision == core.CollisionComplete {
				if _, err := os.Stat(transcriptTarget); os.IsNotExist(err) {
					if err := moveReplacing(payload.TranscriptFile, transcriptTarget); err != nil {
						return core.JobResult{}, err
					}
				}
			} else {
				if err := moveReplacing(payload.TranscriptFile, transcriptTarget); err != nil {
					return core.JobResult{}, err
				}
			}
		}
	}
	if artworkTarget != "" {
		if !samePath(payload.ArtworkFile, artworkTarget) {
			if collision == core.CollisionComplete {
				if _, err := os.Stat(artworkTarget); os.IsNotExist(err) {
					if err := moveReplacing(payload.ArtworkFile, artworkTarget); err != nil {
						return core.JobResult{}, err
					}
				}
			} else {
				if err := moveReplacing(payload.ArtworkFile, artworkTarget); err != nil {
					return core.JobResult{}, err
				}
			}
		}
	}

	metadata := MediaMetadata{
		Title:                 payload.Title,
		SourceName:            payload.SourceName,
		SourceKind:            payload.SourceKind,
		PublicationDate:       payload.PublicationDate,
		DownloadedAt:          time.Now().UTC(),
		OriginalInputURL:      payload.OriginalInputURL,
		MediaPath:             mediaTarget,
		TranscriptionLanguage: payload.TranscriptionLanguage,
	}
	if _, err := os.Stat(subtitleTarget); err == nil {
		metadata.SubtitlePath = subtitleTarget
	}
	if _, err := os.Stat(transcriptTarget); err == nil {
		metadata.TranscriptPath = transcriptTarget
	}
	if artworkTarget != "" {
		if _, err := os.Stat(artworkTarget); err == nil {
			metadata.ArtworkPath = artworkTarget
		}
	}
	if err := writeJSON(metadataTarget, metadata); err != nil {
		return core.JobResult{}, err
	}

	result := core.JobResult{MediaPath: mediaTarget, MetadataPath: metadataTarget}
	if metadata.SubtitlePath != "" {
		result.SubtitlePath = metadata.SubtitlePath
	}
	if metadata.TranscriptPath != "" {
		result.TranscriptPath = metadata.TranscriptPath
	}
	return result, nil
}

func (o *Organizer) organizeDirectory(payload OrganizationPayload, collision core.CollisionDecision) (core.JobResult, error) {
	top := topFolderName(payload.SourceKind)
	sourceFolderName := util.SanitizePathComponent(payload.SourceName, 96)
	sourceDir := filepath.Join(payload.OutputRoot, top, sourceFolderName)
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return core.JobResult{}, err
	}
	preferredName := strings.TrimSpace(payload.CustomName)
	albumFolder := filepath.Base(payload.MediaPath)
	if preferredName != "" {
		albumFolder = util.SanitizePathComponent(preferredName, 140)
	}
	albumDir := filepath.Join(sourceDir, albumFolder)
	sourceAlbum, _ := filepath.Abs(payload.MediaPath)
	targetAlbum, _ := filepath.Abs(albumDir)
	same := sourceAlbum == targetAlbum

	if !same {
		merged := false
		if _, err := os.Stat(albumDir); err == nil {
			switch collision {
			case core.CollisionOverwrite:
				if err := os.RemoveAll(albumDir); err != nil {
					return core.JobResult{}, err
				}
			case core.CollisionRename:
				albumDir = filepath.Join(sourceDir, uniqueDirectoryName(sourceDir, albumFolder))
			case core.CollisionComplete:
				if err := mergeDirectoryMissing(payload.MediaPath, albumDir); err != nil {
					return core.JobResult{}, err
				}
				merged = true
			default:
				return core.JobResult{}, fmt.Errorf("job annule suite a une collision de fichiers")
			}
		}
		if !merged {
			if err := moveReplacing(payload.MediaPath, albumDir); err != nil {
				return core.JobResult{}, err
			}
		}
	}

	metadataTarget := filepath.Join(albumDir, "album.json")
	metadata := MediaMetadata{
		Title:                 payload.Title,
		SourceName:            payload.SourceName,
		SourceKind:            payload.SourceKind,
		PublicationDate:       payload.PublicationDate,
		DownloadedAt:          time.Now().UTC(),
		OriginalInputURL:      payload.OriginalInputURL,
		MediaPath:             albumDir,
		TranscriptionLanguage: payload.TranscriptionLanguage,
	}
	if err := writeJSON(metadataTarget, metadata); err != nil {
		return core.JobResult{}, err
	}

	return core.JobResult{MediaPath: albumDir, MetadataPath: metadataTarget}, nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func topFolderName(source core.JobSourceKind) string {
	switch source {
	case core.SourceYouTube:
		return "YouTube"
	case core.SourceRSS:
		return "RSS"
	case core.SourceQobuz:
		return "qobuz"
	default:
		return "Media"
	}
}

func folderName(payload OrganizationPayload) string {
	switch payload.SourceKind {
	case core.SourceRSS:
		return util.SanitizePathComponent(payload.Title, 140)
	case core.SourceYouTube:
		uploader := util.SanitizePathComponent(payload.SourceName, 80)
		date := "NA"
		if payload.PublicationDate != nil {
			date = payload.PublicationDate.UTC().Format("20060102")
		}
		title := util.SanitizePathComponent(payload.Title, 120)
		return util.SanitizePathComponent(uploader+" - "+date+" - "+title, 180)
	case core.SourceQobuz:
		return util.SanitizePathComponent(payload.Title, 140)
	default:
		return util.SanitizePathComponent(payload.Title, 140)
	}
}

func moveReplacing(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	if err := renamePath(src, dst); err != nil {
		if !isCrossDeviceLinkError(err) {
			return err
		}
		return copyAcrossDevices(src, dst)
	}
	return nil
}

func copyAcrossDevices(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := copyDirectory(src, dst); err != nil {
			_ = os.RemoveAll(dst)
			return err
		}
		return os.RemoveAll(src)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("type de fichier non supporte pour un deplacement inter-volume: %s", src)
	}
	if err := copyRegularFile(src, dst, info.Mode().Perm()); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return os.Remove(src)
}

func copyDirectory(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("type de fichier non supporte pour un deplacement inter-volume: %s", srcPath)
		}
		if err := copyRegularFile(srcPath, dstPath, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyRegularFile(srcFile, dstFile string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dstFile), 0o755); err != nil {
		return err
	}
	in, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer in.Close()
	if mode == 0 {
		mode = 0o644
	}
	tmpFile := dstFile + ".tmp-copy"
	out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmpFile)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpFile)
		return closeErr
	}
	if err := os.Rename(tmpFile, dstFile); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}
	return nil
}

func isCrossDeviceLinkError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cross-device link") ||
		strings.Contains(msg, "different disk drive") ||
		strings.Contains(msg, "not same device")
}

func mergeDirectoryMissing(sourceDir, targetDir string) error {
	sourceDir = strings.TrimSpace(sourceDir)
	targetDir = strings.TrimSpace(targetDir)
	if sourceDir == "" || targetDir == "" || samePath(sourceDir, targetDir) {
		return nil
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	if err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dst := filepath.Join(targetDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if _, err := os.Stat(dst); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(path, dst); err == nil {
			return nil
		}
		return moveReplacing(path, dst)
	}); err != nil {
		return err
	}
	return os.RemoveAll(sourceDir)
}

func existsAnyTarget(dir, base, ext string) bool {
	candidates := []string{
		filepath.Join(dir, base+"."+ext),
		filepath.Join(dir, base+".srt"),
		filepath.Join(dir, base+".txt"),
		filepath.Join(dir, base+".json"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	entries, _ := os.ReadDir(dir)
	prefix := base + ".cover."
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			return true
		}
	}
	return false
}

func removeExistingTargets(dir, base, ext string) error {
	candidates := []string{
		filepath.Join(dir, base+"."+ext),
		filepath.Join(dir, base+".srt"),
		filepath.Join(dir, base+".txt"),
		filepath.Join(dir, base+".json"),
	}
	for _, p := range candidates {
		_ = os.RemoveAll(p)
	}
	entries, _ := os.ReadDir(dir)
	prefix := base + ".cover."
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

func uniqueName(dir, base, ext string) string {
	idx := 1
	for {
		candidate := fmt.Sprintf("%s (%d)", base, idx)
		if !existsAnyTarget(dir, candidate, ext) {
			return candidate
		}
		idx++
	}
}

func uniqueDirectoryName(dir, base string) string {
	idx := 1
	for {
		candidate := fmt.Sprintf("%s (%d)", base, idx)
		if _, err := os.Stat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
		idx++
	}
}

func samePath(left, right string) bool {
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
