package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/config"
	"termdevtools/esclient"
	"termdevtools/i18n"
	"termdevtools/parser"
	"termdevtools/refdata"
)

// CheatsheetFileName is the optional file loaded into the editor by default
// on startup, located next to the binary. See SPEC.md §9.1.
const CheatsheetFileName = "cheatsheet.txt"

// ExportsDirName is the subfolder (of the user's configuration directory)
// where Ctrl+S exports the result displayed in the right panel. See SPEC.md
// §3.3 and §9.1.
const ExportsDirName = "exports"

// Paths gathers the file locations resolved by the caller (main.go) — see
// SPEC.md §9.1 for the detail of each.
type Paths struct {
	// Cheatsheet is an optional file next to the binary: a team's own
	// default editor content, which takes precedence over Starter.
	Cheatsheet string
	// Starter is the editor's default content the first time a cluster is
	// connected to, when nothing is saved for it yet and there is no
	// Cheatsheet file. Empty: the editor starts empty.
	Starter string
	// Exports is the directory, created on the first export, that Ctrl+S
	// writes the displayed result to: the user's own, so that exporting
	// doesn't depend on the binary's directory being writable.
	Exports string
	// Reference locates the optional files extending the reference data
	// built into the binary: the team's (next to the binary) and the
	// user's (configuration directory).
	Reference refdata.Sources
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
	tapp          *tview.Application
	client        *esclient.Client
	cfg           *config.Config
	timeout       time.Duration
	exportsDir    string
	queriesPath   string
	variablesPath string
	variables     map[string]string
	endpoints     []string
	// catColumns holds the _cat commands known for the connected cluster
	// (its keys: what makes a typed path recognizable as a _cat command)
	// with the columns built into the binary for each — a fallback only.
	catColumns map[string][]string
	// catLive holds, per _cat command, the columns completion actually
	// offers: those the cluster itself reported ("?help", asked the first
	// time a command's columns are completed), or the built-in fallback if
	// it couldn't be asked. catPending marks requests still in flight.
	catLive    map[string][]string
	catPending map[string]bool
	// recipes are the ready-made requests that apply to the connected
	// cluster, in the order the palette (F8) lists them.
	recipes []refdata.Recipe
	msgs    *i18n.Strings
	// target is the connected cluster's distribution and version (detected
	// at connection, see ConnectResult.Target): what reference data is
	// selected for.
	target refdata.Target
	// reference is where the team's and user's reference files live;
	// kept to reload them on demand (F7).
	reference refdata.Sources

	editor *Editor
	result *ResultView
	status *StatusBar

	root       *tview.Flex
	mainFlex   *tview.Flex
	searchBar  *tview.InputField
	leftWeight int

	completionList        *tview.List
	completionStart       int
	completionEnd         int
	completionTypedPrefix string
	completionTypeahead   string

	pages       *tview.Pages
	helpView    *tview.TextView
	helpVisible bool

	palette        *recipePalette
	recipesVisible bool

	// screen is used for clipboard copy (F2, OSC 52, see SPEC.md §3.3);
	// captured on the first render via SetAfterDrawFunc (tview.Application
	// has no direct accessor to the screen it creates itself).
	screen tcell.Screen

	// focusedIsEditor tells which of the two panels holds — or held, while
	// a popup or the search bar has it — the focus. Kept up to date by the
	// panels themselves (see NewApp), so that a mouse click counts too.
	focusedIsEditor  bool
	searchTarget     string // "editor" or "result"
	editorSearchPos  int
	resultSearchLine int

	// execution numbers the requests sent (Ctrl+E): only the answer to the
	// latest one is displayed.
	execution int

	// queriesLoadFailed is set when this cluster's saved requests exist but
	// couldn't be read at startup: the editor doesn't hold them, and saving
	// it on exit would replace them with whatever it holds instead. Cleared
	// by an explicit save (Ctrl+S), the user's own decision.
	queriesLoadFailed bool
	// exitWarned is set once quitting has been refused because the left
	// panel couldn't be saved: the next Ctrl+C quits regardless.
	exitWarned bool
}

