package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/config"
	"termdevtools/esclient"
	"termdevtools/parser"
)

// CheatsheetFileName est le fichier optionnel chargé par défaut dans
// l'éditeur au démarrage, situé à côté du binaire. Voir SPEC.md §9.1.
const CheatsheetFileName = "cheatsheet.txt"

// ExportsDirName est le sous-dossier (à côté du binaire) où Ctrl+S exporte
// le résultat affiché depuis le panneau droit. Voir SPEC.md §3.3 et §9.1.
const ExportsDirName = "exports"

// EndpointsFileName est le fichier optionnel (à côté du binaire) listant
// les endpoints proposés par l'auto-complétion Tab — permet à l'équipe de
// l'ajuster à sa version d'Elasticsearch sans recompiler. Voir SPEC.md
// §3.2 et §9.1.
const EndpointsFileName = "endpoints.txt"

// CatColumnsFileName est le fichier optionnel (à côté du binaire) listant
// les colonnes h=/s= proposées par l'auto-complétion Tab pour les
// commandes _cat/*. Voir SPEC.md §3.2 et §9.1.
const CatColumnsFileName = "cat_columns.txt"

// Paths regroupe les emplacements de fichiers résolus par l'appelant
// (main.go) — voir SPEC.md §9.1 pour le détail de chacun.
type Paths struct {
	Cheatsheet string
	Exports    string
	Endpoints  string
	CatColumns string
}

// Bornes du ratio de largeur entre panneau gauche et droit (sur un total de
// splitTotalWeight), ajustable via Ctrl++/Ctrl+- (SPEC.md §4).
const (
	splitTotalWeight = 10
	splitMinWeight   = 1
	splitMaxWeight   = splitTotalWeight - splitMinWeight
	splitStep        = 1
)

// App assemble le layout principal (éditeur, résultat, barre de statut) et
// gère le focus ainsi que les raccourcis clavier globaux. Voir SPEC.md §3-4.
type App struct {
	tapp        *tview.Application
	client      *esclient.Client
	timeout     time.Duration
	exportsDir  string
	queriesPath string
	endpoints   []string
	catColumns  map[string][]string

	editor *Editor
	result *ResultView
	status *StatusBar

	root       *tview.Flex
	mainFlex   *tview.Flex
	searchBar  *tview.InputField
	leftWeight int

	completionList  *tview.List
	completionStart int
	completionEnd   int

	pages       *tview.Pages
	helpView    *tview.TextView
	helpVisible bool

	// screen sert à la copie presse-papier (F2, OSC 52, cf. SPEC.md §3.3) ;
	// capturé au premier rendu via SetAfterDrawFunc (tview.Application n'a
	// pas d'accesseur direct vers l'écran qu'il crée lui-même).
	screen tcell.Screen

	focusedIsEditor  bool
	searchTarget     string // "editor" ou "result"
	editorSearchPos  int
	resultSearchLine int
}

