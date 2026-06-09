# Domaine | services media

## Role

Ce domaine couvre les integrations media et de post-traitement hors Qobuz strict.

## Services reperes

- `internal/services/youtube.go`
- `internal/services/rss.go`
- `internal/services/translation_languages.go`
- `internal/services/whisper_models.go`
- `internal/services/vad_models.go`
- `internal/services/diagnostics.go`
- `internal/services/artwork.go`

## API associee

Routes notables relevees dans `internal/httpapi/router.go`:

- `/api/rss/episodes`
- `/api/youtube/catalog`
- `/api/youtube/dates`
- `/api/lyrics/lrclib/search`
- `/api/translation/languages`
- `/api/whisper/models`
- `/api/whisper/vad-models`
- `/api/diagnostics`

## Capacites actuellement connues

- exploration YouTube pour certaines URL de chaine ou playlist
- exploration RSS pour choisir des episodes
- memorisation de flux RSS favoris pour les recharger plus vite depuis l'UI
- recherche LRCLIB pour les lyrics
- installation de paires de traduction Argos
- gestion de modeles Whisper
- gestion de modeles VAD Silero compatibles `whisper.cpp`
- runtime Python dedie pour `pyannote.audio`, verification d'acces au pipeline `community-1` et installation automatique du venv applicatif associe
- diagnostics et installation de dependances
- reglage global du format audio YouTube pour les jobs Music/Audio (`mp3` par defaut, puis `m4a`, `opus`, `flac`, `wav`, `aac` disponibles)
- classement global de preferences audio YouTube pour les jobs Music/Audio, melangeant formats natifs sans conversion et conversions fallback
- reglages globaux yt-dlp pour integrer les metadonnees et la miniature dans les fichiers telecharges, actives par defaut
- option globale de recadrage `500x500` des miniatures YouTube audio/music, appliquee apres le telechargement via `ffmpeg`

## Connaissance durable

- en mode de collision `completer`, un job RSS doit reutiliser une sortie existante uniquement si l'URL media de l'episode correspond reellement au job courant; partager seulement le meme podcast n'est pas un critere suffisant
- les podcasts RSS favoris ne demandent pas de nouvelle API dediee: ils sont persistes dans `WebSettings` et exploites via la route existante `/api/settings`, tandis que les episodes restent charges a la demande via `/api/rss/episodes`
- le catalogue VAD applicatif est volontairement petit dans cette v1: modeles Silero `ggml-silero-v5.1.2.bin` et `ggml-silero-v6.2.0.bin`, installes dans un dossier applicatif separe des modeles Whisper (`.../bin/models/vad`)
- VAD et tinydiarize sont deux fonctions differentes dans l'app: le VAD sert a detecter les segments de parole pour accelerer/fiabiliser la transcription, tandis que tinydiarize sert a marquer les tours de parole et exige un modele Whisper `*-tdrz`
- `pyannote` est maintenant un troisieme etat de diarisation, additif par rapport a `tinydiarize`: il s'appuie sur un venv Python dedie, charge localement `pyannote/speaker-diarization-community-1`, garde la telemetrie desactivee et ne pousse pas l'audio vers un service cloud
- le diagnostic `pyannote` distingue runtime Python absent, module `pyannote.audio` absent, token/acces modele manquant, et runtime completement pret; l'installation auto ne couvre que le runtime local, pas l'acceptation manuelle du modele Hugging Face
- les jobs YouTube Music/Audio doivent demander a `yt-dlp` une extraction audio explicite (`--extract-audio --audio-format <format>`) pour eviter de livrer un `.webm` quand l'utilisateur attend un fichier audio standard
- le classement audio YouTube permet maintenant de preferer un natif sans conversion (`native:m4a`, `native:webm`, `native:best`) avant de tomber sur une conversion (`convert:m4a`, `convert:mp3`, etc.); une conversion n'est pas consideree terminale cote UI/backend pour permettre un fallback si un post-traitement echoue
- l'integration de miniature via `yt-dlp` est appliquee aux formats usuels compatibles (`mp3`, `m4a`, `opus`, `flac` et video); elle est ignoree pour `wav`/`aac` avec avertissement UI car le support conteneur/lecteur est limite
- le recadrage `500x500` des miniatures YouTube audio/music est volontairement non bloquant: en cas d'echec `ffmpeg` ou de thumbnail introuvable, le job garde le fichier audio avec la cover originale et consigne l'avertissement dans ses logs

## Limites / sujets ouverts

- mode `youtube_description` mentionne comme prepare cote UI mais encore non executable selon le scan initial
- choix explicite d'une sortie video `mp4` ou d'une double sortie audio/video YouTube non encore expose clairement comme fonctionnalite finalisee
- telechargement uniquement des sous-titres encore attendu

## Pages liees

- [../project-overview.md](../project-overview.md)
- [../issues/functional-gap-backlog.md](../issues/functional-gap-backlog.md)
- [./jobs-pipeline.md](./jobs-pipeline.md)
- [./frontend-ui.md](./frontend-ui.md)
