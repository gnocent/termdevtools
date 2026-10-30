package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/gnocent/termdevtools/config"
	"github.com/gnocent/termdevtools/esclient"
	"github.com/gnocent/termdevtools/i18n"
)

// Tests of the open connections and of switching between them (SPEC.md
// §3.0): Sessions, driven from the connection screen as a user would.

// testCluster is a cluster for these tests. It answers the "GET /" that
// validates a connection, counting them, and any other request with its own
// name, so that a result says which cluster gave it. A path holding "slow"
// waits, as slowServer's do, until release is closed or the client goes
// away.
type testCluster struct {
	name  string
	url   string
	roots atomic.Int32
	// arrived receives "METHOD path" for each slow request, as it arrives;
	// outcome then receives "released" or "cancelled" for it.
	arrived     chan string
	outcome     chan string
	release     chan struct{}
	releaseOnce sync.Once
}

func newTestCluster(t *testing.T, name string) *testCluster {
	t.Helper()
	c := &testCluster{name: name, arrived: make(chan string, 8), outcome: make(chan string, 8), release: make(chan struct{})}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			c.roots.Add(1)
			_, _ = w.Write([]byte(`{"version": {"number": "9.5.4"}, "tagline": "You Know, for Search"}`))
		case strings.Contains(r.URL.Path, "slow"):
			c.arrived <- r.Method + " " + r.URL.Path
			select {
			case <-c.release:
				_, _ = w.Write([]byte(`{"slow":"` + name + `"}`))
				c.outcome <- "released"
			case <-r.Context().Done():
				c.outcome <- "cancelled"
			}
		default:
			_, _ = w.Write([]byte(`{"cluster":"` + name + `"}`))
		}
	}))
	t.Cleanup(srv.Close)
	// Registered after srv.Close, so run before it: Close waits for the
	// handlers still blocked.
	t.Cleanup(c.releaseAll)
	c.url = srv.URL
	return c
}

func (c *testCluster) releaseAll() {
	c.releaseOnce.Do(func() { close(c.release) })
}

func (c *testCluster) waitArrived(t *testing.T) {
	t.Helper()
	select {
	case <-c.arrived:
	case <-time.After(5 * time.Second):
		t.Fatalf("the slow request never reached %s", c.name)
	}
}

func (c *testCluster) waitOutcome(t *testing.T) string {
	t.Helper()
	select {
	case got := <-c.outcome:
		return got
	case <-time.After(5 * time.Second):
		t.Fatalf("the slow request on %s never ended", c.name)
		return ""
	}
}

// sessionsHarness runs Sessions on a simulated 80×24 screen, its
// config.yaml knowing the clusters it was given, in that order.
type sessionsHarness struct {
	s      *Sessions
	tapp   *tview.Application
	screen tcell.SimulationScreen
}

func newSessionsHarness(t *testing.T, clusters ...*testCluster) *sessionsHarness {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // config.yaml and the saved requests
	cfg := &config.Config{Language: i18n.FR, DefaultTimeoutSeconds: 5}
	for _, c := range clusters {
		cfg.Clusters = append(cfg.Clusters, config.Cluster{URL: c.url, AuthType: config.AuthNone})
	}

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(80, 24)

	tapp := tview.NewApplication().SetScreen(screen)
	s := NewSessions(tapp, cfg, Paths{Cheatsheet: "/nonexistent/cheatsheet.txt", Exports: t.TempDir()})
	tapp.SetRoot(s.Root(), true)
	startTestApplication(t, tapp, screen)
	waitForDraw(t, screen)
	return &sessionsHarness{s: s, tapp: tapp, screen: screen}
}

// key injects a key and waits for the interface to go through it.
func (h *sessionsHarness) key(t *testing.T, key tcell.Key) {
	t.Helper()
	mod := tcell.ModNone
	if key >= tcell.KeyCtrlA && key <= tcell.KeyCtrlZ {
		mod = tcell.ModCtrl
	}
	h.screen.InjectKey(key, 0, mod)
	waitForDraw(t, h.screen)
}

// current is the App on screen, or under the connection screen.
func (h *sessionsHarness) current() *App {
	var app *App
	onApplication(h.tapp, func() { app = h.s.current })
	return app
}

