package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"termdevtools/parser"
	"termdevtools/refdata"
)

var os219 = refdata.Target{Distribution: refdata.OpenSearch, Version: refdata.Version{Major: 2, Minor: 19}, HasVersion: true}

// paletteView is what the recipe palette shows at one instant.
type paletteView struct {
	visible      bool
	filter       string
	shown        []refdata.Recipe
	labels       []string
	current      int
	title        string
	preview      string
	previewTitle string
}

// readPalette reads the palette from the application's own goroutine.
func readPalette(app *App) paletteView {
	var v paletteView
	onUI(app, func() {
		p := app.palette
		v.visible = app.recipesVisible
		v.filter = p.filter.GetText()
		v.shown = append([]refdata.Recipe(nil), p.shown...)
		for i := 0; i < p.list.GetItemCount(); i++ {
			label, _ := p.list.GetItemText(i)
			v.labels = append(v.labels, label)
		}
		v.current = p.list.GetCurrentItem()
		v.title = p.frame.GetTitle()
		v.preview = p.preview.GetText(true)
		v.previewTitle = p.preview.GetTitle()
	})
	return v
}

func statusText(app *App) string {
	text := stoppedText
	onUI(app, func() { text = app.status.status.GetText(true) })
	return text
}

// openPalette presses F8 and waits for the palette.
func openPalette(t *testing.T, app *App, screen tcell.SimulationScreen) paletteView {
	t.Helper()
	screen.InjectKey(tcell.KeyF8, 0, tcell.ModNone)
	eventually(t, "the recipe palette to open", func() bool { return readPalette(app).visible })
	return readPalette(app)
}

// typeFilter types text into the open palette and waits for it to apply.
func typeFilter(t *testing.T, app *App, screen tcell.SimulationScreen, text string) paletteView {
	t.Helper()
	before := readPalette(app).filter
	injectText(screen, text)
	eventually(t, "the filter to apply", func() bool { return readPalette(app).filter == before+text })
	return readPalette(app)
}

// TestRecipePaletteInsertsSelectedRecipe walks through the palette's main
// use: F8, a few words to narrow the list down, Enter — the recipe lands at
// the end of the editor, after what was already there, with the cursor on
// its request so that Ctrl+E runs it.
func TestRecipePaletteInsertsSelectedRecipe(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET _cluster/health")
	eventually(t, "the typed request", func() bool { return editorText(app) == "GET _cluster/health" })

	all := openPalette(t, app, screen)
	if len(all.shown) < 50 || len(all.labels) != len(all.shown) {
		t.Fatalf("expected the whole catalog to be listed, got %d recipes and %d labels", len(all.shown), len(all.labels))
	}
	if !strings.Contains(all.title, fmt.Sprintf(app.msgs.RecipesTitleFmt, len(all.shown))) || !strings.Contains(all.title, "Elasticsearch 9.5") {
		t.Errorf("expected the title to give the count and the cluster, got %q", all.title)
	}

	filtered := typeFilter(t, app, screen, "WHY unassigned")
	if len(filtered.shown) == 0 || len(filtered.shown) >= len(all.shown) {
		t.Fatalf("expected the filter to narrow the list down, got %d of %d", len(filtered.shown), len(all.shown))
	}
	want := filtered.shown[0]
	if want.Title != "Why is a shard unassigned?" {
		t.Fatalf("expected the allocation-explain recipe first, got %q", want.Title)
	}
	if !strings.Contains(filtered.preview, "GET _cluster/allocation/explain") {
		t.Errorf("expected the preview to show the selected recipe, got %q", filtered.preview)
	}

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	eventually(t, "the palette to close", func() bool { return !readPalette(app).visible })

	wantText := "GET _cluster/health\n\n# " + want.Title + "\n" + want.Body + "\n"
	if got := editorText(app); got != wantText {
		t.Fatalf("unexpected editor content after insertion:\n%q\nwant:\n%q", got, wantText)
	}

	var cursorLine int
	var editorFocused bool
	app.tapp.QueueUpdate(func() {
		cursorLine = app.editor.CursorLine()
		editorFocused = app.tapp.GetFocus() == app.editor.Primitive()
	})
	if !editorFocused {
		t.Error("expected the focus to be back on the editor")
	}
	if req, err := parser.RequestAtLine(wantText, cursorLine); err != nil || req.Path != "_cluster/allocation/explain" {
		t.Errorf("expected the cursor (line %d) on the recipe's request, got %+v (%v)", cursorLine, req, err)
	}
	if status := statusText(app); !strings.Contains(status, fmt.Sprintf(app.msgs.InfoRecipeInsertedFmt, want.Title)) {
		t.Errorf("expected the status bar to confirm the insertion, got %q", status)
	}

	// One undo takes the whole recipe back out.
	screen.InjectKey(tcell.KeyCtrlZ, 0, tcell.ModNone)
	eventually(t, "undo to remove the recipe", func() bool { return editorText(app) == "GET _cluster/health" })
}

