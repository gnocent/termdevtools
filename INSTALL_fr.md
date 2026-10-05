*(English version: [INSTALL.md](INSTALL.md))*

# Installer et paramétrer TermDevTools

Ce guide va du téléchargement à la première requête, puis détaille chaque réglage. Pour une vue d'ensemble du produit, voir le [README](README_fr.md).

- [1. Ce qu'il faut](#1-ce-quil-faut)
- [2. Installer le binaire](#2-installer-le-binaire)
- [3. Première connexion](#3-première-connexion)
- [4. Premiers pas dans l'interface](#4-premiers-pas-dans-linterface)
- [5. Réglages : `config.yaml`](#5-réglages--configyaml)
- [6. Vos fichiers : requêtes, variables, recettes, endpoints](#6-vos-fichiers--requêtes-variables-recettes-endpoints)
- [7. Installation partagée](#7-installation-partagée)
- [8. Mettre à jour](#8-mettre-à-jour)
- [9. Désinstaller](#9-désinstaller)
- [10. En cas de problème](#10-en-cas-de-problème)

## 1. Ce qu'il faut

- **Un seul fichier** : le binaire `termdevtools`. Aucune dépendance, aucun fichier à poser à côté, aucun accès à Internet à l'exécution.
- **Un terminal d'au moins 80 colonnes sur 24 lignes.** Recommandés : Windows Terminal, le Terminal de macOS ou iTerm2, n'importe quel terminal Linux, y compris à travers SSH. PuTTY est déconseillé (plusieurs raccourcis y sont mal transmis).
- **Un accès réseau au cluster** depuis la machine où tourne TermDevTools, et un compte autorisé au minimum à lire la racine du cluster (`GET /`) : c'est la requête qui valide la connexion.

Clusters pris en charge : Elasticsearch de la 7.17 à la 9.x, OpenSearch 2.x et 3.x auto-hébergé.

## 2. Installer le binaire

Les binaires sont publiés sur la page [Releases](https://github.com/gnocent/termdevtools/releases) :

| Plateforme | Fichier |
|---|---|
| Linux (x86-64) | `termdevtools-linux-amd64` |
| Windows (x86-64) | `termdevtools-windows-amd64.exe` |
| macOS (Apple Silicon) | `termdevtools-darwin-arm64` |
| Sommes de contrôle | `SHA256SUMS` |

### 2.1 Linux

1. Téléchargez `termdevtools-linux-amd64` et `SHA256SUMS` dans le même dossier.
2. Vérifiez le fichier :
   ```bash
   sha256sum -c SHA256SUMS --ignore-missing
   ```
   La ligne `termdevtools-linux-amd64: OK` (ou `Réussi`) doit s'afficher.
3. Rendez-le exécutable et placez-le dans un dossier de votre `PATH` :
   ```bash
   chmod +x termdevtools-linux-amd64
   mkdir -p ~/.local/bin
   mv termdevtools-linux-amd64 ~/.local/bin/termdevtools
   ```
4. Vérifiez :
   ```bash
   termdevtools --version
   ```
   Si la commande est introuvable, `~/.local/bin` n'est pas dans votre `PATH` : ajoutez `export PATH="$HOME/.local/bin:$PATH"` à votre `~/.bashrc`, ou lancez le binaire par son chemin complet.

**Machine sans accès à Internet** : téléchargez les deux fichiers ailleurs, transférez-les (`scp`, support amovible), puis reprenez à l'étape 2. Rien d'autre n'est à transférer.

### 2.2 macOS (Apple Silicon)

1. Téléchargez `termdevtools-darwin-arm64` et `SHA256SUMS`.
2. Vérifiez le fichier : la commande ci-dessous affiche une empreinte, qui doit être identique à celle de la ligne `termdevtools-darwin-arm64` de `SHA256SUMS`.
   ```bash
   shasum -a 256 termdevtools-darwin-arm64
   ```
3. Rendez-le exécutable et placez-le dans votre `PATH` :
   ```bash
   chmod +x termdevtools-darwin-arm64
   mkdir -p ~/.local/bin
   mv termdevtools-darwin-arm64 ~/.local/bin/termdevtools
   ```
4. Le binaire n'est pas signé par un compte développeur Apple. Si macOS refuse de le lancer parce qu'il a été téléchargé, retirez la marque de quarantaine :
   ```bash
   xattr -d com.apple.quarantine ~/.local/bin/termdevtools
   ```
5. Vérifiez avec `termdevtools --version`.

Les Mac à processeur Intel ne sont pas fournis en binaire : compilez depuis les sources (§2.4).

### 2.3 Windows

1. Téléchargez `termdevtools-windows-amd64.exe` et `SHA256SUMS`.
2. Vérifiez le fichier dans PowerShell : l'empreinte affichée doit être celle de la ligne correspondante de `SHA256SUMS` (la casse des lettres n'a pas d'importance).
   ```powershell
   Get-FileHash .\termdevtools-windows-amd64.exe -Algorithm SHA256
   ```
3. Renommez-le `termdevtools.exe` et placez-le dans un dossier à vous, par exemple `%LOCALAPPDATA%\termdevtools\`.
4. Lancez-le depuis **Windows Terminal** (ou PowerShell) par son chemin, ou ajoutez ce dossier à votre `PATH`.
5. Le binaire n'est pas signé : Windows peut afficher un avertissement au premier lancement.

### 2.4 Depuis les sources

Il faut [Go](https://go.dev/) 1.25 ou plus récent.

```bash
git clone https://github.com/gnocent/termdevtools.git
cd termdevtools
./install.sh        # Linux, macOS
```

```powershell
.\install.ps1       # Windows
```

Les scripts compilent pour votre machine et installent le binaire dans `~/.local/share/termdevtools` (lié dans `~/.local/bin`) ou `%LOCALAPPDATA%\termdevtools`. Les variables `TERMDEVTOOLS_INSTALL_DIR` et `TERMDEVTOOLS_BIN_DIR` changent ces emplacements. Pour seulement compiler : `go build -o termdevtools .`

## 3. Première connexion

Lancez `termdevtools`. L'interface démarre en anglais ; une fois connecté, `F3` la passe en français et retient ce choix (ou mettez `language: fr` dans `config.yaml`, §5, avant de relancer). Les libellés ci-dessous sont ceux, en anglais, d'un premier lancement, suivis entre parenthèses de leur équivalent en français.

1. **L'écran de connexion** liste les clusters déjà utilisés — aucun la première fois — puis **« + New connection »** (*+ Nouvelle connexion*) et **« Quit »** (*Quitter*). Choisissez « + New connection » avec les flèches et `Entrée`.
2. **URL** : l'adresse du cluster, schéma et port compris, par exemple `https://es.example.com:9200`. N'y mettez pas d'identifiants (`https://utilisateur:motdepasse@…` est refusé : l'URL est enregistrée).
3. **Authentication** (*Authentification*) : `Tab` jusqu'au champ, `Entrée` pour ouvrir la liste, puis choisissez. Les champs qui suivent s'adaptent à ce choix.

   | Choix | Champs à remplir |
   |---|---|
   | none (*aucune*) | — |
   | Basic Auth | *Username*, *Password* (*Mot de passe*) |
   | API Key | *API Key ID*, *API Key secret* |
   | client certificate (mTLS) (*certificat client (mTLS)*) | *Client certificate*, *Client private key*, *Key passphrase* si la clé est chiffrée |

4. **TLS** (URL en `https://` seulement) :
   - *CA file* (*Fichier CA*) : le certificat de l'autorité qui a signé celui du cluster, s'il ne fait pas partie des autorités connues de votre système. `Entrée` sur ce champ ouvre un sélecteur de fichier quand un dossier par défaut est configuré (§5) ; sinon, tapez le chemin.
   - *Verify server certificate (TLS)* (*Vérifier le certificat serveur (TLS)*) : coché par défaut. Ne le décochez que pour un test, sur un réseau de confiance.
5. `Tab` jusqu'à **« Connect »** (*Se connecter*), puis `Entrée`. `Echap` ou « Cancel » (*Annuler*) revient à la liste.

Une fois connecté, la barre de statut indique ce qui a été reconnu : `ES 9.5.4`, `OS 2.19.6`.

**Ce qui est retenu, et ce qui ne l'est pas.** L'URL, le type d'authentification, le nom d'utilisateur, l'identifiant de clé d'API et les chemins de certificats sont enregistrés dans `config.yaml` : à la prochaine connexion, ce cluster apparaît dans la liste et seul le secret est redemandé. Les mots de passe, secrets de clé d'API et passphrases ne sont **jamais** écrits sur le disque.

### Créer une clé d'API (Elasticsearch)

Depuis un outil déjà connecté au cluster (TermDevTools avec Basic Auth convient) :

```
POST _security/api_key
{
  "name": "termdevtools"
}
```

La réponse contient `id` et `api_key` : ce sont l'*API Key ID* et l'*API Key secret* à saisir. La valeur `encoded` de la même réponse n'est pas acceptée telle quelle.

### Certificat client (mTLS)

- Le certificat et la clé sont attendus au format PEM, dans deux fichiers.
- Une clé protégée par une passphrase est acceptée dans les deux formats courants : `BEGIN ENCRYPTED PRIVATE KEY` (PKCS#8, celui qu'OpenSSL écrit par défaut) et `BEGIN RSA PRIVATE KEY` avec un en-tête `DEK-Info`.
- Les fichiers `.p12`/`.pfx` ne sont pas lus : convertissez-les en PEM avec OpenSSL.

## 4. Premiers pas dans l'interface

- **À gauche**, l'éditeur : une requête par bloc, comme dans la console de Kibana. À la première connexion à un cluster, il contient déjà quelques requêtes valables partout.
  ```
  GET _cluster/health

  GET _cat/indices?v&s=store.size:desc
  ```
- **`Ctrl+E`** exécute la requête où se trouve le curseur ; le résultat s'affiche **à droite**.
- **`F8`** ouvre le catalogue de recettes : tapez quelques mots (`unassigned`, `disk`, `snapshot`…), choisissez avec les flèches, `Entrée` insère la recette à la fin de l'éditeur, prête à être exécutée.
- **`Tab`** (ou `F10`) complète un endpoint en cours de frappe, et les colonnes des commandes `_cat` après `h=` ou `s=`.
- **`Ctrl+S`** sauvegarde vos requêtes pour ce cluster ; elles le sont aussi en quittant avec **`Ctrl+C`**.
- **`F1`** affiche tous les raccourcis.

## 5. Réglages : `config.yaml`

Le fichier est créé au premier lancement, chaque paramètre accompagné d'un commentaire (en anglais).

| Système | Emplacement |
|---|---|
| Linux, macOS | `~/.config/termdevtools/config.yaml` |
| Windows | `%USERPROFILE%\.config\termdevtools\config.yaml` |

Si la variable `XDG_CONFIG_HOME` est définie, le dossier est `$XDG_CONFIG_HOME/termdevtools/`.

| Paramètre | Valeur par défaut | Rôle |
|---|---|---|
| `default_timeout_seconds` | `120` | Délai maximal d'une requête, en secondes. |
| `language` | `en` | Langue de l'interface : `en` ou `fr`. `F3` la change et met cette ligne à jour. |
| `mouse` | `false` | Active la souris (clic pour changer de panneau, choisir dans une liste). Désactivée, la sélection et le copier-coller du terminal restent disponibles. |
| `default_ca_dir` | `/etc/pki/tls/certs` sous Linux, vide ailleurs | Dossier proposé pour le champ *Fichier CA* et parcouru par son sélecteur. |
| `default_client_cert_dir` | idem | Même chose pour le certificat et la clé du client. |
| `clusters` | vide | L'historique des connexions, le plus récent en premier. Tenu à jour par le programme. |

Pour chaque cluster, deux lignes facultatives remplacent la détection automatique quand elle ne peut pas aboutir — un proxy qui masque la réponse du cluster, ou un OpenSearch en mode compatibilité, qui annonce une fausse version :

```yaml
clusters:
  - url: https://search.example.com
    auth_type: basic
    username: admin
    distribution: opensearch   # elasticsearch | opensearch
    version: "2.19"
    tls:
      verify: true
```

Modifiez le fichier avec TermDevTools fermé : le programme le réécrit à chaque connexion réussie (il conserve vos commentaires et les clés qu'il ne connaît pas).

## 6. Vos fichiers : requêtes, variables, recettes, endpoints

Tous se trouvent dans le dossier de configuration (§5), et aucun n'est obligatoire.

| Fichier | Contenu | Créé par |
|---|---|---|
| `queries_<cluster>.txt` | Le contenu de l'éditeur pour ce cluster. | `Ctrl+S`, et en quittant |
| `variables_<cluster>.txt` | Vos variables `${nom}` pour ce cluster, une par ligne : `index=mon-index`. | le programme, à la première connexion |
| `recipes/*.txt` | Vos recettes, ajoutées au catalogue. | le programme crée un modèle commenté, `my-recipes.txt` |
| `endpoints.txt` | Vos endpoints, ajoutés à la complétion : un par ligne. | vous |
| `exports/` | Les résultats exportés, un fichier horodaté par export. | `Ctrl+S` sur le résultat |

**Variables.** Une requête peut contenir `${index}` : la valeur est prise dans le fichier de variables au moment de l'exécution. Les recettes du catalogue utilisent `index`, `node`, `repository`, `snapshot`, `field` et `task_id`. Une variable non définie arrête la requête avec un message qui la nomme.

**Recettes.** Un fichier de recettes s'écrit comme l'éditeur, avec quelques directives en commentaire :

```
# @group Snapshots
# @recipe Snapshots nocturnes
# @tags backup
# Ce commentaire s'affiche dans l'aperçu.
GET _snapshot/nightly/_all
```

- `@group` : le thème ; un thème existant range la recette avec celles du binaire.
- `@recipe` : le titre ; avec le groupe et le titre d'une recette intégrée, elle la remplace.
- `@tags` : des mots supplémentaires pour le filtre.
- `@es >=8.7` ou `@opensearch >=2.4 <3.0` : réserve la recette à certains clusters.

Après toute modification, **`F7`** recharge sans redémarrer. Une erreur dans un fichier est signalée dans la barre de statut, avec le fichier et la ligne. Les fichiers doivent être en UTF-8.

Pour lire les recettes et endpoints intégrés, ou vous en inspirer :

```bash
termdevtools --export-defaults ~/termdevtools-defaults
```

## 7. Installation partagée

Plusieurs personnes peuvent utiliser le même binaire sur un serveur : chacune garde son historique, ses requêtes et ses variables dans son propre dossier de configuration.

Ce qui est déposé **à côté du binaire** vaut pour tous :

| Fichier | Effet |
|---|---|
| `recipes/*.txt` | Recettes communes, ajoutées au catalogue de chacun. |
| `endpoints.txt` | Endpoints communs, ajoutés à la complétion. |
| `cheatsheet.txt` | Contenu de départ de l'éditeur, à la place de celui du binaire. |

**Le dossier du binaire doit n'être modifiable que par des personnes de confiance** : ce qui s'y trouve est proposé à tous les utilisateurs. Il peut rester en lecture seule pour les utilisateurs : le programme n'y écrit rien, et les exports (`Ctrl+S` sur le résultat) vont dans le dossier de configuration de chacun (§6).

## 8. Mettre à jour

Remplacez le binaire par le nouveau. Vos fichiers ne sont pas touchés.

**Depuis la 0.5**, trois fichiers étaient installés à côté du binaire :

- `cat_columns.txt` n'est plus lu : supprimez-le.
- `endpoints.txt` est lu comme un complément de la liste intégrée : supprimez-le, sauf si vous y aviez ajouté vos propres endpoints.
- `cheatsheet.txt` fournit toujours le contenu de départ de l'éditeur : supprimez-le pour obtenir celui du binaire.

L'interface démarre désormais en anglais tant qu'aucune langue n'a été choisie : si elle était en français, appuyez une fois sur `F3` après la connexion, le choix est retenu.

**Depuis la 0.6 ou une version antérieure**, les exports étaient écrits dans un dossier `exports/` à côté du binaire. Ils le sont désormais dans le dossier de configuration (§6). L'ancien dossier n'est ni déplacé ni supprimé : récupérez-y vos fichiers si vous en avez besoin.

Voir aussi « À savoir avant de mettre à jour » dans le [journal des versions](CHANGELOG_fr.md).

## 9. Désinstaller

Supprimez le binaire et le dossier de configuration (§5) : TermDevTools n'écrit rien ailleurs. Une version jusqu'à la 0.6 a pu laisser, à côté du binaire, un dossier `exports/` et des fichiers `crash-<date>.log`.

## 10. En cas de problème

Les messages sont cités en anglais, tels qu'un premier lancement les affiche, puis en français (après `F3`).

| Symptôme | Cause probable, et que faire |
|---|---|
| `x509: certificate signed by unknown authority` | Le certificat du cluster est signé par une autorité inconnue de votre système : renseignez le champ *CA file* (*Fichier CA*). |
| `The cluster responded HTTP 401` / `Le cluster a répondu HTTP 401` | Identifiants refusés. |
| `The cluster responded HTTP 403` / `Le cluster a répondu HTTP 403` | Le compte n'a pas le droit de lire la racine du cluster (`GET /`) : il lui faut au moins le privilège de supervision (`monitor`). |
| `The cluster responded HTTP 301` (ou `302`…) suivi d'une adresse | L'URL saisie redirige ailleurs : utilisez l'adresse indiquée. Les redirections ne sont jamais suivies. |
| `No credentials in the URL…` / `Pas d'identifiants dans l'URL…` | Retirez `utilisateur:motdepasse@` de l'URL et choisissez Basic Auth. |
| La barre de statut affiche `Cluster` au lieu de `ES …` ou `OS …` | Le cluster n'a pas été reconnu : tout est proposé sans filtrage. Indiquez `distribution` et `version` dans `config.yaml` (§5). |
| `Tab` ne complète pas | Certains terminaux interceptent `Tab` : utilisez `F10`. |
| `Ctrl+←/→` ne change pas de panneau sous macOS | Le système l'intercepte : utilisez `Option+←/→`. |
| `F2` ne copie rien | La copie passe par le terminal (OSC 52) : PuTTY ne la prend pas en charge, `tmux` et `screen` demandent une configuration. |
| Une requête sur une énorme réponse échoue avec « response larger than 64 MB » | Restreignez-la : `filter_path`, `size`, ou `h=` pour les commandes `_cat`. |
| Un proxy d'entreprise est nécessaire pour atteindre le cluster | Non pris en charge pour l'instant : `HTTPS_PROXY` n'est pas lu. |
| Le programme a planté | Un fichier `crash-<date>.log` est écrit dans le dossier de configuration (§5), ou affiché s'il ne peut pas l'être : joignez-le à un [rapport d'anomalie](https://github.com/gnocent/termdevtools/issues), après avoir vérifié qu'il ne contient rien de confidentiel. |
