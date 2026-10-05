*(Version française : [README_fr.md](README_fr.md))*

# TermDevTools

A terminal-mode simulator of Kibana's **DevTools** view, for querying an Elasticsearch or OpenSearch cluster directly from a terminal — Linux (including RHEL 8/9/10), Windows, or macOS — without a browser or a working Kibana. A single binary, with nothing else to install.

> [!IMPORTANT]
> **What's new in 0.6 (beta)**
>
> - **Recipe catalog (`F8`)** — about a hundred ready-made requests for the usual investigations: cluster health, unassigned shards, disk, nodes, tasks, snapshots, index lifecycle, upgrades.
> - **Elasticsearch and OpenSearch, by version** — the distribution and version are detected at connection (Elasticsearch 7.17 to 9.x, OpenSearch 2.x and 3.x); only the recipes and endpoints that exist on that cluster are offered.
> - **A single file to install** — recipes, endpoints and `_cat` columns are built into the binary; releases come with their SHA-256 checksums.
> - **Your own recipes and endpoints** — plain text files, added to the built-in ones and reloaded with `F7`.
> - **Safer** — HTTP redirections never followed, credentials refused in the URL, atomic saves, `Ctrl+C` always saving before quitting, PKCS#8 encrypted client keys accepted.
>
> This is a **beta version**: checked against eleven real clusters, not yet by users other than its author. Details, what to know before upgrading from 0.5, and known limits: **[changelog](CHANGELOG.md)**.

