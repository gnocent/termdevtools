package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// Tests of the request in progress: Esc abandons it, the status bar counts
// how long it has been running, and a newer request cancels it if it only
// reads (SPEC.md §3.1, §3.2).

// slowServer answers at once every request but those whose path holds
// "slow", which wait until release is closed or the client goes away.
type slowServer struct {
	url string
	// arrived receives "METHOD path" for each slow request, as it arrives;
	// outcome then receives "released" or "cancelled" for it.
	arrived     chan string
	outcome     chan string
	release     chan struct{}
	releaseOnce sync.Once
}

// releaseAll lets the slow requests still waiting answer.
func (s *slowServer) releaseAll() {
	s.releaseOnce.Do(func() { close(s.release) })
}

func newSlowServer(t *testing.T) *slowServer {
	t.Helper()
	s := &slowServer{arrived: make(chan string, 8), outcome: make(chan string, 8), release: make(chan struct{})}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "slow") {
			_, _ = w.Write([]byte(`{"answer":"fast"}`))
			return
		}
		s.arrived <- r.Method + " " + r.URL.Path
		select {
		case <-s.release:
			_, _ = w.Write([]byte(`{"answer":"slow"}`))
			s.outcome <- "released"
		case <-r.Context().Done():
			s.outcome <- "cancelled"
		}
	}))
	t.Cleanup(srv.Close)
	// Registered after srv.Close, so run before it: Close waits for the
	// handlers still blocked.
	t.Cleanup(s.releaseAll)
	s.url = srv.URL
	return s
}

// waitArrived waits for the server to receive a slow request.
func (s *slowServer) waitArrived(t *testing.T) {
	t.Helper()
	select {
	case <-s.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("the slow request never reached the server")
	}
}

// waitOutcome waits for what became of a slow request.
func (s *slowServer) waitOutcome(t *testing.T) string {
	t.Helper()
	select {
	case got := <-s.outcome:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("the slow request never ended")
		return ""
	}
}

// formatPrefix is what a message format reads before its first verb.
func formatPrefix(format string) string {
	return strings.SplitN(format, "%", 2)[0]
}

func resultPlainText(app *App) string {
	return uiValue(app, app.result.PlainText)
}

func requestInProgress(app *App) bool {
	return uiValue(app, func() bool { return app.running != nil })
}

// TestEscAbandonsTheRequestInProgress checks that Esc closes the
// connection of a read in progress, says so, and leaves the result panel
// with what it showed.
func TestEscAbandonsTheRequestInProgress(t *testing.T) {
	s := newSlowServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: s.url})

	execute(t, app, screen, "GET fast")
	eventually(t, "the fast answer", func() bool { return strings.Contains(resultPlainText(app), `"answer": "fast"`) })

	execute(t, app, screen, "GET slow")
	s.waitArrived(t)
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got := s.waitOutcome(t); got != "cancelled" {
		t.Errorf("expected the connection to be closed, the server saw the request %s", got)
	}
	if status := statusText(app); !strings.Contains(status, formatPrefix(app.msgs.InfoRequestAbandonedFmt)) {
		t.Errorf("expected the status bar to say the request was abandoned, got %q", status)
	}
	if requestInProgress(app) {
		t.Error("expected no request in progress any more")
	}
	// Time for the cancelled call to come back, were it to be displayed.
	time.Sleep(200 * time.Millisecond)
	if got := resultPlainText(app); !strings.Contains(got, "# GET fast") {
		t.Errorf("expected the result panel to keep the previous result, got:\n%s", got)
	}
	if status := statusText(app); !strings.Contains(status, formatPrefix(app.msgs.InfoRequestAbandonedFmt)) {
		t.Errorf("expected the abandoned request's error not to replace the message, got %q", status)
	}
}

// TestEscAbandoningAWriteWarns checks that abandoning a write says it may
// have been carried out all the same.
func TestEscAbandoningAWriteWarns(t *testing.T) {
	s := newSlowServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: s.url})

	execute(t, app, screen, "PUT slow")
	s.waitArrived(t)
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if status := statusText(app); !strings.Contains(status, formatPrefix(app.msgs.WarnWriteAbandonedFmt)) {
		t.Errorf("expected the status bar to warn the write may have gone through, got %q", status)
	}
}

