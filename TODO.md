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
- `dist/` contient des sorties de build/de packaging deja generees.

## Constats utiles releves dans le code

- La recherche d'artiste Qobuz dans l'app est deja cablee via `/api/qobuz/search-artists` et une modale dediee dans `web/index.html`.
- LRCLIB est deja integre avec mode automatique, mode manuel et compteurs de lyrics trouves dans le suivi des jobs.
- Le suivi de progression existe deja partiellement: pourcentage de telechargement YouTube, compteurs `done/total` pour Qobuz et lyrics, temps total et temps par etape.
- Le mode `youtube_description` apparait deja dans l'UI, mais le code signale explicitement qu'il n'est pas encore implemente.
- Le logo web est maintenant branche via un asset runtime dedie et un favicon navigateur servi par le backend local.
- L'interface telechargement est maintenant separee entre une vue `Ajouter un job` et une vue `Jobs en cours`, avec suivi de file et logs isoles du formulaire.
- Le choix explicite `mp3/mp4/les deux` pour YouTube ne ressort pas dans l'UI actuelle; le backend choisit surtout selon le type de contenu (`audio` ou `video`).
- Je n'ai pas repere de mode dedie pour `telecharger uniquement les sous-titres`; aujourd'hui les sous-titres sont produits via la transcription.
- Le mode de collision `completer` doit comparer un job RSS a l'URL exacte de l'episode, pas seulement au podcast, sinon un episode peut etre reutilise a la place d'un autre.

## Zones de code probablement concernees par les prochains sujets

- Progression, logs, compteurs, temps et statut des morceaux: `internal/jobs/coordinator.go`, `internal/jobs/runner.go`, `internal/core/models.go`, `web/index.html`
- Telechargement YouTube, formats de sortie et sous-titres: `internal/jobs/runner.go`, `internal/services/youtube.go`, `web/index.html`
- Lyrics LRCLIB et futur fallback description YouTube: `internal/jobs/runner.go`, `internal/httpapi/router.go`, `web/index.html`
- Organisation des dossiers de sortie: `internal/jobs/organizer.go`, `internal/util/paths.go`, `web/index.html`
- Branding web et packaging d'icones: `web/index.html`, `internal/httpapi/router.go`, `assets/ui/`, `assets/macos/`, `assets/windows/`, scripts de packaging

## Suivi fonctionnel

### A faire

- [ ] Ne compter un morceau Qobuz comme fait que quand il est termine, pas des qu'il commence
- [ ] Fonction pour telecharger uniquement les sous-titres
- [ ] Reelle barre de progression telechargement / transcription / etc.
- [ ] Poursuivre la refonte graphique de l'interface 21loader au-dela du split `Ajouter un job` / `Jobs en cours`
- [ ] Recuperation des paroles dans la description des videos YouTube
- [ ] Pouvoir gerer directement depuis l'app la tete des ramifications dossiers creees (ex: sur le NAS)
- [ ] Pouvoir choisir `mp3` / `mp4` ou les deux pour les musiques YouTube
- [ ] Pouvoir obtenir les lyrics sur les musiques YouTube

### Deja fait

- [x] Recherche d'artiste directement sur l'app
- [x] Scinder l'espace telechargement en vues `Ajouter un job` et `Jobs en cours`
- [x] Ajout du logo dans l'en-tete et le favicon navigateur
- [x] Afficher la vraie progression de transcription au lieu d'un 43% global trompeur
- [x] Empecher l'ajout d'un job identique deja present en mode `completer`
- [x] Eviter la reutilisation erronee d'un autre episode RSS en mode `completer`
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

## Notes

- Le point `Recuperation des paroles dans la description des videos YouTube` semble deja prepare cote UI, mais pas encore execute cote moteur.
- Le point `Reelle barre de progression` est partiellement entame techniquement: il y a deja des compteurs et pourcentages, mais pas encore une restitution complete et homogene dans l'interface.
