# 21loader_crossplatforme

Portage cross-platform du mode web PersoDL vers Go.

## Ce que contient ce dossier

- Backend API en Go (routes compatibles avec l'UI web existante)
- UI web extraite depuis la version Swift (`web/index.html`)
- Gestion des jobs (queue sequentielle, pause/reprise/annulation)
- Integrations: `yt-dlp`, `ffmpeg`, `qobuz-dl`, `whisper-cli`, `argostranslate` (optionnel)
- Diagnostics de dependances + endpoint d'installation automatique

## Lancement

```bash
cd 21loader_crossplatforme
go run ./cmd/server --host 0.0.0.0 --port 8080
```

Puis ouvrir `http://localhost:8080`.

## Build

```bash
cd 21loader_crossplatforme
go build ./cmd/server
```

## Endpoints principaux

- `GET /`
- `GET /healthz`
- `GET /api/status`
- `GET /api/settings`
- `PUT /api/settings`
- `GET /api/diagnostics`
- `POST /api/dependencies/install`
- `POST /api/jobs`
- `GET /api/jobs`
- `GET /api/jobs/{id}`
- `GET /api/jobs/{id}/logs`
- `POST /api/jobs/{id}/pause`
- `POST /api/jobs/{id}/resume`
- `POST /api/jobs/{id}/cancel`
- `POST /api/youtube/catalog`
- `POST /api/youtube/dates`
- `POST /api/rss/episodes`
- `POST /api/qobuz/search-artists`
- `POST /api/qobuz/artist-catalog`
- `POST /api/qobuz/album-tracks`
- `GET /api/artwork?url=...&size=...`

## Notes

- Stockage settings/logs dans le dossier utilisateur via `os.UserConfigDir` / `os.UserCacheDir`.
- `qobuz-dl` utilise des scripts Python locaux sous `assets/scripts`.
- La traduction des fichiers Whisper (`.srt`/`.txt`) via Argos requiert Python + module `argostranslate`.
  - L'installateur de dépendances crée un venv PersoDL dédié pour éviter les blocages PEP 668.
- Le backend expose CORS `*` (reseau de confiance recommande).