// TestEscWithoutRequestChangesNothing checks that Esc, with no request in
// progress, leaves the status bar alone.
func TestEscWithoutRequestChangesNothing(t *testing.T) {
	app, screen := newTestApp(t)
	before := statusText(app)
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)
	if after := statusText(app); after != before {
		t.Errorf("expected the status bar to stay %q, got %q", before, after)
	}
}

// TestEscClosesTheSearchBarFirst checks that Esc pressed to close the
// search bar doesn't also abandon the request in progress.
func TestEscClosesTheSearchBarFirst(t *testing.T) {
	s := newSlowServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: s.url})

	execute(t, app, screen, "GET slow")
	s.waitArrived(t)
	screen.InjectKey(tcell.KeyCtrlF, 0, tcell.ModCtrl)
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if !requestInProgress(app) {
		t.Error("expected the request to be still in progress")
	}
}

// TestElapsedTimeIsShownWhileWaiting checks that the status bar counts the
// seconds of a request in progress, and says how to abandon it.
func TestElapsedTimeIsShownWhileWaiting(t *testing.T) {
	s := newSlowServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: s.url})

	execute(t, app, screen, "GET slow")
	want := fmt.Sprintf(app.msgs.StatusRunningForFmt, 1)
	eventually(t, "the elapsed time", func() bool { return strings.Contains(statusText(app), want) })
}

// TestElapsedTimeDoesNotCoverANewerMessage checks that a message written
// while the request is in progress (a save) stays in sight.
func TestElapsedTimeDoesNotCoverANewerMessage(t *testing.T) {
	s := newSlowServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: s.url})

	execute(t, app, screen, "GET slow")
	s.waitArrived(t)
	screen.InjectKey(tcell.KeyCtrlS, 0, tcell.ModCtrl)
	waitForDraw(t, screen)
	saved := statusText(app)
	if strings.Contains(saved, app.msgs.StatusRunning) {
		t.Fatalf("expected Ctrl+S to write its own message, got %q", saved)
	}

	time.Sleep(elapsedTick + 300*time.Millisecond)
	if got := statusText(app); got != saved {
		t.Errorf("expected the save message to stay, got %q", got)
	}
}

// TestNewerRequestCancelsAReadInProgress checks that a request sent while
// a read is in progress closes the read's connection.
func TestNewerRequestCancelsAReadInProgress(t *testing.T) {
	s := newSlowServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: s.url})

	execute(t, app, screen, "GET slow")
	s.waitArrived(t)
	execute(t, app, screen, "GET fast")

	if got := s.waitOutcome(t); got != "cancelled" {
		t.Errorf("expected the read to be cancelled, the server saw it %s", got)
	}
	eventually(t, "the fast answer", func() bool { return strings.Contains(resultPlainText(app), `"answer": "fast"`) })
}

// TestNewerRequestLetsAWriteFinish checks that a request sent while a write
// is in progress leaves the write's connection open.
func TestNewerRequestLetsAWriteFinish(t *testing.T) {
	s := newSlowServer(t)
	app, screen := newTestAppWith(t, testAppOptions{clusterURL: s.url})

	execute(t, app, screen, "PUT slow")
	s.waitArrived(t)
	execute(t, app, screen, "GET fast")
	eventually(t, "the fast answer", func() bool { return strings.Contains(resultPlainText(app), `"answer": "fast"`) })

	// Time for a cancellation to reach the server, were there one.
	time.Sleep(200 * time.Millisecond)
	s.releaseAll()
	if got := s.waitOutcome(t); got != "released" {
		t.Errorf("expected the write to be left to finish, the server saw it %s", got)
	}
	// Time for the write's late answer to reach the interface, were it to be
	// shown: the result displayed stays that of the last request sent.
	time.Sleep(200 * time.Millisecond)
	if got := resultPlainText(app); !strings.Contains(got, "# GET fast") || strings.Contains(got, "slow") {
		t.Errorf("expected the result of the last request sent to stay displayed, got:\n%s", got)
	}
}