// NewApp builds the main screen for an already-established connection.
// paths is resolved by the caller (main.go) — see SPEC.md §9.1 for the
// detail of the locations.
func NewApp(tapp *tview.Application, cr ConnectResult, cfg *config.Config, paths Paths) *App {
	msgs := i18n.For(cfg.Language)
	a := &App{
		tapp:             tapp,
		client:           cr.Client,
		cfg:              cfg,
		timeout:          time.Duration(cfg.DefaultTimeoutSeconds) * time.Second,
		exportsDir:       paths.Exports,
		msgs:             msgs,
		editor:           NewEditor(msgs),
		result:           NewResultView(msgs),
		status:           NewStatusBar(cr.Target.Label(), cr.Cluster.URL, cr.DisplayUser, msgs),
		target:           cr.Target,
		focusedIsEditor:  true,
		editorSearchPos:  -1,
		resultSearchLine: -1,
		leftWeight:       splitTotalWeight / 2,
	}
	// First, so that any load error reported below takes precedence over it.
	if cr.Warning != "" {
		a.status.SetWarning(cr.Warning)
	}

	queriesPath, err := config.QueriesPathForURL(cr.Cluster.URL)
	if err != nil {
		a.status.SetError(err.Error())
	}
	a.queriesPath = queriesPath

	variablesPath, err := config.VariablesPathForURL(cr.Cluster.URL)
	if err != nil {
		a.status.SetError(err.Error())
	}
	a.variablesPath = variablesPath
	if variablesPath != "" {
		if vars, err := LoadVariablesFile(variablesPath); err != nil {
			a.status.SetError(fmt.Sprintf(msgs.ErrLoadFailedFmt, variablesPath, err))
		} else {
			a.variables = vars
		}
	}

	a.reference = paths.Reference
	a.loadReferenceData()

	a.loadInitialQueries(paths.Cheatsheet, paths.Starter)

	// Which panel has the focus is learned from the panels themselves rather
	// than from the shortcuts that move it: with mouse support on, a click
	// moves it too, and Ctrl+S, Ctrl+F or Ctrl+E would otherwise act on the
	// panel the keyboard last chose.
	a.editor.OnFocus(func() { a.focusedIsEditor = true })
	a.result.OnFocus(func() { a.focusedIsEditor = false })

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
		case tcell.KeyRune:
			a.typeaheadCompletion(event.Rune())
			return nil
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			a.typeaheadCompletionBackspace()
			return nil
		}
		return event
	})
	// However the list loses the focus — closed, or a mouse click elsewhere —
	// it goes away: left on screen, a later click on one of its items would
	// apply offsets computed for a text that has changed since.
	a.completionList.SetBlurFunc(func() {
		a.completionTypeahead = ""
		a.root.ResizeItem(a.completionList, 0, 0)
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
	// short terminal. The width, on the other hand, is fixed rather than
	// proportional (content reads better at a stable line length than
	// stretched to a wide terminal) — capped at 76 to stay clear of the
	// classic 80-column terminal floor once the side margins and border are
	// accounted for; going wider clips/corrupts the display on anything at
	// or below that width (confirmed via the 80-column simulated screen the
	// e2e tests use).
	helpOverlay := modalPage{tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(a.helpView, 76, 0, true).
			AddItem(nil, 0, 1, false),
			0, 9, true).
		AddItem(nil, 0, 1, false)}

	a.palette = newRecipePalette(a)

	a.pages = tview.NewPages().
		AddPage("main", a.root, true, true).
		AddPage("help", helpOverlay, true, false).
		AddPage(recipesPageName, a.palette.overlay(), true, false)

	return a
}

// loadReferenceData (re)builds what completion offers from the reference
// data — built into the binary, extended by the team's and user's files —
// keeping only what applies to the connected cluster. Returns the catalog's
// load problems, already reported in the status bar (the first one, with a
// count of the others), for callers that want to say more.
func (a *App) loadReferenceData() []refdata.Problem {
	catalog := refdata.Load(a.reference)
	a.endpoints = catalog.Endpoints(a.target)
	a.catColumns = catalog.CatColumns(a.target)
	// A _cat endpoint offered by completion is a known command even when
	// the built-in table has no column for it (one the user added, or a
	// version between two of those the table was built from): its columns
	// then come from the cluster alone.
	for _, endpoint := range a.endpoints {
		if cmd, isCat := strings.CutPrefix(endpoint, "_cat/"); isCat {
			cmd, _, _ = strings.Cut(cmd, "?")
			if _, known := a.catColumns[cmd]; !known && cmd != "" {
				a.catColumns[cmd] = nil
			}
		}
	}
	// What the cluster reported is asked again after a reload.
	a.catLive = make(map[string][]string)
	a.catPending = make(map[string]bool)
	a.recipes = catalog.Recipes(a.target)

	if n := len(catalog.Problems); n > 0 {
		message := catalog.Problems[0].String()
		if n > 1 {
			message = fmt.Sprintf(a.msgs.WarnMoreProblemsFmt, message, n-1)
		}
		a.status.SetWarning(message)
	}
	return catalog.Problems
}

