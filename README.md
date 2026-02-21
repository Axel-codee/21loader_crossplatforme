# 21loader_crossplatforme

Portage cross-platform du mode web PersoDL vers Go.

Ce projet fournit une application locale complete pour telecharger et organiser des medias depuis YouTube, RSS (podcasts) et Qobuz, avec transcription Whisper, traduction Argos, lyrics LRCLIB, diagnostics des dependances, et supervision en temps reel des jobs.

## Sommaire

- Vue d'ensemble
- Stack et architecture
- Prerequis
- Installation detaillee (toutes plateformes)
- Lancement rapide
- Guide d'utilisation de l'UI
- Fonctionnalites detaillees (liste exhaustive)
- Organisation des sorties media
- Stockage local (settings, logs, cache)
- API HTTP (routes completes)
- Limites connues
- Depannage rapide

## Vue d'ensemble

L'application expose:

- Un backend API en Go.
- Une UI web unique servie localement (`web/index.html`).
- Une queue de jobs sequentielle (un seul job actif).
- Un pipeline en 5 etapes:
  - `download`
  - `lyrics`
  - `transcription`
  - `muxing`
  - `organization`
- Des modules systeme:
  - diagnostics/outils
  - installation/mise a jour de dependances
  - gestionnaire de modeles Whisper
  - gestionnaire des langues Argos

## Stack et architecture

- Go: backend, orchestration jobs, API HTTP, persistences.
- HTML/CSS/JS natif: UI complete sans framework.
- CLI externes integrees:
  - `yt-dlp`
  - `ffmpeg`
  - `qobuz-dl`
  - `whisper-cli` (alias `whisper-cpp` accepte)
  - Python + `argostranslate` (option traduction)
- Services metier:
  - `YouTubeService`
  - `RSSService`
  - `QobuzService`
  - `TranslationLanguageService`
  - `WhisperModelService`
  - `DiagnosticsService`
  - `ArtworkThumbnailService`
- Orchestration:
  - `Coordinator`: validation payloads, queue, etats, logs, actions.
  - `Runner`: execution reelle du pipeline.
  - `Organizer`: rangement final + metadata JSON.

## Prerequis

### Minimum pour lancer

- Go 1.22+.

### Selon features utilisees

- YouTube/RSS fallback: `yt-dlp`.
- Extraction audio, mux, embed artwork: `ffmpeg`.
- Qobuz: `qobuz-dl` (et config qobuz-dl valide).
- Transcription: `whisper-cli` + modele GGML.
- Traduction: Python + `argostranslate`.

## Installation detaillee (toutes plateformes)

Cette section decrit un setup complet "pret a produire des jobs" sur chaque OS.

### 1) Recuperer le projet

```bash
git clone <url-du-repo>
cd 21loader_crossplatforme
```

### 2) Installer les prerequis de base

#### macOS (Homebrew)

1. Installer Homebrew si absent.
2. Installer les outils:

```bash
brew install go yt-dlp ffmpeg qobuz-dl whisper-cpp python
```

3. Installer Argos dans le venv PersoDL:

```bash
python3.13 -m venv "$HOME/Library/Application Support/PersoDL/argostranslate-venv" \
  || python3.12 -m venv "$HOME/Library/Application Support/PersoDL/argostranslate-venv" \
  || python3 -m venv "$HOME/Library/Application Support/PersoDL/argostranslate-venv"
"$HOME/Library/Application Support/PersoDL/argostranslate-venv/bin/python3" -m pip install --upgrade pip argostranslate
```

#### Windows (winget recommande)

1. Ouvrir PowerShell (normal ou admin selon politique machine).
2. Installer les outils:

```powershell
winget install --id GoLang.Go -e
winget install --id yt-dlp.yt-dlp -e
winget install --id Gyan.FFmpeg -e
winget install --id Python.Python.3.12 -e
winget install --id ggml-org.whisper.cpp -e
```

3. Installer `qobuz-dl` via pipx:

```powershell
py -m pip install --user pipx
py -m pipx install qobuz-dl
```

4. Installer Argos dans le venv PersoDL:

```powershell
py -3.13 -m venv "$env:APPDATA\PersoDL\argostranslate-venv" `
  || py -3.12 -m venv "$env:APPDATA\PersoDL\argostranslate-venv" `
  || py -3 -m venv "$env:APPDATA\PersoDL\argostranslate-venv"
"$env:APPDATA\PersoDL\argostranslate-venv\Scripts\python.exe" -m pip install --upgrade pip argostranslate
```

