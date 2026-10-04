package ui

import (
	"bytes"
	"encoding/json"
	"testing"

	"termdevtools/i18n"
)

func TestCompletionPrefixAtEndOfLine(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("GET _cat/s", true) // cursor at end of text

	prefix, start, end, ok := e.CompletionPrefix()
	if !ok {
		t.Fatal("expected ok=true for a method+partial-path line")
	}
	if prefix != "_cat/s" {
		t.Errorf("expected prefix %q, got %q", "_cat/s", prefix)
	}
	if start != 4 || end != 10 {
		t.Errorf("expected range [4,10), got [%d,%d)", start, end)
	}
}

func TestCompletionPrefixEmpty(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("GET ", true)

	prefix, _, _, ok := e.CompletionPrefix()
	if !ok {
		t.Fatal("expected ok=true right after the method and a space")
	}
	if prefix != "" {
		t.Errorf("expected empty prefix, got %q", prefix)
	}
}

func TestCompletionPrefixNotAMethodLine(t *testing.T) {
	cases := []string{
		"# un commentaire",
		`{"query": {}}`,
		"  ",
		"GETX _cat/s", // not a recognized method
	}
	for _, text := range cases {
		e := NewEditor(i18n.For(i18n.FR))
		e.view.SetText(text, true)
		if _, _, _, ok := e.CompletionPrefix(); ok {
			t.Errorf("expected ok=false for %q", text)
		}
	}
}

func TestCompletionPrefixSecondLine(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("# cheatsheet\nGET _cat/sh", true)

	prefix, start, end, ok := e.CompletionPrefix()
	if !ok {
		t.Fatal("expected ok=true on the second line")
	}
	if prefix != "_cat/sh" {
		t.Errorf("expected prefix %q, got %q", "_cat/sh", prefix)
	}
	// "# cheatsheet\n" is 13 bytes (all-ASCII here, so also 13 runes); "GET "
	// adds 4 more. See TestLineColAtByteOffsets for a case where the two
	// diverge.
	if start != 13+4 {
		t.Errorf("expected start %d, got %d", 13+4, start)
	}
	if end-start != len(prefix) {
		t.Errorf("expected end-start == len(prefix), got end=%d start=%d", end, start)
	}
}

// TestLineColAtByteOffsets checks that lineColAt (and everything built on
// it: CursorLine, CompletionPrefix) reasons in UTF-8 **byte** offsets, not
// runes — confirmed by reading tview's own source that
// TextArea.GetSelection/Select/Replace all count bytes internally. Before
// this was byte-based, a position on a line *after* one containing any
// multi-byte character (an accented French one, typically) would come out
// short by however many extra bytes that character contributed — wrong
// column, and for CursorLine specifically, potentially the wrong *line*
// entirely (which request Ctrl+E executes).
func TestLineColAtByteOffsets(t *testing.T) {
	text := "# résumé\nGET _cat/health"
	// "résumé" has two 2-byte accented characters, so "# résumé\n" is 11
	// bytes, not the 9 a rune count would suggest.
	offset := len("# résumé\nGET _cat/") // right before "health"
	row, col := lineColAt(text, offset)
	if row != 1 {
		t.Fatalf("expected row 1, got %d", row)
	}
	if want := len("GET _cat/"); col != want {
		t.Errorf("expected col %d, got %d — byte/rune offset mismatch", want, col)
	}
}

func TestApplyCompletion(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("GET _cat/s", true)

	_, start, end, ok := e.CompletionPrefix()
	if !ok {
		t.Fatal("expected ok=true")
	}
	e.ApplyCompletion(start, end, "_cat/shards")

	if got := e.Text(); got != "GET _cat/shards" {
		t.Errorf("expected %q, got %q", "GET _cat/shards", got)
	}
}

// TestReformatBodyIndentsCompactJSON checks F4's core behavior (SPEC.md §7
// backlog #1, Kibana's "auto indent"): a compact JSON body gets re-indented
// in place, the method line above it left untouched.
func TestReformatBodyIndentsCompactJSON(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	const compact = `{"query":{"match_all":{}}}`
	e.view.SetText("POST _search\n"+compact, true)

	if ok := e.ReformatBody(1, 1); !ok {
		t.Fatal("expected ReformatBody to report a change")
	}

	var want bytes.Buffer
	if err := json.Indent(&want, []byte(compact), "", "  "); err != nil {
		t.Fatalf("json.Indent: %v", err)
	}
	if got, wantText := e.Text(), "POST _search\n"+want.String(); got != wantText {
		t.Errorf("expected:\n%s\ngot:\n%s", wantText, got)
	}
}

// TestReformatBodyNoOpOnInvalidJSON checks that a body that isn't valid
// JSON (e.g. still being typed) is left untouched rather than mangled.
func TestReformatBodyNoOpOnInvalidJSON(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	const text = "POST _search\n{not json"
	e.view.SetText(text, true)

	if ok := e.ReformatBody(1, 1); ok {
		t.Error("expected no-op for invalid JSON")
	}
	if got := e.Text(); got != text {
		t.Errorf("expected the text to stay unchanged, got %q", got)
	}
}

// TestReformatBodyMultiLineNoOpWhenAlreadyIndented checks that
// lineRangeOffsets correctly reconstructs a body spanning several lines
// (not just a single-line compact one) — reformatting an already-indented
// body is a no-op, and content after it (a second request) stays untouched.
func TestReformatBodyMultiLineNoOpWhenAlreadyIndented(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	const text = "POST _search\n{\n  \"query\": {}\n}\nGET _cat/health"
	e.view.SetText(text, true)

	if ok := e.ReformatBody(1, 3); ok {
		t.Error("expected no-op: the body is already indented")
	}
	if got := e.Text(); got != text {
		t.Errorf("expected the text to stay unchanged, got %q", got)
	}
}
