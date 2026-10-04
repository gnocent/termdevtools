package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"termdevtools/refdata"
)

// writeReferenceFile creates dir/name with content, for tests that extend
// the built-in reference data through the user's or team's directory.
func writeReferenceFile(t *testing.T, dir, name, content string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, name), content)
}

// TestUserEndpointsExtendCompletion checks that an endpoint from the user's
// endpoints.txt is offered by completion alongside the built-in ones.
func TestUserEndpointsExtendCompletion(t *testing.T) {
	userDir := t.TempDir()
	writeReferenceFile(t, userDir, "endpoints.txt", "# mine\n_my/custom/endpoint\n")
	app, screen := newTestAppWith(t, testAppOptions{reference: refdata.Sources{UserDir: userDir}})

	injectText(screen, "GET _my")
	waitForDraw(t, screen)
	screen.InjectKey(tcell.KeyF10, 0, tcell.ModNone)
	waitForDraw(t, screen)

	if got := editorText(app); got != "GET _my/custom/endpoint" {
		t.Errorf("expected the user's endpoint to complete, got %q", got)
	}

	// The built-in endpoints are still there: the file adds, it doesn't
	// replace.
	if len(matchPrefix("_cluster/", app.endpoints)) == 0 {
		t.Error("expected the built-in endpoints to remain available")
	}
}

// TestEndpointsFilteredByDetectedCluster checks that completion only offers
// what exists on the connected cluster: an OpenSearch-only endpoint from
// the user's file isn't proposed on Elasticsearch, and is on OpenSearch.
func TestEndpointsFilteredByDetectedCluster(t *testing.T) {
	userDir := t.TempDir()
	writeReferenceFile(t, userDir, "endpoints.txt", "_only/on/opensearch @opensearch\n")
	sources := refdata.Sources{UserDir: userDir}

	onES, _ := newTestAppWith(t, testAppOptions{reference: sources})
	if got := matchPrefix("_only", onES.endpoints); len(got) != 0 {
		t.Errorf("Elasticsearch: expected the OpenSearch-only endpoint to be hidden, got %v", got)
	}

	onOS, _ := newTestAppWith(t, testAppOptions{
		reference: sources,
		target:    refdata.Target{Distribution: refdata.OpenSearch, Version: refdata.Version{Major: 2, Minor: 19}, HasVersion: true},
	})
	if got := matchPrefix("_only", onOS.endpoints); len(got) != 1 {
		t.Errorf("OpenSearch: expected the endpoint to be offered, got %v", got)
	}
}

// TestReferenceFileProblemsAreReported checks that a malformed line in a
// reference file is surfaced in the status bar (file and line), instead of
// being silently dropped.
func TestReferenceFileProblemsAreReported(t *testing.T) {
	userDir := t.TempDir()
	writeReferenceFile(t, userDir, "endpoints.txt", "_ok\n_bad @nope\n")
	app, _ := newTestAppWith(t, testAppOptions{reference: refdata.Sources{UserDir: userDir}})

	var status string
	app.tapp.QueueUpdate(func() { status = app.status.status.GetText(true) })
	if !strings.Contains(status, "endpoints.txt:2") {
		t.Errorf("expected the status bar to point at endpoints.txt line 2, got %q", status)
	}
	if len(matchPrefix("_ok", app.endpoints)) != 1 {
		t.Error("expected the valid line of the same file to load")
	}
}
