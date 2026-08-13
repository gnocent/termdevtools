package ui

import (
	"os"
	"path/filepath"
	"testing"
)

var testColumns = map[string][]string{
	"indices": {"health", "h", "status", "s", "index", "i", "docs.count", "dc"},
	"shards":  {"index", "i", "shard", "s", "state", "st"},
}

func TestMatchCatCommandTrailingSlashBeforeQuery(t *testing.T) {
	// "_cat/indices/?h=...": the trailing "/" before the parameters is
	// optional in HTTP and is equivalent to "_cat/indices?h=...".
	cmd, ok := matchCatCommand("indices/", testColumns)
	if !ok || cmd != "indices" {
		t.Errorf("expected (\"indices\", true), got (%q, %v)", cmd, ok)
	}
}

func TestCatColumnCompletionTrailingSlashBeforeQuery(t *testing.T) {
	candidates, _, ok := catColumnCompletion("_cat/indices/?h=st", testColumns)
	if !ok {
		t.Fatal("expected ok=true despite the trailing '/' before '?'")
	}
	want := []string{"status"}
	if len(candidates) != len(want) || candidates[0] != want[0] {
		t.Errorf("expected %v, got %v", want, candidates)
	}
}

func TestMatchCatCommandExact(t *testing.T) {
	cmd, ok := matchCatCommand("shards", testColumns)
	if !ok || cmd != "shards" {
		t.Errorf("expected (\"shards\", true), got (%q, %v)", cmd, ok)
	}
}

func TestMatchCatCommandWithFilterSegment(t *testing.T) {
	// "_cat/shards/monindex" filters on index "monindex" — the recognized
	// command must stay "shards".
	cmd, ok := matchCatCommand("shards/monindex", testColumns)
	if !ok || cmd != "shards" {
		t.Errorf("expected (\"shards\", true), got (%q, %v)", cmd, ok)
	}
}

func TestMatchCatCommandLongestWins(t *testing.T) {
	cols := map[string][]string{
		"shards":         {"a"},
		"shards/special": {"b"},
	}
	cmd, ok := matchCatCommand("shards/special/foo", cols)
	if !ok || cmd != "shards/special" {
		t.Errorf("expected the longest match (\"shards/special\", true), got (%q, %v)", cmd, ok)
	}
}

func TestMatchCatCommandMultiSegmentCommand(t *testing.T) {
	cols := map[string][]string{
		"ml/anomaly_detectors": {"id", "state"},
	}
	cmd, ok := matchCatCommand("ml/anomaly_detectors/myjob", cols)
	if !ok || cmd != "ml/anomaly_detectors" {
		t.Errorf("expected (\"ml/anomaly_detectors\", true), got (%q, %v)", cmd, ok)
	}
}

func TestMatchCatCommandNoWordBoundary(t *testing.T) {
	// "shardsxyz" must not match "shards": it's not a filter after a '/'
	// boundary, just a different/unknown command name.
	if cmd, ok := matchCatCommand("shardsxyz", testColumns); ok {
		t.Errorf("expected no match for 'shardsxyz', got (%q, %v)", cmd, ok)
	}
}

func TestMatchCatCommandUnknown(t *testing.T) {
	if cmd, ok := matchCatCommand("unknown/thing", testColumns); ok {
		t.Errorf("expected no match for an unknown command, got (%q, %v)", cmd, ok)
	}
}

func TestCatColumnCompletionWithFilterSegment(t *testing.T) {
	// "_cat/shards/monindex?h=st" must behave like "_cat/shards?h=st".
	candidates, subLen, ok := catColumnCompletion("_cat/shards/monindex?h=st", testColumns)
	if !ok {
		t.Fatal("expected ok=true despite the filter segment before '?'")
	}
	// "st" is itself an alias for "state" in testColumns: both match the
	// "st" prefix.
	want := []string{"st", "state"}
	if len(candidates) != len(want) || candidates[0] != want[0] || candidates[1] != want[1] {
		t.Errorf("expected %v, got %v", want, candidates)
	}
	if subLen != len("st") {
		t.Errorf("expected subPrefixLen=%d, got %d", len("st"), subLen)
	}
}

func TestCatColumnCompletionHParam(t *testing.T) {
	candidates, subLen, ok := catColumnCompletion("_cat/indices?h=st", testColumns)
	if !ok {
		t.Fatal("expected ok=true for a h= parameter on a known _cat command")
	}
	if subLen != len("st") {
		t.Errorf("expected subPrefixLen=%d, got %d", len("st"), subLen)
	}
	want := []string{"status"}
	if len(candidates) != len(want) || candidates[0] != want[0] {
		t.Errorf("expected %v, got %v", want, candidates)
	}
}

