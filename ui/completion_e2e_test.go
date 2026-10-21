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

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/esclient"
	"github.com/gnocent/termdevtools/i18n"
	"github.com/gnocent/termdevtools/refdata"
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
	// protection and label are the cluster's, as the connection form would
	// hand them over (none by default).
	protection, label string
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
		Client: client, Cluster: config.Cluster{URL: url, Protection: opts.protection, Label: opts.label}, DisplayUser: "test",
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

	startTestApplication(t, tapp, screen)

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
	done    chan struct{}
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

func TestCompletionPreservesQueryAfterCursor(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"format=json", "GET _cat/plugins?v&format=json"},
		{"v", "GET _cat/plugins?v"},
		{"v=false&format=json", "GET _cat/plugins?v=false&format=json"},
		{"format=json&%76=false", "GET _cat/plugins?format=json&%76=false"},
		{"tag=a%2Bb&tag=c+d", "GET _cat/plugins?v&tag=a%2Bb&tag=c+d"},
		{"", "GET _cat/plugins?v"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			app, screen := newTestApp(t)
			text := "GET _cat/pl?" + tc.query + "\nGET _cluster/health"
			uiValue(app, func() bool {
				app.editor.view.SetText(text, false)
				cursor := len("GET _cat/pl")
				app.editor.view.Select(cursor, cursor)
				return true
			})
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
			waitForDraw(t, screen)
			if got, want := editorText(app), tc.want+"\nGET _cluster/health"; got != want {
				t.Fatalf("completion must merge parameters without changing the next request: got %q, want %q", got, want)
			}
		})
	}
}

