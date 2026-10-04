// Command termdevtools is a terminal-mode Elasticsearch client, inspired by
// Kibana's DevTools view. See SPEC.md for the detail of design choices.
package main

import (
	"flag"
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
	"termdevtools/refdata"
	"termdevtools/ui"
)

// version identifies the build. Release binaries are stamped by
// build-release.sh (-ldflags "-X main.version=..."); anything else — go
// build, go run, the install scripts — says "dev".
var version = "dev"

func main() {
	// Before anything is read or created: asking a binary what it is must
	// work on a machine where it has never run.
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			fmt.Println("termdevtools " + version)
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		// cfg (and so cfg.Language) isn't available yet at this point —
		// i18n.For("") gives the interface's default language, English.
		fmt.Fprintf(os.Stderr, i18n.For("").ErrConfigLoadFmt+"\n", err)
		os.Exit(1)
	}
	msgs := i18n.For(cfg.Language)

	exportDir := flag.String("export-defaults", "", msgs.UsageExportDefaults)
	flag.Bool("version", false, msgs.UsageVersion) // handled above; declared for the usage text
	flag.Parse()

	exeDir, err := config.ExecutableDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, msgs.ErrExecDirFmt+"\n", err)
		os.Exit(1)
	}
	// Empty if the configuration directory can't be resolved: the user's
	// reference files are then simply not looked for (config.Load above
	// would already have failed for the same reason anyway).
	userDir, _ := config.ConfigDir()
	reference := refdata.Sources{TeamDir: exeDir, UserDir: userDir}

	if *exportDir != "" {
		// Exported where reference files are read from, the built-in recipes
		// would all come back as the user's own: frozen copies, hiding the
		// corrections of every later version.
		if reference.Reads(*exportDir) {
			fmt.Fprintf(os.Stderr, msgs.ErrExportIntoSourceFmt+"\n", *exportDir)
			os.Exit(1)
		}
		written, err := refdata.ExportDefaults(*exportDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, msgs.ErrExportDefaultsFmt+"\n", err)
			os.Exit(1)
		}
		fmt.Printf(msgs.InfoExportedDefaultsFmt+"\n", len(written), *exportDir)
		return
	}

	// Best-effort, like the other self-documenting files (config.yaml,
	// variables): without it the user's recipes simply have no template.
	_ = refdata.CreateUserTemplate(userDir)
	paths := ui.Paths{
		Cheatsheet: filepath.Join(exeDir, ui.CheatsheetFileName),
		Starter:    refdata.Starter(),
		Exports:    filepath.Join(exeDir, ui.ExportsDirName),
		Reference:  reference,
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
			app.SaveQueriesOnSignal()
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
// hand. Best-effort only: a panic inside tcell's own separate
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
