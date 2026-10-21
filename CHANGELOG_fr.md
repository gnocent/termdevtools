*(English version: [CHANGELOG.md](CHANGELOG.md))*

# Journal des versions

## 0.7.4 (bêta) — octobre 2026

**En une phrase** : le secret d'un cluster peut être lu dans une variable d'environnement ou un fichier au lieu d'être saisi à chaque connexion.

### Nouveautés

- **Secret sans ressaisie** : `secret_env: NOM` (une variable d'environnement) ou `secret_file: chemin` (un fichier, chemin absolu ou en `~/`), ajoutés à la main sur l'entrée d'un cluster dans `config.yaml`, disent où lire son secret — mot de passe, secret ou forme `encoded` d'une API key, jeton Bearer, passphrase de la clé client, celui que demande son authentification. Le formulaire s'ouvre avec le champ rempli et le focus sur « Se connecter » : `Entrée` suffit. La zone de message nomme la variable ou le fichier.
- **Jamais bloquant** : variable absente ou vide, fichier manquant, illisible, vide ou de plus de 64 Kio, chemin relatif, les deux clés sur une même entrée, ou un fichier qui n'est pas un fichier texte ordinaire en UTF-8 (un tube, un périphérique, de l'UTF-16 comme en écrit `>` dans Windows PowerShell 5.1) — le champ reste vide, le message dit pourquoi, et le secret se saisit comme avant. Rien n'est envoyé avant « Se connecter ».
- **Le programme n'écrit toujours aucun secret** : `config.yaml` dit où est le secret, jamais ce qu'il est, et `F9` garde ses rappels à compléter. Le fichier est sous votre responsabilité : sous Linux et macOS, un fichier que d'autres utilisateurs peuvent lire est utilisé quand même, avec un avertissement (`chmod 600`).

### Corrections

