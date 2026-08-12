package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// screenText concatène tout le contenu texte actuellement affiché par
// l'écran simulé, sans mise en forme — utile pour vérifier qu'un contenu
// donné est bien rendu à l'écran.
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

	// Sur un terminal bas (ici 24 lignes), la section "Fichiers" ne tient
	// pas au-dessus de la ligne de flottaison : elle doit rester
	// accessible en scrollant (TextView.SetWrap laisse le défilement
	// standard actif, cf. handleGlobalKeys qui laisse passer les touches
	// autres qu'Echap pendant que l'aide est affichée).
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

	// Ctrl+S ne doit pas déclencher une sauvegarde/export pendant que
	// l'aide est affichée : elle capte Entrée/Echap pour elle-même, tout
	// le reste doit rester sans effet sur le reste de l'appli.
	screen.InjectKey(tcell.KeyCtrlS, 0, tcell.ModCtrl)
	waitForDraw(t, screen)

	if !app.helpVisible {
		t.Error("expected help to remain open, unaffected by other shortcuts")
	}
}
