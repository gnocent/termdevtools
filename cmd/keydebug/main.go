// Command keydebug is a diagnostic tool: it prints exactly how the current
// terminal reports each key press (Key, Rune, Modifiers) via tcell, the
// same library TermDevTools uses. Used to debug why a keyboard shortcut
// doesn't behave as expected on a given terminal/platform, instead of
// guessing — see SPEC.md §4 for prior findings from this kind of
// investigation (Windows, various terminals).
//
// Not part of the main termdevtools binary; build and run separately:
//
//	go run ./cmd/keydebug
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
)

func main() {
	screen, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintln(os.Stderr, "NewScreen:", err)
		os.Exit(1)
	}
	if err := screen.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "Init:", err)
		os.Exit(1)
	}
	defer screen.Fini()

	header := []string{
		"keydebug — press keys/combos to see exactly how this terminal reports them.",
		"Try: Enter, Ctrl+Enter, Option+Enter, Alt+Enter, Left/Right with Ctrl and Option/Alt.",
		"Press Ctrl+C twice in a row to quit (single Ctrl+C is logged like any other key).",
		"",
	}

	row := len(header)
	draw := func() {
		screen.Clear()
		for y, line := range header {
			drawLine(screen, y, line)
		}
		screen.Show()
	}
	draw()

	lastWasCtrlC := false
	for {
		ev := screen.PollEvent()
		e, ok := ev.(*tcell.EventKey)
		if !ok {
			if _, isResize := ev.(*tcell.EventResize); isResize {
				screen.Sync()
			}
			continue
		}

		if e.Key() == tcell.KeyCtrlC {
			if lastWasCtrlC {
				return
			}
			lastWasCtrlC = true
		} else {
			lastWasCtrlC = false
		}

		line := fmt.Sprintf("Key=%-14v Rune=%-8q (%d)  Modifiers=%s", e.Key(), e.Rune(), e.Rune(), modString(e.Modifiers()))
		if row >= 60 {
			draw()
			row = len(header)
		}
		drawLine(screen, row, line)
		row++
		screen.Show()
	}
}

func modString(m tcell.ModMask) string {
	var parts []string
	if m&tcell.ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if m&tcell.ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if m&tcell.ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if m&tcell.ModMeta != 0 {
		parts = append(parts, "Meta")
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, "+")
}

func drawLine(screen tcell.Screen, y int, text string) {
	for x, r := range text {
		screen.SetContent(x, y, r, nil, tcell.StyleDefault)
	}
}
