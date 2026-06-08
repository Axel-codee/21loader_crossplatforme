# Source | 2026-04-07 | session qobuz incompleteread

## Contexte

Cette page capture une conversation archivee dans `session-ses_29d5.md` a propos d'echecs Qobuz intermittents sur certains albums et a certains moments, avec log `IncompleteRead`.

## Source brute

- `session-ses_29d5.md`

## Informations durables extraites

- Le symptome cible est un echec `IncompleteRead` pendant certains telechargements Qobuz, surtout sur de gros fichiers hi-res.
- Le diagnostic historique pointe un retry actuel trop naif cote `21loader`: relance du process sans changement de strategie.
- La session releve que `qobuz-dl` telecharge en flux simple `requests.get(..., stream=True)` sans vraie reprise partielle.
- Le probleme etait considere comme connu cote upstream `qobuz-dl`, avec une issue ouverte et une PR de durcissement du downloader.
- Les pistes proposees dans la session sont:
  - retry par piste avec URL fraiche, timeout, backoff et pause entre pistes
  - fallback de qualite apres plusieurs echecs
  - telechargement segmente en solution plus lourde
  - borne de retries et message d'erreur clair cote app meme en correctif minimal

## Remarque de validite

Cette page capture une connaissance historique issue d'une conversation passee. Le code actuel doit etre revalide avant de supposer que l'issue est encore ouverte ou qu'aucune des solutions n'a ete integree depuis.

## Pages impactees

- [../issues/qobuz-incompleteread-download-failures.md](../issues/qobuz-incompleteread-download-failures.md)
- [../solutions/qobuz-hardening-options.md](../solutions/qobuz-hardening-options.md)