// NewApp construit l'écran principal pour une connexion déjà établie.
// paths est résolu par l'appelant (main.go) — voir SPEC.md §9.1 pour le
// détail des emplacements.
func NewApp(tapp *tview.Application, cr ConnectResult, cfg *config.Config, paths Paths) *App {
	a := &App{
		tapp:             tapp,
		client:           cr.Client,
		timeout:          time.Duration(cfg.DefaultTimeoutSeconds) * time.Second,
		exportsDir:       paths.Exports,
		editor:           NewEditor(),
		result:           NewResultView(),
		status:           NewStatusBar(cr.Cluster.URL, cr.DisplayUser),
		focusedIsEditor:  true,
		editorSearchPos:  -1,
		resultSearchLine: -1,
		leftWeight:       splitTotalWeight / 2,
	}

	queriesPath, err := config.QueriesPathForURL(cr.Cluster.URL)
	if err != nil {
		a.status.SetError(err.Error())
	}
	a.queriesPath = queriesPath

	endpoints, err := LoadEndpointsFile(paths.Endpoints)
	if err != nil {
		a.status.SetError(fmt.Sprintf("échec du chargement de %s : %s", paths.Endpoints, err))
	}
	if len(endpoints) == 0 {
		endpoints = knownEndpoints
	}
	a.endpoints = endpoints

	catCols, err := LoadCatColumnsFile(paths.CatColumns)
	if err != nil {
		a.status.SetError(fmt.Sprintf("échec du chargement de %s : %s", paths.CatColumns, err))
	}
	if len(catCols) == 0 {
		catCols = catColumns
	}
	a.catColumns = catCols

	a.loadInitialQueries(paths.Cheatsheet)

	a.searchBar = tview.NewInputField().SetLabel("Rechercher : ")
	a.searchBar.SetDoneFunc(a.handleSearchDone)

	a.completionList = tview.NewList().ShowSecondaryText(false)
	a.completionList.SetBorder(true).SetTitle(" Compléter (Entrée, Echap pour annuler) ")
	a.completionList.SetSelectedFunc(func(_ int, mainText, _ string, _ rune) {
		a.editor.ApplyCompletion(a.completionStart, a.completionEnd, mainText)
		a.closeCompletion()
	})
	a.completionList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			a.closeCompletion()
			return nil
		case tcell.KeyTab:
			// Un Tab de plus fait défiler les suggestions, comme des Tab
			// répétés en complétion shell classique.
			next := a.completionList.GetCurrentItem() + 1
			if next >= a.completionList.GetItemCount() {
				next = 0
			}
			a.completionList.SetCurrentItem(next)
			return nil
		}
		return event
	})

	a.mainFlex = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.editor.Widget(), 0, a.leftWeight, true).
		AddItem(a.result.Widget(), 0, splitTotalWeight-a.leftWeight, false)

	a.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.mainFlex, 0, 1, true).
		AddItem(a.searchBar, 0, 0, false).
		AddItem(a.completionList, 0, 0, false).
		AddItem(a.status.Widget(), 2, 0, false)

	a.helpView = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	a.helpView.SetBorder(true).SetTitle(" Aide (Echap pour fermer) ")
	a.helpView.SetText(helpContent)

	// Popup centré, marges autour pour laisser voir l'appli en arrière-plan
	// (idiome tview classique : Flex imbriqués avec des espaceurs nil).
	// Hauteur proportionnelle (pas fixe) pour s'adapter à la taille du
	// terminal ; le TextView reste scrollable si le contenu déborde quand
	// même sur un terminal très bas.
	helpOverlay := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(a.helpView, 68, 0, true).
			AddItem(nil, 0, 1, false),
			0, 9, true).
		AddItem(nil, 0, 1, false)

	a.pages = tview.NewPages().
		AddPage("main", a.root, true, true).
		AddPage("help", helpOverlay, true, false)

	return a
}

// loadInitialQueries charge la sauvegarde personnelle propre à ce cluster
// (a.queriesPath, Ctrl+S ou sauvegarde automatique en sortie de programme)
// si elle existe ; sinon, se rabat sur la cheatsheet d'équipe. Voir
// SPEC.md §3.2 et §9.1.
func (a *App) loadInitialQueries(cheatsheetPath string) {
	if a.queriesPath != "" {
		loaded, err := a.editor.LoadFile(a.queriesPath)
		if err != nil {
			a.status.SetError(fmt.Sprintf("échec du chargement de %s : %s", a.queriesPath, err))
			return
		}
		if loaded {
			return
		}
	}

	if _, err := a.editor.LoadFile(cheatsheetPath); err != nil {
		a.status.SetError(fmt.Sprintf("échec du chargement de %s : %s", cheatsheetPath, err))
	}
}

// Root renvoie le composant racine à afficher.
func (a *App) Root() tview.Primitive {
	return a.pages
}

// Start branche les raccourcis clavier globaux et donne le focus initial à
// l'éditeur. À appeler une fois Root() affiché (SetRoot/SwitchToPage).
func (a *App) Start() {
	a.tapp.SetInputCapture(a.handleGlobalKeys)
	a.tapp.SetAfterDrawFunc(func(screen tcell.Screen) {
		a.screen = screen
	})
	a.focusEditor()
}