func TestCatColumnCompletionMultipleColumnsOnlyLastOneCompleted(t *testing.T) {
	// "health," is already typed and must not be taken into account in the
	// prefix to complete: only "s" (after the last comma) is.
	candidates, subLen, ok := catColumnCompletion("_cat/indices?h=health,s", testColumns)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if subLen != len("s") {
		t.Errorf("expected subPrefixLen=%d, got %d", len("s"), subLen)
	}
	found := map[string]bool{}
	for _, c := range candidates {
		found[c] = true
	}
	if !found["s"] || !found["status"] {
		t.Errorf("expected candidates starting with 's' (s, status), got %v", candidates)
	}
}

func TestCatColumnCompletionSParam(t *testing.T) {
	candidates, _, ok := catColumnCompletion("_cat/shards?s=in", testColumns)
	if !ok {
		t.Fatal("expected ok=true for a s= parameter")
	}
	want := []string{"index"}
	if len(candidates) != len(want) || candidates[0] != want[0] {
		t.Errorf("expected %v, got %v", want, candidates)
	}
}

func TestCatColumnCompletionSortDirection(t *testing.T) {
	// Once ":" is typed after a column in s=, we complete asc/desc, not a
	// column name.
	candidates, subLen, ok := catColumnCompletion("_cat/shards?s=index:de", testColumns)
	if !ok {
		t.Fatal("expected ok=true for a sort-direction completion")
	}
	if subLen != len("de") {
		t.Errorf("expected subPrefixLen=%d, got %d", len("de"), subLen)
	}
	want := []string{"desc"}
	if len(candidates) != len(want) || candidates[0] != want[0] {
		t.Errorf("expected %v, got %v", want, candidates)
	}
}

func TestCatColumnCompletionColonOnlySpecialForS(t *testing.T) {
	// For h= (unlike s=), a ':' does not trigger sort-direction completion:
	// it's part of the column text to compare as-is. The context stays
	// recognized (ok=true) even though, as here, no test column is
	// literally named "status:de".
	candidates, subLen, ok := catColumnCompletion("_cat/indices?h=status:de", testColumns)
	if !ok {
		t.Fatal("expected ok=true: still a recognized _cat h= context")
	}
	if len(candidates) != 0 {
		t.Errorf("expected no match for the literal column name 'status:de', got %v", candidates)
	}
	if subLen != len("status:de") {
		t.Errorf("expected subPrefixLen=%d (':' not special-cased for h=), got %d", len("status:de"), subLen)
	}
}

func TestCatColumnCompletionIgnoredCases(t *testing.T) {
	cases := []string{
		"_search",                  // not a _cat command
		"_cat/indices",             // no '?'
		"_cat/indices?v",           // neither h= nor s=
		"_cat/unknown_cmd?h=st",    // _cat command unknown to the table
		"_cat/indices?format=json", // irrelevant parameter in last position
	}
	for _, prefix := range cases {
		if _, _, ok := catColumnCompletion(prefix, testColumns); ok {
			t.Errorf("expected ok=false for %q", prefix)
		}
	}
}

func TestCatColumnCompletionMultipleParams(t *testing.T) {
	// The parameter currently being typed is the one after the last '&'.
	candidates, _, ok := catColumnCompletion("_cat/indices?v&h=st", testColumns)
	if !ok {
		t.Fatal("expected ok=true")
	}
	want := []string{"status"}
	if len(candidates) != len(want) || candidates[0] != want[0] {
		t.Errorf("expected %v, got %v", want, candidates)
	}
}

func TestLoadCatColumnsFileMissing(t *testing.T) {
	cols, err := LoadCatColumnsFile(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cols != nil {
		t.Errorf("expected nil for a missing file, got %v", cols)
	}
}

func TestLoadCatColumnsFileParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cat_columns.txt")
	content := "# commentaire général\n\n# _cat/indices\nhealth\nh\nstatus\n\n# _cat/shards\nindex\ni\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cols, err := LoadCatColumnsFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantIndices := []string{"health", "h", "status"}
	if len(cols["indices"]) != len(wantIndices) {
		t.Fatalf("expected indices=%v, got %v", wantIndices, cols["indices"])
	}
	for i, c := range wantIndices {
		if cols["indices"][i] != c {
			t.Errorf("expected indices=%v, got %v", wantIndices, cols["indices"])
			break
		}
	}

	wantShards := []string{"index", "i"}
	if len(cols["shards"]) != len(wantShards) {
		t.Fatalf("expected shards=%v, got %v", wantShards, cols["shards"])
	}
}
