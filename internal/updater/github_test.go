package updater

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestSelectReleaseAssetPrefersWindowsSetupForArch(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "21loader-2026.06.08-arm64-setup.exe", BrowserDownloadURL: "https://example.invalid/arm.exe"},
		{Name: "21loader-2026.06.08-amd64.zip", BrowserDownloadURL: "https://example.invalid/app.zip"},
		{Name: "21loader-2026.06.08-amd64-setup.exe", BrowserDownloadURL: "https://example.invalid/amd.exe"},
	}

	asset, err := SelectReleaseAsset(assets, "windows", "amd64")
	if err != nil {
		t.Fatalf("SelectReleaseAsset failed: %v", err)
	}
	if asset.Name != "21loader-2026.06.08-amd64-setup.exe" {
		t.Fatalf("unexpected asset: %s", asset.Name)
	}
}

func TestSelectReleaseAssetAcceptsUniversalMacDMG(t *testing.T) {
	assets := []ReleaseAsset{{Name: "21loader-2026.06.08.dmg", BrowserDownloadURL: "https://example.invalid/app.dmg"}}

	asset, err := SelectReleaseAsset(assets, "darwin", "arm64")
	if err != nil {
		t.Fatalf("SelectReleaseAsset failed: %v", err)
	}
	if asset.Name != "21loader-2026.06.08.dmg" {
		t.Fatalf("unexpected asset: %s", asset.Name)
	}
}

func TestSelectReleaseAssetRejectsUnsupportedPlatform(t *testing.T) {
	_, err := SelectReleaseAsset([]ReleaseAsset{{Name: "21loader-2026.06.08.dmg"}}, "freebsd", "amd64")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchLatestReleaseUsesGitHubAuth(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		return response(200, `{"tag_name":"v1","assets":[{"name":"21loader-v1.dmg","url":"https://api.github.test/assets/1"}]}`), nil
	})}

	release, err := fetchLatestRelease(context.Background(), client, DefaultRepo, "test-token")
	if err != nil {
		t.Fatalf("fetchLatestRelease failed: %v", err)
	}
	if release.TagName != "v1" || len(release.Assets) != 1 || release.Assets[0].APIURL == "" {
		t.Fatalf("unexpected release: %#v", release)
	}
}

func TestDownloadAssetUsesAPIURLWhenAuthenticated(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.String(); got != "https://api.github.test/assets/1" {
			t.Fatalf("unexpected download URL: %q", got)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := req.Header.Get("Accept"); got != "application/octet-stream" {
			t.Fatalf("unexpected accept header: %q", got)
		}
		return response(200, "package"), nil
	})}

	path, err := downloadAsset(context.Background(), client, ReleaseAsset{
		APIURL:             "https://api.github.test/assets/1",
		Name:               "21loader-test.dmg",
		BrowserDownloadURL: "https://browser.github.test/download",
	}, io.Discard, "test-token")
	if err != nil {
		t.Fatalf("downloadAsset failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != "package" {
		t.Fatalf("unexpected downloaded content: %q", data)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
