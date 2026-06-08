# Wiki projet 21loader

Ce dossier contient une base de connaissance persistante sur `21loader-cross`.

Le but n'est pas de refaire une documentation statique classique. Le but est de conserver, relier et faire evoluer la connaissance utile du projet au fil du temps, surtout a partir:

- des conversations de travail
- des scans du depot
- des fichiers de reference deja presents dans le repo
- des diagnostics, bugs, tests et decisions prises en cours de route

Le wiki sert notamment a:

- retrouver rapidement l'architecture et les zones de code importantes
- enregistrer des problemes observes
- lier ces problemes a des hypotheses, solutions, correctifs et validations
- relier les elements du backlog `TODO.md` a des problemes, domaines et solutions
- garder une trace chronologique de ce qui a ete appris

Points d'entree:

- [index.md](./index.md): catalogue du wiki
- [schema.md](./schema.md): conventions de maintenance du wiki
- [project-overview.md](./project-overview.md): vue d'ensemble du projet
- [architecture.md](./architecture.md): synthese de l'architecture technique
- [log.md](./log.md): journal chronologique du wiki

Workflow minimal:

1. Ajouter ou mettre a jour une page source dans `wiki/sources/` quand une conversation ou un scan produit une information durable.
2. Propager cette information dans les pages de synthese, d'issue ou de solution concernees.
3. Mettre a jour `wiki/index.md`.
4. Ajouter une entree dans `wiki/log.md`.

Le wiki doit privilegier les faits confirmes, signaler les hypotheses comme telles, et eviter les doublons.
