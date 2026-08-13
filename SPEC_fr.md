*(English version: [SPEC.md](SPEC.md))*

# TermDevTools — Spécification du projet

> Simulateur en mode terminal de la vue "DevTools" de Kibana, pour soumettre des requêtes à un cluster Elasticsearch sans passer par un navigateur.

Statut : finalisé pour la v1 — en attente de relecture finale avant lancement de l'implémentation.

---

## 1. Contexte et objectif

- **Problème résolu** : Nous avons parfois des cas où un cluster Elastic n'a pas de Kibana ou son Kibana est non fonctionnel. Pour faciliter les investigations, avoir un équivalent de Dev Tools directement dans le terminal est bien plus efficient que faire quelques commandes curl (avec les soucis de ssl, de requêtes API complexes, …)
- **Utilisateurs cibles** : l'équipe en charge du run des clusters Elasticsearch.
- **Environnements cibles** : RHEL 8, RHEL 9, RHEL 10 (terminal only, pas de GUI)
- **Contrainte de portabilité** : binaire unique, sans dépendance système au-delà de la libc de base

## 2. Choix technique

- **Langage retenu** : **Go**
- **Bibliothèque TUI retenue** : [`tview`](https://github.com/rivo/tview) (widgets prêts à l'emploi : `TextArea` pour l'éditeur, `TextView` pour le JSON, `Flex`/`Grid` pour le layout, `SetInputCapture` pour les raccourcis globaux), basé sur [`tcell`](https://github.com/gdamore/tcell). Choisi plutôt que `bubbletea` pour sa simplicité de développement sur ce cas d'usage (layout classique à widgets, pas de rendu custom complexe).
- **Bibliothèque client HTTP/JSON** : stdlib Go (`net/http` + `encoding/json`), pas de dépendance externe nécessaire a priori
- **Méthode de compilation/distribution** : binaire statique (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build`), pas de dépendance à la libc du système → portable tel quel entre RHEL 8/9/10, et intégrable facilement dans un outil de déploiement/configuration management existant.

## 3. Interface utilisateur (TUI)

### 3.0 Connexion

Au lancement, un écran de connexion liste les URLs des clusters connus (pas de nom séparé : l'URL est déjà l'identifiant le plus explicite), triées du plus récemment utilisé au moins récent (ordre de la liste `clusters` dans `config.yaml`, propre à l'utilisateur courant — cf. §9.1 et §9.2), et propose toujours en plus une option **"Nouvelle connexion"**.

- Sélection d'un cluster existant : les champs non sensibles (type d'auth, chemins CA/cert, username, API key ID) sont pré-remplis depuis `config.yaml` ; seul le secret (mot de passe, API key secret, passphrase de clé privée) est redemandé selon le type d'auth.
- **Nouvelle connexion** : formulaire interactif complet à saisir — URL, type d'authentification (aucune / Basic Auth / API Key / certificat client mTLS), puis selon le type : username, API key ID, chemin du CA (pré-rempli avec `default_ca_dir`), chemins cert/clé client (pré-remplis avec `default_client_cert_dir`), activation ou non de la vérification TLS — et enfin le(s) secret(s) correspondant(s).
- **Champs affichés dynamiquement**, pour ne montrer que ce qui est pertinent :
  - URL en `http://` (non https) → champs TLS masqués (fichier CA, case "vérifier le certificat", et les champs certificat/clé client puisqu'un certificat client fait partie de la poignée de main TLS)
  - Authentification "aucune" → aucun champ d'auth affiché
  - "Basic Auth" → uniquement username/mot de passe
  - "certificat client (mTLS)" → uniquement les champs certificat (masque username/mot de passe) ; masqués eux aussi si l'URL est en `http://` (cf. ci-dessus)
- **Champ actif mis en évidence** : le champ ayant le focus est affiché en couleurs inversées (fond/texte), pour rester repérable même quand le seul curseur clignotant ne suffit pas.
- Dans les deux cas, une fois la connexion réussie, l'entrée (nouvelle ou existante) est déplacée/insérée en **première position** de la liste `clusters` dans `config.yaml` — aucun secret n'y est jamais écrit.
- Une fois la connexion effectuée, on ne travaille que sur un seul cluster Elasticsearch jusqu'à déconnexion (= fermeture du programme, cf. §4), au sein d'un layout général inspiré des Dev Tools de Kibana.

### 3.1 Layout général

- Écran divisé en deux panneaux verticaux, largeur relative ajustable en cours de session via `Ctrl+Maj+←/→` (cf. §4) :
  - **Panneau gauche** : éditeur de requêtes (texte libre, une ou plusieurs lignes, ex. `GET _cat/indices`). On chargera un contenu par défaut présent dans un fichier `cheatsheet.txt` (même dossier que le binaire) s'il existe.
  - **Panneau droit** : résultat JSON de la dernière requête exécutée (donc vide au démarrage).
  - **Retour à la ligne automatique** dans les deux panneaux (une ligne trop longue pour la largeur affichable est repliée sur l'écran, purement visuel) — n'affecte ni le texte réel de l'éditeur (donc pas le parsing des requêtes, cf. §3.2), ni le contenu exporté/copié depuis le résultat (cf. §3.3). Point d'implémentation à retenir : `TextArea.GetCursor()` et `TextView.ScrollTo()` raisonnent tous deux en **ligne affichée** (post-retour à la ligne), pas en ligne logique, dès que le wrap est actif — s'y fier directement aurait cassé le ciblage de requête (`Ctrl+Entrée`), la complétion (`Tab`) et le défilement de recherche (`Ctrl+F` à droite). Contournés respectivement via `TextArea.GetSelection()` (position en décalage absolu dans le texte, indépendante de l'affichage) et le mécanisme de régions de tview (`Highlight`/`ScrollToHighlight`, ancré au contenu plutôt qu'à un numéro de ligne).
- **Barre de statut** : cluster connecté, utilisateur courant, statut "requête en cours..." pendant l'appel, puis code HTTP + temps de réponse une fois la requête terminée. Le chrono affiché en direct pendant l'attente est repoussé en v2 (cf. §7) pour ne pas complexifier la première version.
- **Écran d'aide (`F1`)** : popup superposé au layout (centré, hauteur proportionnelle au terminal, scrollable si le contenu déborde), rappelant le fonctionnement des deux panneaux, la liste des raccourcis (§4) et l'emplacement des fichiers de configuration/templates (`config.yaml`, `queries_*.txt`, `cheatsheet.txt`, `endpoints.txt`, `cat_columns.txt`, `exports/` — cf. §9.1). Se ferme avec `Echap`, sans effet sur le contenu de l'éditeur ni du résultat.

### 3.2 Éditeur (panneau gauche)

- Composant tview : `TextArea` (multi-ligne, gère nativement le curseur/la sélection)
- Contenu : Texte contenant une ou plusieurs requêtes API (commençant par GET, PUT, POST ou DELETE avec ensuite l'endpoint et les paramètres, et sur les lignes suivantes, le JSON de payload à transmettre). L'éditeur détecte la fin du JSON sous une requête (équilibrage des accolades) pour comprendre la séparation avec la requête suivante. Toute ligne commençant par `#` est un commentaire, donc ignorée.
- Exécution : `Ctrl+Entrée` exécute la requête où se trouve le curseur, **uniquement si le focus est sur le panneau gauche** (sans effet si le focus est à droite, cf. §4). L'appel est lancé en asynchrone ; la status bar passe à "requête en cours...", puis le panneau droit et la status bar sont mis à jour une fois la réponse obtenue.
- Fichier par défaut : `cheatsheet.txt`, chargé au démarrage s'il existe (même dossier que le binaire).
- **Sauvegarde par cluster** : la sauvegarde du contenu de l'éditeur est propre au **cluster auquel on est connecté** (identifié par son URL) **et à l'utilisateur courant** — un fichier `~/.config/termdevtools/queries_<URL assainie>.txt` par cluster déjà utilisé par cet utilisateur (à côté de `config.yaml`, cf. §9.1 pour le détail de l'assainissement du nom de fichier).
- **Déclenchement de la sauvegarde** :
  - `Ctrl+S` (focus panneau gauche) : sauvegarde explicite, avec confirmation dans la status bar.
  - **Automatique en sortie de programme** : le contenu est sauvegardé sans action explicite à la fermeture (`Ctrl+C`, ou signal externe `SIGTERM`/`SIGHUP` — ex. session SSH coupée), en plus du `Ctrl+S` explicite. Best-effort, silencieux (pas de confirmation possible à ce stade). `SIGKILL` reste, comme pour tout programme, impossible à intercepter.
- **Chargement au démarrage**, une fois la connexion à un cluster établie : si une sauvegarde existe déjà **pour ce cluster (URL) et cet utilisateur**, elle est chargée en priorité ; sinon, `cheatsheet.txt` sert de contenu par défaut.
- **Coloration syntaxique : écartée, pas seulement différée** — `tview.TextArea` (seul widget de la bibliothèque permettant l'édition multi-ligne : curseur, sélection, undo, presse-papiers) ne supporte explicitement pas le texte multi-couleur (documentation officielle : *"Multi-color text is not supported"*), contrairement à `TextView` utilisé en lecture seule à droite (§3.3). L'obtenir nécessiterait de reconstruire un éditeur maison au-dessus d'un `TextView` colorable (curseur/sélection/édition réimplémentés à la main) — jugé disproportionné pour un outil interne v1. Vérifié qu'aucune version plus récente de tview ne lève cette limite (v0.42.0 = dernière version au 2026-08-12).
- **Auto-complétion (`Tab`, focus panneau gauche)** : proposée uniquement quand le curseur est en train de taper une ligne `MÉTHODE endpoint_partiel` (pas dans un corps JSON ni ailleurs — `Tab` y garde son comportement standard d'insertion d'une tabulation). Comparaison insensible à la casse du préfixe tapé contre une **liste d'endpoints connus**, centrée sur l'administration/exploitation (`_cat/*`, `_cluster/*`, `_nodes/*`, endpoints d'admin d'index...) — pas de découverte dynamique des noms d'index réels (idée notée pour une version future, cf. §7).
  - 0 correspondance → message en status bar, rien d'autre.
  - 1 correspondance → complétion directe, sans interaction supplémentaire.
  - Plusieurs correspondances → liste déroulante à choisir (flèches puis Entrée pour valider, Echap pour annuler) ; un `Tab` supplémentaire pendant que la liste est ouverte fait défiler les suggestions.
  - **`/` final optionnel** : en HTTP, un `/` en toute fin de chemin avant les paramètres est facultatif (`_cat/indices/?h=...` équivaut à `_cat/indices?h=...`). Aucun endpoint connu n'en stocke un, donc il est ignoré pour la comparaison — la complétion remplace tout le segment tapé (le `/` avec), pas seulement ce qui le précède. Ce cas ne se pose pas pour les colonnes `h=`/`s=` ci-dessous : la reconnaissance de commande `_cat` (au préfixe le plus long, à une frontière `/`) l'absorbe déjà naturellement.
  - **Source de la liste** : `endpoints.txt` (un endpoint par ligne, `#` = commentaire) à côté du binaire s'il existe (§9.1) — remplace alors entièrement la liste par défaut intégrée au binaire. Permet à l'équipe d'ajuster la liste à sa version d'Elasticsearch sans recompiler.
  - **Liste par défaut** : extraite de la spec OpenAPI officielle ([elastic/elasticsearch-specification](https://github.com/elastic/elasticsearch-specification), branche `9.5`), filtrée aux endpoints sans paramètre de chemin (les `/{index}/...` sont hors périmètre, cf. ci-dessus) et aux domaines admin de base — `_cat` (toutes les commandes, `?v` systématique pour les en-têtes de colonnes), `_cluster`, `_nodes`, index/recherche/snapshot/ILM/SLM/licence. Volontairement écartés : ML, sécurité, watcher, transform, rollup, SQL/ES\|QL, CCR, connectors, inference, enrich. À régénérer depuis une branche plus récente du même dépôt quand la version cible d'Elasticsearch change significativement.
- **Colonnes `h=`/`s=` des commandes `_cat/*`** : cas particulier de l'auto-complétion ci-dessus, prioritaire sur la complétion d'endpoint générique. Reconnu quand le curseur est en train de taper le paramètre `h=` (colonnes affichées) ou `s=` (tri) d'une commande `_cat/xxx` déjà identifiée (ex. `_cat/indices?h=health,st`) :
  - seule la dernière colonne tapée (après la dernière virgule) est complétée, ce qui précède est préservé tel quel ;
  - pour `s=`, si la colonne est déjà suivie de `:`, complète la direction de tri (`asc`/`desc`) plutôt qu'un nom de colonne (ex. `s=docs.count:de` → `desc`) ; ce cas ne s'applique pas à `h=`, où un `:` fait partie du texte comparé tel quel ;
  - **filtre en fin de chemin** : de nombreuses commandes `_cat` acceptent un filtre (nom d'index, de nœud...) entre la commande et les paramètres, ex. `_cat/shards/monindex?h=...`. La commande est reconnue comme la plus longue entrée de la table `commande → colonnes` qui préfixe le chemin à une frontière `/` (jamais une correspondance partielle de mot : `shardsxyz` ne matche pas `shards`) — sans quoi `shards/monindex` ne correspondrait à aucune commande connue et rien ne serait proposé ;
  - la liste de colonnes proposée dépend de la commande `_cat` en cours (ex. les colonnes de `_cat/shards` diffèrent de celles de `_cat/indices`) — table `commande → colonnes` distincte de la liste plate des endpoints, avec le même principe de source (§9.1) : fichier `cat_columns.txt` à côté du binaire s'il existe, sinon table par défaut intégrée au binaire, générée le 2026-08-12 à partir de `GET _cat/<commande>?help` interrogé sur un cluster Elasticsearch 9.5.0 réel. **Seuls les noms complets de colonne sont retenus** (ex. `docs.count`), pas leurs alias courts (`dc`) : plus parlants, et ça limite le nombre de propositions pour les commandes qui ont beaucoup de colonnes (`_cat/indices`, `_cat/nodes`, `_cat/shards`...) — les alias restent ajoutables à la main dans `cat_columns.txt` pour qui en a l'usage.

### 3.3 Résultat (panneau droit)

- Format d'affichage : JSON prettifié (réponses usuelles) ou texte casse fixe (réponse à une commande _cat par exemple)
- Coloration syntaxique si JSON : oui en v1
- Historique des résultats : Non
- Gestion des réponses volumineuses : Scroll manuel avec touches haut/bas
- Affichage des erreurs (requête invalide, cluster injoignable, timeout) : dans la status bar
- **Export (`Ctrl+S`, focus panneau droit)** : écrit le résultat actuellement affiché dans le sous-dossier `exports/` du binaire (créé si besoin), nom de fichier horodaté (`AAAAMMJJ-HHMMSS`), extension `.json` si c'est du JSON valide, `.txt` sinon. Confirmation (avec chemin) affichée dans la status bar ; erreur (ex. rien à exporter) affichée de la même façon.
- **Copie dans le presse-papier (`F2`)** : `tview.Application.EnableMouse(true)` empêche la sélection de texte native du terminal (l'appli capte les événements souris) — pas de sélection possible à la souris dans ce panneau. `F2` copie donc l'intégralité du résultat affiché, via le mécanisme terminal standard **OSC 52** (`tcell.Screen.SetClipboard`) : le terminal local reçoit une séquence d'échappement lui demandant de copier vers *son propre* presse-papier, ce qui fonctionne même à travers SSH (le presse-papier n'est jamais celui du serveur distant). Confirmation affichée dans la status bar, mais **sans garantie que la copie a réellement eu lieu** : ni tcell ni le protocole OSC 52 ne renvoient de confirmation, et le support dépend du terminal (fonctionne sur la plupart des terminaux modernes — Windows Terminal, iTerm2, GNOME Terminal/VTE récent... — mais pas sur PuTTY nu, ni dans tmux/screen sans configuration de passthrough particulière). À vérifier en usage réel.

## 4. Raccourcis clavier

Mettre une barre d'aide sous la barre d'état avec rappel des raccourcis.
| Action | Touche | Statut |
|---|---|---|
| Exécuter la requête sous le curseur | `Ctrl+Entrée` | Défini |
| Basculer focus panneau gauche ↔ droit | `Ctrl+←`/`Ctrl+→` | Défini |
| Quitter l'application (sauvegarde automatiquement le panneau gauche, §3.2) | `Ctrl+C` | Défini |
| Nouvelle requête / effacer l'éditeur | Edition libre du texte pannel gauche | Défini |
| Ouvrir/changer la connexion au cluster | On quitte le programme et on relance | Défini |
| Rechercher dans les requêtes | `Ctrl+F` dans pannel gauche | Défini |
| Rechercher dans le résultat JSON | `Ctrl+F` dans pannel droit | Défini |
| Redimensionner le split gauche/droite | `Ctrl+Maj+←` (rétrécir la gauche) / `Ctrl+Maj+→` (l'agrandir) | Défini |
| Sauvegarder (gauche) / exporter (droite) | `Ctrl+S`, comportement contextuel selon le panneau focus (§3.2, §3.3) | Défini |
| Compléter un endpoint en cours de frappe | `Tab` dans pannel gauche, sur une ligne `MÉTHODE endpoint` (§3.2) | Défini |
| Afficher l'aide (fonctionnement + raccourcis) | `F1`, `Echap` pour fermer | Défini |
| Copier le résultat dans le presse-papier | `F2` (§3.3) | Défini |

> `Ctrl+Entrée` n'est actif que lorsque le focus est sur le panneau gauche (édition des requêtes) — sans effet depuis le panneau droit.
>
> **Historique des essais** : `Ctrl+Echap` (quitter) et `Ctrl++`/`Ctrl+-` (redimensionner) prévus initialement se sont révélés interceptés en pratique — `Ctrl+Echap` par Windows lui-même (raccourci système sur les postes de l'équipe), `Ctrl++`/`Ctrl+-` par le terminal (zoom de police, notamment sous Windows Terminal). Retenus à la place : `Ctrl+C` (seul raccourci de sortie, universellement disponible au niveau terminal) et `Ctrl+Maj+←/→` pour le split. `Ctrl+H`, envisagé pour l'aide, a été écarté au profit de `F1` : `Ctrl+H` correspond au code de contrôle historique `0x08` (BS), documenté par tview comme raccourci alternatif de Backspace dans `TextArea`/`InputField` — l'utiliser globalement aurait cassé l'effacement de caractère via `Ctrl+H` dans l'éditeur et le formulaire de connexion. Cela reste à confirmer sur l'ensemble des terminaux/postes de l'équipe — en cas de nouveau blocage, prévoir une bascule vers d'autres touches de fonction (F5...), qui échappent à ce type de conflit. Même logique pour la copie du résultat : `Ctrl+Maj+C` (convention "copier" de nombreux terminaux modernes, choisie pour ne pas entrer en collision avec `Ctrl+C` déjà pris par Quitter) a été écarté — c'est justement le risque : cette combinaison est elle-même fréquemment interceptée par le terminal *avant* d'atteindre l'appli. `F2` retenu à la place.
>
> `Ctrl+S` est un code de contrôle classique (comme `Ctrl+F`), donc a priori fiable partout — la seule réserve historique est le contrôle de flux XON/XOFF de certains terminaux (`Ctrl+S` fige la sortie jusqu'à `Ctrl+Q`), normalement désactivé par le mode raw que le programme active. À signaler si un blocage de ce type est malgré tout observé.

## 5. Connexion au cluster Elasticsearch

- cf. §3.0 pour le flux et §9.2 pour le schéma de `config.yaml`
- **Authentification supportée** : aucune, Basic Auth (login/password), API Key, certificat client (mTLS)
- **TLS** : vérification de certificat (CA situé par défaut dans `default_ca_dir`, chemin surchargeable par connexion), option pour l'ignorer
- **Certificats** : deux dossiers par défaut configurables globalement (`default_ca_dir`, `default_client_cert_dir`) pour pré-remplir les chemins lors de la saisie d'une nouvelle connexion
- **Stockage des secrets** : aucun — mot de passe, API key secret et passphrase de clé privée sont redemandés à chaque connexion ; seuls les éléments non sensibles (URL, type d'auth, username, API key ID, chemins CA/cert) sont persistés dans `config.yaml`, avec l'entrée la plus récemment utilisée en tête de liste

## 6. Requêtes supportées

- **Syntaxe d'entrée** : format libre façon Kibana Console (`METHODE chemin` + corps JSON optionnel sur les lignes suivantes)
- **Méthodes HTTP à supporter** : GET, POST, PUT, DELETE
- **Validation avant envoi** : vérifier que le JSON du corps est valide avant d'exécuter
- **Timeout par défaut** : 2 minutes, paramétrable via `default_timeout_seconds` dans `config.yaml` (§9.2)

## 7. Hors scope v1 (backlog futur)

- Chrono de la requête en cours mis à jour en direct dans la status bar (v1 affiche uniquement le résultat final : code HTTP + durée totale une fois la réponse reçue)
- Auto-complétion dynamique des noms d'index réels du cluster connecté (v1 se limite à une liste statique d'endpoints connus, cf. §3.2)
- Coloration syntaxique avancée

## 8. Contraintes non-fonctionnelles

- **Dépendances runtime** : aucune au-delà de la libc standard présente sur RHEL 8/9/10
- **Performance** : Doit supporter de gros résultats (plusieurs Mo). 
- **Packaging final envisagé** : simple binaire à copier
- **Nom du projet / binaire** : termdevtools

## 9. Architecture technique proposée

### 9.1 Emplacement des fichiers

L'historique de connexions est propre à l'utilisateur (deux personnes lançant le même binaire partagé sur un même serveur ne doivent pas se marcher dessus), alors que la cheatsheet est plutôt un contenu d'équipe attaché à l'installation. Deux emplacements distincts, donc :

- **Dossier de configuration utilisateur** (`~/.config/termdevtools/`, ou `$XDG_CONFIG_HOME/termdevtools/` si cette variable est définie), créé automatiquement (permissions `0700`) dès la première écriture :
  - `config.yaml` — clusters connus, mis à jour automatiquement à chaque connexion réussie, aucun secret dedans (§9.2)
  - `queries_<URL assainie>.txt` — un fichier par cluster déjà utilisé par cet utilisateur, contenant la dernière sauvegarde du panneau gauche pour ce cluster (§3.2). Écrit par `Ctrl+S` et automatiquement en sortie de programme. Nom construit à partir de l'URL du cluster, en remplaçant par `_` tout caractère qui n'est ni alphanumérique, ni `.`, `_` ou `-` (donc notamment `:` et `/`) — ex. `https://es-prod.example.com:9200` → `queries_https___es-prod.example.com_9200.txt`. Deux URL différentes qui se ressembleraient au point de produire le même nom après cette normalisation partageraient (cas rare) le même fichier — limite acceptée pour garder des noms lisibles plutôt que hachés.
- **Dossier de l'exécutable** (celui du binaire, pas le répertoire courant du shell) :
  - `cheatsheet.txt` — contenu par défaut de l'éditeur, chargé au démarrage seulement si aucune sauvegarde `queries_*.txt` n'existe encore pour le cluster/utilisateur courant (optionnel, §3.2)
  - `endpoints.txt` — liste d'endpoints proposée par l'auto-complétion `Tab`, remplace la liste par défaut intégrée au binaire si présent (§3.2). **Livré coché dans le dépôt** (contrairement à `cheatsheet.txt`/`config.yaml`, qui restent de simples gabarits `.example`) : évoluant peu, on le traite comme une entrée standard du projet plutôt que comme un template à copier. Reste néanmoins facultatif à l'exécution — sans lui (ex. déploiement où seul le binaire est copié), la liste par défaut intégrée au binaire prend le relais. Permet d'ajuster la liste à la version d'Elasticsearch de l'équipe sans recompiler ; à régénérer depuis la spec OpenAPI (§3.2) si elle diverge trop d'une future version.
  - `cat_columns.txt` — colonnes `h=`/`s=` proposées par l'auto-complétion pour chaque commande `_cat/*` (§3.2), même traitement que `endpoints.txt` (livré coché dans le dépôt, remplace la table par défaut intégrée au binaire si présent, facultatif à l'exécution). Format en sections `# _cat/commande` suivies d'une colonne par ligne. Généré le 2026-08-12 depuis `GET _cat/<commande>?help` sur un cluster Elasticsearch 9.5.0 réel — à régénérer de la même façon si les colonnes divergent trop d'une future version.
  - `exports/` — résultats exportés par `Ctrl+S` depuis le panneau droit, un fichier horodaté par export (créé à la demande, §3.3)

### 9.2 Schéma `config.yaml` (`~/.config/termdevtools/config.yaml`)

```yaml
default_timeout_seconds: 120
language: fr  # langue de l'interface : "fr" (défaut) ou "en" — voir le package i18n
default_ca_dir: /etc/pki/termdevtools/ca              # pré-remplissage du champ CA pour une nouvelle connexion
default_client_cert_dir: /etc/pki/termdevtools/certs  # pré-remplissage des champs cert/clé client (mTLS)

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
```

### 9.3 Structure de projet Go suggérée

```
termdevtools/
├── main.go                 // point d'entrée : charge config, lance écran de connexion, puis l'UI
├── go.mod
├── config/
│   └── config.go           // lecture/écriture config.yaml, déplacement de l'entrée utilisée en tête de liste
├── i18n/
│   └── i18n.go             // tables de messages fr/en pour l'interface, sélectionnées via config.Language
├── esclient/
│   └── client.go           // client HTTP (auth none/basic/api_key/mtls, TLS), exécution d'une requête
├── parser/
│   └── parser.go           // découpe le contenu de l'éditeur en requêtes (méthode, endpoint, payload, commentaires)
├── ui/
│   ├── connect.go          // écran de connexion initial (tview.Form)
│   ├── app.go              // assemblage Flex, gestion du focus, raccourcis globaux
│   ├── editor.go           // panneau gauche (TextArea)
│   ├── completion.go       // endpoints + colonnes _cat par défaut, chargement endpoints.txt/cat_columns.txt, détection h=/s=, filtrage par préfixe
│   ├── result.go           // panneau droit (TextView + coloration JSON)
│   └── statusbar.go        // barre de statut + barre d'aide raccourcis
├── cheatsheet.txt.example
├── endpoints.txt          // livré coché dans le dépôt (§9.1), pas un simple .example
├── cat_columns.txt        // idem, colonnes h=/s= par commande _cat
└── config.yaml.example
```

### 9.4 Flux d'exécution d'une requête

1. Focus sur le panneau gauche, curseur positionné sur une requête, `Ctrl+Entrée`.
2. `parser` extrait méthode + endpoint + payload JSON autour du curseur et valide le JSON.
3. JSON invalide → message d'erreur en status bar, rien n'est envoyé.
4. JSON valide → appel HTTP lancé dans une goroutine ; status bar → "requête en cours...".
5. Réponse reçue → `esclient` renvoie code HTTP, durée, corps ; mise à jour de l'UI via `QueueUpdateDraw` (thread-safe avec tview) : panneau droit rempli (JSON prettifié et coloré, ou texte brut pour les réponses `_cat`), status bar → code HTTP + durée.

## 10. Questions ouvertes

Aucune question bloquante identifiée à ce stade. Section réutilisable pour toute question qui émergerait pendant l'implémentation.

