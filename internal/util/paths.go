package util

import (
	"os"
	"path/filepath"
)

const AppName = "21loader"

type AppPaths struct {
	ApplicationSupport  string
	Caches              string
	LogsDirectory       string
	SettingsFile        string
	WebSettingsFile     string
	BugReportsDirectory string
	JobsCacheDirectory  string
}

func ResolveAppPaths() (AppPaths, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return AppPaths{}, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return AppPaths{}, err
	}
	appSupport := filepath.Join(cfg, AppName)
	appCache := filepath.Join(cache, AppName)
	p := AppPaths{
		ApplicationSupport:  appSupport,
		Caches:              appCache,
		LogsDirectory:       filepath.Join(appSupport, "Logs"),
		SettingsFile:        filepath.Join(appSupport, "settings.json"),
		WebSettingsFile:     filepath.Join(appSupport, "web-settings.json"),
		BugReportsDirectory: filepath.Join(appSupport, "BugReports"),
		JobsCacheDirectory:  filepath.Join(appCache, "Jobs"),
	}
	return p, nil
}

func (p AppPaths) Ensure() error {
	for _, dir := range []string{p.ApplicationSupport, p.Caches, p.LogsDirectory, p.BugReportsDirectory, p.JobsCacheDirectory} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p AppPaths) WorkspaceDirectory(jobID string) string {
	return filepath.Join(p.JobsCacheDirectory, jobID)
}

func (p AppPaths) LogFile(jobID string) string {
	return filepath.Join(p.LogsDirectory, jobID+".log")
}
