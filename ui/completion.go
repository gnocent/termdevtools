package ui

import (
	"sort"
	"strings"
)

// The data completion draws from — endpoints, _cat commands and their
// columns — lives in the refdata package (built into the binary, selected
// for the connected cluster). This file only holds the matching logic.

// matchPrefix returns, among candidates, those starting with prefix
// (case-insensitive), sorted. An empty prefix returns the full list. Used
// both for endpoints and for _cat columns (h=/s=) and sort directions
// (asc/desc).
func matchPrefix(prefix string, candidates []string) []string {
	lower := strings.ToLower(prefix)
	var matches []string
	for _, c := range candidates {
		if strings.HasPrefix(strings.ToLower(c), lower) {
			matches = append(matches, c)
		}
	}
	sort.Strings(matches)
	return matches
}

// sortDirections are the possible values for a column's sort direction in
// the s= parameter (e.g. "s=docs.count:desc").
var sortDirections = []string{"asc", "desc"}

// matchCatCommand looks, among the keys of columns, for the longest (most
// "covering") _cat command that prefixes path — either an exact match, or
// followed by a '/' (never a partial word match: "shardsxyz" must not
// match "shards"). Necessary because many _cat commands accept a filter at
// the end of the path before the parameters, e.g. "_cat/shards/myindex?h=..."
// (filtering on index "myindex") — without this, "shards/myindex" would
// match no known command and no column would be suggested.
func matchCatCommand(path string, columns map[string][]string) (command string, ok bool) {
	for cmd := range columns {
		if cmd == "" {
			continue
		}
		if path == cmd || strings.HasPrefix(path, cmd+"/") {
			if len(cmd) > len(command) {
				command = cmd
				ok = true
			}
		}
	}
	return command, ok
}

// catEdit describes an in-progress edit inside the h= (displayed columns)
// or s= (sort) parameter of a _cat command.
type catEdit struct {
	// command is the _cat command recognized ("indices", "ml/datafeeds").
	command string
	// segment is the text being completed: the last column typed so far,
	// or, once direction is set, the beginning of "asc"/"desc".
	segment string
	// direction is set once a ':' follows the column in s=: the sort
	// direction is being typed, not a column name.
	direction bool
}

// parseCatEdit attempts to interpret prefix (the text typed after the HTTP
// method, see Editor.CompletionPrefix) as an in-progress edit inside the
// h= or s= parameter of a _cat command, e.g. "_cat/indices?h=health,st" or
// "_cat/shards?s=docs.count:de". commands' keys are the known _cat commands
// (see matchCatCommand). Returns ok=false if prefix doesn't match this case
// (a regular endpoint, not a known _cat command, or not inside an h=/s=
// parameter) — the caller then falls back to regular endpoint completion.
func parseCatEdit(prefix string, commands map[string][]string) (edit catEdit, ok bool) {
	if !strings.HasPrefix(prefix, "_cat/") {
		return catEdit{}, false
	}
	qIdx := strings.IndexByte(prefix, '?')
	if qIdx < 0 {
		return catEdit{}, false
	}
	path := prefix[len("_cat/"):qIdx]
	query := prefix[qIdx+1:]

	// Many _cat commands accept a filter at the end of the path before the
	// parameters (e.g. "_cat/shards/myindex?h=..." filtering on index
	// "myindex"). So we recognize the longest (most "covering") _cat
	// command that prefixes path at a '/' boundary — not a partial word
	// match (e.g. "shardsxyz" must not match "shards").
	command, ok := matchCatCommand(path, commands)
	if !ok {
		return catEdit{}, false
	}

	// The parameter currently being typed: whatever follows the last '&'
	// (or the whole query string if there is none).
	param := query
	if amp := strings.LastIndexByte(query, '&'); amp >= 0 {
		param = query[amp+1:]
	}

	var value string
	var isSortParam bool
	switch {
	case strings.HasPrefix(param, "h="):
		value = param[len("h="):]
	case strings.HasPrefix(param, "s="):
		value = param[len("s="):]
		isSortParam = true
	default:
		return catEdit{}, false
	}

	// Comma-separated columns: only the last one, being typed, should be
	// completed.
	segment := value
	if comma := strings.LastIndexByte(value, ','); comma >= 0 {
		segment = value[comma+1:]
	}

	// s=column:asc|desc — once ':' is typed, we complete the direction, not
	// the column name (already fully typed at this point).
	if isSortParam {
		if colon := strings.IndexByte(segment, ':'); colon >= 0 {
			return catEdit{command: command, segment: segment[colon+1:], direction: true}, true
		}
	}
	return catEdit{command: command, segment: segment}, true
}

// catColumnCompletion returns the candidates for the _cat edit in progress
// in prefix (see parseCatEdit), taking the command's columns from columns.
// subPrefixLen (in bytes, see Editor.CursorOffset for why bytes and not
// runes) allows computing the portion to replace: only the column (or
// direction) being typed, not everything before it. ok=false when prefix
// isn't such an edit, or when no column is known for the command.
func catColumnCompletion(prefix string, columns map[string][]string) (candidates []string, subPrefixLen int, ok bool) {
	edit, ok := parseCatEdit(prefix, columns)
	if !ok {
		return nil, 0, false
	}
	if edit.direction {
		return matchPrefix(edit.segment, sortDirections), len(edit.segment), true
	}
	cols := columns[edit.command]
	if len(cols) == 0 {
		return nil, 0, false
	}
	return matchPrefix(edit.segment, cols), len(edit.segment), true
}
