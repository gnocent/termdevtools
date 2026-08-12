package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/config"
	"termdevtools/esclient"
)

// newTestApp démarre une App complète sur un écran simulé (aucun terminal
// ni cluster réel requis : esclient.New ne se connecte pas tant qu'aucune
// requête n'est exécutée). Sert à vérifier, sans pty, le câblage du popup
// de complétion Tab qu'aucun test unitaire pur ne peut couvrir.
func newTestApp(t *testing.T) (*App, tcell.SimulationScreen) {
	t.Helper()

	client, err := esclient.New(esclient.Params{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("esclient.New: %v", err)
	}
	cr := ConnectResult{Client: client, Cluster: config.Cluster{URL: "http://127.0.0.1:1"}, DisplayUser: "test"}
	cfg := &config.Config{DefaultTimeoutSeconds: 5}

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(80, 24)

	tapp := tview.NewApplication().SetScreen(screen)
	paths := Paths{
		Cheatsheet: "/nonexistent/cheatsheet.txt",
		Exports:    t.TempDir(),
		Endpoints:  "/nonexistent/endpoints.txt",
		CatColumns: "/nonexistent/cat_columns.txt",
	}
	app := NewApp(tapp, cr, cfg, paths)
	tapp.SetRoot(app.Root(), true)
	app.Start()

	go func() {
		_ = tapp.Run()
	}()
	t.Cleanup(tapp.Stop)

	waitForDraw(t, screen)
	return app, screen
}

// waitForDraw laisse le temps à la goroutine de l'Application de traiter
// les événements déjà injectés et de redessiner.
func waitForDraw(t *testing.T, screen tcell.SimulationScreen) {
	t.Helper()
	time.Sleep(30 * time.Millisecond)
}

func injectText(screen tcell.SimulationScreen, s string) {
	for _, r := range s {
		screen.InjectKey(tcell.KeyRune, r, tcell.ModNone)
	}
}

func TestCompletionSingleMatchAppliesInline(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/pl")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	got := app.editor.Text()
	want := "GET _cat/plugins?v"
	if got != want {
		t.Errorf("expected editor text %q after single-match completion, got %q", want, got)
	}
}

// TestCompletionTrailingSlashIsIgnored couvre le cas signalé : le "/" final
// avant les paramètres (ou en toute fin de chemin) est optionnel en HTTP —
// "_cat/plugins/" doit se compléter comme "_cat/plugins", pas échouer faute
// de correspondance exacte (aucun endpoint connu ne stocke de "/" final).
func TestCompletionTrailingSlashIsIgnored(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/plugins/")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	got := app.editor.Text()
	want := "GET _cat/plugins?v"
	if got != want {
		t.Errorf("expected the trailing '/' to be ignored and replaced, got %q want %q", got, want)
	}
}

func TestCompletionMultiMatchOpensListAndEnterApplies(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/s")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if app.completionList.GetItemCount() < 2 {
		t.Fatalf("expected multiple completion candidates, got %d", app.completionList.GetItemCount())
	}
	if app.tapp.GetFocus() != app.completionList {
		t.Fatal("expected focus to be on the completion list while it's open")
	}

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if app.tapp.GetFocus() != app.editor.Primitive() {
		t.Error("expected focus to return to the editor after selecting a completion")
	}

	firstMatch, _ := app.completionList.GetItemText(0)
	_ = firstMatch
	got := app.editor.Text()
	if got == "GET _cat/s" {
		t.Error("expected the editor text to change after Enter, it did not")
	}
	t.Logf("editor text after completion: %q", got)
}

func TestCompletionEscapeCancelsWithoutChange(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/s")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := app.editor.Text(), "GET _cat/s"; got != want {
		t.Errorf("expected editor text unchanged after Escape, got %q want %q", got, want)
	}
	if app.tapp.GetFocus() != app.editor.Primitive() {
		t.Error("expected focus back on the editor after Escape")
	}
}

func TestTabInsideJSONBodyIsNotIntercepted(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "POST _search")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "{")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	want := "POST _search\n{\t"
	if got := app.editor.Text(); got != want {
		t.Errorf("expected a literal tab inserted in JSON body context, got %q want %q", got, want)
	}
}
