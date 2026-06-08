# Solution | detection Homebrew depuis l'app macOS

## Statut

- `implemented 2026-06-08`
- lie a `BACKLOG-16`

## Probleme

- Quand 21loader est lance depuis Finder ou `/Applications`, macOS ne fournit pas forcement le meme `PATH` qu'un shell interactif.
- Les binaires installes via Homebrew, notamment `yt-dlp` et `ffmpeg`, peuvent donc etre presents sur la machine mais invisibles pour les diagnostics et les jobs.

## Implementation

- `cmd/server/main.go` appelle `util.EnsureRuntimeSearchPath()` au demarrage, avant l'initialisation des services.
- `internal/util/pathenv.go` preserve le `PATH` existant puis ajoute les emplacements usuels:
  - repertoire binaire gere par 21loader
  - `LOADER21_EXTRA_PATH` si fourni
  - `~/.local/bin`
  - `~/Library/Python/<version>/bin`
  - `/opt/homebrew/bin`, `/opt/homebrew/sbin`
  - `/usr/local/bin`, `/usr/local/sbin`
  - chemins systeme standards

## Effet attendu

- Les diagnostics detectent les dependances deja installees.
- Les executions directes de `yt-dlp`, Qobuz, Whisper et autres CLI profitent du meme `PATH` enrichi.
- Les jobs YouTube resolvent aussi `yt-dlp` en chemin absolu via `util.ResolveToolExecutable`, avec fallback explicite vers les dossiers Homebrew macOS.

## Notes

- `LOADER21_EXTRA_PATH` permet d'ajouter des chemins particuliers sans modifier l'application.
