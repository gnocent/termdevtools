package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestCopyResultSendsPlainTextToClipboard(t *testing.T) {
	app, screen := newTestApp(t)

	onUI(app, func() { app.result.Show("GET", "_cluster/health", nil, []byte(`{"status":"green"}`)) })
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyF2, 0, tcell.ModNone)
	waitForDraw(t, screen)

	got := string(screen.GetClipboardData())
	want := uiValue(app, app.result.PlainText)
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
	// An untouched clipboard is also what a silent F2 would leave: the error
	// must have been reported.
	if status := statusText(app); !strings.Contains(status, app.msgs.ErrNothingToCopy) {
		t.Errorf("expected the status bar to say there is nothing to copy, got %q", status)
	}
}
