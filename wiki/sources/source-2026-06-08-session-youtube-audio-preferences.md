# Source | session preferences audio YouTube

## Contexte

- Date: 2026-06-08
- Sujet: qualite audio YouTube, compatibilite Plex et arbitrage entre formats natifs et conversions.

## Faits confirmes pendant la session

- YouTube fournit souvent le meilleur audio en `webm/opus`, ce qui maximise la qualite sans reconversion mais reste moins pratique pour certains lecteurs ou bibliotheques Plex.
- Convertir un `webm/opus` en `m4a/aac` est generalement preferable a une conversion MP3 a debit comparable, mais reste une recompression avec pertes.
- Le besoin utilisateur n'est pas seulement de choisir un format final: il faut pouvoir classer des priorites telles que `M4A natif`, `WEBM natif`, puis `M4A converti`.
- Une option convertie n'est pas forcement terminale dans l'UI, car une conversion peut echouer et il peut etre utile d'essayer une autre conversion ensuite.

## Decision implementee

- Ajouter `youtubeAudioPreferences` comme liste ordonnee de valeurs `native:<format>` et `convert:<format>`.
- Supporter les natifs `native:m4a`, `native:webm`, `native:best`.
- Supporter les conversions `convert:mp3`, `convert:m4a`, `convert:opus`, `convert:flac`, `convert:wav`, `convert:aac`.
- Garder `youtubeAudioFormat` comme format converti historique/fallback, avec migration implicite vers `convert:<youtubeAudioFormat>` quand la nouvelle liste est absente.

## Pages liees

- [../issues/functional-gap-backlog.md](../issues/functional-gap-backlog.md)
- [../domains/media-services.md](../domains/media-services.md)
- [../domains/frontend-ui.md](../domains/frontend-ui.md)
