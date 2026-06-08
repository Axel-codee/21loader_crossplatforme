# Domaine | qobuz

## Role

Ce domaine couvre l'integration Qobuz dans l'application, a la fois pour l'exploration du catalogue et pour les telechargements reels.

## Pieces principales

- `internal/services/qobuz.go`: recherche artistes, verification credentials, catalogues artistes, playlists et pistes d'album
- `internal/jobs/runner.go`: lancement effectif des downloads Qobuz
- `assets/scripts/qobuz_common.py`: logique Python partagee et patchs runtime `qobuz-dl`
- `assets/scripts/qobuz_cli_wrapper.py`: wrapper CLI Python autour de `qobuz-dl`

## Capacites observees

- recherche d'artistes Qobuz depuis l'application
- lecture de catalogue artiste
- lecture de playlist Qobuz
- lecture des pistes d'un album
- verification de connexion Qobuz
- contournement par token de session Qobuz

## Durcissement de telechargement observe dans le depot

Etat observe dans les fichiers lus lors de cette mise a jour:

- le wrapper Python installe un patch de telechargement et un patch de token avant d'appeler `qobuz_dl.main()`
- `assets/scripts/qobuz_common.py` definit:
  - `QOBUZ_TRACK_DOWNLOAD_ATTEMPTS = 5`
  - `QOBUZ_DOWNLOAD_TIMEOUT = (10, 60)`
  - `QOBUZ_INTER_TRACK_DELAY = 1.0`
  - `QOBUZ_QUALITY_FALLBACKS = (27, 7, 6, 5)`
- `internal/jobs/runner.go` borne les retries de haut niveau a `3` tentatives max
- `internal/jobs/runner.go` essaie d'utiliser le wrapper Python pour les downloads Qobuz quand il est disponible

## Risques / limites

- les erreurs CDN Qobuz peuvent rester partiellement externes au projet
- le fallback de qualite peut produire un album mixte en qualite dans les cas degrades
- le telechargement segmente reste une piste plus lourde si les cas hi-res persistent

## Tests notables reperes

- `TestRunQobuzDownloadCommandRetriesAfterIncompleteRead`
- `TestRunQobuzDownloadCommandStopsAfterMaxRetryableAttempts`
- `TestRunQobuzDownloadCommandRetriesWithoutOGCoverAfterCoverTooLargeError`

## Pages liees

- [../issues/qobuz-incompleteread-download-failures.md](../issues/qobuz-incompleteread-download-failures.md)
- [../solutions/qobuz-hardening-options.md](../solutions/qobuz-hardening-options.md)
- [../sources/source-2026-04-07-session-qobuz-incompleteread.md](../sources/source-2026-04-07-session-qobuz-incompleteread.md)
- [../sources/source-2026-04-07-session-qobuz-hardening-implementation.md](../sources/source-2026-04-07-session-qobuz-hardening-implementation.md)
