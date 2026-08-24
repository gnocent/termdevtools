package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"termdevtools/i18n"
)

// TestWrapRowStartsNoWrap checks the unambiguous baseline: a width wide
// enough that nothing wraps, so rows only start at "\n" boundaries.
func TestWrapRowStartsNoWrap(t *testing.T) {
	got := wrapRowStarts("GET _cat/health\nPOST _search", 100)
	want := []int{0, 16}
	if !intsEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestWrapRowStartsCharacterFallback checks wrapping a run with no space
// (or other break opportunity) at all: tview.TextArea falls back to
// breaking at the grapheme boundary closest to the width, i.e. every
// "width" characters — this is the least ambiguous wrapping case, and the
// one the previous, naive rune/byte-count-based approach (rejected earlier
// this project for search/completion, see CursorOffset in editor.go) would
// actually have gotten right too; word-boundary wrapping (see
// TestWrapRowStartsWordBoundary) is the case that needed uniseg.
func TestWrapRowStartsCharacterFallback(t *testing.T) {
	got := wrapRowStarts("aaaaaaaaaa", 4)
	want := []int{0, 4, 8}
	if !intsEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestWrapRowStartsWordBoundary checks that wrapping breaks after a space
// (the last "line can break" opportunity) rather than mid-word, when one is
// available — the actual point of reusing TextArea's own wrapping logic.
func TestWrapRowStartsWordBoundary(t *testing.T) {
	got := wrapRowStarts("aaaa bbbb cccc dddd", 9)
	want := []int{0, 5, 10}
	if !intsEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestWrapRowStartsMixedWrapAndNewline checks a word-wrapped run followed
// by an explicit "\n" followed by more word-wrapping — the general case
// combining both break kinds in one pass.
func TestWrapRowStartsMixedWrapAndNewline(t *testing.T) {
	got := wrapRowStarts("aaaa bbbb cccc dddd\nGET _cat/health", 9)
	want := []int{0, 5, 10, 20, 29}
	if !intsEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestWrapRowStartsEmptyText checks the degenerate case: still one row (row
// 0), matching an empty TextArea still being "line 1".
func TestWrapRowStartsEmptyText(t *testing.T) {
	got := wrapRowStarts("", 40)
	want := []int{0}
	if !intsEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func intsEqual(a, b []int) bool {
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

// TestGutterTextNumbersOnlyLineStarts checks gutterText's own logic in
// isolation from wrapRowStarts (a hand-built rowStarts, so there's no
// question of whether the wrap points themselves are right — see the
// wrapRowStarts tests above for that): row 0 and row 2 start new logical
// lines (numbered 1 and 2), row 1 is a wrapped continuation of line 1
// (blank).
func TestGutterTextNumbersOnlyLineStarts(t *testing.T) {
	text := "aaaa bbbb \nsecond line"
	//        0    5    10(\n)11
	rowStarts := []int{0, 5, 11}

	got := gutterText(text, rowStarts, 0, 10, 1)
	want := "[gray]1\n \n2"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestGutterTextRespectsScrollOffset checks that only the visible window
// [rowOffset, rowOffset+height) is rendered — scrolled-past rows
// contribute to the logical line count but produce no output.
func TestGutterTextRespectsScrollOffset(t *testing.T) {
	text := "one\ntwo\nthree\nfour\nfive"
	rowStarts := []int{0, 4, 8, 14, 19} // one row per line, no wrapping

	got := gutterText(text, rowStarts, 2, 2, 1) // scrolled to show "three","four"
	want := "[gray]3\n4"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestGutterTextEmptyWindow checks a zero-height viewport produces no
// output rather than panicking.
func TestGutterTextEmptyWindow(t *testing.T) {
	if got := gutterText("one\ntwo", []int{0, 4}, 0, 0, 1); got != "[gray]" {
		t.Errorf("expected just the color tag (no visible rows) for a zero-height window, got %q", got)
	}
}

// TestRefreshGutterRendersLineNumbers is the end-to-end check: a real
// Editor, with a real (if not application-driven) draw pass at a known
// size, actually populates the gutter TextView with the expected numbers —
// confirms refreshGutter's wiring (container rect, ResizeItem, scroll
// offset) on top of the two pieces already tested in isolation above.
func TestRefreshGutterRendersLineNumbers(t *testing.T) {
	e := NewEditor(i18n.For(""))
	e.view.SetText("GET _cat/health\nPOST _search\nGET _cat/indices", false)
	e.container.SetRect(0, 0, 40, 24) // wide enough that nothing wraps

	e.refreshGutter()

	want := "1\n2\n3"
	if got := e.gutter.GetText(true); got != want {
		t.Errorf("expected gutter text %q, got %q", want, got)
	}
}

// TestGutterIsColoredAndPaddedAfterTheNumber is a regression test for the
// exact bug reported: an earlier version paired a right-aligned gutter box
// with trailing padding, which put the blank margin on the wrong side (next
// to the border, not between the numbers and the request text) — and used
// no color, so the numbers were indistinguishable from the text right next
// to them.
//
// Checks color directly on the raw (untrimmed) content: the gray tag must
// be present. Checks the padding indirectly rather than via GetText — a
// box's unfilled columns are a rendering-time effect of alignment, not
// literal characters in the stored text, so GetText wouldn't see them
// either way regardless of which side they end up on. What actually
// distinguishes "blank margin after the number" from "blank margin before
// it" is alignment plus box width: left-aligned (checked by the number
// starting the text with no leading padding) inside a box gutterPadding
// columns wider than the number itself (checked directly, same as
// TestRefreshGutterWidthGrowsWithLineCount) leaves that margin trailing,
// next to the request text — not leading, next to the border.
func TestGutterIsColoredAndPaddedAfterTheNumber(t *testing.T) {
	e := NewEditor(i18n.For(""))
	e.view.SetText("GET _cat/health", false)
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(40, 24)
	e.container.SetRect(0, 0, 40, 24)
	e.container.Draw(screen)

	raw := e.gutter.GetText(false)
	if !strings.HasPrefix(raw, "[gray]") {
		t.Errorf("expected the gutter content to start with the gray color tag, got %q", raw)
	}
	if want := "[gray]1"; raw != want {
		t.Errorf("expected the stored text to be just %q (no leading padding baked in), got %q", want, raw)
	}

	_, _, gotWidth, _ := e.gutter.GetRect()
	if wantWidth := 1 + gutterPadding; gotWidth != wantWidth {
		t.Errorf("expected the gutter box to be %d columns wide (1-digit number + gutterPadding), got %d", wantWidth, gotWidth)
	}
}

// TestRefreshGutterBlanksWrappedContinuation checks the visual point of
// this whole feature: a single logical line that wraps across several
// display rows gets its number once, not once per display row.
func TestRefreshGutterBlanksWrappedContinuation(t *testing.T) {
	e := NewEditor(i18n.For(""))
	e.view.SetText("aaaa bbbb cccc dddd\nGET _cat/health", false)
	// GetInnerRect subtracts the container's own border (SetBorder(true) in
	// NewEditor): -2 columns, -2 rows. Inner width will be
	// totalWidth-2-gutterWidth; gutterWidth is 1 digit + gutterPadding(2)
	// = 3 for a 2-line document, so pick a total width that leaves exactly
	// 9 columns for text (matches TestWrapRowStartsMixedWrapAndNewline's
	// width=9 case: 5 display rows, [0,5,10,20,29]). Inner height 4 (rect
	// height 6) keeps the check to the first four rows — the wrapped
	// "aaaa.../GET _cat/" line — the point being made here; a fifth,
	// also-blank continuation row ("health") would just repeat it.
	e.container.SetRect(0, 0, 9+3+2, 4+2)

	e.refreshGutter()

	// GetText(true) strips the "[gray]" color tag but keeps the literal
	// space content of a blank (continuation) row.
	want := "1\n \n \n2"
	if got := e.gutter.GetText(true); got != want {
		t.Errorf("expected gutter text %q, got %q", want, got)
	}
}

// TestRefreshGutterWidthGrowsWithLineCount checks that the gutter widens as
// the document grows past a power of ten (1 digit -> 2 digits), the same
// way most editors' gutters do. Unlike the other tests here, this one needs
// an actual Draw pass (not just a direct refreshGutter call): resizing only
// updates the container Flex's target size for its next layout — the
// gutter's own rect (what GetRect reports) is applied by Flex.Draw itself.
func TestRefreshGutterWidthGrowsWithLineCount(t *testing.T) {
	e := NewEditor(i18n.For(""))
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(40, 24)
	e.container.SetRect(0, 0, 40, 24)

	nineLines := "1\n2\n3\n4\n5\n6\n7\n8\n9"
	e.view.SetText(nineLines, false)
	e.container.Draw(screen)
	_, _, gotWidth, _ := e.gutter.GetRect()
	if gotWidth != 3 { // 1 digit + gutterPadding(2)
		t.Errorf("expected gutter width 3 for a 9-line document, got %d", gotWidth)
	}

	tenLines := nineLines + "\n10"
	e.view.SetText(tenLines, false)
	e.container.Draw(screen)
	_, _, gotWidth, _ = e.gutter.GetRect()
	if gotWidth != 4 { // 2 digits + gutterPadding(2)
		t.Errorf("expected gutter width 4 for a 10-line document, got %d", gotWidth)
	}
}