// loadInitialQueries loads the personal save specific to this cluster
// (a.queriesPath, Ctrl+S or automatic save on program exit) if it exists;
// otherwise the team's cheatsheet file if there is one; otherwise the
// built-in starter. See SPEC.md §3.2 and §9.1.
func (a *App) loadInitialQueries(cheatsheetPath, starter string) {
	if a.queriesPath != "" {
		loaded, err := a.editor.LoadFile(a.queriesPath)
		if err != nil {
			a.queriesLoadFailed = true
			a.status.SetError(fmt.Sprintf(a.msgs.ErrLoadFailedFmt, a.queriesPath, err))
			return
		}
		if loaded {
			return
		}
	}

	loaded, err := a.editor.LoadFile(cheatsheetPath)
	if err != nil {
		a.status.SetError(fmt.Sprintf(a.msgs.ErrLoadFailedFmt, cheatsheetPath, err))
		return
	}
	if !loaded && starter != "" {
		a.editor.SetInitialText(starter)
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
	if event.Key() == tcell.KeyCtrlC {
		// The only exit shortcut: Ctrl+Esc turned out to be intercepted by
		// Windows itself (it opens the Start menu), so it was dropped.
		// Ctrl+C stays universally available at the terminal level
		// and must never be able to lock the user out. Handled before
		// anything else: left to a popup (help, recipes, search, completion),
		// it would reach tview's own default Ctrl+C handling, which stops the
		// application without saving the left panel.
		if err := a.SaveQueriesOnExit(); err != nil && !a.exitWarned {
			// Quitting now would lose what the left panel holds: say so
			// once, the next Ctrl+C quits regardless.
			a.exitWarned = true
			a.status.SetError(fmt.Sprintf(a.msgs.ErrExitSaveFailedFmt, err))
			return nil
		}
		a.tapp.Stop()
		return nil
	}
	if a.helpVisible {
		switch event.Key() {
		case tcell.KeyEscape:
			a.closeHelp()
			return nil
		case tcell.KeyF3:
			// Switching language while help is open re-renders it in place
			// (see toggleLanguage/applyLanguage) — useful to see the effect
			// immediately, since the help screen is the shortcuts reference.
			a.toggleLanguage()
			return nil
		}
		return event // let the help TextView scroll if content overflows
	}
	if a.recipesVisible {
		if event.Key() == tcell.KeyEscape {
			// The palette's filter field closes it too, but a mouse click
			// may have taken the focus elsewhere in the popup.
			a.closeRecipes()
			return nil
		}
		return event // the recipe palette handles its other keys (recipes.go)
	}
	// HasFocus rather than comparing with the application's focus: after a
	// mouse click, that is an inner part of the widget, not the widget.
	if a.searchBar.HasFocus() {
		return event // the search bar handles Enter/Escape itself
	}
	if a.completionList.HasFocus() {
		return event // the completion list handles Enter/Escape/Tab itself
	}

	switch {
	case isExecuteShortcut(event):
		if a.focusedIsEditor {
			a.executeCurrent()
		}
		return nil
	case isShrinkShortcut(event):
		a.resizeSplit(-splitStep)
		return nil
	case isGrowShortcut(event):
		a.resizeSplit(splitStep)
		return nil
	case isFocusLeftShortcut(event):
		a.focusEditor()
		return nil
	case isFocusRightShortcut(event):
		a.focusResultPanel()
		return nil
	case event.Key() == tcell.KeyCtrlF:
		a.openSearch()
		return nil
	case event.Key() == tcell.KeyCtrlS:
		a.handleSave()
		return nil
	case isCompletionShortcut(event) && a.focusedIsEditor:
		if a.tryCompletion() {
			return nil
		}
		if event.Key() == tcell.KeyF10 {
			// Unlike Tab, F10 has no "insert a literal character" fallback
			// meaning outside a completion context — just swallow it.
			return nil
		}
		return event // outside a completion context: Tab inserts a tab, standard behavior
	case event.Key() == tcell.KeyF1:
		a.showHelp()
		return nil
	case event.Key() == tcell.KeyF2:
		a.copyResult()
		return nil
	case event.Key() == tcell.KeyF3:
		a.toggleLanguage()
		return nil
	case event.Key() == tcell.KeyF4:
		if a.focusedIsEditor {
			a.reformatBody()
		}
		return nil
	case event.Key() == tcell.KeyF9:
		if a.focusedIsEditor {
			a.copyAsCurl()
		}
		return nil
	case event.Key() == tcell.KeyF7:
		// Not gated on focusedIsEditor, unlike F4/F9: this reloads
		// background data sources, it doesn't act on whatever request
		// happens to be under the cursor.
		a.reloadUserFiles()
		return nil
	case event.Key() == tcell.KeyF8:
		a.openRecipes()
		return nil
	}
	return event
}

// isExecuteShortcut reports whether event should trigger request execution.
// Ctrl+E is the primary, guaranteed-reliable trigger: a raw control byte,
// just like Ctrl+F/Ctrl+S, with no modifier-flag ambiguity whatsoever.
// Ctrl+Enter (and, on macOS, Option/Alt+Enter — see hasShortcutModifier) is
// kept as a best-effort alternative for terminals that do report it, but
// cannot be relied on in general: confirmed on a real macOS terminal (by
// dumping its raw key events) that Enter is reported identically — same Key, same zero
// Modifiers — whether or not Ctrl, Option, or Alt is held. Ctrl+M *is*
// Enter's control byte; some terminals just never attach modifier
// information to it at all, for any modifier.
func isExecuteShortcut(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyCtrlE {
		return true
	}
	return event.Key() == tcell.KeyEnter && hasShortcutModifier(event)
}

// isCompletionShortcut reports whether event should trigger endpoint/column
// completion (SPEC.md §3.2). Tab is the natural, expected trigger and stays
// the primary one — but it's a plain, unmodified single key, exactly the
// category confirmed reliable in every terminal tested so far (unlike
// Ctrl/Shift/Option combinations), so its failure to reach the app at all
// on a real, unrelated pair of terminals (Windows cmd.exe and PuTTY, both
// on the same machine) isn't a decodable encoding quirk to work around —
// something ahead of the app (terminal or OS) is swallowing Tab itself,
// most likely for its own focus-navigation purposes. F10 is added as a
// guaranteed-reliable alternative for exactly that case. Ctrl+Space is a
// third option, offered mainly for muscle memory (the usual IDE/editor
// completion shortcut) — no particular terminal-reliability edge over F10.
func isCompletionShortcut(event *tcell.EventKey) bool {
	return event.Key() == tcell.KeyTab || event.Key() == tcell.KeyF10 || event.Key() == tcell.KeyCtrlSpace
}

// hasShortcutModifier reports whether event carries the modifier used to
// trigger the app's Ctrl-style shortcuts (execute, focus switch, resize):
// Ctrl, or Option/Alt as an additional accepted alternative on top of it.
// Added for macOS, where Ctrl+←/→ is intercepted at the OS level by default
// (Mission Control desktop switching) and Ctrl+Enter can't even be
// distinguished from plain Enter in classic terminal encoding — Ctrl+M
// *is* Enter's control byte. Deliberately NOT extended to Ctrl+F/Ctrl+S/
// Ctrl+C: those are raw, universally reliable control bytes with no such
// conflict, and by default macOS terminals turn Option+letter into an
// accented character instead of signaling a modifier at all.
func hasShortcutModifier(event *tcell.EventKey) bool {
	return event.Modifiers()&tcell.ModCtrl != 0 || event.Modifiers()&tcell.ModAlt != 0
}

// isShrinkShortcut/isGrowShortcut detect the resize-split shortcuts
// (SPEC.md §4): F5 (shrink the left panel) / F6 (grow it) are the primary,
// guaranteed-reliable triggers — plain function keys, no modifier encoding
// involved at all, in the same spirit as Ctrl+E for execute. Ctrl+Shift+←/→
// (or Option/Alt+Shift+←/→, via hasShortcutModifier) is kept as a
// best-effort alternative, but confirmed unreliable on at least one real
// macOS terminal: Shift+Alt+←/→ arrives there as a *plain* KeyLeft/KeyRight
// with zero modifiers (seen in a dump of its raw key events) — identical to an unmodified arrow
// key press, with no way to tell the two apart at the key-event level, so
// it can never be relied on there. These cases are tested BEFORE the plain
// Ctrl/Option+←/→ case (focus switch) so Ctrl+Shift+←/→ isn't shadowed by it.
func isShrinkShortcut(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyF5 {
		return true
	}
	return event.Key() == tcell.KeyLeft && hasShortcutModifier(event) && event.Modifiers()&tcell.ModShift != 0
}

func isGrowShortcut(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyF6 {
		return true
	}
	return event.Key() == tcell.KeyRight && hasShortcutModifier(event) && event.Modifiers()&tcell.ModShift != 0
}

// isAltWordBack/isAltWordForward detect the classic Meta-b / Meta-f
// word-navigation encoding (ESC b / ESC f) — confirmed, by dumping the raw
// key events of a real macOS terminal, to be what that terminal actually sends for
// Option/Alt+Left and Option/Alt+Right, instead of a modified arrow-key
// sequence: Key ends up KeyRune (not KeyLeft/KeyRight at all), with Rune
// 'b'/'f' and only ModAlt set. Without this, hasShortcutModifier's
// Ctrl/Option fallback for arrow keys never fires on that terminal — the
// event it produces doesn't even look like an arrow key to begin with. The
// tradeoff: literally typing Option+B or Option+F (not the arrow keys) in
// the editor is swallowed as a focus switch instead of inserting a
// character — accepted as unlikely in practice, the same kind of tradeoff
// already made for Ctrl+Enter vs plain Enter.
func isAltWordBack(event *tcell.EventKey) bool {
	return event.Key() == tcell.KeyRune && event.Rune() == 'b' && event.Modifiers()&tcell.ModAlt != 0
}

func isAltWordForward(event *tcell.EventKey) bool {
	return event.Key() == tcell.KeyRune && event.Rune() == 'f' && event.Modifiers()&tcell.ModAlt != 0
}

// isFocusLeftShortcut/isFocusRightShortcut detect Ctrl/Option+←/→ (focus
// switch, SPEC.md §4), accepting either encoding a terminal might use for
// Option/Alt+arrow: a modified KeyLeft/KeyRight (hasShortcutModifier), or
// the Meta-b/Meta-f rune encoding above.
func isFocusLeftShortcut(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyLeft && hasShortcutModifier(event) {
		return true
	}
	return isAltWordBack(event)
}

func isFocusRightShortcut(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyRight && hasShortcutModifier(event) {
		return true
	}
	return isAltWordForward(event)
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

// openRecipes shows the recipe palette (F8, SPEC.md §3.2) over the current
// layout.
func (a *App) openRecipes() {
	a.recipesVisible = true
	a.palette.open()
	a.pages.ShowPage(recipesPageName)
	a.tapp.SetFocus(a.palette.filter)
}

func (a *App) closeRecipes() {
	a.recipesVisible = false
	a.pages.HidePage(recipesPageName)
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
	path, body, err := a.resolveRequest(req)
	if err != nil {
		a.status.SetError(err.Error())
		return
	}
	if err := parser.ValidateBody(body); err != nil {
		a.status.SetError(err.Error())
		return
	}

	a.status.SetRunning()
	method := req.Method
	a.execution++
	execution := a.execution

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
		defer cancel()
		result, err := a.client.Execute(ctx, method, path, body)
		// Indenting and colorizing a large response takes time: done here,
		// not in the interface's goroutine, which it would freeze.
		var rendered renderedResult
		if err == nil {
			rendered = renderResult(method, path, result.Headers, result.Body)
		}

		a.tapp.QueueUpdateDraw(func() {
			if execution != a.execution {
				// Another request was sent since: its answer is the one
				// expected, whichever arrives first.
				return
			}
			// A new result: its search starts from the top again.
			a.resultSearchLine = -1
			if err != nil {
				a.status.SetError(err.Error())
				a.result.ShowError(method, path, err.Error())
				return
			}
			a.status.SetResult(result.StatusCode, result.Duration)
			a.result.display(rendered)
		})
	}()
}

// reformatBody implements F4 (SPEC.md §7 backlog #1, Kibana's "auto
// indent"): re-indents the JSON body of the request under the cursor, in
// place. Validated the same way as Ctrl+E (parser.ValidateBody) so the
// error shown for genuinely malformed JSON matches what execution would
// have reported.
func (a *App) reformatBody() {
	req, err := parser.RequestAtLine(a.editor.Text(), a.editor.CursorLine())
	if err != nil {
		a.status.SetError(err.Error())
		return
	}
	if len(req.Body) == 0 {
		a.status.SetError(a.msgs.ErrNoBodyToFormat)
		return
	}
	if err := parser.ValidateBody(req.Body); err != nil {
		a.status.SetError(err.Error())
		return
	}
	if a.editor.ReformatBody(req.StartLine+1, req.EndLine) {
		a.status.SetIdle()
		return
	}
	// Nothing changed: the body is already indented — or the lines it spans
	// aren't JSON as they stand. The parser skips "#" lines between the
	// request line and the body, or inside it, which is why the body was
	// found valid above; re-indenting can't put them back in place.
	lines := strings.Split(a.editor.Text(), "\n")
	if req.EndLine < len(lines) && !json.Valid([]byte(strings.Join(lines[req.StartLine+1:req.EndLine+1], "\n"))) {
		a.status.SetError(a.msgs.ErrFormatCommentsInBody)
	}
}

// copyAsCurl implements F9 (SPEC.md §7 backlog #3, "copy as cURL"): copies
// an equivalent curl command for the request under the cursor to the
// clipboard (OSC 52, same mechanism as F2/copyResult) — handy to replicate
// a call outside the tool (a script, a colleague, a bug report). See
// esclient.Client.CurlCommand for why the actual secret (password, API key
// secret, key passphrase) is replaced with a placeholder rather than
// included as-is.
func (a *App) copyAsCurl() {
	req, err := parser.RequestAtLine(a.editor.Text(), a.editor.CursorLine())
	if err != nil {
		a.status.SetError(err.Error())
		return
	}
	path, body, err := a.resolveRequest(req)
	if err != nil {
		a.status.SetError(err.Error())
		return
	}
	cmd := a.client.CurlCommand(req.Method, path, body)
	if a.screen != nil {
		a.screen.SetClipboard([]byte(cmd))
	}
	a.status.SetInfo(a.msgs.InfoCurlCopied)
}

// resolveRequest applies variable substitution (${name}, SPEC.md §7
// backlog #4) to req's path and body — shared by executeCurrent and
// copyAsCurl, the two "what would actually be sent" operations.
// reformatBody deliberately does NOT go through this: it edits the saved
// query text itself, which must keep ${name} literal, not the resolved
// value, or the placeholder would be lost from the editor for good.
//
// Returns an error naming every undefined variable referenced (deduplicated
// across path and body) instead of silently sending a request with literal
// "${name}" text still in it.
func (a *App) resolveRequest(req *parser.Request) (path string, body []byte, err error) {
	path, missingPath := substituteVariables(req.Path, a.variables)
	bodyText, missingBody := substituteVariables(string(req.Body), a.variables)

	seen := make(map[string]bool)
	var missing []string
	for _, name := range append(missingPath, missingBody...) {
		if !seen[name] {
			seen[name] = true
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return "", nil, fmt.Errorf(a.msgs.ErrUnknownVariablesFmt, strings.Join(missing, ", "))
	}
	return path, []byte(bodyText), nil
}

// reloadUserFiles implements F7: re-reads from disk everything the user (or
// the team) maintains by hand — the current cluster's variables (SPEC.md
// §3.2), and the recipes and endpoints that extend the built-in ones
// (§9.1). None of these files has an in-app editor and none is watched, so
// this is how a change made in an external editor while the app is running
// takes effect without reconnecting. _cat columns are asked from the
// cluster again too.
func (a *App) reloadUserFiles() {
	if a.variablesPath != "" {
		vars, err := LoadVariablesFile(a.variablesPath)
		if err != nil {
			a.status.SetError(fmt.Sprintf(a.msgs.ErrLoadFailedFmt, a.variablesPath, err))
			return
		}
		a.variables = vars
	}

	// A problem in a reference file is already in the status bar: leave it
	// there rather than cover it with the summary.
	if problems := a.loadReferenceData(); len(problems) == 0 {
		a.status.SetInfo(fmt.Sprintf(a.msgs.InfoReloadedFmt, len(a.variables), len(a.recipes), len(a.endpoints)))
	}
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

	if edit, ok := parseCatEdit(prefix, a.catColumns); ok {
		// A sort direction (asc/desc) needs no column list; a column does,
		// and the cluster is asked for it the first time.
		table := a.catColumns
		if !edit.direction {
			if _, asked := a.catLive[edit.command]; !asked {
				// Completion resumes by itself once the cluster has answered.
				a.fetchCatColumns(edit.command)
				return true
			}
			table = a.catLive
		}
		if candidates, subLen, ok := catColumnCompletion(prefix, table); ok {
			a.offerCompletions(end-subLen, end, candidates)
			return true
		}
		// No column known at all for this command: complete it as a
		// regular endpoint.
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

// catHelpTimeout bounds the wait for a _cat command's column list: the
// request is answered by the receiving node alone, without touching the
// cluster, so anything slower than this means the node is in trouble and
// the built-in table is the better answer.
const catHelpTimeout = 3 * time.Second

// fetchCatColumns asks the cluster which columns a _cat command has ("GET
// _cat/<command>?help") — exact for whatever version it runs, unlike the
// built-in table — then resumes the completion that needed them, provided
// the editor hasn't changed in the meantime. If the cluster can't be asked,
// the built-in table is used instead, for the rest of the session (until a
// reload, F7). Asynchronous: the interface stays responsive however slow
// the cluster is.
func (a *App) fetchCatColumns(command string) {
	if a.catPending[command] {
		return
	}
	a.catPending[command] = true
	a.status.SetRunning()
	running := a.status.Version()
	text, cursor := a.editor.Text(), a.editor.CursorOffset()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), catHelpTimeout)
		defer cancel()
		result, err := a.client.Execute(ctx, "GET", "_cat/"+command+"?help", nil)

		a.tapp.QueueUpdateDraw(func() {
			delete(a.catPending, command)

			var columns []string
			if err == nil && result.StatusCode == 200 {
				columns = refdata.ParseCatHelp(result.Body)
			}
			fromCluster := len(columns) > 0
			if !fromCluster {
				columns = a.catColumns[command]
			}
			a.catLive[command] = columns

			if !a.editor.Primitive().HasFocus() || a.editor.Text() != text || a.editor.CursorOffset() != cursor {
				// The user moved on while waiting: nothing to complete
				// anymore, but the columns are there for next time. The
				// status line is only cleared if it still says "running"
				// for this request — not if it now shows, say, the result
				// of a request executed in the meantime.
				if a.status.Version() == running {
					a.status.SetIdle()
				}
				return
			}
			a.tryCompletion()
			if !fromCluster {
				a.status.SetWarning(a.msgs.WarnCatColumnsBuiltIn)
			}
		})
	}()
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
		// Clear any earlier error (e.g. a previous Tab/F10 press that found
		// no completion) before showing the list — otherwise it stays
		// displayed, in red, behind/around a completion that did work.
		a.status.SetIdle()
		a.openCompletion(start, end, matches)
	}
}

