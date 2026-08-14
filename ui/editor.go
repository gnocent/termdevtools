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

// CursorOffset returns the cursor position as an absolute **byte** offset in
// the full text — confirmed by reading tview's own source (position
// tracking advances by len(cluster), a string's byte length, not a rune
// count) that this is what TextArea.GetSelection/Select/Replace all use
// internally, despite none of it being spelled out in their doc comments.
// Getting this wrong (an earlier version of this code used rune offsets
// throughout) silently selects/replaces the wrong range as soon as any
// multi-byte UTF-8 character — an accented French one, typically — precedes
// the position in question: not a crash, just a wrong-looking result, which
// is what made it easy to miss until a real report of search "landing on
// unrelated words" pointed at it.
//
// Unlike TextArea.GetCursor (whose "line" becomes a display line, not a
// logical line, as soon as automatic word wrap is active — see SPEC.md
// §3.1), GetSelection returns a position independent of visual rendering:
// "if there is no selection, start and end are the cursor position" (tview's
// official documentation). With an active selection, returns the end
// closest to the moving bound — no practical impact since Ctrl+E and
// Tab/F10 only use it when there's no selection.
func (e *Editor) CursorOffset() int {
	_, _, end := e.view.GetSelection()
	return end
}

// lineColAt returns the logical line (0-indexed, split on real "\n") and
// **byte** column corresponding to the absolute byte offset offset in text
// — see CursorOffset for why bytes, not runes. Purely textual, so
// independent of visual word wrap.
func lineColAt(text string, offset int) (row, col int) {
	lines := strings.Split(text, "\n")
	consumed := 0
	for i, line := range lines {
		lineLen := len(line)
		if offset <= consumed+lineLen {
			return i, offset - consumed
		}
		consumed += lineLen + 1 // +1 for the "\n" separator
	}
	last := len(lines) - 1
	return last, len(lines[last])
}

// CursorLine returns the (0-indexed) index of the logical line the cursor
// is on — the actual text's line, not the display line after automatic
// word wrap (SetWrap, see SPEC.md §3.1). Used to target which request
// Ctrl+E executes (parser.RequestAtLine) — the byte/rune mix-up described
// at CursorOffset could previously point this at the wrong line entirely
// (not just the wrong column) whenever earlier lines held non-ASCII text.
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

// SelectRange positions the cursor/selection between the (byte) offsets
// start and end of the full text — see CursorOffset — used by search
// (Ctrl+F) — and scrolls the match into view. Scrolling is needed because
// tview.TextArea.Select's own doc comment is explicit: "Scroll offsets will
// be preserved" — unlike normal cursor movement (typing, arrow keys), which
// tview keeps on-screen automatically, a Select() match outside the current
// viewport would otherwise select correctly but stay invisible, looking
// exactly like search "landing in the wrong place".
func (e *Editor) SelectRange(start, end int) {
	e.view.Select(start, end)
	e.scrollToCursor()
}

// scrollToCursor adjusts the row offset so the cursor's current (display)
// row is visible, replicating tview's own internal auto-scroll (used for
// normal cursor movement, not exposed publicly) since Select() explicitly
// opts out of it. GetCursor()'s rows are wrap-relative display rows — unlike
// CursorLine's logical line, that's exactly what a scroll offset (itself
// display-row-based) needs here.
func (e *Editor) scrollToCursor() {
	_, _, row, _ := e.view.GetCursor()
	_, _, _, height := e.view.GetInnerRect()
	if height <= 0 {
		return
	}
	rowOffset, colOffset := e.view.GetOffset()
	switch {
	case row < rowOffset:
		rowOffset = row
	case row >= rowOffset+height:
		rowOffset = row - height + 1
	default:
		return // already visible
	}
	e.view.SetOffset(rowOffset, colOffset)
}

// FindNext looks for the next (case-insensitive) occurrence of query after
// the byte offset after, wrapping back to the start if needed. Returns
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
// complete along with its bounds (byte offsets, full text — see
// CursorOffset — usable with ApplyCompletion). ok=false outside of this
// context (e.g. inside a JSON body), in which case Tab keeps its standard
// behavior (inserting a tab).
func (e *Editor) CompletionPrefix() (prefix string, start, end int, ok bool) {
	text := e.Text()
	offset := e.CursorOffset()
	lines := strings.Split(text, "\n")

	row, col := lineColAt(text, offset)
	if row < 0 || row >= len(lines) {
		return "", 0, 0, false
	}

	line := lines[row]
	if col < 0 {
		col = 0
	}
	if col > len(line) {
		col = len(line)
	}

	m := completionLineRe.FindStringSubmatch(line[:col])
	if m == nil {
		return "", 0, 0, false
	}
	prefix = m[2]

	lineStart := offset - col // absolute byte offset of the logical line's start
	end = lineStart + col
	start = end - len(prefix)
	return prefix, start, end, true
}

// ApplyCompletion replaces the text between start and end (byte offsets,
// see CompletionPrefix) with replacement, and places the cursor right after.
func (e *Editor) ApplyCompletion(start, end int, replacement string) {
	e.view.Replace(start, end, replacement)
	newPos := start + len(replacement)
	e.view.Select(newPos, newPos)
}
