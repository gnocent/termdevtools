*(Version française : [README_fr.md](README_fr.md))*

# TermDevTools

A terminal-mode simulator of Kibana's **DevTools** view, for querying an Elasticsearch cluster directly from a Linux terminal (RHEL 8/9/10), without a browser or a working Kibana.

## Why

Sometimes an Elasticsearch cluster has no Kibana available, or its Kibana is down — typically during an investigation, which is exactly when you'd need it most. Doing the equivalent by hand with `curl` is possible but tedious (TLS handling, multi-line requests, formatting the JSON response...). TermDevTools reproduces most of the comfort of Kibana's DevTools — a request editor, execute-at-cursor, formatted JSON responses — in a single terminal binary.

## Features

- **Two-panel interface**: request editor on the left (`METHOD endpoint` + optional JSON body), formatted JSON result on the right.
- **Execute at cursor** (`Ctrl+Enter`): several requests can coexist in the editor, separated by blank lines; the one under the cursor is executed.
- **Auto-completion** (`Tab`) for endpoints (`_cat/*`, `_cluster/*`, `_nodes/*`, index management, ILM/SLM, snapshots, license...) and, for `_cat/*` commands, for the column names of the `h=`/`s=` parameters. Lists are customizable without recompiling via `endpoints.txt` and `cat_columns.txt`.
- **Search** (`Ctrl+F`) in the editor as well as in the result.
- **Automatic save** of in-progress requests per cluster and per user (on exit and via `Ctrl+S`), reloaded on reconnection.
- **Export** of the displayed result to a timestamped file (`Ctrl+S`, right panel) and **clipboard copy** via OSC 52 (`F2`, works over SSH).
- **Connection**: Basic Auth, API Key, or client certificate (mTLS), with or without TLS verification; history of previously used clusters (never storing a secret there — see [Security](#security)).
- **Built-in help** (`F1`): reminder of shortcuts and file locations.

Full detail of design choices and behavior: [SPEC.md](SPEC.md).

## Installation

### Prebuilt binaries

Static binaries are provided for Linux (amd64), Windows (amd64), and macOS (Apple Silicon / arm64) — see the repository's [Releases](../../releases) section. No dependency to install: just download the binary for your platform and make it executable (`chmod +x` on Linux/macOS).

### Building from source

Requires [Go](https://go.dev/) 1.25 or later.

```bash
git clone <repo-url>
cd TermDevTools
go build -o termdevtools .
```

The binary is static (`CGO_ENABLED=0`): it needs no system library beyond the base libc, and can be copied as-is onto any RHEL 8/9/10 machine (or any other Linux amd64 distribution), with no installation step.

The [`build-release.sh`](build-release.sh) script builds all three target platforms (`linux/amd64`, `windows/amd64`, `darwin/arm64`) and bundles each binary with its companion files under `dist/<platform>/`.

## Configuration and companion files

On first launch, no configuration is needed: a connection screen lets you enter a cluster's URL and credentials directly. The following files are then read **if they exist**, next to the binary:

| File | Purpose |
|---|---|
| `cheatsheet.txt` | Default editor content on first launch against a given cluster (copy `cheatsheet.txt.example`). |
| `endpoints.txt` | List of endpoints offered for auto-completion (replaces the built-in list). |
| `cat_columns.txt` | `_cat/*` command → columns table, for auto-completion of the `h=`/`s=` parameters. |

A sample user configuration (`~/.config/termdevtools/config.yaml`, generated automatically on first connection) is provided for reference in `config.yaml.example` — **it never contains a secret**: passwords, API key secrets, and passphrases are always re-requested on connection, never written to disk (see [Security](#security)).

The interface language (French by default, or English) is set via `language: fr` / `language: en` in that same `config.yaml`.

## Keyboard shortcuts

| Action | Key |
|---|---|
| Execute the request under the cursor | `Ctrl+Enter` |
| Switch focus left ↔ right panel | `Ctrl+←` / `Ctrl+→` |
| Quit (auto-saves the left panel) | `Ctrl+C` |
| Search in requests / in the result | `Ctrl+F` (depending on focused panel) |
| Resize the left/right split | `Ctrl+Shift+←` / `Ctrl+Shift+→` |
| Save (left) / export (right) | `Ctrl+S` (depending on focused panel) |
| Complete an endpoint / a column | `Tab` (left panel) |
| Copy the result to the clipboard | `F2` |
| Help | `F1` (`Esc` to close) |

## Security

- **TLS verified by default**: server certificate verification is enabled unless explicitly disabled when connecting.
- **No secret ever persisted**: password, API Key secret, and private key passphrase are never written to `config.yaml` — only the URL, auth type, and non-sensitive identifiers (username, API key ID, certificate paths) are, with restricted permissions (`0600` for files, `0700` for directories).
- **Clipboard copy (`F2`)** via OSC 52: the local terminal receives the data to copy without it ever passing through a server-side clipboard — but this mechanism gives no guarantee of success (depends on the terminal in use).
- The project went through a security review (code review, no execution of external commands, `govulncheck` with no known exploitable vulnerability) before publication — see also the [disclaimer](#disclaimer--limitation-of-liability) below.

## License

This project is distributed under the **[GNU Affero General Public License v3.0](LICENSE)** (AGPLv3): you're free to use, study, modify, and redistribute it, provided the source code (including your modifications) remains available under the same terms — including when the tool is exposed over a network (service mode usage).

> **Note of intent (not legally binding)**: the spirit of this project is to remain a community tool, improved collectively, not a product resold as-is. The AGPLv3 does not formally forbid commercial use — only a non-commercial license would, at the cost of heavier and less "open source" restrictions — but that's the use its author hopes to see made of it.

## Disclaimer / limitation of liability

TermDevTools is published **as is**, without warranty of any kind, express or implied — including, without limitation, the warranties of merchantability, fitness for a particular purpose, and non-infringement (see sections 15 through 17 of the [AGPLv3 license](LICENSE), which govern).

In particular:

- This project is developed and maintained **on its author's free time**, with no commitment to availability, maintenance, security patches, or future evolution.
- The author and contributors **decline any responsibility** for the direct or indirect consequences of using this tool — including, without limitation, data loss, service interruption, or any action executed against an Elasticsearch cluster through this tool (TermDevTools executes requests exactly as you write them, with no confirmation beyond what is described in [SPEC.md](SPEC.md)).
- Using this tool against a production cluster remains **the sole responsibility of the person using it**: always review your requests, particularly destructive operations (`DELETE`, mapping updates, etc.), just as you would with any Elasticsearch client (Kibana, `curl`, or otherwise).
- Future evolutions of the project (or the lack thereof) are the responsibility only of whoever makes them, at the time they are made.
