# Source | session logo web 2026-04-09

## Contexte

- Demande utilisateur: afficher le logo 21loader en haut a gauche dans l'interface et dans les icones d'onglet du navigateur.

## Constats utiles

- Le logo fourni dans le depot etait `icone.png` a la racine.
- Le packaging desktop embarque deja le dossier `assets/`, ce qui en fait le meilleur point d'ancrage runtime pour un logo web persistant.
- Le backend HTTP ne servait jusque-la que `web/index.html` et les routes API.

## Decision d'implementation

- Copier le logo vers `assets/ui/21loader-logo.png` pour qu'il fasse partie des assets runtime standards.
- Servir le logo web via `/app-logo.png`.
- Servir le favicon navigateur via `/favicon.ico` a partir de `assets/windows/21loader.ico`.
- Integrer le logo dans la barre haute de `web/index.html` afin qu'il reste visible en haut a gauche sur l'interface.

## Fichiers relies

- `assets/ui/21loader-logo.png`
- `internal/httpapi/router.go`
- `web/index.html`
- `TODO.md`
- `wiki/issues/functional-gap-backlog.md`