// TestRecipeInsertionIsItsOwnUndoStep checks that typing right after an
// insertion doesn't weld the two together: undoing the keystroke must not
// take the whole recipe away with it.
func TestRecipeInsertionIsItsOwnUndoStep(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET _cluster/health")
	eventually(t, "the typed request", func() bool { return editorText(app) == "GET _cluster/health" })

	first := openPalette(t, app, screen).shown[0]
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	inserted := "GET _cluster/health\n\n# " + first.Title + "\n" + first.Body + "\n"
	eventually(t, "the recipe to be inserted", func() bool { return editorText(app) == inserted })

	injectText(screen, "x")
	eventually(t, "the keystroke", func() bool { return editorText(app) != inserted })
	screen.InjectKey(tcell.KeyCtrlZ, 0, tcell.ModNone)
	eventually(t, "undo to remove the keystroke only", func() bool { return editorText(app) == inserted })
}

// click presses and releases the left mouse button at a screen position.
func click(screen tcell.SimulationScreen, x, y int) {
	screen.InjectMouse(x, y, tcell.Button1, tcell.ModNone)
	screen.InjectMouse(x, y, tcell.ButtonNone, tcell.ModNone)
}

// TestRecipePaletteMouse checks the palette with mouse support on: a click
// on a recipe inserts that recipe — wherever the row lies on screen, over
// the editor or over the result panel — and a click elsewhere in the popup
// never reaches the panels hidden behind it.
func TestRecipePaletteMouse(t *testing.T) {
	// The list's rows, and the preview, as laid out on screen.
	layout := func(app *App) (listX, listY, listWidth, previewX, previewY int) {
		app.tapp.QueueUpdate(func() {
			listX, listY, listWidth, _ = app.palette.list.GetInnerRect()
			previewX, previewY, _, _ = app.palette.preview.GetInnerRect()
		})
		return
	}

	for name, fromRight := range map[string]bool{"row over the editor": false, "row over the result panel": true} {
		t.Run(name, func(t *testing.T) {
			app, screen := newTestAppWith(t, testAppOptions{mouse: true})
			all := openPalette(t, app, screen)
			var listX, listY, listWidth int
			eventually(t, "the palette to be laid out", func() bool {
				listX, listY, listWidth, _, _ = layout(app)
				return listWidth > 0
			})

			x := listX + 2
			if fromRight {
				x = listX + listWidth - 3
			}
			click(screen, x, listY+3)
			eventually(t, "the palette to close", func() bool { return !readPalette(app).visible })

			want := all.shown[3]
			if got := editorText(app); got != "# "+want.Title+"\n"+want.Body+"\n" {
				t.Errorf("expected the clicked recipe (%q) to be inserted, got %q", want.Title, got)
			}
		})
	}

	t.Run("clicks beside the list", func(t *testing.T) {
		app, screen := newTestAppWith(t, testAppOptions{mouse: true})
		openPalette(t, app, screen)
		var previewX, previewY int
		eventually(t, "the palette to be laid out", func() bool {
			_, _, _, previewX, previewY = layout(app)
			return previewX > 0
		})

		// On the preview, then outside the popup, on the editor's border
		// showing behind it: the keyboard must stay with the filter field.
		click(screen, previewX+2, previewY+1)
		click(screen, 0, 1)
		typeFilter(t, app, screen, "disk")
		if got := editorText(app); got != "" {
			t.Errorf("expected nothing to reach the editor behind the popup, got %q", got)
		}

		screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
		eventually(t, "Esc to close the palette", func() bool { return !readPalette(app).visible })
	})
}