// switching tells whether the connection screen is shown in session.
func (h *sessionsHarness) switching() bool {
	var switching bool
	onApplication(h.tapp, func() { switching = h.s.switching })
	return switching
}

func (h *sessionsHarness) openCount() int {
	var n int
	onApplication(h.tapp, func() { n = len(h.s.apps) })
	return n
}

// running reports whether the application's event loop is still going.
func (h *sessionsHarness) running() bool {
	done := make(chan struct{})
	go func() {
		h.tapp.QueueUpdate(func() {})
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(300 * time.Millisecond):
		return false
	}
}

// choose selects the item at index in the connection screen's list and
// presses Enter on it.
func (h *sessionsHarness) choose(t *testing.T, index int) {
	t.Helper()
	h.key(t, tcell.KeyHome)
	for i := 0; i < index; i++ {
		h.key(t, tcell.KeyDown)
	}
	h.key(t, tcell.KeyEnter)
}

// indexOf is the index of url's line in the connection screen's list, -1 if
// it has none.
func (h *sessionsHarness) indexOf(url string) int {
	index := -1
	onApplication(h.tapp, func() {
		list := h.s.connect.list
		for i := 0; i < list.GetItemCount(); i++ {
			if main, _ := list.GetItemText(i); main == url {
				index = i
			}
		}
	})
	return index
}

// newConnectionIndex is the index of "New connection" in the list: right
// after the known clusters.
func (h *sessionsHarness) newConnectionIndex() int {
	var n int
	onApplication(h.tapp, func() { n = len(h.s.cfg.Clusters) })
	return n
}

// toConnectButton tabs through the form until the focus is on Connect.
func (h *sessionsHarness) toConnectButton(t *testing.T) {
	t.Helper()
	for i := 0; i < 12; i++ {
		var onConnect bool
		onApplication(h.tapp, func() {
			button, ok := h.tapp.GetFocus().(*tview.Button)
			onConnect = ok && button.GetLabel() == i18n.For(h.s.cfg.Language).ButtonConnect
		})
		if onConnect {
			return
		}
		h.key(t, tcell.KeyTab)
	}
	t.Fatalf("the Connect button never got the focus; screen:\n%s", screenText(h.screen))
}

// connectTo connects to the known cluster c from the connection screen's
// list and waits for its screen.
func (h *sessionsHarness) connectTo(t *testing.T, c *testCluster) *App {
	t.Helper()
	h.choose(t, h.indexOf(c.url))
	h.toConnectButton(t)
	h.key(t, tcell.KeyEnter)
	return h.waitFor(t, c)
}

// waitFor waits for c's screen to be the one on screen.
func (h *sessionsHarness) waitFor(t *testing.T, c *testCluster) *App {
	t.Helper()
	var app *App
	eventually(t, c.name+" on screen", func() bool {
		app = h.current()
		return app != nil && app.clusterURL == c.url && !h.switching()
	})
	return app
}

// typeNewConnection opens the form of a new connection from the list and
// presses Connect for url.
func (h *sessionsHarness) typeNewConnection(t *testing.T, url string) {
	t.Helper()
	h.choose(t, h.newConnectionIndex())
	injectText(h.screen, url) // the focus starts on the URL
	waitForDraw(t, h.screen)
	h.toConnectButton(t)
	h.key(t, tcell.KeyEnter)
}

func focusedPrimitive(h *sessionsHarness) tview.Primitive {
	var focused tview.Primitive
	onApplication(h.tapp, func() { focused = h.tapp.GetFocus() })
	return focused
}

