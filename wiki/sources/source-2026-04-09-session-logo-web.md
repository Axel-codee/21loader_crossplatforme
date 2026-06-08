# Source | session logo web 2026-04-09

## Contexte

- Demande utilisateur: afficher le logo 21loader en haut a gauche dans l'interface et dans les icones d'onglet du navigateur.

## Constats utiles

- Le logo fourni dans le depot etait `icone.png` a la racine.
- Le packaging desktop embarque deja le dossier `assets/`, ce qui en fait le meilleur point d'ancrage runtime pour un logo web persistant.
- Le backend HTTP ne servait jusque-la que `web/index.html` et les routes API.
- Mise a jour 2026-06-08: `icone.png` a la racine est la source canonique pour toutes les icones; les assets sous `assets/` sont derives de cette source.

## Decision d'implementation

- Servir le logo web via `/app-logo.png` a partir de `icone.png`.
- Servir le favicon navigateur via `/favicon.ico` a partir de `icone.png`.
- Regenerer `assets/ui/21loader-logo.png`, `assets/macos/AppIcon.icns` et `assets/windows/21loader.ico` depuis `icone.png` quand l'icone change.
- Integrer le logo dans la barre haute de `web/index.html` afin qu'il reste visible en haut a gauche sur l'interface.

## Fichiers relies

- `assets/ui/21loader-logo.png`
- `icone.png`
- `assets/macos/AppIcon.icns`
- `assets/windows/21loader.ico`
- `internal/httpapi/router.go`
- `web/index.html`
- `TODO.md`
- `wiki/issues/functional-gap-backlog.md`