func (a *App) openCompletion(start, end int, matches []string) {
	a.completionStart, a.completionEnd = start, end
	// The already-typed text that produced matches (e.g. "_cat/s") — every
	// item in the list starts with it, so further typeahead keystrokes are
	// matched against this prefix plus what's typed next, not against the
	// bare keystrokes alone (see applyCompletionTypeahead).
	a.completionTypedPrefix = a.editor.Text()[start:end]
	a.completionTypeahead = ""
	a.updateCompletionTitle()

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
	a.completionTypeahead = ""
	a.root.ResizeItem(a.completionList, 0, 0)
	a.focusEditor()
}

// typeaheadCompletion appends r to the completion list's type-ahead search
// buffer and jumps to the first item whose text starts with the buffer,
// case-insensitively — lets you narrow down a long suggestion list by
// typing instead of scrolling. The buffer only resets when the list itself
// is opened or closed (see openCompletion/closeCompletion), never on its
// own after a pause: an earlier version reset it after 700ms of inactivity,
// which silently dropped already-typed characters (e.g. "/") whenever
// typing paused even briefly — confusingly timing-dependent, since
// Backspace already covers deliberate correction.
func (a *App) typeaheadCompletion(r rune) {
	a.completionTypeahead += string(r)
	a.applyCompletionTypeahead()
}