// savedQueries reads what is saved of the left panel for c.
func savedQueries(t *testing.T, c *testCluster) string {
	t.Helper()
	path, err := config.QueriesPathForURL(c.url)
	if err != nil {
		t.Fatalf("QueriesPathForURL: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		return "<nothing saved: " + err.Error() + ">"
	}
	return string(saved)
}

// TestSessionsStartOnTheConnectionScreen checks that the program starts on
// the connection screen, from which Ctrl+C quits.
func TestSessionsStartOnTheConnectionScreen(t *testing.T) {
	a := newTestCluster(t, "alpha")
	h := newSessionsHarness(t, a)

	if text := screenText(h.screen); !strings.Contains(text, i18n.For(i18n.FR).ConnectListTitle) || !strings.Contains(text, a.url) {
		t.Fatalf("expected the list of known clusters, got:\n%s", text)
	}
	h.screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	eventually(t, "Ctrl+C to quit", func() bool { return !h.running() })
}

// TestSessionsConnectionShowsItsScreen checks that a connection opens its
// screen, its keys working there, and that Ctrl+C saves its left panel.
func TestSessionsConnectionShowsItsScreen(t *testing.T) {
	a := newTestCluster(t, "alpha")
	h := newSessionsHarness(t, a)
	app := h.connectTo(t, a)

	injectText(h.screen, "GET _alpha")
	eventually(t, "the typed request", func() bool { return editorText(app) == "GET _alpha" })
	h.key(t, tcell.KeyCtrlE)
	eventually(t, "the answer", func() bool { return strings.Contains(resultPlainText(app), `"cluster": "alpha"`) })

	h.screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	eventually(t, "Ctrl+C to quit", func() bool { return !h.running() })
	if got := savedQueries(t, a); got != "GET _alpha" {
		t.Errorf("expected the left panel to be saved on exit, got %q", got)
	}
}

// --- Switching (Ctrl+O) ----------------------------------------------------

// TestSwitchKeepsEachConnectionAsItWas checks the scenario of issue #10:
// work on A, switch to B, come back to A — its requests, cursor and result
// as they were left, without connecting to it again.
func TestSwitchKeepsEachConnectionAsItWas(t *testing.T) {
	a, b := newTestCluster(t, "alpha"), newTestCluster(t, "beta")
	h := newSessionsHarness(t, a, b)
	appA := h.connectTo(t, a)

	execute(t, appA, h.screen, "GET _alpha\nGET _second")
	eventually(t, "alpha's answer", func() bool { return strings.Contains(resultPlainText(appA), `"cluster": "alpha"`) })
	h.key(t, tcell.KeyUp)
	text, cursor := editorText(appA), uiValue(appA, appA.editor.CursorOffset)
	result := resultPlainText(appA)

	h.key(t, tcell.KeyCtrlO)
	if !h.switching() {
		t.Fatal("expected Ctrl+O to open the connection screen")
	}
	if got := savedQueries(t, a); got != text {
		t.Errorf("expected alpha's left panel to be saved on switching, got %q", got)
	}
	marker := i18n.For(i18n.FR).ConnectOpenMarker
	if screen := screenText(h.screen); !strings.Contains(screen, marker) {
		t.Errorf("expected alpha to be marked as open, got:\n%s", screen)
	}

	appB := h.connectTo(t, b)
	if appB == appA {
		t.Fatal("expected a screen of its own for beta")
	}
	if got := editorText(appB); got != "" {
		t.Errorf("expected beta's own (empty) left panel, got %q", got)
	}
	if header := headerText(appB); !strings.Contains(header, b.url) {
		t.Errorf("expected the header to name beta, got %q", header)
	}

	h.key(t, tcell.KeyCtrlO)
	h.choose(t, h.indexOf(a.url)) // open: no form
	if got := h.waitFor(t, a); got != appA {
		t.Fatal("expected alpha's own screen back")
	}
	if got := editorText(appA); got != text {
		t.Errorf("expected alpha's requests as they were, got %q", got)
	}
	if got := uiValue(appA, appA.editor.CursorOffset); got != cursor {
		t.Errorf("expected the cursor where it was (%d), got %d", cursor, got)
	}
	if got := resultPlainText(appA); got != result {
		t.Errorf("expected alpha's result as it was, got:\n%s", got)
	}
	if focused := focusedPrimitive(h); focused != appA.editor.Primitive() {
		t.Errorf("expected the focus back on the editor, got %T", focused)
	}
	if n := a.roots.Load(); n != 1 {
		t.Errorf("expected no new connection to alpha, it received %d GET /", n)
	}
	if n := h.openCount(); n != 2 {
		t.Errorf("expected 2 open connections, got %d", n)
	}
}

// TestSwitchSaysAFailedSaveAtOnce checks that a left panel that can't be
// saved on switching doesn't stop the switch, is said on the connection
// screen at once and on its own status bar, and stays as it was in memory.
func TestSwitchSaysAFailedSaveAtOnce(t *testing.T) {
	a := newTestCluster(t, "alpha")
	h := newSessionsHarness(t, a)
	appA := h.connectTo(t, a)
	injectText(h.screen, "GET _kept")
	eventually(t, "the typed request", func() bool { return editorText(appA) == "GET _kept" })
	// Nowhere to save: the save file's directory is a file.
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeTestFile(t, blocker, "in the way\n")
	onUI(appA, func() { appA.queriesPath = filepath.Join(blocker, "queries.txt") })

	h.key(t, tcell.KeyCtrlO)
	failure := formatPrefix(i18n.For(i18n.FR).ErrSaveFailedFmt)
	if !h.switching() {
		t.Fatal("expected the switch to go ahead")
	}
	if screen := screenText(h.screen); !strings.Contains(screen, failure) {
		t.Errorf("expected the connection screen to say the save failed, got:\n%s", screen)
	}

	h.key(t, tcell.KeyEscape)
	if status := statusText(appA); !strings.Contains(status, failure) {
		t.Errorf("expected alpha's status bar to say the save failed, got %q", status)
	}
	if got := editorText(appA); got != "GET _kept" {
		t.Errorf("expected alpha's requests as they were, got %q", got)
	}
}

// TestSwitchEscGoesBack checks that Esc on the list goes back to the cluster
// the screen was opened from, the focus on the panel that had it — and that
// Esc on a form only goes back to the list, as at startup.
func TestSwitchEscGoesBack(t *testing.T) {
	a := newTestCluster(t, "alpha")
	h := newSessionsHarness(t, a)
	appA := h.connectTo(t, a)
	onUI(appA, appA.focusResultPanel)

	h.key(t, tcell.KeyCtrlO)
	h.choose(t, h.newConnectionIndex())
	h.key(t, tcell.KeyEscape)
	if !h.switching() {
		t.Fatal("expected Esc on the form to go back to the list")
	}
	h.key(t, tcell.KeyEscape)
	if h.switching() || h.current() != appA {
		t.Fatal("expected Esc on the list to go back to alpha")
	}
	if focused := focusedPrimitive(h); focused != appA.result.Primitive() {
		t.Errorf("expected the focus back on the result panel, got %T", focused)
	}
}

// TestSwitchToAnOpenClusterTypedAgain checks that an open cluster's URL,
// typed as a new connection, goes back to it rather than opening a second
// connection: one per URL.
func TestSwitchToAnOpenClusterTypedAgain(t *testing.T) {
	a := newTestCluster(t, "alpha")
	h := newSessionsHarness(t, a)
	appA := h.connectTo(t, a)

	h.key(t, tcell.KeyCtrlO)
	h.typeNewConnection(t, a.url)
	if got := h.waitFor(t, a); got != appA {
		t.Fatal("expected alpha's own screen back")
	}
	if n := h.openCount(); n != 1 {
		t.Errorf("expected a single connection, got %d", n)
	}
	if n := a.roots.Load(); n != 1 {
		t.Errorf("expected no new connection to alpha, it received %d GET /", n)
	}
	// What the form held is not applied: said, since it was typed for that.
	if status := statusText(appA); !strings.Contains(status, appA.msgs.WarnAlreadyOpen) {
		t.Errorf("expected the status bar to say the open connection is kept as it was, got %q", status)
	}
}

// TestAttemptLeftBehindDoesNotTakeOver checks that a connection attempt
// still in progress when an open cluster is gone back to is given up:
// answering later, it would take the screen over — and could open a second
// connection to a cluster already open, the keys going to the one whose
// protection isn't the one shown.
func TestAttemptLeftBehindDoesNotTakeOver(t *testing.T) {
	a := newTestCluster(t, "alpha")
	hold := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(hold) }) }
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-hold
		_, _ = w.Write([]byte(`{"version": {"number": "9.5.4"}}`))
	}))
	t.Cleanup(slow.Close)
	t.Cleanup(release) // registered last: runs first, so that slow can close
	h := newSessionsHarness(t, a)
	appA := h.connectTo(t, a)

	h.key(t, tcell.KeyCtrlO)
	h.typeNewConnection(t, slow.URL) // no answer yet
	// alpha's URL typed in the same form instead, Connect pressed again.
	onApplication(h.tapp, func() {
		h.s.connect.attemptConnect(config.Cluster{URL: a.url, AuthType: config.AuthNone}, connectSecrets{})
	})
	if got := h.waitFor(t, a); got != appA {
		t.Fatal("expected alpha's own screen back")
	}

	release() // the slow cluster answers at last
	time.Sleep(300 * time.Millisecond)
	if h.current() != appA || h.switching() {
		t.Error("expected the attempt given up on not to take the screen over")
	}
	if n := h.openCount(); n != 1 {
		t.Errorf("expected a single connection, got %d", n)
	}
}

