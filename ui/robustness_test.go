package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/config"
	"termdevtools/i18n"
)

// Tests of defects found by the code review of October 2026: each sets up
// the situation in which one of them showed, and checks what should happen
// instead.

// appRunning reports whether the application's event loop is still going —
// what Ctrl+C is supposed to end.
func appRunning(app *App) bool {
	done := make(chan struct{})
	go func() {
		app.tapp.QueueUpdate(func() {})
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(300 * time.Millisecond):
		return false
	}
}

// --- Editor ---------------------------------------------------------------

// TestAutoCloseLeavesBracketsInsideStringsAlone checks that "{" and "["
// typed as the content of a string aren't given a closer: a query string
// such as "age:[10 TO *}" came out with a stray "]".
func TestAutoCloseLeavesBracketsInsideStringsAlone(t *testing.T) {
	e := NewEditor(i18n.For(i18n.FR))
	e.view.SetText("", true)

	const typed = `"age:[10 TO *}"`
	injectRunes(e, typed)
	if got := e.Text(); got != typed {
		t.Errorf("expected the string to come out as typed (%q), got %q", typed, got)
	}

	// Outside a string, brackets are still paired.
	e.view.SetText("", true)
	injectRunes(e, `{"a":[`)
	if got := e.Text(); got != `{"a":[]}` {
		t.Errorf("expected the structure to be closed (%q), got %q", `{"a":[]}`, got)
	}
}

// TestUndoAfterCompletionKeepsTheCompletion checks that the first character
// typed after a completion is an undo step of its own: undoing it used to
// take the completion away with it.
func TestUndoAfterCompletionKeepsTheCompletion(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/pl")
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	eventually(t, "the completion", func() bool { return editorText(app) == "GET _cat/plugins?v" })

	injectText(screen, "x")
	eventually(t, "the keystroke", func() bool { return editorText(app) == "GET _cat/plugins?vx" })
	screen.InjectKey(tcell.KeyCtrlZ, 0, tcell.ModNone)
	eventually(t, "undo to remove the keystroke only", func() bool { return editorText(app) == "GET _cat/plugins?v" })
}

// TestF4SaysWhyItDoesNothing checks F4 on a body that holds a "#" line:
// the parser skips it, so the body is valid, but it can't be re-indented —
// which used to leave F4 without any effect or message.
func TestF4SaysWhyItDoesNothing(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "POST _search")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "# all of them")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, `{"query":{"match_all":{}}}`)
	const typed = "POST _search\n# all of them\n{\"query\":{\"match_all\":{}}}"
	eventually(t, "the typed request", func() bool { return editorText(app) == typed })

	screen.InjectKey(tcell.KeyF4, 0, tcell.ModNone)
	eventually(t, "the explanation", func() bool {
		return strings.Contains(statusText(app), app.msgs.ErrFormatCommentsInBody)
	})
	if got := editorText(app); got != typed {
		t.Errorf("expected the request to be left as it was, got %q", got)
	}
}

// TestF4OnAnIndentedBodyStaysQuiet checks the other reason F4 changes
// nothing — a body already indented — which is no error.
func TestF4OnAnIndentedBodyStaysQuiet(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "POST _search")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, `{"size":1}`)
	screen.InjectKey(tcell.KeyF4, 0, tcell.ModNone)
	const indented = "POST _search\n{\n  \"size\": 1\n}"
	eventually(t, "the body to be indented", func() bool { return editorText(app) == indented })

	screen.InjectKey(tcell.KeyF4, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if status := statusText(app); strings.Contains(status, app.msgs.ErrFormatCommentsInBody) {
		t.Errorf("expected no complaint about an already indented body, got %q", status)
	}
}

// TestCompletionTitleShowsLettersOnlySearchText checks the title of the
// completion list when the text searched for is made of letters only, as a
// _cat column's start is: written between square brackets, it was taken for
// a style tag and not displayed at all.
func TestCompletionTitleShowsLettersOnlySearchText(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET _cat/indices?h=st")
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	eventually(t, "the completion list", func() bool {
		var open bool
		app.tapp.QueueUpdate(func() { open = app.completionList.HasFocus() && app.completionList.GetItemCount() > 1 })
		return open
	})
	eventually(t, "the search text in the title", func() bool { return strings.Contains(screenText(screen), "[st]") })
}

