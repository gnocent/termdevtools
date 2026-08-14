package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestFindNextByteOffsets checks that findNext returns byte offsets, not
// rune offsets — confirmed by reading tview's own source that
// TextArea.Select/Replace/GetSelection all count UTF-8 bytes internally
// (position tracking advances by len(cluster), a string's byte length).
// Before this was byte-based, a match after any multi-byte character (an
// accented French one, typically) would compute a start/end short of the
// true position, selecting the wrong text once handed to
// Editor.SelectRange — a real report described this as search "landing on
// unrelated words".
func TestFindNextByteOffsets(t *testing.T) {
	text := "# résumé détaillé\nGET _cat/health"
	// "résumé détaillé" has four 2-byte accented characters (é×3, é again),
	// so its byte length exceeds its rune count — exactly the gap that a
	// rune-based offset would silently lose.
	start, end, found := findNext(text, "health", -1)
	if !found {
		t.Fatal("expected to find \"health\"")
	}
	wantStart := len("# résumé détaillé\nGET _cat/")
	wantEnd := wantStart + len("health")
	if start != wantStart || end != wantEnd {
		t.Errorf("expected byte range [%d,%d), got [%d,%d) — byte/rune offset mismatch if off by the accented bytes", wantStart, wantEnd, start, end)
	}
	if got := text[start:end]; got != "health" {
		t.Errorf("expected text[start:end] to be %q, got %q", "health", got)
	}
}

// TestSearchSelectsCorrectMatchAfterAccentedText is the end-to-end version
// of TestFindNextByteOffsets: a real Ctrl+F search, after a line containing
// accented French text, must select exactly the matched word — not
// something shifted by the accented bytes it doesn't account for.
func TestSearchSelectsCorrectMatchAfterAccentedText(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "# Ceci est un résumé détaillé de la procédure")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "GET _cat/health")
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyCtrlF, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	injectText(screen, "health")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)

	selected, _, _ := app.editor.view.GetSelection()
	if selected != "health" {
		t.Errorf("expected the search to select %q, got %q — landed on the wrong byte range after accented text", "health", selected)
	}
}