// TestSecondConnectionToAnOpenURLIsRefused checks one connection per URL
// where connections are opened, whichever way one gets there.
func TestSecondConnectionToAnOpenURLIsRefused(t *testing.T) {
	a := newTestCluster(t, "alpha")
	h := newSessionsHarness(t, a)
	appA := h.connectTo(t, a)

	client, err := esclient.New(esclient.Params{URL: a.url})
	if err != nil {
		t.Fatalf("esclient.New: %v", err)
	}
	onApplication(h.tapp, func() {
		h.s.connected(ConnectResult{Client: client, Cluster: config.Cluster{URL: a.url, Protection: config.ProtectionReadOnly}, Target: es95})
	})
	if n := h.openCount(); n != 1 {
		t.Errorf("expected a single connection to alpha, got %d", n)
	}
	if h.current() != appA {
		t.Error("expected alpha's open connection on screen")
	}
}

// TestFailedSwitchKeepsTheSession checks that a connection that fails leaves
// the one on screen as it was, to be gone back to.
func TestFailedSwitchKeepsTheSession(t *testing.T) {
	a := newTestCluster(t, "alpha")
	h := newSessionsHarness(t, a)
	appA := h.connectTo(t, a)
	injectText(h.screen, "GET _kept")
	waitForDraw(t, h.screen)

	h.key(t, tcell.KeyCtrlO)
	h.typeNewConnection(t, "http://127.0.0.1:1")
	failure := formatPrefix(i18n.For(i18n.FR).ErrConnectFailedFmt)
	eventually(t, "the failure", func() bool { return strings.Contains(screenText(h.screen), failure) })

	h.key(t, tcell.KeyEscape) // the form, back to the list
	h.key(t, tcell.KeyEscape) // the list, back to alpha
	if h.switching() || h.current() != appA {
		t.Fatal("expected to be back on alpha")
	}
	if got := editorText(appA); got != "GET _kept" {
		t.Errorf("expected alpha's requests as they were, got %q", got)
	}
	if n := h.openCount(); n != 1 {
		t.Errorf("expected a single connection, got %d", n)
	}
}

