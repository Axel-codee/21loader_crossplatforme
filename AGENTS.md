# Repository Guidelines

## Project Structure & Module Organization
`cmd/server/main.go` is the local entrypoint. Core application code lives under `internal/`: `httpapi` serves the UI and JSON API, `jobs` runs the download pipeline, `services` wraps YouTube/RSS/Qobuz/diagnostics features, `core` holds shared models, and `util`/`sys` contain helpers and process abstractions. The frontend is a single static file at `web/index.html`. Runtime scripts and icons live in `assets/`, but `icone.png` at the repository root is the canonical source image for all app icons and derived icon assets. Packaging helpers are in `scripts/windows/` and `scripts/macos/`. Treat `dist/` as generated release output, not hand-edited source.

## Build, Test, and Development Commands
Use `go run ./cmd/server --host 0.0.0.0 --port 8080` for local development, then open `http://localhost:8080`. Use `go build -o server ./cmd/server` to produce a local binary. Run `go test ./...` before every PR; it currently passes across all packages. Packaging commands:

- `scripts/windows/build-exe.sh --version 2026.04.05`
- `scripts/windows/build-installer.sh --version 2026.04.05`
- `scripts/macos/build-dmg.sh --version 2026.04.05`

The Windows scripts write bundles into `dist/windows/`; the macOS script builds a `.app` and `.dmg` in `dist/macos/`.

## Coding Style & Naming Conventions
Follow standard Go style and keep files `gofmt`-clean. Use lower-case package names, exported identifiers in `CamelCase`, and private helpers in `camelCase`. Keep HTTP route logic in `internal/httpapi` and business rules in `internal/jobs` or `internal/services`; avoid pushing orchestration into `main.go`. Frontend changes should preserve the current no-framework approach in `web/index.html`. Shell scripts should stay Bash with `set -euo pipefail`, and environment variables should remain `LOADER21_*`.

## Testing Guidelines
Place tests next to the code they cover with `_test.go` suffixes, following the existing `internal/...` pattern. Prefer focused unit tests with mocks or temporary directories over tests that depend on real external CLIs or network access. Name tests after behavior, for example `TestFetchLyricsFromLRCLIBProcessesAllAlbumTracks`. Run `go test ./...` locally after touching API handlers, job orchestration, or service wrappers.

## Commit & Pull Request Guidelines
Recent history uses short subject lines, often in French. Keep commits brief and descriptive, imperative when possible, and avoid placeholders such as `ton message`. For pull requests, include a concise summary, the commands you ran (`go test ./...`, packaging scripts if relevant), and screenshots when `web/index.html` changes affect the UI.

## Knowledge Wiki
The repo now contains a persistent knowledge base under `wiki/`. When a conversation or investigation produces durable project knowledge, update the relevant pages in `wiki/`, especially `wiki/index.md`, `wiki/log.md`, source pages in `wiki/sources/`, issue pages in `wiki/issues/`, and solution pages in `wiki/solutions/`. Prefer updating existing pages over creating duplicates, and distinguish confirmed facts from hypotheses or historical notes that need revalidation.

## TODO And Improvement Maintenance
`TODO.md` is the short operational backlog of the project and must stay connected to the wiki backlog page at `wiki/issues/functional-gap-backlog.md`.

When the user proposes a new improvement, feature, UX change, bugfix idea, or correction to an existing backlog item, update `TODO.md` in the same turn unless the user explicitly says not to. If the item adds durable context, rationale, problem framing, or links to potential solutions, also update the relevant wiki pages in the same turn. Keep `TODO.md` concise, and use the wiki to store the richer cross-links between backlog items, problems, investigations, and solutions.

## Release Update Reminder
After each completed project modification, propose updating the published GitHub app releases so users can install or update to the latest changes.
