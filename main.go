// Command termdevtools is a terminal-mode Elasticsearch client, inspired by
// Kibana's DevTools view. See SPEC.md for the detail of design choices.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"time"

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
	defer recoverCrash(tapp, exeDir, msgs)
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

	tapp.SetRoot(pages, true).EnableMouse(cfg.Mouse)
	if err := tapp.Run(); err != nil {
		fmt.Fprintf(os.Stderr, msgs.ErrFatalFmt+"\n", err)
		os.Exit(1)
	}
}

// recoverCrash catches a panic in the main goroutine — where
// tview.Application.Run processes events — and writes it, with a full stack
// trace, to a timestamped file next to the binary: the only way to get a
// real diagnosis for a crash nobody watching can reproduce or transcribe by
// hand (same idea as cmd/keydebug, applied to panics instead of raw key
// events). Best-effort only: a panic inside tcell's own separate
// input-reading goroutine, rather than in the application code Run() itself
// processes, would not be caught here — Go's recover only catches panics in
// the same goroutine as the deferred call.
func recoverCrash(tapp *tview.Application, exeDir string, msgs *i18n.Strings) {
	r := recover()
	if r == nil {
		return
	}

	// Best-effort attempt to restore the terminal before printing anything —
	// guarded so a second panic here (screen state may already be broken)
	// doesn't hide the original crash report below.
	func() {
		defer func() { _ = recover() }()
		tapp.Stop()
	}()

	report := fmt.Sprintf("%v\n\n%s", r, debug.Stack())
	path := filepath.Join(exeDir, fmt.Sprintf("crash-%s.log", time.Now().Format("20060102-150405")))

	fmt.Fprintf(os.Stderr, msgs.ErrCrashedFmt+"\n", r)
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, msgs.ErrCrashReportWriteFailedFmt+"\n", err)
		fmt.Fprint(os.Stderr, report)
	} else {
		fmt.Fprintf(os.Stderr, msgs.InfoCrashReportFmt+"\n", path)
	}
	os.Exit(1)
}
