package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"termdevtools/parser"
)

// TestResolveRequestSubstitutesKnownVariables checks App.resolveRequest
// directly: every "${name}" reference in both the path and the body gets
// replaced.
func TestResolveRequestSubstitutesKnownVariables(t *testing.T) {
	app, _ := newTestApp(t)
	app.variables = map[string]string{"idx": "my-index"}

	req := &parser.Request{Method: "GET", Path: "${idx}/_search", Body: []byte(`{"query":{"term":{"a":"${idx}"}}}`)}
	path, body, err := app.resolveRequest(req)
	if err != nil {
		t.Fatalf("resolveRequest: %v", err)
	}
	if path != "my-index/_search" {
		t.Errorf("expected path %q, got %q", "my-index/_search", path)
	}
	if want := `{"query":{"term":{"a":"my-index"}}}`; string(body) != want {
		t.Errorf("expected body %q, got %q", want, body)
	}
}

// TestResolveRequestReportsUnknownVariables checks that an undefined
// variable produces an error naming it, rather than silently sending
// "${name}" literally to the cluster.
func TestResolveRequestReportsUnknownVariables(t *testing.T) {
	app, _ := newTestApp(t)

	req := &parser.Request{Method: "GET", Path: "${missing_var}/_search"}
	_, _, err := app.resolveRequest(req)
	if err == nil {
		t.Fatal("expected an error for an unknown variable")
	}
	if !strings.Contains(err.Error(), "missing_var") {
		t.Errorf("expected the error to name the missing variable, got: %v", err)
	}
}

// TestCtrlEAbortsOnUnknownVariable checks the actual F9/Ctrl+E-level
// behavior: an undefined variable stops execution before it ever reaches
// the network call — the status bar names it instead of silently sending
// the literal "${name}" text.
func TestCtrlEAbortsOnUnknownVariable(t *testing.T) {
	app, screen := newTestApp(t)

	injectText(screen, "GET ${missing_var}")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)
	waitForDraw(t, screen)

	text := screenText(screen)
	if strings.Contains(text, app.msgs.StatusRunning) {
		t.Error("expected execution to be aborted before reaching the network call")
	}
	if !strings.Contains(text, "missing_var") {
		t.Errorf("expected the status bar to name the missing variable, got:\n%s", text)
	}
}

// TestF9CurlCommandUsesSubstitutedVariables checks that "copy as cURL" (F9)
// also resolves variables — it's a "what would actually be sent" operation,
// same as Ctrl+E.
func TestF9CurlCommandUsesSubstitutedVariables(t *testing.T) {
	app, screen := newTestApp(t)
	app.variables = map[string]string{"idx": "my-index"}

	injectText(screen, "GET ${idx}/_search")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF9, 0, tcell.ModNone)
	waitForDraw(t, screen)

	got := string(screen.GetClipboardData())
	if !strings.Contains(got, "my-index/_search") {
		t.Errorf("expected the curl command to use the substituted path, got %q", got)
	}
	if strings.Contains(got, "${idx}") {
		t.Errorf("expected no literal ${idx} placeholder left in the curl command, got %q", got)
	}
}

// TestF9AbortsOnUnknownVariable mirrors TestCtrlEAbortsOnUnknownVariable
// for the copy-as-cURL path: nothing gets copied when a variable is
// undefined, rather than a curl command with a broken placeholder in it.
func TestF9AbortsOnUnknownVariable(t *testing.T) {
	_, screen := newTestApp(t)

	injectText(screen, "GET ${missing_var}")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF9, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got := screen.GetClipboardData(); len(got) != 0 {
		t.Errorf("expected no curl command copied when a variable is undefined, got %q", got)
	}
}

// TestF7ReloadsVariablesFromDisk checks the actual point of F7: a change
// made to the variables file by an external editor, while the app is
// already running (and so already loaded a now-stale copy), takes effect
// without needing to reconnect.
func TestF7ReloadsVariablesFromDisk(t *testing.T) {
	app, screen := newTestApp(t)

	if err := os.WriteFile(app.variablesPath, []byte("idx=my-index\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	screen.InjectKey(tcell.KeyF7, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if app.variables["idx"] != "my-index" {
		t.Errorf("expected the reloaded variables to include idx=my-index, got %v", app.variables)
	}
}

// TestReformatBodyDoesNotSubstituteVariables checks that F4 never resolves
// "${name}" placeholders — reformatBody edits the saved query text itself,
// so doing so would permanently bake the resolved value into the editor,
// losing the reusable placeholder for good. Unlike Ctrl+E/F9, F4 must
// leave variable references completely alone.
func TestReformatBodyDoesNotSubstituteVariables(t *testing.T) {
	app, screen := newTestApp(t)
	app.variables = map[string]string{"idx": "my-index"}

	injectText(screen, `POST _search`)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	injectText(screen, `{"index":"${idx}"}`)
	waitForDraw(t, screen)

	screen.InjectKey(tcell.KeyF4, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got := app.editor.Text(); !strings.Contains(got, "${idx}") {
		t.Errorf("expected the ${idx} placeholder to survive reformatting untouched, got:\n%s", got)
	}
}
