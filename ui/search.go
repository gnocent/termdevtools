package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// foldCase lowercases s without ever changing its length in bytes: a rune
// whose lowercase form is encoded on a different number of bytes ("İ", two
// bytes, becomes "i̇", three) is left as it is. What is found at an offset
// in the folded text is then at the same offset in the original — which
// strings.ToLower doesn't guarantee.
func foldCase(s string) string {
	return strings.Map(func(r rune) rune {
		if lower := unicode.ToLower(r); utf8.RuneLen(lower) == utf8.RuneLen(r) {
			return lower
		}
		return r
	}, s)
}

// findNext looks for the next occurrence of query (case-insensitive) in
// text, starting from the byte offset after (exclusive), wrapping back to
// the start of the text if nothing is found afterwards. The returned
// offsets are byte offsets, compatible with TextArea.Select/Replace — which
// count UTF-8 bytes internally (confirmed in tview's own source: position
// tracking advances by len(cluster), a string's byte length, not a rune
// count), not runes. Case is folded by foldCase, so that the offsets found
// in the folded text are those of the original.
func findNext(text, query string, after int) (start, end int, found bool) {
	if query == "" {
		return 0, 0, false
	}
	lower := foldCase(text)
	lowerQuery := foldCase(query)

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
