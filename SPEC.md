*(Version française : [SPEC_fr.md](SPEC_fr.md))*

# TermDevTools — Project specification

> Terminal-mode simulator of Kibana's "DevTools" view, for submitting requests to an Elasticsearch or OpenSearch cluster without going through a browser.

Status: this document describes the design as shipped — version 0.6 (beta). The version history is in [CHANGELOG.md](CHANGELOG.md), the step-by-step installation in [INSTALL.md](INSTALL.md).

---

## 1. Context and goal

- **Problem solved**: an Elasticsearch or OpenSearch cluster sometimes has no Kibana (or OpenSearch Dashboards), or one that is down — often during an investigation, precisely. An equivalent of Dev Tools right in the terminal is then far more efficient than a series of `curl` commands (TLS handling, multi-line requests, formatting the JSON…).
- **Target users**: the people who administer and operate Elasticsearch or OpenSearch clusters.
- **Target environments**: Linux without a graphical interface first (RHEL 8/9/10 and any other amd64 distribution), that is the servers a cluster is reached from; Windows and macOS as well, for use from one's own workstation.
- **Target clusters**: Elasticsearch from 7.17 to 9.x, and self-managed OpenSearch 2.x and 3.x — the distribution and version are detected at connection (§5), and what the tool suggests is selected for them (§9.5). OpenSearch managed by AWS, which requires SigV4 request signing, is out of scope (§7).
- **Portability constraint**: single binary per platform, no system dependency beyond the base libc on Linux (nothing equivalent needed on Windows/macOS), and **no companion file**: the tool is meant for servers without Internet access, where an installation that is one file to copy matters more than being able to update reference data without a new binary.

## 2. Technical choices

