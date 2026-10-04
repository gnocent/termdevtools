package ui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/config"
	"termdevtools/esclient"
	"termdevtools/i18n"
	"termdevtools/refdata"
)

// newTestApp starts a full App on a simulated screen (no real terminal or
// cluster required: esclient.New doesn't connect until a request is
// executed). Used to check, without a pty, the Tab completion popup's
// wiring, which no pure unit test can cover.
func newTestApp(t *testing.T) (*App, tcell.SimulationScreen) {
	t.Helper()
	return newTestAppLang(t, "")
}

// newTestAppLang is newTestApp with an explicit interface language ("fr" or
// "en"; "" is the tests' own default, see testAppOptions), to verify
// config.Config.Language is actually honored end-to-end (see
// TestInterfaceLanguageEnglish).
func newTestAppLang(t *testing.T, lang string) (*App, tcell.SimulationScreen) {
	t.Helper()
	return newTestAppWith(t, testAppOptions{lang: lang})
}

// testAppOptions customizes newTestAppWith; the zero value gives newTestApp's
// app: in French, unreachable cluster, detected as es95 (the language and
// the Elasticsearch version these tests' expectations were written against —
// the texts they look for and what completion offers depend on them).
type testAppOptions struct {
	// lang is the interface language; French when empty, whatever the
	// interface's own default is (TestInterfaceLanguageDefault checks that
	// one).
	lang string
	// defaultLang leaves the language unset instead, as in a configuration
	// that doesn't mention it.
	defaultLang bool
	// clusterURL points the app's client at a real (test) server instead of
	// an unreachable address.
	clusterURL string
	// target replaces es95 as the detected cluster; undetected leaves it
	// unknown instead (the zero Target).
	target     refdata.Target
	undetected bool
	warning    string
	// reference locates team/user reference files (none by default: only
	// what is built into the binary).
	reference refdata.Sources
	// What the editor may start with (see App.loadInitialQueries): the
	// built-in starter text, the content of a team's cheatsheet file, and
	// the content already saved for this cluster. All empty by default: the
	// editor starts empty.
	starter, cheatsheet, saved string
	// mouse enables mouse support, as "mouse: true" in config.yaml does.
	mouse bool
	// savedUnreadable puts something that can't be read as a file (a
	// directory) where this cluster's saved requests are expected.
	savedUnreadable bool
}

func newTestAppWith(t *testing.T, opts testAppOptions) (*App, tcell.SimulationScreen) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // sandbox config.Save() (F3 language toggle) away from the real user config

	url := opts.clusterURL
	if url == "" {
		url = "http://127.0.0.1:1"
	}
	client, err := esclient.New(esclient.Params{URL: url})
	if err != nil {
		t.Fatalf("esclient.New: %v", err)
	}
	target := opts.target
	if target == (refdata.Target{}) && !opts.undetected {
		target = es95
	}
	cr := ConnectResult{
		Client: client, Cluster: config.Cluster{URL: url}, DisplayUser: "test",
		Target: target, Warning: opts.warning,
	}
	lang := opts.lang
	if lang == "" && !opts.defaultLang {
		lang = i18n.FR
	}
	cfg := &config.Config{DefaultTimeoutSeconds: 5, Language: lang}

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(80, 24)

	tapp := tview.NewApplication().SetScreen(screen).EnableMouse(opts.mouse)
	paths := Paths{
		Cheatsheet: "/nonexistent/cheatsheet.txt",
		Starter:    opts.starter,
		Exports:    t.TempDir(),
		Reference:  opts.reference,
	}
	if opts.cheatsheet != "" {
		paths.Cheatsheet = filepath.Join(t.TempDir(), CheatsheetFileName)
		writeTestFile(t, paths.Cheatsheet, opts.cheatsheet)
	}
	if opts.saved != "" || opts.savedUnreadable {
		savedPath, err := config.QueriesPathForURL(url)
		if err != nil {
			t.Fatalf("QueriesPathForURL: %v", err)
		}
		if opts.savedUnreadable {
			if err := os.MkdirAll(savedPath, 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
		} else {
			writeTestFile(t, savedPath, opts.saved)
		}
	}
	app := NewApp(tapp, cr, cfg, paths)
	if opts.clusterURL == "" {
		// No cluster to ask: behave as if every _cat command's columns had
		// already been asked for and the built-in table used instead, so
		// column completion is immediate rather than waiting for a
		// connection attempt to fail (see App.fetchCatColumns; the live
		// path has its own tests, against a test server).
		for cmd, columns := range app.catColumns {
			app.catLive[cmd] = columns
		}
	}
	tapp.SetRoot(app.Root(), true)
	app.Start()

	// In front of the application's own key handling, installed by Start:
	// what lets waitForDraw know where the application is at.
	synchro := &testSync{tapp: tapp, reached: make(chan struct{}, 64)}
	tapp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == syncKey {
			synchro.reached <- struct{}{}
			return nil
		}
		return app.handleGlobalKeys(event)
	})
	testSyncs.Store(screen, synchro)
	t.Cleanup(func() { testSyncs.Delete(screen) })

	go func() {
		_ = tapp.Run()
	}()
	t.Cleanup(tapp.Stop)

	waitForDraw(t, screen)
	return app, screen
}

