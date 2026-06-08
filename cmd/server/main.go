package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"21loader-cross/internal/httpapi"
	"21loader-cross/internal/jobs"
	"21loader-cross/internal/services"
	"21loader-cross/internal/sys"
	"21loader-cross/internal/updater"
	"21loader-cross/internal/util"
)

var appVersion = "dev"

func main() {
	util.EnsureRuntimeSearchPath()

	args := os.Args[1:]
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "update":
			runUpdate(args[1:])
			return
		case "version":
			fmt.Println(appVersion)
			return
		}
		if args[0] == "--version" || args[0] == "-version" {
			fmt.Println(appVersion)
			return
		}
	}
	runServer(args)
}

func runServer(args []string) {
	flags := flag.NewFlagSet("21loader", flag.ExitOnError)
	host := flags.String("host", "0.0.0.0", "Host to bind")
	port := flags.Int("port", 8080, "Port to bind")
	openBrowser := flags.Bool("open", false, "Open the local UI in the default browser")
	if err := flags.Parse(args); err != nil {
		log.Fatalf("Arguments invalides: %v", err)
	}

	if *port <= 0 || *port > 65535 {
		log.Fatalf("Port invalide: %d", *port)
	}

	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Impossible de determiner le dossier courant: %v", err)
	}
	baseDir, _ = filepath.Abs(baseDir)

	paths, err := util.ResolveAppPaths()
	if err != nil {
		log.Fatalf("Impossible de resoudre les chemins applicatifs: %v", err)
	}

	processRunner := &sys.Runner{}
	organizer := jobs.NewOrganizer()
	runner := jobs.NewRunner(processRunner, organizer, paths, baseDir)
	diagnostics := services.NewDiagnosticsService(processRunner)
	translation := services.NewTranslationLanguageService(processRunner, baseDir)
	whisperModels := services.NewWhisperModelService()
	vadModels := services.NewVADModelService()
	qobuz := services.NewQobuzService(processRunner, baseDir)
	rss := services.NewRSSService()
	youtube := services.NewYouTubeService(processRunner)
	artwork := services.NewArtworkThumbnailService(
		filepath.Join(paths.Caches, "Web", "RSSArtworkThumbnails"),
	)

	coordinator, err := jobs.NewCoordinator(paths, runner, diagnostics, translation, whisperModels, vadModels, qobuz, rss, youtube)
	if err != nil {
		log.Fatalf("Erreur initialisation coordinator: %v", err)
	}

	server, err := httpapi.NewServer(coordinator, artwork, baseDir, appVersion)
	if err != nil {
		log.Fatalf("Erreur initialisation serveur HTTP: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("21loaderWeb cross-platform demarre sur http://%s", addr)
	log.Printf("Depuis un autre appareil: http://<IP_DE_LA_MACHINE>:%d", *port)
	if *openBrowser {
		go openWhenReady(*host, *port)
	}

	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		log.Fatalf("Erreur serveur: %v", err)
	}
}

func runUpdate(args []string) {
	flags := flag.NewFlagSet("21loader update", flag.ExitOnError)
	repo := flags.String("repo", updater.DefaultRepo, "GitHub repository owner/name")
	if err := flags.Parse(args); err != nil {
		log.Fatalf("Arguments update invalides: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := updater.Update(ctx, updater.Options{Repo: *repo, CurrentVersion: appVersion}); err != nil {
		log.Fatalf("Mise a jour impossible: %v", err)
	}
}

func openWhenReady(host string, port int) {
	browserHost := host
	if browserHost == "" || browserHost == "0.0.0.0" || browserHost == "::" {
		browserHost = "127.0.0.1"
	}
	url := fmt.Sprintf("http://%s:%d", browserHost, port)
	for i := 0; i < 100; i++ {
		resp, err := http.Get(url + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				openURL(url)
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
