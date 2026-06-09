# Issue | backlog fonctionnel ouvert

## Statut

- `open`
- type: backlog produit / ergonomie / pipeline

## Source principale

- `TODO.md`

## Synchronisation

- Cette page est la vue wiki du backlog operationnel maintenu dans `TODO.md`.
- `TODO.md` reste la liste courte et editable rapidement.
- Cette page sert a rattacher les sujets a des domaines, problemes plus fins et solutions quand ils apparaissent.

## Entrees backlog

| ID | Sujet | Statut | Domaine principal | Liens actuels |
| --- | --- | --- | --- | --- |
| `BACKLOG-01` | Ne compter un morceau Qobuz comme fait que quand il est termine, pas des qu'il commence | `open` | jobs/pipeline | [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md), [../domains/qobuz.md](../domains/qobuz.md) |
| `BACKLOG-02` | Telecharger uniquement les sous-titres | `open` | services media | [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-03` | Reelle barre de progression telechargement / transcription / etc. | `open` | jobs/pipeline | [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md), [../domains/frontend-ui.md](../domains/frontend-ui.md) |
| `BACKLOG-04` | Avancement de la transcription | `done 2026-04-12` | jobs/pipeline | [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md), [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-05` | Ajout du logo dans l'en-tete web et le favicon navigateur | `done 2026-04-09` | frontend/ui | [../domains/frontend-ui.md](../domains/frontend-ui.md), [../sources/source-2026-04-09-session-logo-web.md](../sources/source-2026-04-09-session-logo-web.md) |
| `BACKLOG-06` | Refonte graphique de l'interface 21loader | `open` | frontend/ui | [../domains/frontend-ui.md](../domains/frontend-ui.md), [../sources/source-2026-04-07-session-ui-briefing.md](../sources/source-2026-04-07-session-ui-briefing.md) |
| `BACKLOG-07` | Recuperation des paroles dans la description des videos YouTube | `open` | services media | [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-08` | Gerer directement depuis l'app la tete des ramifications dossiers crees | `open` | jobs/pipeline | [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md) |
| `BACKLOG-09` | Choisir une sortie video `mp4` ou une double sortie audio/video pour YouTube | `open` | services media | [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-10` | Obtenir les lyrics sur les musiques YouTube | `open` | services media | [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-11` | Empecher l'ajout d'un job identique deja present en mode `completer` | `done 2026-04-12` | jobs/pipeline | [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md) |
| `BACKLOG-12` | Ajouter des podcasts RSS favoris selectionnables depuis `Ajouter un job` | `done 2026-04-17` | frontend/ui | [../domains/frontend-ui.md](../domains/frontend-ui.md), [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-13` | Pouvoir regler le nombre de threads utilises par Whisper | `open` | services media | [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-14` | Ajouter pyannote comme moteur de diarisation optionnel | `done 2026-05-02` | services media / jobs-pipeline / frontend/ui | [../domains/media-services.md](../domains/media-services.md), [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md), [../domains/frontend-ui.md](../domains/frontend-ui.md) |
| `BACKLOG-15` | Ajouter la commande terminal `21loader` et `21loader update` | `done 2026-06-08` | packaging / distribution | [../solutions/terminal-cli-github-release-updater.md](../solutions/terminal-cli-github-release-updater.md) |
| `BACKLOG-16` | Detecter les dependances Homebrew deja installees depuis l'app macOS | `done 2026-06-08` | packaging / diagnostics / services media | [../solutions/macos-runtime-path-homebrew.md](../solutions/macos-runtime-path-homebrew.md), [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-17` | Authentifier l'updater pour un repo GitHub prive | `done 2026-06-08` | packaging / distribution | [../solutions/terminal-cli-github-release-updater.md](../solutions/terminal-cli-github-release-updater.md) |
| `BACKLOG-18` | Corriger le launcher macOS appele via symlink terminal | `done 2026-06-08` | packaging / distribution | [../solutions/terminal-cli-github-release-updater.md](../solutions/terminal-cli-github-release-updater.md) |
| `BACKLOG-19` | Executer `yt-dlp` via resolution absolue pour les jobs YouTube | `done 2026-06-08` | services media / jobs-pipeline / diagnostics | [../solutions/macos-runtime-path-homebrew.md](../solutions/macos-runtime-path-homebrew.md), [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-20` | Faire de `icone.png` la source canonique des icones | `done 2026-06-08` | frontend/ui / packaging | [../sources/source-2026-04-09-session-logo-web.md](../sources/source-2026-04-09-session-logo-web.md), [../domains/frontend-ui.md](../domains/frontend-ui.md) |
| `BACKLOG-21` | Rappeler de proposer une mise a jour GitHub apres modification projet | `done 2026-06-08` | collaboration / release | [../solutions/terminal-cli-github-release-updater.md](../solutions/terminal-cli-github-release-updater.md) |
| `BACKLOG-22` | Ajouter un reglage de format audio YouTube par defaut | `done 2026-06-08` | services media / frontend-ui | [../domains/media-services.md](../domains/media-services.md), [../domains/frontend-ui.md](../domains/frontend-ui.md) |
| `BACKLOG-23` | Arreter l'ancien serveur inactif apres update/lancement | `done 2026-06-08` | packaging / distribution | [../solutions/terminal-cli-github-release-updater.md](../solutions/terminal-cli-github-release-updater.md) |
| `BACKLOG-24` | Afficher la release courante dans la Web UI | `done 2026-06-08` | frontend-ui / distribution | [../domains/frontend-ui.md](../domains/frontend-ui.md), [../solutions/terminal-cli-github-release-updater.md](../solutions/terminal-cli-github-release-updater.md) |
| `BACKLOG-25` | Integrer metadonnees et miniature dans les fichiers telecharges par yt-dlp | `done 2026-06-08` | services media / frontend-ui | [../domains/media-services.md](../domains/media-services.md), [../domains/frontend-ui.md](../domains/frontend-ui.md) |
| `BACKLOG-26` | Classer les preferences audio YouTube entre formats natifs et conversions fallback | `done 2026-06-08` | services media / frontend-ui | [../domains/media-services.md](../domains/media-services.md), [../domains/frontend-ui.md](../domains/frontend-ui.md) |

## Zones de code probablement concernees

- `internal/jobs/coordinator.go`
- `internal/jobs/runner.go`
- `internal/jobs/organizer.go`
- `internal/core/models.go`
- `internal/services/youtube.go`
- `internal/httpapi/router.go`
- `web/index.html`
- `cmd/server/main.go`
- `internal/updater/`
- `scripts/windows/`
- `scripts/macos/`
- `.github/workflows/release.yml`
- `internal/util/pathenv.go`
- `internal/util/binresolve.go`
- `AGENTS.md`

## Notes

- Cette page represente un registre de sujets ouverts plutot qu'un bug unique.
- Le statut exact de chaque ligne devra etre reevalue au fil des conversations et des changements de code.
- `BACKLOG-05` a ete traite le `2026-04-09`; la ligne reste ici comme point de traceabilite pour le branding web.
- `BACKLOG-04` a ete traite le `2026-04-12`; l'UI affiche maintenant le pourcentage reel de l'etape `transcription` au lieu du pourcentage global du pipeline.
- `BACKLOG-11` a ete traite le `2026-04-12`; un doublon logique exact est maintenant refuse a l'enqueue quand la collision est en mode `completer`.
- `BACKLOG-12` a ete traite le `2026-04-17`; les flux RSS favoris sont maintenant persistables dans les reglages web et reutilisables depuis la vue `Ajouter un job`.
- `BACKLOG-14` a ete traite le `2026-05-02`; l'app expose maintenant un provider de diarisation generique avec `tinydiarize` et `pyannote`, un runtime Python dedie pour `pyannote`, un merge speaker-dominant par segment Whisper et des artefacts `.pyannote.json/.txt/.srt` separes.
- `BACKLOG-15` a ete traite le `2026-06-08`; l'app expose `21loader` et `21loader update`, avec un updater qui consomme les GitHub Releases produites par GitHub Actions.
- `BACKLOG-16` a ete traite le `2026-06-08`; l'app enrichit son `PATH` au demarrage pour trouver Homebrew et les binaires utilisateur meme lorsqu'elle est lancee depuis Finder/Applications.
- `BACKLOG-17` a ete traite le `2026-06-08`; l'updater sait utiliser un token GitHub explicite ou `gh auth token` pour acceder aux releases d'un repo prive.
- `BACKLOG-18` a ete traite le `2026-06-08`; le launcher macOS resout maintenant le vrai chemin du bundle avant de chercher `Contents/Resources/app`, meme lorsqu'il est appele depuis `/usr/local/bin/21loader`.
- `BACKLOG-19` a ete traite le `2026-06-08`; les jobs YouTube et les helpers de catalogue/titre/dates utilisent maintenant un chemin resolu pour `yt-dlp` au lieu de dependre d'un nom brut dans le `PATH`.
- `BACKLOG-20` a ete traite le `2026-06-08`; `icone.png` est desormais documente comme source canonique et les assets derives ont ete regeneres depuis cette image.
- `BACKLOG-21` a ete traite le `2026-06-08`; `AGENTS.md` contient maintenant le rappel de proposer une mise a jour des releases GitHub apres modification projet terminee.
- `BACKLOG-22` a ete traite le `2026-06-08`; les reglages exposent un format audio YouTube par defaut (`mp3`, `m4a`, `opus`, `flac`, `wav`, `aac`) et les jobs audio/music demandent a `yt-dlp` d'extraire/convertir l'audio au lieu de conserver un `.webm`.
- `BACKLOG-23` a ete traite le `2026-06-08`; le healthcheck expose la version du serveur et le launcher macOS arrete un ancien serveur inactif avant de demarrer la nouvelle app, tandis que les scripts d'update stoppent le serveur avant copie.
- `BACKLOG-24` a ete traite le `2026-06-08`; `/api/status` expose maintenant la version applicative et l'en-tete de la Web UI l'affiche sous forme de badge `Release ...`.
- `BACKLOG-25` a ete traite le `2026-06-08`; les reglages yt-dlp exposent l'integration des metadonnees et de la miniature, activees par defaut, avec un avertissement pour les formats audio dont le support miniature est limite.
- `BACKLOG-26` a ete traite le `2026-06-08`; les reglages YouTube Audio acceptent maintenant un classement mixte de priorites natives (`native:m4a`, `native:webm`, `native:best`) et de conversions fallback (`convert:<format>`), sans bloquer plusieurs conversions successives.
- Les identifiants `BACKLOG-xx` servent de points d'ancrage stables pour relier plus tard une solution, une issue detaillee ou une investigation a un item de backlog precis.

## Pages liees

- [../project-overview.md](../project-overview.md)
- [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md)
- [../domains/frontend-ui.md](../domains/frontend-ui.md)
- [../sources/source-2026-04-07-initial-project-scan.md](../sources/source-2026-04-07-initial-project-scan.md)
