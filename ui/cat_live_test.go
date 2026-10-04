package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"termdevtools/refdata"
)

// catHelpServer is a test cluster answering "GET _cat/<command>?help" from
// help (command -> response body); a command not in it gets a 500.
type catHelpServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	// hold, when set, blocks every answer until release is called.
	hold        chan struct{}
	releaseOnce sync.Once
}

// release lets the answers held back go through. Also called when the test
// ends, whatever its outcome: the server can't shut down with a handler
// still blocked.
func (s *catHelpServer) release() {
	s.releaseOnce.Do(func() {
		if s.hold != nil {
			close(s.hold)
		}
	})
}

func newCatHelpServer(t *testing.T, help map[string]string) *catHelpServer {
	t.Helper()
	s := &catHelpServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.URL.RequestURI())
		hold := s.hold
		s.mu.Unlock()
		if hold != nil {
			<-hold
		}

		command := strings.TrimPrefix(r.URL.Path, "/_cat/")
		body, known := help[command]
		if !known || !r.URL.Query().Has("help") {
			http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	t.Cleanup(s.release) // registered last: runs first
	return s
}

func (s *catHelpServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// onUI runs f on the application's own goroutine — the only place its
// widgets may be read from — and reports whether it ran. tview's QueueUpdate
// alone waits forever once the application has stopped: a test whose
// application quit when it shouldn't have would hang instead of failing.
func onUI(app *App, f func()) bool {
	done := make(chan struct{})
	go func() {
		app.tapp.QueueUpdate(f)
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(3 * time.Second):
		return false
	}
}

// uiValue returns what f evaluates to on the application's own goroutine:
// how a test reads a widget or a field of the running application. The zero
// value if the application has stopped.
func uiValue[T any](app *App, f func() T) T {
	var value T
	onUI(app, func() { value = f() })
	return value
}

// stoppedText is what the helpers reading the interface return when the
// application is no longer there to answer.
const stoppedText = "<the application has stopped>"

// editorText reads the editor's content from the application's own
// goroutine.
func editorText(app *App) string {
	text := stoppedText
	onUI(app, func() { text = app.editor.Text() })
	return text
}

// eventually polls cond until it holds, failing the test after a few
// seconds — for outcomes that depend on a background request.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

// A "?help" body in the format real clusters use, with a column the
// built-in table doesn't have.
const indicesHelp = `health      | h   | current health status
status      | s   | open/close status
zz.future   | zzf | a column this binary has never heard of
`

// TestCatColumnsAskedFromCluster checks that the columns offered are the
// ones the cluster reports, not the built-in table's: here one that only
// the cluster knows.
func TestCatColumnsAskedFromCluster(t *testing.T) {
	srv := newCatHelpServer(t, map[string]string{"indices": indicesHelp})
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: srv.URL})

	injectText(screen, "GET _cat/indices?h=zz")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)

	eventually(t, "the cluster-only column to complete", func() bool {
		return editorText(app) == "GET _cat/indices?h=zz.future"
	})
}

// TestCatColumnsAskedOncePerCommand checks that the cluster is asked once
// per command, then answered from memory.
func TestCatColumnsAskedOncePerCommand(t *testing.T) {
	srv := newCatHelpServer(t, map[string]string{"indices": indicesHelp})
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: srv.URL})

	injectText(screen, "GET _cat/indices?h=zz")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	eventually(t, "the first completion", func() bool {
		return editorText(app) == "GET _cat/indices?h=zz.future"
	})

	injectText(screen, ",hea")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	eventually(t, "the second completion", func() bool {
		return editorText(app) == "GET _cat/indices?h=zz.future,health"
	})

	if n := srv.requestCount(); n != 1 {
		t.Errorf("expected a single ?help request, got %d: %v", n, srv.requests)
	}
}

