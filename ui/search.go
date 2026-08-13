package ui

import "strings"

// findNext looks for the next occurrence of query (case-insensitive) in
// text, starting from the rune offset after (exclusive), wrapping back to
// the start of the text if nothing is found afterwards. The returned
// offsets are in runes, compatible with TextArea.Select.
func findNext(text, query string, after int) (start, end int, found bool) {
	if query == "" {
		return 0, 0, false
	}
	lowerRunes := []rune(strings.ToLower(text))
	queryRunes := []rune(strings.ToLower(query))

	search := func(from int) (int, bool) {
		for i := from; i+len(queryRunes) <= len(lowerRunes); i++ {
			if runesEqual(lowerRunes[i:i+len(queryRunes)], queryRunes) {
				return i, true
			}
		}
		return 0, false
	}

	if idx, ok := search(after + 1); ok {
		return idx, idx + len(queryRunes), true
	}
	if idx, ok := search(0); ok {
		return idx, idx + len(queryRunes), true
	}
	return 0, 0, false
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
