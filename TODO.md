# TODO 21loader

Ce fichier est le backlog operationnel court du projet.
Pour le contexte durable, les liens avec problemes et solutions, voir aussi `wiki/issues/functional-gap-backlog.md`.

## Structure actuelle du projet

- `cmd/server/main.go` initialise l'application locale: chemins applicatifs, services metier, orchestrateur de jobs et serveur HTTP.
- `internal/httpapi/` expose l'UI (`/`) et l'API JSON (`/api/...`) pour les reglages, diagnostics, jobs, Qobuz, RSS, YouTube, LRCLIB et systeme.
- `internal/jobs/` contient le coeur du pipeline: file d'attente, etats, logs, progression, temps d'execution, telechargement, lyrics, transcription, mux et organisation finale.
- `internal/services/` regroupe les integrations externes: YouTube, RSS, Qobuz, diagnostics/outils, modeles Whisper, langues Argos et artwork.
- `internal/core/models.go` centralise les DTO HTTP et les champs de suivi des jobs (`CurrentStepProgress`, compteurs lyrics/Qobuz, temps par etape, resultat final).
- `web/index.html` contient toute l'interface frontend sans framework: formulaire de job, modales de selection, polling de statut, tableau des jobs, logs et reglages.
- `assets/scripts/` contient les scripts Python utilises par Qobuz et Argos Translate.
- `assets/macos/`, `assets/windows/`, `scripts/macos/` et `scripts/windows/` couvrent les icones et le packaging desktop.
- `icone.png` a la racine est la source canonique pour toutes les icones; les assets derives (`assets/ui/21loader-logo.png`, `assets/macos/AppIcon.icns`, `assets/windows/21loader.ico`) doivent etre regeneres depuis cette image.
- `.github/workflows/release.yml` construit les assets de release Windows/macOS quand un tag `v*` est pousse.
- `dist/` contient des sorties de build/de packaging deja generees.

## Constats utiles releves dans le code

- La recherche d'artiste Qobuz dans l'app est deja cablee via `/api/qobuz/search-artists` et une modale dediee dans `web/index.html`.
- LRCLIB est deja integre avec mode automatique, mode manuel et compteurs de lyrics trouves dans le suivi des jobs.
- Le suivi de progression existe deja partiellement: pourcentage de telechargement YouTube, compteurs `done/total` pour Qobuz et lyrics, temps total et temps par etape.
- Le mode `youtube_description` apparait deja dans l'UI, mais le code signale explicitement qu'il n'est pas encore implemente.
- Le logo web est maintenant branche via un asset runtime dedie et un favicon navigateur servi par le backend local.
- Le backend sert directement `icone.png` pour `/app-logo.png` et `/favicon.ico`; les packages copient aussi `icone.png` dans le runtime.
- Le packaging macOS regenere l'icone depuis `icone.png` quand Pillow est disponible, mais peut utiliser `assets/macos/AppIcon.icns` comme fallback CI.
- L'interface telechargement est maintenant separee entre une vue `Ajouter un job` et une vue `Jobs en cours`, avec suivi de file et logs isoles du formulaire.
- Le choix explicite `mp3/mp4/les deux` pour YouTube ne ressort pas dans l'UI actuelle; le backend choisit surtout selon le type de contenu (`audio` ou `video`).
- Les jobs YouTube Music/Audio peuvent maintenant utiliser un format audio par defaut reglable (`mp3`, `m4a`, `opus`, `flac`, `wav`, `aac`) et extraient l'audio via `yt-dlp` au lieu de garder le conteneur natif `.webm`.
- Les jobs traites par `yt-dlp` peuvent integrer les metadonnees et la miniature dans le fichier final via deux reglages actives par defaut; la miniature est ignoree pour `wav`/`aac` car le support conteneur/lecteur est limite.
- Je n'ai pas repere de mode dedie pour `telecharger uniquement les sous-titres`; aujourd'hui les sous-titres sont produits via la transcription.
- Le mode de collision `completer` doit comparer un job RSS a l'URL exacte de l'episode, pas seulement au podcast, sinon un episode peut etre reutilise a la place d'un autre.
- La commande terminal `21loader` est exposee par les packages; `21loader update` consomme la derniere GitHub Release, pas le dernier commit brut de `main`.
- Sur macOS, une app lancee depuis Finder n'herite pas toujours du `PATH` du terminal; 21loader enrichit maintenant son `PATH` avec les dossiers Homebrew/Python utilisateur usuels pour detecter les dependances deja installees.
- Comme le repo GitHub est prive, `21loader update` doit s'authentifier via `LOADER21_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN` ou `gh auth token`.
- Le launcher macOS doit resoudre les symlinks (`/usr/local/bin/21loader`, `~/.local/bin/21loader`) avant de chercher `Contents/Resources/app`.
- Le launcher/updater doit eviter qu'un ancien `21loader-server` inactif continue a servir une ancienne UI apres `21loader update`.
- Les jobs YouTube doivent resoudre `yt-dlp` via le resolver commun, pas l'executer en nom brut, pour couvrir Homebrew quand le `PATH` applicatif est incomplet.

## Zones de code probablement concernees par les prochains sujets

