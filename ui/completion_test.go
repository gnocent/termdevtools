package ui

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestMatchEndpointsPrefix(t *testing.T) {
	got := matchPrefix("_cat/s", knownEndpoints)
	want := []string{"_cat/segments?v", "_cat/shards?v", "_cat/snapshots?v"}
	if !sort.StringsAreSorted(got) {
		t.Errorf("expected sorted results, got %v", got)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %v, got %v", want, got)
			break
		}
	}
}

func TestMatchEndpointsEmptyPrefixReturnsAll(t *testing.T) {
	got := matchPrefix("", knownEndpoints)
	if len(got) != len(knownEndpoints) {
		t.Errorf("expected all %d known endpoints, got %d", len(knownEndpoints), len(got))
	}
}

func TestMatchEndpointsNoMatch(t *testing.T) {
	got := matchPrefix("does_not_exist", knownEndpoints)
	if len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

func TestMatchEndpointsCaseInsensitive(t *testing.T) {
	got := matchPrefix("_CAT/SH", knownEndpoints)
	if len(got) == 0 {
		t.Error("expected case-insensitive matching to find results")
	}
}

func TestLoadEndpointsFileMissing(t *testing.T) {
	endpoints, err := LoadEndpointsFile(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err != nil {
		t.Fatalf("unexpected error for a missing file: %v", err)
	}
	if endpoints != nil {
		t.Errorf("expected nil endpoints for a missing file, got %v", endpoints)
	}
}

func TestLoadEndpointsFileParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "endpoints.txt")
	content := "# commentaire\n\n_cat/indices\n  _cat/shards  \n# encore un commentaire\n_search\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	endpoints, err := LoadEndpointsFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"_cat/indices", "_cat/shards", "_search"}
	if len(endpoints) != len(want) {
		t.Fatalf("expected %v, got %v", want, endpoints)
	}
	for i := range want {
		if endpoints[i] != want[i] {
			t.Errorf("expected %v, got %v", want, endpoints)
			break
		}
	}
}
