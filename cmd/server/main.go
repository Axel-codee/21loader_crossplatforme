package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"21loader-cross/internal/httpapi"
	"21loader-cross/internal/jobs"
	"21loader-cross/internal/services"
	"21loader-cross/internal/sys"
	"21loader-cross/internal/util"
)

func main() {
	host := flag.String("host", "0.0.0.0", "Host to bind")
	port := flag.Int("port", 8080, "Port to bind")
	flag.Parse()

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
	qobuz := services.NewQobuzService(processRunner, baseDir)
	rss := services.NewRSSService()
	youtube := services.NewYouTubeService(processRunner)
	artwork := services.NewArtworkThumbnailService(
		filepath.Join(paths.Caches, "Web", "RSSArtworkThumbnails"),
	)

	coordinator, err := jobs.NewCoordinator(paths, runner, diagnostics, translation, whisperModels, qobuz, rss, youtube)
	if err != nil {
		log.Fatalf("Erreur initialisation coordinator: %v", err)
	}

	server, err := httpapi.NewServer(coordinator, artwork, baseDir)
	if err != nil {
		log.Fatalf("Erreur initialisation serveur HTTP: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("21loaderWeb cross-platform demarre sur http://%s", addr)
	log.Printf("Depuis un autre appareil: http://<IP_DE_LA_MACHINE>:%d", *port)

	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		log.Fatalf("Erreur serveur: %v", err)
	}
}
