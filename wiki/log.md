# Log du wiki

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
