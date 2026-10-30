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
// it doesn't fit: the message, at the expense of the shortcuts, dropped one
// by one and from the last, each of them whole.
func TestStatusLineGivesWayToTheMessage(t *testing.T) {
	sb := NewStatusBar(i18n.For(i18n.FR))
	cases := []struct {
		message string
		width   int
		want    string
	}{
		{"prêt", 40, "prêt            F1 aide  Ctrl+E exécuter"},
		{"prêt", 80, "prêt            F1 aide  Ctrl+E exécuter  F8 recettes  Ctrl+C quitter  F3 langue"},
		{"HTTP 200 en 42ms", 78, "HTTP 200 en 42ms         F1 aide  Ctrl+E exécuter  F8 recettes  Ctrl+C quitter"},
		{"envoi annulé : rien n'a été envoyé", 78, "envoi annulé : rien n'a été envoyé       F1 aide  Ctrl+E exécuter  F8 recettes"},
		{"envoi annulé : rien n'a été envoyé", 50, "envoi annulé : rien n'a été envoyé         F1 aide"},
		{"envoi annulé : rien n'a été envoyé", 43, "envoi annulé : rien n'a été envoyé"},
		{"envoi annulé : rien n'a été envoyé", 20, "envoi annulé : rien…"},
	}
	for _, c := range cases {
		sb.SetInfo(c.message)
		got := plainText(sb.compose(c.width))
		if got != c.want {
			t.Errorf("%q in %d columns: expected\n%q, got\n%q", c.message, c.width, c.want, got)
		}
		if used := uniseg.StringWidth(got); used > c.width {
			t.Errorf("%q in %d columns: %d columns used", c.message, c.width, used)
		}
	}
}

// TestStatusLineKeepsColorsAndBrackets checks that the message keeps its
// color once the line is cut short, and that brackets in it aren't taken
// for color tags.
func TestStatusLineKeepsColorsAndBrackets(t *testing.T) {
	sb := NewStatusBar(i18n.For(i18n.FR))
	sb.SetError("[red] HTTP 404 sur /my-index")
	got := sb.compose(30)
	if !strings.HasPrefix(got, "[red]") || plainText(got) != "[red] HTTP 404 sur /my-index" {
		t.Errorf("expected the message alone, in red and as written, got %q (%q)", got, plainText(got))
	}
}
