# Log du wiki

## [2026-06-08] fix | detection Homebrew depuis app macOS

- Ajout d'une initialisation du `PATH` applicatif au demarrage pour couvrir `/opt/homebrew/bin`, `/usr/local/bin`, `~/.local/bin` et les dossiers Python utilisateur.
- Correction du cas ou `yt-dlp` installe via Homebrew est visible dans le terminal mais pas dans 21loader lance depuis Finder/Applications.
- Synchronisation de `TODO.md` et du backlog wiki avec `BACKLOG-16`.

## [2026-06-08] fix | updater repo prive

- Ajout d'une authentification optionnelle pour `21loader update` via `LOADER21_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN` ou `gh auth token`.
- Le telechargement authentifie utilise l'URL API de l'asset GitHub avec `Accept: application/octet-stream`.
- Synchronisation de `TODO.md` et du backlog wiki avec `BACKLOG-17`.

## [2026-06-08] feat | commande terminal et updater GitHub Releases

- Ajout d'une CLI `21loader` avec sous-commande `21loader update`.
- Ajout d'un updater qui selectionne l'asset compatible dans la derniere GitHub Release.
- Mise a jour des scripts de packaging Windows/macOS et ajout d'un workflow GitHub Actions de publication.
- Synchronisation de `TODO.md` et du backlog wiki avec `BACKLOG-15`.

## [2026-04-07] bootstrap | creation du wiki projet

- Creation de la structure `wiki/` avec index, schema, log et pages de synthese.
- Initialisation du wiki pour un usage centre sur les conversations, les scans du depot et les diagnostics techniques.

## [2026-04-07] ingest | scan initial du depot

- Ingestion des informations stables depuis `README.md`, `TODO.md`, `AGENTS.md` et un premier scan de fichiers clefs du backend.
- Creation des pages [project-overview.md](./project-overview.md) et [architecture.md](./architecture.md).
- Creation du backlog [issues/functional-gap-backlog.md](./issues/functional-gap-backlog.md).

## [2026-04-07] ingest | session qobuz incompleteread

- Ingestion de `session-ses_29d5.md` comme source de connaissance sur un incident Qobuz recurrent.
- Creation de l'issue [issues/qobuz-incompleteread-download-failures.md](./issues/qobuz-incompleteread-download-failures.md).
- Creation de la page solution [solutions/qobuz-hardening-options.md](./solutions/qobuz-hardening-options.md).

## [2026-04-07] enrich | domaines du wiki

- Creation du dossier `wiki/domains/` avec des pages dediees pour le pipeline, Qobuz, le frontend et les services media.
- Ajout d'une navigation par domaine dans [index.md](./index.md).

## [2026-04-07] ingest | autres sujets issus de la session archivee

- Ajout d'une source sur l'implementation du durcissement Qobuz observee dans le depot.
- Ajout d'une source sur le briefing de future refonte UI et les contraintes de collaboration design.

## [2026-04-07] maintain | synchronisation todo et wiki

- Correction du libelle TODO sur le comptage des morceaux Qobuz pour refleter le besoin reel: compter une piste terminee, pas seulement commencee.
- Ajout de la refonte graphique dans `TODO.md`.
- Renforcement du lien entre `TODO.md` et `wiki/issues/functional-gap-backlog.md`.
- Ajout d'identifiants `BACKLOG-xx` pour relier plus facilement backlog, problemes et solutions.

## [2026-04-09] maintain | branding logo web

- Synchronisation de `TODO.md` et du backlog wiki pour sortir `BACKLOG-05` du flux des sujets ouverts.
- Ajout d'une source de connaissance sur l'integration du logo web et du favicon navigateur.
- Mise a jour du domaine [frontend-ui.md](./domains/frontend-ui.md) avec les points d'ancrage techniques du branding web.

## [2026-04-17] feat | favoris RSS dans le formulaire de job

