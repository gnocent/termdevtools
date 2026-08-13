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
	"termdevtools/i18n"
	"termdevtools/parser"
)

// CheatsheetFileName is the optional file loaded into the editor by default
// on startup, located next to the binary. See SPEC.md §9.1.
const CheatsheetFileName = "cheatsheet.txt"

// ExportsDirName is the subfolder (next to the binary) where Ctrl+S exports
// the result displayed in the right panel. See SPEC.md §3.3 and §9.1.
const ExportsDirName = "exports"

// EndpointsFileName is the optional file (next to the binary) listing the
// endpoints offered by Tab auto-completion — lets the team adjust it to
// their Elasticsearch version without recompiling. See SPEC.md §3.2 and §9.1.
const EndpointsFileName = "endpoints.txt"

// CatColumnsFileName is the optional file (next to the binary) listing the
// h=/s= columns offered by Tab auto-completion for _cat/* commands. See
// SPEC.md §3.2 and §9.1.
const CatColumnsFileName = "cat_columns.txt"

// Paths gathers the file locations resolved by the caller (main.go) — see
// SPEC.md §9.1 for the detail of each.
type Paths struct {
	Cheatsheet string
	Exports    string
	Endpoints  string
	CatColumns string
}

// Bounds of the width ratio between the left and right panels (out of a
// total of splitTotalWeight), adjustable via Ctrl+Shift+←/→ (SPEC.md §4).
const (
	splitTotalWeight = 10
	splitMinWeight   = 1
	splitMaxWeight   = splitTotalWeight - splitMinWeight
	splitStep        = 1
)

// App assembles the main layout (editor, result, status bar) and manages
// focus as well as global keyboard shortcuts. See SPEC.md §3-4.
type App struct {
	tapp        *tview.Application
	client      *esclient.Client
	timeout     time.Duration
	exportsDir  string
	queriesPath string
	endpoints   []string
	catColumns  map[string][]string
	msgs        *i18n.Strings

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

	// screen is used for clipboard copy (F2, OSC 52, see SPEC.md §3.3);
	// captured on the first render via SetAfterDrawFunc (tview.Application
	// has no direct accessor to the screen it creates itself).
	screen tcell.Screen

	focusedIsEditor  bool
	searchTarget     string // "editor" or "result"
	editorSearchPos  int
	resultSearchLine int
}

