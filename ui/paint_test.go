package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// editorCell returns the rune and style of a cell of the editor's text, at
// column and row within what it displays.
func editorCell(t *testing.T, app *App, screen tcell.SimulationScreen, column, row int) (rune, tcell.Style) {
	t.Helper()
	var left, top int
	onUI(app, func() { left, top, _, _ = app.editor.view.GetInnerRect() })
	cells, width, _ := screenContents(screen)
	cell := cells[(top+row)*width+left+column]
	return cell.Runes[0], cell.Style
}

// paintedApp opens an app whose editor shows text.
func paintedApp(t *testing.T, text string) (*App, tcell.SimulationScreen) {
	t.Helper()
	app, screen := newTestApp(t)
	onUI(app, func() { app.editor.SetInitialText(text) })
	waitForDraw(t, screen)
	return app, screen
}

// expectCell checks the character, color and weight of a cell.
func expectCell(t *testing.T, app *App, screen tcell.SimulationScreen, column, row int, char rune, color tcell.Color, bold bool) {
	t.Helper()
	got, style := editorCell(t, app, screen, column, row)
	foreground, _, attributes := style.Decompose()
	if got != char || foreground != color || (attributes&tcell.AttrBold != 0) != bold {
		t.Errorf("cell %d,%d: expected %q in %v (bold: %v), got %q in %v (bold: %v)",
			column, row, char, color, bold, got, foreground, attributes&tcell.AttrBold != 0)
	}
}

func TestMethodsAndCommentsArePainted(t *testing.T) {
	app, screen := paintedApp(t, "# health\nGET _cat/health\n\nPUT my-index\n{\n  \"size\": 1\n}\n\nDELETE old\npost a/_search")

	expectCell(t, app, screen, 0, 0, '#', themeMuted, false)
	expectCell(t, app, screen, 7, 0, 'h', themeMuted, false)

	expectCell(t, app, screen, 0, 1, 'G', themeTeal, true)
	expectCell(t, app, screen, 2, 1, 'T', themeTeal, true)
	expectCell(t, app, screen, 4, 1, '_', themeText, false) // the path stays plain

	expectCell(t, app, screen, 0, 3, 'P', themeAccent, true)
	expectCell(t, app, screen, 0, 4, '{', themeText, false) // and so does a body
	expectCell(t, app, screen, 3, 5, 's', themeText, false)

	expectCell(t, app, screen, 0, 8, 'D', themeRed, true)
	expectCell(t, app, screen, 5, 8, 'E', themeRed, true)
	expectCell(t, app, screen, 7, 8, 'o', themeText, false)

	expectCell(t, app, screen, 0, 9, 'p', themeAccent, true) // as the parser reads it
}

// TestOnlyRequestLinesArePainted checks that a line that isn't a request is
// left alone, whatever it starts with: the color says what will be sent.
func TestOnlyRequestLinesArePainted(t *testing.T) {
	app, screen := paintedApp(t, "GET\nDELETE two words\nGETTING there\n\tGET tabbed")
	for row, char := range []rune{'G', 'D', 'G'} {
		expectCell(t, app, screen, 0, row, char, themeText, false)
	}
	got, style := editorCell(t, app, screen, tview.TabSize, 3)
	if foreground, _, _ := style.Decompose(); got != 'G' || foreground != themeText {
		t.Errorf("expected a request behind a tab left alone, got %q in %v", got, foreground)
	}
}

func TestIndentedRequestIsPaintedWhereItIs(t *testing.T) {
	app, screen := paintedApp(t, "  DELETE old\n  # note")
	expectCell(t, app, screen, 0, 0, ' ', themeText, false)
	expectCell(t, app, screen, 2, 0, 'D', themeRed, true)
	expectCell(t, app, screen, 2, 1, '#', themeMuted, false)
}

// TestPaintStaysInsideTheEditor checks that on a panel narrower than a
// method, what doesn't fit isn't painted over what stands next to the editor.
func TestPaintStaysInsideTheEditor(t *testing.T) {
	app, screen := paintedApp(t, "DELETE old")
	onUI(app, func() {
		for app.leftWeight > splitMinWeight {
			app.resizeSplit(-splitStep)
		}
	})
	waitForDraw(t, screen)

	var left, top, width int
	onUI(app, func() { left, top, width, _ = app.editor.view.GetInnerRect() })
	if width >= len("DELETE") {
		t.Fatalf("expected a panel narrower than the method, got %d columns", width)
	}
	cells, screenWidth, _ := screenContents(screen)
	for x := left + width; x < left+len("DELETE"); x++ {
		foreground, _, attributes := cells[top*screenWidth+x].Style.Decompose()
		if foreground == themeRed || attributes&tcell.AttrBold != 0 {
			t.Errorf("column %d: expected nothing painted outside the editor, got %v", x, foreground)
		}
	}
}

// TestSelectionKeepsItsColors checks that the selection, which F-keys and
// the search act on, is drawn as usual over a method.
func TestSelectionKeepsItsColors(t *testing.T) {
	app, screen := paintedApp(t, "GET _cat/health")
	onUI(app, func() { app.editor.SelectRange(0, 2) })
	waitForDraw(t, screen)

	_, style := editorCell(t, app, screen, 0, 0)
	if foreground, background, _ := style.Decompose(); foreground != themeBackground || background != themeText {
		t.Errorf("expected the selection's colors, got %v on %v", foreground, background)
	}
	expectCell(t, app, screen, 2, 0, 'T', themeTeal, true)
}

// TestPaintFollowsDisplayRows checks the painting against what the gutter
// also relies on: wrapped lines and scrolling.
func TestPaintFollowsDisplayRows(t *testing.T) {
	t.Run("a wrapped comment, on all its rows", func(t *testing.T) {
		app, screen := paintedApp(t, "# "+strings.Repeat("word ", 20)+"\nGET next")
		expectCell(t, app, screen, 0, 1, 'w', themeMuted, false)
		// The request comes after the comment's rows, wherever they end.
		rows := uiValue(app, func() int { return len(app.editor.wrappedStarts) })
		expectCell(t, app, screen, 0, rows-1, 'G', themeTeal, true)
	})

	t.Run("a wrapped request, on its first row only", func(t *testing.T) {
		app, screen := paintedApp(t, "GET "+strings.Repeat("a", 60))
		expectCell(t, app, screen, 0, 0, 'G', themeTeal, true)
		expectCell(t, app, screen, 0, 1, 'a', themeText, false)
	})

	t.Run("scrolled into a wrapped comment", func(t *testing.T) {
		app, screen := paintedApp(t, "# "+strings.Repeat("word ", 20))
		onUI(app, func() { app.editor.view.SetOffset(1, 0) })
		waitForDraw(t, screen)
		expectCell(t, app, screen, 0, 0, 'w', themeMuted, false)
	})

	t.Run("scrolled", func(t *testing.T) {
		text := strings.Repeat("plain line\n", 40) + "DELETE old\nGET next"
		app, screen := paintedApp(t, text)
		onUI(app, func() { app.editor.view.SetOffset(35, 0) })
		waitForDraw(t, screen)
		expectCell(t, app, screen, 0, 0, 'p', themeText, false)
		expectCell(t, app, screen, 0, 5, 'D', themeRed, true)
		expectCell(t, app, screen, 0, 6, 'G', themeTeal, true)
	})
}
