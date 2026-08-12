package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestCatColumnCompletionEndToEnd(t *testing.T) {
	app, screen := newTestApp(t)

	// Complétion h= : préfixe unique parmi les colonnes réelles de _cat/indices.
	injectText(screen, "GET _cat/indices?h=heal")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/indices?h=health"; got != want {
		t.Fatalf("expected %q after h= completion, got %q", want, got)
	}

	// Une deuxième colonne, séparée par une virgule : seule la partie après
	// la dernière virgule doit être complétée, le reste préservé. Les alias
	// courts (ex. "storeSize") ne sont plus dans la liste par défaut, donc
	// "sto" ne correspond plus qu'à "store.size" : complétion directe.
	injectText(screen, ",sto")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/indices?h=health,store.size"; got != want {
		t.Fatalf("expected %q after second h= completion, got %q", want, got)
	}
}

func TestCatColumnSortDirectionCompletionEndToEnd(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/shards?s=index:de")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/shards?s=index:desc"; got != want {
		t.Fatalf("expected %q after sort-direction completion, got %q", want, got)
	}
}

// TestCatColumnCompletionWithFilterSegmentEndToEnd couvre le cas signalé :
// un filtre (nom d'index, de nœud...) entre la commande _cat et les
// paramètres, ex. "_cat/shards/monindex?h=...". La commande _cat doit
// toujours être reconnue malgré ce segment supplémentaire.
func TestCatColumnCompletionWithFilterSegmentEndToEnd(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/shards/monindex?h=stat")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/shards/monindex?h=state"; got != want {
		t.Fatalf("expected %q after h= completion despite the filter segment, got %q", want, got)
	}
}
