package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rivo/tview"

	"termdevtools/i18n"
)

// Editor is the left panel: the request editor. See SPEC.md §3.2.
type Editor struct {
	view *tview.TextArea
}

// NewEditor creates an empty editor.
func NewEditor(msgs *i18n.Strings) *Editor {
	area := tview.NewTextArea().SetWrap(true)
	area.SetBorder(true).SetTitle(msgs.EditorTitle)
	return &Editor{view: area}
}

// Widget returns the tview component to insert into the layout.
func (e *Editor) Widget() tview.Primitive {
	return e.view
}

// Primitive returns the underlying TextArea (for SetFocus, focus comparisons...).
func (e *Editor) Primitive() tview.Primitive {
	return e.view
}

// SetLanguage re-applies the panel's chrome (border title) in msgs' language
// — the content itself (the requests being edited) is untouched.
func (e *Editor) SetLanguage(msgs *i18n.Strings) {
	e.view.SetTitle(msgs.EditorTitle)
}

// Text returns the editor's full content.
func (e *Editor) Text() string {
	return e.view.GetText()
}

// CursorOffset returns the cursor position as an absolute (rune) offset in
// the full text. Unlike TextArea.GetCursor (whose "line" becomes a display
// line, not a logical line, as soon as automatic word wrap is active — see
// SPEC.md §3.1), GetSelection returns a position independent of visual
// rendering: "if there is no selection, start and end are the cursor
// position" (tview's official documentation). With an active selection,
// returns the end closest to the moving bound — no practical impact since
// Ctrl+Enter and Tab only use it when there's no selection.
func (e *Editor) CursorOffset() int {
	_, _, end := e.view.GetSelection()
	return end
}

// lineColAt returns the logical line (0-indexed, split on real "\n") and
// column (in runes) corresponding to the absolute offset offset in text.
// Purely textual, so independent of visual word wrap.
func lineColAt(text string, offset int) (row, col int) {
	lines := strings.Split(text, "\n")
	consumed := 0
	for i, line := range lines {
		lineLen := len([]rune(line))
		if offset <= consumed+lineLen {
			return i, offset - consumed
		}
		consumed += lineLen + 1 // +1 for the "\n" separator
	}
	last := len(lines) - 1
	return last, len([]rune(lines[last]))
}

// CursorLine returns the (0-indexed) index of the logical line the cursor
// is on — the actual text's line, not the display line after automatic
// word wrap (SetWrap, see SPEC.md §3.1).
func (e *Editor) CursorLine() int {
	row, _ := lineColAt(e.Text(), e.CursorOffset())
	return row
}

// LoadFile loads path into the editor if it exists, and returns true if
// content was actually loaded. Its absence is not an error — used both for
// the cheatsheet and for the personal save (Ctrl+S), both optional (see
// SPEC.md §3.2 and §9.1).
func (e *Editor) LoadFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	e.view.SetText(string(data), false)
	return true, nil
}

// SaveToFile writes the editor's full content to path, creating the parent
// directory if needed (Ctrl+S, see SPEC.md §3.2).
func (e *Editor) SaveToFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(e.Text()), 0o600)
}

// SelectRange positions the cursor/selection between the (rune) offsets
// start and end of the full text — used by search (Ctrl+F).
func (e *Editor) SelectRange(start, end int) {
	e.view.Select(start, end)
}

// FindNext looks for the next (case-insensitive) occurrence of query after
// the rune offset after, wrapping back to the start if needed. Returns
// (start, end, true) if found.
func (e *Editor) FindNext(query string, after int) (int, int, bool) {
	return findNext(e.Text(), query, after)
}

// completionLineRe detects a "METHOD partial_path" line being typed, up to
// the cursor (no end-of-line anchor: whatever follows the cursor, if
// anything, doesn't come into play).
var completionLineRe = regexp.MustCompile(`(?i)^(GET|POST|PUT|DELETE)[ \t]+(\S*)$`)

// CompletionPrefix returns, if the cursor is in the middle of typing an
// endpoint ("METHOD partial_path" line, see SPEC.md §3.2), the prefix to
// complete along with its bounds (in runes, full text — usable with
// ApplyCompletion). ok=false outside of this context (e.g. inside a JSON
// body), in which case Tab keeps its standard behavior (inserting a tab).
func (e *Editor) CompletionPrefix() (prefix string, start, end int, ok bool) {
	text := e.Text()
	offset := e.CursorOffset()
	lines := strings.Split(text, "\n")

	row, col := lineColAt(text, offset)
	if row < 0 || row >= len(lines) {
		return "", 0, 0, false
	}

	lineRunes := []rune(lines[row])
	if col < 0 {
		col = 0
	}
	if col > len(lineRunes) {
		col = len(lineRunes)
	}

	m := completionLineRe.FindStringSubmatch(string(lineRunes[:col]))
	if m == nil {
		return "", 0, 0, false
	}
	prefix = m[2]

	lineStart := offset - col // absolute offset of the logical line's start
	end = lineStart + col
	start = end - len([]rune(prefix))
	return prefix, start, end, true
}

// DebugCursorContext returns the current logical line's text up to the
// cursor, along with the row/column CompletionPrefix computes from it —
// exposed for F10's "no completion context" fallback (app.go) to
// self-diagnose why a context wasn't recognized, since Tab's silent
// passthrough in that case gives no such visibility (same idea as
// cmd/keydebug, applied to editor/cursor state instead of raw key events).
func (e *Editor) DebugCursorContext() (lineUpToCursor string, row, col int) {
	text := e.Text()
	offset := e.CursorOffset()
	lines := strings.Split(text, "\n")

	row, col = lineColAt(text, offset)
	if row < 0 || row >= len(lines) {
		return "", row, col
	}
	lineRunes := []rune(lines[row])
	if col < 0 {
		col = 0
	}
	if col > len(lineRunes) {
		col = len(lineRunes)
	}
	return string(lineRunes[:col]), row, col
}

// ApplyCompletion replaces the text between start and end (rune offsets,
// see CompletionPrefix) with replacement, and places the cursor right after.
func (e *Editor) ApplyCompletion(start, end int, replacement string) {
	e.view.Replace(start, end, replacement)
	newPos := start + len([]rune(replacement))
	e.view.Select(newPos, newPos)
}