// TestSwitchWaitsForTheHelpToClose checks that Ctrl+O does nothing while a
// popup is open — here the help: like the other shortcuts, it acts on the
// panels.
func TestSwitchWaitsForTheHelpToClose(t *testing.T) {
	a := newTestCluster(t, "alpha")
	h := newSessionsHarness(t, a)
	appA := h.connectTo(t, a)

	h.key(t, tcell.KeyF1)
	h.key(t, tcell.KeyCtrlO)
	if h.switching() {
		t.Error("expected Ctrl+O to wait for the help to close")
	}
	if !helpVisible(appA) {
		t.Error("expected the help to stay open")
	}
}

// --- Requests off screen ---------------------------------------------------

// TestRequestCarriesOnOffScreen checks that a request in progress on A,
// read or write, isn't cancelled by switching to B: its answer lands in A's
// panel, not B's, and B's status bar says it came.
func TestRequestCarriesOnOffScreen(t *testing.T) {
	for _, method := range []string{"GET", "PUT"} {
		t.Run(method, func(t *testing.T) {
			a, b := newTestCluster(t, "alpha"), newTestCluster(t, "beta")
			h := newSessionsHarness(t, a, b)
			appA := h.connectTo(t, a)
			execute(t, appA, h.screen, method+" slow")
			a.waitArrived(t)

			h.key(t, tcell.KeyCtrlO)
			appB := h.connectTo(t, b)
			a.releaseAll()
			if got := a.waitOutcome(t); got != "released" {
				t.Fatalf("expected alpha's request to be left to finish, the server saw it %s", got)
			}
			notice := fmt.Sprintf(i18n.For(i18n.FR).InfoAnsweredElsewhereFmt, a.url)
			eventually(t, "the notice on beta", func() bool { return strings.Contains(statusText(appB), notice) })
			if got := resultPlainText(appB); strings.Contains(got, "slow") {
				t.Errorf("expected nothing of alpha's answer in beta's panel, got:\n%s", got)
			}

			h.key(t, tcell.KeyCtrlO)
			h.choose(t, h.indexOf(a.url))
			h.waitFor(t, a)
			if got := resultPlainText(appA); !strings.Contains(got, `"slow": "alpha"`) {
				t.Errorf("expected alpha's answer in its panel, got:\n%s", got)
			}
		})
	}
}