// NewApp builds the main screen for an already-established connection.
// paths is resolved by the caller (main.go) — see SPEC.md §9.1 for the
// detail of the locations.
func NewApp(tapp *tview.Application, cr ConnectResult, cfg *config.Config, paths Paths) *App {
	msgs := i18n.For(cfg.Language)
	a := &App{
		tapp:             tapp,
		client:           cr.Client,
		timeout:          time.Duration(cfg.DefaultTimeoutSeconds) * time.Second,
		exportsDir:       paths.Exports,
		msgs:             msgs,
		editor:           NewEditor(msgs),
		result:           NewResultView(msgs),
		status:           NewStatusBar(cr.Cluster.URL, cr.DisplayUser, msgs),
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
		a.status.SetError(fmt.Sprintf(msgs.ErrLoadFailedFmt, paths.Endpoints, err))
	}
	if len(endpoints) == 0 {
		endpoints = knownEndpoints
	}
	a.endpoints = endpoints

	catCols, err := LoadCatColumnsFile(paths.CatColumns)
	if err != nil {
		a.status.SetError(fmt.Sprintf(msgs.ErrLoadFailedFmt, paths.CatColumns, err))
	}
	if len(catCols) == 0 {
		catCols = catColumns
	}
	a.catColumns = catCols

	a.loadInitialQueries(paths.Cheatsheet)

	a.searchBar = tview.NewInputField().SetLabel(msgs.SearchLabel)
	a.searchBar.SetDoneFunc(a.handleSearchDone)

	a.completionList = tview.NewList().ShowSecondaryText(false)
	a.completionList.SetBorder(true).SetTitle(msgs.CompletionTitle)
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
			// One more Tab cycles through the suggestions, like repeated
			// Tabs in classic shell completion.
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
	a.helpView.SetBorder(true).SetTitle(msgs.HelpViewTitle)
	a.helpView.SetText(msgs.HelpContent)

	// Centered popup, margins around it to let the app show through in the
	// background (classic tview idiom: nested Flex with nil spacers).
	// Proportional (not fixed) height to adapt to the terminal size; the
	// TextView stays scrollable if the content still overflows on a very
	// short terminal.
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

// loadInitialQueries loads the personal save specific to this cluster
// (a.queriesPath, Ctrl+S or automatic save on program exit) if it exists;
// otherwise, falls back to the team cheatsheet. See SPEC.md §3.2 and §9.1.
func (a *App) loadInitialQueries(cheatsheetPath string) {
	if a.queriesPath != "" {
		loaded, err := a.editor.LoadFile(a.queriesPath)
		if err != nil {
			a.status.SetError(fmt.Sprintf(a.msgs.ErrLoadFailedFmt, a.queriesPath, err))
			return
		}
		if loaded {
			return
		}
	}

	if _, err := a.editor.LoadFile(cheatsheetPath); err != nil {
		a.status.SetError(fmt.Sprintf(a.msgs.ErrLoadFailedFmt, cheatsheetPath, err))
	}
}

// Root returns the root component to display.
func (a *App) Root() tview.Primitive {
	return a.pages
}

// Start wires up the global keyboard shortcuts and gives the editor initial
// focus. Must be called once Root() is displayed (SetRoot/SwitchToPage).
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
		return event // let the help TextView scroll if content overflows
	}
	if a.tapp.GetFocus() == a.searchBar {
		return event // the search bar handles Enter/Escape itself
	}
	if a.tapp.GetFocus() == a.completionList {
		return event // the completion list handles Enter/Escape/Tab itself
	}

	switch {
	case event.Key() == tcell.KeyCtrlC:
		// The only exit shortcut: Ctrl+Esc turned out to be intercepted by
		// Windows itself on the team's machines (an OS shortcut), so it was
		// dropped. Ctrl+C stays universally available at the terminal level
		// and must never be able to lock the user out.
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
		return event // outside a completion context: Tab inserts a tab, standard behavior
	case event.Key() == tcell.KeyF1:
		a.showHelp()
		return nil
	case event.Key() == tcell.KeyF2:
		a.copyResult()
		return nil
	}
	return event
}

// isCtrlShiftLeft/isCtrlShiftRight detect Ctrl+Shift+←/→, used to resize
// the split (SPEC.md §4) — Ctrl++/Ctrl+- initially planned turned out to be
// intercepted by the terminal/OS (font zoom under Windows Terminal in
// particular). These cases are tested BEFORE plain Ctrl+←/→ (focus switch)
// so they aren't shadowed by it.
func isCtrlShiftLeft(event *tcell.EventKey) bool {
	return event.Key() == tcell.KeyLeft && event.Modifiers()&tcell.ModCtrl != 0 && event.Modifiers()&tcell.ModShift != 0
}

func isCtrlShiftRight(event *tcell.EventKey) bool {
	return event.Key() == tcell.KeyRight && event.Modifiers()&tcell.ModCtrl != 0 && event.Modifiers()&tcell.ModShift != 0
}

// resizeSplit moves the divider between the left and right panels by delta
// steps (positive = grows the left, negative = shrinks it), clamped to
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

// showHelp displays the help popup (F1, SPEC.md §3.1) over the current
// layout, without disturbing the editor's or result's content.
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
				a.status.SetError(a.msgs.ErrNoMatchFound)
			}
		} else {
			line, found := a.result.FindNext(query, a.resultSearchLine)
			if found {
				a.result.HighlightLine(line)
				a.resultSearchLine = line
				a.status.SetIdle()
			} else {
				a.status.SetError(a.msgs.ErrNoMatchFound)
			}
		}
		// The field stays open: pressing Enter again = next occurrence.
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

