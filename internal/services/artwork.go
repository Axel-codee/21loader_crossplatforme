package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultArtworkTimeout   = 8 * time.Second
	defaultArtworkMaxRead   = 64 * 1024 * 1024
	defaultArtworkQuality   = 78
	minArtworkThumbnailSize = 48
	maxArtworkThumbnailSize = 256
)

type ArtworkThumbnail struct {
	Data        []byte
	ContentType string
}

type ArtworkThumbnailService struct {
	client   *http.Client
	cacheDir string

	mu       sync.Mutex
	inFlight map[string]*artworkTask
}

type artworkTask struct {
	wg      sync.WaitGroup
	payload ArtworkThumbnail
	err     error
}

func NewArtworkThumbnailService(cacheDir string) *ArtworkThumbnailService {
	cleanCacheDir := strings.TrimSpace(cacheDir)
	if cleanCacheDir == "" {
		cleanCacheDir = filepath.Join(os.TempDir(), "PersoDL", "Web", "RSSArtworkThumbnails")
	}
	_ = os.MkdirAll(cleanCacheDir, 0o755)

	client := &http.Client{
		Timeout: defaultArtworkTimeout + 2*time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        64,
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &ArtworkThumbnailService{
		client:   client,
		cacheDir: cleanCacheDir,
		inFlight: map[string]*artworkTask{},
	}
}

func (s *ArtworkThumbnailService) ThumbnailData(
	ctx context.Context,
	rawURL string,
	maxPixel int,
) (ArtworkThumbnail, error) {
	normalizedURL, err := normalizeArtworkURL(rawURL)
	if err != nil {
		return ArtworkThumbnail{}, err
	}

	clampedSize := clampInt(maxPixel, minArtworkThumbnailSize, maxArtworkThumbnailSize)
	cacheKey := artworkCacheKey(normalizedURL, clampedSize, defaultArtworkQuality)
	cacheFile := filepath.Join(s.cacheDir, cacheKey+".img")

	if cached, err := os.ReadFile(cacheFile); err == nil && len(cached) > 0 {
		return ArtworkThumbnail{
			Data:        cached,
			ContentType: normalizeImageContentType("", cached),
		}, nil
	}

	s.mu.Lock()
	if existing, ok := s.inFlight[cacheKey]; ok {
		s.mu.Unlock()
		existing.wg.Wait()
		if existing.err != nil {
			return ArtworkThumbnail{}, existing.err
		}
		return existing.payload, nil
	}
	task := &artworkTask{}
	task.wg.Add(1)
	s.inFlight[cacheKey] = task
	s.mu.Unlock()

	payload, buildErr := s.buildThumbnail(ctx, normalizedURL, clampedSize)
	if buildErr == nil {
		_ = os.MkdirAll(s.cacheDir, 0o755)
		_ = writeFileAtomically(cacheFile, payload.Data, 0o644)
	}

	task.payload = payload
	task.err = buildErr
	task.wg.Done()

	s.mu.Lock()
	delete(s.inFlight, cacheKey)
	s.mu.Unlock()

	if buildErr != nil {
		return ArtworkThumbnail{}, buildErr
	}
	return payload, nil
}

func (s *ArtworkThumbnailService) buildThumbnail(
	ctx context.Context,
	artworkURL string,
	maxPixel int,
) (ArtworkThumbnail, error) {
	requestCtx, cancel := context.WithTimeout(ctx, defaultArtworkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, artworkURL, nil)
	if err != nil {
		return ArtworkThumbnail{}, fmt.Errorf("URL d'illustration invalide")
	}
	req.Header.Set(
		"Accept",
		"image/avif,image/webp,image/apng,image/*;q=0.8,*/*;q=0.5",
	)
	req.Header.Set(
		"User-Agent",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) PersoDLWeb/1.0",
	)

	resp, err := s.client.Do(req)
	if err != nil {
		return ArtworkThumbnail{}, fmt.Errorf("impossible de telecharger l'illustration: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ArtworkThumbnail{}, fmt.Errorf("impossible de telecharger l'illustration (HTTP %d)", resp.StatusCode)
	}

	sourceData, err := io.ReadAll(io.LimitReader(resp.Body, defaultArtworkMaxRead))
	if err != nil || len(sourceData) == 0 {
		return ArtworkThumbnail{}, fmt.Errorf("impossible de lire l'illustration")
	}

	contentType := strings.ToLower(strings.TrimSpace(firstHeaderPart(resp.Header.Get("Content-Type"))))

	thumbnail, err := makeJPEGThumbnailWithStdlib(sourceData, maxPixel, defaultArtworkQuality)
	if err == nil {
		return ArtworkThumbnail{
			Data:        thumbnail,
			ContentType: "image/jpeg",
		}, nil
	}

	// If thumbnail conversion fails (webp/avif/other formats), return source bytes directly.
	// Browsers decode more image formats than Go's stdlib decoder.
	if isLikelyImageContentType(contentType) || looksLikeImageURL(artworkURL) || looksLikeImageBytes(sourceData) {
		return ArtworkThumbnail{
			Data:        sourceData,
			ContentType: normalizeImageContentType(contentType, sourceData),
		}, nil
	}

	return ArtworkThumbnail{}, fmt.Errorf("impossible de decoder l'illustration")
}

func normalizeArtworkURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("URL d'illustration invalide")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil {
		return "", fmt.Errorf("URL d'illustration invalide")
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("seules les URLs http/https sont acceptees pour les illustrations")
	}

	return parsed.String(), nil
}

