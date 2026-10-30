*(Version française : [README_fr.md](README_fr.md))*

# TermDevTools

[![CI](https://github.com/gnocent/termdevtools/actions/workflows/ci.yml/badge.svg)](https://github.com/gnocent/termdevtools/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/gnocent/termdevtools?include_prereleases)](https://github.com/gnocent/termdevtools/releases)
[![License](https://img.shields.io/github/license/gnocent/termdevtools)](LICENSE)

Kibana's **DevTools** view in a terminal, to query an Elasticsearch or OpenSearch cluster when Kibana isn't there — or is down, in the middle of an investigation. Linux (including RHEL 8/9/10), Windows or macOS; a single binary, nothing else to install.

> [!IMPORTANT]
> **What's new, release by release** — details and known limits in the [changelog](CHANGELOG.md)
>
> - **0.9.0** — a new look: fixed colors, a header with the product and the cluster, methods colored in the editor, a `▼` when there is more to scroll to.
> - **0.8.1** — `F1` gives the tool's version and the cluster's; `F2` and `F9` copy in any terminal on a Mac.
> - **0.8.0** — several clusters in one session: `Ctrl+O` switches between them, each found again as you left it.
> - **0.7.5** — `Esc` abandons the request in progress; the status bar counts the seconds of the wait.
> - **0.7.4** — a cluster's secret is read from an environment variable or a file (`secret_env`, `secret_file`).
> - **0.7.3** — production guardrails: requests that write are confirmed or refused, behind a banner.
> - **0.7.2** — installs with `go install`; completion keeps the query after the cursor.
> - **0.7.1** — an incomplete request line no longer runs the one above it; an http connection fixed on Linux.
> - **0.7** — HTTP and SOCKS5 proxies, API keys the way Kibana gives them, Bearer tokens.
> - **0.6** — nothing but the binary; Elasticsearch and OpenSearch recognized with their version; recipe catalog (`F8`).

## Demo

<p align="center"><img src="demotermdevtools.gif" alt="Animated demo: connecting to a production cluster whose password is read from an environment variable, a request, completing an endpoint and _cat columns, a recipe from the catalog, a search reformatted then searched, switching to an OpenSearch and back, then a DELETE on the production cluster stopped by the confirmation"></p>

> [!NOTE]
> **How this project is made.** I'm not a developer: I write the specifications ([SPEC.md](SPEC.md)) and check the result; the code is written by [Claude Code](https://claude.com/claude-code), pure *vibe coding*, sometimes with the help of contributors. What makes up for it: about 400 automated tests, including the interface driven on a simulated screen; CI on Linux, Windows and macOS for every pull request; every recipe run against eleven real clusters; an independent security review before the releases that touch secrets or protections (0.7.3, 0.7.4, 0.8.0). This is a **beta**, still used by few people: what was checked, and what wasn't, is in [Security](#security).

## Features

- **Two panels**: requests on the left, Kibana Console style (`METHOD endpoint` + JSON body), the formatted result on the right, with a reminder of the request and the response headers.
- **Execute at cursor** (`Ctrl+E`): several requests live in the editor, the one under the cursor is sent.
- **Recipe catalog** (`F8`): about a hundred ready-made requests for the usual investigations, filtered as you type; add your own.
- **Adapts to the cluster**: Elasticsearch 7.17 to 9.x, OpenSearch 2.x and 3.x, detected at connection; only what exists on that cluster is offered.
- **Auto-completion** (`Tab`) of endpoints and of `_cat` columns (`h=`, `s=`), the latter asked from the cluster itself.
- **Several clusters in one session** (`Ctrl+O`): a request in progress carries on off screen, the status bar says when its answer comes.
- **Production guardrails**, cluster by cluster: writes confirmed or refused, a red or yellow header with a label such as `PROD`.
- **Readable at a glance**: a header with the product, the URL and the user, fixed colors whatever the terminal, colored methods (`DELETE` in red), a `▼` when there is more to scroll to.
- **Connection**: Basic Auth, API key, Bearer token or client certificate (mTLS), through an HTTP or SOCKS5 proxy; the secret typed, or read from a variable or a file, never saved.
- **Day to day**: `${name}` variables, JSON reformatting (`F4`), copy as a `curl` command (`F9`, secrets redacted), search (`Ctrl+F`), export (`Ctrl+S`), clipboard copy over SSH (`F2`), requests saved automatically per cluster.

Every choice in detail: [SPEC.md](SPEC.md).

## Installation

One file per platform, and nothing else, on the [Releases](https://github.com/gnocent/termdevtools/releases) page:

| Platform | File |
|---|---|
| Linux (x86-64) | `termdevtools-linux-amd64` |
| Windows (x86-64) | `termdevtools-windows-amd64.exe` |
| macOS (Apple Silicon) | `termdevtools-darwin-arm64` |

```bash
sha256sum -c SHA256SUMS --ignore-missing     # checks the downloaded file
chmod +x termdevtools-linux-amd64
mv termdevtools-linux-amd64 ~/.local/bin/termdevtools
```

Or, with [Go](https://go.dev/) 1.25 or later: `go install github.com/gnocent/termdevtools@latest`.

The binary is static: it can be copied as is, including onto a machine with no Internet access. It isn't signed; macOS and Windows may point it out on first launch. Platform by platform, install scripts, shared installation and upgrading from an earlier version: the **[installation guide](INSTALL.md)**.

## Quick start

1. **Launch** `termdevtools`. The connection screen lists the clusters already used and "+ New connection".
2. **Connect**: URL, protection if it's production, authentication, secret. Everything but the secret is remembered for next time.
3. **Write a request** on the left, `GET _cluster/health` for instance, and **run it** with `Ctrl+E`.
4. **Or pick a recipe**: `F8`, a few words (`unassigned`, `disk`, `snapshot`…), `Enter`, then `Ctrl+E`.
5. **Switch to another cluster**: `Ctrl+O`. The first one stays open, as it was.

Step by step, field by field: [guide, §3](INSTALL.md#3-first-connection).

## Configuration

Nothing to prepare: `~/.config/termdevtools/config.yaml` is created on first launch, every setting documented in place, and remembers the clusters used — never their secret.

- **Secret without retyping**: `secret_env: ES_PW` or `secret_file: ~/.es-prod.secret` on a cluster's entry.
- **Proxy**: `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY`, or `proxy:` per cluster (`http://…`, `socks5://…`, `none`).
- **Language** (`language: fr`, or `F3`) and **mouse** (`mouse: true`, off by default).
- **Your files**, in `~/.config/termdevtools/`: requests and `${name}` variables per cluster, your recipes (`recipes/`) and endpoints, reloaded with `F7`; `termdevtools --export-defaults <directory>` writes out the built-in ones.

Every setting and every file: [guide, §5 and §6](INSTALL.md#5-settings-configyaml).

## Keyboard shortcuts

| Action | Key |
|---|---|
| Execute the request under the cursor | `Ctrl+E` |
| Abandon the request in progress | `Esc` |
| Open another connection, go back to an open one | `Ctrl+O` (`Esc` to go back) |
| Switch panel | `Ctrl+←` / `Ctrl+→` |
| Search (in the active panel) | `Ctrl+F` |
| Save (left) / export (right) | `Ctrl+S` |
| Recipe catalog | `F8` |
| Complete an endpoint or a column | `Tab`, `F10` or `Ctrl+Space` |
| Reformat the JSON under the cursor | `F4` |
| Copy the request as a `curl` command | `F9` |
| Reload your files (variables, recipes, endpoints) | `F7` |
| Copy the result to the clipboard | `F2` |
| Resize the panels | `F5` / `F6` |
| Interface language (fr/en) | `F3` |
| Help | `F1` |
| Quit (saves every open connection) | `Ctrl+C` |

Depending on the terminal, some combinations don't get through: `Ctrl+Enter` (use `Ctrl+E`), `Ctrl+←/→` on macOS and PuTTY (`Alt+←/→`), `Ctrl+Shift+←/→` (`F5`/`F6`), `Tab` on PuTTY (`F10`). **PuTTY is discouraged**; no issue with the native terminals of macOS and Windows 10+.

## Security

**What the program does, and doesn't**

- **It only talks to the cluster you chose**, through a proxy only if your environment or `config.yaml` designates one, and showing it. No telemetry, no update check, no external command.
- **No secret is ever written to disk**: it is typed on every connection, or read from the variable or the file you name — that file is yours to protect. A URL carrying credentials is refused.
- **TLS verified by default** (TLS 1.2 at least); redirections are never followed.
- **Nothing displayed is interpreted by the terminal**, whatever the cluster answers.
- **Files for you alone** (`0600`, directories `0700`, on Linux and macOS), written atomically; `F9` never includes the secrets.

**What it trusts**

- **The binary's directory and your configuration directory**: their recipes and endpoints are offered as they are.
- **The proxy**: in https, it only sees the address; in http, it sees everything, credentials included.
- **You**: without protection, a request is sent as it is written, `DELETE` included. The protection saves you from a slip, it doesn't stop someone who means to write; to really prevent it, use an account with no write privilege.

**What was checked, and what wasn't**

- Security review and automated tests for each of the points above; `govulncheck` reports no known vulnerability (dependencies and Go 1.27.3, which builds the binaries).
- Against real clusters: every recipe (Elasticsearch 7.17 to 9.5, OpenSearch 2.0 to 3.9), authentication by password, API key and Bearer token, the protection; switching between clusters, by a contributor, on lab instances.
- The proxy is only checked with test servers, not with a real corporate proxy.
- **No independent external audit**, and unsigned binaries: check their SHA-256 checksum. See also the [disclaimer](#disclaimer--limitation-of-liability) below.

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
