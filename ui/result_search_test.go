package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"termdevtools/i18n"
)

// TestFindNextAfterAShorterResult reproduces a crash: the position of the
// last match is kept from one search to the next, and may lie beyond the end
// of a shorter result displayed since. Searching that result for something
// it doesn't contain then indexed past its last line.
func TestFindNextAfterAShorterResult(t *testing.T) {
	r := NewResultView(i18n.For(i18n.FR))
	r.Show("GET", "_cat/health?v", nil, []byte("one\ntwo\nthree"))

	if line, found := r.FindNext("absent", 300); found {
		t.Errorf("expected no match, got line %d", line)
	}
	// A stale position must not hide a match either: the search wraps around.
	if line, found := r.FindNext("two", 300); !found || line != 2 {
		t.Errorf("expected the match on line 2 (after the reminder line), got %d, %v", line, found)
	}
}

// TestResultSearchSurvivesANewResult is the same scenario end to end: find a
// match far down a long result, display a short one, search it for a term it
// doesn't have.
func TestResultSearchSurvivesANewResult(t *testing.T) {
	app, screen := newTestApp(t)

	search := func(query string) {
		t.Helper()
		screen.InjectKey(tcell.KeyCtrlF, 0, tcell.ModNone)
		eventually(t, "the search bar to open", func() bool {
			var open bool
			app.tapp.QueueUpdate(func() { open = app.searchBar.HasFocus() })
			return open
		})
		injectText(screen, query)
		eventually(t, "the query to be typed", func() bool {
			var typed string
			app.tapp.QueueUpdate(func() { typed = app.searchBar.GetText() })
			return typed == query
		})
		screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	}

	app.tapp.QueueUpdate(func() {
		app.result.Show("GET", "_long", nil, []byte(strings.Repeat("filler\n", 300)+"needle\n"))
		app.focusResultPanel()
	})
	search("needle")
	eventually(t, "the match far down the long result", func() bool {
		var line int
		app.tapp.QueueUpdate(func() { line = app.resultSearchLine })
		return line == 301
	})
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)

	app.tapp.QueueUpdate(func() {
		app.result.Show("GET", "_short", nil, []byte("only\nthree\nlines"))
		app.focusResultPanel()
	})
	search("absent")
	eventually(t, "the search to report no match", func() bool {
		return strings.Contains(statusText(app), app.msgs.ErrNoMatchFound)
	})
}