Si `whisper-cli` n'est pas disponible apres installation, utiliser la page `Systeme > Diagnostics` pour l'installation assistee.

#### Linux (APT, DNF, Pacman)

Choisir le bloc correspondant a votre distribution.

##### Debian/Ubuntu (APT)

```bash
sudo apt-get update
sudo apt-get install -y golang-go yt-dlp ffmpeg python3 python3-venv pipx
pipx install qobuz-dl
# Selon distribution:
sudo apt-get install -y whisper-cpp || true
python3.12 -m venv "$HOME/.config/PersoDL/argostranslate-venv" \
  || python3.11 -m venv "$HOME/.config/PersoDL/argostranslate-venv" \
  || python3 -m venv "$HOME/.config/PersoDL/argostranslate-venv"
"$HOME/.config/PersoDL/argostranslate-venv/bin/python3" -m pip install --upgrade pip argostranslate
```

##### Fedora/RHEL (DNF)

```bash
sudo dnf install -y golang yt-dlp ffmpeg python3 pipx
pipx install qobuz-dl
# Selon distribution:
sudo dnf install -y whisper-cpp || true
python3.12 -m venv "$HOME/.config/PersoDL/argostranslate-venv" \
  || python3.11 -m venv "$HOME/.config/PersoDL/argostranslate-venv" \
  || python3 -m venv "$HOME/.config/PersoDL/argostranslate-venv"
"$HOME/.config/PersoDL/argostranslate-venv/bin/python3" -m pip install --upgrade pip argostranslate
```

##### Arch/Manjaro (Pacman)

```bash
sudo pacman -Sy --noconfirm go yt-dlp ffmpeg python python-pipx
pipx install qobuz-dl
# Selon distribution:
sudo pacman -Sy --noconfirm whisper-cpp || true
python3.12 -m venv "$HOME/.config/PersoDL/argostranslate-venv" \
  || python3.11 -m venv "$HOME/.config/PersoDL/argostranslate-venv" \
  || python3 -m venv "$HOME/.config/PersoDL/argostranslate-venv"
"$HOME/.config/PersoDL/argostranslate-venv/bin/python3" -m pip install --upgrade pip argostranslate
```

### 3) Verification des installations

Verifier que les binaires repondent:

```bash
go version
yt-dlp --version
ffmpeg -version
qobuz-dl --help
whisper-cli --version || whisper-cpp --version
```

Verification Argos:

```bash
python3 -c "import argostranslate; print('argostranslate OK')"
```

Sur Windows, utiliser:

```powershell
py -c "import argostranslate; print('argostranslate OK')"
```

### 4) Premier lancement

```bash
cd 21loader_crossplatforme
go run ./cmd/server --host 0.0.0.0 --port 8080
```

Puis ouvrir `http://localhost:8080`.

### 5) Finalisation recommandee dans l'UI

1. Aller sur `Systeme`.
2. Cliquer `Rafraichir` dans `Diagnostics et dependances`.
3. Installer/mettre a jour les outils proposes si besoin.
4. Aller sur `Gestionnaire de modeles Whisper` et installer au moins un modele.
5. Aller sur `Reglages`:
   - definir le dossier de sortie par defaut
   - saisir email/mot de passe Qobuz si utilise
   - activer cookies Firefox si necessaire pour YouTube

## Lancement rapide

```bash
cd 21loader_crossplatforme
go run ./cmd/server --host 0.0.0.0 --port 8080
```

Puis ouvrir:

- `http://localhost:8080`
- ou `http://<IP_MACHINE>:8080` depuis un autre appareil du LAN.

### Build

```bash
cd 21loader_crossplatforme
go build ./cmd/server
```

### Flags serveur

- `--host` (defaut: `0.0.0.0`)
- `--port` (defaut: `8080`)

## Guide d'utilisation de l'UI

L'UI est organisee en 3 pages:

- `Telechargements`
- `Reglages`
- `Systeme`

Workflow "Nouveau job" en 5 blocs:

1. Choix plateforme/type (`auto`, `youtube`, `rss`, `qobuz` + `video/audio/music`).
2. Source (URL ou recherche artiste Qobuz).
3. Selection (albums/videos/episodes selon contexte).
4. Options pipeline (transcription, traduction, lyrics, collisions).
5. Destination + nom perso + arguments avances.

## Fonctionnalites detaillees (liste exhaustive)

