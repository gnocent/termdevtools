*(Version française : [SPEC_fr.md](SPEC_fr.md))*

# TermDevTools — Project specification

> Terminal-mode simulator of Kibana's "DevTools" view, for submitting requests to an Elasticsearch cluster without going through a browser.

Status: finalized for v1 — implementation complete, this document now tracks the shipped design.

---

## 1. Context and goal

- **Problem solved**: We sometimes run into cases where an Elastic cluster has no Kibana, or its Kibana isn't working. To make investigations easier, having a DevTools equivalent directly in the terminal is much more efficient than typing a handful of curl commands (with the associated SSL headaches, complex API requests, …).
- **Target users**: the team responsible for running Elasticsearch clusters.
- **Target environments**: RHEL 8, RHEL 9, RHEL 10 (terminal only, no GUI).
- **Portability constraint**: single binary, no system dependency beyond the base libc.

## 2. Technical choices

- **Language chosen**: **Go**.
- **TUI library chosen**: [`tview`](https://github.com/rivo/tview) (ready-made widgets: `TextArea` for the editor, `TextView` for the JSON, `Flex`/`Grid` for layout, `SetInputCapture` for global shortcuts), built on [`tcell`](https://github.com/gdamore/tcell). Chosen over `bubbletea` for how simple it is to develop against for this use case (a classic widget-based layout, no complex custom rendering).
- **HTTP/JSON client library**: Go's stdlib (`net/http` + `encoding/json`), no external dependency needed a priori.
- **Build/distribution method**: static binary (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build`), no dependency on the system's libc → portable as-is across RHEL 8/9/10, and easy to fold into an existing deployment/configuration-management tool.

## 3. User interface (TUI)

### 3.0 Connection

On launch, a connection screen lists the URLs of known clusters (no separate name: the URL is already the most explicit identifier), sorted from most to least recently used (order of the `clusters` list in `config.yaml`, specific to the current user — see §9.1 and §9.2), and always additionally offers a **"New connection"** option.

- Selecting an existing cluster: non-sensitive fields (auth type, CA/cert paths, username, API key ID) are pre-filled from `config.yaml`; only the secret (password, API key secret, private key passphrase) is asked for again, depending on the auth type.
- **New connection**: a full interactive form to fill in — URL, authentication type (none / Basic Auth / API Key / mTLS client certificate), then depending on the type: username, API key ID, CA path (pre-filled with `default_ca_dir`), client cert/key paths (pre-filled with `default_client_cert_dir`), whether to enable TLS verification — and finally the corresponding secret(s).
- **Fields displayed dynamically**, to show only what's relevant:
  - URL in `http://` (not https) → TLS fields hidden (CA file, "verify certificate" checkbox, and the client certificate/key fields, since a client certificate is part of the TLS handshake)
  - "None" authentication → no auth field shown
  - "Basic Auth" → only username/password
  - "client certificate (mTLS)" → only the certificate fields (hides username/password); also hidden if the URL is `http://` (see above)
- **Active field highlighted**: the field currently focused is displayed in inverted colors (background/text), so it stays identifiable even when the blinking cursor alone isn't enough.
- In both cases, once the connection succeeds, the entry (new or existing) is moved/inserted at the **first position** of the `clusters` list in `config.yaml` — no secret is ever written there.
- Once connected, you only ever work against a single Elasticsearch cluster until disconnecting (= closing the program, see §4), inside a general layout inspired by Kibana's DevTools.

### 3.1 General layout

- Screen split into two vertical panels, with a relative width adjustable during the session via `Ctrl+Shift+←/→` (see §4):
  - **Left panel**: request editor (free-form text, one or more lines, e.g. `GET _cat/indices`). Default content is loaded from a `cheatsheet.txt` file (same directory as the binary) if it exists.
  - **Right panel**: JSON result of the last executed request (so empty on startup).
  - **Automatic word wrap** in both panels (a line too long for the display width is visually folded on screen) — this affects neither the editor's actual text (so not request parsing, see §3.2), nor the content exported/copied from the result (see §3.3). Implementation note worth keeping in mind: `TextArea.GetCursor()` and `TextView.ScrollTo()` both reason in terms of **displayed line** (post-wrap), not logical line, as soon as wrap is active — relying on them directly would have broken request targeting (`Ctrl+Enter`), completion (`Tab`), and search scrolling (`Ctrl+F` on the right). Worked around respectively via `TextArea.GetSelection()` (absolute offset position in the text, independent of display) and tview's region mechanism (`Highlight`/`ScrollToHighlight`, anchored to content rather than a line number).
- **Status bar**: connected cluster, current user, "request in progress..." status during the call, then HTTP code + response time once the request completes. A live-updating timer during the wait is pushed to v2 (see §7) to keep the first version simple.
- **Help screen (`F1`)**: a popup overlaid on the layout (centered, height proportional to the terminal, scrollable if content overflows), reminding how the two panels work, the list of shortcuts (§4), and the location of configuration/template files (`config.yaml`, `queries_*.txt`, `cheatsheet.txt`, `endpoints.txt`, `cat_columns.txt`, `exports/` — see §9.1). Closes with `Esc`, with no effect on the editor's or result's content.

### 3.2 Editor (left panel)

- tview component: `TextArea` (multi-line, natively handles cursor/selection).
- Content: text containing one or more API requests (starting with GET, PUT, POST, or DELETE, followed by the endpoint and parameters, and on the following lines, the JSON payload to send). The editor detects the end of the JSON under a request (brace balancing) to understand the separation with the next request. Any line starting with `#` is a comment, and is therefore ignored.
- Execution: `Ctrl+Enter` executes the request the cursor is in, **only if the left panel is focused** (no effect if the right panel is focused, see §4). The call is launched asynchronously; the status bar switches to "request in progress...", then the right panel and status bar are updated once the response is received.
- Default file: `cheatsheet.txt`, loaded at startup if it exists (same directory as the binary).
- **Per-cluster save**: the editor content save is specific to the **cluster you're connected to** (identified by its URL) **and to the current user** — one `~/.config/termdevtools/queries_<sanitized URL>.txt` file per cluster already used by that user (next to `config.yaml`, see §9.1 for the detail of filename sanitization).
- **Save triggers**:
  - `Ctrl+S` (left panel focused): explicit save, with confirmation in the status bar.
  - **Automatic on program exit**: content is saved with no explicit action on close (`Ctrl+C`, or an external `SIGTERM`/`SIGHUP` signal — e.g. a dropped SSH session), in addition to explicit `Ctrl+S`. Best-effort, silent (no confirmation possible at that point). `SIGKILL`, as with any program, remains impossible to intercept.
- **Loading at startup**, once connected to a cluster: if a save already exists **for this cluster (URL) and this user**, it's loaded first; otherwise, `cheatsheet.txt` serves as the default content.
- **Syntax highlighting: dropped, not just deferred** — `tview.TextArea` (the library's only widget supporting multi-line editing: cursor, selection, undo, clipboard) explicitly does not support multi-color text (official documentation: *"Multi-color text is not supported"*), unlike the read-only `TextView` used on the right (§3.3). Getting it would require rebuilding a custom editor on top of a colorable `TextView` (cursor/selection/editing reimplemented by hand) — judged disproportionate for an internal v1 tool. Confirmed that no newer version of tview lifts this limitation (v0.42.0 = latest version as of 2026-08-12).
- **Auto-completion (`Tab`, left panel focused)**: offered only when the cursor is in the middle of typing a `METHOD partial_endpoint` line (not in a JSON body or anywhere else — `Tab` keeps its standard tab-insertion behavior there). Case-insensitive comparison of the typed prefix against a **list of known endpoints**, centered on administration/operations (`_cat/*`, `_cluster/*`, `_nodes/*`, index admin endpoints...) — no dynamic discovery of real index names (noted as an idea for a future version, see §7).
  - 0 match → message in the status bar, nothing else.
  - 1 match → direct completion, no further interaction.
  - Several matches → a dropdown list to choose from (arrows then Enter to confirm, Esc to cancel); an extra `Tab` while the list is open cycles through the suggestions.
  - **Optional trailing `/`**: in HTTP, a trailing `/` right before the parameters is optional (`_cat/indices/?h=...` is equivalent to `_cat/indices?h=...`). No known endpoint stores one, so it's ignored for comparison — completion replaces the whole typed segment (the `/` included), not just what precedes it. This case doesn't arise for the `h=`/`s=` columns below: `_cat` command recognition (longest prefix, at a `/` boundary) already naturally absorbs it.
  - **List source**: `endpoints.txt` (one endpoint per line, `#` = comment) next to the binary if it exists (§9.1) — then entirely replaces the default list built into the binary. Lets the team adjust the list to their Elasticsearch version without recompiling.
  - **Default list**: extracted from the official OpenAPI spec ([elastic/elasticsearch-specification](https://github.com/elastic/elasticsearch-specification), branch `9.5`), filtered to endpoints with no path parameter (`/{index}/...` ones are out of scope, see above) and to core admin domains — `_cat` (all commands, `?v` systematically for column headers), `_cluster`, `_nodes`, index/search/snapshot/ILM/SLM/license. Deliberately left out: ML, security, watcher, transform, rollup, SQL/ES\|QL, CCR, connectors, inference, enrich. To be regenerated from a more recent branch of the same repo when the target Elasticsearch version changes significantly.
- **`h=`/`s=` columns for `_cat/*` commands**: a special case of the auto-completion above, taking priority over generic endpoint completion. Recognized when the cursor is in the middle of typing the `h=` (displayed columns) or `s=` (sort) parameter of an already-identified `_cat/xxx` command (e.g. `_cat/indices?h=health,st`):
  - only the last typed column (after the last comma) is completed, what precedes it is preserved as-is;
  - for `s=`, if the column is already followed by `:`, completes the sort direction (`asc`/`desc`) rather than a column name (e.g. `s=docs.count:de` → `desc`); this case doesn't apply to `h=`, where a `:` is part of the compared text as-is;
  - **trailing path filter**: many `_cat` commands accept a filter (index name, node name...) between the command and the parameters, e.g. `_cat/shards/myindex?h=...`. The command is recognized as the longest entry in the `command → columns` table that prefixes the path at a `/` boundary (never a partial word match: `shardsxyz` does not match `shards`) — otherwise `shards/myindex` would match no known command and nothing would be suggested;
  - the list of proposed columns depends on the current `_cat` command (e.g. the columns of `_cat/shards` differ from those of `_cat/indices`) — a `command → columns` table distinct from the flat endpoint list, with the same sourcing principle (§9.1): a `cat_columns.txt` file next to the binary if it exists, otherwise a default table built into the binary, generated on 2026-08-12 from `GET _cat/<command>?help` queried against a real Elasticsearch 9.5.0 cluster. **Only full column names are kept** (e.g. `docs.count`), not their short aliases (`dc`): more descriptive, and it limits the number of suggestions for commands with many columns (`_cat/indices`, `_cat/nodes`, `_cat/shards`...) — aliases can still be added by hand to `cat_columns.txt` for anyone who wants them.

### 3.3 Result (right panel)

- Display format: pretty-printed JSON (typical responses) or fixed-width text (e.g. a response to a `_cat` command).
- Syntax highlighting for JSON: yes in v1.
- Result history: No.
- Handling large responses: manual scroll with up/down keys.
- Displaying errors (invalid request, unreachable cluster, timeout): in the status bar.
- **Export (`Ctrl+S`, right panel focused)**: writes the currently displayed result into the binary's `exports/` subfolder (created if needed), a timestamped filename (`YYYYMMDD-HHMMSS`), `.json` extension if it's valid JSON, `.txt` otherwise. Confirmation (with path) shown in the status bar; error (e.g. nothing to export) shown the same way.
- **Copy to clipboard (`F2`)**: `tview.Application.EnableMouse(true)` prevents native terminal text selection (the app captures mouse events) — no mouse selection possible in this panel. `F2` therefore copies the entire displayed result, via the standard terminal mechanism **OSC 52** (`tcell.Screen.SetClipboard`): the local terminal receives an escape sequence asking it to copy to *its own* clipboard, which works even over SSH (the clipboard is never the remote server's). Confirmation shown in the status bar, but **with no guarantee the copy actually happened**: neither tcell nor the OSC 52 protocol return a confirmation, and support depends on the terminal (works on most modern terminals — Windows Terminal, iTerm2, recent GNOME Terminal/VTE... — but not on plain PuTTY, nor in tmux/screen without specific passthrough configuration). To be verified in real-world use.

## 4. Keyboard shortcuts

Put a help bar under the status bar as a shortcut reminder.
| Action | Key | Status |
|---|---|---|
| Execute the request under the cursor | `Ctrl+Enter` | Defined |
| Switch focus left ↔ right panel | `Ctrl+←`/`Ctrl+→` | Defined |
| Quit the application (auto-saves the left panel, §3.2) | `Ctrl+C` | Defined |
| New request / clear the editor | Free-form editing of the left panel text | Defined |
| Open/change the cluster connection | Quit the program and relaunch it | Defined |
| Search in requests | `Ctrl+F` in the left panel | Defined |
| Search in the JSON result | `Ctrl+F` in the right panel | Defined |
| Resize the left/right split | `Ctrl+Shift+←` (shrink the left) / `Ctrl+Shift+→` (grow it) | Defined |
| Save (left) / export (right) | `Ctrl+S`, behavior depends on the focused panel (§3.2, §3.3) | Defined |
| Complete an endpoint while typing | `Tab` in the left panel, on a `METHOD endpoint` line (§3.2) | Defined |
| Show help (how it works + shortcuts) | `F1`, `Esc` to close | Defined |
| Copy the result to the clipboard | `F2` (§3.3) | Defined |
| Switch the interface language (fr/en) | `F3` | Defined |

> `Ctrl+Enter` is only active when the left panel (request editing) is focused — no effect from the right panel.
>
> **History of attempts**: `Ctrl+Esc` (quit) and `Ctrl++`/`Ctrl+-` (resize), initially planned, turned out to be intercepted in practice — `Ctrl+Esc` by Windows itself (a system shortcut on the team's machines), `Ctrl++`/`Ctrl+-` by the terminal (font zoom, notably under Windows Terminal). Kept instead: `Ctrl+C` (the only exit shortcut, universally available at the terminal level) and `Ctrl+Shift+←/→` for the split. `Ctrl+H`, considered for help, was dropped in favor of `F1`: `Ctrl+H` corresponds to the historical control code `0x08` (BS), documented by tview as an alternate Backspace shortcut in `TextArea`/`InputField` — using it globally would have broken character deletion via `Ctrl+H` in the editor and the connection form. This still needs confirming across all of the team's terminals/machines — should a new conflict show up, plan to switch to other function keys (F5...), which are exempt from this kind of clash. Same logic for copying the result: `Ctrl+Shift+C` (the "copy" convention on many modern terminals, chosen to avoid colliding with `Ctrl+C` already taken by Quit) was dropped — that's exactly the risk: this combination is itself frequently intercepted by the terminal *before* reaching the app. `F2` was kept instead.
>
> `Ctrl+S` is a classic control code (like `Ctrl+F`), so a priori reliable everywhere — the only historical caveat is the XON/XOFF flow control of some terminals (`Ctrl+S` freezes output until `Ctrl+Q`), normally disabled by the raw mode the program enables. To be reported if such a freeze is nonetheless observed.
>
> **macOS**: `Ctrl+←/→` is intercepted at the OS level by default (Mission Control desktop switching), and `Ctrl+Enter` can't even be distinguished from plain `Enter` in classic terminal encoding — `Ctrl+M` *is* `Enter`'s control byte. `Option`/`Alt` is therefore accepted as an additional modifier, on top of `Ctrl`, for exactly the three shortcuts affected: execute (`Ctrl+Enter` / `Option+Enter`), focus switch (`Ctrl+←/→` / `Option+←/→`), and resize (`Ctrl+Shift+←/→` / `Option+Shift+←/→`) — see `hasShortcutModifier` in `ui/app.go`. Deliberately **not** extended to `Ctrl+F`/`Ctrl+S`/`Ctrl+C`: those are raw, universally reliable control bytes with no such conflict, and by default macOS terminals turn `Option`+letter into an accented character instead of signaling a modifier at all (unless "Use Option as Meta key" is enabled in the terminal's own preferences).

## 5. Connecting to the Elasticsearch cluster

- See §3.0 for the flow and §9.2 for the `config.yaml` schema.
- **Supported authentication**: none, Basic Auth (login/password), API Key, client certificate (mTLS).
- **TLS**: certificate verification (CA located by default in `default_ca_dir`, path overridable per connection), option to skip it.
- **Certificates**: two globally configurable default directories (`default_ca_dir`, `default_client_cert_dir`) to pre-fill paths when entering a new connection.
- **Secret storage**: none — password, API key secret, and private key passphrase are re-requested on every connection; only non-sensitive elements (URL, auth type, username, API key ID, CA/cert paths) are persisted in `config.yaml`, with the most recently used entry at the top of the list.

## 6. Supported requests

- **Input syntax**: free-form, Kibana Console style (`METHOD path` + optional JSON body on the following lines).
- **HTTP methods to support**: GET, POST, PUT, DELETE.
- **Validation before sending**: check that the body's JSON is valid before executing.
- **Default timeout**: 2 minutes, configurable via `default_timeout_seconds` in `config.yaml` (§9.2).

## 7. Out of scope for v1 (future backlog)

- Live-updating timer for the in-progress request in the status bar (v1 only shows the final result: HTTP code + total duration once the response is received).
- Dynamic auto-completion of the connected cluster's real index names (v1 is limited to a static list of known endpoints, see §3.2).
- Advanced syntax highlighting.

## 8. Non-functional constraints

- **Runtime dependencies**: none beyond the standard libc present on RHEL 8/9/10.
- **Performance**: must support large results (several MB).
- **Intended final packaging**: a single binary to copy.
- **Project/binary name**: termdevtools.

## 9. Proposed technical architecture

### 9.1 File locations

Connection history is specific to the user (two people launching the same shared binary on the same server shouldn't step on each other), whereas the cheatsheet is more of a team-level content attached to the installation. Hence two separate locations:

- **User configuration directory** (`~/.config/termdevtools/`, or `$XDG_CONFIG_HOME/termdevtools/` if that variable is set), created automatically (`0700` permissions) on first write:
  - `config.yaml` — known clusters, updated automatically on every successful connection, no secret in it (§9.2).
  - `queries_<sanitized URL>.txt` — one file per cluster already used by this user, containing the latest save of the left panel for that cluster (§3.2). Written by `Ctrl+S` and automatically on program exit. Name built from the cluster's URL, replacing with `_` any character that isn't alphanumeric, `.`, `_`, or `-` (so notably `:` and `/`) — e.g. `https://es-prod.example.com:9200` → `queries_https___es-prod.example.com_9200.txt`. Two different URLs that happened to be similar enough to produce the same name after this normalization would (rare case) share the same file — an accepted limitation to keep names readable rather than hashed.
- **Executable's directory** (the binary's, not the shell's current working directory):
  - `cheatsheet.txt` — default editor content, loaded at startup only if no `queries_*.txt` save yet exists for the current cluster/user (optional, §3.2).
  - `endpoints.txt` — list of endpoints offered by `Tab` auto-completion, replaces the default list built into the binary if present (§3.2). **Checked into the repository** (unlike `cheatsheet.txt`/`config.yaml`, which remain plain `.example` templates): since it changes rarely, it's treated as a standard project input rather than a template to copy. Still optional at runtime, though — without it (e.g. a deployment where only the binary is copied), the default list built into the binary takes over. Lets the list be adjusted to the team's Elasticsearch version without recompiling; to be regenerated from the OpenAPI spec (§3.2) if it diverges too much from a future version.
  - `cat_columns.txt` — `h=`/`s=` columns offered by auto-completion for each `_cat/*` command (§3.2), same treatment as `endpoints.txt` (checked into the repository, replaces the default table built into the binary if present, optional at runtime). Format: `# _cat/command` sections followed by one column per line. Generated on 2026-08-12 from `GET _cat/<command>?help` against a real Elasticsearch 9.5.0 cluster — to be regenerated the same way if the columns diverge too much from a future version.
  - `exports/` — results exported via `Ctrl+S` from the right panel, one timestamped file per export (created on demand, §3.3).

### 9.2 `config.yaml` schema (`~/.config/termdevtools/config.yaml`)

```yaml
default_timeout_seconds: 120
language: fr  # interface language: "fr" (default) or "en" — see the i18n package; also switchable live with F3, which rewrites this line
default_ca_dir: /etc/pki/termdevtools/ca              # pre-fills the CA field for a new connection
default_client_cert_dir: /etc/pki/termdevtools/certs  # pre-fills the client cert/key fields (mTLS)

# order = usage history, most recently connected first
# (no separate name: the URL identifies the cluster)
clusters:
  - url: https://es-prod.example.com:9200
    auth_type: basic        # none | basic | api_key | mtls
    username: svc_devtools  # used if auth_type: basic (password never stored)
    api_key_id: ""          # used if auth_type: api_key (secret never stored)
    tls:
      verify: true
      ca_file: /etc/pki/ca-trust/es-prod-ca.pem
      client_cert: ""        # used if auth_type: mtls
      client_key: ""         # used if auth_type: mtls

  - url: https://es-staging.example.com:9200
    auth_type: none
    tls:
      verify: false
```

### 9.3 Suggested Go project structure

```
termdevtools/
├── main.go                 // entry point: loads config, launches the connection screen, then the UI
├── go.mod
├── config/
│   └── config.go           // reads/writes config.yaml, moves the used entry to the top of the list
├── i18n/
│   └── i18n.go             // fr/en message catalogs for the interface, selected via config.Language
├── esclient/
│   └── client.go           // HTTP client (auth none/basic/api_key/mtls, TLS), executes a request
├── parser/
│   └── parser.go           // splits the editor content into requests (method, endpoint, payload, comments)
├── ui/
│   ├── connect.go          // initial connection screen (tview.Form)
│   ├── app.go              // Flex assembly, focus management, global shortcuts
│   ├── editor.go           // left panel (TextArea)
│   ├── completion.go       // default endpoints + _cat columns, loading endpoints.txt/cat_columns.txt, h=/s= detection, prefix filtering
│   ├── result.go           // right panel (TextView + JSON highlighting)
│   └── statusbar.go        // status bar + shortcuts help bar
├── cheatsheet.txt.example
├── endpoints.txt          // checked into the repository (§9.1), not a plain .example
├── cat_columns.txt        // same, h=/s= columns per _cat command
└── config.yaml.example
```

### 9.4 Request execution flow

1. Left panel focused, cursor positioned on a request, `Ctrl+Enter`.
2. `parser` extracts method + endpoint + JSON payload around the cursor and validates the JSON.
3. Invalid JSON → error message in the status bar, nothing is sent.
4. Valid JSON → HTTP call launched in a goroutine; status bar → "request in progress...".
5. Response received → `esclient` returns the HTTP code, duration, body; UI updated via `QueueUpdateDraw` (thread-safe with tview): right panel filled in (pretty-printed and highlighted JSON, or plain text for `_cat` responses), status bar → HTTP code + duration.

## 10. Open questions

No blocking question identified at this stage. Section kept available for any question that might come up during future work.
