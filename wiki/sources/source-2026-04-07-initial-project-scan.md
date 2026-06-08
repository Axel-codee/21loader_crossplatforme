# Source | 2026-04-07 | scan initial du depot

## Contexte

Premier scan du depot pour initialiser le wiki projet.

## Sources brutes consultees

- `README.md`
- `TODO.md`
- `AGENTS.md`
- `cmd/server/main.go`
- `internal/httpapi/router.go`
- `internal/jobs/coordinator.go`
- `internal/jobs/runner.go`
- `internal/services/qobuz.go`
- `internal/core/models.go`
- `internal/util/paths.go`

## Informations durables extraites

- Le projet est une application locale Go avec UI web statique et API HTTP locale.
- Le pipeline metier est structure autour de `download`, `lyrics`, `transcription`, `muxing`, `organization`.
- Le `Coordinator` tient la file de jobs, les reglages, les logs et les integrations metier.
- Le `Runner` execute le telechargement et le post-traitement reel.
- Les integrations externes majeures sont YouTube, RSS, Qobuz, Whisper, Argos et les diagnostics outillage.
- Le stockage applicatif cross-platform est gere via `internal/util/paths.go`.
- Le backlog ouvert le plus visible provient actuellement de `TODO.md`.

## Incertitudes / limites

- Ce scan donne une vue structurelle, pas une validation exhaustive de chaque fonctionnalite.
- Le statut exact de certains sujets de `TODO.md` devra etre revalide si le code evolue.

## Pages impactees

- [../project-overview.md](../project-overview.md)
- [../architecture.md](../architecture.md)
- [../issues/functional-gap-backlog.md](../issues/functional-gap-backlog.md)
