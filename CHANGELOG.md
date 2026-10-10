*(Version française : [CHANGELOG_fr.md](CHANGELOG_fr.md))*

# Changelog

## 0.7 (beta) — October 2026

**In one sentence**: TermDevTools reaches a cluster through a proxy, takes API keys the way Kibana gives them as well as Bearer tokens, and no longer writes anything next to its binary.

**Why "beta"**: like 0.6, this version has only been used by its author so far. Going through a proxy was checked with test proxies, not with a real corporate proxy. Feedback is welcome.

### What's new

- **HTTP and SOCKS5 proxies**: a cluster is reached through the proxy the `HTTPS_PROXY` / `HTTP_PROXY` environment variables designate, unless `NO_PROXY` excludes it. For one cluster in particular, `proxy:` on its entry in `config.yaml` takes precedence: a proxy URL (`http://…`, or `socks5://…` — what `ssh -D` opens), or `none` for a direct connection. `F9` writes it into the curl command when `config.yaml` sets it.
- **API key in its `encoded` form**: the key as Kibana shows it when created is pasted straight into the secret field, with no identifier. The identifier is taken from the key and saved. Entering the identifier and the secret works as before.
- **Bearer token**: a new authentication type, for a service account token, an access token or a JWT. Like any secret, it is asked again on every connection and never saved.
- **Exports go to your configuration directory**: `Ctrl+S` on the result writes to `~/.config/termdevtools/exports/`, no longer next to the binary. Exporting therefore also works in a shared or read-only installation, and each user has their own. The status bar still shows the path of the file written.
- **The crash report** (`crash-<date>.log`) is written to the same place: the program no longer writes anything to its binary's directory.
- **Messages of the connection screen on three lines**: a long error is no longer cut after the first.

### Security

- **A proxy is never used without being shown**: while connecting, at the head of the message if the connection fails, and in the status bar once connected. Its credentials, for their part, are never displayed nor copied, and are sent to it alone.
- **A proxy reached over TLS (`https://proxy…`) is refused**: the cluster's TLS settings — its client certificate, a verification turned off — would apply to the proxy as well.
- **Nothing secret in `config.yaml`**, as before: credentials in `proxy:` are refused, and a Bearer token is never saved.
- **An API key secret with no identifier is refused before anything is sent**, with a message saying what to enter.
- **Binaries built with Go 1.27.2.** That version of Go fixes flaws of its standard library published since 0.6, two of which concern a client like this one: the desynchronization of an HTTP/1 connection after a proxy rejects a `CONNECT` tunnel (GO-2026-6605) and the bypass of a memory limit when parsing headers (GO-2026-6608). `go.mod` now asks for that version.

### Before upgrading from 0.6

- **Proxy**: 0.6 ignored `HTTPS_PROXY` and `HTTP_PROXY`. If either is set on your machine, a cluster reached directly so far will go through that proxy. If it must not, add it to `NO_PROXY`, or set `proxy: none` on its entry in `config.yaml`. On failure, the message starts with the proxy gone through and recalls both remedies.
- **Exports**: an `exports/` directory left next to the binary is neither moved nor deleted; take your files from it if you need them.
- **API keys**: nothing to do. Existing entries keep their identifier; only the label of the secret field changes.

### Checks

- Password, API key (in both forms) and Bearer token: checked against a real Elasticsearch 9.5.4 with security on — accepted with the right secret, turned down with a wrong one.
- Proxy: checked with a test HTTP proxy and a test SOCKS5 server — a tunnel to a cluster in https with its certificate verified end to end, each side's credentials kept to itself.
- `govulncheck`: no known vulnerability, neither in the dependencies nor in the standard library of Go 1.27.2. With Go 1.27.1, it reported eight in the standard library.
- The reference data (recipes, endpoints, `_cat` columns) has not changed since 0.6: its checks against eleven real clusters are those of 0.6 and were not run again.
- No independent external audit was carried out.

### Known limits

- OpenSearch managed by AWS with IAM authentication (SigV4) is not supported.
- Proxy: its credentials are only given through the environment variable, with Basic authentication; NTLM, Kerberos, PAC files and Windows' proxy settings are not supported. No real corporate proxy was tried.
- Bearer token: only an Elasticsearch service account token was tried; Elasticsearch and OpenSearch JWTs were not. The program doesn't renew an expired token.
- The demo animation of the README was recorded with 0.5: it shows neither the recipe catalog nor these additions.

## 0.6 (beta) — October 2026

**In one sentence**: TermDevTools now needs nothing but its binary, recognizes Elasticsearch and OpenSearch and their version, and offers a catalog of ready-made requests suited to the cluster.

**Why "beta"**: the recipe catalog, OpenSearch support and the selection by version are new. They were checked against eleven real clusters (see below), but not yet by users other than their author. Feedback is welcome.

### What's new