// tryCompletion implements Tab in the left panel (SPEC.md §3.2, §4):
// completes directly if there's only a single match, opens a list to
// choose from if there are several. Returns false if the cursor isn't in
// the middle of typing an endpoint (Tab then keeps its standard behavior,
// see handleGlobalKeys).
//
// Two completion contexts are recognized: the h=/s= columns of a _cat/*
// command being typed (catColumnCompletion, takes priority since it's more
// specific), otherwise a regular endpoint name.
func (a *App) tryCompletion() bool {
	prefix, start, end, ok := a.editor.CompletionPrefix()
	if !ok {
		return false
	}

	if candidates, subLen, ok := catColumnCompletion(prefix, a.catColumns); ok {
		a.offerCompletions(end-subLen, end, candidates)
		return true
	}

	// The trailing "/" before the parameters (or at the very end of the
	// path) is optional in HTTP — "_cat/indices/?h=..." is equivalent to
	// "_cat/indices?h=...". No known endpoint stores one: without removing
	// it before comparison, a trailing "/" would never match anything. The
	// completion replaces the whole typed segment (start..end, so the "/"
	// included), not just the text preceding it — no need to adjust the
	// bounds for this.
	endpointPrefix := strings.TrimSuffix(prefix, "/")
	a.offerCompletions(start, end, matchPrefix(endpointPrefix, a.endpoints))
	return true
}

// offerCompletions applies the result of a completion search, regardless of
// its context (endpoint or _cat column): completes directly if there's only
// a single match, opens the list to choose from if there are several,
// otherwise reports no match.
func (a *App) offerCompletions(start, end int, matches []string) {
	switch len(matches) {
	case 0:
		a.status.SetError(a.msgs.ErrNoCompletion)
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
	a.root.ResizeItem(a.completionList, height+2, 0) // +2: top/bottom border
	a.tapp.SetFocus(a.completionList)
}

func (a *App) closeCompletion() {
	a.root.ResizeItem(a.completionList, 0, 0)
	a.focusEditor()
}

// handleSave implements Ctrl+S, whose behavior depends on which panel has
// focus (SPEC.md §3.2, §3.3, §4): saves the requests from the left,
// exports the result from the right.
func (a *App) handleSave() {
	if a.focusedIsEditor {
		a.saveQueries()
	} else {
		a.exportResult()
	}
}

func (a *App) saveQueries() {
	if err := a.editor.SaveToFile(a.queriesPath); err != nil {
		a.status.SetError(fmt.Sprintf(a.msgs.ErrSaveFailedFmt, err))
		return
	}
	a.status.SetInfo(fmt.Sprintf(a.msgs.InfoSavedFmt, a.queriesPath))
}

// SaveQueriesOnExit silently saves (best-effort, no user feedback possible
// at this point) the editor content before the program closes — Ctrl+C or
// an external signal (SIGTERM/SIGHUP, see main.go). Complements the
// explicit Ctrl+S save. See SPEC.md §3.2.
func (a *App) SaveQueriesOnExit() {
	if a.queriesPath == "" {
		return
	}
	_ = a.editor.SaveToFile(a.queriesPath)
}

func (a *App) exportResult() {
	path, err := a.result.Export(a.exportsDir)
	if err != nil {
		a.status.SetError(fmt.Sprintf(a.msgs.ErrExportFailedFmt, err))
		return
	}
	a.status.SetInfo(fmt.Sprintf(a.msgs.InfoExportedFmt, path))
}

// copyResult implements F2: copies the entire displayed result to the local
// clipboard via OSC 52 (SPEC.md §3.3) — needed because EnableMouse(true)
// prevents native terminal selection in this panel. Best-effort: neither
// tcell nor OSC 52 return confirmation that the copy actually succeeded on
// the terminal side.
func (a *App) copyResult() {
	text := a.result.PlainText()
	if text == "" {
		a.status.SetError(a.msgs.ErrNothingToCopy)
		return
	}
	if a.screen != nil {
		a.screen.SetClipboard([]byte(text))
	}
	a.status.SetInfo(a.msgs.InfoCopied)
}
