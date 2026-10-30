package ui

import (
	"math"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// contrast is the WCAG contrast ratio between two colors.
func contrast(a, b tcell.Color) float64 {
	luminance := func(c tcell.Color) float64 {
		r, g, b := c.RGB()
		channel := func(v int32) float64 {
			s := float64(v) / 255
			if s <= 0.03928 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
	}
	la, lb := luminance(a), luminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// TestThemeIsReadable pins the contrast of what the interface puts on what:
// 4.5 for text (WCAG AA), 3 for the borders, which are chrome.
func TestThemeIsReadable(t *testing.T) {
	cases := []struct {
		name    string
		fg, bg  tcell.Color
		minimum float64
	}{
		{"text", themeText, themeBackground, 4.5},
		{"muted text", themeMuted, themeBackground, 4.5},
		{"accent", themeAccent, themeBackground, 4.5},
		{"teal", themeTeal, themeBackground, 4.5},
		{"yellow", themeYellow, themeBackground, 4.5},
		{"pink", themePink, themeBackground, 4.5},
		{"red", themeRed, themeBackground, 4.5},
		{"header", themeText, themeSurface, 4.5},
		{"header, muted", themeMuted, themeSurface, 4.5},
		{"form field", themeText, themeField, 4.5},
		{"Elasticsearch badge", themeBackground, themeTeal, 4.5},
		{"OpenSearch badge", themeBackground, themeAccent, 4.5},
		{"writes confirmed", themeWhite, themeDanger, 4.5},
		{"read-only", themeBackground, themeYellow, 4.5},
		{"border", themeBorder, themeBackground, 3},
	}
	for _, c := range cases {
		if got := contrast(c.fg, c.bg); got < c.minimum {
			t.Errorf("%s: contrast %.1f, expected at least %.1f", c.name, got, c.minimum)
		}
	}
}

// TestThemeSurvivesFewerColors checks that the colors that must stay apart
// still are once a terminal without 24-bit colors has picked its closest
// ones: tcell does that from the palette of the terminal's first colors.
func TestThemeSurvivesFewerColors(t *testing.T) {
	for _, colors := range []int{8, 16, 256} {
		palette := make([]tcell.Color, colors)
		for i := range palette {
			palette[i] = tcell.PaletteColor(i)
		}
		background := tcell.FindColor(themeBackground, palette)
		for name, color := range map[string]tcell.Color{
			"text": themeText, "muted text": themeMuted, "border": themeBorder, "accent": themeAccent,
			"teal": themeTeal, "yellow": themeYellow, "red": themeRed,
		} {
			if tcell.FindColor(color, palette) == background {
				t.Errorf("%d colors: %s can't be told from the background", colors, name)
			}
		}
	}
}

// TestColorTagsFollowTheTheme guards what the theme relies on: tview reads
// a tag's color through tcell.ColorNames.
func TestColorTagsFollowTheTheme(t *testing.T) {
	for name, want := range map[string]tcell.Color{
		"white": themeText, "gray": themeMuted, "aqua": themeAccent, "green": themeTeal,
		"yellow": themeYellow, "fuchsia": themePink, "red": themeRed,
	} {
		if got := tcell.GetColor(name); got != want {
			t.Errorf("[%s]: expected %v, got %v", name, want, got)
		}
	}
}

// TestThemeLeavesNoHole checks that nothing on the main screen, a popup
// included, is left on the terminal's own background: it would show through
// as patches on anything but a black terminal.
func TestThemeLeavesNoHole(t *testing.T) {
	check := func(t *testing.T, screen tcell.SimulationScreen) {
		t.Helper()
		_, width, height := screenContents(screen)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if got := screenBackground(screen, x, y); got == tcell.ColorDefault || got == tcell.ColorBlack {
					t.Fatalf("cell %d,%d: background %v", x, y, got)
				}
			}
		}
	}

	_, screen := newTestApp(t)
	check(t, screen)
	screen.InjectKey(tcell.KeyF1, 0, tcell.ModNone)
	waitForDraw(t, screen)
	check(t, screen)
}
