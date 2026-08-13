// Command termdevtools is a terminal-mode Elasticsearch client, inspired by
// Kibana's DevTools view. See SPEC.md for the detail of design choices.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/config"
	"termdevtools/i18n"
	"termdevtools/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// cfg (and so cfg.Language) isn't available yet at this point —
		// i18n.For("") falls back to French, matching the interface's
		// original default language.
		fmt.Fprintf(os.Stderr, i18n.For("").ErrConfigLoadFmt+"\n", err)
		os.Exit(1)
	}
	msgs := i18n.For(cfg.Language)

	exeDir, err := config.ExecutableDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, msgs.ErrExecDirFmt+"\n", err)
		os.Exit(1)
	}
	paths := ui.Paths{
		Cheatsheet: filepath.Join(exeDir, ui.CheatsheetFileName),
		Exports:    filepath.Join(exeDir, ui.ExportsDirName),
		Endpoints:  filepath.Join(exeDir, ui.EndpointsFileName),
		CatColumns: filepath.Join(exeDir, ui.CatColumnsFileName),
	}

	tapp := tview.NewApplication()
	pages := tview.NewPages()

	// Ctrl+C must be able to quit right from the connection screen, before
	// App (and its own, more complete SetInputCapture) even exists —
	// otherwise no exit shortcut is active until the connection succeeds.
	// App.Start() will replace this minimal handler with its own.
	tapp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			tapp.Stop()
			return nil
		}
		return event
	})

	// Reference to the current App (nil until a connection has succeeded),
	// so the signal handler below can save in-progress requests even on an
	// external shutdown of the program.
	var currentApp atomic.Pointer[ui.App]

	connectPage := ui.BuildConnectPage(tapp, cfg, func(cr ui.ConnectResult) {
		app := ui.NewApp(tapp, cr, cfg, paths)
		currentApp.Store(app)
		pages.AddAndSwitchToPage("main", app.Root(), true)
		app.Start()
	})
	pages.AddPage("connect", connectPage, true, true)

	// Best-effort save of the left panel in addition to explicit Ctrl+S
	// (SPEC.md §3.2): Ctrl+C is already covered by App.handleGlobalKeys (it's
	// just a control byte read by the terminal in raw mode, not an OS
	// signal). SIGTERM/SIGHUP cover external shutdowns (kill, a dropped SSH
	// session) — SIGKILL, as with any program, remains impossible to
	// intercept.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sig
		if app := currentApp.Load(); app != nil {
			app.SaveQueriesOnExit()
		}
		tapp.Stop()
	}()

	tapp.SetRoot(pages, true).EnableMouse(true)
	if err := tapp.Run(); err != nil {
		fmt.Fprintf(os.Stderr, msgs.ErrFatalFmt+"\n", err)
		os.Exit(1)
	}
}
