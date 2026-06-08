# Solution | durcissement qobuz face a incompleteread

## Statut

- `implemented-observed-in-worktree`

## Probleme cible

- [../issues/qobuz-incompleteread-download-failures.md](../issues/qobuz-incompleteread-download-failures.md)

## Option 1

Faire passer les downloads Qobuz par un wrapper capable de durcir le telechargement piste par piste:

- nombre max d'essais
- backoff exponentiel
- suppression du temporaire avant retry
- URL fraiche a chaque retry
- timeout reseau explicite
- courte pause entre pistes

Pourquoi cette option est interessante:

- changement plus pragmatique qu'un fork profond
- meilleur alignement avec la panne observee qu'un simple redemarrage de process
- compatible avec l'existence deja constatee d'un wrapper Python dans le repo

## Option 2

Ajouter un fallback de qualite apres plusieurs echecs sur une piste.

Sequence citee dans la conversation source:

- `27`
- `7`
- `6`
- `5` eventuellement en dernier recours

Avantage principal:

- augmenter les chances d'obtenir un album complet meme si le plus haut niveau de qualite echoue.

## Option 3

Conserver comme plan plus lourd un telechargement segmente avec dechiffrement/remux.

Cette option a ete identifiee comme potentiellement plus robuste mais plus couteuse en complexite et maintenance.

## Correctif minimal cote application

Meme sans correctif profond du downloader, la conversation recommande aussi:

- une borne de retries explicite
- un message d'erreur final clair
- l'arret des boucles de retry indefinies

## Remarque de validite

Cette page est issue d'abord d'une proposition conversationnelle, puis d'une verification des fichiers du depot. Les options 1 et 2 sont observees dans le worktree actuel. L'option 3 reste une piste de secours plus lourde.

## Sources liees

- [../sources/source-2026-04-07-session-qobuz-incompleteread.md](../sources/source-2026-04-07-session-qobuz-incompleteread.md)
- [../sources/source-2026-04-07-session-qobuz-hardening-implementation.md](../sources/source-2026-04-07-session-qobuz-hardening-implementation.md)