- **Recipe catalog (`F8`)**: about a hundred ready-made requests, filed by theme — overview, shards and allocation, nodes and resources, indices, tasks, snapshots, index lifecycle, upgrade and maintenance, search. Filter by typing, preview, `Enter` inserts the recipe at the end of the editor, cursor on its request. Recipes are written in English.
- **Elasticsearch and OpenSearch, by version**: the distribution and version of the cluster are detected at connection and shown in the status bar (`ES 9.5.4`, `OS 2.19.6`). Only the recipes and endpoints that exist on that cluster are offered. Coverage: Elasticsearch 7.17 to 9.x, self-managed OpenSearch 2.x and 3.x.
- **No file to install next to the binary anymore**: recipes, endpoints and `_cat` columns are built in. Releases are plain binaries, with their SHA-256 checksums.
- **Your own recipes and endpoints**: plain text files in `~/.config/termdevtools/` (or next to the binary, to share them), added to the built-in ones without replacing them. `F7` reloads them; a mistake in a file is reported with the file and the line.
- **`_cat` columns asked from the cluster**: completion of the `h=` and `s=` parameters queries the cluster itself, so it is exact whatever its version.
- **Client keys encrypted in the PKCS#8 format** (mTLS): the format OpenSSL writes by default is now read, in addition to the older encrypted PEM format.
- **`termdevtools --version`** and **`termdevtools --export-defaults <directory>`** (writes the built-in recipes and endpoints out as files, to read them or start from them).

### Security and reliability

- **HTTP redirections are no longer followed.** A redirection is displayed as it is. Previously, a `302` answer silently turned a `DELETE` or a `POST` into a `GET` of the new address.
- **A URL carrying credentials is refused** (`https://user:password@host`): the URL is saved in `config.yaml`, where the password would have ended up.
- **`Ctrl+C` always saves before quitting**, including when the help, the search bar or a list is open. If the save fails, the program says so and waits for a second `Ctrl+C`.
- **Atomic saves**: a full disk or an interruption during a write no longer destroys the previous content (requests, `config.yaml`).
- **`config.yaml` keeps its comments** after every connection, as well as the keys this version doesn't know and a default directory deliberately left empty.
- **A response larger than 64 MB is no longer loaded**: the request is reported as failed, with the advice to narrow it down.
- **Fixes**: crash of the search in the result after a shorter result; a slow connection given up on can no longer replace the session opened since; only the answer to the last request sent is displayed; with the mouse enabled, the focus follows clicks.

### Before upgrading from 0.5

- **Companion files**: `cat_columns.txt` is no longer read and can be deleted. `endpoints.txt`, if left next to the binary, is read as an *addition* to the built-in list: delete it unless you had added endpoints of your own to it. `cheatsheet.txt` keeps its role; delete it to get the built-in starting content.
- **Starting content of the editor**: a few universal requests instead of a long cheatsheet, whose content is now in the catalog (`F8`). Requests you already saved don't change.
- **An `http://` address that redirected to `https://`**: the connection now fails, naming the new address; correct the URL.
- **Credentials in the URL**: such an entry in `config.yaml` is refused at connection; remove them from the URL and choose Basic Auth.
- **`F7`** now reloads all your files (variables, recipes, endpoints), not only the variables.
- **Interface in English by default**: the program starts in English as long as no language was chosen; `F3` switches to French and remembers that choice. This also applies to an existing installation where `F3` was never used (0.5 didn't durably record the language until it had been changed): press `F3` once. A language already chosen with `F3` is kept.
- **Generated files are in English**: the comments of a new `config.yaml` and of a new variables file are in English. Existing files are not rewritten.

### Checks

- Detection, endpoints, `_cat` columns and **every request of every recipe** run against eleven real clusters: Elasticsearch 7.17.29, 8.0.1, 8.11.4, 8.19.22, 9.0.8, 9.5.4; OpenSearch 2.0.1, 2.11.1, 2.19.6, 3.0.0, 3.9.0.
- PKCS#8 keys: checked against keys written by OpenSSL and through a full mTLS connection.
- `govulncheck`: no known vulnerability in the dependencies.

### Known limits

- OpenSearch managed by AWS with IAM authentication (SigV4) is not supported.
- No HTTP(S) proxy: `HTTPS_PROXY` is not honored.
- API keys are entered as identifier + secret, not in their `encoded` form.
- Exports (`Ctrl+S` on the result) are written next to the binary: in a shared or read-only installation, exporting fails.
- The demo animation of the README was recorded with 0.5: it doesn't show the recipe catalog.

## 0.5 — 24 August 2026

- Reusable `${name}` variables, per cluster, reloaded with `F7`.
- Line numbers in the editor, automatic closing of braces, brackets and quotes.
- File picker for the certificates of the connection screen.

## 0.4 — 14 August 2026

- `F10` as an alternative to `Tab` for completion, on terminals that intercept `Tab`.
- Mouse off by default.
- Crash report written to a file.
- Improved completion, reminder of the request at the top of the result.
- Fixed an offset in search, completion and request targeting after accented characters.

## 0.3 — 13 August 2026

- Install scripts (`install.sh`, `install.ps1`).
- On macOS, `Option`/`Alt` accepted in place of `Ctrl` for three shortcuts.

## 0.2 — 13 August 2026

- Interface in French or English, `F3` to switch language.

## 0.1 — 12 August 2026

- First published version: request editor, execution of the request under the cursor, formatted JSON result.
