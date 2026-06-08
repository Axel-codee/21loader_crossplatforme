package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const DefaultRepo = "Axel-codee/21loader_crossplatforme"

type Options struct {
	Repo           string
	CurrentVersion string
	HTTPClient     *http.Client
	Output         io.Writer
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Assets  []ReleaseAsset `json:"assets"`
}

type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func Update(ctx context.Context, opts Options) error {
	repo := strings.TrimSpace(opts.Repo)
	if repo == "" {
		repo = DefaultRepo
	}
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}

	release, err := fetchLatestRelease(ctx, client, repo)
	if err != nil {
		return err
	}
	asset, err := SelectReleaseAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	versionNote := strings.TrimSpace(release.TagName)
	if versionNote == "" {
		versionNote = strings.TrimSpace(release.Name)
	}
	if versionNote == "" {
		versionNote = "latest"
	}

	fmt.Fprintf(out, "Derniere release: %s\n", versionNote)
	fmt.Fprintf(out, "Asset selectionne: %s\n", asset.Name)

	packagePath, err := downloadAsset(ctx, client, *asset, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Package telecharge: %s\n", packagePath)

	message, err := ApplyPackage(ctx, packagePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(message) != "" {
		fmt.Fprintln(out, message)
	}
	return nil
}

func fetchLatestRelease(ctx context.Context, client *http.Client, repo string) (releaseInfo, error) {
	url := "https://api.github.com/repos/" + strings.Trim(repo, "/") + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "21loader-updater")

	resp, err := client.Do(req)
	if err != nil {
		return releaseInfo{}, fmt.Errorf("impossible de contacter GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = resp.Status
		}
		return releaseInfo{}, fmt.Errorf("GitHub releases indisponible: %s", detail)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return releaseInfo{}, fmt.Errorf("reponse GitHub invalide: %w", err)
	}
	if len(release.Assets) == 0 {
		return releaseInfo{}, errors.New("la derniere release GitHub ne contient aucun asset installable")
	}
	return release, nil
}

func SelectReleaseAsset(assets []ReleaseAsset, goos, goarch string) (*ReleaseAsset, error) {
	var best *ReleaseAsset
	bestScore := -1
	for i := range assets {
		asset := assets[i]
		score := assetScore(asset.Name, goos, goarch)
		if score > bestScore {
			best = &assets[i]
			bestScore = score
		}
	}
	if best == nil || bestScore < 0 {
		return nil, fmt.Errorf("aucun asset compatible avec %s/%s dans la derniere release", goos, goarch)
	}
	return best, nil
}

func assetScore(name, goos, goarch string) int {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" || !strings.Contains(lower, "21loader") {
		return -1
	}
	score := 0
	switch goos {
	case "windows":
		if strings.HasSuffix(lower, "-setup.exe") {
			score += 100
		} else if strings.HasSuffix(lower, ".zip") {
			score += 70
		} else {
			return -1
		}
		if archMatches(lower, goarch) {
			score += 20
		}
	case "darwin":
		if strings.HasSuffix(lower, ".dmg") {
			score += 100
		} else {
			return -1
		}
		if archMatches(lower, goarch) {
			score += 20
		}
	case "linux":
		if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".zip") {
			score += 60
		} else {
			return -1
		}
		if archMatches(lower, goarch) {
			score += 20
		}
	default:
		return -1
	}
	return score
}

func archMatches(name, goarch string) bool {
	switch goarch {
	case "amd64":
		return strings.Contains(name, "amd64") || strings.Contains(name, "x64") || strings.Contains(name, "x86_64")
	case "arm64":
		return strings.Contains(name, "arm64") || strings.Contains(name, "aarch64")
	default:
		return strings.Contains(name, goarch)
	}
}

func downloadAsset(ctx context.Context, client *http.Client, asset ReleaseAsset, out io.Writer) (string, error) {
	url := strings.TrimSpace(asset.BrowserDownloadURL)
	if url == "" {
		return "", fmt.Errorf("asset GitHub sans URL de telechargement: %s", asset.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "21loader-updater")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("telechargement impossible: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("telechargement refuse par GitHub: %s", resp.Status)
	}

	dir := filepath.Join(os.TempDir(), "21loader-update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Base(strings.TrimSpace(asset.Name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "21loader-update-package"
	}
	target := filepath.Join(dir, name)

	outFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(outFile, resp.Body)
	closeErr := outFile.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return target, nil
}