// --- Saving ---------------------------------------------------------------

// TestCtrlCWarnsWhenTheSaveFails checks that quitting doesn't silently lose
// the left panel when it can't be saved: the first Ctrl+C says so and stays,
// the second one quits.
func TestCtrlCWarnsWhenTheSaveFails(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET _precious")
	eventually(t, "the typed request", func() bool { return editorText(app) == "GET _precious" })

	// Nowhere to save: the save file's directory is a file.
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeTestFile(t, blocker, "in the way\n")
	app.tapp.QueueUpdate(func() { app.queriesPath = filepath.Join(blocker, "queries.txt") })

	screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	// The message's own start, before the reason it is given.
	warning := strings.SplitN(app.msgs.ErrExitSaveFailedFmt, "%", 2)[0]
	eventually(t, "the warning", func() bool { return strings.Contains(statusText(app), warning) })
	if !appRunning(app) {
		t.Fatal("expected the application to stay open after the first Ctrl+C")
	}

	screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	eventually(t, "the second Ctrl+C to quit", func() bool { return !appRunning(app) })
}

// TestUnreadSavedRequestsAreNotOverwritten checks what happens when the
// saved requests exist but couldn't be read at startup. The editor doesn't
// hold them: quitting must not write it over them.
func TestUnreadSavedRequestsAreNotOverwritten(t *testing.T) {
	t.Run("the failure is remembered", func(t *testing.T) {
		app, _ := newTestAppWith(t, testAppOptions{savedUnreadable: true})
		var failed bool
		app.tapp.QueueUpdate(func() { failed = app.queriesLoadFailed })
		if !failed {
			t.Error("expected the failed load to be remembered")
		}
		if status := statusText(app); !strings.Contains(status, filepath.Base(app.queriesPath)) {
			t.Errorf("expected the status bar to report the file that couldn't be read, got %q", status)
		}
	})

	t.Run("an empty editor quits without saving", func(t *testing.T) {
		app, screen := newTestAppWith(t, testAppOptions{saved: "GET _precious\n"})
		app.tapp.QueueUpdate(func() {
			// As if reading the file had failed: nothing loaded.
			app.queriesLoadFailed = true
			app.editor.SetInitialText("")
		})

		screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
		eventually(t, "Ctrl+C to quit", func() bool { return !appRunning(app) })
		if saved, err := os.ReadFile(app.queriesPath); err != nil || string(saved) != "GET _precious\n" {
			t.Errorf("expected the saved requests to be left alone, got %q (%v)", saved, err)
		}
	})

	t.Run("new content is not lost in silence either", func(t *testing.T) {
		app, screen := newTestAppWith(t, testAppOptions{saved: "GET _precious\n"})
		app.tapp.QueueUpdate(func() {
			app.queriesLoadFailed = true
			app.editor.SetInitialText("")
		})
		injectText(screen, "GET _new")
		eventually(t, "the typed request", func() bool { return editorText(app) == "GET _new" })

		screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
		eventually(t, "the warning", func() bool { return strings.Contains(statusText(app), app.msgs.ErrSavedRequestsUnread) })
		if saved, _ := os.ReadFile(app.queriesPath); string(saved) != "GET _precious\n" {
			t.Errorf("expected the saved requests to be left alone, got %q", saved)
		}

		// Ctrl+S is the user's decision to replace them; quitting then saves
		// as usual.
		screen.InjectKey(tcell.KeyCtrlS, 0, tcell.ModNone)
		eventually(t, "the explicit save", func() bool {
			saved, _ := os.ReadFile(app.queriesPath)
			return string(saved) == "GET _new"
		})
		var failed bool
		app.tapp.QueueUpdate(func() { failed = app.queriesLoadFailed })
		if failed {
			t.Error("expected an explicit save to clear the failed-load state")
		}
	})
}