- **Language chosen**: **Go**.
- **TUI library chosen**: [`tview`](https://github.com/rivo/tview) (ready-made widgets: `TextArea` for the editor, `TextView` for the JSON, `Flex`/`Grid` for layout, `SetInputCapture` for global shortcuts), built on [`tcell`](https://github.com/gdamore/tcell). Chosen over `bubbletea` for how simple it is to develop against for this use case (a classic widget-based layout, no complex custom rendering).
- **HTTP/JSON client library**: Go's stdlib (`net/http` + `encoding/json`), no external dependency needed a priori.
- **Build/distribution method**: static binary (`CGO_ENABLED=0 go build`), no dependency on the system's libc → portable as-is across RHEL 8/9/10 (and any other Linux amd64 distribution), and easy to fold into an existing deployment/configuration-management tool. [`build-release.sh`](build-release.sh) cross-compiles the same source for `linux/amd64`, `windows/amd64`, and `darwin/arm64` in one pass, stamps the version into it (`termdevtools --version`) and produces the `SHA256SUMS` file a binary can be checked against once carried elsewhere — see [INSTALL.md](INSTALL.md).
- **Reference data**: endpoints, `_cat` columns and recipes are plain text files of the source tree, compiled into the binary with `go:embed` (§9.5).

## 3. User interface (TUI)

### 3.0 Connection

On launch, a connection screen lists the URLs of known clusters (no separate name: the URL is already the most explicit identifier), sorted from most to least recently used (order of the `clusters` list in `config.yaml`, specific to the current user — see §9.1 and §9.2), and always additionally offers a **"New connection"** option.

- Selecting an existing cluster: non-sensitive fields (auth type, CA/cert paths, username, API key ID) are pre-filled from `config.yaml`; only the secret (password, API key secret, private key passphrase) is asked for again, depending on the auth type.
- **New connection**: a full interactive form to fill in — URL, authentication type (none / Basic Auth / API Key / mTLS client certificate), then depending on the type: username, API key ID, CA path (pre-filled with `default_ca_dir`), client cert/key paths (pre-filled with `default_client_cert_dir`), whether to enable TLS verification — and finally the corresponding secret(s).
- **Certificate picker (`Enter` on the CA file / client cert / client key fields)**: opens a popup — a small file browser, 70 columns wide — listing the entries found directly inside the corresponding configured directory (`default_ca_dir` for the CA field, `default_client_cert_dir` for both client cert/key fields — §9.2), to pick from instead of typing a filename from memory; subdirectories are listed first (suffixed with the OS path separator), then files, each group alphabetical. Arrows then `Enter` picks a file or browses into a subdirectory; `Backspace` goes back up to the parent directory; `Esc` cancels entirely without changing the field, from any depth. Reports a clear error in the connection screen's message line instead of opening an empty popup when the corresponding directory isn't configured; falls back to browsing the user's home directory (rather than erroring) when it's configured but doesn't exist on disk (§5).
- **Fields displayed dynamically**, to show only what's relevant:
  - URL in `http://` (not https) → TLS fields hidden (CA file, "verify certificate" checkbox, and the client certificate/key fields, since a client certificate is part of the TLS handshake)
  - "None" authentication → no auth field shown
  - "Basic Auth" → only username/password
  - "client certificate (mTLS)" → only the certificate fields (hides username/password); also hidden if the URL is `http://` (see above)
- **Active field highlighted**: the field currently focused is displayed in inverted colors (background/text), so it stays identifiable even when the blinking cursor alone isn't enough.
- In both cases, once the connection succeeds, the entry (new or existing) is moved/inserted at the **first position** of the `clusters` list in `config.yaml` — no secret is ever written there.
- **Detection**: the answer to the `GET /` that validates the connection also tells which distribution and version the cluster runs (§5). Nothing to choose or fill in; a cluster that can't be identified is still connected to.
- **One attempt at a time**: an attempt given up on (`Esc` or the Cancel button) or superseded by a newer one is ignored when its answer finally arrives — a cluster slow to answer can't replace the session opened on another one since, and pressing Connect twice opens a single session.
- **What is refused before any connection**: a URL carrying credentials (`https://user:password@host`). The URL is the one thing about a cluster that is saved and displayed as is — in `config.yaml`, in the status bar, in the `curl` commands copied: the password would end up there. The message points to Basic Auth.
- **What is not a connection**: a `3xx` answer. Redirections are never followed (§5); the screen shows the status and the address the answer points to, for the URL to be corrected.
- Once connected, you only ever work against a single cluster until disconnecting (= closing the program, see §4), inside a general layout inspired by Kibana's DevTools.

### 3.1 General layout

- Screen split into two vertical panels, with a relative width adjustable during the session via `F5`/`F6` (see §4):
  - **Left panel**: request editor (free-form text, one or more lines, e.g. `GET _cat/indices`). Its starting content is described in §3.2.
  - **Right panel**: JSON result of the last executed request (so empty on startup).
  - **Automatic word wrap** in both panels (a line too long for the display width is visually folded on screen) — this affects neither the editor's actual text (so not request parsing, see §3.2), nor the content exported/copied from the result (see §3.3). Implementation note worth keeping in mind: `TextArea.GetCursor()` and `TextView.ScrollTo()` both reason in terms of **displayed line** (post-wrap), not logical line, as soon as wrap is active — relying on them directly would have broken request targeting (`Ctrl+Enter`), completion (`Tab`), and search scrolling (`Ctrl+F` on the right). Worked around respectively via `TextArea.GetSelection()` (absolute offset position in the text, independent of display) and tview's region mechanism (`Highlight`/`ScrollToHighlight`, anchored to content rather than a line number).
    A related but distinct gap surfaced later: `TextArea.Select()` (used for the left panel's search matches) is correctly wrap-safe — it's the offset-based method just described — but its own doc comment is explicit that it "preserves" the scroll offset, unlike normal cursor movement (typing, arrow keys), which tview scrolls into view automatically. A match outside the visible viewport was therefore selected correctly internally while staying invisible on screen — indistinguishable, to the user, from search "landing in the wrong place". Fixed by `Editor.scrollToCursor` (`ui/editor.go`), replicating tview's own (unexported) auto-scroll logic using `GetCursor`'s display row (correct here, unlike for logical-line purposes) together with the public `GetOffset`/`SetOffset`.

    A further report — search actually *selecting the wrong text* (not just off-screen, e.g. matching content shifted a few characters away from the real match) — turned out to be a second, more fundamental bug in the same offset-based method, not a wrap issue at all: `TextArea.GetSelection`/`Select`/`Replace` all count **UTF-8 bytes** internally (confirmed by reading tview's own source — position tracking advances by `len(cluster)`, a string's byte length), a fact none of their doc comments spell out. This codebase's own offset arithmetic (`findNext` in `ui/search.go`, `lineColAt`/`CompletionPrefix` in `ui/editor.go`) used **rune** counts (`len([]rune(...))`) throughout instead. The two agree exactly as long as every character before the position in question is single-byte ASCII — and silently diverge, by however many extra bytes accumulate, as soon as any multi-byte UTF-8 character (an accented letter, typically) appears earlier in the buffer. Not a crash, just a wrong-looking result: search selecting a few characters away from the real match, endpoint completion replacing the wrong span, and — since `CursorLine` is built on the same `lineColAt` — potentially **`Ctrl+E` targeting the wrong request entirely** whenever an earlier line held non-ASCII text. Fixed by switching all of this codebase's own offset arithmetic to byte lengths (`len(s)`) instead of rune counts, matching tview's convention exactly; see `TestLineColAtByteOffsets` and `TestFindNextByteOffsets`, both of which fail against the pre-fix code with content reproducing the original report. One last case of the same kind: search ignores case, and lowercasing a whole text can change its length in bytes ("İ", two bytes, becomes "i̇", three) — so case is folded letter by letter, only where the lowercase form takes the same number of bytes (`foldCase`, `ui/search.go`).
- **Status bar**: connected cluster, current user, "request in progress..." status during the call, then HTTP code + response time once the request completes. A live-updating timer during the wait is pushed to v2 (see §7) to keep the first version simple. The detected distribution and version, in short form (`ES 9.5.4`, `OS 2.19.6`), **take the place of the "Cluster" caption** in front of the URL rather than being added to the line: on an 80-column terminal the line has no room to spare, and anything added to its start is taken from the message at its end.
- **Help screen (`F1`)**: a popup overlaid on the layout (centered, height proportional to the terminal, scrollable if content overflows), reminding how the two panels work, the list of shortcuts (§4), and the location of the files the user may create or edit (§9.1). Closes with `Esc`, with no effect on the editor's or result's content.

### 3.2 Editor (left panel)

- tview component: `TextArea` (multi-line, natively handles cursor/selection), laid out beside a line-number gutter (SPEC.md §7 backlog #5) inside a shared bordered panel — the number is shown once per logical line, blank on a word-wrapped continuation row, matching the usual editor convention. `TextArea` exposes no public API for where it wraps, so the gutter recomputes that itself (`ui/gutter.go`, `wrapRowStarts`) using the same underlying library (`github.com/rivo/uniseg`) `TextArea` uses internally, kept in step by construction rather than by reverse-engineered approximation.
- **Auto-closing brackets/quotes** (SPEC.md §7 backlog #6, `ui/autoclose.go`): typing `{`, `[`, or `"` inserts the matching closer too, cursor placed in between; typing a closer that's already sitting right there (typically the one just auto-inserted) steps over it instead of duplicating it; Backspace between an empty pair removes both sides at once. Deliberately conservative given this was flagged as the highest UX-risk item on the backlog: does nothing while a selection is active (TextArea's own default typing-replaces-selection behavior applies instead), and doesn't auto-close a bracket typed as plain content inside an already-open string — tracked per line by counting unescaped `"` characters before the cursor, sufficient on its own since an unescaped newline inside a JSON string is invalid JSON to begin with (a string literal can never legitimately span more than one line).
- Content: text containing one or more API requests (starting with GET, PUT, POST, or DELETE, followed by the endpoint and parameters, and on the following lines, the JSON payload to send). The editor detects the end of the JSON under a request (brace balancing) to understand the separation with the next request. Any line starting with `#` is a comment, and is therefore ignored.
- Execution: `Ctrl+E` (also `Ctrl+Enter` where the terminal reports it — see §4) executes the request the cursor is in, **only if the left panel is focused** (no effect if the right panel is focused, see §4). The call is launched asynchronously; the status bar switches to "request in progress...", then the right panel and status bar are updated once the response is received. Only the answer to the last request sent is displayed: an earlier, slower one that answers afterwards is dropped.
- **Reusable variables** (`${name}`, SPEC.md §7 backlog #4): a `${name}` reference anywhere in the path or body is substituted with a stored value before the request is actually sent (`Ctrl+E`) or turned into a `curl` command (`F9`) — `App.resolveRequest`, shared by both, the only two "what would actually be sent" operations; `F4` (reformat) deliberately does not go through it, since it edits the saved query text itself and must leave `${name}` literal. An undefined variable aborts with a status-bar error naming it (deduplicated across path and body) rather than sending the literal placeholder text. Values come from `~/.config/termdevtools/variables_<sanitized URL>.txt` — one file per cluster per user, same scheme as the query save below (`name=value` lines, `#` comments, `ui/variables.go`'s `parseVariables`) — loaded on connection and reloaded on demand with `F7` (no in-app editor; hand-edited, like the user's recipes and endpoints, §9.1). Self-documenting on first use, same approach as `config.yaml` (`LoadVariablesFile`, mirroring `config.WriteDefaultConfigFile`).
- **Starting content**: see "Loading at startup" below.
- **Per-cluster save**: the editor content save is specific to the **cluster you're connected to** (identified by its URL) **and to the current user** — one `~/.config/termdevtools/queries_<sanitized URL>.txt` file per cluster already used by that user (next to `config.yaml`, see §9.1 for the detail of filename sanitization).
- **Save triggers**:
  - `Ctrl+S` (left panel focused): explicit save, with confirmation in the status bar.
  - **Automatic on program exit**: content is saved with no explicit action on close (`Ctrl+C`, or an external `SIGTERM`/`SIGHUP` signal — e.g. a dropped SSH session), in addition to explicit `Ctrl+S`. Best-effort, silent (no confirmation possible at that point). `SIGKILL`, as with any program, remains impossible to intercept.
  - **Never at the expense of what is already saved**: the file is replaced in one step (written beside it, then renamed over it), so a full disk or a crash in the middle of a save leaves the previous content intact. If the save on `Ctrl+C` fails, the status bar says so and the program stays; a second `Ctrl+C` quits regardless. If saved requests exist but couldn't be read at startup, the editor — which doesn't hold them — isn't written over them on exit; `Ctrl+S` remains the explicit way to replace them.
- **Loading at startup**, once connected to a cluster: if a save already exists **for this cluster (URL) and this user**, it's loaded; otherwise a `cheatsheet.txt` next to the binary, if a team installed one; otherwise the **starter** built into the binary — a short welcome comment and a handful of requests that work on any cluster (`GET /`, `_cluster/health`, `_cat/nodes`...), pointing to the recipe catalog for the rest. The starter replaces the long cheatsheet that versions up to 0.5 loaded: its content is now in the catalog, which is searchable and adapted to the cluster, instead of a wall of text to scroll through.
- **Recipe catalog (`F8`)**: a popup over the layout, 76 columns wide like the help screen, listing the ready-made requests that apply to the connected cluster — about a hundred, grouped by theme (overview, shards and allocation, nodes and resources, indices, tasks, snapshots, index lifecycle, upgrade and maintenance, search and documents), in English only.
  - A **filter field** keeps the focus the whole time: typing narrows the list down (every word typed must appear, case-insensitively, in the recipe's group, title, tags or body), `↑`/`↓` (also `Ctrl+P`/`Ctrl+N`, `PgUp`/`PgDn`) move the selection without wrapping around, `Enter` inserts, `Esc` closes. The application's other shortcuts are inactive while it is open, except `Ctrl+C`.
  - A **preview** below the list shows the selected recipe as it will be inserted, comments dimmed.
  - **Insertion**: at the **end** of the editor, separated from what precedes by a blank line, under a `# title` comment — never at the cursor, where it could land in the middle of an existing request. The cursor is placed on the recipe's first request, so `Ctrl+E` runs it at once; one undo removes it.
  - A recipe may hold several requests and uses `${name}` variables for what depends on the cluster (`${index}`, `${node}`, `${repository}`, `${snapshot}`, `${field}`, `${task_id}`): running it with a variable undefined names the variable to define, as for any request.
  - Recipes that exist in two forms (index lifecycle: ILM on Elasticsearch, ISM on OpenSearch) share a title: only the form that applies is listed. When the cluster couldn't be identified, both are, each followed by the clusters it is meant for.
  - **The user's own recipes** are listed with the built-in ones, marked as added, the preview's border naming their file. Format, locations and merge rules: §9.1 and §9.5.
- **Reloading (`F7`)**: re-reads everything the user or the team maintains by hand — the cluster's variables, recipes, endpoints — and forgets the `_cat` columns learned from the cluster. The status bar sums up what was loaded, or names the first problem found (file and line) and how many others there are.
- **Syntax highlighting: dropped, not just deferred** — `tview.TextArea` (the library's only widget supporting multi-line editing: cursor, selection, undo, clipboard) explicitly does not support multi-color text (official documentation: *"Multi-color text is not supported"*), unlike the read-only `TextView` used on the right (§3.3). Getting it would require rebuilding a custom editor on top of a colorable `TextView` (cursor/selection/editing reimplemented by hand) — judged disproportionate for this tool. Confirmed that no newer version of tview lifts this limitation (v0.42.0 = latest version as of 2026-08-12).
- **Auto-completion (`Tab` or `F10`, left panel focused)**: offered only when the cursor is in the middle of typing a `METHOD partial_endpoint` line (not in a JSON body or anywhere else — `Tab` keeps its standard tab-insertion behavior there; `F10` has no such fallback meaning and is simply swallowed). `F10` was added after confirming Tab itself gets swallowed before reaching the app on some terminals (see §4); no fallback modifier detection can work around a key that never reaches the app at all. Case-insensitive comparison of the typed prefix against the **list of endpoints known for the connected cluster**, centered on administration/operations (`_cat/*`, `_cluster/*`, `_nodes/*`, index admin endpoints...) — no dynamic discovery of real index names (noted as an idea for a future version, see §7).
  - 0 match → message in the status bar, nothing else.
  - 1 match → direct completion, no further interaction.
  - Several matches → a dropdown list to choose from (arrows then Enter to confirm, Esc to cancel); an extra `Tab` while the list is open cycles through the suggestions. Typing further letters narrows the selection to the first item starting with what's been typed so far (case-insensitive, `Backspace` to correct), useful to jump straight to an entry in a long list instead of scrolling. The list's title always shows that full search text (what was typed before `Tab` plus any type-ahead keystrokes since) — needed because matching is a plain prefix check with no special handling of the `/` separator: typing `i` right after completing `_cat` searches for `_cati`, not `_cat/i`, and silently matches nothing since every `_cat/*` candidate has a `/` there; the visible search text is what makes that outcome legible instead of the list just not reacting.
  - **Optional trailing `/`**: in HTTP, a trailing `/` right before the parameters is optional (`_cat/indices/?h=...` is equivalent to `_cat/indices?h=...`). No known endpoint stores one, so it's ignored for comparison — completion replaces the whole typed segment (the `/` included), not just what precedes it. This case doesn't arise for the `h=`/`s=` columns below: `_cat` command recognition (longest prefix, at a `/` boundary) already naturally absorbs it.
  - **List source**: the list built into the binary, each endpoint tagged with the clusters it exists on, filtered for the connected one (§9.5); extended — never replaced — by an `endpoints.txt` next to the binary (the team's) and one in the user's configuration directory (§9.1), one endpoint per line, `#` = comment, same optional tags.
  - **Built-in list**: generated from the official API specifications — [elastic/elasticsearch-specification](https://github.com/elastic/elasticsearch-specification), every minor branch from `7.17` to `9.5`, and the [OpenSearch API specification](https://github.com/opensearch-project/opensearch-api-specification) — then checked against real clusters (§9.5). Filtered to endpoints with no path parameter (`/{index}/...` ones are out of scope, see above) and to administration domains: `_cat` (all commands, `?v` systematically for column headers), `_cluster`, `_nodes`, indices, snapshots, tasks, ingest, dangling indices, and the core search/document endpoints (`_search`, `_count`, `_bulk`, `_reindex`...) on both; ILM, SLM, license, features, migration, node shutdown, searchable snapshots, SSL, X-Pack info on Elasticsearch; ISM, snapshot management, `_list`, query insights, search pipelines, remote store on OpenSearch. Deliberately left out: ML, security, watcher, transform, rollup, SQL/ES\|QL, CCR, connectors, inference, enrich.
- **`h=`/`s=` columns for `_cat/*` commands**: a special case of the auto-completion above, taking priority over generic endpoint completion. Recognized when the cursor is in the middle of typing the `h=` (displayed columns) or `s=` (sort) parameter of an already-identified `_cat/xxx` command (e.g. `_cat/indices?h=health,st`):
  - only the last typed column (after the last comma) is completed, what precedes it is preserved as-is;
  - for `s=`, if the column is already followed by `:`, completes the sort direction (`asc`/`desc`) rather than a column name (e.g. `s=docs.count:de` → `desc`); this case doesn't apply to `h=`, where a `:` is part of the compared text as-is;
  - **trailing path filter**: many `_cat` commands accept a filter (index name, node name...) between the command and the parameters, e.g. `_cat/shards/myindex?h=...`. The command is recognized as the longest entry in the `command → columns` table that prefixes the path at a `/` boundary (never a partial word match: `shardsxyz` does not match `shards`) — otherwise `shards/myindex` would match no known command and nothing would be suggested;
  - the list of proposed columns depends on the current `_cat` command (e.g. the columns of `_cat/shards` differ from those of `_cat/indices`) **and is asked from the cluster itself**: the first time a command's columns are completed, `GET _cat/<command>?help` is sent in the background (3-second limit; answered by the receiving node alone, without touching the cluster), the answer is kept for the session — until `F7` — and the completion that was waiting resumes by itself, provided the editor hasn't changed in the meantime. Exact for whatever version the cluster runs, including one more recent than the binary, and for a `_cat` command added by the user to `endpoints.txt`. Only names made of letters, digits, `_`, `.` and `-` are kept from the answer: what comes out of it is offered in a list, then written into the editor, and a cluster's answer has no business putting anything else there. If the cluster can't be asked (timeout, error, insufficient privileges), a **table built into the binary** is used instead, with a warning in the status bar: generated from the `?help` of the real clusters of §9.5 and deliberately conservative — only columns present on every tested version of the range it is given for. There is no user-level file for these columns: what the cluster says is always better than a list maintained by hand (the `cat_columns.txt` that versions up to 0.5 read next to the binary is ignored). **Only full column names are offered** (e.g. `docs.count`), not their short aliases (`dc`): more descriptive, and it limits the number of suggestions for commands with many columns (`_cat/indices`, `_cat/nodes`, `_cat/shards`...).

### 3.3 Result (right panel)

- Display format: pretty-printed JSON (typical responses) or fixed-width text (e.g. a response to a `_cat` command).
- **Request reminder**: the panel's first line is always a `# METHOD path` comment (no JSON body) recalling which request produced the displayed result — e.g. `# GET _cat/health?v`. Part of the panel's plain text, so it's included in exports and clipboard copy too (below), not just the on-screen display.
- **Response headers** (SPEC.md §7 backlog #2): every HTTP header on the response is listed right after the reminder, one `# Header: value` comment line per header (sorted by name), same plain-text treatment as the reminder itself — included in exports and clipboard copy. Gray like the reminder, except a `Warning` header (RFC 7234 — Elasticsearch sets it to flag a deprecated API in use) shown in yellow so it stands out. Not shown for a transport-level failure (`ShowError`): there's no response to have headers from.
- Syntax highlighting for JSON: yes in v1.
- Result history: No.
- Handling large responses: manual scroll with up/down keys. Indenting and colorizing are done outside the interface's own goroutine, which stays responsive meanwhile. A response larger than 64 MB is not displayed: the request is reported as failed, with the advice to narrow it down (`filter_path`, `size`, `h=`) — loaded whole, it would exhaust memory before anything could be shown. The duration reported covers the whole response, body included.
- Displaying errors (invalid request, unreachable cluster, timeout): in the status bar.
- **Redirections**: a `3xx` answer is displayed for what it is — its status, and its `Location` header among the headers — never followed (§5).
- **Nothing displayed is interpreted by the terminal**: a response body, its headers or an error message may hold escape sequences (writing to the clipboard, retitling the window, hiding text); the display library never writes a control character to the screen, which a test checks for every way in (`ui/terminal_safety_test.go`).
- **Export (`Ctrl+S`, right panel focused)**: writes the currently displayed result (request reminder included) into the `exports/` subfolder of the user's configuration directory (§9.1, created if needed), a timestamped filename (`YYYYMMDD-HHMMSS`), `.json` extension if the response body is valid JSON, `.txt` otherwise — note that the leading `#` reminder line means the exported `.json` file isn't itself strictly valid JSON, a deliberate trade-off for traceability. Confirmation (with path) shown in the status bar; error (e.g. nothing to export) shown the same way.
- **Copy to clipboard (`F2`)**: when `config.Mouse` is enabled, `tview.Application.EnableMouse(true)` prevents native terminal text selection (the app captures mouse events instead) — no mouse selection possible in this panel in that case. `F2` copies the entire displayed result regardless of the mouse setting, via the standard terminal mechanism **OSC 52** (`tcell.Screen.SetClipboard`): the local terminal receives an escape sequence asking it to copy to *its own* clipboard, which works even over SSH (the clipboard is never the remote server's). Confirmation shown in the status bar, but **with no guarantee the copy actually happened**: neither tcell nor the OSC 52 protocol return a confirmation, and support depends on the terminal (works on most modern terminals — Windows Terminal, iTerm2, recent GNOME Terminal/VTE... — but not on plain PuTTY, nor in tmux/screen without specific passthrough configuration). To be verified in real-world use.

## 4. Keyboard shortcuts

A help bar under the status bar reminds the shortcuts; `F1` lists them all.

| Action | Key | Status |
|---|---|---|
| Execute the request under the cursor | `Ctrl+E` (`Ctrl+Enter` also works on terminals that report it) | Defined |
| Switch focus left ↔ right panel | `Ctrl+←`/`Ctrl+→` | Defined |
| Quit the application (auto-saves the left panel, §3.2) | `Ctrl+C` | Defined |
| New request / clear the editor | Free-form editing of the left panel text | Defined |
| Open/change the cluster connection | Quit the program and relaunch it | Defined |
| Search in requests | `Ctrl+F` in the left panel | Defined |
| Search in the JSON result | `Ctrl+F` in the right panel | Defined |
| Resize the left/right split | `F5` (shrink the left) / `F6` (grow it) — `Ctrl+Shift+←/→` also works on terminals that report it | Defined |
| Save (left) / export (right) | `Ctrl+S`, behavior depends on the focused panel (§3.2, §3.3) | Defined |
| Open the recipe catalog | `F8` — type to filter, `↑`/`↓` to move, `Enter` to insert, `Esc` to close (§3.2) | Defined |
| Complete an endpoint while typing | `Tab`, `F10`, or `Ctrl+Space` in the left panel, on a `METHOD endpoint` line (§3.2) | Defined |
| Reformat (re-indent) the JSON body of the request under the cursor | `F4` in the left panel, no effect if there's no body or it isn't valid JSON; a `#` line inside or before the body can't be re-indented around and is reported | Defined |
| Copy the request under the cursor as an equivalent `curl` command | `F9` in the left panel — secrets replaced with a placeholder (§7 backlog #3) | Defined |
| Reload the hand-edited files (variables, recipes, endpoints) | `F7`, no panel restriction (§3.2) | Defined |
| Show help (how it works + shortcuts) | `F1`, `Esc` to close | Defined |
| Copy the result to the clipboard | `F2` (§3.3) | Defined |
| Switch the interface language (fr/en) | `F3` | Defined |

> `Ctrl+E` (and `Ctrl+Enter`, where the terminal reports it) is only active when the left panel (request editing) is focused — no effect from the right panel.
>
> **macOS**: `Ctrl+←/→` is intercepted at the OS level by default (Mission Control desktop switching); `Option`/`Alt+←/→` is accepted as a fallback for panel focus switch (see `hasShortcutModifier` in `ui/app.go`).
>
> **Mouse** (`mouse: true`, §9.2): a click gives the focus to the panel clicked, and the shortcuts that depend on the focused panel (`Ctrl+S`, `Ctrl+F`, `Ctrl+E`) follow it; a popup takes every click while it is open, and the completion list closes when the focus leaves it.
>
> Some combinations (`Ctrl+Enter`, `Ctrl+Shift+←/→`, `Tab`) are reported inconsistently — or not at all — depending on the terminal. Terminal-agnostic alternatives cover every case: `Ctrl+E` (execute), `F5`/`F6` (resize), `F10` (complete). **PuTTY is known to mishandle several shortcuts and is strongly discouraged** — no issues found on native macOS or Windows 10+ terminals. When in doubt, prefer the alternatives above.

## 5. Connecting to the cluster

- See §3.0 for the flow and §9.2 for the `config.yaml` schema.
- **Distribution and version**: read from the body of the `GET /` that validates the connection (`refdata.DetectTarget`), with no extra request.
  - `version.distribution` equal to `opensearch` → OpenSearch, at `version.number`.
  - Otherwise, a `tagline` mentioning OpenSearch → OpenSearch **without a version**: this is its compatibility mode (`compatibility.override_main_response_version`), which announces a fake `7.10.2` and drops the `distribution` field.
  - Otherwise, Elasticsearch's own `tagline` (`You Know, for Search`) → Elasticsearch at `version.number` — or without a version if `version.build_flavor` is `serverless` (Serverless has none that means anything).
  - Another value in `distribution` (a fork announcing itself), or an answer that is none of the above → unknown.
  - **Never a reason to refuse the connection**: an unknown distribution means nothing is filtered, a missing version that version bounds are ignored (§9.5) — offering too much is better than hiding what works.
  - **Override**: `distribution:` and `version:` on a cluster's entry in `config.yaml` (§9.2) replace what was detected, for a cluster behind a proxy that rewrites `GET /` or an OpenSearch in compatibility mode. Never written by the program; kept when the entry is moved to the top of the history. An invalid value is ignored with a warning in the status bar, and the connection proceeds with what was detected.
- **Validating the connection**: a `GET /` (15 seconds at most). The account used must therefore be allowed to run it — the `monitor` cluster privilege on Elasticsearch, `cluster:monitor/main` on OpenSearch; a `401` or a `403` is displayed as is.
- **Supported authentication**: none, Basic Auth (login/password), API Key (identifier and secret, sent as `Authorization: ApiKey base64(id:secret)`), client certificate (mTLS).
- **TLS**: certificate verification (CA located by default in `default_ca_dir`, path overridable per connection), option to skip it. TLS 1.2 at least (the default of Go's standard library). A CA file, when given, replaces the system's authorities for that connection.
- **Redirections are not followed**: a `301`, `302`, `307`… answer is returned as it is. Followed, a `301`/`302` turns a `POST`, `PUT` or `DELETE` into a `GET` of the new address without a word, and a `307`/`308` replays the request, body included, to wherever the answer points: neither is what the user wrote. `curl`, which `F9` gives the equivalent command for, doesn't follow them either.
- **No proxy**: the `HTTPS_PROXY`/`NO_PROXY` variables are not honored (§7).
- **Certificates**: two globally configurable default directories (`default_ca_dir`, `default_client_cert_dir`) to pre-fill paths when entering a new connection and to source the certificate picker popup (§3.0) — default to `/etc/pki/tls/certs` (RHEL/CentOS' standard TLS certificate directory) on Linux only, empty (nothing pre-filled) on Windows and macOS since that path doesn't exist there; overridable or clearable (`""`) per §9.2. Neither setting is a gate: if the configured directory doesn't exist on disk, the picker falls back to browsing the user's home directory instead of erroring out, and the field itself can always be typed by hand for a certificate kept anywhere else. Only when even that fallback isn't usable does the picker report a clear error naming the setting, not a raw OS error.
- **Client key protected by a passphrase**: both formats are read — the encrypted PKCS#8 format (`BEGIN ENCRYPTED PRIVATE KEY`, what OpenSSL has written by default since 1.1.0) and the legacy encrypted PEM format (`BEGIN RSA PRIVATE KEY` with a `DEK-Info` header). For PKCS#8, the standard library having no support for it, the PBES2 scheme is implemented in `esclient/pkcs8.go`: a key derived by PBKDF2 (HMAC-SHA1 to SHA-512), encryption by AES-128/192/256 or triple DES in CBC mode — every combination `openssl pkcs8 -topk8 -v2` produces, checked against keys written by OpenSSL itself. Keys derived with scrypt, and the older PBES1 and PKCS#12 schemes, are refused with an error that names what isn't supported; a wrong passphrase is reported as such.
- **Secret storage**: none — password, API key secret, and private key passphrase are re-requested on every connection; only non-sensitive elements (URL, auth type, username, API key ID, CA/cert paths) are persisted in `config.yaml`, with the most recently used entry at the top of the list. The same rule extends to the "copy as cURL" feature (`F9`, §4): the generated command includes the real, non-sensitive auth details (username, API key ID, certificate/key paths) but replaces the actual secret with a placeholder — a clipboard copy has no guardrail equivalent to "never written to disk." A corollary: a URL carrying credentials is refused (§3.0), since the URL itself is saved.
- **What is trusted, and what is not**: the binary's directory and the user's configuration directory are trusted inputs — what is put there (recipes, endpoints, cheatsheet) is offered to the user; the binary must therefore not be installed in a directory anyone can write to. Those files are nonetheless refused as a whole if they hold control characters (§9.5). The cluster is not trusted: its answers are displayed without being interpreted (§3.3), bounded in size (§3.3), and the column names it supplies to completion are filtered (§3.2).

## 6. Supported requests

- **Input syntax**: free-form, Kibana Console style (`METHOD path` + optional JSON body on the following lines).
- **HTTP methods to support**: GET, POST, PUT, DELETE.
- **Validation before sending**: check that the body's JSON is valid before executing.
- **Default timeout**: 2 minutes, configurable via `default_timeout_seconds` in `config.yaml` (§9.2).

## 7. Out of scope, and roadmap

What isn't done, by choice or not yet:

- Live-updating timer for the in-progress request in the status bar (only the final result is shown: HTTP code + total duration once the response is received) — item 7 below.
- Dynamic auto-completion of the connected cluster's real index names (completion is limited to known endpoints and `_cat` columns, see §3.2) — item 10 below.
- Syntax highlighting in the editor (dropped, see §3.2).

**Candidates inspired by Kibana Dev Tools** — all shipped as of this writing, roughly in decreasing-interest order as originally proposed: ~~Reformat the request body's JSON in place~~ (Kibana's "auto indent") as `F4` (§4); ~~Show the response's HTTP headers~~ as part of the result panel (§3.3); ~~"Copy as cURL"~~ as `F9` (§4) — secrets redacted, see `esclient.Client.CurlCommand`; ~~Line numbers in the editor gutter~~ as part of the editor panel (§3.2); ~~Reusable variables~~ as `${name}` substitution, reloaded with `F7` (§3.2, §9.1); ~~Auto-closing brackets/quotes~~ while typing in the editor (§3.2, `ui/autoclose.go`).

**Roadmap after v0.5** — recorded on 2026-10-01 after comparing with geek-fun/dockit, cars10/elasticvue and elastic/cli. The order is the intended sequence; every item is still to be refined before it is built.

1. ~~**Built-in, version-aware reference data and recipes**~~ — shipped: recipes, endpoints and `_cat` columns inside the binary, selected from the distribution (Elasticsearch or OpenSearch) and version detected at connection, extended by the user's own files and reloaded with `F7` (§3.2, §5, §9.1, §9.5). Nothing but the binary to install. Brings self-managed OpenSearch into scope.
2. **HTTP(S) proxy** — honor `HTTPS_PROXY`/`NO_PROXY`, optionally a per-cluster `proxy:`.
3. **Extended authentication** — API key in its `encoded` form, Bearer tokens, Cloud ID. SigV4 stays out.
4. **Production guardrails** — per-cluster read-only mode, configurable confirmation for destructive paths, a visible "PROD" banner.
5. **Secrets without retyping, first tier** — read the secret from an environment variable or a file. An opt-in OS keychain is a separate, later decision (it changes the "no secret ever persisted" promise of §5).
6. **Watch mode** — re-run the request under the cursor every N seconds and highlight what changed.
7. **Cancel a running request, live timer** — covers the "live-updating timer" bullet above.
8. **Diff between two runs** of the same request.
9. **Cluster "top" screen** — health, nodes, unassigned/relocating shards, running tasks, thread-pool rejections, auto-refreshed.
10. **Dynamic completion** — index, alias, data stream, field, node and repository names; covers the "dynamic auto-completion" bullet above.
11. **Headless mode** — `run -f file --json`, no TUI; only worthwhile once item 5 exists.
12. **Large-JSON navigation** — folding, jump to path, jq-like filter.

Not scheduled yet, in decreasing interest: URL-parameter and query-DSL completion; execution history; ES|QL/SQL tabular rendering; multiple buffers and arbitrary files; tabular view with CSV/Markdown export; multi-cluster comparison; bulk import/export; a configurable export directory, should the user's (§9.1) not be enough; **AWS SigV4 request signing**, which is what OpenSearch managed by AWS (Amazon OpenSearch Service, Serverless) requires when it is secured by IAM — left out of item 1 because it is neither small nor self-contained: every request must be signed (canonical request, payload hash, date-scoped key), and credentials come from a chain of sources (environment, shared profile, SSO, instance or container role, with session tokens that expire mid-session) that either pulls in the AWS SDK, at odds with the dependency-free static binary, or has to be re-implemented. A managed domain that accepts Basic Auth (fine-grained access control with an internal user database) should be reachable as any other OpenSearch — not tested.

Deliberately left out: AI assistant or MCP server; forms for snapshots/ILM/templates; built-in SSH tunnels; non-Elasticsearch-API backends (DynamoDB, MongoDB…).

## 8. Non-functional constraints

- **Runtime dependencies**: none — the standard libc present on RHEL 8/9/10 (or any other Linux amd64 distribution) on that platform, nothing at all to install on Windows or macOS.
- **Performance**: must support large results (several MB).
- **Packaging**: a single binary to copy, with no file to install next to it, published with its SHA-256 checksum (`SHA256SUMS`). `termdevtools --version` tells which version it is, before any file is read or created. `termdevtools --export-defaults <directory>` writes the data built into it out as files — to read what is built in, or as examples for one's own files (§9.1); it refuses to overwrite an existing file, and to write into either directory reference files are read from (the built-in recipes would come back as the user's own, frozen copies hiding the corrections of later versions).
- **Project/binary name**: termdevtools.

## 9. Technical architecture

### 9.1 File locations

**Nothing is required next to the binary**: every file below is either created by the program or an optional addition. What the program writes into the files it creates — the comments documenting `config.yaml`, the variables file and the recipe template — is in English, whatever the interface language. Connection history is specific to the user (two people launching the same shared binary on the same server shouldn't step on each other), whereas a team may want to share recipes with everyone using one installation. Hence two separate locations, read in that order of priority after the data built into the binary (§9.5):

- **User configuration directory** (`~/.config/termdevtools/`, or `$XDG_CONFIG_HOME/termdevtools/` if that variable is set), created automatically (`0700` permissions) on first write:
  - `config.yaml` — known clusters, updated automatically on every successful connection, no secret in it (§9.2). If the file doesn't exist yet, it's created on startup with every setting shown at its default value, a comment above each explaining what it does — self-documenting, so a user can discover what's configurable without reading this spec. Existing values are never touched; if the file does exist but is missing one or more settings (e.g. saved by an older version of the program, before some setting existed, or hand-trimmed down to a couple of keys), the missing ones are appended the same way, so it stays fully self-documenting as the program evolves. Saving it (every successful connection, `F3`) updates the values in place: the comments, and any key this version doesn't know, are kept — as is a default directory deliberately set to `""`.
  - `queries_<sanitized URL>.txt` — one file per cluster already used by this user, containing the latest save of the left panel for that cluster (§3.2). Written by `Ctrl+S` and automatically on program exit. Name built from the cluster's URL, replacing with `_` any character that isn't alphanumeric, `.`, `_`, or `-` (so notably `:` and `/`) — e.g. `https://es-prod.example.com:9200` → `queries_https___es-prod.example.com_9200.txt`. Two different URLs that happened to be similar enough to produce the same name after this normalization would (rare case) share the same file — an accepted limitation to keep names readable rather than hashed.
  - `variables_<sanitized URL>.txt` — one file per cluster already used by this user, holding the reusable `${name}` values substituted into requests (§3.2, §7 backlog #4) — same one-file-per-cluster-per-user scheme and filename sanitization as `queries_*.txt`, just a different prefix (`config.VariablesPathForURL`). Hand-edited (no in-app editor), self-documenting: created with an explanatory comment the first time this user connects to this cluster, if it doesn't exist yet. Loaded on connection, reloaded on demand with `F7` (no file-watching).
  - `recipes/*.txt` — the user's own recipes, added to the catalog (§3.2; format and merge rules in §9.5). Every `.txt` file of the directory is read, in name order. The directory is created on first launch with `my-recipes.txt`, a template made of comments only — it explains the format and adds nothing until edited. It is written once: a user who deletes the file isn't given it again as long as the directory remains.
  - `endpoints.txt` — the user's own endpoints, added to those offered by completion (§3.2). Optional, never created by the program.
  - `exports/` — results exported via `Ctrl+S` from the right panel, one timestamped file per export (created on demand, §3.3). Each user's own, like the rest of the directory: exporting doesn't depend on the rights on the binary's directory, and a cluster's results can't be read by the other users of the installation. Up to 0.6 this directory was the binary's, and exporting failed in a shared or read-only installation (`/opt`, `/usr/local/bin`).
  - `crash-<timestamp>.log` — written only if the program panics: the recovered value and a full stack trace (`recoverCrash` in `main.go`), for diagnosing a crash on a terminal nobody watching can reproduce or transcribe by hand. If the file can't be written, the report is printed instead. Best-effort — a panic in tcell's own input-reading goroutine, rather than in code `tview.Application.Run` processes itself, isn't caught this way.
- **Executable's directory** (the binary's, not the shell's current working directory) — all optional, and the program writes nothing there:
  - `recipes/*.txt`, `endpoints.txt` — same formats as above, for content shared by everyone using that installation (a team's own recipes). The user's files come on top of them.
  - `cheatsheet.txt` — a team's own starting content for the editor, loaded only if no `queries_*.txt` save yet exists for the current cluster/user, in place of the built-in starter (§3.2).
- **Files of versions up to 0.5**, which installed three companion files next to the binary: `cat_columns.txt` is no longer read; `endpoints.txt` is read as the team's additions (above) instead of replacing the built-in list — harmless, since a line without a tag never removes the version constraint of the built-in entry it duplicates (§9.5); `cheatsheet.txt` keeps its role. The repository no longer carries `endpoints.txt`, `cat_columns.txt` nor `cheatsheet.txt.example` at its root, and the install scripts no longer copy anything but the binary — they only point out such leftovers. The same goes for an `exports/` directory left next to the binary by a version up to 0.6: it is no longer written to, and is neither moved nor deleted.

### 9.2 `config.yaml` schema (`~/.config/termdevtools/config.yaml`)

```yaml
default_timeout_seconds: 120
language: en  # interface language: "en" (default) or "fr" — see the i18n package; also switchable live with F3, which rewrites this line
mouse: false  # mouse support, off by default (see §3.3) — everything has a keyboard equivalent
default_ca_dir: ""          # pre-fills the CA field for a new connection and the certificate picker (§3.0) — defaults to /etc/pki/tls/certs (RHEL/CentOS' standard TLS cert directory) on Linux, "" (disabled) on Windows/macOS
default_client_cert_dir: "" # pre-fills the client cert/key fields (mTLS) and the certificate picker — same default rule

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

  - url: https://search-behind-proxy.example.com
    auth_type: none
    distribution: opensearch # optional: elasticsearch | opensearch — replaces the detection (§5)
    version: "2.19"          # optional: replaces the detected version
    tls:
      verify: true
```

### 9.3 Project structure

```
termdevtools/
├── main.go                 // entry point: loads config, launches the connection screen, then the UI
├── go.mod
├── config/
│   └── config.go           // reads/writes config.yaml, moves the used entry to the top of the list
├── i18n/
│   └── i18n.go             // fr/en message catalogs for the interface, selected via config.Language
├── esclient/
│   ├── client.go           // HTTP client (auth none/basic/api_key/mtls, TLS), executes a request
│   ├── curl.go             // the request as an equivalent curl command, secrets redacted
│   └── pkcs8.go            // decryption of client keys in the encrypted PKCS#8 format
├── parser/
│   └── parser.go           // splits the editor content into requests (method, endpoint, payload, comments)
├── refdata/                // reference data built into the binary (§9.5)
│   ├── target.go           // distribution + version, detection from "GET /"
│   ├── constraint.go       // "@es >=8.7 <9.0" tags and their matching
│   ├── endpoints.go        // endpoints.txt format, merge of layers
│   ├── catcolumns.go       // cat_columns.txt format, parsing of "GET _cat" and "?help"
│   ├── recipes.go          // recipe file format, merge of layers
│   ├── catalog.go          // embedded data + team's and user's files, selection for a target
│   ├── export.go           // --export-defaults, user's recipe template
│   └── data/               // the embedded files: endpoints.txt, cat_columns.txt, starter.txt, my-recipes.txt, recipes/*.txt
├── ui/
│   ├── connect.go          // initial connection screen (tview.Form)
│   ├── app.go              // Flex assembly, focus management, global shortcuts
│   ├── editor.go           // left panel (TextArea)
│   ├── completion.go       // prefix filtering, h=/s= detection for _cat commands
│   ├── recipes.go          // recipe catalog popup (F8)
│   ├── result.go           // right panel (TextView + JSON highlighting)
│   └── statusbar.go        // status bar + shortcuts help bar
├── tools/                  // developer tools, not shipped
│   ├── genrefdata/         // generates refdata/data/endpoints.txt and cat_columns.txt
│   └── testclusters.sh     // starts/stops the real clusters the integration tests run against
├── internal/testclusters/  // the list of those clusters, shared by the generator and the tests
└── config.yaml.example
```

### 9.4 Request execution flow

1. Left panel focused, cursor positioned on a request, `Ctrl+E` (or `Ctrl+Enter`).
2. `parser` extracts method + endpoint + JSON payload around the cursor and validates the JSON.
3. Invalid JSON → error message in the status bar, nothing is sent.
4. Valid JSON → HTTP call launched in a goroutine; status bar → "request in progress...".
5. Response received → `esclient` returns the HTTP code, duration, body; UI updated via `QueueUpdateDraw` (thread-safe with tview): right panel filled in (pretty-printed and highlighted JSON, or plain text for `_cat` responses), status bar → HTTP code + duration.

### 9.5 Reference data (package `refdata`)

What completion and the recipe catalog offer is data, not code: plain text files under `refdata/data/`, compiled into the binary (`go:embed`) and selected, once connected, for the cluster's distribution and version (§5).

- **Why plain embedded files.** A small user-level SQLite database was considered and dropped: the whole set is a few hundred read-only lines; text files stay hand-editable, diffable and reviewable, and the same format serves the user's own additions; a database would add either cgo (losing the static binary) or a large pure-Go driver, for no query the tool needs. One set of files per version was dropped too — the supported window holds some thirty minor versions — in favor of one catalog whose entries each say where they apply.
- **Constraints.** An entry may be followed by tags: `@es`, `@es >=8.7`, `@es >=7.17 <9.0`, `@opensearch >=2.4` (`@elasticsearch` and `@os` are accepted too). A bound is a version, `>=` inclusive or `<` exclusive; several intervals may be given for one distribution.
  - No tag: the entry applies everywhere.
  - Tags for one distribution only: the entry isn't offered on the other.
  - Matching **fails open**: on a cluster whose distribution is unknown nothing is filtered out, and on one whose distribution is known but not its version, version bounds are ignored.
- **Files** (under `refdata/data/`):
  - `endpoints.txt` — one endpoint per line, optional tags. Generated, not hand-edited.
  - `cat_columns.txt` — `# _cat/<command> [tags]` sections, then one column per line with optional tags. Generated. Only the fallback of §3.2.
  - `recipes/*.txt` — the catalog, one file per theme, listed in the order of the file names.
  - `starter.txt` — the editor's content on a first connection (§3.2).
  - `my-recipes.txt` — the template written into the user's `recipes/` directory (§9.1).
- **Recipe file format** — the editor's own (requests and `#` comments) plus directives written as comments, so that a recipe file can be pasted into the editor as is:
  - `# @group <name>` — theme of the recipes that follow; without it, the file's name.
  - `# @recipe <title>` — starts a recipe, which runs up to the next `@recipe` or `@group`.
  - `# @tags <words>` — extra words for the catalog's filter.
  - `# @es ...`, `# @opensearch ...` — the recipe's constraint; before the first `@recipe`, the default for the whole file.
  - Any other `# @word` line is an ordinary comment. A file without any `@recipe` is a single recipe named after the file, provided it holds a request; an `@recipe` without a request is reported as a problem.
- **Layers.** Embedded data first, then the team's files (binary's directory), then the user's (configuration directory) — `endpoints.txt` and `recipes/*.txt` only (§9.1). Layers are **merged**, a file never replaces the built-in data as a whole:
  - *Endpoints*: union. An endpoint already known keeps its constraint unless the higher layer states one — so a line without a tag (typically from the `endpoints.txt` of an older installation) cannot make a version-specific endpoint appear everywhere.
  - *Recipes*: a recipe with the same group and title (ignoring case) as recipes of a lower layer replaces them — all of them, since a title may exist there in one variant per distribution. Any other recipe is added at the end of its group, or in a new group after the existing ones.
  - A line that can't be read is skipped and reported — file and line — in the status bar, at connection and on `F7`; the rest of the file still loads. So is what would otherwise be dropped or misread in silence: a request, `@tags` or a constraint written outside any recipe, a recipe whose only "request" is a line the editor wouldn't run, an interval no version can be in (`>=9.0 <8.0`). A file that isn't UTF-8 text (typically UTF-16, as written by `>` in Windows PowerShell) is reported and ignored as a whole; a UTF-8 byte order mark is tolerated. So is a file holding a control character other than a tab or an end of line: an escape sequence, which no editor shows, would otherwise end up in the requests the recipe is inserted into. A mistake in the embedded data itself is a build error, caught by the package's tests.
- **Generation** (`tools/genrefdata`, a developer tool):
  - `endpoints` reads the official API specifications — `output/schema/schema.json` of every minor branch of elastic/elasticsearch-specification from `7.17` to `9.5`, and the OpenSearch OpenAPI document — keeps the namespaces of §3.2 and the endpoints without path parameter, and derives each constraint from the specification's own "since" annotation when there is one, otherwise from the branches the endpoint is present in. A short table of **overrides**, each with its reason, corrects what real clusters contradict (a wrong "since", a branch lagging behind the product, approximate `x-version-added` values on the OpenSearch side).
  - `catcolumns` asks the real clusters below (`GET _cat`, then `GET _cat/<command>?help`).
  - `explain <text>` shows where an entry's constraint comes from.
- **Ground truth.** `tools/testclusters.sh up` starts eleven single-node containers, security disabled: Elasticsearch 7.17.29, 8.0.1, 8.11.4, 8.19.22, 9.0.8 and 9.5.4; OpenSearch 2.0.1, 2.11.1, 2.19.6, 3.0.0 and 3.9.0. The integration tests (`go test -tags integration ./refdata/`) check, on each of them:
  - that the distribution and version are detected;
  - that every endpoint offered for it exists there, and that none of those hidden from it answers;
  - that the built-in `_cat` table names the cluster's commands and no column it doesn't have;
  - every recipe offered for it, by **running each of its requests** — against a small fixture: an index with an alias and a block, a snapshot repository, a snapshot — and by checking every `_cat` column it names against the cluster's own `?help` (an unknown column is silently ignored by `_cat`, so only this catches a typo). An answer that is a bare `{}` fails too, unless listed as expected: it is how a filter that selects nothing shows.

  These tests are **destructive** — they change cluster settings, create, restore and delete indices and snapshots — and refuse any cluster that isn't on the local machine unless `TDT_IT_ALLOW_REMOTE=1` is set. The containers are published on the loopback interface only.
- **Precision between two tested versions**: conservative. An endpoint or column that appeared somewhere between two tested versions is dated by the specification when it says so, otherwise by the first tested version where it exists — better not offered than offered wrongly. `_cat` columns escape the question entirely at run time, being asked from the cluster.
- **A cluster newer than the binary** gets whatever has no upper bound, that is what applied to the most recent version known at build time.

## 10. Open questions

No blocking question identified at this stage. Section kept available for any question that might come up during future work.