// TestRecipePaletteIntoEmptyEditor checks the insertion's other edge: no
// blank line is added in front when there is nothing before.
func TestRecipePaletteIntoEmptyEditor(t *testing.T) {
	app, screen := newTestApp(t)

	first := openPalette(t, app, screen).shown[0]
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	eventually(t, "the palette to close", func() bool { return !readPalette(app).visible })

	if got, want := editorText(app), "# "+first.Title+"\n"+first.Body+"\n"; got != want {
		t.Errorf("unexpected editor content:\n%q\nwant:\n%q", got, want)
	}
}

// TestRecipePaletteFitsASmallTerminal checks what is actually drawn on the
// classic 80x24 terminal: title with its hint, filter field, list and
// preview all visible at once.
func TestRecipePaletteFitsASmallTerminal(t *testing.T) {
	app, screen := newTestApp(t)
	all := openPalette(t, app, screen)

	var text string
	eventually(t, "the palette to be drawn", func() bool {
		text = screenText(screen)
		return strings.Contains(text, app.msgs.RecipeFilterLabel)
	})
	t.Logf("screen:\n%s", text)

	first := all.shown[0]
	for what, want := range map[string]string{
		"title":           fmt.Sprintf(app.msgs.RecipesTitleFmt, len(all.shown)),
		"keys hint":       app.msgs.RecipesHint,
		"first recipe":    first.Group + " › " + first.Title,
		"preview caption": strings.TrimSpace(app.msgs.RecipePreviewTitle),
		"preview content": strings.Split(first.Body, "\n")[0],
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected the %s (%q) on screen", what, want)
		}
	}
}

func TestRecipePaletteEscapeClosesWithoutChange(t *testing.T) {
	app, screen := newTestApp(t)
	injectText(screen, "GET _cat/nodes?v")
	eventually(t, "the typed request", func() bool { return editorText(app) == "GET _cat/nodes?v" })

	openPalette(t, app, screen)
	typeFilter(t, app, screen, "disk")
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	eventually(t, "the palette to close", func() bool { return !readPalette(app).visible })

	if got := editorText(app); got != "GET _cat/nodes?v" {
		t.Errorf("expected Esc to leave the editor alone, got %q", got)
	}

	// Reopened, the palette starts over: no filter, the whole list.
	if reopened := openPalette(t, app, screen); reopened.filter != "" || len(reopened.shown) != len(app.recipes) {
		t.Errorf("expected a fresh palette, got filter %q and %d recipes", reopened.filter, len(reopened.shown))
	}
}

// TestRecipePaletteSelectionKeys checks that the list is driven from the
// filter field: arrows move the selection (and the preview with it) without
// wrapping around, and typing still filters.
func TestRecipePaletteSelectionKeys(t *testing.T) {
	app, screen := newTestApp(t)
	all := openPalette(t, app, screen)

	press := func(key tcell.Key, wantCurrent int) paletteView {
		t.Helper()
		screen.InjectKey(key, 0, tcell.ModNone)
		eventually(t, fmt.Sprintf("the selection to reach %d", wantCurrent), func() bool {
			return readPalette(app).current == wantCurrent
		})
		return readPalette(app)
	}

	press(tcell.KeyDown, 1)
	second := press(tcell.KeyDown, 2)
	if first := strings.Split(all.shown[2].Body, "\n")[0]; !strings.Contains(second.preview, first) {
		t.Errorf("expected the preview to follow the selection, got %q", second.preview)
	}
	press(tcell.KeyCtrlP, 1)
	press(tcell.KeyUp, 0)

	// No wrap-around at the top: the key is taken, the selection stays.
	screen.InjectKey(tcell.KeyUp, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyCtrlN, 0, tcell.ModNone)
	press(tcell.KeyCtrlN, 2)

	screen.InjectKey(tcell.KeyPgDn, 0, tcell.ModNone)
	eventually(t, "PgDn to move by more than one", func() bool { return readPalette(app).current > 3 })

	// Typing resets the selection to the first match.
	if filtered := typeFilter(t, app, screen, "snapshot"); filtered.current != 0 {
		t.Errorf("expected the first match to be selected after filtering, got %d", filtered.current)
	}
}