// TestEscOnlyAbandonsTheRequestOnScreen checks that Esc on B leaves A's
// request in progress alone.
func TestEscOnlyAbandonsTheRequestOnScreen(t *testing.T) {
	a, b := newTestCluster(t, "alpha"), newTestCluster(t, "beta")
	h := newSessionsHarness(t, a, b)
	appA := h.connectTo(t, a)
	execute(t, appA, h.screen, "GET slow")
	a.waitArrived(t)

	h.key(t, tcell.KeyCtrlO)
	h.connectTo(t, b)
	h.key(t, tcell.KeyEscape)
	if !requestInProgress(appA) {
		t.Error("expected alpha's request to be still in progress")
	}
	a.releaseAll()
	if got := a.waitOutcome(t); got != "released" {
		t.Errorf("expected alpha's request to be left to finish, the server saw it %s", got)
	}
}

// --- Exit and language -----------------------------------------------------

// openBoth connects to a then b, typing a request of its own into each, and
// leaves b on screen.
func openBoth(t *testing.T, h *sessionsHarness, a, b *testCluster) (appA, appB *App) {
	t.Helper()
	appA = h.connectTo(t, a)
	injectText(h.screen, "GET _alpha")
	waitForDraw(t, h.screen)
	h.key(t, tcell.KeyCtrlO)
	appB = h.connectTo(t, b)
	injectText(h.screen, "GET _beta")
	eventually(t, "beta's typed request", func() bool { return editorText(appB) == "GET _beta" })
	return appA, appB
}

// TestQuitGivesTheFocusBackToAnOpenPopup checks that a connection put on
// screen while a popup of its own is open — here the confirmation of a
// write, when its left panel can't be saved on Ctrl+C — gives the keys to
// that popup, not to the editor hidden under it.
func TestQuitGivesTheFocusBackToAnOpenPopup(t *testing.T) {
	a := newTestCluster(t, "alpha")
	h := newSessionsHarness(t, a)
	appA := h.connectTo(t, a)
	onUI(appA, func() { appA.protection = config.ProtectionConfirm })
	execute(t, appA, h.screen, "DELETE /precious")
	if !confirmVisible(appA) {
		t.Fatalf("expected the confirmation, got:\n%s", screenText(h.screen))
	}
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeTestFile(t, blocker, "in the way\n")
	onUI(appA, func() { appA.queriesPath = filepath.Join(blocker, "queries.txt") })

	h.screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	warning := formatPrefix(i18n.For(i18n.FR).ErrExitSaveFailedFmt)
	eventually(t, "the warning", func() bool { return strings.Contains(statusText(appA), warning) })
	var label string
	onApplication(h.tapp, func() {
		if button, ok := h.tapp.GetFocus().(*tview.Button); ok {
			label = button.GetLabel()
		}
	})
	if label != i18n.For(i18n.FR).ButtonCancel {
		t.Errorf("expected the focus on the confirmation's Cancel, got %T %q", focusedPrimitive(h), label)
	}
}

// TestCtrlCSavesEveryConnection checks that quitting saves the left panel of
// every open connection, not only the one on screen.
func TestCtrlCSavesEveryConnection(t *testing.T) {
	a, b := newTestCluster(t, "alpha"), newTestCluster(t, "beta")
	h := newSessionsHarness(t, a, b)
	appA, _ := openBoth(t, h, a, b)
	onUI(appA, func() { appA.editor.view.SetText("GET _alpha_changed", false) }) // since the switch saved it

	h.screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	eventually(t, "Ctrl+C to quit", func() bool { return !h.running() })
	if got := savedQueries(t, a); got != "GET _alpha_changed" {
		t.Errorf("expected alpha's left panel to be saved, got %q", got)
	}
	if got := savedQueries(t, b); got != "GET _beta" {
		t.Errorf("expected beta's left panel to be saved, got %q", got)
	}
}

