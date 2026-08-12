package ui

import "strings"

// findNext cherche la prochaine occurrence de query (insensible à la casse)
// dans text, à partir de l'offset en runes after (exclu), avec retour au
// début du texte si rien n'est trouvé après. Les offsets renvoyés sont en
// runes, compatibles avec TextArea.Select.
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
