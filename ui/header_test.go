package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/i18n"
	"github.com/gnocent/termdevtools/refdata"
)

// es954 and os2196 are the targets of the header's tests.
var (
	es954  = es95
	os2196 = refdata.Target{Distribution: refdata.OpenSearch, Version: refdata.Version{Major: 2, Minor: 19, Patch: 6}, HasVersion: true}
)

// headerText returns what app's header says, whatever its width.
func headerText(app *App) string {
	return uiValue(app, func() string { return plainText(app.header.compose(0)) })
}

// TestHeaderGivesWayInOrder checks what the header keeps when it doesn't
// fit: the brand goes first, then the user, then the URL is cut short, and
// last it goes too.
func TestHeaderGivesWayInOrder(t *testing.T) {
	const url, user = "https://es-prod.example.test:9200", "svc_devtools"
	h := newHeader(es954, url, user, config.ProtectionNone, "", i18n.For(i18n.FR))
	cases := []struct {
		width int
		want  string
	}{
		{78, "TermDevTools  ES 9.5.4  https://es-prod.example.test:9200         svc_devtools"},
		{71, "TermDevTools  ES 9.5.4  https://es-prod.example.test:9200  svc_devtools"},
		{70, " ES 9.5.4  https://es-prod.example.test:9200" + strings.Repeat(" ", 14) + "svc_devtools"},
		{58, " ES 9.5.4  https://es-prod.example.test:9200  svc_devtools"},
		{57, " ES 9.5.4  https://es-prod.example.test:9200"},
		{30, " ES 9.5.4  https://es-prod.ex…"},
		{23, " ES 9.5.4  https://es-…"},
		{22, " ES 9.5.4 "},
	}
	for _, c := range cases {
		got := plainText(h.compose(c.width))
		if got != c.want {
			t.Errorf("%d columns: expected\n%q, got\n%q", c.width, c.want, got)
		}
		if used := uniseg.StringWidth(got); used > c.width {
			t.Errorf("%d columns: %d columns used", c.width, used)
		}
	}
}

// TestHeaderKeepsTheProtection checks that a protection, unlike the brand,
// stays however little room there is.
func TestHeaderKeepsTheProtection(t *testing.T) {
	h := newHeader(es954, "https://es-prod.example.test:9200", "svc_devtools", config.ProtectionConfirm, "PROD", i18n.For(i18n.FR))
	cases := []struct {
		width int
		want  string
	}{
		{78, "PROD · confirmation des écritures  ES 9.5.4  https://es-prod.example.test:9200"},
		{60, "PROD · confirmation des écritures  ES 9.5.4  https://es-pro…"},
		{50, "PROD · confirmation des écritures  ES 9.5.4 "},
	}
	for _, c := range cases {
		if got := plainText(h.compose(c.width)); got != c.want {
			t.Errorf("%d columns: expected\n%q, got\n%q", c.width, c.want, got)
		}
	}
}

// TestHeaderShowsWhatIsWritten checks that brackets in what a cluster's
// settings hold aren't taken for color tags.
func TestHeaderShowsWhatIsWritten(t *testing.T) {
	h := newHeader(refdata.Target{}, "http://[::1]:9200", "[red]svc", config.ProtectionReadOnly, "[red]PROD", i18n.For(i18n.FR))
	got := plainText(h.compose(0))
	for _, want := range []string{"[red]PROD · lecture seule", "http://[::1]:9200", "[red]svc"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in %q", want, got)
		}
	}
}

// TestHeaderBadgeTellsDistributionsApart checks the color of the badge, on
// the main screen: what tells two clusters apart at a glance with Ctrl+O.
func TestHeaderBadgeTellsDistributionsApart(t *testing.T) {
	cases := []struct {
		name string
		opts testAppOptions
		want string
		bg   tcell.Color
	}{
		{"Elasticsearch", testAppOptions{target: es954}, " ES 9.5.4 ", themeTeal},
		{"OpenSearch", testAppOptions{target: os2196}, " OS 2.19.6 ", themeAccent},
		{"unknown", testAppOptions{undetected: true}, " Cluster ", themeField},
		{"protected", testAppOptions{target: es954, protection: config.ProtectionConfirm, label: "PROD"}, " ES 9.5.4 ", themeBackground},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, screen := newTestAppWith(t, c.opts)
			column := strings.Index(screenRow(screen, 0), c.want)
			if column < 0 {
				t.Fatalf("expected %q on the first row, got %q", c.want, screenRow(screen, 0))
			}
			column = uniseg.StringWidth(screenRow(screen, 0)[:column])
			for _, x := range []int{column, column + len(c.want) - 1} {
				if got := screenBackground(screen, x, 0); got != c.bg {
					t.Errorf("column %d: expected background %v, got %v", x, c.bg, got)
				}
			}
		})
	}
}

// TestHeaderAndStatusLineTakeOneRowEach checks what makes the header free:
// the panels keep the height they had with a status line and a help bar.
func TestHeaderAndStatusLineTakeOneRowEach(t *testing.T) {
	_, screen := newTestApp(t)
	_, _, height := screenContents(screen)

	if row := screenRow(screen, 1); !strings.Contains(row, "Requêtes") || !strings.Contains(row, "Résultat") {
		t.Errorf("expected the top of the panels on the second row, got %q", row)
	}
	if row := screenRow(screen, height-2); !strings.Contains(row, "┗") {
		t.Errorf("expected the bottom of the panels on the row before last, got %q", row)
	}
	if row := screenRow(screen, height-1); !strings.Contains(row, "prêt") || !strings.Contains(row, "F1 aide") {
		t.Errorf("expected the message and the shortcuts on the last row, got %q", row)
	}
}

// TestFocusShowsOnThePanels checks that the panel with the focus stands out
// by its shape and its color, and that the focus moves with the keyboard.
func TestFocusShowsOnThePanels(t *testing.T) {
	app, screen := newTestApp(t)

	borderColor := func(x int) tcell.Color {
		cells, width, _ := screenContents(screen)
		color, _, _ := cells[1*width+x].Style.Decompose()
		return color
	}
	if row := screenRow(screen, 1); !strings.HasPrefix(row, "┏") || !strings.HasSuffix(row, "┐") {
		t.Errorf("expected the editor's border to be the focused one, got %q", row)
	}
	if got := borderColor(0); got != themeAccent {
		t.Errorf("expected the editor's border in the accent color, got %v", got)
	}
	if got := borderColor(79); got != themeBorder {
		t.Errorf("expected the result's border in the plain color, got %v", got)
	}

	onUI(app, app.focusResultPanel)
	waitForDraw(t, screen)
	if row := screenRow(screen, 1); !strings.HasPrefix(row, "┌") || !strings.HasSuffix(row, "┓") {
		t.Errorf("expected the result's border to be the focused one, got %q", row)
	}
	if got := borderColor(79); got != themeAccent {
		t.Errorf("expected the result's border in the accent color, got %v", got)
	}
}