- **Formulaire de connexion sur 80 colonnes** : l'URL d'une nouvelle connexion, les chemins du certificat client, de sa clé et du CA, et en français la passphrase, débordaient sur le cadre du formulaire ; ces champs sont désormais rétrécis pour y tenir, leur contenu défilant.
- **Champ secret après un changement d'authentification** : passer à un autre type d'authentification puis revenir laissait le champ vide à l'écran alors que le secret saisi était toujours envoyé ; le champ montre désormais ce qu'il contient.
- **Changement de panneau sous PuTTY** (#8) : l'aide (`F1`) et la documentation réservaient `Alt+←/→` à macOS ; ce raccourci change de panneau sur tous les systèmes, et c'est celui à utiliser là où `Ctrl+←/→` ne passe pas, comme sous PuTTY. Le conseil de l'aide sur PuTTY le cite désormais.

### À savoir avant de mettre à jour depuis la 0.7.3

- **Rien à faire** : un `config.yaml` sans `secret_env` ni `secret_file` se comporte comme avant — le secret est demandé à chaque connexion.

### Vérifications

- `go vet` et `go test ./...` passent, sous Windows : tests unitaires et tests de l'interface sur un écran simulé de 80×24 ; les tests des secrets passent aussi sous Linux, où sont vérifiés l'avertissement sur un fichier lisible par d'autres et le refus d'un périphérique.
- Basic Auth, API key et jeton Bearer se connectent sur un Elasticsearch 9.5.4 réel dont la sécurité est activée, chacun avec son secret lu dans une variable et dans un fichier, en passant par le formulaire de connexion. Les tests réels de l'authentification et de la protection (Elasticsearch 9.5.4, OpenSearch 3.9.0) passent toujours.
- Une relecture de sécurité des secrets a précédé la publication : aucune fuite du secret, rien d'envoyé avant « Se connecter ». Les deux points relevés — un tube ou un terminal désigné par `secret_file` figeait l'interface, un fichier en UTF-16 envoyait un secret déformé — sont corrigés et couverts par des tests.
- Recette visuelle des messages du formulaire, en français et en anglais, en 80×24.
- `govulncheck` : aucune vulnérabilité connue, ni dans les dépendances ni dans la bibliothèque standard de Go 1.27.3, qui compile les binaires.
- Les données de référence (recettes, endpoints, colonnes `_cat`) n'ont pas changé depuis la 0.6.
- Aucun audit externe indépendant n'a été réalisé.

### Limites connues

- Le secret est lu à l'ouverture du formulaire ; `F9` ne renvoie ni à la variable ni au fichier (il garde son rappel à compléter). Pas de trousseau du système.
- Une « nouvelle connexion » saisie avec l'URL d'un cluster déjà connu remplace son entrée : les clés écrites à la main (`secret_env`, `secret_file`, `proxy`, `distribution`, `version`) sont perdues. Choisir le cluster dans la liste les conserve.
- Les limites de la 0.7 restent valables.

## 0.7.3 (bêta) — octobre 2026

**En une phrase** : un cluster de production peut être protégé — confirmation ou refus des requêtes qui écrivent, et un bandeau qui le signale.

### Nouveautés

- **Protection par cluster**, choisie dans le formulaire de connexion, juste sous l'URL, et enregistrée dans `config.yaml` (`protection:`) :
  - « confirmation des écritures » (`confirm`) : une requête qui écrit ouvre une fenêtre qui nomme le cluster et la requête, avec le focus sur « Annuler » — `Entrée` par réflexe ou `Echap` n'envoie rien. « Envoyer » envoie la requête telle qu'elle a été résolue au `Ctrl+E`.
  - « lecture seule » (`read_only`) : une requête qui écrit est refusée, rien n'est envoyé, et la barre de statut dit comment l'autoriser.
  - « aucune » (`none`), le défaut : rien ne change pour les clusters existants.
- **Ce qui écrit** : tout `PUT`, tout `DELETE`, et tout `POST` absent d'une liste de lectures connues (`_search`, `_count`, `_msearch`, `_mget`, `_field_caps`, `_analyze`, `_explain`, les simulations de pipeline…) — `_bulk` et `_delete_by_query`, mais aussi `_refresh`, `_flush` ou `_forcemerge`, qui agissent sur le cluster. Le classement porte sur la requête après substitution des variables `${nom}`. Les `GET` passent, sauf `_refresh`, `_flush` et le SQL ou le PPL d'OpenSearch ; `F9`, la complétion et les recettes ne sont pas concernés. Le chemin classé est celui qui part réellement (sans ce qui suit un `#`), et un chemin à segment `.` ou `..` est toujours une écriture.
- **Bandeau** en haut de l'écran principal d'un cluster protégé : rouge en confirmation, jaune en lecture seule, avec un libellé facultatif (« PROD · lecture seule »). Il suit la langue de l'interface (`F3`). Un cluster sans protection garde exactement la mise en page des versions précédentes.
- **Historique des clusters** : la protection apparaît sous l'URL de chaque cluster protégé (`auth: basic · PROD · confirmation des écritures`).
- **Valeur inconnue** dans `config.yaml` (une faute de frappe comme `protection: readonly`) : la lecture seule s'applique, la plus restrictive, et le formulaire le signale.

Le niveau est fixé à la connexion : pour en changer, on se reconnecte. C'est un filet contre l'erreur de manipulation, pas une sécurité — n'importe qui pouvant lancer l'outil peut le retirer. Pour empêcher vraiment d'écrire, utilisez un compte sans droit d'écriture sur le cluster.

### Corrections

- **Le message de la barre d'état n'est plus coupé dès son premier mot sur un terminal de 80 colonnes** : une URL et un nom d'utilisateur réalistes ne lui laissaient que quelques colonnes. Il passe désormais avant le reste — l'utilisateur s'efface d'abord, puis l'URL est raccourcie, puis le cluster — et n'est coupé, avec un `…`, que s'il ne tient pas seul.

### Sécurité

- **Binaires compilés avec Go 1.27.3**, la dernière version de Go ; `go.mod` demande désormais cette version.

### À savoir avant de mettre à jour depuis la 0.7.2

- **Rien à faire** : un `config.yaml` sans `protection:` se charge comme avant, sans protection ni bandeau, et une connexion sans protection n'écrit pas de clé `protection:`.
- **Barre d'état** : sur un terminal étroit, le nom d'utilisateur, puis l'URL, s'effacent devant le message quand la place manque.

### Vérifications

- `go vet` et `go test ./...` passent, sous Windows : tests unitaires, dont un cas par motif de la liste des lectures, et tests de l'interface sur un écran simulé de 80×24.
- La protection est vérifiée sur un Elasticsearch 9.5.4 et un OpenSearch 3.9.0 réels : la suppression d'un index est refusée en lecture seule, où une recherche passe ; elle n'a pas lieu tant que la confirmation est annulée, a lieu une fois confirmée, et aussitôt sans protection. Les deux clusters exécutent bien `GET _refresh` et `GET _flush`, classés en écritures.
- Une relecture de sécurité de la protection a précédé la publication ; les contournements relevés (un `#` dans le chemin, des segments `..`, `GET _refresh`) sont corrigés et couverts par des tests.
- Recette visuelle des écrans de la protection, en 80×24.
- `govulncheck` : aucune vulnérabilité connue, ni dans les dépendances ni dans la bibliothèque standard de Go 1.27.3.
- Les données de référence (recettes, endpoints, colonnes `_cat`) n'ont pas changé depuis la 0.6.
- Aucun audit externe indépendant n'a été réalisé.

### Limites connues

- La liste des lectures n'est pas extensible : une lecture qui n'y figure pas est confirmée, ou refusée en lecture seule, à tort. Seuls Elasticsearch 9.5 et OpenSearch 3.9 ont servi aux essais réels de la protection.
- Les limites de la 0.7 restent valables.

## 0.7.2 (bêta) — octobre 2026

**En une phrase** : TermDevTools s'installe avec `go install`, et la complétion garde la requête qui suit le curseur et accepte un `/` initial.

### Nouveautés

- **`go install github.com/gnocent/termdevtools@latest`** : le module porte désormais le chemin du dépôt, et les versions sont taguées sur trois nombres (`v0.7.2`), la forme que Go reconnaît. Le binaire est placé dans `GOBIN`, ou dans `$(go env GOPATH)/bin` si `GOBIN` n'est pas défini. Les binaires précompilés restent la voie recommandée.
- **`--version` sans version inscrite** : un binaire installé avec `go install` donne la version installée, et un binaire compilé par un simple `go build` dans un clone donne la version que Go déduit de git (`v0.7.3-0.<date>-<commit>`, avec `+dirty` si des fichiers ont été modifiés) au lieu de `dev`. `go run` affiche toujours `dev`.

### Complétion

- **Requête après le curseur** : compléter `_cat/ind` dans `GET _cat/ind?format=json` donne `_cat/indices?v&format=json` — les paramètres par défaut du candidat et votre requête sous un seul `?` — au lieu d'un second `?`. Votre requête est gardée telle que tapée, et une clé que vous fixez remplace le défaut de même clé.
- **`/` initial** : `GET /_cat/pl` ne complétait rien ; il donne désormais `GET /_cat/plugins?v`, en gardant le `/`. Une entrée `/_cat/...` d'un `endpoints.txt` est aussi reconnue comme commande `_cat`, et ses colonnes se complètent.
- **Une recherche au fil de la frappe qui ne trouve rien** : `Entrée` n'insère plus l'entrée restée sélectionnée par une recherche précédente ; elle ne fait rien tant que la recherche n'est pas corrigée (`Retour arrière`), annulée (`Echap`) ou remplacée par un choix explicite — flèches, `Tab`, `PgUp`/`PgDn`, `Home`/`End`, ou un clic sur une entrée.

### Développement

- `tools/testclusters.sh` fonctionne avec le `/bin/bash` 3.2 de macOS, et un test refuse les constructions Bash 4 qui le cassaient.

Merci à @efd6 pour la demande (#1) et à @amineamanzou pour les modifications (#3, #4, #6).

## 0.7.1 (bêta) — octobre 2026

### Corrections

- **Une ligne de requête incomplète n'exécute plus la requête du dessus** : `Ctrl+E` sur une ligne comme `GET`, en cours de saisie, envoyait la requête la plus proche au-dessus, un `DELETE` par exemple. Rien n'est envoyé désormais, et la barre de statut indique qu'aucune requête n'a été trouvée. Une ligne vide ou de commentaire entre deux requêtes exécute toujours celle du dessus.
- **Une nouvelle connexion n'échoue plus sous Linux à cause des dossiers de certificats** : `default_ca_dir` et `default_client_cert_dir` remplissaient les champs de certificat d'une nouvelle connexion avec un dossier, ensuite chargé comme un fichier — même en http, où ces champs sont masqués. Ils ne fixent plus que le point de départ du sélecteur de certificat.
- **Les fichiers TLS ne sont lus que s'ils servent** : le fichier CA seulement en https avec vérification, le certificat client seulement en https. Un chemin enregistré pour un cluster en http, ou sans vérification, ne bloque plus la connexion. La commande de `F9` suit la même règle.
- **Deux exports dans la même seconde ne s'écrasent plus** : le nom du fichier se termine désormais par un nombre aléatoire (`AAAAMMJJ-HHMMSS-<nombre>.json`).
- **`F7` pendant la récupération des colonnes `_cat`** : une réponse antérieure au rechargement ne remplace plus les nouvelles colonnes.

Merci à @fbaligand pour le signalement (#2) et à @amineamanzou pour les correctifs (#5, #7).

## 0.7 (bêta) — octobre 2026

**En une phrase** : TermDevTools atteint un cluster à travers un proxy, accepte les clés d'API telles que Kibana les donne ainsi que les jetons Bearer, et n'écrit plus rien à côté de son binaire.

**Pourquoi « bêta »** : comme la 0.6, cette version n'a encore été utilisée que par son auteur. Le passage par un proxy a été vérifié avec des proxys de test, pas avec un proxy d'entreprise réel. Les retours sont bienvenus.

### Nouveautés

- **Proxy HTTP et SOCKS5** : un cluster est joint à travers le proxy que désignent les variables d'environnement `HTTPS_PROXY` / `HTTP_PROXY`, sauf si `NO_PROXY` l'exclut. Pour un cluster en particulier, `proxy:` sur son entrée dans `config.yaml` l'emporte : une URL de proxy (`http://…`, ou `socks5://…` — ce qu'ouvre `ssh -D`), ou `none` pour une connexion directe. `F9` l'inscrit dans la commande curl quand c'est `config.yaml` qui le fixe.
- **Clé d'API sous sa forme `encoded`** : la clé telle que Kibana l'affiche à sa création se colle directement dans le champ du secret, sans identifiant. Celui-ci est tiré de la clé et enregistré. La saisie de l'identifiant et du secret fonctionne comme avant.
- **Jeton Bearer** : nouveau type d'authentification, pour un jeton de compte de service, un jeton d'accès ou un JWT. Comme tout secret, il est redemandé à chaque connexion et jamais enregistré.
- **Les exports vont dans votre dossier de configuration** : `Ctrl+S` sur le résultat écrit dans `~/.config/termdevtools/exports/`, et plus à côté du binaire. L'export fonctionne donc aussi dans une installation partagée ou en lecture seule, et chaque utilisateur a les siens. La barre de statut affiche toujours le chemin du fichier écrit.
- **Le rapport de plantage** (`crash-<date>.log`) est écrit au même endroit : le programme n'écrit plus rien dans le dossier de son binaire.
- **Messages de l'écran de connexion sur trois lignes** : une erreur longue n'est plus coupée après la première.

### Sécurité

- **Un proxy n'est jamais utilisé sans être affiché** : pendant la connexion, en tête du message si elle échoue, et dans la barre de statut une fois connecté. Ses identifiants, eux, ne sont jamais affichés ni copiés, et ne sont envoyés qu'à lui.
- **Un proxy joint en TLS (`https://proxy…`) est refusé** : les réglages TLS du cluster — son certificat client, une vérification désactivée — s'appliqueraient aussi au proxy.
- **Rien de secret dans `config.yaml`**, comme avant : des identifiants dans `proxy:` sont refusés, et un jeton Bearer n'est jamais enregistré.
- **Un secret de clé d'API sans identifiant est refusé avant tout envoi**, avec un message qui dit quoi saisir.
- **Binaires compilés avec Go 1.27.2.** Cette version de Go corrige des failles de sa bibliothèque standard publiées depuis la 0.6, dont deux concernent un client comme celui-ci : la désynchronisation d'une connexion HTTP/1 après le refus d'un tunnel `CONNECT` par un proxy (GO-2026-6605) et le contournement d'une limite mémoire à l'analyse d'en-têtes (GO-2026-6608). `go.mod` demande désormais cette version.

### À savoir avant de mettre à jour depuis la 0.6

- **Proxy** : la 0.6 ignorait `HTTPS_PROXY` et `HTTP_PROXY`. Si l'une d'elles est définie sur votre poste, un cluster joint en direct jusqu'ici passera par ce proxy. S'il ne le doit pas, ajoutez-le à `NO_PROXY`, ou mettez `proxy: none` sur son entrée dans `config.yaml`. En cas d'échec, le message commence par le proxy emprunté et rappelle ces deux remèdes.
- **Exports** : un dossier `exports/` resté à côté du binaire n'est ni déplacé ni supprimé ; récupérez-y vos fichiers si vous en avez besoin.
- **Clés d'API** : rien à faire. Les entrées existantes gardent leur identifiant ; seul le libellé du champ du secret change.

### Vérifications

- Mot de passe, clé d'API (sous ses deux formes) et jeton Bearer : vérifiés sur un Elasticsearch 9.5.4 réel dont la sécurité est activée — acceptés avec le bon secret, refusés avec un mauvais.
- Proxy : vérifié avec un proxy HTTP et un serveur SOCKS5 de test — tunnel vers un cluster en https avec certificat vérifié de bout en bout, identifiants de chacun gardés pour lui seul.
- `govulncheck` : aucune vulnérabilité connue, ni dans les dépendances ni dans la bibliothèque standard de Go 1.27.2. Avec Go 1.27.1, il en signalait huit dans la bibliothèque standard.
- Les données de référence (recettes, endpoints, colonnes `_cat`) n'ont pas changé depuis la 0.6 : leurs vérifications sur onze clusters réels restent celles de la 0.6 et n'ont pas été refaites.
- Aucun audit externe indépendant n'a été réalisé.

### Limites connues

- OpenSearch managé par AWS avec authentification IAM (SigV4) n'est pas pris en charge.
- Proxy : ses identifiants ne se donnent que par la variable d'environnement, en authentification Basic ; NTLM, Kerberos, les fichiers PAC et les réglages proxy de Windows ne sont pas pris en charge. Aucun proxy d'entreprise réel n'a été essayé.
- Jeton Bearer : seul un jeton de compte de service d'Elasticsearch a été essayé ; les JWT d'Elasticsearch et d'OpenSearch ne l'ont pas été. Le programme ne renouvelle pas un jeton expiré.
- L'animation de démonstration du README a été enregistrée avec la 0.5 : elle ne montre ni le catalogue de recettes, ni ces nouveautés.

## 0.6 (bêta) — octobre 2026

**En une phrase** : TermDevTools n'a plus besoin que de son binaire, reconnaît Elasticsearch et OpenSearch ainsi que leur version, et propose un catalogue de requêtes prêtes à l'emploi adapté au cluster.

**Pourquoi « bêta »** : le catalogue de recettes, la prise en charge d'OpenSearch et la sélection par version sont nouveaux. Ils ont été vérifiés sur onze clusters réels (voir plus bas), mais pas encore par d'autres utilisateurs que leur auteur. Les retours sont bienvenus.

### Nouveautés

- **Catalogue de recettes (`F8`)** : une centaine de requêtes prêtes à l'emploi, classées par thème — vue d'ensemble, shards et allocation, nœuds et ressources, index, tâches, snapshots, cycle de vie des index, montée de version et maintenance, recherche. On filtre en tapant, on prévisualise, `Entrée` insère la recette à la fin de l'éditeur, curseur sur sa requête. Les recettes sont rédigées en anglais.
- **Elasticsearch et OpenSearch, selon la version** : la distribution et la version du cluster sont détectées à la connexion et affichées dans la barre de statut (`ES 9.5.4`, `OS 2.19.6`). Seuls les recettes et les endpoints qui existent sur ce cluster sont proposés. Couverture : Elasticsearch de la 7.17 à la 9.x, OpenSearch 2.x et 3.x auto-hébergé.
- **Plus aucun fichier à installer à côté du binaire** : recettes, endpoints et colonnes `_cat` y sont intégrés. Les releases sont de simples binaires, accompagnés de leurs sommes SHA-256.
- **Vos propres recettes et endpoints** : de simples fichiers texte dans `~/.config/termdevtools/` (ou à côté du binaire, pour les partager), qui s'ajoutent à ceux du binaire sans les remplacer. `F7` les recharge ; une erreur dans un fichier est signalée avec le fichier et la ligne.
- **Colonnes `_cat` demandées au cluster** : la complétion des paramètres `h=` et `s=` interroge le cluster lui-même, elle est donc exacte quelle que soit sa version.
- **Clés client chiffrées au format PKCS#8** (mTLS) : le format qu'OpenSSL écrit par défaut est maintenant lu, en plus de l'ancien format PEM chiffré.
- **`termdevtools --version`** et **`termdevtools --export-defaults <dossier>`** (écrit les recettes et endpoints intégrés sous forme de fichiers, pour les lire ou s'en inspirer).

### Sécurité et fiabilité

- **Les redirections HTTP ne sont plus suivies.** Une redirection est affichée telle quelle. Auparavant, une réponse `302` transformait sans rien dire un `DELETE` ou un `POST` en `GET` de la nouvelle adresse.
- **Une URL contenant des identifiants est refusée** (`https://utilisateur:motdepasse@hôte`) : l'URL est enregistrée dans `config.yaml`, le mot de passe s'y serait retrouvé.
- **`Ctrl+C` sauvegarde toujours avant de quitter**, y compris quand l'aide, la recherche ou une liste est ouverte. Si la sauvegarde échoue, le programme le dit et attend un second `Ctrl+C`.
- **Sauvegardes atomiques** : un disque plein ou une coupure pendant l'écriture ne détruit plus le contenu précédent (requêtes, `config.yaml`).
- **`config.yaml` garde ses commentaires** après chaque connexion, ainsi que les clés que la version ne connaît pas et un dossier par défaut volontairement vide.
- **Une réponse de plus de 64 Mo n'est plus chargée** : la requête est signalée en échec, avec le conseil de la restreindre.
- **Corrections** : plantage de la recherche dans le résultat après un résultat plus court ; une connexion lente abandonnée ne peut plus remplacer la session ouverte entre-temps ; seule la réponse à la dernière requête envoyée est affichée ; avec la souris activée, le focus suit les clics.

### À savoir avant de mettre à jour depuis la 0.5

- **Fichiers annexes** : `cat_columns.txt` n'est plus lu et peut être supprimé. `endpoints.txt`, s'il est resté à côté du binaire, est lu comme un *complément* de la liste intégrée : supprimez-le sauf si vous y aviez ajouté vos propres endpoints. `cheatsheet.txt` garde son rôle ; supprimez-le pour obtenir le contenu de départ intégré.
- **Contenu de départ de l'éditeur** : quelques requêtes universelles au lieu d'une longue cheatsheet, dont le contenu se trouve maintenant dans le catalogue (`F8`). Vos requêtes déjà sauvegardées ne changent pas.
- **Adresse en `http://` qui redirigeait vers `https://`** : la connexion échoue désormais en indiquant la nouvelle adresse ; corrigez l'URL.
- **Identifiants dans l'URL** : une entrée de ce type dans `config.yaml` est refusée à la connexion ; retirez-les de l'URL et choisissez Basic Auth.
- **`F7`** recharge maintenant tous vos fichiers (variables, recettes, endpoints), plus seulement les variables.
- **Interface en anglais par défaut** : le programme démarre en anglais tant qu'aucune langue n'a été choisie ; `F3` passe au français et retient ce choix. Cela vaut aussi pour une installation existante où `F3` n'a jamais servi (la 0.5 n'enregistrait pas durablement la langue tant qu'elle n'avait pas été changée) : appuyez une fois sur `F3`. Une langue déjà choisie avec `F3` est conservée.
- **Fichiers générés en anglais** : les commentaires d'un nouveau `config.yaml` et d'un nouveau fichier de variables sont en anglais. Les fichiers existants ne sont pas réécrits.

### Vérifications

- Détection, endpoints, colonnes `_cat` et **chaque requête de chaque recette** exécutés sur onze clusters réels : Elasticsearch 7.17.29, 8.0.1, 8.11.4, 8.19.22, 9.0.8, 9.5.4 ; OpenSearch 2.0.1, 2.11.1, 2.19.6, 3.0.0, 3.9.0.
- Clés PKCS#8 : vérifiées sur des clés écrites par OpenSSL et par une connexion mTLS complète.
- `govulncheck` : aucune vulnérabilité connue dans les dépendances.

### Limites connues

- OpenSearch managé par AWS avec authentification IAM (SigV4) n'est pas pris en charge.
- Pas de proxy HTTP(S) : `HTTPS_PROXY` n'est pas pris en compte.
- Les clés d'API se saisissent sous la forme identifiant + secret, pas sous la forme `encoded`.
- Les exports (`Ctrl+S` sur le résultat) sont écrits à côté du binaire : dans une installation partagée ou en lecture seule, l'export échoue.
- L'animation de démonstration du README a été enregistrée avec la 0.5 : elle ne montre pas le catalogue de recettes.

## 0.5 — 24 août 2026

- Variables réutilisables `${nom}`, par cluster, rechargées avec `F7`.
- Numéros de ligne dans l'éditeur, fermeture automatique des accolades, crochets et guillemets.
- Sélecteur de fichier pour les certificats de l'écran de connexion.

## 0.4 — 14 août 2026

- `F10` comme alternative à `Tab` pour la complétion, sur les terminaux qui interceptent `Tab`.
- Souris désactivée par défaut.
- Rapport de plantage écrit dans un fichier.
- Complétion améliorée, rappel de la requête en tête du résultat.
- Correction d'un décalage dans la recherche, la complétion et le ciblage de la requête après des caractères accentués.

## 0.3 — 13 août 2026

- Scripts d'installation (`install.sh`, `install.ps1`).
- Sous macOS, `Option`/`Alt` accepté à la place de `Ctrl` pour trois raccourcis.

## 0.2 — 13 août 2026

- Interface en français ou en anglais, `F3` pour changer de langue.

## 0.1 — 12 août 2026

- Première version publiée : éditeur de requêtes, exécution de la requête sous le curseur, résultat JSON mis en forme.
