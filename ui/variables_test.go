package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSubstituteVariablesReplacesKnown checks the basic case: every
// "${name}" with a matching entry in vars is replaced, none reported
// missing.
func TestSubstituteVariablesReplacesKnown(t *testing.T) {
	vars := map[string]string{"idx": "my-index", "size": "10"}
	got, missing := substituteVariables("${idx}/_search?size=${size}", vars)

	want := "my-index/_search?size=10"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing variables, got %v", missing)
	}
}

// TestSubstituteVariablesReportsMissingAndLeavesThemAsIs checks that an
// undefined variable is left untouched in the result (not silently dropped
// or replaced with an empty string) and reported in missing.
func TestSubstituteVariablesReportsMissingAndLeavesThemAsIs(t *testing.T) {
	got, missing := substituteVariables("${unknown}/_search", nil)

	if got != "${unknown}/_search" {
		t.Errorf("expected the unresolved reference to stay literal, got %q", got)
	}
	if len(missing) != 1 || missing[0] != "unknown" {
		t.Errorf("expected missing=[\"unknown\"], got %v", missing)
	}
}

// TestSubstituteVariablesDeduplicatesMissing checks that the same missing
// name repeated several times is only reported once, in first-appearance
// order.
func TestSubstituteVariablesDeduplicatesMissing(t *testing.T) {
	_, missing := substituteVariables("${b} and ${a} and ${b} again", nil)

	want := []string{"b", "a"}
	if len(missing) != len(want) {
		t.Fatalf("expected %v, got %v", want, missing)
	}
	for i := range want {
		if missing[i] != want[i] {
			t.Errorf("expected %v, got %v", want, missing)
			break
		}
	}
}

// TestSubstituteVariablesIgnoresUnrelatedBraces checks that plain JSON
// braces (no leading "$") are never mistaken for a variable reference.
func TestSubstituteVariablesIgnoresUnrelatedBraces(t *testing.T) {
	body := `{"query":{"match_all":{}}}`
	got, missing := substituteVariables(body, nil)
	if got != body {
		t.Errorf("expected plain JSON to be left untouched, got %q", got)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing variables in plain JSON, got %v", missing)
	}
}

// TestParseVariablesIgnoresCommentsAndBlankLines checks the same "#"
// comment / blank-line convention already used by LoadEndpointsFile.
func TestParseVariablesIgnoresCommentsAndBlankLines(t *testing.T) {
	data := "# a comment\n\nidx=my-index\n  \n# another comment\nenv=staging\n"
	got := parseVariables([]byte(data))

	want := map[string]string{"idx": "my-index", "env": "staging"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("expected %s=%q, got %q", k, v, got[k])
		}
	}
}

// TestParseVariablesAllowsEqualsInValue checks that only the first "=" is a
// separator, so a value can itself contain one (e.g. a query string).
func TestParseVariablesAllowsEqualsInValue(t *testing.T) {
	got := parseVariables([]byte("query=field=value\n"))
	if got["query"] != "field=value" {
		t.Errorf("expected query=%q, got %q", "field=value", got["query"])
	}
}

// TestParseVariablesTrimsWhitespace checks that stray spaces around the
// name and value (easy to introduce hand-editing the file) don't turn into
// a name/value nobody can actually reference.
func TestParseVariablesTrimsWhitespace(t *testing.T) {
	got := parseVariables([]byte("  idx = my-index  \n"))
	if got["idx"] != "my-index" {
		t.Errorf("expected idx=%q, got %v", "my-index", got)
	}
}

// TestLoadVariablesFileCreatesDefaultWhenMissing checks the same
// self-documenting-on-first-use behavior as config.WriteDefaultConfigFile:
// no file yet -> one gets created, explaining the feature, and an empty
// (not nil) map is returned with no error.
func TestLoadVariablesFileCreatesDefaultWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "variables_test.txt")

	vars, err := LoadVariablesFile(path)
	if err != nil {
		t.Fatalf("LoadVariablesFile: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("expected no variables from a freshly created file, got %v", vars)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected the file to have been created, but: %v", err)
	}
	if !strings.Contains(string(data), "${nom}") && !strings.Contains(string(data), "F7") {
		t.Errorf("expected the generated file to explain the feature, got:\n%s", data)
	}
}

// TestLoadVariablesFileParsesExisting checks that an existing file's
// content is read (and not clobbered) — the counterpart of
// config.TestLoadPreservesExistingValues for this feature.
func TestLoadVariablesFileParsesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "variables_test.txt")
	const content = "idx=my-index\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	vars, err := LoadVariablesFile(path)
	if err != nil {
		t.Fatalf("LoadVariablesFile: %v", err)
	}
	if vars["idx"] != "my-index" {
		t.Errorf("expected idx=%q, got %v", "my-index", vars)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected the existing file to remain untouched, got:\n%s", data)
	}
}
