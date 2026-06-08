# Source | 2026-04-07 | implementation du durcissement qobuz

## Contexte

La conversation archivee `session-ses_29d5.md` ne s'est pas arretee au diagnostic `IncompleteRead`: elle rapporte ensuite l'implementation des deux premieres solutions retenues.

## Sources brutes consultees

- `session-ses_29d5.md`
- `assets/scripts/qobuz_cli_wrapper.py`
- `assets/scripts/qobuz_common.py`
- `internal/jobs/runner.go`
- `internal/jobs/runner_test.go`

## Informations durables extraites

- les downloads Qobuz passent par le wrapper Python quand il est disponible
- le wrapper installe un patch de token et un patch de download avant l'execution de `qobuz-dl`
- la resilience par piste observee dans `qobuz_common.py` inclut:
  - `5` tentatives max par piste
  - URL de telechargement regenee sur retry
  - timeout reseau explicite `10s/60s`
  - suppression du temporaire avant reprise
  - pause inter-pistes `1s`
- un fallback de qualite est defini en `27 -> 7 -> 6 -> 5`
- le retry Go de haut niveau est borne a `3` tentatives max
- des tests existent pour couvrir retry, epuisement des tentatives et fallback `--og-cover`

## Remarque de validite

Ces informations ne sont plus seulement historiques: elles sont coherentes avec l'etat observe dans les fichiers du depot au moment de cette mise a jour. Elles restent toutefois a revalider si le code evolue fortement par la suite.

## Pages impactees

- [../domains/qobuz.md](../domains/qobuz.md)
- [../issues/qobuz-incompleteread-download-failures.md](../issues/qobuz-incompleteread-download-failures.md)
- [../solutions/qobuz-hardening-options.md](../solutions/qobuz-hardening-options.md)
