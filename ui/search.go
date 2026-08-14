package ui

import "strings"

// findNext looks for the next occurrence of query (case-insensitive) in
// text, starting from the byte offset after (exclusive), wrapping back to
// the start of the text if nothing is found afterwards. The returned
// offsets are byte offsets, compatible with TextArea.Select/Replace — which
// count UTF-8 bytes internally (confirmed in tview's own source: position
// tracking advances by len(cluster), a string's byte length, not a rune
// count), not runes. Assumes strings.ToLower does not change text's byte
// length, true in practice for the content this app deals with (JSON,
// endpoint paths, French comments) — same assumption already relied on
// implicitly before this function counted bytes instead of runes.
func findNext(text, query string, after int) (start, end int, found bool) {
	if query == "" {
		return 0, 0, false
	}
	lower := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)

	search := func(from int) (int, bool) {
		if from < 0 {
			from = 0
		}
		if from > len(lower) {
			return 0, false
		}
		idx := strings.Index(lower[from:], lowerQuery)
		if idx < 0 {
			return 0, false
		}
		return from + idx, true
	}

	if idx, ok := search(after + 1); ok {
		return idx, idx + len(query), true
	}
	if idx, ok := search(0); ok {
		return idx, idx + len(query), true
	}
	return 0, 0, false
}
