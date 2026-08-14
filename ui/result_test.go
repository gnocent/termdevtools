package ui

import (
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
	r.Show("GET", "_cat/health?v", []byte(`{"status":"green"}`))

	want := "# GET _cat/health?v\n{\n  \"status\": \"green\"\n}"
	if got := r.PlainText(); got != want {
		t.Errorf("expected plain text %q, got %q", want, got)
	}
}

// TestShowPrependsRequestReminderNonJSON checks the same reminder line for
// a plain-text (non-JSON) response, e.g. a _cat/* command.
func TestShowPrependsRequestReminderNonJSON(t *testing.T) {
	r := NewResultView(i18n.For(""))
	r.Show("GET", "_cat/health?v", []byte("epoch cluster status\n123 mycluster green"))

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
	r.Show("GET", "_cat/indices?v", []byte(`{"a":1}`))

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