// TestSaveOnSignalGoesThroughTheInterface checks the save done when the
// program is terminated from outside: asked from another goroutine, carried
// out by the interface's own.
func TestSaveOnSignalGoesThroughTheInterface(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET _cluster/health")
	eventually(t, "the typed request", func() bool { return editorText(app) == "GET _cluster/health" })

	app.SaveQueriesOnSignal() // from the test's goroutine, as from the signal handler's
	if saved, err := os.ReadFile(app.queriesPath); err != nil || string(saved) != "GET _cluster/health" {
		t.Errorf("expected the left panel to be saved, got %q (%v)", saved, err)
	}
}

// --- Requests -------------------------------------------------------------

// TestLateAnswerDoesNotReplaceANewerResult checks that when two requests
// are in flight, the result displayed is that of the last one sent — not
// whichever answers last.
func TestLateAnswerDoesNotReplaceANewerResult(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	slowDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-release
			_, _ = w.Write([]byte(`{"answer":"slow"}`))
			close(slowDone)
			return
		}
		_, _ = w.Write([]byte(`{"answer":"fast"}`))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: srv.URL})

	resultText := func() string {
		var text string
		app.tapp.QueueUpdate(func() { text = app.result.PlainText() })
		return text
	}

	injectText(screen, "GET slow")
	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, "GET fast")
	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	eventually(t, "the fast answer", func() bool { return strings.Contains(resultText(), `"answer": "fast"`) })

	releaseOnce.Do(func() { close(release) })
	select {
	case <-slowDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the slow request never completed")
	}
	// Time for the slow answer to reach the interface, were it to be shown.
	time.Sleep(200 * time.Millisecond)
	if got := resultText(); !strings.Contains(got, "# GET fast") || strings.Contains(got, "slow") {
		t.Errorf("expected the result of the last request sent to stay displayed, got:\n%s", got)
	}
}

// --- Mouse ----------------------------------------------------------------

// TestMouseClickMovesThePanelFocus checks that with mouse support on, the
// shortcuts that depend on the focused panel follow a click: Ctrl+S exports
// from the result panel, saves from the editor.
func TestMouseClickMovesThePanelFocus(t *testing.T) {
	app, screen := newTestAppWith(t, testAppOptions{mouse: true})

	click(screen, 60, 5) // in the result panel
	screen.InjectKey(tcell.KeyCtrlS, 0, tcell.ModNone)
	eventually(t, "Ctrl+S to act on the result panel", func() bool {
		return strings.Contains(statusText(app), app.msgs.ErrNothingToExport)
	})
	if _, err := os.Stat(app.queriesPath); !os.IsNotExist(err) {
		t.Errorf("expected nothing to be saved from the result panel (stat: %v)", err)
	}

	click(screen, 10, 5) // back in the editor
	screen.InjectKey(tcell.KeyCtrlS, 0, tcell.ModNone)
	eventually(t, "Ctrl+S to save the editor", func() bool {
		_, err := os.Stat(app.queriesPath)
		return err == nil
	})
}

// TestCompletionListClosesWhenClickedAway checks that the completion list
// doesn't outlive the focus: left open after a click elsewhere, a later
// click on one of its items would apply to a text that has changed since.
func TestCompletionListClosesWhenClickedAway(t *testing.T) {
	app, screen := newTestAppWith(t, testAppOptions{mouse: true})

	injectText(screen, "GET _cat/s")
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	listHeight := func() int {
		var height int
		app.tapp.QueueUpdateDraw(func() {})
		app.tapp.QueueUpdate(func() { _, _, _, height = app.completionList.GetRect() })
		return height
	}
	eventually(t, "the completion list", func() bool { return listHeight() > 0 })

	click(screen, 60, 5) // in the result panel
	eventually(t, "the completion list to close", func() bool { return listHeight() == 0 })
	if got := editorText(app); got != "GET _cat/s" {
		t.Errorf("expected the editor to be left as typed, got %q", got)
	}
}

