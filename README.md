# 21loader_crossplatforme

Portage cross-platform du mode web 21loader vers Go.

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

### 0) Installer Go (obligatoire)

L'application se lance avec `go run` et se build avec `go build`, donc Go doit etre installe en premier.
Version minimale recommandee: `go 1.22`.

#### macOS

```bash
brew install go
go version
```

#### Windows

```powershell
winget install --id GoLang.Go -e
go version
```

Si `go` n'est pas reconnu, fermer/reouvrir le terminal (ou session), puis verifier que `C:\Program Files\Go\bin` est dans le `PATH`.

#### Linux

Installer Go via le gestionnaire de paquets:

```bash
# Debian / Ubuntu
sudo apt-get update && sudo apt-get install -y golang-go

# Fedora / RHEL
sudo dnf install -y golang

# Arch / Manjaro
sudo pacman -Sy --noconfirm go
```

Puis verifier:

```bash
go version
```

Si la version fournie par la distribution est inferieure a `1.22`, installer une version officielle depuis `go.dev` (tarball Linux), puis verifier a nouveau `go version`.

### 1) Recuperer le projet

```bash
git clone <url-du-repo>
cd 21loader_crossplatforme
```

### 2) Premier lancement (sans installer manuellement les outils)

```bash
cd 21loader_crossplatforme
go run ./cmd/server --host 0.0.0.0 --port 8080
```

Puis ouvrir `http://localhost:8080`.

### 3) Installer les dependances depuis l'application (recommande)

1. Aller sur `Systeme`.
2. Cliquer `Rafraichir` dans `Diagnostics et dependances`.
3. Utiliser les boutons `Installer` / `Mettre a jour` proposes pour chaque outil manquant.
4. Aller sur `Gestionnaire de modeles Whisper` et installer au moins un modele.
5. Si vous voulez la traduction:
   - activer la traduction dans un job
   - installer la paire de langues via le bouton `+` (Argos) dans la section langues.
6. Aller sur `Reglages`:
   - definir le dossier de sortie par defaut
   - saisir email/mot de passe Qobuz si utilise
   - activer cookies Firefox si necessaire pour YouTube

### 4) Verification rapide apres installation auto

Lancer un job simple depuis `Telechargements`:

- une URL YouTube video publique,
- sans options avancees,
- avec un dossier de sortie valide.

Puis verifier dans le tableau jobs:

- progression des etapes,
- statut final (`completed` attendu),
- presence des fichiers de sortie.

### 5) Si l'installation automatique ne fonctionne pas (fallback manuel)

N'utiliser cette section que si l'installation via `Systeme > Diagnostics` echoue.

#### macOS (Homebrew)

```bash
brew install yt-dlp ffmpeg qobuz-dl whisper-cpp python
python3.13 -m venv "$HOME/Library/Application Support/21loader/argostranslate-venv" \
  || python3.12 -m venv "$HOME/Library/Application Support/21loader/argostranslate-venv" \
  || python3 -m venv "$HOME/Library/Application Support/21loader/argostranslate-venv"
"$HOME/Library/Application Support/21loader/argostranslate-venv/bin/python3" -m pip install --upgrade pip argostranslate
```

#### Windows (winget + pipx)

```powershell
winget install --id yt-dlp.yt-dlp -e
winget install --id Gyan.FFmpeg -e
winget install --id Python.Python.3.12 -e
winget install --id ggml-org.whisper.cpp -e
py -m pip install --user pipx
py -m pipx install qobuz-dl
py -3.13 -m venv "$env:APPDATA\21loader\argostranslate-venv" `
  || py -3.12 -m venv "$env:APPDATA\21loader\argostranslate-venv" `
  || py -3 -m venv "$env:APPDATA\21loader\argostranslate-venv"
"$env:APPDATA\21loader\argostranslate-venv\Scripts\python.exe" -m pip install --upgrade pip argostranslate
```

#### Linux (APT / DNF / Pacman)

##### Debian/Ubuntu (APT)

```bash
sudo apt-get update
sudo apt-get install -y yt-dlp ffmpeg python3 python3-venv pipx
pipx install qobuz-dl
sudo apt-get install -y whisper-cpp || true
python3.12 -m venv "$HOME/.config/21loader/argostranslate-venv" \
  || python3.11 -m venv "$HOME/.config/21loader/argostranslate-venv" \
  || python3 -m venv "$HOME/.config/21loader/argostranslate-venv"
"$HOME/.config/21loader/argostranslate-venv/bin/python3" -m pip install --upgrade pip argostranslate
```

##### Fedora/RHEL (DNF)

```bash
sudo dnf install -y yt-dlp ffmpeg python3 pipx
pipx install qobuz-dl
sudo dnf install -y whisper-cpp || true
python3.12 -m venv "$HOME/.config/21loader/argostranslate-venv" \
  || python3.11 -m venv "$HOME/.config/21loader/argostranslate-venv" \
  || python3 -m venv "$HOME/.config/21loader/argostranslate-venv"
"$HOME/.config/21loader/argostranslate-venv/bin/python3" -m pip install --upgrade pip argostranslate
```

##### Arch/Manjaro (Pacman)