- Mise a jour de `internal/core/models.go` et `internal/jobs/coordinator.go` pour persister une liste de podcasts RSS favoris dans les reglages web existants.
- Mise a jour de `web/index.html` pour ajouter un bouton d'ajout/retrait de favori a droite de l'URL RSS et une modale `Podcasts preferes` qui remplit le flux puis ouvre la selection d'episodes.
- Ajout de tests backend sur la normalisation des favoris RSS et synchronisation de `TODO.md` ainsi que du backlog wiki.

## [2026-04-17] feat | options whisper avancees et modeles VAD

- Extension de `internal/core/models.go`, `internal/jobs/coordinator.go`, `internal/jobs/runner.go` et `internal/jobs/organizer.go` pour supporter VAD, segmentation SRT, prompt initial, JSON complet Whisper et tinydiarize avec artefacts supplementaires.
- Ajout d'un gestionnaire de modeles VAD dedie (`internal/services/vad_models.go`, routes `/api/whisper/vad-models*`) sur le meme principe que le gestionnaire de modeles Whisper.
- Mise a jour de `web/index.html` pour exposer tous les reglages Whisper cote job et cote `Reglages moteur`, ajouter le manager VAD et permettre de memoriser un prompt Whisper par podcast RSS favori.
- Ajout de tests sur l'assemblage des arguments Whisper, la priorite `job > podcast > global`, la persistance des nouveaux reglages et l'organisation des nouveaux artefacts.

## [2026-04-10] maintain | split UI creation suivi

- Mise a jour de `TODO.md` pour refleter le split de l'interface telechargement entre `Ajouter un job` et `Jobs en cours`.
- Mise a jour du domaine [frontend-ui.md](./domains/frontend-ui.md) avec la nouvelle navigation principale et la contrainte UX durable sur la selection RSS explicite.

## [2026-04-10] fix | completion RSS par episode

- Mise a jour de `internal/jobs/runner.go` pour empecher qu'un job RSS en mode `completer` reutilise la sortie d'un autre episode du meme podcast sans correspondance d'URL media.
- Ajout d'un test de non-regression dans `internal/jobs/runner_test.go`.
- Synchronisation de `TODO.md` et du domaine [media-services.md](./domains/media-services.md) avec cette contrainte de matching RSS.

## [2026-04-10] enrich | statut UI reutilise

- Le pipeline et les DTO de jobs distinguent maintenant une etape `reutilise` d'une etape `ok` quand le mode `completer` saute une etape deja presente.
- Mise a jour de `web/index.html` pour afficher `reutilise` dans la synthese des jobs a la place d'un `ok` ambigu.

## [2026-04-12] fix | progression transcription et doublons de jobs

- Mise a jour de `web/index.html` pour afficher la progression reelle de l'etape `transcription` a partir de `currentStepProgress`, plutot qu'un pourcentage global du pipeline fige autour de `43%`.
- Mise a jour de `internal/jobs/coordinator.go` pour refuser l'ajout d'un job logiquement identique quand la collision est en mode `completer`.
- Synchronisation de `TODO.md`, du backlog wiki et du domaine [jobs-pipeline.md](./domains/jobs-pipeline.md).

## [2026-05-02] feat | diarisation pyannote additive

- Extension de `internal/core/models.go`, `internal/jobs/coordinator.go`, `internal/jobs/runner.go`, `internal/jobs/organizer.go` et `internal/services/diagnostics.go` pour introduire un provider de diarisation generique (`none`, `tinydiarize`, `pyannote`) avec compatibilite implicite pour les anciens reglages `tinydiarize`.
- Ajout d'un runtime Python dedie `pyannote` (`internal/util/pyannote.go`), d'un wrapper local `assets/scripts/pyannote_diarize.py`, d'un merge segment-par-segment Whisper/pyannote et de nouveaux artefacts `.pyannote.json/.txt/.srt` separes des artefacts `tinydiarize`.
- Mise a jour de `web/index.html` pour passer a un bloc `Diarisation` generique, ajouter la carte de preparation `Pyannote` dans `Reglages`, griser l'option tant que le runtime/acces modele n'est pas pret, puis synchroniser `TODO.md` et les domaines wiki relies.
