# Issue | qobuz incompleteread sur certains telechargements

## Statut

- `mitigated-in-current-worktree`
- impact: fiabilite Qobuz, surtout sur certains gros FLAC hi-res

## Probleme

Des telechargements Qobuz peuvent echouer avec un log du type `Connection broken: IncompleteRead(...)`, puis etre sautes ou relances sans reel changement de strategie.

## Symptomes / constats historiques

- erreur observee pendant le telechargement d'un album Qobuz
- forte suspicion sur les gros fichiers hi-res
- retry cote application juge peu utile quand il relance juste la meme commande
- risque d'echec repete si la source CDN reste defaillante

## Cause confirmee ou hypothese

Connaissance historique issue de la conversation source:

- cause probable: combinaison `qobuz-dl` + CDN Qobuz/Akamai sur certains flux volumineux
- faiblesse identifiee: absence de vraie strategie de reprise/timeout/resilience dans le flux telechargement cible
- faiblesse cote app: retry trop naif si le process est seulement relance a l'identique

## Zones / fichiers concernes

- `internal/jobs/runner.go`
- `assets/scripts/qobuz_cli_wrapper.py`
- `assets/scripts/qobuz_common.py`
- integration `qobuz-dl`

## Solutions liees

- [../solutions/qobuz-hardening-options.md](../solutions/qobuz-hardening-options.md)

## Sources liees

- [../sources/source-2026-04-07-session-qobuz-incompleteread.md](../sources/source-2026-04-07-session-qobuz-incompleteread.md)
- [../sources/source-2026-04-07-session-qobuz-hardening-implementation.md](../sources/source-2026-04-07-session-qobuz-hardening-implementation.md)

## Mitigation observee

Le depot lu au moment de cette mise a jour contient deja une mitigation concrete:

- wrapper Python installe pour les downloads Qobuz quand disponible
- retry piste par piste
- timeout et backoff
- fallback de qualite `27 -> 7 -> 6 -> 5`
- borne de retry de haut niveau cote Go

Le risque n'est donc pas considere comme supprime, mais plutot comme partiellement absorbe par le code actuel.
