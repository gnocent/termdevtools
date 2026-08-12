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

Détail complet des choix et du comportement : [SPEC.md](SPEC.md).

## Installation

### Binaires précompilés

Des binaires statiques sont fournis pour Linux (amd64), Windows (amd64) et macOS (Apple Silicon / arm64) — voir la section [Releases](../../releases) du dépôt. Aucune dépendance à installer : il suffit de télécharger le binaire correspondant à votre plateforme et de le rendre exécutable (`chmod +x` sous Linux/macOS).

### Compilation depuis les sources

Nécessite [Go](https://go.dev/) 1.25 ou supérieur.

```bash
git clone <url-du-dépôt>
cd TermDevTools
go build -o termdevtools .
```

Le binaire est statique (`CGO_ENABLED=0`) : il n'a besoin d'aucune bibliothèque système au-delà de la libc de base, et peut être copié tel quel sur n'importe quelle machine RHEL 8/9/10 (ou toute autre distribution Linux amd64), sans installation.

Le script [`build-release.sh`](build-release.sh) compile les trois plateformes cibles (`linux/amd64`, `windows/amd64`, `darwin/arm64`) et regroupe chaque binaire avec ses fichiers annexes dans `dist/<plateforme>/`.

## Configuration et fichiers annexes

Au premier lancement, aucune configuration n'est nécessaire : un écran de connexion permet de saisir directement l'URL et les identifiants d'un cluster. Les fichiers suivants sont ensuite lus **s'ils existent**, à côté du binaire :

| Fichier | Rôle |
|---|---|
| `cheatsheet.txt` | Contenu par défaut de l'éditeur au premier lancement sur un cluster donné (copier `cheatsheet.txt.example`). |
| `endpoints.txt` | Liste des endpoints proposés en auto-complétion (remplace la liste intégrée). |
| `cat_columns.txt` | Table commande `_cat/*` → colonnes, pour l'auto-complétion des paramètres `h=`/`s=`. |

Un exemple de configuration utilisateur (`~/.config/termdevtools/config.yaml`, généré automatiquement à la première connexion) est fourni à titre indicatif dans `config.yaml.example` — **il ne contient jamais de secret** : mots de passe, clés d'API et passphrases sont redemandés à chaque connexion, jamais écrits sur disque (voir [Sécurité](#sécurité)).

## Raccourcis clavier

| Action | Touche |
|---|---|
| Exécuter la requête sous le curseur | `Ctrl+Entrée` |
| Basculer focus panneau gauche ↔ droit | `Ctrl+←` / `Ctrl+→` |
| Quitter (sauvegarde automatique du panneau gauche) | `Ctrl+C` |
| Rechercher dans les requêtes / dans le résultat | `Ctrl+F` (selon le panneau focus) |
| Redimensionner le split gauche/droite | `Ctrl+Maj+←` / `Ctrl+Maj+→` |
| Sauvegarder (gauche) / exporter (droite) | `Ctrl+S` (selon le panneau focus) |
| Compléter un endpoint / une colonne | `Tab` (panneau gauche) |
| Copier le résultat dans le presse-papier | `F2` |
| Aide | `F1` (`Echap` pour fermer) |

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
- L'auteur et les contributeurs **déclinent toute responsabilité** pour les conséquences directes ou indirectes de l'utilisation de cet outil — y compris, sans s'y limiter, une perte de données, une interruption de service, ou toute action exécutée sur un cluster Elasticsearch via cet outil (TermDevTools exécute les requêtes telles que vous les écrivez, sans confirmation supplémentaire au-delà de ce qui est décrit dans [SPEC.md](SPEC.md)).
- L'utilisation de cet outil contre un cluster de production reste **sous l'entière responsabilité de la personne qui l'utilise** : vérifiez toujours vos requêtes, en particulier les opérations destructrices (`DELETE`, mises à jour de mapping, etc.), comme vous le feriez avec n'importe quel client Elasticsearch (Kibana, `curl`, ou autre).
- Les évolutions futures du projet (ou leur absence) n'engagent que leurs auteurs respectifs au moment où elles sont apportées.
