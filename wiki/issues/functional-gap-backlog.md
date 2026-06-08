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
| `BACKLOG-09` | Choisir `mp3` / `mp4` ou les deux pour les musiques YouTube | `open` | services media | [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-10` | Obtenir les lyrics sur les musiques YouTube | `open` | services media | [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-11` | Empecher l'ajout d'un job identique deja present en mode `completer` | `done 2026-04-12` | jobs/pipeline | [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md) |
| `BACKLOG-12` | Ajouter des podcasts RSS favoris selectionnables depuis `Ajouter un job` | `done 2026-04-17` | frontend/ui | [../domains/frontend-ui.md](../domains/frontend-ui.md), [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-13` | Pouvoir regler le nombre de threads utilises par Whisper | `open` | services media | [../domains/media-services.md](../domains/media-services.md) |
| `BACKLOG-14` | Ajouter pyannote comme moteur de diarisation optionnel | `done 2026-05-02` | services media / jobs-pipeline / frontend/ui | [../domains/media-services.md](../domains/media-services.md), [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md), [../domains/frontend-ui.md](../domains/frontend-ui.md) |
| `BACKLOG-15` | Ajouter la commande terminal `21loader` et `21loader update` | `done 2026-06-08` | packaging / distribution | [../solutions/terminal-cli-github-release-updater.md](../solutions/terminal-cli-github-release-updater.md) |

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

## Notes

- Cette page represente un registre de sujets ouverts plutot qu'un bug unique.
- Le statut exact de chaque ligne devra etre reevalue au fil des conversations et des changements de code.
- `BACKLOG-05` a ete traite le `2026-04-09`; la ligne reste ici comme point de traceabilite pour le branding web.
- `BACKLOG-04` a ete traite le `2026-04-12`; l'UI affiche maintenant le pourcentage reel de l'etape `transcription` au lieu du pourcentage global du pipeline.
- `BACKLOG-11` a ete traite le `2026-04-12`; un doublon logique exact est maintenant refuse a l'enqueue quand la collision est en mode `completer`.
- `BACKLOG-12` a ete traite le `2026-04-17`; les flux RSS favoris sont maintenant persistables dans les reglages web et reutilisables depuis la vue `Ajouter un job`.
- `BACKLOG-14` a ete traite le `2026-05-02`; l'app expose maintenant un provider de diarisation generique avec `tinydiarize` et `pyannote`, un runtime Python dedie pour `pyannote`, un merge speaker-dominant par segment Whisper et des artefacts `.pyannote.json/.txt/.srt` separes.
- `BACKLOG-15` a ete traite le `2026-06-08`; l'app expose `21loader` et `21loader update`, avec un updater qui consomme les GitHub Releases produites par GitHub Actions.
- Les identifiants `BACKLOG-xx` servent de points d'ancrage stables pour relier plus tard une solution, une issue detaillee ou une investigation a un item de backlog precis.

## Pages liees

- [../project-overview.md](../project-overview.md)
- [../domains/jobs-pipeline.md](../domains/jobs-pipeline.md)
- [../domains/frontend-ui.md](../domains/frontend-ui.md)
- [../sources/source-2026-04-07-initial-project-scan.md](../sources/source-2026-04-07-initial-project-scan.md)
