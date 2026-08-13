package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// screenText concatenates all the text content currently displayed by the
// simulated screen, with no formatting — useful to check that given content
// is actually rendered on screen.
func screenText(screen tcell.SimulationScreen) string {
	cells, w, h := screen.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			b.WriteString(string(cells[y*w+x].Runes))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestHelpOpensAndClosesWithEscape(t *testing.T) {
	app, screen := newTestApp(t)

	screen.InjectKey(tcell.KeyF1, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if !app.helpVisible {
		t.Fatal("expected helpVisible=true after F1")
	}
	if app.tapp.GetFocus() != app.helpView {
		t.Error("expected focus to be on the help view while it's open")
	}
	if text := screenText(screen); !strings.Contains(text, "Raccourcis clavier") {
		t.Errorf("expected shortcuts section to be rendered on screen, got:\n%s", text)
	}

	// On a short terminal (24 lines here), the "Fichiers" section doesn't
	// fit above the fold: it must remain reachable by scrolling
	// (TextView.SetWrap keeps standard scrolling active, see
	// handleGlobalKeys which lets through keys other than Escape while
	// help is displayed).
	screen.InjectKey(tcell.KeyEnd, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if text := screenText(screen); !strings.Contains(text, "Fichiers") || !strings.Contains(text, "config.yaml") {
		t.Errorf("expected the files section to be reachable by scrolling to the end, got:\n%s", text)
	}

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if app.helpVisible {
		t.Error("expected helpVisible=false after Escape")
	}
	if app.tapp.GetFocus() != app.editor.Primitive() {
		t.Error("expected focus back on the editor after closing help")
	}
}

func TestHelpDoesNotModifyEditorContent(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/health?v")
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyF1, 0, tcell.ModNone)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/health?v"; got != want {
		t.Errorf("expected editor text unchanged by the help popup, got %q want %q", got, want)
	}
}

func TestHelpIgnoresOtherShortcutsWhileOpen(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/health?v")
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyF1, 0, tcell.ModNone)
	waitForDraw(t, screen)

	// Ctrl+S must not trigger a save/export while help is displayed: it
	// captures Enter/Escape for itself, everything else must remain
	// without effect on the rest of the app.
	screen.InjectKey(tcell.KeyCtrlS, 0, tcell.ModCtrl)
	waitForDraw(t, screen)

	if !app.helpVisible {
		t.Error("expected help to remain open, unaffected by other shortcuts")
	}
}

// TestInterfaceLanguageEnglish checks that config.Config.Language actually
// switches the rendered interface, end-to-end — not just that the i18n
// catalog compiles. Covers both a status-bar string (StatusBar.SetIdle) and
// the F1 help screen.
func TestInterfaceLanguageEnglish(t *testing.T) {
	_, screen := newTestAppLang(t, "en")

	text := screenText(screen)
	if !strings.Contains(text, "ready") {
		t.Errorf("expected the English status bar ('ready'), got:\n%s", text)
	}
	if strings.Contains(text, "prêt") {
		t.Errorf("did not expect French status text with Language=en, got:\n%s", text)
	}

	screen.InjectKey(tcell.KeyF1, 0, tcell.ModNone)
	waitForDraw(t, screen)

	text = screenText(screen)
	if !strings.Contains(text, "Keyboard shortcuts") {
		t.Errorf("expected the English help screen, got:\n%s", text)
	}
	if strings.Contains(text, "Raccourcis clavier") {
		t.Errorf("did not expect French help text with Language=en, got:\n%s", text)
	}
}
