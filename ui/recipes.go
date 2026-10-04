package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/i18n"
	"termdevtools/refdata"
)

// recipesPageName is the tview.Pages key of the recipe palette, shown over
// the main layout like the help popup.
const recipesPageName = "recipes"

// recipePalette is the F8 popup (SPEC.md §3.2): the catalog of ready-made
// requests that apply to the connected cluster, narrowed down by typing,
// with a preview of the one selected; Enter inserts it into the editor.
//
// Focus stays on the filter field the whole time: typing filters, the
// arrow keys move the selection in the list below it.
type recipePalette struct {
	app *App

	frame   *tview.Flex
	filter  *tview.InputField
	list    *tview.List
	preview *tview.TextView

	// shown are the recipes currently listed, in list order.
	shown []refdata.Recipe
}

func newRecipePalette(a *App) *recipePalette {
	p := &recipePalette{
		app:     a,
		filter:  tview.NewInputField(),
		list:    tview.NewList().ShowSecondaryText(false),
		preview: tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(false),
	}
	p.preview.SetBorder(true)

	p.filter.SetChangedFunc(func(string) { p.refresh() })
	p.filter.SetInputCapture(p.handleKey)
	// The index is taken from the callback: tview calls it before the list's
	// own current item is updated.
	p.list.SetChangedFunc(func(index int, _, _ string, _ rune) { p.showPreviewOf(index) })
	// Only reachable with the mouse (the keyboard never leaves the filter
	// field): a click on a recipe inserts it — the one clicked, hence the
	// callback's index again.
	p.list.SetSelectedFunc(func(index int, _, _ string, _ rune) { p.insert(index) })
	p.list.SetDoneFunc(func() { a.closeRecipes() })
	// A click on the preview would leave the keyboard there, with nothing to
	// type into: hand it back to the filter field.
	p.preview.SetFocusFunc(func() { a.tapp.SetFocus(p.filter) })

	p.frame = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.filter, 1, 0, true).
		AddItem(p.list, 0, 1, false).
		AddItem(p.preview, 0, 1, false)
	p.frame.SetBorder(true)

	p.applyLanguage(a.msgs)
	return p
}

// overlay centers the palette over the main layout — same idiom and same
// 76-column width as the help popup (see NewApp), for the same reason.
func (p *recipePalette) overlay() tview.Primitive {
	return modalPage{tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(p.frame, 76, 0, true).
			AddItem(nil, 0, 1, false),
			0, 9, true).
		AddItem(nil, 0, 1, false)}
}

// modalPage makes a popup's page take every mouse event while it is shown.
// tview.Pages offers what its top page doesn't consume to the page below: a
// click beside the popup — or even on it, a list ignoring the button press
// that precedes a click — would give the focus to a panel hidden behind,
// which the keyboard would then edit blindly.
type modalPage struct {
	*tview.Flex
}

func (m modalPage) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	inner := m.Flex.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		_, capture := inner(action, event, setFocus)
		return true, capture
	}
}

func (p *recipePalette) applyLanguage(msgs *i18n.Strings) {
	p.filter.SetLabel(msgs.RecipeFilterLabel)
	p.updateTitle()
	p.showPreview()
}

// updateTitle shows how many recipes are listed and, when known, which
// cluster they were selected for.
func (p *recipePalette) updateTitle() {
	msgs := p.app.msgs
	title := fmt.Sprintf(msgs.RecipesTitleFmt, len(p.shown))
	if name := p.app.target.Name(); name != "" {
		title += " — " + name
	}
	p.frame.SetTitle(" " + title + " — " + msgs.RecipesHint + " ")
}

// open resets the palette (empty filter, full list) each time it is shown.
func (p *recipePalette) open() {
	p.filter.SetText("") // triggers refresh through the changed callback...
	p.refresh()          // ...unless the text was already empty
}

