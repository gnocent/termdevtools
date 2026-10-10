*(English version: [CHANGELOG.md](CHANGELOG.md))*

# Journal des versions

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
