# Domaine | frontend et UI

## Role

Ce domaine couvre l'interface web locale de `21loader` et les connaissances utiles pour ses futures evolutions ergonomiques ou graphiques.

## Piece principale

- `web/index.html`: UI complete sans framework

## Zones visibles relevees dans le depot

- vue dediee `Ajouter un job`
- vue dediee `Jobs en cours` avec tableau et logs
- reglages applicatifs
- diagnostics et dependances
- gestionnaire de modeles Whisper
- gestionnaire de modeles VAD
- pickers/modales Qobuz, RSS, YouTube et LRCLIB

## Contraintes techniques actuellement connues

- conserver l'approche `HTML/CSS/JS` simple sans framework frontend
- respecter le fonctionnement desktop et mobile
- garder la coherence avec l'API locale existante

## Branding web actuellement branche

- `icone.png` a la racine est la source canonique du logo et des icones
- le logo web runtime est servi depuis `icone.png`; `assets/ui/21loader-logo.png` reste un asset derive regenere depuis cette source
- l'interface l'affiche en haut a gauche depuis `web/index.html`
- le backend sert le logo via `/app-logo.png` et le favicon navigateur via `/favicon.ico` avec les octets de `icone.png`

## Connaissance durable issue d'une conversation UI

Une conversation archivee a etabli un mode de collaboration utile pour une future refonte graphique:

- les captures d'ecran ne sont pas obligatoires mais aident beaucoup
- un bon brief UI doit idealement preciser:
  - objectif
  - ecrans prioritaires
  - problemes actuels
  - style vise
  - contraintes techniques
  - references visuelles
- `shadcn/ui` est pertinent comme source d'inspiration ergonomique et visuelle, mais pas comme solution technique directe dans l'architecture actuelle
- Dribbble est utile pour montrer une direction esthetique, mais pas pour valider seul une UX reelle

## Connaissance durable issue de l'implementation UI 2026-04-10

- la navigation principale distingue maintenant `Ajouter un job`, `Jobs en cours`, `Reglages` et `Systeme`
- la vue `Ajouter un job` concentre uniquement la preparation du job et garde les pickers Qobuz/RSS/YouTube/LRCLIB dans le meme fichier `web/index.html`
- la vue `Jobs en cours` isole la file, les actions de pilotage et les logs pour eviter le melange avec le formulaire
- le flux RSS ne doit pas dependre uniquement d'une heuristique d'URL (`feed`, `/rss`, `.xml`): si la source est explicitement choisie en `RSS`, les outils de selection d'episodes doivent rester accessibles
- les favoris RSS se gerent au plus pres du champ URL dans `Ajouter un job`: ajout/retrait a droite de l'entree, puis reouverture via une modale `Podcasts preferes` sans page de gestion separee
- la synthese des jobs doit distinguer une etape `reutilise` d'une etape `ok` quand le mode de collision `completer` saute effectivement un telechargement, une transcription, une traduction ou un mux deja presents
- les options Whisper avancees sont maintenant toutes visibles dans le formulaire job avec aide courte: VAD, segmentation SRT, prompt initial, JSON complet et tinydiarize; seuls les reglages VAD detailles restent dans une zone repliable
- `Reglages moteur` porte les valeurs globales par defaut des memes options Whisper/VAD, puis chaque job peut les surcharger localement
- un podcast RSS favori peut memoriser son propre prompt Whisper directement depuis le formulaire job; quand on re-selectionne ce podcast, le prompt job est pre-rempli avec la priorite `job > podcast > global`
- tinydiarize reste dans l'UI standard mais l'interface rappelle qu'il exige un modele Whisper `*-tdrz`; ses variantes TXT/SRT sont optionnelles et le JSON diarise reste l'artefact prioritaire
- le bloc `Tinydiarize` a ete remplace par un bloc `Diarisation` generique dans `Ajouter un job` et dans `Reglages`, avec un selecteur `Aucun / Tinydiarize / Pyannote`
- `Pyannote` reste visible mais grise dans le selecteur de diarisation tant que le diagnostic ne confirme pas un runtime local pret et un acces valide au modele `community-1`
- `Reglages` ajoute une sous-carte `Pyannote` avec statut runtime, bouton d'installation du venv dedie, champ token Hugging Face masque, verification d'acces et mini assistant pas a pas

## Sujets recurrents a suivre

- progression plus lisible dans le tableau des jobs
- hierarchie visuelle generale de l'interface
- future refonte graphique de l'interface au-dela du split creation/suivi
- harmonisation plus large du branding et des icones desktop/web

## Pages liees

- [../issues/functional-gap-backlog.md](../issues/functional-gap-backlog.md)
- [../sources/source-2026-04-07-session-ui-briefing.md](../sources/source-2026-04-07-session-ui-briefing.md)
- [../domains/jobs-pipeline.md](./jobs-pipeline.md)