func (a *App) handleGlobalKeys(event *tcell.EventKey) *tcell.EventKey {
	if a.helpVisible {
		if event.Key() == tcell.KeyEscape {
			a.closeHelp()
			return nil
		}
		return event // laisse le TextView de l'aide défiler si le contenu déborde
	}
	if a.tapp.GetFocus() == a.searchBar {
		return event // la barre de recherche gère elle-même Entrée/Échap
	}
	if a.tapp.GetFocus() == a.completionList {
		return event // la liste de complétion gère elle-même Entrée/Echap/Tab
	}

	switch {
	case event.Key() == tcell.KeyCtrlC:
		// Seul raccourci de sortie : Ctrl+Echap s'est avéré intercepté par
		// Windows lui-même sur les postes de l'équipe (raccourci OS), donc
		// abandonné. Ctrl+C reste universellement disponible au niveau
		// terminal et ne doit jamais pouvoir bloquer l'utilisateur.
		a.SaveQueriesOnExit()
		a.tapp.Stop()
		return nil
	case event.Key() == tcell.KeyEnter && event.Modifiers()&tcell.ModCtrl != 0:
		if a.focusedIsEditor {
			a.executeCurrent()
		}
		return nil
	case isCtrlShiftLeft(event):
		a.resizeSplit(-splitStep)
		return nil
	case isCtrlShiftRight(event):
		a.resizeSplit(splitStep)
		return nil
	case event.Key() == tcell.KeyLeft && event.Modifiers()&tcell.ModCtrl != 0:
		a.focusEditor()
		return nil
	case event.Key() == tcell.KeyRight && event.Modifiers()&tcell.ModCtrl != 0:
		a.focusResultPanel()
		return nil
	case event.Key() == tcell.KeyCtrlF:
		a.openSearch()
		return nil
	case event.Key() == tcell.KeyCtrlS:
		a.handleSave()
		return nil
	case event.Key() == tcell.KeyTab && a.focusedIsEditor:
		if a.tryCompletion() {
			return nil
		}
		return event // hors contexte de complétion : Tab insère une tabulation, comportement standard
	case event.Key() == tcell.KeyF1:
		a.showHelp()
		return nil
	case event.Key() == tcell.KeyF2:
		a.copyResult()
		return nil
	}
	return event
}

// isCtrlShiftLeft/isCtrlShiftRight détectent Ctrl+Maj+←/→, utilisé pour
// redimensionner le split (SPEC.md §4) — Ctrl++/Ctrl+- initialement prévus
// se sont révélés interceptés par le terminal/l'OS (zoom de police sous
// Windows Terminal notamment). Ces cas sont testés AVANT le Ctrl+←/→ simple
// (changement de focus) pour ne pas être masqués par lui.
func isCtrlShiftLeft(event *tcell.EventKey) bool {
	return event.Key() == tcell.KeyLeft && event.Modifiers()&tcell.ModCtrl != 0 && event.Modifiers()&tcell.ModShift != 0
}

func isCtrlShiftRight(event *tcell.EventKey) bool {
	return event.Key() == tcell.KeyRight && event.Modifiers()&tcell.ModCtrl != 0 && event.Modifiers()&tcell.ModShift != 0
}

// resizeSplit déplace la séparation entre panneau gauche et droit de delta
// crans (positif = agrandit la gauche, négatif = la rétrécit), borné à
// [splitMinWeight, splitMaxWeight].
func (a *App) resizeSplit(delta int) {
	newWeight := a.leftWeight + delta
	if newWeight < splitMinWeight {
		newWeight = splitMinWeight
	}
	if newWeight > splitMaxWeight {
		newWeight = splitMaxWeight
	}
	if newWeight == a.leftWeight {
		return
	}
	a.leftWeight = newWeight
	a.mainFlex.ResizeItem(a.editor.Widget(), 0, a.leftWeight)
	a.mainFlex.ResizeItem(a.result.Widget(), 0, splitTotalWeight-a.leftWeight)
}

