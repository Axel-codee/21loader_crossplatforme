# Architecture

## Flux haut niveau

1. `cmd/server/main.go` initialise les chemins applicatifs, les services metier, le `Runner`, le `Coordinator` et le serveur HTTP.
2. `internal/httpapi/router.go` sert `web/index.html` et expose les routes JSON sous `/api/...`.
3. `Coordinator` valide les requetes, gere les reglages, la file de jobs, les etats, les logs et les operations systeme.
4. `Runner` execute le pipeline reel de telechargement et post-traitement.
5. `Organizer` finalise les sorties et les metadonnees.

## Composants principaux

### Frontend

- fichier unique: `web/index.html`
- UI sans framework
- consomme l'API HTTP locale pour les jobs, reglages, diagnostics, Qobuz, RSS, YouTube, Whisper, traduction et lyrics

### HTTP API

`internal/httpapi/router.go` enregistre notamment:

- `/api/settings`
- `/api/diagnostics`
- `/api/dependencies/install`
- `/api/translation/languages`
- `/api/whisper/models`
- `/api/qobuz/...`
- `/api/rss/episodes`
- `/api/youtube/...`
- `/api/lyrics/lrclib/search`
- `/api/jobs`

### Orchestration des jobs

`internal/jobs/coordinator.go` centralise:

- l'etat global du tableau de bord
- la liste des jobs et le job actif
- les reglages web
- les maps d'options et de nommage par job
- l'acces aux services diagnostics, traduction, Whisper, Qobuz, RSS et YouTube

`internal/jobs/runner.go` centralise:

- la creation du workspace temporaire
- le choix du mode de telechargement selon la source et la collision
- les callbacks de logs, progression, comptage et display name
- l'execution du pipeline media

### Services externes

Les services identifies dans `cmd/server/main.go` et `internal/services/` sont:

- `DiagnosticsService`
- `TranslationLanguageService`
- `WhisperModelService`
- `QobuzService`
- `RSSService`
- `YouTubeService`
- `ArtworkThumbnailService`

## Particularites utiles

- Le backend integre deja un wrapper Python Qobuz (`assets/scripts/qobuz_common.py`, `assets/scripts/qobuz_cli_wrapper.py`) en plus de l'appel a `qobuz-dl`.
- Les modeles de job dans `internal/core/models.go` incluent statuts, etapes, types de contenu et decisions de collision.
- Les chemins applicatifs sont resolus de maniere cross-platform via `internal/util/paths.go`.

## Risques et sujets recurrents a suivre

- robustesse du pipeline Qobuz sur gros FLAC hi-res
- homogenisation de la progression exposee a l'UI
- front unique `web/index.html`, donc toute evolution UI importante s'y concentre

## Pages liees

- [project-overview.md](./project-overview.md)
- [domains/jobs-pipeline.md](./domains/jobs-pipeline.md)
- [domains/qobuz.md](./domains/qobuz.md)
- [domains/frontend-ui.md](./domains/frontend-ui.md)
- [issues/qobuz-incompleteread-download-failures.md](./issues/qobuz-incompleteread-download-failures.md)
- [sources/source-2026-04-07-initial-project-scan.md](./sources/source-2026-04-07-initial-project-scan.md)
