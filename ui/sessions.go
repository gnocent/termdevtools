package ui

import (
	"fmt"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/gnocent/termdevtools/config"
)

// connectPageName is the tview.Pages key of the connection screen among the
// sessions' pages; each App has its own, appPageName.
const connectPageName = "connect"

// appPageName is the tview.Pages key of the App connected to url: one App
// per URL (Sessions.find).
func appPageName(url string) string {
	return "app " + url
}

// Sessions holds the connections open in one run of the program — one App
// per cluster URL, each with all its state (requests, cursor, result, request
// in progress) — and the connection screen that opens them: at startup, then
// in session with Ctrl+O (SPEC.md §3.0). One App is on screen at a time; the
// others carry on in the background, their answers landing in their own
// panels.
//
// The keyboard goes through a single input capture, installed here once:
// App.Start would have each App replace the previous one's.
type Sessions struct {
	tapp  *tview.Application
	cfg   *config.Config
	paths Paths
	pages *tview.Pages

	// apps are the open connections, in the order they were opened. Only
	// changed from the interface's goroutine, under mu: SaveOnSignal reads
	// them from the signal handler's.
	mu   sync.Mutex
	apps []*App
	// current is the App on screen, or under the connection screen while
	// switching; nil until the first connection.
	current   *App
	switching bool
	// connect is the connection screen shown last: Esc on its list goes
	// back to current.
	connect *connectScreen
	// exitWarned is set once quitting has been refused because a left panel
	// couldn't be saved: the next Ctrl+C quits regardless.
	exitWarned bool
}

// NewSessions builds the program's screens, starting on the connection
// screen, and installs their keyboard shortcuts. paths is resolved by the
// caller (main.go), as for NewApp.
func NewSessions(tapp *tview.Application, cfg *config.Config, paths Paths) *Sessions {
	s := &Sessions{tapp: tapp, cfg: cfg, paths: paths, pages: tview.NewPages()}
	s.connect = newConnectScreen(tapp, cfg, s.connected, s)
	s.pages.AddPage(connectPageName, s.connect.pages, true, true)
	tapp.SetInputCapture(s.handleKeys)
	return s
}

// Root returns the root component to display.
func (s *Sessions) Root() tview.Primitive {
	return s.pages
}

// handleKeys hands the keyboard to the App on screen. On the connection
// screen, only Ctrl+C is taken — it must quit from there too, saving every
// open connection — and, in session, Esc on the list: back to the cluster
// the screen was opened from. The form handles Esc itself.
func (s *Sessions) handleKeys(event *tcell.EventKey) *tcell.EventKey {
	if s.current != nil && !s.switching {
		return s.current.handleGlobalKeys(event)
	}
	switch event.Key() {
	case tcell.KeyCtrlC:
		s.quit()
		return nil
	case tcell.KeyEscape:
		if s.switching {
			if front, _ := s.connect.pages.GetFrontPage(); front == "list" {
				s.show(s.current)
				return nil
			}
		}
	}
	return event
}

// connected opens a new connection's App and puts it on screen — unless its
// URL is open already, which goes back to that one: two connections to one
// URL would share its saved requests, and the keys could go to the one whose
// protection isn't the one shown.
func (s *Sessions) connected(cr ConnectResult) {
	if open := s.find(cr.Cluster.URL); open != nil {
		s.show(open)
		return
	}
	app := NewApp(s.tapp, cr, s.cfg, s.paths)
	app.host = s
	s.mu.Lock()
	s.apps = append(s.apps, app)
	s.mu.Unlock()
	s.pages.AddPage(appPageName(app.clusterURL), app.Root(), true, false)
	s.show(app)
}

// find returns the open connection to url, nil if there is none — or no
// Sessions at all, for a connection screen built on its own.
func (s *Sessions) find(url string) *App {
	if s == nil {
		return nil
	}
	for _, app := range s.apps {
		if app.clusterURL == url {
			return app
		}
	}
	return nil
}

// openSwitch implements Ctrl+O: the connection screen, in session. A cluster
// already open goes straight back to its screen, another one through the
// form, as at startup. The left panel on screen is saved first, as on exit:
// it stays safe on disk whatever happens while another cluster is on screen.
// A save that fails doesn't stop the switch — the left panel is still in
// memory, saved again on exit — but is said at once, on the connection
// screen, and on the cluster's own status bar for when it is gone back to.
func (s *Sessions) openSwitch() {
	saveErr := s.current.SaveQueriesOnExit()
	s.switching = true
	s.connect = newConnectScreen(s.tapp, s.cfg, s.connected, s)
	s.pages.AddAndSwitchToPage(connectPageName, s.connect.pages, true)
	if saveErr != nil {
		message := fmt.Sprintf(s.current.msgs.ErrSaveFailedFmt, saveErr)
		s.connect.setMessage(message, "red")
		s.current.status.SetError(message)
	}
}

// show puts app on screen, as it was left.
func (s *Sessions) show(app *App) {
	s.current = app
	s.switching = false
	// An attempt still in progress on the connection screen is given up, as
	// Esc does: answering later, it would take the screen over.
	s.connect.attempt++
	// Switching pages hands the focus to the page's first panel, the editor,
	// which then counts as the one that had it: what had it is read first.
	editorHadIt := app.focusedIsEditor
	s.pages.SwitchToPage(appPageName(app.clusterURL))
	app.focusedIsEditor = editorHadIt
	app.show()
}

// answered is told by app that the answer to its request has come. Off
// screen, app keeps it for when it is gone back to: the status bar on
// screen says so, naming its cluster.
func (s *Sessions) answered(app *App) {
	if s.current != app {
		s.current.status.SetInfo(fmt.Sprintf(s.current.msgs.InfoAnsweredElsewhereFmt, app.clusterURL))
	}
}

// quit implements Ctrl+C, and the connection screen's Quit: every open
// connection's left panel is saved first. One that can't be saved is put on
// screen with the reason, once; the next Ctrl+C quits regardless.
func (s *Sessions) quit() {
	for _, app := range s.apps {
		if err := app.SaveQueriesOnExit(); err != nil && !s.exitWarned {
			s.exitWarned = true
			s.show(app)
			app.status.SetError(fmt.Sprintf(app.msgs.ErrExitSaveFailedFmt, err))
			return
		}
	}
	s.tapp.Stop()
}

// SaveOnSignal is quit's save for the handler of SIGTERM/SIGHUP in main.go:
// every open connection's left panel, under one time limit (see
// saveOnSignal).
func (s *Sessions) SaveOnSignal() {
	s.mu.Lock()
	apps := append([]*App(nil), s.apps...)
	s.mu.Unlock()
	saveOnSignal(s.tapp, apps)
}
