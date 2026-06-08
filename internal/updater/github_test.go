package updater

import "testing"

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
