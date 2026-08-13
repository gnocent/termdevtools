package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/config"
	"termdevtools/esclient"
)

// newTestApp starts a full App on a simulated screen (no real terminal or
// cluster required: esclient.New doesn't connect until a request is
// executed). Used to check, without a pty, the Tab completion popup's
// wiring, which no pure unit test can cover.
func newTestApp(t *testing.T) (*App, tcell.SimulationScreen) {
	t.Helper()
	return newTestAppLang(t, "")
}

// newTestAppLang is newTestApp with an explicit interface language ("fr",
// "en", or "" for the default), to verify config.Config.Language is
// actually honored end-to-end (see TestInterfaceLanguageEnglish).
func newTestAppLang(t *testing.T, lang string) (*App, tcell.SimulationScreen) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // sandbox config.Save() (F3 language toggle) away from the real user config

	client, err := esclient.New(esclient.Params{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("esclient.New: %v", err)
	}
	cr := ConnectResult{Client: client, Cluster: config.Cluster{URL: "http://127.0.0.1:1"}, DisplayUser: "test"}
	cfg := &config.Config{DefaultTimeoutSeconds: 5, Language: lang}

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

// waitForDraw gives the Application's goroutine time to process the
// already-injected events and redraw.
func waitForDraw(t *testing.T, screen tcell.SimulationScreen) {
	t.Helper()
	time.Sleep(30 * time.Millisecond)
}

func injectText(screen tcell.SimulationScreen, s string) {
	for _, r := range s {
		screen.InjectKey(tcell.KeyRune, r, tcell.ModNone)
	}
}

// TestF10TriggersCompletionLikeTab checks that F10 — the guaranteed-
// reliable alternative added after Tab was confirmed swallowed entirely by
// two unrelated terminals (Windows cmd.exe and PuTTY) on the same real
// machine — completes an endpoint exactly like Tab (see isCompletionShortcut
// in app.go).
func TestF10TriggersCompletionLikeTab(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/pl")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	waitForDraw(t, screen)

	got := app.editor.Text()
	want := "GET _cat/plugins?v"
	if got != want {
		t.Errorf("expected F10 to complete like Tab, got %q want %q", got, want)
	}
}

// TestF10OutsideCompletionContextIsSwallowed checks that F10, unlike Tab,
// never inserts a literal character when there's nothing to complete — it
// has no "normal typing key" fallback meaning.
func TestF10OutsideCompletionContextIsSwallowed(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "POST _search")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "{")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	waitForDraw(t, screen)

	want := "POST _search\n{"
	if got := app.editor.Text(); got != want {
		t.Errorf("expected F10 to be swallowed with no effect outside a completion context, got %q want %q", got, want)
	}
}

// TestF10ShowsDebugContextOutsideCompletion checks that F10 reports the
// line text and cursor row/col it saw, when it doesn't recognize a
// completion context — added to self-diagnose a real report where the
// context detection itself, not the F10 keystroke, was the actual problem
// (a "GET " line failed to be recognized on one specific machine, despite
// F10 itself arriving as a clean, unambiguous KeyF10 event there).
func TestF10ShowsDebugContextOutsideCompletion(t *testing.T) {
	_, screen := newTestApp(t)

	// A JSON body line is a reliable, unambiguous "no completion context"
	// case: unlike "GET " (empty prefix — matches with the *full* endpoint
	// list, not what's under test here), "{" can never match
	// completionLineRe at all.
	injectText(screen, "POST _search")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "{")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	waitForDraw(t, screen)

	// The full message (including the status bar's own "Cluster: ... |
	// Utilisateur: ..." prefix) can exceed the simulated screen's 80
	// columns before reaching the %q line text — checking the message's
	// own stable lead-in is enough to confirm this code path fired.
	text := screenText(screen)
	if !strings.Contains(text, "pas de contexte de") {
		t.Errorf("expected the status bar to show the no-completion-context diagnostic, got:\n%s", text)
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

// TestCompletionTrailingSlashIsIgnored covers the reported case: the
// trailing "/" before the parameters (or at the very end of the path) is
// optional in HTTP — "_cat/plugins/" must complete like "_cat/plugins",
// not fail for lack of an exact match (no known endpoint stores a
// trailing "/").
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
