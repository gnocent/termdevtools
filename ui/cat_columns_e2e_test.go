package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestCatColumnCompletionEndToEnd(t *testing.T) {
	app, screen := newTestApp(t)

	// h= completion: unique prefix among _cat/indices' real columns.
	injectText(screen, "GET _cat/indices?h=heal")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/indices?h=health"; got != want {
		t.Fatalf("expected %q after h= completion, got %q", want, got)
	}

	// A second column, comma-separated: only the part after the last comma
	// should be completed, the rest preserved. Short aliases (e.g.
	// "storeSize") are no longer in the default list, so "sto" now only
	// matches "store.size": direct completion.
	injectText(screen, ",sto")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/indices?h=health,store.size"; got != want {
		t.Fatalf("expected %q after second h= completion, got %q", want, got)
	}
}

func TestCatColumnSortDirectionCompletionEndToEnd(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/shards?s=index:de")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/shards?s=index:desc"; got != want {
		t.Fatalf("expected %q after sort-direction completion, got %q", want, got)
	}
}

// TestCatColumnCompletionWithFilterSegmentEndToEnd covers the reported
// case: a filter (index name, node name...) between the _cat command and
// the parameters, e.g. "_cat/shards/myindex?h=...". The _cat command must
// still be recognized despite this extra segment.
func TestCatColumnCompletionWithFilterSegmentEndToEnd(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/shards/monindex?h=stat")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/shards/monindex?h=state"; got != want {
		t.Fatalf("expected %q after h= completion despite the filter segment, got %q", want, got)
	}
}
