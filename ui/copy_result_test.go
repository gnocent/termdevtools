package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestCopyResultSendsPlainTextToClipboard(t *testing.T) {
	app, screen := newTestApp(t)

	app.result.Show([]byte(`{"status":"green"}`))
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyF2, 0, tcell.ModNone)
	waitForDraw(t, screen)

	got := string(screen.GetClipboardData())
	want := app.result.PlainText()
	if got != want {
		t.Errorf("expected clipboard to contain %q, got %q", want, got)
	}
	if got == "" {
		t.Fatal("expected non-empty clipboard content")
	}
}

func TestCopyResultEmptyShowsError(t *testing.T) {
	app, screen := newTestApp(t)

	// Nothing has been displayed in the right panel yet.
	screen.InjectKey(tcell.KeyF2, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got := string(screen.GetClipboardData()); got != "" {
		t.Errorf("expected clipboard untouched when there is nothing to copy, got %q", got)
	}
	_ = app
}
