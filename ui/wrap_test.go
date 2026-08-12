package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestCursorLineWrapSafe vérifie que CursorLine reste correct (ligne
// logique du texte réel) même quand une ligne précédente déborde sur
// plusieurs lignes affichées — le bug qu'aurait provoqué un simple
// TextArea.GetCursor() une fois le retour à la ligne automatique actif
// (SPEC.md §3.1).
func TestCursorLineWrapSafe(t *testing.T) {
	app, screen := newTestApp(t)
	screen.SetSize(20, 24) // étroit : force le débordement visuel de la ligne JSON
	waitForDraw(t, screen)

	injectText(screen, "GET _cat/health")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "POST _search")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, `{"query":{"match_all_with_a_very_long_property_name_to_force_wrapping":{}}}`)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "GET _cat/indices")
	waitForDraw(t, screen)

	text := app.editor.Text()
	logicalLines := strings.Split(text, "\n")
	wantRow := len(logicalLines) - 1 // curseur en fin de saisie, dernière ligne logique

	if got := app.editor.CursorLine(); got != wantRow {
		t.Errorf("expected logical CursorLine=%d, got %d\ntext=%q", wantRow, got, text)
	}
	if logicalLines[wantRow] != "GET _cat/indices" {
		t.Fatalf("test setup sanity check failed: last logical line is %q", logicalLines[wantRow])
	}
}

// TestCompletionPrefixWrapSafe vérifie que la complétion Tab cible toujours
// la bonne portion de texte quand une ligne précédente a débordé
// visuellement.
func TestCompletionPrefixWrapSafe(t *testing.T) {
	app, screen := newTestApp(t)
	screen.SetSize(20, 24)
	waitForDraw(t, screen)

	injectText(screen, "POST _search")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, `{"query":{"match_all_with_a_very_long_property_name_to_force_wrapping":{}}}`)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "GET _cat/heal")
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	// La liste par défaut ajoute systématiquement "?v" aux commandes _cat.
	got := app.editor.Text()
	if !strings.HasSuffix(got, "GET _cat/health?v") {
		t.Errorf("expected completion to apply to the last line despite wrapping, got %q", got)
	}
}

// TestHighlightLineWrapSafe vérifie que la recherche (Ctrl+F) dans le
// panneau droit met en évidence le bon contenu (via les régions tview,
// ancrées au texte) plutôt qu'un numéro de ligne affichée — c'est
// précisément ce que TextView.ScrollTo(ligne, ...) ne garantit plus une
// fois le retour à la ligne automatique actif (SPEC.md §3.1) : une ligne
// précédente qui déborde décale toutes les lignes affichées suivantes,
// donc cibler par région plutôt que par index évite le bug par
// construction, indépendamment de la largeur du terminal.
func TestHighlightLineWrapSafe(t *testing.T) {
	app, screen := newTestApp(t)

	body := `{
  "a_very_long_field_name_that_will_definitely_wrap_on_a_narrow_terminal": true,
  "needle": "found_me"
}`
	app.result.Show([]byte(body))
	waitForDraw(t, screen)

	lines := strings.Split(app.result.PlainText(), "\n")
	targetLine := -1
	for i, l := range lines {
		if strings.Contains(l, "needle") {
			targetLine = i
			break
		}
	}
	if targetLine < 0 {
		t.Fatal("test setup sanity check failed: 'needle' line not found in plain text")
	}

	app.result.HighlightLine(targetLine)
	waitForDraw(t, screen)

	if hl := app.result.view.GetHighlights(); len(hl) != 1 || hl[0] != searchRegionID {
		t.Errorf("expected the search region to be highlighted, got %v", hl)
	}
	if region := app.result.view.GetRegionText(searchRegionID); !strings.Contains(region, "needle") {
		t.Errorf("expected the highlighted region to contain the target line, got %q", region)
	}
}
