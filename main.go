// Command termdevtools est un client Elasticsearch en mode terminal, inspiré
// de la vue DevTools de Kibana. Voir SPEC.md pour le détail des choix.
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
	"termdevtools/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Erreur de chargement de config.yaml :", err)
		os.Exit(1)
	}

	exeDir, err := config.ExecutableDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Impossible de déterminer le dossier de l'exécutable :", err)
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

	// Ctrl+C doit pouvoir quitter dès l'écran de connexion, avant même que
	// App (et son propre SetInputCapture plus complet) n'existe — sinon
	// aucun raccourci de sortie n'est actif tant que la connexion n'a pas
	// abouti. App.Start() remplacera ce capteur minimal par le sien.
	tapp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			tapp.Stop()
			return nil
		}
		return event
	})

	// Référence à l'App courante (nil tant qu'aucune connexion n'a abouti),
	// pour permettre au gestionnaire de signaux ci-dessous de sauvegarder
	// les requêtes en cours même en cas d'arrêt externe du programme.
	var currentApp atomic.Pointer[ui.App]

	connectPage := ui.BuildConnectPage(tapp, cfg, func(cr ui.ConnectResult) {
		app := ui.NewApp(tapp, cr, cfg, paths)
		currentApp.Store(app)
		pages.AddAndSwitchToPage("main", app.Root(), true)
		app.Start()
	})
	pages.AddPage("connect", connectPage, true, true)

	// Sauvegarde best-effort du panneau gauche en plus de Ctrl+S explicite
	// (SPEC.md §3.2) : Ctrl+C est déjà couvert par App.handleGlobalKeys (ce
	// n'est qu'un octet de contrôle lu par le terminal en mode raw, pas un
	// signal OS). SIGTERM/SIGHUP couvrent les arrêts externes (kill, session
	// SSH coupée) — SIGKILL reste, comme pour tout programme, impossible à
	// intercepter.
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
		fmt.Fprintln(os.Stderr, "Erreur fatale :", err)
		os.Exit(1)
	}
}
