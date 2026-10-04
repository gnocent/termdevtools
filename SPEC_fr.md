*(English version: [SPEC.md](SPEC.md))*

# TermDevTools — Spécification du projet

> Simulateur en mode terminal de la vue "DevTools" de Kibana, pour soumettre des requêtes à un cluster Elasticsearch ou OpenSearch sans passer par un navigateur.

Statut : ce document décrit la conception telle qu'elle est livrée — version 0.6 (bêta). Le journal des versions est dans [CHANGELOG_fr.md](CHANGELOG_fr.md), l'installation pas à pas dans [INSTALL_fr.md](INSTALL_fr.md).

---

## 1. Contexte et objectif

- **Problème résolu** : il arrive qu'un cluster Elasticsearch ou OpenSearch n'ait pas de Kibana (ou d'OpenSearch Dashboards), ou que celui-ci soit hors service — souvent au moment d'une investigation, justement. Un équivalent des Dev Tools directement dans le terminal est alors bien plus efficace qu'une suite de commandes `curl` (gestion du TLS, requêtes sur plusieurs lignes, mise en forme du JSON…).
- **Utilisateurs cibles** : les personnes qui administrent et exploitent des clusters Elasticsearch ou OpenSearch.
- **Environnements cibles** : Linux sans interface graphique d'abord (RHEL 8/9/10 et toute autre distribution amd64), c'est-à-dire les serveurs depuis lesquels on atteint un cluster ; Windows et macOS également, pour un usage depuis son propre poste.
- **Clusters cibles** : Elasticsearch de la 7.17 à la 9.x, et OpenSearch 2.x et 3.x auto-hébergé — la distribution et la version sont détectées à la connexion (§5), et ce que l'outil suggère est sélectionné pour elles (§9.5). OpenSearch managé par AWS, qui impose la signature SigV4 des requêtes, est hors périmètre (§7).
- **Contrainte de portabilité** : un binaire unique par plateforme, sans dépendance système au-delà de la libc de base sous Linux (rien d'équivalent nécessaire sous Windows/macOS), et **sans fichier annexe** : l'outil vise des serveurs sans accès à Internet, où une installation qui se résume à un fichier à copier compte davantage que de pouvoir mettre à jour les données de référence sans nouveau binaire.

## 2. Choix technique

- **Langage retenu** : **Go**
- **Bibliothèque TUI retenue** : [`tview`](https://github.com/rivo/tview) (widgets prêts à l'emploi : `TextArea` pour l'éditeur, `TextView` pour le JSON, `Flex`/`Grid` pour le layout, `SetInputCapture` pour les raccourcis globaux), basé sur [`tcell`](https://github.com/gdamore/tcell). Choisi plutôt que `bubbletea` pour sa simplicité de développement sur ce cas d'usage (layout classique à widgets, pas de rendu custom complexe).
- **Bibliothèque client HTTP/JSON** : stdlib Go (`net/http` + `encoding/json`), pas de dépendance externe nécessaire a priori
- **Méthode de compilation/distribution** : binaire statique (`CGO_ENABLED=0 go build`), pas de dépendance à la libc du système → portable tel quel entre RHEL 8/9/10 (et toute autre distribution Linux amd64), et intégrable facilement dans un outil de déploiement/configuration management existant. [`build-release.sh`](build-release.sh) compile la même source pour `linux/amd64`, `windows/amd64` et `darwin/arm64` en une seule fois, y inscrit la version (`termdevtools --version`) et produit le fichier `SHA256SUMS` qui permet de vérifier un binaire une fois transporté — cf. [INSTALL_fr.md](INSTALL_fr.md).
- **Données de référence** : endpoints, colonnes `_cat` et recettes sont de simples fichiers texte de l'arbre des sources, compilés dans le binaire avec `go:embed` (§9.5).

## 3. Interface utilisateur (TUI)

### 3.0 Connexion

Au lancement, un écran de connexion liste les URLs des clusters connus (pas de nom séparé : l'URL est déjà l'identifiant le plus explicite), triées du plus récemment utilisé au moins récent (ordre de la liste `clusters` dans `config.yaml`, propre à l'utilisateur courant — cf. §9.1 et §9.2), et propose toujours en plus une option **"Nouvelle connexion"**.

- Sélection d'un cluster existant : les champs non sensibles (type d'auth, chemins CA/cert, username, API key ID) sont pré-remplis depuis `config.yaml` ; seul le secret (mot de passe, API key secret, passphrase de clé privée) est redemandé selon le type d'auth.
- **Nouvelle connexion** : formulaire interactif complet à saisir — URL, type d'authentification (aucune / Basic Auth / API Key / certificat client mTLS), puis selon le type : username, API key ID, chemin du CA (pré-rempli avec `default_ca_dir`), chemins cert/clé client (pré-remplis avec `default_client_cert_dir`), activation ou non de la vérification TLS — et enfin le(s) secret(s) correspondant(s).
- **Sélecteur de certificat (`Entrée` sur les champs fichier CA / certificat client / clé client)** : ouvre une popup — un petit navigateur de fichiers, large de 70 colonnes — listant les entrées présentes directement dans le dossier configuré correspondant (`default_ca_dir` pour le champ CA, `default_client_cert_dir` pour les deux champs certificat/clé client — §9.2), à choisir plutôt que de taper un nom de fichier de mémoire ; les sous-dossiers sont listés en premier (suffixés par le séparateur de chemin du système), puis les fichiers, chaque groupe trié alphabétiquement. Flèches puis `Entrée` choisit un fichier ou descend dans un sous-dossier ; `Retour arrière` remonte au dossier parent ; `Echap` annule entièrement sans modifier le champ, quelle que soit la profondeur atteinte. Affiche un message d'erreur clair dans la ligne de message de l'écran de connexion plutôt que d'ouvrir une popup vide quand le dossier correspondant n'est pas configuré ; bascule sur le dossier personnel de l'utilisateur (plutôt que d'échouer) quand il est configuré mais n'existe pas sur le disque (§5).
- **Champs affichés dynamiquement**, pour ne montrer que ce qui est pertinent :
  - URL en `http://` (non https) → champs TLS masqués (fichier CA, case "vérifier le certificat", et les champs certificat/clé client puisqu'un certificat client fait partie de la poignée de main TLS)
  - Authentification "aucune" → aucun champ d'auth affiché
  - "Basic Auth" → uniquement username/mot de passe
  - "certificat client (mTLS)" → uniquement les champs certificat (masque username/mot de passe) ; masqués eux aussi si l'URL est en `http://` (cf. ci-dessus)
- **Champ actif mis en évidence** : le champ ayant le focus est affiché en couleurs inversées (fond/texte), pour rester repérable même quand le seul curseur clignotant ne suffit pas.
- Dans les deux cas, une fois la connexion réussie, l'entrée (nouvelle ou existante) est déplacée/insérée en **première position** de la liste `clusters` dans `config.yaml` — aucun secret n'y est jamais écrit.
- **Détection** : la réponse au `GET /` qui valide la connexion indique aussi quelle distribution et quelle version le cluster exécute (§5). Rien à choisir ni à saisir ; un cluster qui ne peut pas être identifié est connecté malgré tout.
- **Une tentative à la fois** : une tentative abandonnée (`Echap` ou le bouton Annuler) ou supplantée par une plus récente est ignorée quand sa réponse finit par arriver — un cluster lent à répondre ne peut pas remplacer la session ouverte entre-temps sur un autre, et appuyer deux fois sur Se connecter n'ouvre qu'une session.
- **Ce qui est refusé avant toute connexion** : une URL contenant des identifiants (`https://utilisateur:motdepasse@hôte`). L'URL est la seule chose d'un cluster qui soit enregistrée et affichée telle quelle — dans `config.yaml`, dans la barre de statut, dans les commandes `curl` copiées : le mot de passe s'y retrouverait. Le message renvoie à l'authentification Basic Auth.
- **Ce qui n'est pas une connexion** : une réponse `3xx`. Les redirections ne sont jamais suivies (§5) ; l'écran affiche le code et l'adresse vers laquelle la réponse renvoie, à corriger dans l'URL.
- Une fois la connexion effectuée, on ne travaille que sur un seul cluster jusqu'à déconnexion (= fermeture du programme, cf. §4), au sein d'un layout général inspiré des Dev Tools de Kibana.

### 3.1 Layout général

- Écran divisé en deux panneaux verticaux, largeur relative ajustable en cours de session via `F5`/`F6` (cf. §4) :
  - **Panneau gauche** : éditeur de requêtes (texte libre, une ou plusieurs lignes, ex. `GET _cat/indices`). Son contenu de départ est décrit au §3.2.
  - **Panneau droit** : résultat JSON de la dernière requête exécutée (donc vide au démarrage).
  - **Retour à la ligne automatique** dans les deux panneaux (une ligne trop longue pour la largeur affichable est repliée sur l'écran, purement visuel) — n'affecte ni le texte réel de l'éditeur (donc pas le parsing des requêtes, cf. §3.2), ni le contenu exporté/copié depuis le résultat (cf. §3.3). Point d'implémentation à retenir : `TextArea.GetCursor()` et `TextView.ScrollTo()` raisonnent tous deux en **ligne affichée** (post-retour à la ligne), pas en ligne logique, dès que le wrap est actif — s'y fier directement aurait cassé le ciblage de requête (`Ctrl+Entrée`), la complétion (`Tab`) et le défilement de recherche (`Ctrl+F` à droite). Contournés respectivement via `TextArea.GetSelection()` (position en décalage absolu dans le texte, indépendante de l'affichage) et le mécanisme de régions de tview (`Highlight`/`ScrollToHighlight`, ancré au contenu plutôt qu'à un numéro de ligne).
    Un souci apparenté mais distinct est apparu plus tard : `TextArea.Select()` (utilisé pour les résultats de recherche du panneau gauche) est bien à l'abri du problème de wrap — c'est justement la méthode par décalage absolu décrite ci-dessus — mais sa propre documentation est explicite : elle « préserve » le décalage de défilement, contrairement au déplacement normal du curseur (frappe, flèches), que tview fait défiler automatiquement à l'écran. Un résultat hors de la zone visible était donc bien sélectionné en interne, mais restait invisible à l'écran — indiscernable, du point de vue de l'utilisateur, d'une recherche « qui atterrit au mauvais endroit ». Corrigé par `Editor.scrollToCursor` (`ui/editor.go`), qui reproduit la logique de défilement automatique interne de tview (non exposée) via la ligne affichée de `GetCursor` (correcte ici, contrairement à un usage en ligne logique) combinée aux méthodes publiques `GetOffset`/`SetOffset`.

    Un rapport ultérieur — la recherche *sélectionnait carrément le mauvais texte* (pas juste hors écran, ex. un résultat décalé de quelques caractères par rapport au vrai match) — s'est révélé être un second bug, plus fondamental, dans cette même méthode par décalage, sans aucun rapport avec le wrap : `TextArea.GetSelection`/`Select`/`Replace` comptent tous en **octets UTF-8** en interne (confirmé en lisant le code source de tview — le suivi de position avance de `len(cluster)`, la longueur en octets d'une chaîne), un fait qu'aucune de leurs docs ne précise. Le calcul de décalage propre à ce code (`findNext` dans `ui/search.go`, `lineColAt`/`CompletionPrefix` dans `ui/editor.go`) comptait en **runes** (`len([]rune(...))`) partout à la place. Les deux concordent exactement tant que tout caractère précédant la position visée est de l'ASCII sur un seul octet — et divergent silencieusement, de tous les octets supplémentaires accumulés, dès qu'un caractère UTF-8 multi-octet (une lettre accentuée, typiquement) apparaît plus tôt dans le texte. Pas un crash, juste un résultat qui semble faux : une recherche qui sélectionne quelques caractères à côté du vrai match, une complétion d'endpoint qui remplace la mauvaise portion, et — puisque `CursorLine` repose sur le même `lineColAt` — potentiellement **`Ctrl+E` qui cible la mauvaise requête** dès qu'une ligne précédente contenait du texte non-ASCII. Corrigé en faisant compter tout le calcul de décalage propre à ce code en longueur d'octets (`len(s)`) plutôt qu'en runes, pour coller exactement à la convention de tview ; voir `TestLineColAtByteOffsets` et `TestFindNextByteOffsets`, qui échouent tous deux avec le code d'avant le correctif sur un contenu reproduisant le rapport d'origine. Dernier cas du même ordre : la recherche ignore la casse, et mettre tout le texte en minuscules peut en changer la longueur en octets (« İ », deux octets, devient « i̇ », trois) — la casse est donc repliée lettre par lettre, uniquement quand la minuscule occupe le même nombre d'octets (`foldCase`, `ui/search.go`).
- **Barre de statut** : cluster connecté, utilisateur courant, statut "requête en cours..." pendant l'appel, puis code HTTP + temps de réponse une fois la requête terminée. Le chrono affiché en direct pendant l'attente est repoussé en v2 (cf. §7) pour ne pas complexifier la première version. La distribution et la version détectées, sous forme courte (`ES 9.5.4`, `OS 2.19.6`), **prennent la place du libellé « Cluster »** devant l'URL au lieu de s'ajouter à la ligne : sur un terminal de 80 colonnes la ligne n'a pas de place à perdre, et tout ce qu'on ajoute à son début est pris sur le message qui la termine.
- **Écran d'aide (`F1`)** : popup superposé au layout (centré, hauteur proportionnelle au terminal, scrollable si le contenu déborde), rappelant le fonctionnement des deux panneaux, la liste des raccourcis (§4) et l'emplacement des fichiers que l'utilisateur peut créer ou modifier (§9.1). Se ferme avec `Echap`, sans effet sur le contenu de l'éditeur ni du résultat.

### 3.2 Éditeur (panneau gauche)

- Composant tview : `TextArea` (multi-ligne, gère nativement le curseur/la sélection), placé à côté d'une gouttière de numéros de ligne (SPEC.md §7 backlog #5) dans un même panneau bordé — le numéro n'apparaît qu'une fois par ligne logique, vide sur une ligne de continuation issue du retour à la ligne automatique, convention habituelle des éditeurs. `TextArea` n'expose aucune API publique pour savoir où il a coupé, donc la gouttière recalcule cela elle-même (`ui/gutter.go`, `wrapRowStarts`) avec la même bibliothèque sous-jacente (`github.com/rivo/uniseg`) que `TextArea` utilise en interne, restant ainsi synchronisée par construction plutôt que par approximation.
- **Auto-fermeture des accolades/guillemets** (SPEC.md §7 backlog #6, `ui/autoclose.go`) : taper `{`, `[` ou `"` insère aussi le fermant correspondant, curseur placé entre les deux ; taper un fermant déjà présent juste là (typiquement celui qui vient d'être auto-inséré) passe par-dessus au lieu de le dupliquer ; Retour arrière entre une paire vide supprime les deux côtés d'un coup. Volontairement prudent vu que c'était l'item à plus haut risque d'UX du backlog : ne fait rien tant qu'une sélection est active (le comportement par défaut de `TextArea` — remplacer la sélection par le caractère tapé — s'applique alors), et n'auto-ferme pas une accolade tapée comme simple contenu à l'intérieur d'une chaîne déjà ouverte — suivi ligne par ligne en comptant les `"` non échappés avant le curseur, suffisant en soi puisqu'un retour à la ligne non échappé dans une chaîne JSON est du JSON invalide (une chaîne ne peut jamais légitimement s'étendre sur plusieurs lignes).
- Contenu : Texte contenant une ou plusieurs requêtes API (commençant par GET, PUT, POST ou DELETE avec ensuite l'endpoint et les paramètres, et sur les lignes suivantes, le JSON de payload à transmettre). L'éditeur détecte la fin du JSON sous une requête (équilibrage des accolades) pour comprendre la séparation avec la requête suivante. Toute ligne commençant par `#` est un commentaire, donc ignorée.
- Exécution : `Ctrl+E` (aussi `Ctrl+Entrée` quand le terminal le rapporte — cf. §4) exécute la requête où se trouve le curseur, **uniquement si le focus est sur le panneau gauche** (sans effet si le focus est à droite, cf. §4). L'appel est lancé en asynchrone ; la status bar passe à "requête en cours...", puis le panneau droit et la status bar sont mis à jour une fois la réponse obtenue. Seule la réponse à la dernière requête envoyée est affichée : une requête antérieure, plus lente, qui répond après coup est ignorée.
- **Variables réutilisables** (`${nom}`, SPEC.md §7 backlog #4) : une référence `${nom}` n'importe où dans le chemin ou le corps est substituée par une valeur stockée juste avant l'envoi réel de la requête (`Ctrl+E`) ou sa conversion en commande `curl` (`F9`) — `App.resolveRequest`, partagé par les deux, les seules opérations "ce qui serait vraiment envoyé" ; `F4` (reformater) n'y passe volontairement pas, puisqu'il modifie le texte de la requête sauvegardée lui-même et doit laisser `${nom}` tel quel. Une variable non définie bloque l'action avec une erreur en barre de statut la nommant (dédupliquée entre chemin et corps) plutôt que d'envoyer le placeholder littéral. Les valeurs viennent de `~/.config/termdevtools/variables_<URL assainie>.txt` — un fichier par cluster et par utilisateur, même principe que la sauvegarde des requêtes ci-dessous (lignes `nom=valeur`, commentaires `#`, `parseVariables` dans `ui/variables.go`) — chargées à la connexion, rechargées à la demande avec `F7` (pas d'éditeur intégré ; modifié à la main, comme les recettes et endpoints de l'utilisateur, §9.1). Auto-documenté au premier usage, même approche que `config.yaml` (`LoadVariablesFile`, sur le modèle de `config.WriteDefaultConfigFile`).
- **Contenu de départ** : voir « Chargement au démarrage » ci-dessous.
- **Sauvegarde par cluster** : la sauvegarde du contenu de l'éditeur est propre au **cluster auquel on est connecté** (identifié par son URL) **et à l'utilisateur courant** — un fichier `~/.config/termdevtools/queries_<URL assainie>.txt` par cluster déjà utilisé par cet utilisateur (à côté de `config.yaml`, cf. §9.1 pour le détail de l'assainissement du nom de fichier).
- **Déclenchement de la sauvegarde** :
  - `Ctrl+S` (focus panneau gauche) : sauvegarde explicite, avec confirmation dans la status bar.
  - **Automatique en sortie de programme** : le contenu est sauvegardé sans action explicite à la fermeture (`Ctrl+C`, ou signal externe `SIGTERM`/`SIGHUP` — ex. session SSH coupée), en plus du `Ctrl+S` explicite. Best-effort, silencieux (pas de confirmation possible à ce stade). `SIGKILL` reste, comme pour tout programme, impossible à intercepter.
  - **Jamais aux dépens de ce qui est déjà sauvegardé** : le fichier est remplacé en une seule étape (écrit à côté, puis renommé à sa place), si bien qu'un disque plein ou un plantage en cours de sauvegarde laisse le contenu précédent intact. Si la sauvegarde sur `Ctrl+C` échoue, la barre de statut le dit et le programme reste ouvert ; un second `Ctrl+C` quitte malgré tout. Si des requêtes sauvegardées existent mais n'ont pas pu être lues au démarrage, l'éditeur — qui ne les contient pas — n'est pas écrit par-dessus à la sortie ; `Ctrl+S` reste le moyen explicite de les remplacer.
- **Chargement au démarrage**, une fois la connexion à un cluster établie : si une sauvegarde existe déjà **pour ce cluster (URL) et cet utilisateur**, elle est chargée ; sinon un `cheatsheet.txt` à côté du binaire, si une équipe en a installé un ; sinon l'**amorce** intégrée au binaire — un court commentaire d'accueil et une poignée de requêtes valables sur n'importe quel cluster (`GET /`, `_cluster/health`, `_cat/nodes`...), qui renvoie au catalogue de recettes pour le reste. L'amorce remplace la longue cheatsheet que chargeaient les versions jusqu'à la 0.5 : son contenu se trouve désormais dans le catalogue, où l'on cherche et qui s'adapte au cluster, plutôt que dans un mur de texte à faire défiler.
- **Catalogue de recettes (`F8`)** : popup par-dessus le layout, large de 76 colonnes comme l'écran d'aide, listant les requêtes prêtes à l'emploi qui s'appliquent au cluster connecté — une centaine, regroupées par thème (vue d'ensemble, shards et allocation, nœuds et ressources, index, tâches, snapshots, cycle de vie des index, montée de version et maintenance, recherche et documents), en anglais uniquement.
  - Un **champ de filtre** garde le focus en permanence : la frappe resserre la liste (chaque mot tapé doit apparaître, sans tenir compte de la casse, dans le groupe, le titre, les mots-clés ou le corps de la recette), `↑`/`↓` (aussi `Ctrl+P`/`Ctrl+N`, `PgUp`/`PgDn`) déplacent la sélection sans boucler, `Entrée` insère, `Echap` ferme. Les autres raccourcis de l'application sont inactifs tant qu'il est ouvert, sauf `Ctrl+C`.
  - Un **aperçu** sous la liste montre la recette sélectionnée telle qu'elle sera insérée, commentaires estompés.
  - **Insertion** : à la **fin** de l'éditeur, séparée de ce qui précède par une ligne vide, sous un commentaire `# titre` — jamais au curseur, où elle pourrait tomber au milieu d'une requête existante. Le curseur est placé sur la première requête de la recette, de sorte que `Ctrl+E` l'exécute aussitôt ; une seule annulation la retire.
  - Une recette peut contenir plusieurs requêtes et utilise des variables `${nom}` pour ce qui dépend du cluster (`${index}`, `${node}`, `${repository}`, `${snapshot}`, `${field}`, `${task_id}`) : l'exécuter sans que la variable soit définie nomme la variable à définir, comme pour toute requête.
  - Les recettes qui existent sous deux formes (cycle de vie des index : ILM sur Elasticsearch, ISM sur OpenSearch) portent le même titre : seule la forme qui s'applique est listée. Quand le cluster n'a pas pu être identifié, les deux le sont, chacune suivie des clusters auxquels elle est destinée.
  - **Les recettes de l'utilisateur** sont listées avec celles du binaire, signalées comme ajoutées, la bordure de l'aperçu nommant leur fichier. Format, emplacements et règles de fusion : §9.1 et §9.5.
- **Rechargement (`F7`)** : relit tout ce que l'utilisateur ou l'équipe maintient à la main — les variables du cluster, les recettes, les endpoints — et oublie les colonnes `_cat` apprises du cluster. La barre de statut résume ce qui a été chargé, ou nomme le premier problème rencontré (fichier et ligne) et le nombre des autres.
- **Coloration syntaxique : écartée, pas seulement différée** — `tview.TextArea` (seul widget de la bibliothèque permettant l'édition multi-ligne : curseur, sélection, undo, presse-papiers) ne supporte explicitement pas le texte multi-couleur (documentation officielle : *"Multi-color text is not supported"*), contrairement à `TextView` utilisé en lecture seule à droite (§3.3). L'obtenir nécessiterait de reconstruire un éditeur maison au-dessus d'un `TextView` colorable (curseur/sélection/édition réimplémentés à la main) — jugé disproportionné pour cet outil. Vérifié qu'aucune version plus récente de tview ne lève cette limite (v0.42.0 = dernière version au 2026-08-12).
- **Auto-complétion (`Tab` ou `F10`, focus panneau gauche)** : proposée uniquement quand le curseur est en train de taper une ligne `MÉTHODE endpoint_partiel` (pas dans un corps JSON ni ailleurs — `Tab` y garde son comportement standard d'insertion d'une tabulation ; `F10` n'a pas cette signification de repli et est simplement absorbé sans effet). `F10` a été ajouté après avoir confirmé que `Tab` lui-même est absorbé avant d'atteindre l'appli sur certains terminaux (cf. §4) ; aucune logique de repli par modificateur ne peut contourner une touche qui n'atteint jamais l'appli. Comparaison insensible à la casse du préfixe tapé contre la **liste des endpoints connus pour le cluster connecté**, centrée sur l'administration/exploitation (`_cat/*`, `_cluster/*`, `_nodes/*`, endpoints d'admin d'index...) — pas de découverte dynamique des noms d'index réels (idée notée pour une version future, cf. §7).
  - 0 correspondance → message en status bar, rien d'autre.
  - 1 correspondance → complétion directe, sans interaction supplémentaire.
  - Plusieurs correspondances → liste déroulante à choisir (flèches puis Entrée pour valider, Echap pour annuler) ; un `Tab` supplémentaire pendant que la liste est ouverte fait défiler les suggestions. Taper d'autres lettres resserre la sélection sur la première entrée qui commence par ce qui a été tapé jusque-là (insensible à la casse, `Retour arrière` pour corriger) — utile pour sauter directement à une entrée dans une longue liste plutôt que de faire défiler. Le titre de la liste affiche en permanence ce texte de recherche complet (ce qui a été tapé avant `Tab`, plus les touches tapées depuis) — nécessaire car la recherche est un simple préfixe, sans traitement particulier du séparateur `/` : taper `i` juste après avoir complété `_cat` cherche `_cati`, pas `_cat/i`, et ne trouve donc rien puisque chaque candidat `_cat/*` a un `/` à cet endroit ; le texte de recherche visible rend ce résultat compréhensible plutôt que de laisser la liste sans réaction apparente.
  - **`/` final optionnel** : en HTTP, un `/` en toute fin de chemin avant les paramètres est facultatif (`_cat/indices/?h=...` équivaut à `_cat/indices?h=...`). Aucun endpoint connu n'en stocke un, donc il est ignoré pour la comparaison — la complétion remplace tout le segment tapé (le `/` avec), pas seulement ce qui le précède. Ce cas ne se pose pas pour les colonnes `h=`/`s=` ci-dessous : la reconnaissance de commande `_cat` (au préfixe le plus long, à une frontière `/`) l'absorbe déjà naturellement.
  - **Source de la liste** : la liste intégrée au binaire, chaque endpoint étant marqué des clusters où il existe, filtrée pour le cluster connecté (§9.5) ; étendue — jamais remplacée — par un `endpoints.txt` à côté du binaire (celui de l'équipe) et un autre dans le dossier de configuration de l'utilisateur (§9.1), un endpoint par ligne, `#` = commentaire, mêmes marques facultatives.
  - **Liste intégrée** : générée à partir des spécifications d'API officielles — [elastic/elasticsearch-specification](https://github.com/elastic/elasticsearch-specification), chaque branche mineure de `7.17` à `9.5`, et la [spécification d'API d'OpenSearch](https://github.com/opensearch-project/opensearch-api-specification) — puis vérifiée sur des clusters réels (§9.5). Filtrée aux endpoints sans paramètre de chemin (les `/{index}/...` sont hors périmètre, cf. ci-dessus) et aux domaines d'administration : `_cat` (toutes les commandes, `?v` systématique pour les en-têtes de colonnes), `_cluster`, `_nodes`, index, snapshots, tâches, ingest, index orphelins (dangling), et les endpoints de base de recherche/documents (`_search`, `_count`, `_bulk`, `_reindex`...) des deux côtés ; ILM, SLM, licence, features, migration, arrêt de nœud, searchable snapshots, SSL, informations X-Pack côté Elasticsearch ; ISM, snapshot management, `_list`, query insights, search pipelines, remote store côté OpenSearch. Volontairement écartés : ML, sécurité, watcher, transform, rollup, SQL/ES\|QL, CCR, connectors, inference, enrich.
- **Colonnes `h=`/`s=` des commandes `_cat/*`** : cas particulier de l'auto-complétion ci-dessus, prioritaire sur la complétion d'endpoint générique. Reconnu quand le curseur est en train de taper le paramètre `h=` (colonnes affichées) ou `s=` (tri) d'une commande `_cat/xxx` déjà identifiée (ex. `_cat/indices?h=health,st`) :
  - seule la dernière colonne tapée (après la dernière virgule) est complétée, ce qui précède est préservé tel quel ;
  - pour `s=`, si la colonne est déjà suivie de `:`, complète la direction de tri (`asc`/`desc`) plutôt qu'un nom de colonne (ex. `s=docs.count:de` → `desc`) ; ce cas ne s'applique pas à `h=`, où un `:` fait partie du texte comparé tel quel ;
  - **filtre en fin de chemin** : de nombreuses commandes `_cat` acceptent un filtre (nom d'index, de nœud...) entre la commande et les paramètres, ex. `_cat/shards/monindex?h=...`. La commande est reconnue comme la plus longue entrée de la table `commande → colonnes` qui préfixe le chemin à une frontière `/` (jamais une correspondance partielle de mot : `shardsxyz` ne matche pas `shards`) — sans quoi `shards/monindex` ne correspondrait à aucune commande connue et rien ne serait proposé ;
  - la liste de colonnes proposée dépend de la commande `_cat` en cours (ex. les colonnes de `_cat/shards` diffèrent de celles de `_cat/indices`) **et est demandée au cluster lui-même** : la première fois que les colonnes d'une commande sont complétées, `GET _cat/<commande>?help` est envoyé en arrière-plan (limite de 3 secondes ; le nœud qui reçoit la requête y répond seul, sans solliciter le cluster), la réponse est conservée pour la session — jusqu'à `F7` — et la complétion en attente reprend d'elle-même, à condition que l'éditeur n'ait pas changé entre-temps. Exact quelle que soit la version du cluster, y compris plus récente que le binaire, et pour une commande `_cat` ajoutée par l'utilisateur dans `endpoints.txt`. De la réponse ne sont retenus que les noms faits de lettres, de chiffres, de `_`, `.` et `-` : ce qui en sort est proposé dans une liste puis écrit dans l'éditeur, et la réponse d'un cluster n'a pas à y mettre autre chose. Si le cluster ne peut pas être interrogé (délai dépassé, erreur, droits insuffisants), une **table intégrée au binaire** est utilisée à la place, avec un avertissement en barre de statut : générée à partir du `?help` des clusters réels du §9.5 et volontairement prudente — uniquement les colonnes présentes sur toutes les versions testées de l'intervalle pour lequel elles sont données. Il n'existe pas de fichier utilisateur pour ces colonnes : ce que dit le cluster vaut toujours mieux qu'une liste tenue à la main (le `cat_columns.txt` que les versions jusqu'à la 0.5 lisaient à côté du binaire est ignoré). **Seuls les noms complets de colonne sont proposés** (ex. `docs.count`), pas leurs alias courts (`dc`) : plus parlants, et ça limite le nombre de propositions pour les commandes qui ont beaucoup de colonnes (`_cat/indices`, `_cat/nodes`, `_cat/shards`...).

### 3.3 Résultat (panneau droit)

- Format d'affichage : JSON prettifié (réponses usuelles) ou texte casse fixe (réponse à une commande _cat par exemple)
- **Rappel de la requête** : la première ligne du panneau est toujours un commentaire `# MÉTHODE chemin` (sans le corps JSON) rappelant quelle requête a produit le résultat affiché — ex. `# GET _cat/health?v`. Fait partie du texte brut du panneau, donc inclus dans les exports et la copie presse-papier aussi (voir plus bas), pas seulement à l'affichage.
- **En-têtes de la réponse** (SPEC.md §7 backlog #2) : chaque en-tête HTTP de la réponse est listé juste après le rappel, une ligne commentaire `# En-tête: valeur` par en-tête (triés par nom), même traitement en texte brut que le rappel — inclus dans les exports et la copie presse-papier. Gris comme le rappel, sauf un en-tête `Warning` (RFC 7234 — Elasticsearch l'utilise pour signaler une API dépréciée) affiché en jaune pour qu'il se remarque. Absent en cas d'échec de transport (`ShowError`) : il n'y a pas de réponse dont tirer des en-têtes.
- Coloration syntaxique si JSON : oui en v1
- Historique des résultats : Non
- Gestion des réponses volumineuses : Scroll manuel avec touches haut/bas. L'indentation et la coloration se font hors de la goroutine de l'interface, qui reste réactive pendant ce temps. Une réponse de plus de 64 Mo n'est pas affichée : la requête est signalée en échec, avec le conseil de la restreindre (`filter_path`, `size`, `h=`) — chargée en entier, elle épuiserait la mémoire avant que quoi que ce soit ne s'affiche. La durée indiquée couvre toute la réponse, corps compris.
- Affichage des erreurs (requête invalide, cluster injoignable, timeout) : dans la status bar
- **Redirections** : une réponse `3xx` est affichée pour ce qu'elle est — son code, et son en-tête `Location` parmi les en-têtes — jamais suivie (§5).
- **Rien de ce qui est affiché n'est interprété par le terminal** : le corps d'une réponse, ses en-têtes ou un message d'erreur peuvent contenir des séquences d'échappement (écriture dans le presse-papier, changement de titre, texte masqué) ; la bibliothèque d'affichage n'écrit jamais de caractère de contrôle à l'écran, ce qu'un test vérifie pour chaque point d'entrée (`ui/terminal_safety_test.go`).
- **Export (`Ctrl+S`, focus panneau droit)** : écrit le résultat actuellement affiché (rappel de requête inclus) dans le sous-dossier `exports/` du binaire (créé si besoin), nom de fichier horodaté (`AAAAMMJJ-HHMMSS`), extension `.json` si le corps de la réponse est du JSON valide, `.txt` sinon — la ligne `#` en tête fait que le fichier `.json` exporté n'est lui-même plus strictement du JSON valide, un compromis assumé pour la traçabilité. Confirmation (avec chemin) affichée dans la status bar ; erreur (ex. rien à exporter) affichée de la même façon.
- **Copie dans le presse-papier (`F2`)** : quand `config.Mouse` est activé, `tview.Application.EnableMouse(true)` empêche la sélection de texte native du terminal (l'appli capte les événements souris à la place) — pas de sélection possible à la souris dans ce panneau dans ce cas. `F2` copie l'intégralité du résultat affiché quel que soit le réglage souris, via le mécanisme terminal standard **OSC 52** (`tcell.Screen.SetClipboard`) : le terminal local reçoit une séquence d'échappement lui demandant de copier vers *son propre* presse-papier, ce qui fonctionne même à travers SSH (le presse-papier n'est jamais celui du serveur distant). Confirmation affichée dans la status bar, mais **sans garantie que la copie a réellement eu lieu** : ni tcell ni le protocole OSC 52 ne renvoient de confirmation, et le support dépend du terminal (fonctionne sur la plupart des terminaux modernes — Windows Terminal, iTerm2, GNOME Terminal/VTE récent... — mais pas sur PuTTY nu, ni dans tmux/screen sans configuration de passthrough particulière). À vérifier en usage réel.

## 4. Raccourcis clavier

Une barre d'aide, sous la barre d'état, rappelle les raccourcis ; `F1` en donne la liste complète.

| Action | Touche | Statut |
|---|---|---|
| Exécuter la requête sous le curseur | `Ctrl+E` (`Ctrl+Entrée` fonctionne aussi sur les terminaux qui le rapportent) | Défini |
| Basculer focus panneau gauche ↔ droit | `Ctrl+←`/`Ctrl+→` | Défini |
| Quitter l'application (sauvegarde automatiquement le panneau gauche, §3.2) | `Ctrl+C` | Défini |
| Nouvelle requête / effacer l'éditeur | Edition libre du texte pannel gauche | Défini |
| Ouvrir/changer la connexion au cluster | On quitte le programme et on relance | Défini |
| Rechercher dans les requêtes | `Ctrl+F` dans pannel gauche | Défini |
| Rechercher dans le résultat JSON | `Ctrl+F` dans pannel droit | Défini |
| Redimensionner le split gauche/droite | `F5` (rétrécir la gauche) / `F6` (l'agrandir) — `Ctrl+Maj+←/→` fonctionne aussi sur les terminaux qui le rapportent | Défini |
| Sauvegarder (gauche) / exporter (droite) | `Ctrl+S`, comportement contextuel selon le panneau focus (§3.2, §3.3) | Défini |
| Ouvrir le catalogue de recettes | `F8` — taper pour filtrer, `↑`/`↓` pour se déplacer, `Entrée` pour insérer, `Echap` pour fermer (§3.2) | Défini |
| Compléter un endpoint en cours de frappe | `Tab`, `F10` ou `Ctrl+Espace` dans pannel gauche, sur une ligne `MÉTHODE endpoint` (§3.2) | Défini |
| Reformater (ré-indenter) le JSON du corps de la requête sous le curseur | `F4` dans pannel gauche, sans effet si aucun corps ou JSON invalide ; une ligne `#` dans ou avant le corps empêche la ré-indentation et est signalée | Défini |
| Copier la requête sous le curseur en commande `curl` équivalente | `F9` dans pannel gauche — secrets remplacés par un placeholder (§7 backlog #3) | Défini |
| Recharger les fichiers édités à la main (variables, recettes, endpoints) | `F7`, sans restriction de panneau (§3.2) | Défini |
| Afficher l'aide (fonctionnement + raccourcis) | `F1`, `Echap` pour fermer | Défini |
| Copier le résultat dans le presse-papier | `F2` (§3.3) | Défini |
| Changer la langue de l'interface (fr/en) | `F3` | Défini |

> `Ctrl+E` (et `Ctrl+Entrée`, quand le terminal le rapporte) n'est actif que lorsque le focus est sur le panneau gauche (édition des requêtes) — sans effet depuis le panneau droit.
>
> **macOS** : `Ctrl+←/→` est intercepté au niveau du système par défaut (changement d'espace Mission Control) ; `Option`/`Alt+←/→` est accepté comme repli pour changer de panneau (voir `hasShortcutModifier` dans `ui/app.go`).
>
> **Souris** (`mouse: true`, §9.2) : un clic donne le focus au panneau cliqué, et les raccourcis qui dépendent du panneau actif (`Ctrl+S`, `Ctrl+F`, `Ctrl+E`) le suivent ; une popup capte tous les clics tant qu'elle est ouverte, et la liste de complétion se ferme quand le focus la quitte.
>
> Certaines combinaisons (`Ctrl+Entrée`, `Ctrl+Maj+←/→`, `Tab`) sont rapportées de façon incohérente — voire pas du tout — selon le terminal. Des alternatives indépendantes du terminal couvrent chaque cas : `Ctrl+E` (exécuter), `F5`/`F6` (redimensionner), `F10` (compléter). **PuTTY est connu pour mal gérer plusieurs raccourcis et est fortement déconseillé** — aucun souci constaté sous macOS ou Windows 10+ natifs. En cas de doute, préférez les alternatives ci-dessus.

## 5. Connexion au cluster

- cf. §3.0 pour le flux et §9.2 pour le schéma de `config.yaml`
- **Distribution et version** : lues dans le corps du `GET /` qui valide la connexion (`refdata.DetectTarget`), sans requête supplémentaire.
  - `version.distribution` égal à `opensearch` → OpenSearch, à la version `version.number`.
  - Sinon, un `tagline` qui mentionne OpenSearch → OpenSearch **sans version** : c'est son mode compatibilité (`compatibility.override_main_response_version`), qui annonce un faux `7.10.2` et retire le champ `distribution`.
  - Sinon, le `tagline` propre à Elasticsearch (`You Know, for Search`) → Elasticsearch à la version `version.number` — ou sans version si `version.build_flavor` vaut `serverless` (Serverless n'en a pas qui ait un sens).
  - Une autre valeur dans `distribution` (un fork qui s'annonce), ou une réponse qui n'est rien de ce qui précède → inconnu.
  - **Jamais un motif de refus de connexion** : une distribution inconnue signifie que rien n'est filtré, une version absente que les bornes de version sont ignorées (§9.5) — proposer trop vaut mieux que masquer ce qui fonctionne.
  - **Forçage** : `distribution:` et `version:` sur l'entrée d'un cluster dans `config.yaml` (§9.2) remplacent ce qui a été détecté, pour un cluster placé derrière un proxy qui réécrit `GET /` ou un OpenSearch en mode compatibilité. Jamais écrits par le programme ; conservés quand l'entrée remonte en tête de l'historique. Une valeur invalide est ignorée avec un avertissement en barre de statut, et la connexion se poursuit avec ce qui a été détecté.
- **Validation de la connexion** : un `GET /` (15 secondes au plus). Le compte utilisé doit donc y être autorisé — privilège de cluster `monitor` sur Elasticsearch, `cluster:monitor/main` sur OpenSearch ; un `401` ou un `403` est affiché tel quel.
- **Authentification supportée** : aucune, Basic Auth (login/password), API Key (identifiant et secret, envoyés sous la forme `Authorization: ApiKey base64(id:secret)`), certificat client (mTLS)
- **TLS** : vérification de certificat (CA situé par défaut dans `default_ca_dir`, chemin surchargeable par connexion), option pour l'ignorer. TLS 1.2 au minimum (défaut de la bibliothèque standard de Go). Un fichier CA renseigné remplace les autorités du système pour cette connexion.
- **Redirections non suivies** : une réponse `301`, `302`, `307`… est rendue telle quelle. Suivie, une redirection `301`/`302` transforme sans rien dire un `POST`, un `PUT` ou un `DELETE` en `GET` de la nouvelle adresse, et une `307`/`308` rejoue la requête, corps compris, là où la réponse l'envoie : ni l'un ni l'autre n'est ce que l'utilisateur a écrit. `curl`, dont `F9` donne la commande équivalente, ne les suit pas davantage.
- **Pas de proxy** : les variables `HTTPS_PROXY`/`NO_PROXY` ne sont pas prises en compte (§7).
- **Certificats** : deux dossiers par défaut configurables globalement (`default_ca_dir`, `default_client_cert_dir`) pour pré-remplir les chemins lors de la saisie d'une nouvelle connexion et alimenter le sélecteur de certificat (§3.0) — valent `/etc/pki/tls/certs` (dossier système standard des certificats TLS sur RHEL/CentOS) uniquement sous Linux, vides (rien de pré-rempli) sous Windows et macOS où ce chemin n'existe pas ; surchargeable ou effaçable (`""`) selon §9.2. Aucun des deux ne verrouille le champ : si le dossier configuré n'existe pas sur le disque, le sélecteur bascule sur le dossier personnel de l'utilisateur au lieu d'échouer, et le champ reste de toute façon saisissable à la main pour un certificat rangé ailleurs. C'est seulement quand ce repli échoue aussi que le sélecteur affiche un message clair nommant le paramètre en cause, pas une erreur système brute.
- **Clé client protégée par une passphrase** : les deux formats sont lus — le format PKCS#8 chiffré (`BEGIN ENCRYPTED PRIVATE KEY`, celui qu'OpenSSL écrit par défaut depuis la 1.1.0) et le format PEM chiffré historique (`BEGIN RSA PRIVATE KEY` avec un en-tête `DEK-Info`). Pour PKCS#8, que la bibliothèque standard ne prend pas en charge, le schéma PBES2 est implémenté dans `esclient/pkcs8.go` : clé dérivée par PBKDF2 (HMAC-SHA1 à SHA-512), chiffrement AES-128/192/256 ou triple DES en mode CBC — toutes les combinaisons que produit `openssl pkcs8 -topk8 -v2`, vérifiées sur des clés écrites par OpenSSL lui-même. Les clés dérivées par scrypt, et les anciens schémas PBES1 et PKCS#12, sont refusés avec une erreur qui nomme ce qui n'est pas pris en charge ; une passphrase erronée est signalée comme telle.
- **Stockage des secrets** : aucun — mot de passe, API key secret et passphrase de clé privée sont redemandés à chaque connexion ; seuls les éléments non sensibles (URL, type d'auth, username, API key ID, chemins CA/cert) sont persistés dans `config.yaml`, avec l'entrée la plus récemment utilisée en tête de liste. Même règle pour la fonction "copier en cURL" (`F9`, §4) : la commande générée inclut les vrais détails d'auth non sensibles (username, API key ID, chemins de certificat/clé) mais remplace le vrai secret par un placeholder — une copie presse-papier n'a pas de garde-fou équivalent à « jamais écrit sur disque ». Corollaire : une URL contenant des identifiants est refusée (§3.0), puisque l'URL, elle, est enregistrée.
- **Ce qui est tenu pour sûr, et ce qui ne l'est pas** : le dossier du binaire et le dossier de configuration de l'utilisateur sont des entrées de confiance — ce qu'on y dépose (recettes, endpoints, cheatsheet) est proposé à l'utilisateur ; le binaire ne doit donc pas être installé dans un dossier où n'importe qui peut écrire. Ces fichiers sont néanmoins refusés en entier s'ils contiennent des caractères de contrôle (§9.5). Le cluster, lui, n'est pas tenu pour sûr : ses réponses sont affichées sans être interprétées (§3.3), bornées en taille (§3.3), et les noms de colonnes qu'il fournit à la complétion sont filtrés (§3.2).

## 6. Requêtes supportées

- **Syntaxe d'entrée** : format libre façon Kibana Console (`METHODE chemin` + corps JSON optionnel sur les lignes suivantes)
- **Méthodes HTTP à supporter** : GET, POST, PUT, DELETE
- **Validation avant envoi** : vérifier que le JSON du corps est valide avant d'exécuter
- **Timeout par défaut** : 2 minutes, paramétrable via `default_timeout_seconds` dans `config.yaml` (§9.2)

## 7. Hors périmètre et feuille de route

Ce qui n'est pas fait, par choix ou pas encore :

- Chrono de la requête en cours mis à jour en direct dans la status bar (seul le résultat final est affiché : code HTTP + durée totale une fois la réponse reçue) — item 7 ci-dessous.
- Auto-complétion dynamique des noms d'index réels du cluster connecté (la complétion se limite aux endpoints connus et aux colonnes `_cat`, cf. §3.2) — item 10 ci-dessous.
- Coloration syntaxique dans l'éditeur (écartée, cf. §3.2).

**Candidats inspirés de Kibana Dev Tools** — tous livrés à ce stade, à peu près dans l'ordre d'intérêt décroissant proposé initialement : ~~Reformater sur place le JSON du corps de requête~~ ("auto indent" de Kibana) sous `F4` (§4) ; ~~Afficher les en-têtes HTTP de la réponse~~ dans le panneau de résultat (§3.3) ; ~~"Copier en cURL"~~ sous `F9` (§4) — secrets masqués, cf. `esclient.Client.CurlCommand` ; ~~Numéros de ligne dans la gouttière de l'éditeur~~ intégrés au panneau éditeur (§3.2) ; ~~Variables réutilisables~~ sous forme de substitution `${nom}`, rechargée avec `F7` (§3.2, §9.1) ; ~~Auto-fermeture des accolades/guillemets~~ en tapant dans l'éditeur (§3.2, `ui/autoclose.go`).

**Feuille de route après la v0.5** — consignée le 2026-10-01 après comparaison avec geek-fun/dockit, cars10/elasticvue et elastic/cli. L'ordre est la séquence prévue ; chaque item reste à affiner avant d'être réalisé.

1. ~~**Données de référence et recettes intégrées, adaptées à la version**~~ — livré : recettes, endpoints et colonnes `_cat` dans le binaire, sélectionnés selon la distribution (Elasticsearch ou OpenSearch) et la version détectées à la connexion, étendus par les fichiers de l'utilisateur et rechargés avec `F7` (§3.2, §5, §9.1, §9.5). Plus rien d'autre à installer que le binaire. Fait entrer OpenSearch auto-hébergé dans le périmètre.
2. **Proxy HTTP(S)** — respecter `HTTPS_PROXY`/`NO_PROXY`, éventuellement un `proxy:` par cluster.
3. **Authentification étendue** — API key sous sa forme `encoded`, jetons Bearer, Cloud ID. SigV4 reste hors périmètre.
4. **Garde-fous de production** — mode lecture seule par cluster, confirmation configurable pour les chemins destructifs, bandeau « PROD » visible.
5. **Secrets sans ressaisie, premier palier** — lire le secret depuis une variable d'environnement ou un fichier. Le trousseau de l'OS en opt-in est une décision distincte et ultérieure (elle modifie la promesse « aucun secret persisté » du §5).
6. **Mode watch** — rejouer la requête sous le curseur toutes les N secondes et surligner ce qui a changé.
7. **Annulation d'une requête en cours, chrono en direct** — couvre la puce « chrono » ci-dessus.
8. **Diff entre deux exécutions** d'une même requête.
9. **Écran « top » du cluster** — santé, nœuds, shards non assignés ou en relocation, tâches en cours, rejets des thread pools, rafraîchi automatiquement.
10. **Complétion dynamique** — noms d'index, d'alias, de data streams, de champs, de nœuds et de dépôts ; couvre la puce « auto-complétion dynamique » ci-dessus.
11. **Mode headless** — `run -f fichier --json`, sans TUI ; n'a d'intérêt qu'une fois l'item 5 en place.
12. **Navigation dans les gros JSON** — pliage, saut à un chemin, filtre façon jq.

Non planifiés à ce stade, par intérêt décroissant : exports et rapports de plantage dans le dossier de l'utilisateur plutôt qu'à côté du binaire (§9.1) ; complétion des paramètres d'URL et du DSL ; historique des exécutions ; rendu tabulaire ES|QL/SQL ; buffers multiples et fichiers arbitraires ; vue tabulaire avec export CSV/Markdown ; comparaison multi-cluster ; import/export en masse ; **signature des requêtes AWS SigV4**, ce qu'exige OpenSearch managé par AWS (Amazon OpenSearch Service, Serverless) quand il est protégé par IAM — écartée de l'item 1 parce qu'elle n'est ni petite ni isolée : chaque requête doit être signée (requête canonique, empreinte du corps, clé dérivée de la date), et les identifiants proviennent d'une chaîne de sources (environnement, profil partagé, SSO, rôle d'instance ou de conteneur, avec des jetons de session qui expirent en cours de session) qui impose soit le SDK AWS, contraire au binaire statique sans dépendance, soit sa réimplémentation. Un domaine managé qui accepte Basic Auth (contrôle d'accès fin avec base d'utilisateurs interne) devrait être joignable comme n'importe quel OpenSearch — non testé.

Écartés délibérément : assistant IA ou serveur MCP ; formulaires pour snapshots/ILM/templates ; tunnels SSH intégrés ; backends hors API Elasticsearch (DynamoDB, MongoDB…).

## 8. Contraintes non-fonctionnelles

- **Dépendances runtime** : aucune — la libc standard présente sur RHEL 8/9/10 (ou toute autre distribution Linux amd64) sur cette plateforme, rien du tout à installer sous Windows ou macOS
- **Performance** : Doit supporter de gros résultats (plusieurs Mo). 
- **Packaging** : simple binaire à copier, sans fichier à installer à côté, publié avec sa somme SHA-256 (`SHA256SUMS`). `termdevtools --version` dit de quelle version il s'agit, avant toute lecture ou création de fichier. `termdevtools --export-defaults <dossier>` écrit sous forme de fichiers les données qui y sont intégrées — pour lire ce qu'il contient, ou comme exemples pour ses propres fichiers (§9.1) ; il refuse d'écraser un fichier existant, et d'écrire dans l'un des deux dossiers où sont lus les fichiers de référence (les recettes intégrées reviendraient comme celles de l'utilisateur, copies figées masquant les corrections des versions suivantes).
- **Nom du projet / binaire** : termdevtools

## 9. Architecture technique

### 9.1 Emplacement des fichiers

**Rien n'est requis à côté du binaire** : chaque fichier ci-dessous est soit créé par le programme, soit un ajout facultatif. Ce que le programme écrit dans les fichiers qu'il crée — les commentaires qui documentent `config.yaml`, le fichier de variables et le modèle de recettes — est en anglais, quelle que soit la langue de l'interface. L'historique de connexions est propre à l'utilisateur (deux personnes lançant le même binaire partagé sur un même serveur ne doivent pas se marcher dessus), alors qu'une équipe peut vouloir partager des recettes entre tous les utilisateurs d'une installation. Deux emplacements distincts, donc, lus dans cet ordre de priorité après les données intégrées au binaire (§9.5) :

- **Dossier de configuration utilisateur** (`~/.config/termdevtools/`, ou `$XDG_CONFIG_HOME/termdevtools/` si cette variable est définie), créé automatiquement (permissions `0700`) dès la première écriture :
  - `config.yaml` — clusters connus, mis à jour automatiquement à chaque connexion réussie, aucun secret dedans (§9.2). S'il n'existe pas encore, il est créé au démarrage avec chaque paramètre affiché à sa valeur par défaut, précédé d'un commentaire expliquant son rôle — auto-documenté, pour découvrir ce qui est configurable sans lire cette spec. Les valeurs déjà présentes ne sont jamais modifiées ; si le fichier existe mais qu'il manque un ou plusieurs paramètres (ex. sauvegardé par une version plus ancienne du programme, avant qu'un paramètre n'existe, ou réduit à la main à quelques clés), les manquants sont ajoutés de la même façon, pour que le fichier reste entièrement auto-documenté au fil des évolutions du programme. Sa sauvegarde (chaque connexion réussie, `F3`) met à jour les valeurs sur place : les commentaires, et toute clé que cette version ne connaît pas, sont conservés — de même qu'un dossier par défaut volontairement mis à `""`.
  - `queries_<URL assainie>.txt` — un fichier par cluster déjà utilisé par cet utilisateur, contenant la dernière sauvegarde du panneau gauche pour ce cluster (§3.2). Écrit par `Ctrl+S` et automatiquement en sortie de programme. Nom construit à partir de l'URL du cluster, en remplaçant par `_` tout caractère qui n'est ni alphanumérique, ni `.`, `_` ou `-` (donc notamment `:` et `/`) — ex. `https://es-prod.example.com:9200` → `queries_https___es-prod.example.com_9200.txt`. Deux URL différentes qui se ressembleraient au point de produire le même nom après cette normalisation partageraient (cas rare) le même fichier — limite acceptée pour garder des noms lisibles plutôt que hachés.
  - `variables_<URL assainie>.txt` — un fichier par cluster déjà utilisé par cet utilisateur, contenant les valeurs `${nom}` réutilisables substituées dans les requêtes (§3.2, §7 backlog #4) — même principe un-fichier-par-cluster-par-utilisateur et même assainissement de nom que `queries_*.txt`, juste un préfixe différent (`config.VariablesPathForURL`). Modifié à la main (pas d'éditeur intégré), auto-documenté : créé avec un commentaire explicatif à la première connexion de cet utilisateur à ce cluster, s'il n'existe pas encore. Chargé à la connexion, rechargé à la demande avec `F7` (pas de surveillance du fichier).
  - `recipes/*.txt` — les recettes de l'utilisateur, ajoutées au catalogue (§3.2 ; format et règles de fusion au §9.5). Tous les fichiers `.txt` du dossier sont lus, dans l'ordre de leurs noms. Le dossier est créé au premier lancement avec `my-recipes.txt`, un modèle fait uniquement de commentaires — il explique le format et n'ajoute rien tant qu'il n'est pas modifié. Il n'est écrit qu'une fois : l'utilisateur qui supprime le fichier ne le voit pas revenir tant que le dossier subsiste.
  - `endpoints.txt` — les endpoints de l'utilisateur, ajoutés à ceux que propose la complétion (§3.2). Facultatif, jamais créé par le programme.
- **Dossier de l'exécutable** (celui du binaire, pas le répertoire courant du shell) — tout y est facultatif :
  - `recipes/*.txt`, `endpoints.txt` — mêmes formats que ci-dessus, pour un contenu partagé par tous les utilisateurs de cette installation (les recettes d'une équipe). Les fichiers de l'utilisateur passent par-dessus.
  - `cheatsheet.txt` — contenu de départ de l'éditeur propre à une équipe, chargé seulement si aucune sauvegarde `queries_*.txt` n'existe encore pour le cluster/utilisateur courant, à la place de l'amorce intégrée (§3.2).
  - `exports/` — résultats exportés par `Ctrl+S` depuis le panneau droit, un fichier horodaté par export (créé à la demande, §3.3). **Limite connue** : le dossier est celui du binaire, créé avec des droits réservés à celui qui exporte le premier ; dans une installation partagée ou en lecture seule (`/opt`, `/usr/local/bin`), l'export échoue donc pour les autres utilisateurs, avec un message en barre de statut. Le déplacer dans le dossier de l'utilisateur est une décision à prendre (§7).
  - `crash-<horodatage>.log` — écrit uniquement en cas de plantage : la valeur récupérée et une trace de pile complète (`recoverCrash` dans `main.go`), pour diagnostiquer un crash sur un terminal que personne sous les yeux ne peut reproduire ou retranscrire à la main. Best-effort — un panic dans la goroutine de lecture d'entrée propre à tcell, plutôt que dans le code traité par `tview.Application.Run` lui-même, n'est pas capturé ainsi.
- **Fichiers des versions jusqu'à la 0.5**, qui installaient trois fichiers annexes à côté du binaire : `cat_columns.txt` n'est plus lu ; `endpoints.txt` est lu comme les ajouts de l'équipe (ci-dessus) au lieu de remplacer la liste intégrée — sans danger, puisqu'une ligne sans marque ne retire jamais la contrainte de version de l'entrée intégrée qu'elle duplique (§9.5) ; `cheatsheet.txt` garde son rôle. Le dépôt ne contient plus `endpoints.txt`, `cat_columns.txt` ni `cheatsheet.txt.example` à sa racine, et les scripts d'installation ne copient plus que le binaire — ils se contentent de signaler ces restes.

### 9.2 Schéma `config.yaml` (`~/.config/termdevtools/config.yaml`)

```yaml
default_timeout_seconds: 120
language: en  # langue de l'interface : "en" (défaut) ou "fr" — voir le package i18n ; modifiable à la volée avec F3, qui réécrit cette ligne
mouse: false  # support de la souris, désactivé par défaut (cf. §3.3) — tout a un équivalent clavier
default_ca_dir: ""          # pré-remplissage du champ CA et du sélecteur de certificat (§3.0) — vaut /etc/pki/tls/certs (dossier TLS standard RHEL/CentOS) sous Linux, "" (désactivé) sous Windows/macOS
default_client_cert_dir: "" # pré-remplissage des champs cert/clé client (mTLS) et du sélecteur — même règle de défaut

# ordre = historique d'utilisation, le plus récemment connecté en premier
# (pas de nom séparé : l'URL identifie le cluster)
clusters:
  - url: https://es-prod.example.com:9200
    auth_type: basic        # none | basic | api_key | mtls
    username: svc_devtools  # utilisé si auth_type: basic (mot de passe jamais stocké)
    api_key_id: ""          # utilisé si auth_type: api_key (secret jamais stocké)
    tls:
      verify: true
      ca_file: /etc/pki/ca-trust/es-prod-ca.pem
      client_cert: ""        # utilisé si auth_type: mtls
      client_key: ""         # utilisé si auth_type: mtls

  - url: https://es-staging.example.com:9200
    auth_type: none
    tls:
      verify: false

  - url: https://search-behind-proxy.example.com
    auth_type: none
    distribution: opensearch # facultatif : elasticsearch | opensearch — remplace la détection (§5)
    version: "2.19"          # facultatif : remplace la version détectée
    tls:
      verify: true
```

### 9.3 Structure du projet

```
termdevtools/
├── main.go                 // point d'entrée : charge config, lance écran de connexion, puis l'UI
├── go.mod
├── config/
│   └── config.go           // lecture/écriture config.yaml, déplacement de l'entrée utilisée en tête de liste
├── i18n/
│   └── i18n.go             // tables de messages fr/en pour l'interface, sélectionnées via config.Language
├── esclient/
│   ├── client.go           // client HTTP (auth none/basic/api_key/mtls, TLS), exécution d'une requête
│   ├── curl.go             // la requête sous forme de commande curl équivalente, secrets masqués
│   └── pkcs8.go            // déchiffrement des clés client au format PKCS#8 chiffré
├── parser/
│   └── parser.go           // découpe le contenu de l'éditeur en requêtes (méthode, endpoint, payload, commentaires)
├── refdata/                // données de référence intégrées au binaire (§9.5)
│   ├── target.go           // distribution + version, détection depuis « GET / »
│   ├── constraint.go       // marques « @es >=8.7 <9.0 » et leur correspondance
│   ├── endpoints.go        // format d'endpoints.txt, fusion des couches
│   ├── catcolumns.go       // format de cat_columns.txt, lecture de « GET _cat » et de « ?help »
│   ├── recipes.go          // format des fichiers de recettes, fusion des couches
│   ├── catalog.go          // données embarquées + fichiers de l'équipe et de l'utilisateur, sélection pour une cible
│   ├── export.go           // --export-defaults, modèle de recettes de l'utilisateur
│   └── data/               // les fichiers embarqués : endpoints.txt, cat_columns.txt, starter.txt, my-recipes.txt, recipes/*.txt
├── ui/
│   ├── connect.go          // écran de connexion initial (tview.Form)
│   ├── app.go              // assemblage Flex, gestion du focus, raccourcis globaux
│   ├── editor.go           // panneau gauche (TextArea)
│   ├── completion.go       // filtrage par préfixe, détection h=/s= des commandes _cat
│   ├── recipes.go          // popup du catalogue de recettes (F8)
│   ├── result.go           // panneau droit (TextView + coloration JSON)
│   └── statusbar.go        // barre de statut + barre d'aide raccourcis
├── tools/                  // outils de développement, non livrés
│   ├── genrefdata/         // génère refdata/data/endpoints.txt et cat_columns.txt
│   └── testclusters.sh     // démarre/arrête les clusters réels des tests d'intégration
├── internal/testclusters/  // la liste de ces clusters, partagée par le générateur et les tests
└── config.yaml.example
```

### 9.4 Flux d'exécution d'une requête

1. Focus sur le panneau gauche, curseur positionné sur une requête, `Ctrl+E` (ou `Ctrl+Entrée`).
2. `parser` extrait méthode + endpoint + payload JSON autour du curseur et valide le JSON.
3. JSON invalide → message d'erreur en status bar, rien n'est envoyé.
4. JSON valide → appel HTTP lancé dans une goroutine ; status bar → "requête en cours...".
5. Réponse reçue → `esclient` renvoie code HTTP, durée, corps ; mise à jour de l'UI via `QueueUpdateDraw` (thread-safe avec tview) : panneau droit rempli (JSON prettifié et coloré, ou texte brut pour les réponses `_cat`), status bar → code HTTP + durée.

### 9.5 Données de référence (paquet `refdata`)

Ce que proposent la complétion et le catalogue de recettes est de la donnée, pas du code : de simples fichiers texte sous `refdata/data/`, compilés dans le binaire (`go:embed`) et sélectionnés, une fois connecté, pour la distribution et la version du cluster (§5).

- **Pourquoi de simples fichiers embarqués.** Une petite base SQLite au niveau de l'utilisateur a été envisagée puis écartée : l'ensemble représente quelques centaines de lignes en lecture seule ; des fichiers texte restent modifiables à la main, comparables et relisibles, et le même format sert aux ajouts de l'utilisateur ; une base ajouterait soit cgo (perte du binaire statique), soit un gros pilote en Go pur, pour aucune requête dont l'outil ait besoin. Un jeu de fichiers par version a été écarté lui aussi — la fenêtre couverte compte une trentaine de versions mineures — au profit d'un seul catalogue dont chaque entrée dit où elle s'applique.
- **Contraintes.** Une entrée peut être suivie de marques : `@es`, `@es >=8.7`, `@es >=7.17 <9.0`, `@opensearch >=2.4` (`@elasticsearch` et `@os` sont acceptés aussi). Une borne est une version, `>=` inclusive ou `<` exclusive ; plusieurs intervalles peuvent être donnés pour une même distribution.
  - Aucune marque : l'entrée vaut partout.
  - Des marques pour une seule distribution : l'entrée n'est pas proposée sur l'autre.
  - La correspondance **échoue en ouvrant** : sur un cluster dont la distribution est inconnue rien n'est filtré, et sur un cluster dont la distribution est connue mais pas la version, les bornes de version sont ignorées.
- **Fichiers** (sous `refdata/data/`) :
  - `endpoints.txt` — un endpoint par ligne, marques facultatives. Généré, non modifié à la main.
  - `cat_columns.txt` — sections `# _cat/<commande> [marques]`, puis une colonne par ligne avec marques facultatives. Généré. Uniquement le repli du §3.2.
  - `recipes/*.txt` — le catalogue, un fichier par thème, listés dans l'ordre des noms de fichier.
  - `starter.txt` — le contenu de l'éditeur à la première connexion (§3.2).
  - `my-recipes.txt` — le modèle écrit dans le dossier `recipes/` de l'utilisateur (§9.1).
- **Format d'un fichier de recettes** — celui de l'éditeur (requêtes et commentaires `#`) plus des directives écrites en commentaire, de sorte qu'un fichier de recettes peut être collé tel quel dans l'éditeur :
  - `# @group <nom>` — thème des recettes qui suivent ; à défaut, le nom du fichier.
  - `# @recipe <titre>` — démarre une recette, qui court jusqu'au `@recipe` ou `@group` suivant.
  - `# @tags <mots>` — mots supplémentaires pour le filtre du catalogue.
  - `# @es ...`, `# @opensearch ...` — la contrainte de la recette ; avant le premier `@recipe`, la valeur par défaut pour tout le fichier.
  - Toute autre ligne `# @mot` est un commentaire ordinaire. Un fichier sans aucun `@recipe` est une recette unique portant le nom du fichier, à condition qu'il contienne une requête ; un `@recipe` sans requête est signalé comme un problème.
- **Couches.** Les données embarquées d'abord, puis les fichiers de l'équipe (dossier du binaire), puis ceux de l'utilisateur (dossier de configuration) — `endpoints.txt` et `recipes/*.txt` uniquement (§9.1). Les couches **fusionnent**, un fichier ne remplace jamais les données intégrées dans leur ensemble :
  - *Endpoints* : union. Un endpoint déjà connu garde sa contrainte sauf si la couche supérieure en énonce une — ainsi une ligne sans marque (typiquement issue de l'`endpoints.txt` d'une ancienne installation) ne peut pas faire apparaître partout un endpoint propre à certaines versions.
  - *Recettes* : une recette de mêmes groupe et titre (sans tenir compte de la casse) que des recettes d'une couche inférieure les remplace — toutes, puisqu'un titre peut y exister en une variante par distribution. Toute autre recette est ajoutée à la fin de son groupe, ou dans un nouveau groupe après les existants.
  - Une ligne illisible est ignorée et signalée — fichier et ligne — en barre de statut, à la connexion et sur `F7` ; le reste du fichier est chargé. Il en va de même de ce qui serait sinon écarté ou mal compris en silence : une requête, un `@tags` ou une contrainte écrits hors de toute recette, une recette dont la seule « requête » est une ligne que l'éditeur n'exécuterait pas, un intervalle qu'aucune version ne peut satisfaire (`>=9.0 <8.0`). Un fichier qui n'est pas du texte UTF-8 (typiquement de l'UTF-16, tel qu'écrit par `>` sous Windows PowerShell) est signalé et ignoré en entier ; une marque d'ordre d'octets UTF-8 est tolérée. Il en va de même d'un fichier contenant un caractère de contrôle autre qu'une tabulation ou une fin de ligne : une séquence d'échappement, qu'aucun éditeur ne montre, se retrouverait sinon dans les requêtes où la recette est insérée. Une erreur dans les données embarquées elles-mêmes est une erreur de construction, détectée par les tests du paquet.
- **Génération** (`tools/genrefdata`, un outil de développement) :
  - `endpoints` lit les spécifications d'API officielles — le `output/schema/schema.json` de chaque branche mineure d'elastic/elasticsearch-specification de `7.17` à `9.5`, et le document OpenAPI d'OpenSearch — garde les espaces de noms du §3.2 et les endpoints sans paramètre de chemin, et déduit chaque contrainte de l'annotation « since » de la spécification quand elle existe, sinon des branches où l'endpoint est présent. Une courte table de **corrections**, chacune avec sa raison, rectifie ce que les clusters réels contredisent (un « since » faux, une branche en retard sur le produit, des `x-version-added` approximatifs côté OpenSearch).
  - `catcolumns` interroge les clusters réels ci-dessous (`GET _cat`, puis `GET _cat/<commande>?help`).
  - `explain <texte>` montre d'où vient la contrainte d'une entrée.
- **Vérité terrain.** `tools/testclusters.sh up` démarre onze conteneurs mono-nœud, sécurité désactivée : Elasticsearch 7.17.29, 8.0.1, 8.11.4, 8.19.22, 9.0.8 et 9.5.4 ; OpenSearch 2.0.1, 2.11.1, 2.19.6, 3.0.0 et 3.9.0. Les tests d'intégration (`go test -tags integration ./refdata/`) vérifient, sur chacun :
  - que la distribution et la version sont détectées ;
  - que chaque endpoint proposé pour lui y existe, et qu'aucun de ceux qui lui sont masqués ne répond ;
  - que la table `_cat` intégrée nomme les commandes du cluster et aucune colonne qu'il n'a pas ;
  - chaque recette proposée pour lui, en **exécutant chacune de ses requêtes** — sur un petit jeu d'essai : un index avec un alias et un blocage, un dépôt de snapshots, un snapshot — et en contrôlant chaque colonne `_cat` qu'elle nomme auprès du `?help` du cluster (une colonne inconnue est ignorée en silence par `_cat`, seul ce contrôle détecte donc une faute de frappe). Une réponse réduite à `{}` est un échec elle aussi, sauf si elle est listée comme attendue : c'est ainsi que se manifeste un filtre qui ne sélectionne rien.

  Ces tests sont **destructifs** — ils modifient des réglages du cluster, créent, restaurent et suppriment des index et des snapshots — et refusent tout cluster qui n'est pas sur la machine locale, sauf si `TDT_IT_ALLOW_REMOTE=1` est défini. Les conteneurs ne sont publiés que sur l'interface de boucle locale.
- **Précision entre deux versions testées** : prudente. Un endpoint ou une colonne apparu entre deux versions testées est daté par la spécification quand elle le dit, sinon par la première version testée où il existe — mieux vaut ne pas proposer que proposer à tort. Les colonnes `_cat` échappent entièrement à la question à l'exécution, puisqu'elles sont demandées au cluster.
- **Un cluster plus récent que le binaire** reçoit tout ce qui n'a pas de borne supérieure, c'est-à-dire ce qui valait pour la version la plus récente connue à la compilation.

## 10. Questions ouvertes

Aucune question bloquante identifiée à ce stade. Section réutilisable pour toute question qui émergerait pendant l'implémentation.