func TestRecipePaletteNoMatch(t *testing.T) {
	app, screen := newTestApp(t)
	openPalette(t, app, screen)

	none := typeFilter(t, app, screen, "zzzznothing")
	if len(none.shown) != 0 || !strings.Contains(none.preview, app.msgs.RecipesNoMatch) {
		t.Errorf("expected an empty list saying so, got %d recipes and preview %q", len(none.shown), none.preview)
	}

	// Enter has nothing to insert: the palette stays open, the editor empty.
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	eventually(t, "the backspace to apply", func() bool { return readPalette(app).filter == "zzzznothin" })
	if v := readPalette(app); !v.visible || editorText(app) != "" {
		t.Errorf("expected Enter on an empty list to do nothing (visible %v, editor %q)", v.visible, editorText(app))
	}
}

// TestRecipePaletteKeepsShortcutsToItself checks that the application's
// shortcuts don't fire behind the palette.
func TestRecipePaletteKeepsShortcutsToItself(t *testing.T) {
	app, screen := newTestApp(t)
	openPalette(t, app, screen)

	screen.InjectKey(tcell.KeyF1, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyF8, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	typeFilter(t, app, screen, "x") // every key above has been processed by now

	var helpVisible bool
	app.tapp.QueueUpdate(func() { helpVisible = app.helpVisible })
	if v := readPalette(app); !v.visible || v.filter != "x" || helpVisible {
		t.Errorf("expected the palette to stay as it was (visible %v, filter %q, help %v)", v.visible, v.filter, helpVisible)
	}
	if got := editorText(app); got != "" {
		t.Errorf("expected nothing to reach the editor, got %q", got)
	}
}

// TestRecipesSelectedForDetectedCluster checks that the palette only offers
// what works on the connected cluster — and everything, each recipe labeled
// with the clusters it is meant for, when the cluster couldn't be identified.
func TestRecipesSelectedForDetectedCluster(t *testing.T) {
	bodies := func(app *App) string {
		var b strings.Builder
		for _, r := range app.recipes {
			b.WriteString(r.Body + "\n")
		}
		return b.String()
	}

	onES, _ := newTestApp(t)
	if got := bodies(onES); !strings.Contains(got, "_ilm/") || strings.Contains(got, "_plugins/") {
		t.Error("Elasticsearch: expected the ILM recipes and none of the OpenSearch plugins'")
	}

	onOS, _ := newTestAppWith(t, testAppOptions{target: os219})
	if got := bodies(onOS); !strings.Contains(got, "_plugins/_ism/") || strings.Contains(got, "_ilm/") || strings.Contains(got, "_slm/") {
		t.Error("OpenSearch: expected the ISM recipes and none of Elasticsearch's ILM/SLM ones")
	}

	unknown, screen := newTestAppWith(t, testAppOptions{undetected: true})
	if got := bodies(unknown); !strings.Contains(got, "_ilm/") || !strings.Contains(got, "_plugins/_ism/") {
		t.Error("unidentified cluster: expected every recipe to be offered")
	}
	openPalette(t, unknown, screen)
	variants := typeFilter(t, unknown, screen, "retry failed lifecycle step")
	if len(variants.labels) != 2 || !strings.Contains(variants.labels[0], "(@es") || !strings.Contains(variants.labels[1], "(@opensearch") {
		t.Errorf("expected both variants, each labeled with its clusters, got %q", variants.labels)
	}
}

// TestUserRecipesJoinTheCatalog checks the user's layer: a recipe filed
// under an existing theme is listed with that theme's built-in recipes,
// marked as the user's own; one with the group and title of a built-in
// recipe replaces it.
func TestUserRecipesJoinTheCatalog(t *testing.T) {
	userDir := t.TempDir()
	writeReferenceFile(t, userDir, filepath.Join("recipes", "mine.txt"), `# @group Snapshots
# @recipe Nightly snapshots of my team
GET _snapshot/nightly/_all

# @recipe Snapshot repositories
# Ours, with their settings.
GET _snapshot?pretty
`)
	app, screen := newTestAppWith(t, testAppOptions{reference: refdata.Sources{UserDir: userDir}})
	builtIn, _ := newTestApp(t)

	// One addition, one replacement.
	if got, want := len(app.recipes), len(builtIn.recipes)+1; got != want {
		t.Fatalf("expected %d recipes, got %d", want, got)
	}

	all := openPalette(t, app, screen)
	var group []int // positions of the "Snapshots" recipes in the list
	for i, r := range all.shown {
		if r.Group == "Snapshots" {
			group = append(group, i)
		}
	}
	if len(group) < 3 || group[len(group)-1]-group[0] != len(group)-1 {
		t.Fatalf("expected the Snapshots recipes to be listed together, got positions %v", group)
	}
	first, mine, redefined := group[0], group[len(group)-2], group[len(group)-1]
	if all.shown[mine].Title != "Nightly snapshots of my team" || all.shown[redefined].Body != "# Ours, with their settings.\nGET _snapshot?pretty" {
		t.Fatalf("expected the user's two recipes at the end of the group, got %q then %q", all.shown[mine].Title, all.shown[redefined].Title)
	}
	if !strings.HasSuffix(all.labels[mine], app.msgs.RecipeOwnMark) {
		t.Errorf("expected the user's recipe to be marked, got %q", all.labels[mine])
	}
	if strings.Contains(all.labels[first], app.msgs.RecipeOwnMark) {
		t.Errorf("expected a built-in recipe not to be marked, got %q", all.labels[first])
	}
	replaced := 0
	for _, r := range all.shown {
		if r.Title == "Snapshot repositories" {
			replaced++
		}
	}
	if replaced != 1 {
		t.Errorf("expected the redefined recipe to be listed once, got %d", replaced)
	}

	// The preview of a user's recipe names the file it comes from.
	narrowed := typeFilter(t, app, screen, "nightly")
	if !strings.Contains(narrowed.previewTitle, "mine.txt") {
		t.Errorf("expected the preview to name the recipe's file, got %q", narrowed.previewTitle)
	}
}

// TestF7ReloadsRecipesAndEndpoints checks that files created or changed
// while the application runs take effect on F7, and that the _cat columns
// are asked from the cluster again afterwards.
func TestF7ReloadsRecipesAndEndpoints(t *testing.T) {
	userDir := t.TempDir()
	app, screen := newTestAppWith(t, testAppOptions{reference: refdata.Sources{UserDir: userDir}})

	var recipesBefore, endpointsBefore, catKnown int
	app.tapp.QueueUpdate(func() {
		recipesBefore, endpointsBefore, catKnown = len(app.recipes), len(app.endpoints), len(app.catLive)
	})
	if catKnown == 0 {
		t.Fatal("test setup: expected _cat columns to be known before the reload")
	}

	writeReferenceFile(t, userDir, filepath.Join("recipes", "mine.txt"), "# @recipe Mine\nGET _mine\n")
	writeReferenceFile(t, userDir, "endpoints.txt", "_mine\n")
	screen.InjectKey(tcell.KeyF7, 0, tcell.ModNone)

	var recipes, endpoints, variables int
	eventually(t, "the new recipe to be loaded", func() bool {
		app.tapp.QueueUpdate(func() {
			recipes, endpoints, variables, catKnown = len(app.recipes), len(app.endpoints), len(app.variables), len(app.catLive)
		})
		return recipes == recipesBefore+1
	})
	if endpoints != endpointsBefore+1 {
		t.Errorf("expected one more endpoint, got %d after %d", endpoints, endpointsBefore)
	}
	if catKnown != 0 {
		t.Errorf("expected the _cat columns to be forgotten, %d commands still known", catKnown)
	}
	if status, want := statusText(app), fmt.Sprintf(app.msgs.InfoReloadedFmt, variables, recipes, endpoints); !strings.Contains(status, want) {
		t.Errorf("expected the status bar to sum the reload up (%q), got %q", want, status)
	}

	// A mistake in a file is reported — file and line — instead of the
	// summary, and what is valid still loads.
	writeReferenceFile(t, userDir, filepath.Join("recipes", "mine.txt"), "# @recipe Mine\nGET _mine\n\n# @recipe Empty\n# no request\n")
	screen.InjectKey(tcell.KeyF7, 0, tcell.ModNone)
	eventually(t, "the problem to be reported", func() bool { return strings.Contains(statusText(app), "mine.txt:4") })
	app.tapp.QueueUpdate(func() { recipes = len(app.recipes) })
	if recipes != recipesBefore+1 {
		t.Errorf("expected the valid recipe to remain, got %d recipes after %d", recipes, recipesBefore)
	}
}

// TestEditorStartingContent checks what a session starts with: what was
// saved for the cluster; failing that the team's cheatsheet; failing that
// the built-in starter.
func TestEditorStartingContent(t *testing.T) {
	const starter, cheatsheet, saved = "# starter\nGET /\n", "# team\nGET _cat/indices?v\n", "GET _mine\n"

	cases := []struct {
		name string
		opts testAppOptions
		want string
	}{
		{"nothing at all", testAppOptions{}, ""},
		{"first connection", testAppOptions{starter: starter}, starter},
		{"team cheatsheet", testAppOptions{starter: starter, cheatsheet: cheatsheet}, cheatsheet},
		{"saved session", testAppOptions{starter: starter, cheatsheet: cheatsheet, saved: saved}, saved},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app, _ := newTestAppWith(t, c.opts)
			if got := editorText(app); got != c.want {
				t.Errorf("expected the editor to start with %q, got %q", c.want, got)
			}
		})
	}
}

