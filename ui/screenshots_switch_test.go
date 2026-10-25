//go:build screenshots

// Captures of switching between clusters (Ctrl+O), for a visual review:
//
//	go test -tags screenshots -run TestSwitchScreenshots ./ui/
//
// writes ../plans/recette-bascule/captures.html (plans/ isn't tracked). The
// clusters are local test servers: their URLs are 127.0.0.1's.
package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/gnocent/termdevtools/i18n"
)

const switchScreenshotsFile = "../plans/recette-bascule/captures.html"

func TestSwitchScreenshots(t *testing.T) {
	var captures []capture
	shoot := func(h *sessionsHarness, title, check string) {
		t.Helper()
		waitForDraw(t, h.screen)
		captures = append(captures, capture{title, check, renderScreen(h.screen)})
	}
	fr, en := i18n.For(i18n.FR), i18n.For(i18n.EN)

	for _, lang := range []string{i18n.FR, i18n.EN} {
		msgs, in := fr, ""
		if lang == i18n.EN {
			msgs, in = en, "En anglais : "
		}
		a, b := newTestCluster(t, "alpha"), newTestCluster(t, "beta")
		h := newSessionsHarness(t, a, b)
		appA := h.connectTo(t, a)
		if lang == i18n.EN {
			h.key(t, tcell.KeyF3)
		}
		execute(t, appA, h.screen, "GET _cluster/health\n\nGET slow")
		a.waitArrived(t)
		h.key(t, tcell.KeyCtrlO)
		h.connectTo(t, b)
		h.key(t, tcell.KeyCtrlO)
		shoot(h, in+"écran de connexion rouvert par Ctrl+O, deux connexions ouvertes",
			"Les deux clusters ouverts portent « "+msgs.ConnectOpenMarker+" » après leur type d'authentification. Le repère se lit-il, et dit-il qu'on y revient sans formulaire ?")

		h.key(t, tcell.KeyEscape)
		a.releaseAll()
		a.waitOutcome(t)
		shoot(h, in+"réponse reçue sur un cluster hors écran",
			"Le cluster beta est à l'écran ; la requête lente lancée sur alpha vient de répondre. Le message de la barre d'état est-il clair ?")

		h.key(t, tcell.KeyF1)
		for range 29 {
			h.key(t, tcell.KeyDown)
		}
		shoot(h, in+"aide F1, ligne Ctrl+O",
			"Les lignes Ctrl+O et Ctrl+C, en bas de la liste des raccourcis, sont-elles claires ?")
	}

	if err := os.MkdirAll(filepath.Dir(switchScreenshotsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(switchScreenshotsFile, []byte(capturesPage("bascule entre clusters", captures)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d captures written to %s", len(captures), switchScreenshotsFile)
}
