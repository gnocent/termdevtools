package ui

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"termdevtools/refdata"
)

// hostile is what a compromised cluster — or a file dropped in a shared
// directory — could put in a text this application displays: terminal escape
// sequences. OSC 52 writes to the clipboard, others retitle the window or
// hide what follows; none of it may ever be written to the terminal as is.
const hostile = "seen\x1b]52;c;aGFjaw==\x07 \x1b[8mhidden\x1b[0m \x9b31mcsi \x07bell\x7fdel\x00nul end"

// controlCells returns a description of every cell of the screen holding a
// control character — what the terminal would be sent raw.
func controlCells(screen tcell.SimulationScreen) []string {
	cells, width, _ := screen.GetContents()
	var found []string
	for i, cell := range cells {
		for _, r := range cell.Runes {
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
				found = append(found, fmt.Sprintf("U+%04X at column %d, row %d", r, i%width, i/width))
			}
		}
	}
	return found
}

// TestControlCharactersNeverReachTheTerminal displays hostile text through
// every way in that isn't the user's own typing, and checks the screen.
func TestControlCharactersNeverReachTheTerminal(t *testing.T) {
	check := func(t *testing.T, screen tcell.SimulationScreen, wantVisible string) {
		t.Helper()
		waitForDraw(t, screen)
		if found := controlCells(screen); len(found) > 0 {
			t.Errorf("control characters on screen: %v", found)
		}
		if text := screenText(screen); !strings.Contains(text, wantVisible) {
			t.Errorf("expected the readable part of the text (%q) to be displayed, got:\n%s", wantVisible, text)
		}
	}

	t.Run("response body", func(t *testing.T) {
		app, screen := newTestApp(t)
		onUI(app, func() { app.result.Show("GET", "_cat/health", nil, []byte(hostile+"\nsecond line")) })
		check(t, screen, "second line")
	})

	t.Run("response headers", func(t *testing.T) {
		app, screen := newTestApp(t)
		onUI(app, func() {
			app.result.Show("GET", "_cluster/health", http.Header{"X-Hostile": {hostile}}, []byte(`{"ok":true}`))
		})
		check(t, screen, "X-Hostile")
	})

	t.Run("error message", func(t *testing.T) {
		app, screen := newTestApp(t)
		onUI(app, func() {
			app.result.ShowError("GET", "_cluster/health", hostile)
			app.status.SetError(hostile)
		})
		check(t, screen, "seen")
	})

	t.Run("editor content", func(t *testing.T) {
		// A cheatsheet lives next to the binary, in a directory that may be
		// shared: its content is loaded into the editor as is.
		_, screen := newTestAppWith(t, testAppOptions{cheatsheet: "# " + hostile + "\nGET _cluster/health\n"})
		check(t, screen, "GET _cluster/health")
	})

	// The two cases below put hostile text straight into the application's
	// state: the files it could come from are refused when read (see the
	// refdata package), which is one more reason for it never to get there,
	// not a reason for the display to rely on it.

	t.Run("completion list", func(t *testing.T) {
		app, screen := newTestApp(t)
		onUI(app, func() { app.endpoints = []string{"_zz/one\x1b[8m", "_zz/two\x1b]0;x\x07"} })
		injectText(screen, "GET _zz")
		screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
		eventually(t, "the completion list", func() bool { return completionCount(app) == 2 })
		check(t, screen, "_zz/one")
	})

	t.Run("recipe catalog", func(t *testing.T) {
		app, screen := newTestApp(t)
		onUI(app, func() {
			app.recipes = []refdata.Recipe{{
				Group:  "Hostile\x1b[8m",
				Title:  "Title\x1b]0;x\x07 visible",
				Body:   "# comment \x1b[31m\nGET _cluster/health",
				Source: filepath.Join("some", "dir", "na\x1b[8mme.txt"),
			}}
		})
		openPalette(t, app, screen)
		check(t, screen, "visible")
	})
}
