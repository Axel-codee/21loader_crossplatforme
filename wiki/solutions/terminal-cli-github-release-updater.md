# Solution | commande terminal et updater GitHub Releases

## Statut

- `implemented 2026-06-08`
- lie a `BACKLOG-15`

## Decision confirmee

- La commande utilisateur cible est `21loader`.
- La mise a jour se lance avec `21loader update`.
- L'update consomme la derniere GitHub Release, pas directement le dernier push sur `main`.

## Implementation

- `cmd/server/main.go` route les sous-commandes `update`, `version` et `--version` avant le lancement serveur.
- `internal/updater/` interroge l'API GitHub Releases, choisit l'asset compatible avec `runtime.GOOS/runtime.GOARCH`, le telecharge dans le dossier temporaire puis lance l'application du package.
- Pour un repo prive, l'updater peut s'authentifier avec `LOADER21_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN` ou, si disponible, `gh auth token`.
- Quand l'authentification est active, le telechargement de l'asset passe par l'URL API GitHub avec `Accept: application/octet-stream` au lieu de dependre de l'URL navigateur publique.
- Windows privilegie les assets `*-setup.exe`, avec fallback zip.
- macOS privilegie le DMG et lance une copie de `21loader.app` vers le bundle cible.
- Les scripts d'application d'update stoppent l'ancien `21loader-server` avant de copier les nouveaux fichiers, afin d'eviter qu'une ancienne UI reste servie apres update.
- Linux reste explicitement non supporte tant qu'aucun asset Linux n'est produit.

## Packaging

- `scripts/windows/build-exe.sh` injecte la version dans le binaire Go.
- `scripts/windows/launch-21loader.cmd` transmet `update` au binaire et utilise `--open` pour le lancement normal.
- L'installateur Windows ajoute le dossier d'installation au `PATH` utilisateur.
- `scripts/macos/build-dmg.sh` injecte la version, transmet `update` depuis le launcher `.app`, cree un lien utilisateur `~/.local/bin/21loader` au lancement et ajoute un script `Install Terminal Command.command` dans le DMG pour installer `/usr/local/bin/21loader`.
- Le launcher macOS resout les symlinks avant de calculer `Contents/Resources/app`, afin que `/usr/local/bin/21loader update` et `~/.local/bin/21loader update` fonctionnent comme le lancement direct du bundle.
- Le launcher macOS lit la version du serveur exposee par `/healthz`: il ouvre le serveur deja actif si c'est la meme version, ou arrete un serveur obsolet et inactif avant de lancer le nouveau binaire.

## Publication

- `.github/workflows/release.yml` compile les packages au push d'un tag `v*`.
- Le workflow execute `go test ./...`, construit les assets Windows/macOS puis les attache a la GitHub Release.

## Limites connues

- Les packages ne sont pas signes/notarises; macOS Gatekeeper et Windows SmartScreen peuvent encore afficher des avertissements.
- Sur Windows, l'update zip tente d'arreter `21loader-server` avant copie; l'installateur `.exe` peut encore dependre de Windows Installer si des fichiers sont verrouilles.
- Sur macOS, l'installation de la commande globale `/usr/local/bin/21loader` peut demander un mot de passe administrateur.
- Si le repo reste prive et qu'aucun token/`gh` authentifie n'est disponible, `21loader update` ne peut pas lire la derniere release.
