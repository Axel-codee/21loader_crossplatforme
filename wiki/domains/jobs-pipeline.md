# Domaine | jobs et pipeline

## Role

Ce domaine couvre la creation, l'orchestration, l'execution et le suivi des jobs medias.

## Composants principaux

- `internal/jobs/coordinator.go`: file de jobs, etat global, logs, reglages, interactions avec les services
- `internal/jobs/runner.go`: execution du pipeline reel et pilotage des outils externes
- `internal/jobs/organizer.go`: organisation finale des sorties et metadonnees
- `internal/core/models.go`: etapes, statuts, types de contenu et DTO de suivi

## Etapes connues du pipeline

Selon `internal/core/models.go` et `internal/jobs/runner.go`:

1. `download`
2. `lyrics`
3. `transcription`
4. `muxing`
5. `organization`

## Comportements utiles a retenir

- un seul job actif a la fois dans l'etat actuel du projet
- le `Runner` expose des callbacks pour les logs, la progression, les compteurs et le display name
- le pipeline remonte maintenant explicitement les etapes `reutilisees` en mode `completer`, au-dela des seuls logs texte
- l'UI des jobs affiche maintenant le vrai `currentStepProgress` pendant `transcription`, ce qui evite un pourcentage global fige autour de `43%`
- un doublon logique exact est refuse a l'enqueue quand la collision est en mode `completer`; pour relancer volontairement la meme source, il faut choisir `rename`/`overwrite` ou changer le nom cible

## Tests notables reperes

- `internal/jobs/runner_test.go`
- `internal/jobs/coordinator_lyrics_test.go`
- `internal/jobs/coordinator_build_test.go`
- `internal/jobs/coordinator_display_name_test.go`

## Risques / sujets ouverts

- restitution de progression plus coherente dans l'UI
- meilleure visibilite sur l'avancement piste par piste

## Pages liees

- [../architecture.md](../architecture.md)
- [../issues/functional-gap-backlog.md](../issues/functional-gap-backlog.md)
- [../domains/qobuz.md](./qobuz.md)
- [../domains/media-services.md](./media-services.md)
