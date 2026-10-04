package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestCursorLineWrapSafe checks that CursorLine stays correct (logical line
// of the actual text) even when a previous line overflows across several
// display lines — the bug a plain TextArea.GetCursor() would have caused
// once automatic word wrap is active (SPEC.md §3.1).
func TestCursorLineWrapSafe(t *testing.T) {
	app, screen := newTestApp(t)
	screen.SetSize(20, 24) // narrow: forces the JSON line to visually overflow
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

	text := editorText(app)
	logicalLines := strings.Split(text, "\n")
	wantRow := len(logicalLines) - 1 // cursor at end of input, last logical line

	if got := uiValue(app, app.editor.CursorLine); got != wantRow {
		t.Errorf("expected logical CursorLine=%d, got %d\ntext=%q", wantRow, got, text)
	}
	if logicalLines[wantRow] != "GET _cat/indices" {
		t.Fatalf("test setup sanity check failed: last logical line is %q", logicalLines[wantRow])
	}
}

// TestCompletionPrefixWrapSafe checks that Tab completion always targets the
// right portion of text when a previous line has visually overflowed.
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

	// The default list always appends "?v" to _cat commands.
	got := editorText(app)
	if !strings.HasSuffix(got, "GET _cat/health?v") {
		t.Errorf("expected completion to apply to the last line despite wrapping, got %q", got)
	}
}

// TestEditorSearchScrollsMatchIntoView checks that a Ctrl+F match in the
// left panel actually scrolls into view when it's outside the current
// viewport — tview.TextArea.Select's own doc comment is explicit that it
// "preserves" the scroll offset (unlike normal cursor movement via typing
// or arrow keys, which tview keeps on-screen automatically), so without
// Editor.scrollToCursor a match far from the top would be selected
// correctly internally but stay invisible: exactly what a real report
// described as search "landing in the wrong place".
func TestEditorSearchScrollsMatchIntoView(t *testing.T) {
	app, screen := newTestApp(t)
	screen.SetSize(80, 24)
	waitForDraw(t, screen)

	var lines []string
	for i := 0; i < 40; i++ {
		if i == 35 {
			lines = append(lines, "# NEEDLE")
			continue
		}
		lines = append(lines, "# filler line")
	}
	onUI(app, func() {
		app.editor.view.SetText(strings.Join(lines, "\n"), false)
		app.editor.SelectRange(0, 0) // cursor/scroll back to the top before searching
	})
	waitForDraw(t, screen)

	rowOffset := func() int {
		return uiValue(app, func() int {
			row, _ := app.editor.view.GetOffset()
			return row
		})
	}
	if row := rowOffset(); row != 0 {
		t.Fatalf("test setup sanity check failed: expected to start scrolled to the top, got row offset %d", row)
	}

	screen.InjectKey(tcell.KeyCtrlF, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	injectText(screen, "NEEDLE")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)

	row := rowOffset()
	if row == 0 {
		t.Error("expected the match on line 35 to scroll the viewport, row offset is still 0")
	}
	var cursorFromRow, height int
	onUI(app, func() {
		cursorFromRow, _, _, _ = app.editor.view.GetCursor()
		// The editor's own height, not the terminal's: borders, the status
		// bar and the search bar leave it fewer rows than the screen has.
		_, _, _, height = app.editor.view.GetInnerRect()
	})
	if height <= 0 || height >= 24 {
		t.Fatalf("test setup sanity check failed: unexpected editor height %d", height)
	}
	if cursorFromRow < row || cursorFromRow >= row+height {
		t.Errorf("expected the matched row (%d) to fall within the visible viewport [%d, %d), it doesn't", cursorFromRow, row, row+height)
	}
}

// TestHighlightLineWrapSafe checks that search (Ctrl+F) in the right panel
// highlights the right content (via tview regions, anchored to text) rather
// than a display line number — which is exactly what
// TextView.ScrollTo(line, ...) no longer guarantees once automatic word wrap
// is active (SPEC.md §3.1): a previous line that overflows shifts every
// following display line, so targeting by region rather than by index
// avoids the bug by construction, regardless of terminal width.
func TestHighlightLineWrapSafe(t *testing.T) {
	app, screen := newTestApp(t)

	body := `{
  "a_very_long_field_name_that_will_definitely_wrap_on_a_narrow_terminal": true,
  "needle": "found_me"
}`
	onUI(app, func() { app.result.Show("GET", "_search", nil, []byte(body)) })
	waitForDraw(t, screen)

	lines := strings.Split(uiValue(app, app.result.PlainText), "\n")
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

	onUI(app, func() { app.result.HighlightLine(targetLine) })
	waitForDraw(t, screen)

	if hl := uiValue(app, func() []string { return app.result.view.GetHighlights() }); len(hl) != 1 || hl[0] != searchRegionID {
		t.Errorf("expected the search region to be highlighted, got %v", hl)
	}
	if region := uiValue(app, func() string { return app.result.view.GetRegionText(searchRegionID) }); !strings.Contains(region, "needle") {
		t.Errorf("expected the highlighted region to contain the target line, got %q", region)
	}
}