// writeTestFile creates path (and its directory) with content.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// syncKey is a key no part of the application uses: injected by waitForDraw
// behind the keys of a test, it tells when the application has gone through
// all of them (see testSync).
const syncKey = tcell.KeyF64

// testSync is what waitForDraw needs to synchronize with an application
// started by newTestAppWith: the application, and the channel on which its
// input capture reports each syncKey it receives.
type testSync struct {
	tapp    *tview.Application
	reached chan struct{}
}

// testSyncs holds the testSync of every running test application, by screen.
var testSyncs sync.Map

// waitForDraw waits until the application has processed every key injected
// so far and redrawn the screen, then a little longer.
//
// The first part is exact, for an application started by newTestAppWith:
// keys are processed in the order they were injected, so once syncKey —
// injected last — has been seen, the others have been dealt with. It
// replaces what used to be a blind pause alone, during which a loaded
// machine didn't always get through a long line typed on a narrow screen:
// the test then read a half-typed editor.
//
// The pause that remains gives what happens outside the key queue — a
// request answered in the background — the same time to land as before. A
// test that depends on such an answer should use eventually instead.
func waitForDraw(t *testing.T, screen tcell.SimulationScreen) {
	t.Helper()
	if registered, ok := testSyncs.Load(screen); ok {
		s := registered.(*testSync)
		screen.InjectKey(syncKey, 0, tcell.ModNone)
		select {
		case <-s.reached:
			// Seen by the input capture; the redraw that follows a key is
			// done by the time the application gets to this update.
			s.tapp.QueueUpdate(func() {})
		case <-time.After(10 * time.Second):
			t.Fatal("the application did not get through its pending keys in 10 seconds")
		}
	}
	time.Sleep(75 * time.Millisecond)
}

func injectText(screen tcell.SimulationScreen, s string) {
	for _, r := range s {
		screen.InjectKey(tcell.KeyRune, r, tcell.ModNone)
	}
}

// What the completion list holds, read from the application's own goroutine
// like everything a test reads of a running application (see uiValue).

func completionCount(app *App) int {
	return uiValue(app, app.completionList.GetItemCount)
}

func completionTitle(app *App) string {
	return uiValue(app, app.completionList.GetTitle)
}

// completionSelection is the text of the item currently selected.
func completionSelection(app *App) string {
	return uiValue(app, func() string {
		text, _ := app.completionList.GetItemText(app.completionList.GetCurrentItem())
		return text
	})
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

	got := editorText(app)
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
	injectText(screen, "{") // auto-closed to "{}", see ui/autoclose.go
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	waitForDraw(t, screen)

	want := "POST _search\n{}"
	if got := editorText(app); got != want {
		t.Errorf("expected F10 to be swallowed with no effect outside a completion context, got %q want %q", got, want)
	}
}

func TestCompletionSingleMatchAppliesInline(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/pl")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	got := editorText(app)
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

	got := editorText(app)
	want := "GET _cat/plugins?v"
	if got != want {
		t.Errorf("expected the trailing '/' to be ignored and replaced, got %q want %q", got, want)
	}
}

// TestCompletionClearsPriorErrorMessage checks that a stale "no completion"
// error (red, from an earlier Tab/F10 press with zero matches) doesn't stay
// displayed once a *later* completion attempt actually finds matches and
// opens the list.
func TestCompletionClearsPriorErrorMessage(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET zzzznomatch")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if !strings.Contains(screenText(screen), app.msgs.ErrNoCompletion) {
		t.Fatalf("test setup sanity check failed: expected the %q error after zero matches", app.msgs.ErrNoCompletion)
	}

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "GET _cat/s")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if completionCount(app) < 2 {
		t.Fatalf("expected multiple completion candidates, got %d", completionCount(app))
	}
	if strings.Contains(screenText(screen), app.msgs.ErrNoCompletion) {
		t.Error("expected the earlier no-completion error to be cleared once the list opens")
	}
}

