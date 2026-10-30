package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// markShown tells whether panel's bottom border carries moreBelowMark.
func markShown(t *testing.T, app *App, screen tcell.SimulationScreen, panel tview.Primitive) bool {
	t.Helper()
	var x, y, width, height int
	onUI(app, func() { x, y, width, height = panel.GetRect() })
	cells, screenWidth, _ := screenContents(screen)
	return cells[(y+height-1)*screenWidth+x+width-2].Runes[0] == moreBelowMark
}

// numberedLines gives count lines of text, each of them different.
func numberedLines(count int) string {
	var lines []string
	for i := 1; i <= count; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	return strings.Join(lines, "\n")
}

func TestEditorMarksWhatIsLeftBelow(t *testing.T) {
	app, screen := newTestApp(t)
	panel := app.editor.Widget()
	if markShown(t, app, screen, panel) {
		t.Error("expected no mark on an empty editor")
	}

	var rows int
	onUI(app, func() { _, _, _, rows = app.editor.view.GetInnerRect() })

	onUI(app, func() { app.editor.SetInitialText(numberedLines(rows)) })
	waitForDraw(t, screen)
	if markShown(t, app, screen, panel) {
		t.Error("expected no mark when the text just fits")
	}

	onUI(app, func() { app.editor.SetInitialText(numberedLines(rows + 1)) })
	waitForDraw(t, screen)
	if !markShown(t, app, screen, panel) {
		t.Error("expected a mark when a line is left below")
	}

	onUI(app, func() { app.editor.view.SetOffset(1, 0) })
	waitForDraw(t, screen)
	if markShown(t, app, screen, panel) {
		t.Error("expected no mark once scrolled to the end")
	}
}

func TestResultMarksWhatIsLeftBelow(t *testing.T) {
	app, screen := newTestApp(t)
	panel := app.result.Widget()
	var rows int
	onUI(app, func() { _, _, _, rows = app.result.view.GetInnerRect() })

	show := func(text string) {
		t.Helper()
		onUI(app, func() { app.result.Show("GET", "_cat/health", nil, []byte(text)) })
		waitForDraw(t, screen)
	}

	show("short")
	if markShown(t, app, screen, panel) {
		t.Error("expected no mark on a short result")
	}

	show(numberedLines(rows - 1)) // the request reminder takes a row
	if markShown(t, app, screen, panel) {
		t.Error("expected no mark when the result just fits")
	}

	show(numberedLines(rows))
	if !markShown(t, app, screen, panel) {
		t.Error("expected a mark when a line is left below")
	}

	onUI(app, app.focusResultPanel)
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if markShown(t, app, screen, panel) {
		t.Error("expected no mark once scrolled to the end")
	}
	screen.InjectKey(tcell.KeyUp, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if !markShown(t, app, screen, panel) {
		t.Error("expected the mark back after scrolling up")
	}
}

// TestResultMarkCountsWrappedRows checks that what counts is the rows
// displayed: a single line can overflow the panel, once wrapped.
func TestResultMarkCountsWrappedRows(t *testing.T) {
	app, screen := newTestApp(t)
	onUI(app, func() { app.result.Show("GET", "_cat/health", nil, []byte(strings.Repeat("a", 2000))) })
	waitForDraw(t, screen)
	if !markShown(t, app, screen, app.result.Widget()) {
		t.Error("expected a mark on a line wrapped past the bottom of the panel")
	}
}

// TestMarkLeavesScrollingAlone checks that finding out what is left below
// moves nothing: not the position, and not the next key's effect.
func TestMarkLeavesScrollingAlone(t *testing.T) {
	app, screen := newTestApp(t)
	onUI(app, func() { app.result.Show("GET", "_cat/health", nil, []byte(numberedLines(200))) })
	waitForDraw(t, screen)
	onUI(app, app.focusResultPanel)

	offset := func() int {
		return uiValue(app, func() int {
			row, _ := app.result.view.GetScrollOffset()
			return row
		})
	}
	if got := offset(); got != 0 {
		t.Fatalf("expected the result at its top, got row %d", got)
	}
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if got := offset(); got != 2 {
		t.Errorf("expected row 2 after two steps down, got %d", got)
	}
	screen.InjectKey(tcell.KeyEnd, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if markShown(t, app, screen, app.result.Widget()) {
		t.Error("expected no mark after End")
	}
}

func TestHelpMarksWhatIsLeftBelow(t *testing.T) {
	app, screen := newTestApp(t)
	screen.InjectKey(tcell.KeyF1, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if !markShown(t, app, screen, app.helpPanel) {
		t.Error("expected a mark on a help screen that doesn't fit")
	}
	screen.InjectKey(tcell.KeyEnd, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if markShown(t, app, screen, app.helpPanel) {
		t.Error("expected no mark at the end of the help screen")
	}
}
