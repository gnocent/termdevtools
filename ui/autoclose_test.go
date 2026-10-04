package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"termdevtools/i18n"
)

// TestAutoCloseInsertsBracketPair checks the base case: typing an opener
// inserts its closer too, cursor placed in between — confirmed here by
// typing a further character and checking it lands between the pair, not
// after it.
func TestAutoCloseInsertsBracketPair(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("", true)

	injectRunes(e, "{")
	if got := e.Text(); got != "{}" {
		t.Fatalf("expected %q after typing '{', got %q", "{}", got)
	}
	injectRunes(e, "a")
	if got := e.Text(); got != "{a}" {
		t.Errorf("expected the cursor to sit between the pair (so typing 'a' gives %q), got %q", "{a}", got)
	}
}

// TestAutoCloseInsertsSquareBracketPair mirrors
// TestAutoCloseInsertsBracketPair for "[".
func TestAutoCloseInsertsSquareBracketPair(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("", true)

	injectRunes(e, "[")
	if got := e.Text(); got != "[]" {
		t.Errorf("expected %q, got %q", "[]", got)
	}
}

// TestAutoCloseInsertsQuotePairWhenOpeningAString checks that a quote typed
// outside any open string opens a new one (auto-closed), same as brackets.
func TestAutoCloseInsertsQuotePairWhenOpeningAString(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("", true)

	injectRunes(e, `"`)
	if got := e.Text(); got != `""` {
		t.Errorf("expected %q, got %q", `""`, got)
	}
}

// TestAutoCloseDoesNotReopenInsideAnAlreadyOpenString checks the one
// context-sensitive case: a quote typed while already inside an open string
// (an odd number of unescaped quotes before the cursor on this line) closes
// that string instead of opening a new nested pair — otherwise typing a
// normal closing quote by hand would leave a stray extra one behind.
func TestAutoCloseDoesNotReopenInsideAnAlreadyOpenString(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText(`"abc`, true) // cursor at the end, inside an open string

	injectRunes(e, `"`)
	if got := e.Text(); got != `"abc"` {
		t.Errorf("expected the string to just close (%q), got %q", `"abc"`, got)
	}
}

// TestAutoCloseSkipsOverExistingCloser checks that typing a closer
// immediately before one that's already there (typically auto-inserted a
// moment ago) steps over it instead of inserting a redundant second one.
func TestAutoCloseSkipsOverExistingCloser(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("", true)

	injectRunes(e, "{") // -> "{}", cursor between
	injectRunes(e, "}") // typed closer matches the one right there: skip over
	if got := e.Text(); got != "{}" {
		t.Fatalf("expected skip-over to leave %q untouched, got %q", "{}", got)
	}
	// Cursor should now be past the "}", not before it: further typing
	// appends instead of landing back inside the (now former) pair.
	injectRunes(e, "x")
	if got := e.Text(); got != "{}x" {
		t.Errorf("expected the cursor to have moved past the closer (%q), got %q", "{}x", got)
	}
}

// TestAutoCloseSkipsOverExistingQuoteCloser mirrors
// TestAutoCloseSkipsOverExistingCloser for a quote pair.
func TestAutoCloseSkipsOverExistingQuoteCloser(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("", true)

	injectRunes(e, `"`)  // -> `""`, cursor between
	injectRunes(e, `ab`) // -> `"ab"`, cursor before the closing quote
	injectRunes(e, `"`)  // typed closer matches: skip over, don't duplicate
	if got := e.Text(); got != `"ab"` {
		t.Errorf("expected %q, got %q", `"ab"`, got)
	}
}

// TestAutoCloseFullJSONObjectRoundTrips is the realistic stress case: typing
// out a complete, already-balanced JSON snippet character by character
// (brackets and quotes included, exactly as a user copy-typing an example
// would) must reproduce that exact text — every open triggers an
// auto-inserted closer, and the snippet's own explicit closers each skip
// over one instead of piling up extra ones.
func TestAutoCloseFullJSONObjectRoundTrips(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("", true)

	const body = `{"query":{"match_all":{}}}`
	injectRunes(e, body)

	if got := e.Text(); got != body {
		t.Errorf("expected the fully-typed JSON to round-trip exactly as %q, got %q", body, got)
	}
}

// TestAutoCloseBackspaceCollapsesEmptyPair checks that Backspace between an
// empty pair removes both sides in one go, not just the opener (which would
// otherwise leave a dangling, empty-looking closer behind).
func TestAutoCloseBackspaceCollapsesEmptyPair(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("", true)

	injectRunes(e, "{") // -> "{}", cursor between
	e.view.InputHandler()(backspaceEvent(), nil)
	if got := e.Text(); got != "" {
		t.Errorf("expected the empty pair to be fully removed, got %q", got)
	}
}

// TestAutoCloseBackspaceDoesNotCollapseNonEmptyPair checks that a
// non-empty pair is not collapsed — only a genuinely empty one is; a normal
// single-character Backspace applies otherwise.
func TestAutoCloseBackspaceDoesNotCollapseNonEmptyPair(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("{a}", true) // cursor at the end, after "}"

	e.view.InputHandler()(backspaceEvent(), nil)
	if got := e.Text(); got != "{a" {
		t.Errorf("expected a normal single-character Backspace (%q), got %q", "{a", got)
	}
}

// TestAutoCloseIgnoresEscapedQuotesForStringTracking checks that an escaped
// quote (\") doesn't flip the "inside a string" tracking used to decide
// whether a typed '"' opens a new string or just closes the current one.
func TestAutoCloseIgnoresEscapedQuotesForStringTracking(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	// One real open quote, then an escaped quote (doesn't close the
	// string), so the cursor is still inside the string at the end.
	e.view.SetText(`"abc\"def`, true)

	if !e.insideString() {
		t.Fatal("expected the cursor to still be considered inside the open string")
	}

	injectRunes(e, `"`)
	if got := e.Text(); got != `"abc\"def"` {
		t.Errorf("expected the real string to close (%q), got %q", `"abc\"def"`, got)
	}
}

// TestAutoCloseDoesNothingWithAnActiveSelection checks the safety guard: a
// pending selection is left entirely to TextArea's own default behavior
// (replace-selection-with-typed-character) — auto-close inserting a pair at
// the selection's edge without first accounting for the selected text would
// produce a wrong result instead.
func TestAutoCloseDoesNothingWithAnActiveSelection(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("abc", false)
	e.view.Select(0, 3) // select the whole "abc"

	injectRunes(e, "{")
	if got := e.Text(); got != "{" {
		t.Errorf("expected the selection to be replaced by a plain '{' (TextArea's own default), got %q", got)
	}
}

// injectRunes types s into e as individual KeyRune events, going through
// the same TextArea.SetInputCapture path (auto-close, ui/autoclose.go) real
// keystrokes do — a bare TextArea.SetText/Replace call would bypass it
// entirely.
func injectRunes(e *Editor, s string) {
	for _, r := range s {
		e.view.InputHandler()(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone), nil)
	}
}

func backspaceEvent() *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModNone)
}
