package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"termdevtools/i18n"
)

// TestShowPrependsRequestReminder checks that the "# METHOD path" comment
// line (a reminder of which request produced this result, no JSON payload)
// is prepended ahead of the pretty-printed body — and, crucially, ends up in
// PlainText() too, since that's what export (Ctrl+S) and clipboard copy
// (F2) send.
func TestShowPrependsRequestReminder(t *testing.T) {
	r := NewResultView(i18n.For(""))
	r.Show("GET", "_cat/health?v", nil, []byte(`{"status":"green"}`))

	want := "# GET _cat/health?v\n{\n  \"status\": \"green\"\n}"
	if got := r.PlainText(); got != want {
		t.Errorf("expected plain text %q, got %q", want, got)
	}
}

// TestShowPrependsRequestReminderNonJSON checks the same reminder line for
// a plain-text (non-JSON) response, e.g. a _cat/* command.
func TestShowPrependsRequestReminderNonJSON(t *testing.T) {
	r := NewResultView(i18n.For(""))
	r.Show("GET", "_cat/health?v", nil, []byte("epoch cluster status\n123 mycluster green"))

	want := "# GET _cat/health?v\nepoch cluster status\n123 mycluster green"
	if got := r.PlainText(); got != want {
		t.Errorf("expected plain text %q, got %q", want, got)
	}
}

// TestShowErrorPrependsRequestReminder checks that an error result also
// carries the reminder, so a failure can still be traced back to which
// request caused it once exported or copied.
func TestShowErrorPrependsRequestReminder(t *testing.T) {
	r := NewResultView(i18n.For(""))
	r.ShowError("POST", "_search", "connection refused")

	want := "# POST _search\nconnection refused"
	if got := r.PlainText(); got != want {
		t.Errorf("expected plain text %q, got %q", want, got)
	}
}

// TestExportIncludesRequestReminder checks that the reminder line survives
// into the exported file (Ctrl+S, right panel) — the main point of adding
// it, per the user's request.
func TestExportIncludesRequestReminder(t *testing.T) {
	r := NewResultView(i18n.For(""))
	r.Show("GET", "_cat/indices?v", nil, []byte(`{"a":1}`))

	dir := t.TempDir()
	path, err := r.Export(dir)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if filepath.Ext(path) != ".json" {
		t.Errorf("expected a .json export for a valid JSON body, got %q", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading exported file: %v", err)
	}
	if !strings.HasPrefix(string(data), "# GET _cat/indices?v\n") {
		t.Errorf("expected the exported file to start with the request reminder, got:\n%s", data)
	}
}

// TestShowIncludesResponseHeaders checks that response headers are listed
// as "# Header: value" lines right after the request reminder, sorted by
// name for determinism — and, like the reminder itself, included in
// PlainText() so they carry over to exports and clipboard copy too
// (SPEC.md §7 backlog #2).
func TestShowIncludesResponseHeaders(t *testing.T) {
	r := NewResultView(i18n.For(""))
	headers := http.Header{
		"X-Elastic-Product": {"Elasticsearch"},
		"Content-Type":      {"application/json"},
	}
	r.Show("GET", "_cat/health?v", headers, []byte(`{"status":"green"}`))

	want := "# GET _cat/health?v\n" +
		"# Content-Type: application/json\n" +
		"# X-Elastic-Product: Elasticsearch\n" +
		"{\n  \"status\": \"green\"\n}"
	if got := r.PlainText(); got != want {
		t.Errorf("expected plain text:\n%s\ngot:\n%s", want, got)
	}
}

// TestShowHighlightsWarningHeader checks that a "Warning" response header
// (Elasticsearch sets it to flag a deprecated API in use, RFC 7234) is
// shown in yellow, distinct from the other, plain gray header lines — so a
// deprecation notice doesn't blend in and go unnoticed.
func TestShowHighlightsWarningHeader(t *testing.T) {
	r := NewResultView(i18n.For(""))
	headers := http.Header{
		"Content-Type": {"application/json"},
		"Warning":      {`299 Elasticsearch-9.0.0 "[types removal] Specifying types is deprecated"`},
	}
	r.Show("GET", "my_index/_doc/1", headers, []byte(`{}`))

	if !strings.Contains(r.displayedText, "[yellow]# Warning:") {
		t.Errorf("expected the Warning header line to be colored yellow, got:\n%s", r.displayedText)
	}
	if !strings.Contains(r.displayedText, "[gray]# Content-Type:") {
		t.Errorf("expected the Content-Type header line to stay gray, got:\n%s", r.displayedText)
	}
}

// TestExportIncludesResponseHeaders checks that headers, like the request
// reminder, survive into the exported file.
func TestExportIncludesResponseHeaders(t *testing.T) {
	r := NewResultView(i18n.For(""))
	headers := http.Header{"X-Elastic-Product": {"Elasticsearch"}}
	r.Show("GET", "_cat/indices?v", headers, []byte(`{"a":1}`))

	dir := t.TempDir()
	path, err := r.Export(dir)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading exported file: %v", err)
	}
	want := "# GET _cat/indices?v\n# X-Elastic-Product: Elasticsearch\n"
	if !strings.HasPrefix(string(data), want) {
		t.Errorf("expected the exported file to start with:\n%s\ngot:\n%s", want, data)
	}
}
