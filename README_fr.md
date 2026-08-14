*(English version: [README.md](README.md))*

# TermDevTools

Simulateur en mode terminal de la vue **DevTools** de Kibana, pour interroger un cluster Elasticsearch directement depuis un terminal Linux (RHEL 8/9/10), sans navigateur ni Kibana fonctionnel.

## Pourquoi

Il arrive qu'un cluster Elasticsearch n'ait pas de Kibana disponible, ou que son Kibana soit hors service — typiquement pendant une investigation où c'est justement le moment où on en aurait le plus besoin. Faire l'équivalent en `curl` à la main est possible mais pénible (gestion du TLS, requêtes multi-lignes, mise en forme du JSON de réponse...). TermDevTools reproduit l'essentiel du confort des DevTools de Kibana — éditeur de requêtes, exécution au curseur, réponse JSON formatée — dans un simple binaire terminal.

## Fonctionnalités

- **Interface en deux panneaux** : éditeur de requêtes à gauche (`MÉTHODE endpoint` + corps JSON optionnel), résultat JSON formaté à droite.
- **Exécution au curseur** (`Ctrl+Entrée`) : plusieurs requêtes peuvent cohabiter dans l'éditeur, séparées par des lignes vides ; celle sous le curseur est exécutée.
- **Auto-complétion** (`Tab`) des endpoints (`_cat/*`, `_cluster/*`, `_nodes/*`, gestion d'index, ILM/SLM, snapshots, licence...) et, pour les commandes `_cat/*`, des noms de colonnes des paramètres `h=`/`s=`. Listes personnalisables sans recompiler via `endpoints.txt` et `cat_columns.txt`.
- **Recherche** (`Ctrl+F`) dans l'éditeur comme dans le résultat.
- **Sauvegarde automatique** des requêtes en cours par cluster et par utilisateur (à la fermeture et via `Ctrl+S`), rechargées à la reconnexion.
- **Export** du résultat affiché vers un fichier horodaté (`Ctrl+S`, panneau droit) et **copie presse-papier** via OSC 52 (`F2`, fonctionne à travers SSH).
- **Connexion** : Basic Auth, API Key ou certificat client (mTLS), avec ou sans vérification TLS ; historique des clusters déjà utilisés (sans jamais y stocker de secret — voir [Sécurité](#sécurité)).
- **Aide intégrée** (`F1`) : rappel des raccourcis et de l'emplacement des fichiers.

Détail complet des choix et du comportement : [SPEC_fr.md](SPEC_fr.md).

## Installation

### Installation rapide (recommandé)

Nécessite [Go](https://go.dev/) 1.25 ou supérieur. Compile nativement pour votre plateforme (pas de cross-compilation) et installe le binaire avec ses fichiers annexes au même endroit :

```bash
# Linux / macOS
git clone <url-du-dépôt>
cd TermDevTools
./install.sh
```

```powershell
# Windows (PowerShell)
git clone <url-du-dépôt>
cd TermDevTools
.\install.ps1
```

Chaque script :

- compile `termdevtools` pour votre OS/architecture courants ;
- copie `cat_columns.txt` et `endpoints.txt` à côté, toujours mis à jour depuis le dépôt ;
- initialise `cheatsheet.txt` depuis `cheatsheet.txt.example` **la première fois seulement** — relançable sans risque, n'écrase jamais une cheatsheet déjà personnalisée ;
- indique quoi ajouter à votre `PATH` si l'emplacement d'installation n'y est pas encore.

Emplacement par défaut : `~/.local/share/termdevtools` sous Linux/macOS (lié via un symlink dans `~/.local/bin`, ajouté au `PATH`), `%LOCALAPPDATA%\termdevtools` sous Windows. Personnalisable via la variable d'environnement `TERMDEVTOOLS_INSTALL_DIR` (et `TERMDEVTOOLS_BIN_DIR` sous Linux/macOS pour l'emplacement du symlink) — utile par exemple pour une installation partagée en équipe dans `/opt/termdevtools`.

### Binaires précompilés

Des binaires statiques sont fournis pour Linux (amd64), Windows (amd64) et macOS (Apple Silicon / arm64) — voir la section [Releases](../../releases) du dépôt, où chaque binaire est fourni avec ses fichiers annexes (voir [Arborescence d'installation](#arborescence-dinstallation) ci-dessous). Aucune dépendance à installer : il suffit de télécharger et de le rendre exécutable (`chmod +x` sous Linux/macOS).

### Compilation manuelle / cross-compilation

Pour compiler sans installer, ou cross-compiler vers une plateforme différente de la vôtre :

```bash
git clone <url-du-dépôt>
cd TermDevTools
go build -o termdevtools .
```

Le binaire est statique (`CGO_ENABLED=0`) : il n'a besoin d'aucune bibliothèque système au-delà de la libc de base, et peut être copié tel quel sur n'importe quelle machine RHEL 8/9/10 (ou toute autre distribution Linux amd64), sans installation.

Le script [`build-release.sh`](build-release.sh) compile les trois plateformes cibles (`linux/amd64`, `windows/amd64`, `darwin/arm64`) en une fois et regroupe chaque binaire avec ses fichiers annexes dans `dist/<plateforme>/` — utile pour produire des binaires à distribuer à l'équipe plutôt que pour une installation locale.

### Arborescence d'installation

Tout ce qui suit est **facultatif**, à l'exception du binaire lui-même — TermDevTools fonctionne avec des valeurs par défaut intégrées pour tout le reste.

| Emplacement | Fichier | Rôle |
|---|---|---|
| à côté du binaire | `cheatsheet.txt` | Contenu par défaut de l'éditeur au premier lancement sur un cluster donné (copier/renommer `cheatsheet.txt.example`, ou laisser `install.sh`/`install.ps1` le faire). Absent → éditeur vide. |
| à côté du binaire | `endpoints.txt` | Liste des endpoints proposés en auto-complétion. Absent → utilise une liste intégrée. |
| à côté du binaire | `cat_columns.txt` | Table commande `_cat/*` → colonnes, pour l'auto-complétion des paramètres `h=`/`s=`. Absent → utilise une table intégrée. |
| `~/.config/termdevtools/config.yaml` | — | **Créé automatiquement** à la première connexion réussie — rien à préparer à la main. Voir [Configuration](#configuration) ci-dessous. |

## Configuration

Aucune configuration n'est nécessaire pour démarrer : un écran de connexion permet de saisir directement l'URL et les identifiants d'un cluster, et `~/.config/termdevtools/config.yaml` est créé automatiquement à la première connexion réussie. Un exemple est fourni à titre indicatif dans `config.yaml.example` — **il ne contient jamais de secret** : mots de passe, clés d'API et passphrases sont redemandés à chaque connexion, jamais écrits sur disque (voir [Sécurité](#sécurité)).

La langue de l'interface (français par défaut, ou anglais) se règle via `language: fr` / `language: en` dans ce même `config.yaml` — ou se change à la volée dans l'appli avec `F3`, qui enregistre le choix pour la prochaine fois.

Le support de la souris (cliquer pour donner le focus à un champ ou sélectionner une entrée de liste) est **désactivé par défaut** — mettre `mouse: true` dans `config.yaml` pour l'activer. Toute interaction souris a un équivalent clavier complet (voir [Raccourcis clavier](#raccourcis-clavier)) ; le laisser désactivé garde la sélection/collage natifs du terminal disponibles, puisque l'activer capte les événements souris pour l'appli à la place (`F2` copie toujours le résultat, avec ou sans souris).

## Raccourcis clavier

| Action | Touche |
|---|---|
| Exécuter la requête sous le curseur | `Ctrl+E` [^entree] |
| Basculer focus panneau gauche ↔ droit | `Ctrl+←` / `Ctrl+→` [^focus] |
| Quitter (sauvegarde automatique du panneau gauche) | `Ctrl+C` |
| Rechercher dans les requêtes / dans le résultat | `Ctrl+F` (selon le panneau focus) |
| Redimensionner le split gauche/droite | `F5` / `F6` [^redim] |
| Sauvegarder (gauche) / exporter (droite) | `Ctrl+S` (selon le panneau focus) |
| Compléter un endpoint / une colonne | `Tab` ou `F10` (panneau gauche) [^tab] |
| Copier le résultat dans le presse-papier | `F2` |
| Changer la langue de l'interface (fr/en) | `F3` |
| Aide | `F1` (`Echap` pour fermer) |

[^entree]: `Ctrl+Entrée` fonctionne aussi sur les terminaux qui le rapportent distinctement d'un simple `Entrée` — beaucoup ne le font pas, d'où `Ctrl+E` comme raccourci principal, toujours fiable.
[^focus]: Sur macOS, `Option`/`Alt` fonctionne aussi à la place de `Ctrl` — `Ctrl+←/→` est intercepté par le système par défaut (changement d'espace Mission Control).
[^redim]: `Ctrl+Maj+←/→` (`Option`/`Alt` inclus, sur macOS) fonctionne aussi sur les terminaux qui le rapportent distinctement d'une flèche non modifiée — tous ne le font pas, d'où `F5`/`F6` comme raccourcis principaux, toujours fiables.
[^tab]: Sur certains terminaux (notamment PuTTY), `Tab` est absorbé entièrement — aucun événement clavier n'atteint l'appli. `F10` est une alternative garantie fiable. **PuTTY est globalement déconseillé** : aucun souci sous macOS/Windows 10+ natifs.

## Sécurité

- **TLS vérifié par défaut** : la vérification du certificat serveur est activée sauf désactivation explicite lors de la connexion.
- **Aucun secret persisté** : mot de passe, secret d'API Key et passphrase de clé privée ne sont jamais écrits dans `config.yaml` — seuls l'URL, le type d'authentification et les identifiants non sensibles (username, API key ID, chemins de certificats) le sont, avec des permissions restreintes (`0600` pour les fichiers, `0700` pour les dossiers).
- **Copie presse-papier (`F2`)** via OSC 52 : le terminal local reçoit la donnée à copier sans jamais transiter par un presse-papier serveur — mais aucune garantie de succès n'est renvoyée par ce mécanisme (dépend du terminal utilisé).
- Le projet a été soumis à une relecture de sécurité (revue du code, absence d'exécution de commandes externes, `govulncheck` sans vulnérabilité connue exploitable) avant publication — voir aussi le [disclaimer](#avertissement--limitation-de-responsabilité) ci-dessous.

## Licence

Ce projet est distribué sous licence **[GNU Affero General Public License v3.0](LICENSE)** (AGPLv3) : vous êtes libre de l'utiliser, l'étudier, le modifier et le redistribuer, à condition que le code source (y compris vos modifications) reste disponible dans les mêmes termes — y compris si l'outil est exposé via un réseau (usage en mode service).

> **Note d'intention (non contraignante juridiquement)** : l'esprit de ce projet est de rester un outil communautaire et amélioré collectivement, pas un produit revendu tel quel. L'AGPLv3 n'interdit pas formellement un usage commercial — seule une licence non-commerciale le ferait, au prix de restrictions plus lourdes et moins "open source" — mais c'est l'usage que son auteur espère en voir fait.

## Avertissement / limitation de responsabilité

TermDevTools est un outil publié **tel quel** ("as is"), sans garantie d'aucune sorte, explicite ou implicite — y compris, sans s'y limiter, les garanties de qualité marchande, d'adéquation à un usage particulier et d'absence de contrefaçon (voir les articles 15 à 17 de la [licence AGPLv3](LICENSE), qui font foi).

En particulier :

- Ce projet est développé et maintenu **sur le temps libre de son auteur**, sans engagement de disponibilité, de maintenance, de correctif de sécurité ou d'évolution future.
- L'auteur et les contributeurs **déclinent toute responsabilité** pour les conséquences directes ou indirectes de l'utilisation de cet outil — y compris, sans s'y limiter, une perte de données, une interruption de service, ou toute action exécutée sur un cluster Elasticsearch via cet outil (TermDevTools exécute les requêtes telles que vous les écrivez, sans confirmation supplémentaire au-delà de ce qui est décrit dans [SPEC_fr.md](SPEC_fr.md)).
- L'utilisation de cet outil contre un cluster de production reste **sous l'entière responsabilité de la personne qui l'utilise** : vérifiez toujours vos requêtes, en particulier les opérations destructrices (`DELETE`, mises à jour de mapping, etc.), comme vous le feriez avec n'importe quel client Elasticsearch (Kibana, `curl`, ou autre).
- Les évolutions futures du projet (ou leur absence) n'engagent que leurs auteurs respectifs au moment où elles sont apportées.
