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

// TestAltWordNavigationSwitchesFocus checks the Meta-b/Meta-f rune encoding
// (ESC b / ESC f) confirmed, via cmd/keydebug on a real macOS terminal, to
// be what that terminal actually sends for Option/Alt+Left and
// Option/Alt+Right — not a modified KeyLeft/KeyRight at all, but a plain
// KeyRune event with Rune 'b'/'f' and only ModAlt set (see
// isAltWordBack/isAltWordForward in app.go). Without this, the
// hasShortcutModifier-based fallback tested by TestOptionAltFocusSwitch
// never fires on that terminal, because its events don't even look like
// arrow keys to begin with.
func TestAltWordNavigationSwitchesFocus(t *testing.T) {
	app, screen := newTestApp(t)

	screen.InjectKey(tcell.KeyRune, 'f', tcell.ModAlt)
	waitForDraw(t, screen)
	if app.focusedIsEditor {
		t.Fatal("expected Option/Alt+Right (reported as rune 'f'+Alt) to switch focus to the result panel")
	}

	screen.InjectKey(tcell.KeyRune, 'b', tcell.ModAlt)
	waitForDraw(t, screen)
	if !app.focusedIsEditor {
		t.Fatal("expected Option/Alt+Left (reported as rune 'b'+Alt) to switch focus back to the editor")
	}
}

// TestF5F6ResizeSplit checks that F5/F6 — the primary, guaranteed-reliable
// resize shortcuts — shrink/grow the left panel. Added after confirming,
// via cmd/keydebug on a real macOS terminal, that Shift+Alt+←/→ arrives
// there as a plain KeyLeft/KeyRight with zero modifiers, indistinguishable
// from an unmodified arrow key press (see isShrinkShortcut/isGrowShortcut).
func TestF5F6ResizeSplit(t *testing.T) {
	app, screen := newTestApp(t)
	initial := app.leftWeight

	screen.InjectKey(tcell.KeyF6, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if app.leftWeight != initial+1 {
		t.Fatalf("expected F6 to grow the left panel by 1, got leftWeight=%d (was %d)", app.leftWeight, initial)
	}

	screen.InjectKey(tcell.KeyF5, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if app.leftWeight != initial {
		t.Fatalf("expected F5 to shrink the left panel back by 1, got leftWeight=%d (want %d)", app.leftWeight, initial)
	}
}

// TestOptionAltResizeSplit checks that Ctrl/Option/Alt+Shift+←/→ still
// resizes the split as a best-effort fallback, on terminals that do report
// Shift+Alt+arrow with modifiers — F5/F6 (TestF5F6ResizeSplit) are the
// recommended, guaranteed-reliable shortcuts; this path isn't (see
// isShrinkShortcut/isGrowShortcut).
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

// TestCtrlEExecutes checks that Ctrl+E — the primary, always-reliable
// execute shortcut (a raw control byte, like Ctrl+F/Ctrl+S) — triggers
// request execution. Added after confirming, via cmd/keydebug on a real
// macOS terminal, that Ctrl+Enter/Option+Enter/Alt+Enter are all reported
// identically to plain Enter (Ctrl+M *is* Enter's control byte, and that
// terminal attaches no modifier information to it at all).
func TestCtrlEExecutes(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/health")
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	waitForDraw(t, screen)

	text := screenText(screen)
	if strings.Contains(text, app.msgs.StatusIdle) {
		t.Errorf("expected Ctrl+E to trigger execution (status bar no longer idle), got:\n%s", text)
	}
}

// TestOptionAltEnterExecutes checks that Option/Alt+Enter still triggers
// request execution as a best-effort fallback, on terminals that do report
// Enter with a modifier — Ctrl+E (TestCtrlEExecutes) is the recommended,
// guaranteed-reliable shortcut; this path isn't (see isExecuteShortcut).
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
