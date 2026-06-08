# Schema du wiki

## Principe

Le wiki est la couche de connaissance persistante entre les sources brutes et les futures reponses sur le projet.

Dans ce depot, les sources brutes principales sont:

- les conversations utilisateur/assistant
- les fichiers du repo (`README.md`, `TODO.md`, code, scripts, tests)
- les sorties de diagnostic ou de test quand elles apportent une information durable

Le wiki ne remplace pas les sources. Il les compile.

## Arborescence

- `wiki/README.md`: role du wiki
- `wiki/index.md`: catalogue des pages
- `wiki/log.md`: journal append-only des mises a jour
- `wiki/project-overview.md`: synthese projet stable
- `wiki/architecture.md`: synthese architecture stable
- `wiki/domains/`: vues transverses par domaine fonctionnel ou technique
- `wiki/sources/`: resumes de sources ingerees
- `wiki/issues/`: problemes, risques, limitations, backlog fonctionnel
- `wiki/solutions/`: solutions proposees, implementees ou a valider

## Types de pages

### Page source

But: capturer une conversation, un scan de code ou une autre source durable.

Sections minimales:

- contexte
- source brute ou fichiers d'origine
- informations durables extraites
- pages du wiki impactees
- questions ou incertitudes restantes

### Page issue

But: decrire un probleme, une limitation ou un risque.

Sections minimales:

- statut
- impact
- symptomes ou constat
- cause confirmee ou hypothese
- fichiers/zones concernes
- solutions liees
- sources liees

### Page solution

But: decrire une piste de correction ou une solution retenue.

Sections minimales:

- statut (`proposed`, `implemented`, `validated`, `rejected`)
- probleme cible
- approche
- avantages
- limites / risques
- sources liees

### Page de synthese

But: maintenir une vue stable et compacte du projet.

Regles:

- preferer la synthese a la copie brute
- renvoyer vers les pages source/issue/solution pour le detail
- mettre a jour ces pages quand une information est deja connue plutot que creer une nouvelle page concurrente

### Page de domaine

But: regrouper la connaissance stable d'un grand sujet du projet.

Contenu attendu:

- role du domaine
- composants et fichiers principaux
- comportements utiles a retenir
- risques ou sujets ouverts
- liens vers sources, issues et solutions pertinentes

## Regles de maintenance

1. Toujours preferer mettre a jour une page existante si elle couvre deja le sujet.
2. Distinguer explicitement:
   - fait confirme
   - hypothese
   - information historique a revalider
3. Ne pas effacer l'historique d'un probleme. Si une information devient obsolete, le noter comme tel.
4. Ajouter une entree dans `wiki/log.md` a chaque mise a jour significative.
5. Mettre a jour `wiki/index.md` quand une page est creee ou quand son role change.
6. Lier les pages entre elles avec des liens Markdown explicites.
7. Quand `TODO.md` change de maniere significative, mettre a jour la page backlog du wiki correspondante pour garder le lien entre todo, problemes et solutions.

## Workflow recommande apres une conversation utile

1. Resumer la conversation dans `wiki/sources/`.
2. Si un probleme durable apparait, creer ou mettre a jour une page dans `wiki/issues/`.
3. Si une solution ou une decision apparait, creer ou mettre a jour une page dans `wiki/solutions/`.
4. Si la conversation ajoute ou precise une amelioration produit, mettre a jour `TODO.md` et la page backlog du wiki associee.
5. Mettre a jour les syntheses (`project-overview.md`, `architecture.md`) si l'information change la comprehension globale du projet.
6. Mettre a jour `index.md` et `log.md`.

## Checklist de lint manuel

- une nouvelle information durable n'est pas restee uniquement dans une conversation
- les pages ont des liens entrants et sortants utiles
- les problemes ouverts ont au moins une source associee
- les solutions ne pretendent pas etre validees sans preuve explicite
- les pages historiques signalent quand une revalidation du code actuel est necessaire
