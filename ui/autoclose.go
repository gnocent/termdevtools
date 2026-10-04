package ui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// autoCloseOpen maps each opening character this editor auto-closes to its
// matching closer (SPEC.md §7 backlog #6). "\"" maps to itself: quotes open
// and close with the same character, unlike brackets.
var autoCloseOpen = map[byte]byte{
	'{': '}',
	'[': ']',
	'"': '"',
}

// wireAutoClose installs auto-closing of "{", "[", and "\"" on e's
// TextArea: typing one of them inserts the matching closer too, cursor
// placed in between; typing a closer that's already right there just steps
// over it instead of inserting a redundant second one; Backspace between an
// empty pair removes both sides in one go. Deliberately conservative
// (SPEC.md §7 flags this as the highest UX-risk item on the backlog if the
// pairing logic isn't handled carefully): does nothing at all while a
// selection is active (typing/deleting a selection keeps TextArea's own,
// unrelated default behavior), and never closes a bracket typed inside an
// already-open string ('{' or '[' as plain string content, e.g. typing a
// literal "{" as part of a JSON string value).
func (e *Editor) wireAutoClose() {
	e.view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if e.view.HasSelection() {
			return event
		}
		switch event.Key() {
		case tcell.KeyRune:
			if e.handleAutoCloseRune(event.Rune()) {
				return nil
			}
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if e.deleteEmptyPairAtCursor() {
				return nil
			}
		}
		return event
	})
}

// handleAutoCloseRune implements the actual per-character decision (see
// wireAutoClose); returns true if it inserted/moved the cursor itself, in
// which case the caller must swallow the original key event — false to let
// it fall through to TextArea's normal typing behavior.
func (e *Editor) handleAutoCloseRune(r rune) bool {
	if r > 127 {
		return false // every character this handles is plain ASCII
	}
	b := byte(r)

	switch b {
	case '{', '[':
		if e.insideString() {
			// Plain content of a string ("age:[10 TO *]"): not a structure
			// to close.
			return false
		}
		return e.insertPair(b, autoCloseOpen[b])
	case '"':
		if e.skipOverIfNext(b) {
			return true
		}
		if e.insideString() {
			// Plain typing: this quote most likely closes the string
			// that's currently open, or is an escaped literal — either
			// way, not a new pair to open.
			return false
		}
		return e.insertPair(b, b)
	case '}', ']':
		return e.skipOverIfNext(b)
	}
	return false
}

// insertPair inserts open immediately followed by close at the cursor, then
// places the cursor right between them (e.g. typing "{" inserts "{}" with
// the cursor after the "{").
func (e *Editor) insertPair(open, close byte) bool {
	pos := e.CursorOffset()
	e.view.Replace(pos, pos, string(open)+string(close))
	newPos := pos + 1 // both open and close are single-byte (ASCII)
	e.view.Select(newPos, newPos)
	return true
}

// skipOverIfNext moves the cursor past b, without inserting anything, if b
// is exactly the byte immediately after the cursor — typing a closing
// bracket/quote right where one was already there (typically auto-inserted
// by insertPair a moment ago) should step over it, not insert a redundant
// second one.
func (e *Editor) skipOverIfNext(b byte) bool {
	text := e.Text()
	pos := e.CursorOffset()
	if pos >= len(text) || text[pos] != b {
		return false
	}
	newPos := pos + 1
	e.view.Select(newPos, newPos)
	return true
}

// deleteEmptyPairAtCursor deletes both characters of an empty pair (e.g.
// "{}", "[]", or two adjacent quotes) in one Backspace, when the cursor
// sits exactly between them — otherwise a plain Backspace would remove only
// the opener and leave a dangling, empty-looking closer behind. Returns
// false (event not handled, falls through to a normal single-character
// Backspace) for anything else, including a non-empty pair: "{a}" with the
// cursor right after "a" is not collapsed, only a genuinely empty one is.
func (e *Editor) deleteEmptyPairAtCursor() bool {
	text := e.Text()
	pos := e.CursorOffset()
	if pos <= 0 || pos >= len(text) {
		return false
	}
	before, after := text[pos-1], text[pos]
	if autoCloseOpen[before] != after {
		return false
	}
	e.view.Replace(pos-1, pos+1, "")
	e.view.Select(pos-1, pos-1)
	return true
}

// insideString reports whether the cursor sits inside an open (not yet
// closed) JSON string literal on the current line — found by counting
// unescaped '"' characters from the start of the line up to the cursor: an
// odd count means the last one seen opened a string still awaiting its
// closer. A single line is always enough context: an unescaped newline
// inside a JSON string is invalid JSON, so a string literal never
// legitimately spans more than one line to begin with.
func (e *Editor) insideString() bool {
	text := e.Text()
	row, col := lineColAt(text, e.CursorOffset())
	lines := strings.Split(text, "\n")
	if row < 0 || row >= len(lines) {
		return false
	}
	line := lines[row]
	if col > len(line) {
		col = len(line)
	}

	count := 0
	escaped := false
	for i := 0; i < col; i++ {
		if escaped {
			escaped = false
			continue
		}
		switch line[i] {
		case '\\':
			escaped = true
		case '"':
			count++
		}
	}
	return count%2 == 1
}