```bash
sudo pacman -Sy --noconfirm yt-dlp ffmpeg python python-pipx
pipx install qobuz-dl
sudo pacman -Sy --noconfirm whisper-cpp || true
python3.12 -m venv "$HOME/.config/21loader/argostranslate-venv" \
  || python3.11 -m venv "$HOME/.config/21loader/argostranslate-venv" \
  || python3 -m venv "$HOME/.config/21loader/argostranslate-venv"
"$HOME/.config/21loader/argostranslate-venv/bin/python3" -m pip install --upgrade pip argostranslate
```

Verification manuelle des binaires (optionnel):

```bash
go version
yt-dlp --version
ffmpeg -version
qobuz-dl --help
whisper-cli --version || whisper-cpp --version
```

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
go build -o 21loader ./cmd/server
```

### Commande terminal

Une installation packagee expose la commande:

```bash
21loader
```

Elle lance le serveur local et ouvre l'UI dans le navigateur.

Pour installer la derniere version publiee sur GitHub Releases:

```bash
21loader update
```

L'updater interroge `https://api.github.com/repos/Axel-codee/21loader_crossplatforme/releases/latest`, telecharge l'asset compatible avec l'OS courant, puis lance l'installation. Les builds officiels sont produits par GitHub Actions quand un tag `v*` est pousse.

### Export macOS (.dmg)

Depuis la racine du projet:

```bash
./scripts/macos/build-dmg.sh
```

Resultat:

- App bundle: `dist/macos/build/21loader.app`
- Installateur: `dist/macos/21loader-<version>.dmg`

Options utiles:

```bash
# Definir une version lisible dans le nom du dmg
./scripts/macos/build-dmg.sh --version 0.1.0

# Utiliser une autre icone (PNG ou ICNS)
./scripts/macos/build-dmg.sh --icon /chemin/vers/icone.png
```

Par defaut, le script utilise `assets/macos/AppIcon.icns`.

### Export Windows (.exe portable)

Depuis la racine du projet:

```bash
./scripts/windows/build-exe.sh --arch amd64
```

Variantes:

```bash
# Build Windows ARM64
./scripts/windows/build-exe.sh --arch arm64

# Forcer une version lisible dans le nom de dossier/archive
./scripts/windows/build-exe.sh --version 0.1.0 --arch amd64
```

Resultat:

- Dossier portable: `dist/windows/21loader-<version>-<arch>/`
- Binaire: `dist/windows/21loader-<version>-<arch>/app/21loader-server.exe`
- Lanceur utilisateur: `dist/windows/21loader-<version>-<arch>/21loader.cmd`
- Archive zip (si `zip` est installe): `dist/windows/21loader-<version>-<arch>.zip`

Sur Windows, lancer `21loader.cmd` (pas directement le `.exe`) pour:

- demarrer le serveur local dans le bon dossier runtime (`app/`),
- ouvrir automatiquement le navigateur.

Le lanceur accepte aussi:

```powershell
21loader update
21loader --version
```

### Export Windows (.exe installable)

Depuis la racine du projet:

```bash
./scripts/windows/build-installer.sh --arch amd64
```

Resultat:

- Installateur: `dist/windows/21loader-<version>-<arch>-setup.exe`

L'installateur extrait l'application dans:

- `%LOCALAPPDATA%\Programs\21loader`

Puis:

- crée un raccourci `21loader` dans le menu Démarrer (avec icône),
- ajoute le dossier d'installation au `PATH` utilisateur pour rendre `21loader` disponible dans un nouveau terminal,
- lance automatiquement `21loader.cmd` à la fin de l'installation.

Utilisation quotidienne (Windows):

- ouvre `21loader` depuis le menu Démarrer (pas besoin de relancer le `setup.exe`).
- ou lance `21loader` dans un nouveau terminal.

Mise à jour locale (Windows):

- page `Systeme` > bloc `Mise à jour de 21loader (Windows)`,
- choisir un fichier `*.zip` (package portable) ou `*-setup.exe`,
- cliquer `Appliquer la mise à jour`.

Mise a jour depuis GitHub Releases:

```powershell
21loader update
```

Fermer l'ancienne instance de 21loader avant de lancer l'update evite les fichiers verrouilles pendant le remplacement Windows.

### Publication GitHub Releases

Le workflow `.github/workflows/release.yml` publie les assets installables quand un tag `v*` est pousse:

```bash
git tag v2026.06.08
git push origin v2026.06.08
```

GitHub Actions execute `go test ./...`, construit le setup/zip Windows et le DMG macOS, puis attache les fichiers a la release. `21loader update` consomme cette derniere release.

### Flags serveur

- `--host` (defaut: `0.0.0.0`)
- `--port` (defaut: `8080`)
- `--open` (ouvre automatiquement le navigateur quand `/healthz` repond)

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

Base config: `os.UserConfigDir()/21loader`

- `web-settings.json`
- `Logs/<job-id>.log`
- `BugReports/`
- `argostranslate-venv/`
- `bin/`
- `bin/models/`

Base cache: `os.UserCacheDir()/21loader`

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
  - installer `argostranslate` via Diagnostics (venv 21loader gere).
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
  - `LOADER21_YT_DATES_CONCURRENCY` (concurrence resolution dates YouTube, bornee a 8).
