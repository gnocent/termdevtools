*(English version: [README.md](README.md))*

# TermDevTools

[![CI](https://github.com/gnocent/termdevtools/actions/workflows/ci.yml/badge.svg)](https://github.com/gnocent/termdevtools/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/gnocent/termdevtools?include_prereleases)](https://github.com/gnocent/termdevtools/releases)
[![License](https://img.shields.io/github/license/gnocent/termdevtools)](LICENSE)

La vue **DevTools** de Kibana dans un terminal, pour interroger un cluster Elasticsearch ou OpenSearch quand Kibana n'est pas là — ou en panne, en pleine investigation. Linux (dont RHEL 8/9/10), Windows ou macOS ; un seul binaire, rien d'autre à installer.

> [!IMPORTANT]
> **Nouveautés, version par version** — détail et limites connues dans le [journal des versions](CHANGELOG_fr.md)
>
> - **0.8.0** — plusieurs clusters dans une session : `Ctrl+O` passe de l'un à l'autre, chacun retrouvé tel qu'on l'a laissé.
> - **0.7.5** — `Echap` abandonne la requête en cours ; la barre de statut compte les secondes d'attente.
> - **0.7.4** — le secret d'un cluster se lit dans une variable d'environnement ou un fichier (`secret_env`, `secret_file`).
> - **0.7.3** — garde-fous de production : les requêtes qui écrivent sont confirmées ou refusées, sous un bandeau.
> - **0.7.2** — installation par `go install` ; la complétion garde la requête après le curseur.
> - **0.7.1** — une ligne de requête incomplète n'exécute plus celle du dessus ; correction d'une connexion http sous Linux.
> - **0.7** — proxys HTTP et SOCKS5, clés d'API telles que Kibana les donne, jetons Bearer.
> - **0.6** — plus rien d'autre que le binaire ; Elasticsearch et OpenSearch reconnus avec leur version ; catalogue de recettes (`F8`).

## Démo

<p align="center"><img src="demotermdevtools.gif" alt="Démo animée : connexion à un cluster de production dont le mot de passe est lu dans une variable d'environnement, requête, complétion d'un endpoint et de colonnes _cat, recette du catalogue, recherche reformatée puis fouillée, bascule vers un OpenSearch et retour, puis un DELETE sur le cluster de production arrêté par la confirmation"></p>

> [!NOTE]
> **Comment ce projet est fait.** Je ne suis pas développeur : j'écris les spécifications ([SPEC_fr.md](SPEC_fr.md)) et je vérifie le résultat ; le code est écrit par [Claude Code](https://claude.com/claude-code), en *vibe coding* pur, avec parfois l'aide de contributeurs. Ce qui compense : environ 400 tests automatiques, dont l'interface pilotée sur un écran simulé ; une CI sous Linux, Windows et macOS à chaque pull request ; chaque recette exécutée sur onze clusters réels ; une revue de sécurité indépendante avant les versions qui touchent aux secrets ou aux protections (0.7.3, 0.7.4, 0.8.0). C'est une **version bêta**, encore peu utilisée : ce qui a été vérifié, et ce qui ne l'a pas été, est dans [Sécurité](#sécurité).

## Fonctionnalités

- **Deux panneaux** : à gauche, les requêtes façon Kibana Console (`MÉTHODE endpoint` + corps JSON), à droite le résultat formaté, avec le rappel de la requête et les en-têtes de réponse.
- **Exécution au curseur** (`Ctrl+E`) : plusieurs requêtes cohabitent dans l'éditeur, celle sous le curseur part.
- **Catalogue de recettes** (`F8`) : une centaine de requêtes prêtes à l'emploi pour les investigations courantes, filtrées à la frappe ; ajoutez les vôtres.
- **S'adapte au cluster** : Elasticsearch 7.17 à 9.x, OpenSearch 2.x et 3.x, détectés à la connexion ; seul ce qui existe sur ce cluster est proposé.
- **Auto-complétion** (`Tab`) des endpoints et des colonnes `_cat` (`h=`, `s=`), ces dernières demandées au cluster lui-même.
- **Plusieurs clusters dans une session** (`Ctrl+O`) : une requête en cours continue hors écran, la barre de statut signale sa réponse.
- **Garde-fous de production**, cluster par cluster : écritures confirmées ou refusées, bandeau avec un libellé comme `PROD`.
- **Connexion** : Basic Auth, clé d'API, jeton Bearer ou certificat client (mTLS), à travers un proxy HTTP ou SOCKS5 ; secret saisi, ou lu dans une variable ou un fichier, jamais enregistré.
- **Au quotidien** : variables `${nom}`, reformatage du JSON (`F4`), copie en commande `curl` (`F9`, secrets masqués), recherche (`Ctrl+F`), export (`Ctrl+S`), copie presse-papier à travers SSH (`F2`), sauvegarde automatique des requêtes par cluster.

Détail de chaque choix : [SPEC_fr.md](SPEC_fr.md).

## Installation

Un fichier par plateforme, et rien d'autre, sur la page [Releases](https://github.com/gnocent/termdevtools/releases) :

| Plateforme | Fichier |
|---|---|
| Linux (x86-64) | `termdevtools-linux-amd64` |
| Windows (x86-64) | `termdevtools-windows-amd64.exe` |
| macOS (Apple Silicon) | `termdevtools-darwin-arm64` |

```bash
sha256sum -c SHA256SUMS --ignore-missing     # vérifie le fichier téléchargé
chmod +x termdevtools-linux-amd64
mv termdevtools-linux-amd64 ~/.local/bin/termdevtools
```

Ou, avec [Go](https://go.dev/) 1.25 ou plus : `go install github.com/gnocent/termdevtools@latest`.

Le binaire est statique : il se copie tel quel, y compris sur une machine sans Internet. Il n'est pas signé ; macOS et Windows peuvent le signaler au premier lancement. Plateforme par plateforme, scripts d'installation, installation partagée et mise à jour depuis une version antérieure : le **[guide d'installation](INSTALL_fr.md)**.

## Démarrage rapide

1. **Lancer** `termdevtools`. L'écran de connexion liste les clusters déjà utilisés et « + New connection ». L'interface démarre en anglais ; `F3` la passe en français.
2. **Se connecter** : URL, protection s'il s'agit de production, authentification, secret. Tout est retenu pour la prochaine fois, sauf le secret.
3. **Écrire une requête** à gauche, par exemple `GET _cluster/health`, et l'**exécuter** avec `Ctrl+E`.
4. **Ou choisir une recette** : `F8`, quelques mots (`unassigned`, `disk`, `snapshot`…), `Entrée`, puis `Ctrl+E`.
5. **Passer à un autre cluster** : `Ctrl+O`. Le premier reste ouvert, tel quel.

Pas à pas, champ par champ : [guide, §3](INSTALL_fr.md#3-première-connexion).

## Configuration

Rien à préparer : `~/.config/termdevtools/config.yaml` est créé au premier lancement, chaque réglage documenté sur place, et retient les clusters utilisés — jamais leur secret.

- **Secret sans ressaisie** : `secret_env: ES_PW` ou `secret_file: ~/.es-prod.secret` sur l'entrée d'un cluster.
- **Proxy** : `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY`, ou `proxy:` par cluster (`http://…`, `socks5://…`, `none`).
- **Langue** (`language: fr`, ou `F3`) et **souris** (`mouse: true`, désactivée par défaut).
- **Vos fichiers**, dans `~/.config/termdevtools/` : requêtes et variables `${nom}` par cluster, vos recettes (`recipes/`) et endpoints, rechargés avec `F7` ; `termdevtools --export-defaults <dossier>` écrit ceux du binaire.

Chaque réglage et chaque fichier : [guide, §5 et §6](INSTALL_fr.md#5-réglages--configyaml).

## Raccourcis clavier

| Action | Touche |
|---|---|
| Exécuter la requête sous le curseur | `Ctrl+E` |
| Abandonner la requête en cours | `Echap` |
| Ouvrir une autre connexion, revenir à une connexion ouverte | `Ctrl+O` (`Echap` pour revenir) |
| Changer de panneau | `Ctrl+←` / `Ctrl+→` |
| Rechercher (dans le panneau actif) | `Ctrl+F` |
| Sauvegarder (gauche) / exporter (droite) | `Ctrl+S` |
| Catalogue de recettes | `F8` |
| Compléter un endpoint ou une colonne | `Tab`, `F10` ou `Ctrl+Espace` |
| Reformater le JSON sous le curseur | `F4` |
| Copier la requête en commande `curl` | `F9` |
| Recharger vos fichiers (variables, recettes, endpoints) | `F7` |
| Copier le résultat dans le presse-papier | `F2` |
| Redimensionner les panneaux | `F5` / `F6` |
| Langue de l'interface (fr/en) | `F3` |
| Aide | `F1` |
| Quitter (sauvegarde chaque connexion ouverte) | `Ctrl+C` |

Selon le terminal, certaines combinaisons n'arrivent pas : `Ctrl+Entrée` (utilisez `Ctrl+E`), `Ctrl+←/→` sous macOS et PuTTY (`Alt+←/→`), `Ctrl+Maj+←/→` (`F5`/`F6`), `Tab` sous PuTTY (`F10`). **PuTTY est déconseillé** ; aucun souci avec les terminaux natifs de macOS et de Windows 10+.

## Sécurité

**Ce que le programme fait, et ne fait pas**

- **Il ne parle qu'au cluster choisi**, à travers un proxy seulement si votre environnement ou `config.yaml` en désigne un, et en l'affichant. Aucune télémétrie, aucune recherche de mise à jour, aucune commande externe.
- **Aucun secret n'est écrit sur le disque** : il est saisi à chaque connexion, ou lu dans la variable ou le fichier que vous désignez — ce fichier-là est à protéger. Une URL contenant des identifiants est refusée.
- **TLS vérifié par défaut** (TLS 1.2 au minimum) ; les redirections ne sont jamais suivies.
- **Rien de ce qui est affiché n'est interprété par le terminal**, quelle que soit la réponse du cluster.
- **Fichiers à vous seul** (`0600`, dossiers `0700`, sous Linux et macOS), écrits de façon atomique ; `F9` n'inclut jamais les secrets.

**Ce à quoi il fait confiance**

- **Le dossier du binaire et votre dossier de configuration** : leurs recettes et endpoints sont proposés tels quels.
- **Le proxy** : en https, il ne voit que l'adresse ; en http, il voit tout, identifiants compris.
- **Vous** : sans protection, une requête part telle qu'elle est écrite, `DELETE` compris. La protection évite une erreur, elle n'arrête pas qui veut écrire ; pour l'empêcher vraiment, utilisez un compte sans droit d'écriture.

**Ce qui a été vérifié, et ce qui ne l'a pas été**

- Relecture de sécurité et tests automatisés de chacun des points ci-dessus ; `govulncheck` ne signale aucune vulnérabilité connue (dépendances et Go 1.27.3, qui compile les binaires).
- Sur des clusters réels : chaque recette (Elasticsearch 7.17 à 9.5, OpenSearch 2.0 à 3.9), l'authentification par mot de passe, clé d'API et jeton Bearer, la protection ; la bascule entre clusters, par un contributeur, sur des instances de laboratoire.
- Le proxy n'est vérifié qu'avec des serveurs de test, pas avec un proxy d'entreprise réel.
- **Aucun audit externe indépendant**, et des binaires non signés : vérifiez leur somme SHA-256. Voir aussi l'[avertissement](#avertissement--limitation-de-responsabilité) ci-dessous.

## Licence

Ce projet est distribué sous licence **[GNU Affero General Public License v3.0](LICENSE)** (AGPLv3) : vous êtes libre de l'utiliser, l'étudier, le modifier et le redistribuer, à condition que le code source (y compris vos modifications) reste disponible dans les mêmes termes — y compris si l'outil est exposé via un réseau (usage en mode service).

> **Note d'intention (non contraignante juridiquement)** : l'esprit de ce projet est de rester un outil communautaire et amélioré collectivement, pas un produit revendu tel quel. L'AGPLv3 n'interdit pas formellement un usage commercial — seule une licence non-commerciale le ferait, au prix de restrictions plus lourdes et moins "open source" — mais c'est l'usage que son auteur espère en voir fait.

## Avertissement / limitation de responsabilité

TermDevTools est un outil publié **tel quel** ("as is"), sans garantie d'aucune sorte, explicite ou implicite — y compris, sans s'y limiter, les garanties de qualité marchande, d'adéquation à un usage particulier et d'absence de contrefaçon (voir les articles 15 à 17 de la [licence AGPLv3](LICENSE), qui font foi).

En particulier :

- Ce projet est développé et maintenu **sur le temps libre de son auteur**, sans engagement de disponibilité, de maintenance, de correctif de sécurité ou d'évolution future.
- L'auteur et les contributeurs **déclinent toute responsabilité** pour les conséquences directes ou indirectes de l'utilisation de cet outil — y compris, sans s'y limiter, une perte de données, une interruption de service, ou toute action exécutée sur un cluster Elasticsearch ou OpenSearch via cet outil (TermDevTools exécute les requêtes telles que vous les écrivez, sans confirmation supplémentaire au-delà de ce qui est décrit dans [SPEC_fr.md](SPEC_fr.md)).
- L'utilisation de cet outil contre un cluster de production reste **sous l'entière responsabilité de la personne qui l'utilise** : vérifiez toujours vos requêtes, en particulier les opérations destructrices (`DELETE`, mises à jour de mapping, etc.), comme vous le feriez avec n'importe quel client Elasticsearch (Kibana, `curl`, ou autre).
- Les évolutions futures du projet (ou leur absence) n'engagent que leurs auteurs respectifs au moment où elles sont apportées.