// refresh rebuilds the list from the filter: every word typed must appear
// (case-insensitively) somewhere in the recipe — group, title, tags or body.
func (p *recipePalette) refresh() {
	words := strings.Fields(strings.ToLower(p.filter.GetText()))

	p.shown = p.shown[:0]
	p.list.Clear()
	for _, r := range p.app.recipes {
		haystack := strings.ToLower(r.Group + " " + r.Title + " " + strings.Join(r.Tags, " ") + " " + r.Body)
		matches := true
		for _, w := range words {
			if !strings.Contains(haystack, w) {
				matches = false
				break
			}
		}
		if matches {
			p.shown = append(p.shown, r)
			p.list.AddItem(tview.Escape(p.label(r)), "", 0, nil)
		}
	}
	p.list.SetCurrentItem(0)
	p.updateTitle()
	p.showPreview()
}

// label is how a recipe is listed. Two marks may follow the title: the
// user's own recipes are flagged, and when the cluster couldn't be
// identified — so that nothing was filtered out — the clusters a recipe is
// meant for are spelled out, since the same title may then appear once per
// distribution.
func (p *recipePalette) label(r refdata.Recipe) string {
	label := r.Group + " › " + r.Title
	if r.Source != "" {
		label += " " + p.app.msgs.RecipeOwnMark
	}
	if p.app.target.Distribution == refdata.UnknownDistribution && !r.Constraint.IsZero() {
		label += "  (" + r.Constraint.String() + ")"
	}
	return label
}

// showPreview displays the selected recipe.
func (p *recipePalette) showPreview() {
	p.showPreviewOf(p.list.GetCurrentItem())
}

// showPreviewOf displays the body of the index-th listed recipe, comments
// dimmed; for a team's or user's recipe, the border names the file it comes
// from.
func (p *recipePalette) showPreviewOf(index int) {
	msgs := p.app.msgs
	if index < 0 || index >= len(p.shown) {
		p.preview.SetTitle(msgs.RecipePreviewTitle)
		p.preview.SetText("[gray]" + tview.Escape(msgs.RecipesNoMatch) + "[white]")
		return
	}
	r := p.shown[index]

	title := msgs.RecipePreviewTitle
	if r.Source != "" {
		title = " " + r.Source + " "
	}
	p.preview.SetTitle(title)

	var b strings.Builder
	for _, line := range strings.Split(r.Body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			b.WriteString("[gray]" + tview.Escape(line) + "[white]\n")
		} else {
			b.WriteString(tview.Escape(line) + "\n")
		}
	}
	p.preview.SetText(b.String())
	p.preview.ScrollToBeginning()
}

// handleKey is the filter field's input capture: everything that isn't text
// editing — moving in the list, confirming, leaving.
func (p *recipePalette) handleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		p.app.closeRecipes()
		return nil
	case tcell.KeyEnter:
		p.insert(p.list.GetCurrentItem())
		return nil
	case tcell.KeyDown, tcell.KeyCtrlN:
		p.move(1)
		return nil
	case tcell.KeyUp, tcell.KeyCtrlP:
		p.move(-1)
		return nil
	case tcell.KeyPgDn:
		p.move(p.pageSize())
		return nil
	case tcell.KeyPgUp:
		p.move(-p.pageSize())
		return nil
	case tcell.KeyTab, tcell.KeyBacktab:
		return nil // nothing to move focus to
	}
	return event
}

func (p *recipePalette) pageSize() int {
	_, _, _, height := p.list.GetInnerRect()
	if height < 2 {
		return 1
	}
	return height - 1
}

// move shifts the selection by delta, stopping at either end of the list
// (no wrap-around: on a long list, landing back at the top is disorienting).
func (p *recipePalette) move(delta int) {
	count := p.list.GetItemCount()
	if count == 0 {
		return
	}
	next := p.list.GetCurrentItem() + delta
	if next < 0 {
		next = 0
	}
	if next > count-1 {
		next = count - 1
	}
	p.list.SetCurrentItem(next)
}

// insert adds the index-th listed recipe at the end of the editor — titled
// by a comment, cursor on its first request so that Ctrl+E runs it — and
// closes the palette. Nothing happens if there is no such recipe (Enter on
// an empty list).
func (p *recipePalette) insert(index int) {
	if index < 0 || index >= len(p.shown) {
		return
	}
	r := p.shown[index]
	a := p.app
	a.closeRecipes()
	a.focusEditor()
	a.editor.AppendBlock("# "+r.Title+"\n"+r.Body, 1+r.FirstRequestLine())
	a.status.SetInfo(fmt.Sprintf(a.msgs.InfoRecipeInsertedFmt, r.Title))
}