func (a *App) focusEditor() {
	a.focusedIsEditor = true
	a.tapp.SetFocus(a.editor.Primitive())
}

func (a *App) focusResultPanel() {
	a.focusedIsEditor = false
	a.tapp.SetFocus(a.result.Primitive())
}

func (a *App) openSearch() {
	if a.focusedIsEditor {
		a.searchTarget = "editor"
	} else {
		a.searchTarget = "result"
	}
	a.searchBar.SetText("")
	a.root.ResizeItem(a.searchBar, 1, 0)
	a.tapp.SetFocus(a.searchBar)
}

// showHelp affiche le popup d'aide (F1, SPEC.md §3.1) par-dessus le layout
// courant, sans perturber le contenu de l'éditeur ni du résultat.
func (a *App) showHelp() {
	a.helpVisible = true
	a.pages.ShowPage("help")
	a.tapp.SetFocus(a.helpView)
}

func (a *App) closeHelp() {
	a.helpVisible = false
	a.pages.HidePage("help")
	if a.focusedIsEditor {
		a.focusEditor()
	} else {
		a.focusResultPanel()
	}
}

func (a *App) closeSearch() {
	a.root.ResizeItem(a.searchBar, 0, 0)
	if a.searchTarget == "editor" {
		a.focusEditor()
	} else {
		a.focusResultPanel()
	}
}

func (a *App) handleSearchDone(key tcell.Key) {
	switch key {
	case tcell.KeyEnter:
		query := a.searchBar.GetText()
		if query == "" {
			a.closeSearch()
			return
		}
		if a.searchTarget == "editor" {
			start, end, found := a.editor.FindNext(query, a.editorSearchPos)
			if found {
				a.editor.SelectRange(start, end)
				a.editorSearchPos = start
				a.status.SetIdle()
			} else {
				a.status.SetError("aucune occurrence trouvée")
			}
		} else {
			line, found := a.result.FindNext(query, a.resultSearchLine)
			if found {
				a.result.HighlightLine(line)
				a.resultSearchLine = line
				a.status.SetIdle()
			} else {
				a.status.SetError("aucune occurrence trouvée")
			}
		}
		// Le champ reste ouvert : Entrée à nouveau = occurrence suivante.
	case tcell.KeyEscape:
		a.closeSearch()
	}
}

func (a *App) executeCurrent() {
	req, err := parser.RequestAtLine(a.editor.Text(), a.editor.CursorLine())
	if err != nil {
		a.status.SetError(err.Error())
		return
	}
	if err := parser.ValidateBody(req.Body); err != nil {
		a.status.SetError(err.Error())
		return
	}

	a.status.SetRunning()
	method, path, body := req.Method, req.Path, req.Body

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
		defer cancel()
		result, err := a.client.Execute(ctx, method, path, body)

		a.tapp.QueueUpdateDraw(func() {
			if err != nil {
				a.status.SetError(err.Error())
				a.result.ShowError(err.Error())
				return
			}
			a.status.SetResult(result.StatusCode, result.Duration)
			a.result.Show(result.Body)
		})
	}()
}

// tryCompletion implémente Tab dans le panneau gauche (SPEC.md §3.2, §4) :
// complète directement s'il n'y a qu'une seule correspondance, ouvre une
// liste à choisir s'il y en a plusieurs. Renvoie false si le curseur n'est
// pas en train de taper un endpoint (Tab garde alors son comportement
// standard, cf. handleGlobalKeys).
//
// Deux contextes de complétion sont reconnus : les colonnes h=/s= d'une
// commande _cat/* en cours de frappe (catColumnCompletion, prioritaire car
// plus spécifique), sinon un nom d'endpoint classique.
func (a *App) tryCompletion() bool {
	prefix, start, end, ok := a.editor.CompletionPrefix()
	if !ok {
		return false
	}

	if candidates, subLen, ok := catColumnCompletion(prefix, a.catColumns); ok {
		a.offerCompletions(end-subLen, end, candidates)
		return true
	}

	// Le "/" final avant les paramètres (ou en toute fin de chemin) est
	// optionnel en HTTP — "_cat/indices/?h=..." équivaut à
	// "_cat/indices?h=...". Aucun endpoint connu n'en stocke un : sans le
	// retirer avant la comparaison, un "/" de fin ne matcherait jamais rien.
	// La complétion remplace tout le segment tapé (start..end, donc le "/"
	// avec), pas seulement le texte qui le précède — pas besoin d'ajuster
	// les bornes pour ça.
	endpointPrefix := strings.TrimSuffix(prefix, "/")
	a.offerCompletions(start, end, matchPrefix(endpointPrefix, a.endpoints))
	return true
}