func TestCompletionTypeaheadPreservesQueryAfterCursor(t *testing.T) {
	app, screen := newTestApp(t)
	uiValue(app, func() bool {
		app.editor.view.SetText("GET _cat/s?format=json", false)
		cursor := len("GET _cat/s")
		app.editor.view.Select(cursor, cursor)
		return true
	})
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	injectText(screen, "h")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if got := editorText(app); got != "GET _cat/shards?v&format=json" {
		t.Fatalf("typeahead should search the path while preserving its query, got %q", got)
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

func TestCompletionLeadingSlashIsPreserved(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"GET /_cat/pl", "GET /_cat/plugins?v"},
		{"GET /_cat/plugins/", "GET /_cat/plugins?v"},
		{"GET /_cat/indices?h=heal", "GET /_cat/indices?h=health"},
		{"GET /_cat/shards?s=index:de", "GET /_cat/shards?s=index:desc"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			app, screen := newTestApp(t)
			injectText(screen, tc.input)
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
			waitForDraw(t, screen)
			if got := editorText(app); got != tc.want {
				t.Fatalf("completion with a leading slash: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCompletionLeadingSlashTypeahead(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET /_cat/s")
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	injectText(screen, "h")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if got := editorText(app); got != "GET /_cat/shards?v" {
		t.Fatalf("typeahead should preserve the leading slash, got %q", got)
	}
}

func TestCompletionLeadingSlashCustomEndpoints(t *testing.T) {
	for _, input := range []string{"GET /_review/st", "GET /"} {
		t.Run(input, func(t *testing.T) {
			userDir := t.TempDir()
			writeReferenceFile(t, userDir, "endpoints.txt", "/_review/status\n")
			app, screen := newTestAppWith(t, testAppOptions{reference: refdata.Sources{UserDir: userDir}})
			injectText(screen, input)
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
			if input == "GET /" {
				injectText(screen, "_review/st")
				screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
			}
			waitForDraw(t, screen)
			if got := editorText(app); got != "GET /_review/status" {
				t.Fatalf("slash-prefixed custom endpoints should still complete, got %q", got)
			}
		})
	}
}

func TestCompletionLeadingSlashDoesNotDuplicateBuiltin(t *testing.T) {
	userDir := t.TempDir()
	writeReferenceFile(t, userDir, "endpoints.txt", "/_cat/plugins?v\n")
	app, screen := newTestAppWith(t, testAppOptions{reference: refdata.Sources{UserDir: userDir}})
	injectText(screen, "GET /_cat/pl")
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if got := editorText(app); got != "GET /_cat/plugins?v" {
		t.Fatalf("equivalent candidates should complete inline, got %q", got)
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

// TestCompletionTypeaheadNoMatchCannotApplyStaleSelection refuses Enter alone
// after a failed search and allows correcting that search with Backspace.
func TestCompletionTypeaheadNoMatchCannotApplyStaleSelection(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET _cat/s")
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	injectText(screen, "hzz")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got := editorText(app); got != "GET _cat/s" {
		t.Fatalf("a search with no match applied a stale candidate: %q", got)
	}
	if app.tapp.GetFocus() != app.completionList {
		t.Fatal("the list should stay open so the search can be corrected")
	}
	if !strings.Contains(screenText(screen), app.msgs.ErrNoCompletion) {
		t.Fatal("a search with no match should report it")
	}

	// Removing the unmatched suffix restores the original matching search.
	screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if strings.Contains(screenText(screen), app.msgs.ErrNoCompletion) {
		t.Fatal("the no-match error should clear when a match is restored")
	}
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if got := editorText(app); got != "GET _cat/shards?v" {
		t.Fatalf("the corrected search should apply shards, got %q", got)
	}
}

func TestCompletionTypeaheadNoMatchCanBeCancelled(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET _cat/s")
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	injectText(screen, "zz")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if got := editorText(app); got != "GET _cat/s" {
		t.Fatalf("a failed search changed the editor: %q", got)
	}
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if app.tapp.GetFocus() != app.editor.Primitive() || editorText(app) != "GET _cat/s" {
		t.Fatal("Escape should return to the unchanged editor")
	}
}

func TestCompletionTypeaheadPreservesFallbackWarning(t *testing.T) {
	srv := newCatHelpServer(t, nil)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: srv.URL})
	injectText(screen, "GET _cat/indices?h=s")
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	eventually(t, "the fallback warning and column list", func() bool {
		return completionCount(app) > 1 && strings.Contains(statusText(app), app.msgs.WarnCatColumnsBuiltIn)
	})
	injectText(screen, "t")
	waitForDraw(t, screen)
	if !strings.Contains(statusText(app), app.msgs.WarnCatColumnsBuiltIn) {
		t.Fatal("valid typeahead must not clear the CAT fallback warning")
	}
}

func TestCompletionTypeaheadRecoveryPreservesNewerStatus(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET _cat/s")
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	injectText(screen, "hz")
	waitForDraw(t, screen)
	const newer = "a newer request result"
	onUI(app, func() { app.status.SetInfo(newer) })
	screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if !strings.Contains(statusText(app), newer) {
		t.Fatal("recovering from no match must not clear a newer status")
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

// TestCompletionCombinedRecovery preserves surrounding requests and user query
// values when a failed search is corrected or explicitly selected.
func TestCompletionCombinedRecovery(t *testing.T) {
	for _, target := range []struct {
		name      string
		target    refdata.Target
		mouseWant string
	}{
		{"Elasticsearch", es95, "# résumé\nGET /_cat/segments?format=json&v=false\nGET _cluster/health"},
		{"OpenSearch", refdata.Target{Distribution: refdata.OpenSearch, Version: refdata.Version{Major: 3, Minor: 9}, HasVersion: true}, "# résumé\nGET /_cat/segment_replication?format=json&v=false\nGET _cluster/health"},
	} {
		for _, mouse := range []bool{false, true} {
			name := target.name + "/keyboard"
			if mouse {
				name = target.name + "/mouse"
			}
			t.Run(name, func(t *testing.T) {
				app, screen := newTestAppWith(t, testAppOptions{target: target.target, mouse: mouse})
				const before = "# résumé\nGET /_cat/s?format=json&v=false\nGET _cluster/health"
				onUI(app, func() {
					app.editor.view.SetText(before, false)
					cursor := len("# résumé\nGET /_cat/s")
					app.editor.view.Select(cursor, cursor)
				})
				screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
				injectText(screen, "hzz")
				screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
				waitForDraw(t, screen)
				if got := editorText(app); got != before {
					t.Fatalf("an unmatched search changed the request: %q", got)
				}
				if !uiValue(app, func() bool {
					_, _, _, height := app.completionList.GetRect()
					return height > 0 && app.tapp.GetFocus() == app.completionList
				}) {
					t.Fatal("an unmatched search must leave the popup visible and focused")
				}
				if mouse {
					var x, y int
					onUI(app, func() { x, y, _, _ = app.completionList.GetInnerRect() })
					click(screen, x+1, y)
					waitForDraw(t, screen)
					if got := editorText(app); got != target.mouseWant {
						t.Fatalf("explicit mouse selection changed surrounding text: %q", got)
					}
					return
				}
				screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
				screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
				screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
				waitForDraw(t, screen)
				const want = "# résumé\nGET /_cat/shards?format=json&v=false\nGET _cluster/health"
				if got := editorText(app); got != want {
					t.Fatalf("recovery must preserve the slash, user query and surrounding text: got %q, want %q", got, want)
				}
			})
		}
	}
}

// TestCompletionNoMatchReset clears failed searches on correction or reopening.
func TestCompletionNoMatchReset(t *testing.T) {
	for _, reset := range []string{"backspace", "reopen"} {
		t.Run(reset, func(t *testing.T) {
			app, screen := newTestApp(t)
			injectText(screen, "GET /_cat/s")
			screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
			injectText(screen, "z")
			if reset == "backspace" {
				screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
			} else {
				screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
				screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
			}
			screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
			waitForDraw(t, screen)
			if got := editorText(app); got != "GET /_cat/segments?v" {
				t.Fatalf("reset must allow a normal selection, got %q", got)
			}
			if strings.Contains(statusText(app), app.msgs.ErrNoCompletion) {
				t.Fatal("reset left a stale no-match error")
			}
		})
	}
}

// TestCompletionCustomQueryOverridesRepeatedDefaults preserves repeated and
// escaped user parameters when defaults use equivalent query keys.
func TestCompletionCustomQueryOverridesRepeatedDefaults(t *testing.T) {
	userDir := t.TempDir()
	writeReferenceFile(t, userDir, "endpoints.txt", "/_custom/status?pretty=true&tag=a&tag=b&x%20y=default\n")
	app, screen := newTestAppWith(t, testAppOptions{reference: refdata.Sources{UserDir: userDir}})
	onUI(app, func() {
		app.editor.view.SetText("GET /_custom/st?pretty=false&tag=c%2Bd&tag=e+f&x+y=override", false)
		cursor := len("GET /_custom/st")
		app.editor.view.Select(cursor, cursor)
	})
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, screen)
	const want = "GET /_custom/status?pretty=false&tag=c%2Bd&tag=e+f&x+y=override"
	if got := editorText(app); got != want {
		t.Fatalf("user query must override every equivalent default without re-encoding values: got %q, want %q", got, want)
	}
}

// TestCatCompletionRecoveryPreservesOtherColumnsAndParameters changes only
// the selected CAT column after correcting a failed search.
func TestCatCompletionRecoveryPreservesOtherColumnsAndParameters(t *testing.T) {
	app, screen := newTestApp(t)
	const before = "GET /_cat/indices?h=health,s&format=json"
	onUI(app, func() {
		app.editor.view.SetText(before, false)
		cursor := len("GET /_cat/indices?h=health,s")
		app.editor.view.Select(cursor, cursor)
	})
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	injectText(screen, "tzz")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if got := editorText(app); got != before {
		t.Fatalf("an unmatched CAT column search changed the request: %q", got)
	}
	screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if got, want := editorText(app), "GET /_cat/indices?h=health,status&format=json"; got != want {
		t.Fatalf("column completion changed surrounding parameters: got %q, want %q", got, want)
	}
}

// TestCompletionNoMatchExplicitNavigation allows deliberate list navigation to
// override a failed typeahead search, while Enter alone remains protected.
func TestCompletionNoMatchExplicitNavigation(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tcell.Key
		want string
	}{
		{"down", tcell.KeyDown, "GET _cat/snapshots?v"},
		{"up", tcell.KeyUp, "GET _cat/segments?v"},
		{"left", tcell.KeyLeft, "GET _cat/shards?v"},
		{"right", tcell.KeyRight, "GET _cat/shards?v"},
		{"tab", tcell.KeyTab, "GET _cat/snapshots?v"},
		{"backtab", tcell.KeyBacktab, "GET _cat/segments?v"},
		{"page down", tcell.KeyPgDn, "GET _cat/snapshots?v"},
		{"page up", tcell.KeyPgUp, "GET _cat/segments?v"},
		{"home", tcell.KeyHome, "GET _cat/segments?v"},
		{"end", tcell.KeyEnd, "GET _cat/snapshots?v"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, screen := newTestApp(t)
			injectText(screen, "GET _cat/s")
			screen.InjectKey(tcell.KeyTab, 0, 0)
			injectText(screen, "hzz")
			screen.InjectKey(tc.key, 0, 0)
			waitForDraw(t, screen)
			if strings.Contains(statusText(app), app.msgs.ErrNoCompletion) {
				t.Error("explicit navigation kept failed-search error")
			}
			screen.InjectKey(tcell.KeyEnter, 0, 0)
			waitForDraw(t, screen)
			if got := editorText(app); got != tc.want {
				t.Fatalf("explicit selection: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCompletionNavigationPreservesNewerStatus prevents clearing an unrelated
// asynchronous result when a failed search is overridden explicitly.
func TestCompletionNavigationPreservesNewerStatus(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET _cat/s")
	screen.InjectKey(tcell.KeyTab, 0, 0)
	injectText(screen, "hz")
	waitForDraw(t, screen)
	onUI(app, func() { app.status.SetInfo("new request result") })
	screen.InjectKey(tcell.KeyDown, 0, 0)
	waitForDraw(t, screen)
	if !strings.Contains(statusText(app), "new request result") {
		t.Error("navigation cleared a newer result")
	}
	screen.InjectKey(tcell.KeyEnter, 0, 0)
	waitForDraw(t, screen)
	if got := editorText(app); got != "GET _cat/snapshots?v" {
		t.Fatalf("selection was still blocked: %q", got)
	}
}

// TestSlashPrefixedCustomCatColumns learns columns for CAT commands declared
// with a leading slash, using the cluster's actual help response.
func TestSlashPrefixedCustomCatColumns(t *testing.T) {
	dir := t.TempDir()
	writeReferenceFile(t, dir, "endpoints.txt", "/_cat/custom\n")
	srv := newCatHelpServer(t, map[string]string{"custom": "custom_column | cc | custom column\n"})
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: srv.URL, reference: refdata.Sources{UserDir: dir}})
	injectText(screen, "GET /_cat/custom?h=custom_")
	screen.InjectKey(tcell.KeyTab, 0, 0)
	eventually(t, "custom CAT column offered", func() bool { return editorText(app) == "GET /_cat/custom?h=custom_column" })
	if srv.requestCount() != 1 {
		t.Errorf("expected one help request, got %d", srv.requestCount())
	}
}

// TestCompletionNoMatchBorderClick keeps a non-item click from clearing a
// failed search or leaving an invisible list focused.
func TestCompletionNoMatchBorderClick(t *testing.T) {
	app, screen := newTestAppWith(t, testAppOptions{mouse: true})
	injectText(screen, "GET _cat/s")
	screen.InjectKey(tcell.KeyTab, 0, 0)
	injectText(screen, "hz")
	waitForDraw(t, screen)
	var x, y int
	onUI(app, func() { x, y, _, _ = app.completionList.GetRect() })
	click(screen, x, y)
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEnter, 0, 0)
	waitForDraw(t, screen)
	if got := editorText(app); got != "GET _cat/s" {
		t.Fatalf("border click applied a stale item: %q", got)
	}
	if !uiValue(app, func() bool { _, _, _, h := app.completionList.GetRect(); return h > 0 && app.completionList.HasFocus() }) {
		t.Error("border click hid the focused list")
	}
}