// typeaheadCompletionBackspace removes the last rune from the type-ahead
// buffer and re-applies it, letting a mistyped prefix be corrected without
// closing and reopening the list.
func (a *App) typeaheadCompletionBackspace() {
	runes := []rune(a.completionTypeahead)
	if len(runes) == 0 {
		return
	}
	a.completionTypeahead = string(runes[:len(runes)-1])
	a.applyCompletionTypeahead()
}

// applyCompletionTypeahead selects the first item in the completion list
// whose text starts with completionTypedPrefix+completionTypeahead
// (case-insensitive) — every item already starts with completionTypedPrefix
// (the text typed before the list opened), so this matches from the
// beginning of the item, not just against the newly typed keystrokes. Also
// refreshes the list's title to show that combined search text (see
// updateCompletionTitle): typing "i" right after Tab on "GET _cat" searches
// for "_cati" (no matching item, since every candidate has a "/" there,
// e.g. "_cat/indices?v") — showing the search text makes that visible
// instead of the list silently not reacting to the keystroke.
func (a *App) applyCompletionTypeahead() {
	a.updateCompletionTitle()
	if a.completionTypeahead == "" {
		return
	}
	lower := strings.ToLower(a.completionTypedPrefix + a.completionTypeahead)
	for i := 0; i < a.completionList.GetItemCount(); i++ {
		main, _ := a.completionList.GetItemText(i)
		if strings.HasPrefix(strings.ToLower(main), lower) {
			a.completionList.SetCurrentItem(i)
			return
		}
	}
}

