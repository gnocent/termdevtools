package ui

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/gnocent/termdevtools/i18n"
)

// plainText strips the color tags from a status line.
func plainText(tagged string) string {
	return tview.NewTextView().SetDynamicColors(true).SetText(tagged).GetText(true)
}

// TestStatusLineGivesWayToTheMessage checks what the status line keeps when
// it doesn't fit: the message, at the expense of the user, then of the URL,
// then of the cluster altogether.
func TestStatusLineGivesWayToTheMessage(t *testing.T) {
	sb := NewStatusBar("ES 9.5.4", "https://es-prod.example.test:9200", "svc_devtools", i18n.For(i18n.FR))
	cases := []struct {
		message string
		width   int
		want    string
	}{
		{"prêt", 0, "ES 9.5.4: https://es-prod.example.test:9200  |  Utilisateur: svc_devtools  |  prêt"},
		{"prêt", 82, "ES 9.5.4: https://es-prod.example.test:9200  |  Utilisateur: svc_devtools  |  prêt"},
		{"prêt", 81, "ES 9.5.4: https://es-prod.example.test:9200  |  prêt"},
		{"envoi annulé : rien n'a été envoyé", 82, "ES 9.5.4: https://es-prod.example.test:9200  |  envoi annulé : rien n'a été envoyé"},
		{"envoi annulé : rien n'a été envoyé", 80, "ES 9.5.4: https://es-prod.example.test:9…  |  envoi annulé : rien n'a été envoyé"},
		{"envoi annulé : rien n'a été envoyé", 61, "ES 9.5.4: https://es-…  |  envoi annulé : rien n'a été envoyé"},
		{"envoi annulé : rien n'a été envoyé", 60, "envoi annulé : rien n'a été envoyé"},
		{"envoi annulé : rien n'a été envoyé", 20, "envoi annulé : rien…"},
	}
	for _, c := range cases {
		sb.SetInfo(c.message)
		if got := plainText(sb.compose(c.width)); got != c.want {
			t.Errorf("%q in %d columns: expected\n%q, got\n%q", c.message, c.width, c.want, got)
		}
		if got := plainText(sb.compose(c.width)); c.width > 0 && uniseg.StringWidth(got) > c.width {
			t.Errorf("%q in %d columns: %d columns used", c.message, c.width, uniseg.StringWidth(got))
		}
	}
}

// TestStatusLineKeepsColorsAndBrackets checks that the message keeps its
// color once the line is cut short, and that brackets in it aren't taken
// for color tags.
func TestStatusLineKeepsColorsAndBrackets(t *testing.T) {
	sb := NewStatusBar("ES 9.5.4", "https://es-prod.example.test:9200", "svc_devtools", i18n.For(i18n.FR))
	sb.SetError("[red] HTTP 404 sur /my-index")
	got := sb.compose(40)
	if !strings.HasPrefix(got, "[red]") || plainText(got) != "[red] HTTP 404 sur /my-index" {
		t.Errorf("expected the message alone, in red and as written, got %q (%q)", got, plainText(got))
	}
}