// TestHelpTakesEveryClick checks that a click beside the help popup doesn't
// give the focus to the panel behind it.
func TestHelpTakesEveryClick(t *testing.T) {
	app, screen := newTestAppWith(t, testAppOptions{mouse: true})

	screen.InjectKey(tcell.KeyF1, 0, tcell.ModNone)
	eventually(t, "the help popup", func() bool {
		var open bool
		app.tapp.QueueUpdate(func() { open = app.helpVisible })
		return open
	})

	click(screen, 0, 1) // on the editor's border, showing beside the popup
	injectText(screen, "x")
	waitForDraw(t, screen)
	if got := editorText(app); got != "" {
		t.Errorf("expected nothing to reach the editor behind the help popup, got %q", got)
	}

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	eventually(t, "Esc to close the help popup", func() bool {
		var open bool
		app.tapp.QueueUpdate(func() { open = app.helpVisible })
		return !open
	})
}

// --- Connection -----------------------------------------------------------

// connectHarness is a connection screen over two test clusters, the first
// of which only answers once released.
type connectHarness struct {
	screen  tcell.SimulationScreen
	results chan ConnectResult
	slowURL string
	fastURL string
	release func()
}

func newConnectHarness(t *testing.T) *connectHarness {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	hold := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(hold) }) }
	answer := func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"version": {"number": "9.5.4"}, "tagline": "You Know, for Search"}`))
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-hold
		answer(w)
	}))
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { answer(w) }))
	t.Cleanup(slow.Close)
	t.Cleanup(fast.Close)
	t.Cleanup(release) // registered last: runs first, so that slow can close

	cfg := &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5, Clusters: []config.Cluster{
		{URL: slow.URL, AuthType: config.AuthNone},
		{URL: fast.URL, AuthType: config.AuthNone},
	}}
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(100, 30)

	results := make(chan ConnectResult, 4)
	tapp := tview.NewApplication().SetScreen(screen)
	tapp.SetRoot(BuildConnectPage(tapp, cfg, func(cr ConnectResult) { results <- cr }), true)
	go func() { _ = tapp.Run() }()
	t.Cleanup(tapp.Stop)
	waitForDraw(t, screen)

	return &connectHarness{screen: screen, results: results, slowURL: slow.URL, fastURL: fast.URL, release: release}
}

// connect confirms the form of the cluster selected in the list: Enter
// opens it, Tab reaches the Connect button, Enter presses it — presses times.
func (h *connectHarness) connect(t *testing.T, presses int) {
	t.Helper()
	h.screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, h.screen)
	h.screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForDraw(t, h.screen)
	for i := 0; i < presses; i++ {
		h.screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	}
	waitForDraw(t, h.screen)
}

// TestAbandonedConnectionDoesNotTakeOver checks that a cluster given up on
// while it was slow to answer doesn't, when it finally does, replace the
// session opened on another one since.
func TestAbandonedConnectionDoesNotTakeOver(t *testing.T) {
	h := newConnectHarness(t)

	h.connect(t, 1)                                       // the slow cluster: no answer yet
	h.screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone) // back to the list
	waitForDraw(t, h.screen)
	h.screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone) // the fast one
	waitForDraw(t, h.screen)
	h.connect(t, 1)

	select {
	case cr := <-h.results:
		if cr.Cluster.URL != h.fastURL {
			t.Fatalf("expected to be connected to %s, got %s", h.fastURL, cr.Cluster.URL)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no connection; screen:\n%s", screenText(h.screen))
	}

	h.release() // the slow cluster answers at last
	select {
	case cr := <-h.results:
		t.Errorf("expected the abandoned attempt to be ignored, it connected to %s", cr.Cluster.URL)
	case <-time.After(500 * time.Millisecond):
	}
}

// TestCancelButtonGivesUpTheAttempt is TestAbandonedConnectionDoesNotTakeOver
// for the other way out of the form: its Cancel button.
func TestCancelButtonGivesUpTheAttempt(t *testing.T) {
	h := newConnectHarness(t)

	h.connect(t, 1)                                    // the slow cluster: no answer yet, focus on Connect
	h.screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // to the Cancel button
	waitForDraw(t, h.screen)
	h.screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitForDraw(t, h.screen)

	h.release() // the slow cluster answers at last
	select {
	case cr := <-h.results:
		t.Errorf("expected the cancelled attempt to be ignored, it connected to %s", cr.Cluster.URL)
	case <-time.After(500 * time.Millisecond):
	}
}

// connectOnce opens the connection screen on a single known cluster and
// presses Connect; it returns the screen and the channel a connection would
// be reported on.
func connectOnce(t *testing.T, clusterURL string) (tcell.SimulationScreen, chan ConnectResult) {
	t.Helper()
	return connectOnceTo(t, config.Cluster{URL: clusterURL, AuthType: config.AuthNone})
}

// connectOnceTo is connectOnce for a cluster whose entry in config.yaml
// holds more than its URL.
func connectOnceTo(t *testing.T, cluster config.Cluster) (tcell.SimulationScreen, chan ConnectResult) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5, Clusters: []config.Cluster{cluster}}

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(160, 30) // wide: the message under the form is one line

	results := make(chan ConnectResult, 1)
	tapp := tview.NewApplication().SetScreen(screen)
	tapp.SetRoot(BuildConnectPage(tapp, cfg, func(cr ConnectResult) { results <- cr }), true)
	go func() { _ = tapp.Run() }()
	t.Cleanup(tapp.Stop)
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // the cluster, first in the list
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone) // from the authentication dropdown to Connect
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	return screen, results
}

// TestCredentialsInTheURLAreRefused checks that a URL carrying a password is
// turned down before anything is done with it: the URL is what gets saved in
// config.yaml and shown in the status bar, where no secret may ever be.
func TestCredentialsInTheURLAreRefused(t *testing.T) {
	var reached bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	defer srv.Close()
	withPassword := strings.Replace(srv.URL, "http://", "http://admin:hunter2@", 1)

	screen, results := connectOnce(t, withPassword)
	refusal := i18n.For(i18n.FR).ErrURLCredentials[:30]
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(screenText(screen), refusal) {
		if time.Now().After(deadline) {
			t.Fatalf("expected the refusal to be displayed, got:\n%s", screenText(screen))
		}
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case <-results:
		t.Error("expected no connection")
	case <-time.After(300 * time.Millisecond):
	}
	if reached {
		t.Error("expected nothing to be sent to the cluster")
	}
	if path, err := config.Path(); err == nil {
		if saved, _ := os.ReadFile(path); strings.Contains(string(saved), "hunter2") {
			t.Errorf("the password was written to config.yaml:\n%s", saved)
		}
	}
}

// TestRedirectionIsNotAConnection checks that a cluster address answering
// with a redirection is reported as such, with where it points: what is
// behind it isn't the address the user gave, and requests wouldn't follow.
func TestRedirectionIsNotAConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://login.example.com/sso", http.StatusFound)
	}))
	defer srv.Close()

	screen, results := connectOnce(t, srv.URL)
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(screenText(screen), "login.example.com") {
		if time.Now().After(deadline) {
			t.Fatalf("expected the redirection and its target to be displayed, got:\n%s", screenText(screen))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if text := screenText(screen); !strings.Contains(text, "302") {
		t.Errorf("expected the HTTP status to be displayed, got:\n%s", text)
	}
	select {
	case <-results:
		t.Error("expected no connection")
	case <-time.After(300 * time.Millisecond):
	}
}

// TestConnectPressedTwiceConnectsOnce checks that a second press on Connect
// while the first attempt is in progress doesn't open two sessions.
func TestConnectPressedTwiceConnectsOnce(t *testing.T) {
	h := newConnectHarness(t)

	h.connect(t, 2) // the slow cluster, Connect pressed twice
	h.release()

	select {
	case cr := <-h.results:
		if cr.Cluster.URL != h.slowURL {
			t.Fatalf("expected to be connected to %s, got %s", h.slowURL, cr.Cluster.URL)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no connection; screen:\n%s", screenText(h.screen))
	}
	select {
	case <-h.results:
		t.Error("expected a single session, a second one was opened")
	case <-time.After(500 * time.Millisecond):
	}
}
