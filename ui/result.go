package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/rivo/tview"

	"termdevtools/i18n"
)

// searchRegionID is the single tview region used to highlight/scroll to the
// current search match (Ctrl+F, see HighlightLine) — only one result
// highlighted at a time, so no need for distinct IDs.
const searchRegionID = "match"

// ResultView is the right panel: the result of the last request.
// Pretty-printed and colorized JSON if the response is valid JSON, plain
// text otherwise (e.g. `_cat` responses). See SPEC.md §3.3.
type ResultView struct {
	view          *tview.TextView
	plain         string // displayed text without color tags, for search and export
	displayedText string // last text actually sent to SetText (with color tags), for HighlightLine
	isJSON        bool
	msgs          *i18n.Strings
}

// NewResultView creates an empty result panel.
func NewResultView(msgs *i18n.Strings) *ResultView {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(true).
		SetRegions(true)
	view.SetBorder(true).SetTitle(msgs.ResultTitle)
	return &ResultView{view: view, msgs: msgs}
}

// Widget returns the tview component to insert into the layout.
func (r *ResultView) Widget() tview.Primitive {
	return r.view
}

// Primitive returns the underlying TextView.
func (r *ResultView) Primitive() tview.Primitive {
	return r.view
}

// SetLanguage re-applies the panel's chrome (border title) in msgs'
// language, and switches which language ErrNothingToExport is reported in —
// the displayed result itself is untouched.
func (r *ResultView) SetLanguage(msgs *i18n.Strings) {
	r.msgs = msgs
	r.view.SetTitle(msgs.ResultTitle)
}

// PlainText returns the currently displayed content, without color tags —
// used for export (Ctrl+S) and clipboard copy (F2).
func (r *ResultView) PlainText() string {
	return r.plain
}

// Clear empties the panel (initial state before any execution).
func (r *ResultView) Clear() {
	r.plain = ""
	r.displayedText = ""
	r.isJSON = false
	r.view.Clear()
	r.view.ScrollToBeginning()
}

// Show displays the response body body: pretty-printed and colorized JSON
// if valid, plain text (fixed-width) otherwise.
func (r *ResultView) Show(body []byte) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err == nil {
		r.plain = buf.String()
		r.isJSON = true
		r.displayedText = colorizeJSON(r.plain)
	} else {
		r.plain = string(body)
		r.isJSON = false
		r.displayedText = tview.Escape(r.plain)
	}
	r.view.SetText(r.displayedText)
	r.view.ScrollToBeginning()
}

// ShowError displays an error message in red.
func (r *ResultView) ShowError(message string) {
	r.plain = message
	r.isJSON = false
	r.displayedText = "[red]" + tview.Escape(message) + "[white]"
	r.view.SetText(r.displayedText)
	r.view.ScrollToBeginning()
}

// Export writes the currently displayed result to a timestamped file in
// directory dir (created if needed), as .json if it's valid JSON, .txt
// otherwise. Returns the path of the created file. See SPEC.md §3.3 and §9.1.
func (r *ResultView) Export(dir string) (string, error) {
	if r.plain == "" {
		return "", errors.New(r.msgs.ErrNothingToExport)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	ext := ".txt"
	if r.isJSON {
		ext = ".json"
	}
	path := filepath.Join(dir, time.Now().Format("20060102-150405")+ext)

	if err := os.WriteFile(path, []byte(r.plain), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// HighlightLine scrolls the panel to the given (0-indexed) line, used by
// search (Ctrl+F). Goes through tview's region mechanism (anchored to
// content) rather than TextView.ScrollTo(line, ...), whose line number
// becomes a display line — not a logical one — as soon as automatic word
// wrap is active (SPEC.md §3.1): a plain ScrollTo would target the wrong
// spot as soon as a preceding line had wrapped across several display lines.
func (r *ResultView) HighlightLine(line int) {
	lines := strings.Split(r.displayedText, "\n")
	if line < 0 || line >= len(lines) {
		return
	}

	// JSON tokens never contain "\n" (colorizeJSON only reformats tokens,
	// never inserts/removes a line break), so displayedText has exactly the
	// same line split as plain: wrapping the targeted line in a region is safe.
	tagged := make([]string, len(lines))
	copy(tagged, lines)
	tagged[line] = `["` + searchRegionID + `"]` + tagged[line] + `[""]`

	r.view.SetText(strings.Join(tagged, "\n"))
	r.view.Highlight(searchRegionID)
	r.view.ScrollToHighlight()
}

// FindNext looks for the next (case-insensitive) occurrence of query in the
// displayed plain text, starting from line afterLine (exclusive). Returns
// the line number of the next match, wrapping back to the start.
func (r *ResultView) FindNext(query string, afterLine int) (int, bool) {
	if query == "" || r.plain == "" {
		return 0, false
	}
	lines := strings.Split(r.plain, "\n")
	lowerQuery := strings.ToLower(query)

	search := func(from, to int) (int, bool) {
		for i := from; i < to; i++ {
			if strings.Contains(strings.ToLower(lines[i]), lowerQuery) {
				return i, true
			}
		}
		return 0, false
	}

	if line, ok := search(afterLine+1, len(lines)); ok {
		return line, true
	}
	return search(0, afterLine+1)
}

var jsonTokenRe = regexp.MustCompile(`"(?:\\.|[^"\\])*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null|[{}\[\],:]`)

// colorizeJSON colorizes an already-indented (json.Indent) JSON text with
// tview color tags. Any manual tag construction is only safe if literal "["
// characters in the content are escaped (tview convention).
func colorizeJSON(text string) string {
	var b strings.Builder
	last := 0
	for _, loc := range jsonTokenRe.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		b.WriteString(tview.Escape(text[last:start]))
		tok := text[start:end]

		switch {
		case strings.HasPrefix(tok, `"`):
			if isKeyToken(text, end) {
				b.WriteString("[#5fafff]")
			} else {
				b.WriteString("[#87d787]")
			}
			b.WriteString(tview.Escape(tok))
			b.WriteString("[white]")
		case len(tok) == 1 && strings.ContainsRune("{}[],:", rune(tok[0])):
			b.WriteString(tview.Escape(tok))
		case tok == "true" || tok == "false":
			b.WriteString("[#d78700]")
			b.WriteString(tok)
			b.WriteString("[white]")
		case tok == "null":
			b.WriteString("[gray]")
			b.WriteString(tok)
			b.WriteString("[white]")
		default: // number
			b.WriteString("[#af87ff]")
			b.WriteString(tok)
			b.WriteString("[white]")
		}
		last = end
	}
	b.WriteString(tview.Escape(text[last:]))
	return b.String()
}

// isKeyToken reports whether the string ending at pos is followed (after
// any whitespace) by a ':', in which case it's a JSON key.
func isKeyToken(text string, pos int) bool {
	for pos < len(text) {
		r := rune(text[pos])
		if unicode.IsSpace(r) {
			pos++
			continue
		}
		return text[pos] == ':'
	}
	return false
}