### 1) Sources prises en charge

#### YouTube

- Detection automatique des URL YouTube.
- Support:
  - URL video unique.
  - URL collection (chaine/playlist) dans l'UI.
- Catalogue videos via `POST /api/youtube/catalog`.
- Tri videos par date puis position.
- Selection multiple + filtre + select all/clear.
- Resolution asynchrone des dates/durees (`POST /api/youtube/dates`).
- Cache metadata YouTube cote serveur (TTL 24h).
- Option cookies Firefox (globale + payload API).

Gestion anti-bot YouTube dans le runner:

- Detection des erreurs "not a bot".
- Retry automatique avec cookies navigateur.
- Tentatives sur plusieurs navigateurs/profils:
  - firefox
  - chrome
  - brave
  - chromium
  - edge
- Message d'erreur adapte avec instructions concretes.

#### RSS

- Detection auto des flux RSS.
- Parsing robuste:
  - `title`
  - `pubDate`
  - `dc:date`
  - `enclosure`
  - `media:content`
  - `guid`
  - `link`
  - `itunes:image`
  - `media:thumbnail`
- Selection episodes:
  - modal dedie
  - recherche texte
  - select all / clear
- Download episode:
  - direct HTTP si URL audio directe
  - sinon fallback `yt-dlp`
- Gestion artwork podcast/episode:
  - affichage thumbnail dans l'UI via endpoint artwork
  - tentative d'embed dans fichiers audio compatibles

#### Qobuz

- Detection URL Qobuz + type ressource (`artist`, `album`, etc).
- Recherche artistes (`POST /api/qobuz/search-artists`) avec:
  - nom
  - URL
  - country
  - genres
  - bio
  - derniere sortie
  - image
- Chargement discographie artiste (`POST /api/qobuz/artist-catalog`).
- Selection albums:
  - modal dedie
  - filtre texte
  - filtre type de release
  - select all / clear
- Chargement tracks album a la demande (`POST /api/qobuz/album-tracks`).
- Meta albums exposees:
  - date sortie
  - track count
  - release kind
  - Hi-Res
  - artwork
- Auto-initialisation config qobuz-dl si absente (email/password settings).

### 2) Pipeline job

Pipeline fixe:

1. `download`
2. `lyrics`
3. `transcription`
4. `muxing`
5. `organization`

#### Download

- YouTube: `yt-dlp` + progression parsee.
- RSS:
  - direct HTTP si possible,
  - fallback `yt-dlp`.
- Qobuz: `qobuz-dl` avec options par defaut:
  - `-q 27`
  - `--embed-art`
  - `--og-cover`
  - `--no-db`

Modes de reutilisation:

- Policy `complete`: reutilise media existant (YouTube/RSS) si trouve.
- Policy Qobuz `fetchMissingLyrics` (API): reutilise album existant.

#### Lyrics (LRCLIB)

- Active uniquement pour `contentType=music` + toggle actif.
- Scan des fichiers audio du dossier.
- Recherche LRCLIB avec heuristiques de matching (track/artist/album).
- Retries HTTP (backoff) sur erreurs retryables.
- Ecriture:
  - `.lrc` (synced)
  - sinon `.lyrics.txt`
- Skip si lyrics deja presentes.
- Metriques exposees:
  - done/total
  - found/foundTotal
  - failed

#### Transcription (Whisper)

- Active uniquement hors `music` et si toggle active.
- Extraction WAV mono 16k via `ffmpeg`.
- Execution Whisper:
  - `-osrt`
  - `-otxt`
  - `-l <lang>`
  - `-m <model>`
- Detection executable `whisper-cli`/`whisper-cpp`.
- Detection chemin modele parmi:
  - chemin configure
  - modeles app-managed
  - dossiers systeme connus
- En mode `complete`, reuse sidecars existants si trouves.

#### Traduction (Argos)

- Active si:
  - transcription active
  - toggle traduction actif
  - langues source/cible differentes
  - au moins un sidecar a traduire
- Traduction locale via `assets/scripts/argos_translate_file.py`.
- Formats geres:
  - `txt`
  - `srt`
- Progression parsee (`[argos] ... %`).
- En mode `complete`, skip si traductions deja presentes.
- Conservation des variantes langue:
  - ex: `.en.srt` + `.fr.srt`
  - ex: `.en.txt` + `.fr.txt`

#### Muxing

- Applicable si:
  - source YouTube
  - content video
  - sous-titre disponible
