package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestOptionAltFocusSwitch checks that Option/Alt+←/→ switches panel focus
// just like Ctrl+←/→ — added for macOS, where Ctrl+←/→ is intercepted at
// the OS level by default (Mission Control desktop switching, see
// hasShortcutModifier in app.go).
func TestOptionAltFocusSwitch(t *testing.T) {
	app, screen := newTestApp(t)

	if !app.focusedIsEditor {
		t.Fatal("expected the editor to be focused initially")
	}

	screen.InjectKey(tcell.KeyRight, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	if app.focusedIsEditor {
		t.Fatal("expected Ctrl+Right to switch focus to the result panel")
	}

	screen.InjectKey(tcell.KeyLeft, 0, tcell.ModAlt)
	waitForDraw(t, screen)
	if !app.focusedIsEditor {
		t.Fatal("expected Option/Alt+Left to switch focus back to the editor, like Ctrl+Left")
	}

	screen.InjectKey(tcell.KeyRight, 0, tcell.ModAlt)
	waitForDraw(t, screen)
	if app.focusedIsEditor {
		t.Fatal("expected Option/Alt+Right to switch focus to the result panel, like Ctrl+Right")
	}
}

// TestOptionAltResizeSplit checks that Option/Alt+Shift+←/→ resizes the
// split just like Ctrl+Shift+←/→ — same macOS rationale as
// TestOptionAltFocusSwitch.
func TestOptionAltResizeSplit(t *testing.T) {
	app, screen := newTestApp(t)
	initial := app.leftWeight

	screen.InjectKey(tcell.KeyRight, 0, tcell.ModCtrl|tcell.ModShift)
	waitForDraw(t, screen)
	if app.leftWeight != initial+1 {
		t.Fatalf("expected Ctrl+Shift+Right to grow the left panel by 1, got leftWeight=%d (was %d)", app.leftWeight, initial)
	}

	screen.InjectKey(tcell.KeyLeft, 0, tcell.ModAlt|tcell.ModShift)
	waitForDraw(t, screen)
	if app.leftWeight != initial {
		t.Fatalf("expected Option/Alt+Shift+Left to shrink the left panel back by 1, got leftWeight=%d (want %d)", app.leftWeight, initial)
	}

	screen.InjectKey(tcell.KeyLeft, 0, tcell.ModAlt|tcell.ModShift)
	waitForDraw(t, screen)
	if app.leftWeight != initial-1 {
		t.Fatalf("expected a second Option/Alt+Shift+Left to shrink further, got leftWeight=%d (want %d)", app.leftWeight, initial-1)
	}
}

// TestOptionAltEnterExecutes checks that Option/Alt+Enter triggers request
// execution just like Ctrl+Enter — same macOS rationale, plus Ctrl+Enter is
// inherently ambiguous with plain Enter in classic terminal encoding
// (Ctrl+M *is* Enter's control byte), which Option+Enter isn't.
func TestOptionAltEnterExecutes(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/health")
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModAlt)
	waitForDraw(t, screen)

	// executeCurrent() calls status.SetRunning() synchronously before the
	// (here unreachable, 127.0.0.1:1) HTTP call runs in a goroutine — so the
	// status bar switching away from "ready"/"prêt" is proof the shortcut
	// fired, independent of how the network call itself resolves.
	text := screenText(screen)
	if strings.Contains(text, app.msgs.StatusIdle) {
		t.Errorf("expected Option/Alt+Enter to trigger execution (status bar no longer idle), got:\n%s", text)
	}
}