// TestBuiltInStarterIsRunnable guards the starter shipped in the binary: it
// must hold requests, every one of them usable as is.
func TestBuiltInStarterIsRunnable(t *testing.T) {
	starter := refdata.Starter()
	requests := parser.ParseAll(starter)
	if len(requests) < 3 {
		t.Fatalf("expected the starter to hold a few requests, got %d", len(requests))
	}
	if strings.Contains(starter, "${") || strings.Contains(starter, "\r") {
		t.Error("expected the starter to need no variable and to use plain line feeds")
	}
}

// TestCtrlCSavesBehindPopups checks that quitting saves the left panel
// whatever is open in front of it — Ctrl+C is the only way out, and tview's
// own handling of it, reached when a popup lets the key through, doesn't
// save.
func TestCtrlCSavesBehindPopups(t *testing.T) {
	for name, open := range map[string]tcell.Key{"recipes": tcell.KeyF8, "help": tcell.KeyF1, "search": tcell.KeyCtrlF} {
		t.Run(name, func(t *testing.T) {
			app, screen := newTestApp(t)
			injectText(screen, "GET _cluster/health")
			eventually(t, "the typed request", func() bool { return editorText(app) == "GET _cluster/health" })

			screen.InjectKey(open, 0, tcell.ModNone)
			eventually(t, "the popup to take the focus", func() bool {
				var away bool
				app.tapp.QueueUpdate(func() { away = app.tapp.GetFocus() != app.editor.Primitive() })
				return away
			})

			screen.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
			eventually(t, "the left panel to be saved", func() bool {
				saved, err := os.ReadFile(app.queriesPath)
				return err == nil && string(saved) == "GET _cluster/health"
			})
		})
	}
}