- Remux via `ffmpeg` vers MKV:
  - garde flux media existants
  - remplace/injecte pistes sous-titres locales
  - assigne metadata langue/titre
  - definit une piste par defaut

#### Organization

- Deplacement final des fichiers.
- Gestion des collisions selon policy.
- Support deplacement cross-device (fallback copie).
- Ecriture metadata JSON:
  - media path
  - subtitle path
  - transcript path
  - source info
  - date publication
  - original input URL

### 3) Policies de collision

Policies supportees:

- `overwrite`
- `rename`
- `complete`
- `cancel`
- `fetchMissingLyrics` (API, scenario Qobuz specifique)

Note UI:

- L'UI expose `overwrite`, `rename`, `complete`, `cancel`.

### 4) Queue, etats et actions jobs

- Queue strictement sequentielle.
- Etats:
  - `queued`
  - `running`
  - `paused`
  - `completed`
  - `failed`
  - `cancelled`
- Actions:
  - pause
  - resume
  - cancel
- Limitation OS:
  - Unix/macOS/Linux: pause/resume via signaux.
  - Windows: pause/resume non supporte.
- Metriques de progression exposees:
  - pourcentage global
  - progression etape active
  - elapsed total
  - elapsed etape active
  - elapsed download/lyrics/transcription/traduction
  - compteurs Qobuz/Lyrics

### 5) Logs et supervision live

- Logs live collectes depuis chaque process CLI.
- Endpoint texte brut par job:
  - `GET /api/jobs/{id}/logs`
- Buffer in-memory tronque (~160000 chars).
- Fichier log persistant par job (`Logs/<id>.log`).
- UI:
  - tableau jobs avec refresh auto (~2.5s)
  - panneau logs selectionnable
  - alerte echec job (avec copier logs/erreur + ouverture logs)

### 6) Reglages persistants

Reglages sauvegardes:

- `whisperModelPath`
- `useFirefoxCookies`
- `keepTemporaryFilesOnFailure`
- `qobuzEmail`
- `qobuzPassword`
- `defaultOutputRoot`

Comportements:

- Chargement auto au demarrage.
- Sauvegarde JSON en dossier config utilisateur.
- Valeurs par defaut:
  - output root = home
  - keep temp on failure = true
  - firefox cookies = false

### 7) Diagnostics et installation dependances

Detection:

- disponibilite/version/path des outils:
  - `yt-dlp`
  - `ffmpeg`
  - `qobuz-dl`
  - `whisper-cli`
  - `argostranslate`
- detection alias executable.
- detection "needsUpdate" selon package manager.

Install/update automatique:

- macOS: `brew`
- Windows: `winget` (fallback `choco`, `scoop`)
- Linux: `apt-get`, `dnf`, `pacman`

UI systeme:

- action par outil (installer/mettre a jour)
- progression live:
  - stage
  - tool
  - action
  - command
  - logs
- bloc de commandes manuelles recommandees selon plateforme detectee.

### 8) Gestionnaire de modeles Whisper

Catalogue modele integre:

- `tiny`
- `tiny.en`
- `base`
- `base.en`
- `small`
- `small.en`
- `medium`
- `medium.en`
- `large-v1`
- `large-v2`
- `large-v3`
- `large-v3-turbo`

Fonctions:

- listing modeles installes/non installes.
- distinction:
  - installe par app
  - installation externe detectee
- installation modele:
  - progression bytes + pourcentage + stage
- uninstall:
  - uniquement modeles app-managed
- synchronisation avec selecteurs UI:
  - modele par defaut
  - modele override par job

### 9) Gestion langues/paires Argos

- Catalogue runtime:
  - langues
  - paires source->cible
  - statut installe
  - warnings
- Installation d'une paire depuis l'UI (`+`).
- Endpoints:
  - `GET /api/translation/languages`
  - `POST /api/translation/languages/install`

### 10) Artwork thumbnails

- Endpoint:
  - `GET /api/artwork?url=...&size=...`
  - alias `GET /api/rss/artwork?url=...&size=...`
- Fonction:
  - download image distante
  - resize JPEG si decode possible
  - fallback bytes originaux si format non decode
  - cache disque par hash
- Taille clamp:
  - min 48
  - max 256

### 11) Selecteur natif de dossier

Endpoint:

- `POST /api/system/select-directory`

Backends par OS:

- macOS: `osascript`
- Windows: PowerShell `FolderBrowserDialog`
- Linux: `zenity` ou `kdialog`