// updateCompletionTitle refreshes the completion list's border title to
// show the text currently being searched for: what was already typed
// before the list opened (completionTypedPrefix) plus any type-ahead
// keystrokes since (completionTypeahead).
func (a *App) updateCompletionTitle() {
	search := "[" + a.completionTypedPrefix + a.completionTypeahead + "]"
	// A title is parsed for style tags: "[st]" — letters only, as when
	// completing a _cat column — would be taken for one and vanish. Escaped
	// when, and only when, tview would read it that way: it then counts for
	// nothing in the title's width.
	if escaped := tview.Escape(search); tview.TaggedStringWidth(search) != tview.TaggedStringWidth(escaped) {
		search = escaped
	}
	a.completionList.SetTitle(a.msgs.CompletionTitle + search)
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
	// The file now holds what the user chose to put in it.
	a.queriesLoadFailed = false
	a.status.SetInfo(fmt.Sprintf(a.msgs.InfoSavedFmt, a.queriesPath))
}

// SaveQueriesOnExit saves the editor content before the program closes —
// Ctrl+C, or an external signal through SaveQueriesOnSignal. Complements the
// explicit Ctrl+S save. See SPEC.md §3.2.
//
// Returns an error if the content couldn't be saved, including the case
// where it deliberately wasn't: the saved requests couldn't be read at
// startup (queriesLoadFailed), and writing the editor over them would
// destroy them — unless there is nothing in the editor to lose either.
func (a *App) SaveQueriesOnExit() error {
	if a.queriesPath == "" {
		return nil
	}
	if a.queriesLoadFailed {
		if strings.TrimSpace(a.editor.Text()) == "" {
			return nil
		}
		return errors.New(a.msgs.ErrSavedRequestsUnread)
	}
	return a.editor.SaveToFile(a.queriesPath)
}

