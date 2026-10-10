*(Version française : [INSTALL_fr.md](INSTALL_fr.md))*

# Installing and configuring TermDevTools

This guide goes from the download to the first request, then details every setting. For an overview of the product, see the [README](README.md).

- [1. What you need](#1-what-you-need)
- [2. Installing the binary](#2-installing-the-binary)
- [3. First connection](#3-first-connection)
- [4. First steps in the interface](#4-first-steps-in-the-interface)
- [5. Settings: `config.yaml`](#5-settings-configyaml)
- [6. Your files: requests, variables, recipes, endpoints](#6-your-files-requests-variables-recipes-endpoints)
- [7. Shared installation](#7-shared-installation)
- [8. Upgrading](#8-upgrading)
- [9. Uninstalling](#9-uninstalling)
- [10. Troubleshooting](#10-troubleshooting)

## 1. What you need

- **A single file**: the `termdevtools` binary. No dependency, no file to put next to it, no Internet access at run time.
- **A terminal of at least 80 columns by 24 rows.** Recommended: Windows Terminal, macOS Terminal or iTerm2, any Linux terminal, including over SSH. PuTTY is discouraged (it passes several shortcuts on badly).
- **Network access to the cluster** from the machine TermDevTools runs on, and an account allowed at least to read the cluster's root (`GET /`): that is the request that validates the connection.

Supported clusters: Elasticsearch 7.17 to 9.x, self-managed OpenSearch 2.x and 3.x.

## 2. Installing the binary

Binaries are published on the [Releases](https://github.com/gnocent/termdevtools/releases) page:

| Platform | File |
|---|---|
| Linux (x86-64) | `termdevtools-linux-amd64` |
| Windows (x86-64) | `termdevtools-windows-amd64.exe` |
| macOS (Apple Silicon) | `termdevtools-darwin-arm64` |
| Checksums | `SHA256SUMS` |

### 2.1 Linux

1. Download `termdevtools-linux-amd64` and `SHA256SUMS` into the same directory.
2. Check the file:
   ```bash
   sha256sum -c SHA256SUMS --ignore-missing
   ```
   The line `termdevtools-linux-amd64: OK` must be displayed.
3. Make it executable and put it in a directory of your `PATH`:
   ```bash
   chmod +x termdevtools-linux-amd64
   mkdir -p ~/.local/bin
   mv termdevtools-linux-amd64 ~/.local/bin/termdevtools
   ```
4. Check:
   ```bash
   termdevtools --version
   ```
   If the command isn't found, `~/.local/bin` is not in your `PATH`: add `export PATH="$HOME/.local/bin:$PATH"` to your `~/.bashrc`, or run the binary by its full path.

**Machine without Internet access**: download the two files elsewhere, transfer them (`scp`, removable media), then resume at step 2. Nothing else needs to be transferred.

### 2.2 macOS (Apple Silicon)

1. Download `termdevtools-darwin-arm64` and `SHA256SUMS`.
2. Check the file: the command below prints a digest, which must be identical to the one on the `termdevtools-darwin-arm64` line of `SHA256SUMS`.
   ```bash
   shasum -a 256 termdevtools-darwin-arm64
   ```
3. Make it executable and put it in your `PATH`:
   ```bash
   chmod +x termdevtools-darwin-arm64
   mkdir -p ~/.local/bin
   mv termdevtools-darwin-arm64 ~/.local/bin/termdevtools
   ```
4. The binary is not signed with an Apple developer account. If macOS refuses to run it because it was downloaded, remove the quarantine mark:
   ```bash
   xattr -d com.apple.quarantine ~/.local/bin/termdevtools
   ```
5. Check with `termdevtools --version`.

No binary is provided for Intel Macs: build from source (§2.4).

### 2.3 Windows

1. Download `termdevtools-windows-amd64.exe` and `SHA256SUMS`.
2. Check the file in PowerShell: the digest displayed must be the one on the matching line of `SHA256SUMS` (letter case doesn't matter).
   ```powershell
   Get-FileHash .\termdevtools-windows-amd64.exe -Algorithm SHA256
   ```
3. Rename it `termdevtools.exe` and put it in a directory of your own, for instance `%LOCALAPPDATA%\termdevtools\`.
4. Run it from **Windows Terminal** (or PowerShell) by its path, or add that directory to your `PATH`.
5. The binary is not signed: Windows may display a warning on first launch.

### 2.4 From source

[Go](https://go.dev/) 1.25 or later is required.

```bash
git clone https://github.com/gnocent/termdevtools.git
cd termdevtools
./install.sh        # Linux, macOS
```

```powershell
.\install.ps1       # Windows
```

The scripts build for your machine and install the binary into `~/.local/share/termdevtools` (linked into `~/.local/bin`) or `%LOCALAPPDATA%\termdevtools`. The `TERMDEVTOOLS_INSTALL_DIR` and `TERMDEVTOOLS_BIN_DIR` variables change those locations. To only build: `go build -o termdevtools .`

## 3. First connection

Run `termdevtools`. The interface is in English; once connected, `F3` switches it to French and remembers that choice.

1. **The connection screen** lists the clusters already used — none the first time — then **"+ New connection"** and **"Quit"**. Choose "+ New connection" with the arrow keys and `Enter`.
2. **URL**: the address of the cluster, scheme and port included, for instance `https://es.example.com:9200`. Don't put credentials in it (`https://user:password@…` is refused: the URL is saved).
3. **Authentication**: `Tab` to the field, `Enter` to open the list, then choose. The fields that follow adapt to that choice.

   | Choice | Fields to fill in |
   |---|---|
   | none | — |
   | Basic Auth | *Username*, *Password* |
   | API Key | *API Key ID*, then *Secret, or encoded key (no ID)*: the key's secret — or the whole key in its `encoded` form, in which case the identifier isn't needed |
   | Bearer token | *Token* |
   | client certificate (mTLS) | *Client certificate*, *Client private key*, *Key passphrase* if it is encrypted |

4. **TLS** (`https://` URLs only):
   - *CA file*: the certificate of the authority that signed the cluster's, if it isn't among the authorities your system knows. `Enter` on that field opens a file picker when a default directory is configured (§5); otherwise, type the path.
   - *Verify server certificate (TLS)*: checked by default. Only uncheck it for a test, on a trusted network.
5. `Tab` to **"Connect"**, then `Enter`. `Esc` or "Cancel" goes back to the list.

Once connected, the status bar tells what was recognized: `ES 9.5.4`, `OS 2.19.6`.

**What is remembered, and what is not.** The URL, the authentication type, the username, the API key identifier and the certificate paths are saved in `config.yaml`: on the next connection, that cluster is in the list and only the secret is asked for again. Passwords, API key secrets, tokens and passphrases are **never** written to disk.

### Creating an API key (Elasticsearch)

From a tool already connected to the cluster (TermDevTools with Basic Auth will do):

```
POST _security/api_key
{
  "name": "termdevtools"
}
```

The answer holds `id`, `api_key` and `encoded`. Two ways to enter it, whichever you prefer:

- `id` in *API Key ID* and `api_key` in the secret field;
- `encoded` alone in the secret field, *API Key ID* left empty. That is also the form Kibana shows when a key is created. The identifier is then taken from the key and saved for next time.

### Bearer token

Choose *Bearer token* when the cluster expects an `Authorization: Bearer …` header: an Elasticsearch service account token or access token, an Elasticsearch or OpenSearch JWT. Paste the token alone; if it was copied with the word `Bearer`, that word is removed.

The token is never saved: it is asked again on every connection, and TermDevTools doesn't renew it when it expires. For lasting access, prefer an API key.

### Client certificate (mTLS)

- The certificate and the key are expected in PEM format, in two files.
- A key protected by a passphrase is accepted in both common formats: `BEGIN ENCRYPTED PRIVATE KEY` (PKCS#8, what OpenSSL writes by default) and `BEGIN RSA PRIVATE KEY` with a `DEK-Info` header.
- `.p12`/`.pfx` files are not read: convert them to PEM with OpenSSL.

## 4. First steps in the interface

- **On the left**, the editor: one request per block, as in Kibana's console. The first time you connect to a cluster, it already holds a few requests that work anywhere.
  ```
  GET _cluster/health

  GET _cat/indices?v&s=store.size:desc
  ```
- **`Ctrl+E`** runs the request the cursor is in; the result shows up **on the right**.
- **`F8`** opens the recipe catalog: type a few words (`unassigned`, `disk`, `snapshot`…), choose with the arrow keys, `Enter` inserts the recipe at the end of the editor, ready to run.
- **`Tab`** (or `F10`) completes an endpoint being typed, and the columns of `_cat` commands after `h=` or `s=`.
- **`Ctrl+S`** saves your requests for this cluster; they are saved on quitting with **`Ctrl+C`** too.
- **`F1`** lists every shortcut.

## 5. Settings: `config.yaml`

The file is created on first launch, every setting with a comment explaining it.

| System | Location |
|---|---|
| Linux, macOS | `~/.config/termdevtools/config.yaml` |
| Windows | `%USERPROFILE%\.config\termdevtools\config.yaml` |

If the `XDG_CONFIG_HOME` variable is set, the directory is `$XDG_CONFIG_HOME/termdevtools/`.

| Setting | Default | Purpose |
|---|---|---|
| `default_timeout_seconds` | `120` | Maximum duration of a request, in seconds. |
| `language` | `en` | Interface language: `en` or `fr`. `F3` changes it and updates this line. |
| `mouse` | `false` | Enables the mouse (click to switch panel, to choose in a list). Left off, the terminal's own selection and copy-paste stay available. |
| `default_ca_dir` | `/etc/pki/tls/certs` on Linux, empty elsewhere | Directory offered for the *CA file* field and browsed by its picker. |
| `default_client_cert_dir` | same | The same for the client's certificate and key. |
| `clusters` | empty | The connection history, most recent first. Kept up to date by the program. |

For each cluster, two optional lines replace the automatic detection when it can't succeed — a proxy hiding the cluster's answer, or an OpenSearch in compatibility mode, which reports a fake version:

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

**Proxy.** With nothing set, a cluster is reached through the proxy the environment variables designate, as `curl` does:

| Variable | Purpose |
|---|---|
| `HTTPS_PROXY` | Proxy for clusters in `https://`. |
| `HTTP_PROXY` | Proxy for clusters in `http://`. |
| `NO_PROXY` | Names, domains or addresses to reach without a proxy, separated by commas: `es-internal.example.com,.intra.example.com,10.0.0.0/8`. |

`localhost` and `127.0.0.1` never go through the environment's proxy. On Windows, only these variables are read, not the system's proxy settings.

For one cluster in particular, the optional `proxy:` line takes precedence over the environment:

```yaml
clusters:
  - url: https://es-dmz.example.com:9200
    auth_type: basic
    username: admin
    proxy: http://proxy.example.com:3128   # or socks5://127.0.0.1:1080, or none
    tls:
      verify: true
```

| Value of `proxy:` | Effect |
|---|---|
| `http://host:port` | That HTTP proxy. A cluster in https goes through it in a tunnel: the proxy only sees the address asked for. |
| `socks5://host:port` | That SOCKS5 proxy — for instance `socks5://127.0.0.1:1080` after `ssh -D 1080 bastion`. |
| `none` | A direct connection, whatever the environment says. |

Three limits: a proxy reached over TLS (`https://proxy…`) is refused; a proxy asking for a password is declared in the environment variable (`HTTPS_PROXY=http://user:password@proxy:3128`), never in `config.yaml`; NTLM or Kerberos authentication and PAC files are not supported.

The proxy in use is displayed while connecting, then in the status bar. For a cluster that isn't in the history yet, write its entry by hand as above, or use the environment variables.

Edit the file with TermDevTools closed: the program rewrites it on every successful connection (it keeps your comments and the keys it doesn't know).

## 6. Your files: requests, variables, recipes, endpoints

All of them live in the configuration directory (§5), and none is required.

| File | Content | Created by |
|---|---|---|
| `queries_<cluster>.txt` | The content of the editor for that cluster. | `Ctrl+S`, and on quitting |
| `variables_<cluster>.txt` | Your `${name}` variables for that cluster, one per line: `index=my-index`. | the program, on the first connection |
| `recipes/*.txt` | Your recipes, added to the catalog. | the program creates a commented template, `my-recipes.txt` |
| `endpoints.txt` | Your endpoints, added to completion: one per line. | you |
| `exports/` | The exported results, one timestamped file per export. | `Ctrl+S` on the result |

**Variables.** A request may hold `${index}`: the value is taken from the variables file when the request runs. The recipes of the catalog use `index`, `node`, `repository`, `snapshot`, `field` and `task_id`. An undefined variable stops the request with a message naming it.

**Recipes.** A recipe file is written like the editor, with a few directives in comments:

```
# @group Snapshots
# @recipe Nightly snapshots
# @tags backup
# This comment is shown in the preview.
GET _snapshot/nightly/_all
```

- `@group`: the theme; an existing one files the recipe with the built-in ones.
- `@recipe`: the title; with the group and title of a built-in recipe, it replaces it.
- `@tags`: extra words for the filter.
- `@es >=8.7` or `@opensearch >=2.4 <3.0`: restricts the recipe to some clusters.

After any change, **`F7`** reloads without restarting. A mistake in a file is reported in the status bar, with the file and the line. Files must be UTF-8.

To read the built-in recipes and endpoints, or start from them:

```bash
termdevtools --export-defaults ~/termdevtools-defaults
```

## 7. Shared installation

Several people can use the same binary on a server: each keeps their own history, requests and variables in their own configuration directory.

What is put **next to the binary** applies to everyone:

| File | Effect |
|---|---|
| `recipes/*.txt` | Common recipes, added to everyone's catalog. |
| `endpoints.txt` | Common endpoints, added to completion. |
| `cheatsheet.txt` | Starting content of the editor, in place of the built-in one. |

**The binary's directory must be writable by trusted people only**: what is in it is offered to every user. It can stay read-only for the users: the program writes nothing there, and exports (`Ctrl+S` on the result) go to each user's own configuration directory (§6).

## 8. Upgrading

Replace the binary with the new one. Your files are left alone.

**From 0.5**, three files were installed next to the binary:

- `cat_columns.txt` is no longer read: delete it.
- `endpoints.txt` is read as an addition to the built-in list: delete it, unless you had added endpoints of your own to it.
- `cheatsheet.txt` still provides the editor's starting content: delete it to get the built-in one.

The interface now starts in English as long as no language was chosen: if it was in French, press `F3` once after connecting, the choice is remembered.

**From 0.6 or an earlier version**, exports were written to an `exports/` directory next to the binary. They now go to the configuration directory (§6). The old directory is neither moved nor deleted: take your files from it if you need them.

See also "Before upgrading" in the [changelog](CHANGELOG.md).

## 9. Uninstalling

Delete the binary and the configuration directory (§5): TermDevTools writes nothing anywhere else. A version up to 0.6 may have left, next to the binary, an `exports/` directory and `crash-<date>.log` files.

## 10. Troubleshooting

| Symptom | Likely cause, and what to do |
|---|---|
| `x509: certificate signed by unknown authority` | The cluster's certificate is signed by an authority your system doesn't know: fill in the *CA file*. |
| `The cluster responded HTTP 401` | Credentials refused — a wrong password or API key secret, a wrong or expired Bearer token. |
| `API Key ID missing…` | The secret field holds a secret alone, with no identifier: fill in *API Key ID*, or paste the key in its `encoded` form (§3). |
| `The cluster responded HTTP 403` | The account isn't allowed to read the cluster's root (`GET /`): it needs at least the monitoring privilege (`monitor`). |
| `The cluster responded HTTP 301` (or `302`…) followed by an address | The URL entered redirects elsewhere: use the address shown. Redirections are never followed. |
| `No credentials in the URL…` | Remove `user:password@` from the URL and choose Basic Auth. |
| The status bar says `Cluster` instead of `ES …` or `OS …` | The cluster wasn't recognized: everything is offered, unfiltered. State `distribution` and `version` in `config.yaml` (§5). |
| `Tab` doesn't complete | Some terminals intercept `Tab`: use `F10`. |
| `Ctrl+←/→` doesn't switch panel on macOS | The system intercepts it: use `Option+←/→`. |
| `F2` copies nothing | The copy goes through the terminal (OSC 52): PuTTY doesn't support it, `tmux` and `screen` need to be configured for it. |
| A request with a huge answer fails with "response larger than 64 MB" | Narrow it down: `filter_path`, `size`, or `h=` for `_cat` commands. |
| A proxy is needed to reach the cluster | Set `HTTPS_PROXY`, or `proxy:` on the cluster's entry in `config.yaml` (§5). |
| The message starts with `Through proxy …` | The attempt went through that proxy. If the cluster is reached without a proxy — which was the case up to 0.6, which ignored `HTTPS_PROXY` — add it to `NO_PROXY`, or set `proxy: none` on its entry in `config.yaml` (§5). |
| `HTTP 407`, or `Proxy Authentication Required` | The proxy asks for a password: `HTTPS_PROXY=http://user:password@proxy:3128`. Only Basic authentication is supported. |
| `a proxy reached over TLS (https://) is not supported` | The proxy is declared as `https://`: use its `http://` address, or a SOCKS5 proxy (§5). |
| The program crashed | A `crash-<date>.log` file is written to the configuration directory (§5), or printed if it can't be: attach it to a [bug report](https://github.com/gnocent/termdevtools/issues), after checking that it holds nothing confidential. |