// offerCompletions applique le résultat d'une recherche de complétion,
// quel que soit son contexte (endpoint ou colonne _cat) : complète
// directement s'il n'y a qu'une seule correspondance, ouvre la liste à
// choisir s'il y en a plusieurs, signale l'absence de correspondance sinon.
func (a *App) offerCompletions(start, end int, matches []string) {
	switch len(matches) {
	case 0:
		a.status.SetError("aucune complétion")
	case 1:
		a.editor.ApplyCompletion(start, end, matches[0])
		a.status.SetIdle()
	default:
		a.openCompletion(start, end, matches)
	}
}

func (a *App) openCompletion(start, end int, matches []string) {
	a.completionStart, a.completionEnd = start, end

	a.completionList.Clear()
	for _, m := range matches {
		a.completionList.AddItem(m, "", 0, nil)
	}
	a.completionList.SetCurrentItem(0)

	height := len(matches)
	if height > 8 {
		height = 8
	}
	a.root.ResizeItem(a.completionList, height+2, 0) // +2 : bordure haut/bas
	a.tapp.SetFocus(a.completionList)
}

func (a *App) closeCompletion() {
	a.root.ResizeItem(a.completionList, 0, 0)
	a.focusEditor()
}

// handleSave implémente Ctrl+S, dont le comportement dépend du panneau
// ayant le focus (SPEC.md §3.2, §3.3, §4) : sauvegarde des requêtes depuis
// la gauche, export du résultat depuis la droite.
func (a *App) handleSave() {
	if a.focusedIsEditor {
		a.saveQueries()
	} else {
		a.exportResult()
	}
}

func (a *App) saveQueries() {
	if err := a.editor.SaveToFile(a.queriesPath); err != nil {
		a.status.SetError(fmt.Sprintf("échec de sauvegarde : %s", err))
		return
	}
	a.status.SetInfo(fmt.Sprintf("requêtes sauvegardées dans %s", a.queriesPath))
}

// SaveQueriesOnExit sauvegarde silencieusement (best-effort, pas de retour
// utilisateur possible à ce stade) le contenu de l'éditeur avant fermeture
// du programme — Ctrl+C ou signal externe (SIGTERM/SIGHUP, cf. main.go).
// Complète la sauvegarde explicite Ctrl+S. Voir SPEC.md §3.2.
func (a *App) SaveQueriesOnExit() {
	if a.queriesPath == "" {
		return
	}
	_ = a.editor.SaveToFile(a.queriesPath)
}

func (a *App) exportResult() {
	path, err := a.result.Export(a.exportsDir)
	if err != nil {
		a.status.SetError(fmt.Sprintf("échec d'export : %s", err))
		return
	}
	a.status.SetInfo(fmt.Sprintf("résultat exporté dans %s", path))
}

// copyResult implémente F2 : copie l'intégralité du résultat affiché vers
// le presse-papier local via OSC 52 (SPEC.md §3.3) — nécessaire car
// EnableMouse(true) empêche la sélection native du terminal dans ce
// panneau. Best-effort : ni tcell ni OSC 52 ne renvoient de confirmation
// que la copie a réellement abouti côté terminal.
func (a *App) copyResult() {
	text := a.result.PlainText()
	if text == "" {
		a.status.SetError("aucun résultat à copier")
		return
	}
	if a.screen != nil {
		a.screen.SetClipboard([]byte(text))
	}
	a.status.SetInfo("résultat copié (OSC 52 — nécessite un terminal compatible)")
}