// TestCtrlCShowsAConnectionThatCannotBeSaved checks that a connection off
// screen whose left panel can't be saved is put on screen with the reason,
// the first Ctrl+C staying, the second one quitting.
func TestCtrlCShowsAConnectionThatCannotBeSaved(t *testing.T) {
	a, b := newTestCluster(t, "alpha"), newTestCluster(t, "beta")
	h := newSessionsHarness(t, a, b)
	appA, _ := openBoth(t, h, a, b)
	// Nowhere to save alpha's: its save file's directory is a file.
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeTestFile(t, blocker, "in the way\n")
	onUI(appA, func() { appA.queriesPath = filepath.Join(blocker, "queries.txt") })

	h.screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	warning := formatPrefix(i18n.For(i18n.FR).ErrExitSaveFailedFmt)
	eventually(t, "alpha on screen with the warning", func() bool {
		return h.current() == appA && strings.Contains(statusText(appA), warning)
	})
	if !h.running() {
		t.Fatal("expected the application to stay open after the first Ctrl+C")
	}

	h.screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	eventually(t, "the second Ctrl+C to quit", func() bool { return !h.running() })
	if got := savedQueries(t, b); got != "GET _beta" {
		t.Errorf("expected beta's left panel to be saved all the same, got %q", got)
	}
}

// TestQuitInSessionSavesEveryConnection checks that the connection screen's
// Quit, opened in session, saves as Ctrl+C does.
func TestQuitInSessionSavesEveryConnection(t *testing.T) {
	a, b := newTestCluster(t, "alpha"), newTestCluster(t, "beta")
	h := newSessionsHarness(t, a, b)
	openBoth(t, h, a, b)

	h.key(t, tcell.KeyCtrlO)
	h.screen.InjectKey(tcell.KeyEnd, 0, tcell.ModNone) // Quit, the list's last line
	h.screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	eventually(t, "Quit to quit", func() bool { return !h.running() })
	if got := savedQueries(t, a); got != "GET _alpha" {
		t.Errorf("expected alpha's left panel to be saved, got %q", got)
	}
	if got := savedQueries(t, b); got != "GET _beta" {
		t.Errorf("expected beta's left panel to be saved, got %q", got)
	}
}

// TestSaveOnSignalSavesEveryConnection checks the save done when the program
// is terminated from outside: every open connection's left panel.
func TestSaveOnSignalSavesEveryConnection(t *testing.T) {
	a, b := newTestCluster(t, "alpha"), newTestCluster(t, "beta")
	h := newSessionsHarness(t, a, b)
	appA, _ := openBoth(t, h, a, b)
	onUI(appA, func() { appA.editor.view.SetText("GET _alpha_changed", false) })

	h.s.SaveOnSignal() // from the test's goroutine, as from the signal handler's
	if got := savedQueries(t, a); got != "GET _alpha_changed" {
		t.Errorf("expected alpha's left panel to be saved, got %q", got)
	}
	if got := savedQueries(t, b); got != "GET _beta" {
		t.Errorf("expected beta's left panel to be saved, got %q", got)
	}
}

// TestLanguageFollowsToTheOtherConnections checks that the language switched
// (F3) on one connection is that of the connection screen and of the other
// connections when they come back on screen.
func TestLanguageFollowsToTheOtherConnections(t *testing.T) {
	a, b := newTestCluster(t, "alpha"), newTestCluster(t, "beta")
	h := newSessionsHarness(t, a, b)
	_, appB := openBoth(t, h, a, b)

	h.key(t, tcell.KeyCtrlO)
	h.choose(t, h.indexOf(a.url))
	h.waitFor(t, a)
	h.key(t, tcell.KeyF3) // French to English

	h.key(t, tcell.KeyCtrlO)
	en := i18n.For(i18n.EN)
	if screen := screenText(h.screen); !strings.Contains(screen, strings.TrimSpace(en.ConnectListTitle)) {
		t.Errorf("expected the connection screen in English, got:\n%s", screen)
	}
	h.choose(t, h.indexOf(b.url))
	h.waitFor(t, b)
	if msgs := uiValue(appB, func() *i18n.Strings { return appB.msgs }); msgs != en {
		t.Error("expected beta in English too")
	}
}