// TestCatColumnsFallBackToBuiltInTable checks that when the cluster can't
// be asked, the built-in table is used — and the user is told.
func TestCatColumnsFallBackToBuiltInTable(t *testing.T) {
	srv := newCatHelpServer(t, nil) // answers 500 to everything
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: srv.URL})

	injectText(screen, "GET _cat/indices?h=heal")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)

	eventually(t, "the built-in column to complete", func() bool {
		return editorText(app) == "GET _cat/indices?h=health"
	})
	waitForDraw(t, screen)
	if text := screenText(screen); !strings.Contains(text, app.msgs.WarnCatColumnsBuiltIn[:20]) {
		t.Errorf("expected the status bar to say the built-in table was used, got:\n%s", text)
	}
}

// TestCatColumnsIgnoreAnswerIfEditorChanged checks that an answer arriving
// after the user kept typing doesn't complete anything behind their back.
func TestCatColumnsIgnoreAnswerIfEditorChanged(t *testing.T) {
	srv := newCatHelpServer(t, map[string]string{"indices": indicesHelp})
	srv.hold = make(chan struct{})
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: srv.URL})

	injectText(screen, "GET _cat/indices?h=zz")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	eventually(t, "the ?help request to reach the cluster", func() bool { return srv.requestCount() == 1 })

	injectText(screen, "x") // keeps typing while the request is in flight
	eventually(t, "the keystroke", func() bool { return editorText(app) == "GET _cat/indices?h=zzx" })
	srv.release()

	eventually(t, "the answer to be recorded", func() bool {
		var recorded bool
		app.tapp.QueueUpdate(func() { _, recorded = app.catLive["indices"] })
		return recorded
	})
	if got := editorText(app); got != "GET _cat/indices?h=zzx" {
		t.Errorf("expected the editor to be left as typed, got %q", got)
	}
	// Nothing else happened meanwhile: the "running" status is cleared.
	if status := statusText(app); !strings.Contains(status, app.msgs.StatusIdle) {
		t.Errorf("expected the status bar to be back to idle, got %q", status)
	}
}

// TestLateCatAnswerLeavesNewerStatusAlone checks that the answer to a
// column request, arriving after the user moved on, doesn't wipe what the
// status bar says by then — the result of a request run in the meantime.
func TestLateCatAnswerLeavesNewerStatusAlone(t *testing.T) {
	srv := newCatHelpServer(t, map[string]string{"indices": indicesHelp})
	srv.hold = make(chan struct{})
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: srv.URL})

	injectText(screen, "GET _cat/indices?h=zz")
	eventually(t, "the typed request", func() bool { return editorText(app) == "GET _cat/indices?h=zz" })
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	eventually(t, "the ?help request to reach the cluster", func() bool { return srv.requestCount() == 1 })

	injectText(screen, "x")
	eventually(t, "the keystroke", func() bool { return editorText(app) == "GET _cat/indices?h=zzx" })
	const newer = "HTTP 200 from a request run since"
	app.tapp.QueueUpdate(func() { app.status.SetInfo(newer) })
	srv.release()

	eventually(t, "the answer to be recorded", func() bool {
		var recorded bool
		app.tapp.QueueUpdate(func() { _, recorded = app.catLive["indices"] })
		return recorded
	})
	if status := statusText(app); !strings.Contains(status, newer) {
		t.Errorf("expected the newer status to be left alone, got %q", status)
	}
}

// TestCatColumnsForCommandOutsideBuiltInTable checks that a _cat endpoint
// the built-in table has no column for (here one from the user's own
// endpoints.txt) still gets its columns completed, from the cluster.
func TestCatColumnsForCommandOutsideBuiltInTable(t *testing.T) {
	srv := newCatHelpServer(t, map[string]string{"brand_new": "alpha | a | first\nbeta | b | second\n"})
	userDir := t.TempDir()
	writeReferenceFile(t, userDir, "endpoints.txt", "_cat/brand_new?v\n")
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: srv.URL, reference: refdata.Sources{UserDir: userDir}})

	injectText(screen, "GET _cat/brand_new?h=be")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)

	eventually(t, "the column of the user-added command to complete", func() bool {
		return editorText(app) == "GET _cat/brand_new?h=beta"
	})
}