- Progression, logs, compteurs, temps et statut des morceaux: `internal/jobs/coordinator.go`, `internal/jobs/runner.go`, `internal/core/models.go`, `web/index.html`
- Telechargement YouTube, formats de sortie et sous-titres: `internal/jobs/runner.go`, `internal/services/youtube.go`, `web/index.html`
- Lyrics LRCLIB et futur fallback description YouTube: `internal/jobs/runner.go`, `internal/httpapi/router.go`, `web/index.html`
- Organisation des dossiers de sortie: `internal/jobs/organizer.go`, `internal/util/paths.go`, `web/index.html`
- Branding web et packaging d'icones: `web/index.html`, `internal/httpapi/router.go`, `assets/ui/`, `assets/macos/`, `assets/windows/`, scripts de packaging
- Instructions projet persistantes: `AGENTS.md`
- Commande terminal et updater applicatif: `cmd/server/main.go`, `internal/updater/`, scripts de packaging, `.github/workflows/release.yml`
- Resolution des dependances CLI: `internal/util/pathenv.go`, `internal/util/binresolve.go`, `internal/services/diagnostics.go`, `internal/jobs/runner.go`, `internal/services/youtube.go`

## Suivi fonctionnel

### A faire

- [ ] Ne compter un morceau Qobuz comme fait que quand il est termine, pas des qu'il commence
- [ ] Fonction pour telecharger uniquement les sous-titres
- [ ] Reelle barre de progression telechargement / transcription / etc.
- [ ] Poursuivre la refonte graphique de l'interface 21loader au-dela du split `Ajouter un job` / `Jobs en cours`
- [ ] Recuperation des paroles dans la description des videos YouTube
- [ ] Pouvoir gerer directement depuis l'app la tete des ramifications dossiers creees (ex: sur le NAS)
- [ ] Pouvoir choisir une sortie video `mp4` ou une double sortie audio/video pour YouTube
- [ ] Pouvoir obtenir les lyrics sur les musiques YouTube
- [ ] Pouvoir regler le nombre de threads utilises par Whisper

### Deja fait

- [x] Recherche d'artiste directement sur l'app
- [x] Scinder l'espace telechargement en vues `Ajouter un job` et `Jobs en cours`
- [x] Ajout du logo dans l'en-tete et le favicon navigateur
- [x] Afficher la vraie progression de transcription au lieu d'un 43% global trompeur
- [x] Empecher l'ajout d'un job identique deja present en mode `completer`
- [x] Eviter la reutilisation erronee d'un autre episode RSS en mode `completer`
- [x] Ajouter des podcasts RSS favoris selectionnables depuis `Ajouter un job`
- [x] Ajouter les options Whisper avancees (VAD, segmentation, prompt, JSON complet, tinydiarize) avec gestionnaire de modeles VAD
- [x] Permettre de choisir un modele Whisper dedie a tinydiarize dans les reglages globaux et par job
- [x] Reorganiser l'onglet `Reglages` en cartes separees pour les options generales, yt-dlp, Whisper et Qobuz
- [x] Savoir a quel titre on est sur combien plutot qu'un simple pourcentage / une barre
- [x] Pourcentage de telechargement d'une video YouTube
- [x] Recuperation des lyrics sur LRCLIB
- [x] Compteur de lyrics trouves
- [x] Ajouter le temps qu'a mis un job une fois fini
- [x] Ajouter le nom de la video YouTube dans le nom du job quand il s'agit d'une video seule
- [x] Se faire ramener en bas des logs: probleme traite
- [x] Selectionner du texte dans les logs: probleme traite
- [x] Telecharger des musiques sur YouTube
- [x] Traduction des sous-titres YouTube
- [x] Choix de la langue d'arrivee
- [x] Installation automatique
- [x] Ajouter pyannote comme moteur de diarisation optionnel avec merge par segments Whisper, runtime Python dedie et UI guidee
- [x] Ajouter la commande terminal `21loader` et `21loader update` base sur GitHub Releases
- [x] Detecter les dependances Homebrew deja installees quand 21loader est lance depuis Applications/Finder
- [x] Authentifier `21loader update` pour les GitHub Releases d'un repo prive
- [x] Corriger `21loader update` lance via symlink macOS `/usr/local/bin/21loader`
- [x] Executer `yt-dlp` via resolution absolue pour les jobs YouTube sur macOS/Homebrew
- [x] Faire de `icone.png` la source canonique des icones web/macOS/Windows
- [x] Ajouter dans `AGENTS.md` le rappel de proposer une mise a jour GitHub apres chaque modification projet terminee
- [x] Ajouter un reglage de format audio YouTube par defaut et convertir l'audio via `yt-dlp`
- [x] Arreter l'ancien serveur inactif apres update/lancement pour charger la nouvelle UI
- [x] Afficher la release courante dans l'en-tete de la Web UI
- [x] Ajouter des reglages yt-dlp pour integrer les metadonnees et la miniature dans les fichiers telecharges

## Notes

- Le point `Recuperation des paroles dans la description des videos YouTube` semble deja prepare cote UI, mais pas encore execute cote moteur.
- Le point `Reelle barre de progression` est partiellement entame techniquement: il y a deja des compteurs et pourcentages, mais pas encore une restitution complete et homogene dans l'interface.