Utilise par l'UI pour:

- dossier de sortie job
- dossier de sortie par defaut

### 12) UX UI complete

- Navigation par hash:
  - `#downloads`
  - `#settings`
  - `#system`
- Modales:
  - selection albums Qobuz
  - recherche artistes Qobuz
  - selection episodes RSS
  - selection videos YouTube
- Filtres texte sur chaque modal.
- Select all / clear sur chaque modal.
- Resume metriques jobs dans le tableau.
- Boutons actions job:
  - logs
  - pause/resume
  - cancel

## Organisation des sorties media

Top-level selon source:

- YouTube: `OutputRoot/YouTube/...`
- RSS: `OutputRoot/RSS/...`
- Qobuz: `OutputRoot/qobuz/...`

Artefacts possibles:

- media final
- `.srt`
- `.txt`
- variantes langue (`.<lang>.srt`, `.<lang>.txt`)
- artwork (`*.cover.<ext>`)
- metadata (`*.json` ou `album.json`)

## Stockage local (settings, logs, cache)

Base config: `os.UserConfigDir()/PersoDL`

- `web-settings.json`
- `Logs/<job-id>.log`
- `BugReports/`
- `argostranslate-venv/`
- `bin/`
- `bin/models/`

Base cache: `os.UserCacheDir()/PersoDL`

- `Jobs/<job-id>/` (workspace temporaire)
- `Web/RSSArtworkThumbnails/`

Notes:

- Workspace job supprime a la fin sauf selon option `keepTemporaryFilesOnFailure`.
- Config qobuz-dl externe:
  - Unix-like: `~/.config/qobuz-dl/config.ini`
  - Windows: `%APPDATA%/qobuz-dl/config.ini`

## API HTTP (routes completes)

### UI/health/status

- `GET /`
- `GET /healthz`
- `GET /api/status`

### Settings

- `GET /api/settings`
- `PUT /api/settings`

### Diagnostics/dependances

- `GET /api/diagnostics`
- `POST /api/dependencies/install`
- `GET /api/dependencies/install-progress`

### Traduction

- `GET /api/translation/languages`
- `POST /api/translation/languages/install`

### Systeme

- `POST /api/system/select-directory`

### Whisper models

- `GET /api/whisper/models`
- `GET /api/whisper/models/install-progress?modelID=...`
- `POST /api/whisper/models/install`
- `POST /api/whisper/models/uninstall`

### Qobuz

- `POST /api/qobuz/search-artists`
- `POST /api/qobuz/artist-catalog`
- `POST /api/qobuz/album-tracks`

### RSS/YouTube/artwork

- `POST /api/rss/episodes`
- `POST /api/youtube/catalog`
- `POST /api/youtube/dates`
- `GET /api/artwork?url=...&size=...`
- `GET /api/rss/artwork?url=...&size=...`

### Jobs

- `GET /api/jobs`
- `POST /api/jobs`
- `GET /api/jobs/{id}`
- `GET /api/jobs/{id}/logs`
- `POST /api/jobs/{id}/pause`
- `POST /api/jobs/{id}/resume`
- `POST /api/jobs/{id}/cancel`

## Limites connues

- Pause/resume non supporte sur Windows.
- YouTube anti-bot peut bloquer meme avec cookies.
- Policy `fetchMissingLyrics` disponible API mais pas exposee explicitement dans UI.
- CORS ouvert (`*`), a reserver a un reseau de confiance.
- Serveur bind par defaut sur `0.0.0.0` (accessible LAN si firewall ouvert).
- Credentials Qobuz stockes localement dans les settings.

## Depannage rapide

- Outils manquants:
  - ouvrir page `Systeme` puis `Diagnostics` et lancer install.
- Runtime Argos indisponible:
  - installer `argostranslate` via Diagnostics (venv PersoDL gere).
- Modele Whisper introuvable:
  - installer un modele dans `Gestionnaire de modeles Whisper`.
- Qobuz non configure:
  - renseigner email/password Qobuz dans `Reglages`.
- Erreur anti-bot YouTube:
  - activer cookies navigateur
  - valider challenge dans le meme profil
  - relancer

## Notes techniques

- JSON decode strict (`DisallowUnknownFields`).
- Taille max body JSON: 4 MB.
- Variable env utile:
  - `PERSODL_YT_DATES_CONCURRENCY` (concurrence resolution dates YouTube, bornee a 8).
