# Vue d'ensemble du projet

## Resume

`21loader-cross` est une application locale en Go qui sert une UI web unique et orchestre des jobs de telechargement/traitement medias.

Sources principales actuellement identifiees:

- YouTube
- RSS / podcasts
- Qobuz

Capacites transverses:

- telechargement de medias
- recuperation de lyrics via LRCLIB
- transcription Whisper
- traduction via Argos Translate
- diagnostics et installation de dependances
- suivi des jobs en temps reel

## Forme generale de l'application

- point d'entree local: `cmd/server/main.go`
- frontend: `web/index.html`
- API HTTP et service de l'UI: `internal/httpapi/`
- orchestration et pipeline des jobs: `internal/jobs/`
- integrations externes: `internal/services/`
- DTO et modeles partages: `internal/core/models.go`
- chemins applicatifs et utilitaires: `internal/util/`, `internal/sys/`

## Pipeline metier principal

Les etapes de job definies dans `internal/core/models.go` sont:

1. `download`
2. `lyrics`
3. `transcription`
4. `muxing`
5. `organization`

Le projet expose aujourd'hui une file de jobs sequentielle avec un seul job actif a la fois.

## Dependances runtime externes

- `yt-dlp`
- `ffmpeg`
- `qobuz-dl`
- `whisper-cli` ou `whisper-cpp`
- Python + `argostranslate`

## Stockage local connu

Selon `internal/util/paths.go`, l'application maintient notamment:

- un dossier `ApplicationSupport`
- un dossier de caches
- un dossier de logs
- des fichiers `settings.json` et `web-settings.json`
- un dossier de bug reports
- un cache de jobs temporaires

## Etat fonctionnel utile a retenir

Fonctionnalites deja relevees comme presentes dans le depot:

- recherche d'artistes Qobuz depuis l'app
- recuperation de lyrics LRCLIB
- compteurs de progression partiels selon les etapes
- ajout du temps total des jobs
- installation automatique des dependances depuis l'application

Backlog ou limitations deja signales:

- progression plus homogene et plus visible
- telechargement uniquement des sous-titres
- ajout du logo dans l'UI
- exploitation des lyrics depuis la description YouTube
- parametrage plus fin de l'arborescence de sortie
- choix explicite `mp3` / `mp4` / les deux pour YouTube

Voir aussi [issues/functional-gap-backlog.md](./issues/functional-gap-backlog.md).

## Pages liees

- [architecture.md](./architecture.md)
- [domains/jobs-pipeline.md](./domains/jobs-pipeline.md)
- [domains/qobuz.md](./domains/qobuz.md)
- [domains/frontend-ui.md](./domains/frontend-ui.md)
- [domains/media-services.md](./domains/media-services.md)
- [sources/source-2026-04-07-initial-project-scan.md](./sources/source-2026-04-07-initial-project-scan.md)
- [issues/functional-gap-backlog.md](./issues/functional-gap-backlog.md)