func TestCompletionMultiMatchOpensListAndEnterApplies(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/s")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if completionCount(app) < 2 {
		t.Fatalf("expected multiple completion candidates, got %d", completionCount(app))
	}
	if app.tapp.GetFocus() != app.completionList {
		t.Fatal("expected focus to be on the completion list while it's open")
	}

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if app.tapp.GetFocus() != app.editor.Primitive() {
		t.Error("expected focus to return to the editor after selecting a completion")
	}

	got := editorText(app)
	if got == "GET _cat/s" {
		t.Error("expected the editor text to change after Enter, it did not")
	}
	t.Logf("editor text after completion: %q", got)
}

// TestCompletionTypeaheadJumpsToMatch checks that typing while the
// completion list is open narrows the selection to the first item starting
// with what was typed (case-insensitive) — useful to jump straight to an
// entry without scrolling through a long list (see typeaheadCompletion).
func TestCompletionTypeaheadJumpsToMatch(t *testing.T) {
	app, screen := newTestApp(t)

	// "_cat/s" (already typed) matches segments/shards/snapshots (sorted
	// alphabetically, all shown in the list): typing one more letter, "h",
	// should skip past "segments" straight to "shards".
	injectText(screen, "GET _cat/s")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if completionCount(app) < 3 {
		t.Fatalf("test setup sanity check failed: expected at least 3 candidates, got %d", completionCount(app))
	}

	injectText(screen, "h")
	waitForDraw(t, screen)

	if main := completionSelection(app); main != "_cat/shards?v" {
		t.Fatalf("expected typeahead 'sh' to select %q, got %q", "_cat/shards?v", main)
	}

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := editorText(app), "GET _cat/shards?v"; got != want {
		t.Errorf("expected typeahead selection to be applied on Enter, got %q want %q", got, want)
	}
}

// TestCompletionTypeaheadSurvivesPause checks that a pause between
// keystrokes never drops already-typed characters from the type-ahead
// buffer — a prior version reset the buffer after 700ms of inactivity,
// which silently dropped a character like "/" typed just before a pause,
// producing a search that looked timing-dependent/random to the user
// (typing "GET _cat" + Tab + "/" + a pause + "i" must search "_cat/i", not
// end up searching just "i").
func TestCompletionTypeaheadSurvivesPause(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	injectText(screen, "/")
	waitForDraw(t, screen)
	time.Sleep(900 * time.Millisecond) // longer than the old (now-removed) 700ms timeout
	injectText(screen, "i")
	waitForDraw(t, screen)

	if !strings.Contains(completionTitle(app), "[_cat/i]") {
		t.Fatalf("expected the search text to still be %q after the pause, title is %q", "_cat/i", completionTitle(app))
	}
	if main := completionSelection(app); main != "_cat/indices?v" {
		t.Errorf("expected the pause to leave the '/' in place and select %q, got %q", "_cat/indices?v", main)
	}
}

// TestCompletionTitleShowsSearchText checks that the completion list's
// title shows what's actually being searched for (the text already typed
// before Tab, plus type-ahead keystrokes since) — reported as confusing
// otherwise: typing "GET _cat" then Tab then "i" searches for "_cati", not
// "_cat/i" (every candidate has a "/" in between, e.g. "_cat/indices?v"),
// so without visible feedback the keystroke silently does nothing.
func TestCompletionTitleShowsSearchText(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if !strings.Contains(completionTitle(app), "[_cat]") {
		t.Fatalf("expected the completion list title to show the base search text %q, got %q", "_cat", completionTitle(app))
	}

	injectText(screen, "i")
	waitForDraw(t, screen)

	if !strings.Contains(completionTitle(app), "[_cati]") {
		t.Errorf("expected the completion list title to reflect the typed 'i', got %q", completionTitle(app))
	}
}

func TestCompletionEscapeCancelsWithoutChange(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/s")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got, want := editorText(app), "GET _cat/s"; got != want {
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
	injectText(screen, "{") // auto-closed to "{}", cursor lands between the two, see ui/autoclose.go
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)

	want := "POST _search\n{\t}"
	if got := editorText(app); got != want {
		t.Errorf("expected a literal tab inserted in JSON body context, got %q want %q", got, want)
	}
}