// signalSaveTimeout bounds how long SaveQueriesOnSignal waits for the
// interface's goroutine before saving without it.
const signalSaveTimeout = 2 * time.Second

// SaveQueriesOnSignal is SaveQueriesOnExit for a caller outside the
// interface's goroutine — the handler of SIGTERM/SIGHUP in main.go. The
// editor may only be read from that goroutine, so the save is handed to it;
// if it doesn't get to it in time (stuck rendering something large), the
// editor is read from here after all: a risk worth taking over losing the
// session's requests. Best-effort and silent, the program is going away.
func (a *App) SaveQueriesOnSignal() {
	done := make(chan struct{})
	go func() {
		a.tapp.QueueUpdate(func() { _ = a.SaveQueriesOnExit() })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(signalSaveTimeout):
		_ = a.SaveQueriesOnExit()
	}
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
// clipboard via OSC 52 (SPEC.md §3.3) — the only way to copy from this
// panel when config.Mouse is enabled, since that captures mouse events for
// the app instead of letting the terminal handle native text selection.
// Available either way, mouse or not. Best-effort: neither tcell nor OSC 52
// return confirmation that the copy actually succeeded on the terminal side.
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

// toggleLanguage implements F3: flips the interface language (fr/en),
// persists the choice to config.yaml, and re-renders the already-built
// widgets in place — see applyLanguage. No screen/layout reconstruction is
// needed: every panel already reads its chrome (titles, labels, help text)
// from a.msgs, so switching just means pointing them at the other catalog
// and re-applying it, the same calls each made once at construction time.
func (a *App) toggleLanguage() {
	next := i18n.EN
	if i18n.Normalize(a.cfg.Language) == i18n.EN {
		next = i18n.FR
	}
	a.cfg.Language = next
	a.msgs = i18n.For(next)
	a.applyLanguage()

	if err := a.cfg.Save(); err != nil {
		a.status.SetError(fmt.Sprintf(a.msgs.ErrSaveFailedFmt, err))
		return
	}
	a.status.SetInfo(fmt.Sprintf(a.msgs.InfoLanguageSwitchedFmt, a.msgs.LanguageName))
}

// applyLanguage re-applies a.msgs to every widget built once in NewApp —
// titles, labels, and the help screen's content. The editor's and result
// panel's actual content (requests being edited, last response shown) is
// left untouched.
func (a *App) applyLanguage() {
	a.editor.SetLanguage(a.msgs)
	a.result.SetLanguage(a.msgs)
	a.status.SetLanguage(a.msgs)
	a.searchBar.SetLabel(a.msgs.SearchLabel)
	a.updateCompletionTitle()
	a.helpView.SetTitle(a.msgs.HelpViewTitle)
	a.helpView.SetText(a.msgs.HelpContent)
	a.palette.applyLanguage(a.msgs)
}