- [Demo](#demo)
- [Why](#why)
- [Features](#features)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [Recipes and reference data](#recipes-and-reference-data)
- [Reusable variables](#reusable-variables)
- [Keyboard shortcuts](#keyboard-shortcuts)
- [Security](#security)
- [License](#license)

Other documents: [installation and configuration guide](INSTALL.md) · [changelog](CHANGELOG.md) · [specification](SPEC.md).

## Demo

<p align="center"><img src="demotermdevtools.gif" alt="Animated demo: connecting to a cluster, running requests, browsing an index mapping and _cat/shards, and searching within a result"></p>

*Animation recorded with version 0.5: the recipe catalog (`F8`) isn't in it.*

## Why

Sometimes an Elasticsearch cluster has no Kibana available, or its Kibana is down — typically during an investigation, which is exactly when you'd need it most. Doing the equivalent by hand with `curl` is possible but tedious (TLS handling, multi-line requests, formatting the JSON response...). TermDevTools reproduces most of the comfort of Kibana's DevTools — a request editor, execute-at-cursor, formatted JSON responses — in a single terminal binary.

## Features

- **Two-panel interface**: request editor (with line numbers and auto-closing `{`/`[`/`"`) on the left (`METHOD endpoint` + optional JSON body), formatted JSON result — request reminder and response headers included — on the right.
- **Execute at cursor** (`Ctrl+E`): several requests can coexist in the editor, separated by blank lines; the one under the cursor is executed.
- **Recipe catalog** (`F8`): about a hundred ready-made requests for the usual investigations — cluster health, unassigned shards, disk, nodes, tasks, snapshots, index lifecycle, upgrades — filtered as you type, previewed, inserted into the editor with `Enter`. Add your own; see [Recipes and reference data](#recipes-and-reference-data).
- **Adapts to the cluster**: the distribution and version are detected at connection — Elasticsearch 7.17 to 9.x, OpenSearch 2.x and 3.x — and only the recipes and endpoints that exist on that cluster are offered.
- **Auto-completion** (`Tab`) for endpoints (`_cat/*`, `_cluster/*`, `_nodes/*`, index management, ILM/SLM or ISM, snapshots, ingest, license...) and, for `_cat/*` commands, for the column names of the `h=`/`s=` parameters — asked from the cluster itself, so exact for whatever version it runs.
- **Reformat the JSON body** under the cursor in place (`F4`) and **copy the request as an equivalent `curl` command** (`F9`, secrets redacted).
- **Reusable `${name}` variables** — see [Reusable variables](#reusable-variables).
- **Nothing but the binary**: recipes, endpoints and `_cat` columns are built in. Your own additions are plain text files, reloaded with `F7`.
- **Search** (`Ctrl+F`) in the editor as well as in the result.
- **Automatic save** of in-progress requests per cluster and per user (on exit and via `Ctrl+S`), reloaded on reconnection.
- **Export** of the displayed result to a timestamped file (`Ctrl+S`, right panel) and **clipboard copy** via OSC 52 (`F2`, works over SSH).
- **Connection**: Basic Auth, API Key, or client certificate (mTLS, key encrypted or not), with or without TLS verification; history of previously used clusters (never storing a secret there — see [Security](#security)); a certificate picker (`Enter` on the CA/client cert fields) browses the configured directory instead of typing a filename from memory.
- **Built-in help** (`F1`): reminder of shortcuts and file locations.

Full detail of design choices and behavior: [SPEC.md](SPEC.md).

## Installation

The **[installation and configuration guide](INSTALL.md)** details every step, platform by platform, up to the first request. In short:

### Prebuilt binaries (recommended)

One file per platform, and nothing else, on the [Releases](https://github.com/gnocent/termdevtools/releases) page:

| Platform | File |
|---|---|
| Linux (x86-64) | `termdevtools-linux-amd64` |
| Windows (x86-64) | `termdevtools-windows-amd64.exe` |
| macOS (Apple Silicon) | `termdevtools-darwin-arm64` |

On Linux, for instance:

```bash
sha256sum -c SHA256SUMS --ignore-missing     # checks the downloaded file
chmod +x termdevtools-linux-amd64
mv termdevtools-linux-amd64 ~/.local/bin/termdevtools
termdevtools --version
```

The binary is static: it needs no system library and can be copied as-is onto another machine, including one with no Internet access. The binaries are not signed; macOS and Windows may point it out on first launch (see the [guide](INSTALL.md#2-installing-the-binary)).

### From source

Requires [Go](https://go.dev/) 1.25 or later.

```bash
# Linux / macOS
git clone https://github.com/gnocent/termdevtools.git
cd termdevtools
./install.sh
```

```powershell
# Windows (PowerShell)
git clone https://github.com/gnocent/termdevtools.git
cd termdevtools
.\install.ps1
```

Each script:

- builds `termdevtools` for your current OS/architecture — the binary is all there is to install;
- prints what to add to your `PATH` if the install location isn't on it yet;
- points out, without touching them, the companion files an earlier version (up to 0.5) may have left next to the binary (see [Upgrading from 0.5](#upgrading-from-05)).

Default install location: `~/.local/share/termdevtools` on Linux/macOS (symlinked from `~/.local/bin`), `%LOCALAPPDATA%\termdevtools` on Windows. Override it with the `TERMDEVTOOLS_INSTALL_DIR` environment variable (and `TERMDEVTOOLS_BIN_DIR` on Linux/macOS for the symlink location) — for instance for a shared installation in `/opt/termdevtools`.

To build without installing: `go build -o termdevtools .`

The [`build-release.sh`](build-release.sh) script produces the three published binaries and their `SHA256SUMS` file in `dist/`, stamping them with the version (`./build-release.sh v0.6`, or without an argument what `git describe` says of the checkout) — the one `termdevtools --version` prints.

### Installation layout

Only the binary is needed. Every file below is **optional** or created by the program itself.

| Location | File | Purpose |
|---|---|---|
| `~/.config/termdevtools/` | `config.yaml` | **Created automatically** — nothing to set up by hand. See [Configuration](#configuration) below. |
| `~/.config/termdevtools/` | `queries_<cluster>.txt` | Your requests for each cluster, saved by `Ctrl+S` and on exit. |
| `~/.config/termdevtools/` | `variables_<cluster>.txt` | Your `${name}` variables for each cluster — see [Reusable variables](#reusable-variables). |
| `~/.config/termdevtools/` | `recipes/*.txt`, `endpoints.txt` | Your own recipes and endpoints, added to the built-in ones — see [Recipes and reference data](#recipes-and-reference-data). |
| `~/.config/termdevtools/` | `exports/` | Results exported with `Ctrl+S` from the right panel. Up to 0.6, this directory was next to the binary. |
| next to the binary | `recipes/*.txt`, `endpoints.txt` | Same, shared by everyone using that installation. |
| next to the binary | `cheatsheet.txt` | That installation's own starting content for the editor, in place of the built-in one. |

On Windows, `~` stands for `%USERPROFILE%`. The program writes nothing next to the binary: its directory can be read-only.

### Upgrading from 0.5

Versions up to 0.5 came with three files next to the binary. After replacing the binary:

- `cat_columns.txt` is no longer read (the columns are asked from the cluster): delete it.
- `endpoints.txt` is still read, but as *additions* to the built-in list rather than a replacement. Unless you added endpoints of your own to it, delete it — the built-in list is more complete and matches the cluster's version.
- `cheatsheet.txt` still gives the editor its starting content for a cluster you connect to for the first time. Delete it to get the built-in starter instead; its requests are now in the recipe catalog (`F8`).

The interface now starts in English as long as no language was chosen: `F3` switches back to French and remembers that choice. The other behavior changes (redirections, credentials in the URL, `F7`) are listed in the [changelog](CHANGELOG.md).

## Quick start

1. **Launch it**: `termdevtools` (`termdevtools.exe` on Windows). The connection screen lists any previously used clusters, plus a **"+ New connection"** option.
2. **Connect**: enter the cluster's URL, pick an authentication type (none, Basic Auth, API Key, or client certificate), and the secret if there is one. Everything except the secret is remembered for next time (see [Configuration](#configuration) below).
3. **Write a request** in the left panel, Kibana Console style — method, endpoint, and an optional JSON body on the following lines:
   ```
   GET _cluster/health
   ```
   Several requests can coexist in the editor, separated by blank lines; the one under the cursor is the one that runs. The first time you connect to a cluster, the editor already holds a few requests that work anywhere.
4. **Run it**: `Ctrl+E`. The formatted JSON response shows up in the right panel (see the [demo](#demo) above).
5. **Or pick a recipe**: `F8` opens the catalog. Type a few words (`unassigned`, `disk`, `snapshot`...), move with the arrows, `Enter` inserts the recipe at the end of the editor, cursor on its request — `Ctrl+E` runs it.
6. From there: `Tab` or `F10` auto-completes an endpoint while typing, `F4` reformats the JSON body under the cursor, `F9` copies the request as an equivalent `curl` command, `Ctrl+S` saves your work for next time. Full reference: [Keyboard shortcuts](#keyboard-shortcuts).

The step-by-step version, field by field, with how to create an API key and which certificate formats are accepted: [guide, §3](INSTALL.md#3-first-connection).

## Configuration

No configuration is needed to get started: a connection screen lets you enter a cluster's URL and credentials directly, and `~/.config/termdevtools/config.yaml` is created automatically, every setting documented in place. A sample is provided for reference in `config.yaml.example` — **it never contains a secret**: passwords, API key secrets, and passphrases are always re-requested on connection, never written to disk (see [Security](#security)). Every setting is described in the [guide, §5](INSTALL.md#5-settings-configyaml).

The cluster's distribution and version are detected at every connection and shown in the status bar (`ES 9.5.4`, `OS 2.19.6`). When they can't be — a proxy hiding the cluster's answer, or an OpenSearch in compatibility mode, which reports a fake version — everything that might apply is offered rather than hidden; `distribution:` and `version:` on a cluster's entry in `config.yaml` then let you state them yourself.

The interface language (English by default, or French) is set via `language: en` / `language: fr` in that same `config.yaml` — or switched live from the app with `F3`, which saves the choice for next time.

Mouse support (click to focus a field or select a list entry) is **off by default** — set `mouse: true` in `config.yaml` to enable it. Every mouse interaction has a full keyboard equivalent (see [Keyboard shortcuts](#keyboard-shortcuts)); leaving it off keeps the terminal's own native text selection/copy/paste available, since enabling it captures mouse events for the app instead (`F2` still copies the result either way).

## Recipes and reference data

Everything TermDevTools suggests — the recipes of the catalog (`F8`), the endpoints and `_cat` columns of auto-completion — is built into the binary, each entry tagged with the clusters it applies to. Once connected, you only see what works on that cluster: an Elasticsearch 8 gets the ILM recipes, an OpenSearch 2 the ISM ones.

**Adding your own.** Your recipes go into `~/.config/termdevtools/recipes/`, as plain text files; a commented template, `my-recipes.txt`, is created there on first launch. A recipe file is written like the editor's content, with a few directives in comments:

```
# @group Snapshots
# @recipe Nightly snapshots
# @tags backup
# Shown in the catalog's preview.
GET _snapshot/nightly/_all
```

- `@group` files the recipes under a theme — an existing one puts them with the built-in recipes on that theme.
- `@recipe` starts a recipe. With the group and title of a built-in recipe, it replaces it.
- `@tags` adds words for the filter to match.
- `@es >=8.7` or `@opensearch >=2.4 <3.0` restricts a recipe to some clusters (everywhere without them).

Your recipes are *added* to the built-in ones, never a replacement for the whole catalog, and are marked as yours in the list. `~/.config/termdevtools/endpoints.txt` (one endpoint per line, same optional tags) extends auto-completion the same way. A `recipes/` directory and an `endpoints.txt` next to the binary do the same for everyone sharing that installation. After editing any of them, `F7` reloads without restarting; a mistake in a file is reported in the status bar, with the file and line.

**Reading what is built in.** `termdevtools --export-defaults <directory>` writes the built-in recipes and endpoints out as files — to read them, or as a starting point for your own. It never overwrites an existing file, and refuses the two directories above: exported there, the built-in recipes would be read back as yours and hide those of later versions.

**Keeping up with new versions.** The built-in data covers the versions known when the binary was built; a newer cluster gets what applied to the latest known version. `_cat` columns don't depend on it at all: they are asked from the cluster.

## Reusable variables

Reference a `${name}` placeholder anywhere in a request's URL or JSON body, and it's substituted with a real value just before the request is sent (`Ctrl+E`) or a `curl` command is generated for it (`F9`) — the placeholder itself is what gets saved with your requests, not the resolved value. An undefined variable stops the action with a clear error instead of sending `${name}` to the cluster literally.

Values live in `~/.config/termdevtools/variables_<cluster>.txt` — one per cluster and per user, same scheme as the request save (`Ctrl+S`) — as plain `name=value` lines (`#` for comments). There's no in-app editor for it: open the file directly in your usual text editor, then press `F7` to pick up the change without restarting. The file is created automatically, with an explanatory comment, the first time you connect to a given cluster.

## Keyboard shortcuts

| Action | Key |
|---|---|
| Execute the request under the cursor | `Ctrl+E` [^enter] |
| Switch focus left ↔ right panel | `Ctrl+←` / `Ctrl+→` [^focus] |
| Quit (auto-saves the left panel) | `Ctrl+C` |
| Search in requests / in the result | `Ctrl+F` (depending on focused panel) |
| Resize the left/right split | `F5` / `F6` [^resize] |
| Save (left) / export (right) | `Ctrl+S` (depending on focused panel) |
| Open the recipe catalog | `F8` — type to filter, arrows to move, `Enter` to insert, `Esc` to close |
| Complete an endpoint / a column | `Tab`, `F10`, or `Ctrl+Space` (left panel) [^tab] |
| Reformat (indent) the JSON body under the cursor | `F4` (left panel) |
| Copy the request under the cursor as a `curl` command | `F9` (left panel) — secrets redacted, see [Security](#security) |
| Reload your hand-edited files (variables, recipes, endpoints) | `F7` — see [Recipes and reference data](#recipes-and-reference-data) |
| Copy the result to the clipboard | `F2` |
| Switch the interface language (fr/en) | `F3` |
| Help | `F1` (`Esc` to close) |

[^enter]: `Ctrl+Enter` also works on terminals that report it as distinct from plain `Enter` — many don't, which is why `Ctrl+E` is the primary, always-reliable shortcut.
[^focus]: On macOS, `Option`/`Alt` also works instead of `Ctrl` — `Ctrl+←/→` is intercepted by the system by default (Mission Control desktop switching).
[^resize]: `Ctrl+Shift+←/→` (`Option`/`Alt` included, on macOS) also works on terminals that report it distinctly from an unmodified arrow key — not all do, which is why `F5`/`F6` are the primary, always-reliable shortcuts.
[^tab]: On some terminals (notably PuTTY) `Tab` is swallowed entirely — no key event reaches the app at all. `F10` is a guaranteed-reliable alternative. **PuTTY is generally discouraged**: no issues on native macOS/Windows 10+ terminals.

## Security

**What the program does, and doesn't**

- **It only talks to the cluster you chose.** No telemetry, no update check, no external command run.
- **No secret is ever written to disk**: password, API Key secret and private key passphrase are asked again on every connection. Only the URL, the authentication type and the non-sensitive identifiers (username, API key ID, certificate paths) are saved in `config.yaml`.
- **A URL carrying credentials is refused** (`https://user:password@host`): saved and displayed, it would have exposed the password.
- **TLS verified by default**, TLS 1.2 at least; server certificate verification is only turned off explicitly, connection by connection.
- **HTTP redirections are never followed**: your credentials don't leave for an address other than the one entered, and a request isn't replayed elsewhere. A redirection is displayed as it is.
- **Nothing displayed is interpreted by the terminal**: the control characters of a cluster's answer, of a file or of an error message never reach the screen.
- **Files for you alone**: permissions `0600` for files and `0700` for directories (on Linux and macOS), atomic writes — an interruption during a save doesn't destroy the previous content.
- **`F9` (copy as `curl`)** never includes the secrets: a placeholder to fill in stands in for them.
- **`F2` (clipboard)** goes through OSC 52: the data goes to the local terminal, with no server-side clipboard — but this mechanism doesn't confirm that the copy succeeded.

**What the program trusts**

- **The binary's directory and your configuration directory.** The recipes, endpoints and starting content found there are offered to the user: in a shared installation, the binary's directory must be writable by trusted people only. A recipe or endpoint file holding control characters is refused.
- **You.** A request is sent as it is written, with no confirmation — `DELETE` included.

**What was checked, and what wasn't**

- Security review of the code and automated tests for each of the points above; `govulncheck` reports no known vulnerability in the dependencies.
- Every request of every recipe was run against eleven real clusters (Elasticsearch 7.17 to 9.5, OpenSearch 2.0 to 3.9).
- **No independent external audit was carried out**, and the binaries are not signed: check their SHA-256 checksum. See also the [disclaimer](#disclaimer--limitation-of-liability) below.

## License

This project is distributed under the **[GNU Affero General Public License v3.0](LICENSE)** (AGPLv3): you're free to use, study, modify, and redistribute it, provided the source code (including your modifications) remains available under the same terms — including when the tool is exposed over a network (service mode usage).

> **Note of intent (not legally binding)**: the spirit of this project is to remain a community tool, improved collectively, not a product resold as-is. The AGPLv3 does not formally forbid commercial use — only a non-commercial license would, at the cost of heavier and less "open source" restrictions — but that's the use its author hopes to see made of it.

## Disclaimer / limitation of liability

TermDevTools is published **as is**, without warranty of any kind, express or implied — including, without limitation, the warranties of merchantability, fitness for a particular purpose, and non-infringement (see sections 15 through 17 of the [AGPLv3 license](LICENSE), which govern).

In particular:

- This project is developed and maintained **on its author's free time**, with no commitment to availability, maintenance, security patches, or future evolution.
- The author and contributors **decline any responsibility** for the direct or indirect consequences of using this tool — including, without limitation, data loss, service interruption, or any action executed against an Elasticsearch or OpenSearch cluster through this tool (TermDevTools executes requests exactly as you write them, with no confirmation beyond what is described in [SPEC.md](SPEC.md)).
- Using this tool against a production cluster remains **the sole responsibility of the person using it**: always review your requests, particularly destructive operations (`DELETE`, mapping updates, etc.), just as you would with any Elasticsearch client (Kibana, `curl`, or otherwise).
- Future evolutions of the project (or the lack thereof) are the responsibility only of whoever makes them, at the time they are made.