func makeJPEGThumbnailWithStdlib(sourceData []byte, maxPixel int, quality int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(sourceData))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return nil, fmt.Errorf("image invalide")
	}

	targetWidth, targetHeight := fitWithinMaxPixel(sourceWidth, sourceHeight, maxPixel)
	resized := resizeNearest(src, targetWidth, targetHeight)

	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, resized, &jpeg.Options{
		Quality: clampInt(quality, 20, 90),
	}); err != nil {
		return nil, err
	}

	out := buffer.Bytes()
	if len(out) == 0 {
		return nil, fmt.Errorf("image compressee vide")
	}
	return out, nil
}

func fitWithinMaxPixel(width int, height int, maxPixel int) (int, int) {
	if width <= maxPixel && height <= maxPixel {
		return width, height
	}
	if width >= height {
		targetHeight := int(float64(height) * float64(maxPixel) / float64(width))
		return maxPixel, max(targetHeight, 1)
	}
	targetWidth := int(float64(width) * float64(maxPixel) / float64(height))
	return max(targetWidth, 1), maxPixel
}

func resizeNearest(src image.Image, width int, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	srcBounds := src.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	for y := 0; y < height; y++ {
		sourceY := srcBounds.Min.Y + (y * srcHeight / height)
		for x := 0; x < width; x++ {
			sourceX := srcBounds.Min.X + (x * srcWidth / width)
			dst.Set(x, y, src.At(sourceX, sourceY))
		}
	}
	return dst
}

func artworkCacheKey(url string, maxPixel int, quality int) string {
	source := fmt.Sprintf("%s|%d|%d", url, maxPixel, quality)
	digest := sha256.Sum256([]byte(source))
	return hex.EncodeToString(digest[:])
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) error {
	tmpPath := fmt.Sprintf("%s.tmp-%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func max(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func firstHeaderPart(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ";")
	return strings.TrimSpace(parts[0])
}

func isLikelyImageContentType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/")
}

func looksLikeImageURL(rawURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".gif") ||
		strings.HasSuffix(lower, ".webp") ||
		strings.HasSuffix(lower, ".avif") ||
		strings.HasSuffix(lower, ".heic") ||
		strings.HasSuffix(lower, ".heif") ||
		strings.HasSuffix(lower, ".svg")
}

func looksLikeImageBytes(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return strings.HasPrefix(strings.ToLower(http.DetectContentType(data)), "image/")
}

func normalizeImageContentType(rawHeader string, data []byte) string {
	header := strings.ToLower(strings.TrimSpace(firstHeaderPart(rawHeader)))
	if strings.HasPrefix(header, "image/") {
		return header
	}
	sniffed := strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
	if strings.HasPrefix(sniffed, "image/") {
		return sniffed
	}
	return "image/jpeg"
}
