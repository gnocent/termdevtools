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

	text := app.editor.Text()
	logicalLines := strings.Split(text, "\n")
	wantRow := len(logicalLines) - 1 // cursor at end of input, last logical line

	if got := app.editor.CursorLine(); got != wantRow {
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
	got := app.editor.Text()
	if !strings.HasSuffix(got, "GET _cat/health?v") {
		t.Errorf("expected completion to apply to the last line despite wrapping, got %q", got)
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
