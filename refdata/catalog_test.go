package refdata

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestEmbeddedDataLoads checks the data compiled into the binary: it must
// parse without a single problem (Load panics otherwise) and hold no
// duplicate, since duplicates there would be a generator bug.
func TestEmbeddedDataLoads(t *testing.T) {
	c := Load(Sources{})
	if len(c.Problems) != 0 {
		t.Fatalf("unexpected problems: %v", c.Problems)
	}
	if len(c.endpoints) == 0 {
		t.Fatal("no embedded endpoint")
	}
	seen := make(map[string]bool)
	for _, e := range c.endpoints {
		if seen[e.Path] {
			t.Errorf("embedded endpoint listed twice: %s", e.Path)
		}
		seen[e.Path] = true
	}

	if len(c.catCommands) == 0 {
		t.Fatal("no embedded _cat command")
	}
	for _, cmd := range c.catCommands {
		if len(cmd.Columns) == 0 {
			t.Errorf("_cat/%s has no column", cmd.Name)
		}
	}
}

// TestEmbeddedDataHasNoCarriageReturn guards against a checkout that
// converted the embedded files to CRLF (see .gitattributes).
func TestEmbeddedDataHasNoCarriageReturn(t *testing.T) {
	for _, name := range []string{endpointsFile, catColumnsFile} {
		if strings.ContainsRune(string(mustReadEmbedded(name)), '\r') {
			t.Errorf("embedded %s contains CR characters", name)
		}
	}
}

func TestParseEndpoints(t *testing.T) {
	data := "# a comment\n\n_cat/indices?v\n  _health_report   @es >=8.7  \r\n_plugins/_ism/explain @opensearch\n_broken @nope\n_search\n"
	got, problems := parseEndpoints("endpoints.txt", []byte(data))

	var rendered []string
	for _, e := range got {
		rendered = append(rendered, strings.TrimSpace(e.Path+" "+e.Constraint.String()))
	}
	want := []string{"_cat/indices?v", "_health_report @es >=8.7", "_plugins/_ism/explain @opensearch", "_search"}
	if !reflect.DeepEqual(rendered, want) {
		t.Errorf("got %v, want %v", rendered, want)
	}

	// The malformed line is skipped and reported with its line number; the
	// lines after it still load.
	if len(problems) != 1 || problems[0].Line != 6 || problems[0].File != "endpoints.txt" {
		t.Errorf("expected exactly one problem at line 6, got %v", problems)
	}
}

func TestMergeEndpoints(t *testing.T) {
	parse := func(s string) []Endpoint {
		e, problems := parseEndpoints("test", []byte(s))
		if len(problems) != 0 {
			t.Fatalf("setup: %v", problems)
		}
		return e
	}
	embedded := parse("_cluster/health\n_health_report @es >=8.7\n_ilm/policy @es\n")
	team := parse("_health_report\n_my/team/endpoint\n")
	user := parse("_ilm/policy @es @opensearch\n_my/own/endpoint @opensearch >=2.0\n")

	var rendered []string
	for _, e := range mergeEndpoints(embedded, team, user) {
		rendered = append(rendered, strings.TrimSpace(e.Path+" "+e.Constraint.String()))
	}
	want := []string{
		"_cluster/health",
		// The team's untagged duplicate must not erase the embedded interval.
		"_health_report @es >=8.7",
		// The user's tagged duplicate replaces the embedded constraint.
		"_ilm/policy @es @opensearch",
		"_my/team/endpoint",
		"_my/own/endpoint @opensearch >=2.0",
	}
	if !reflect.DeepEqual(rendered, want) {
		t.Errorf("got  %v\nwant %v", rendered, want)
	}
}

func TestLoadMergesTeamAndUserFiles(t *testing.T) {
	teamDir, userDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(teamDir, endpointsFile), []byte("_team/only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, endpointsFile), []byte("_user/only @opensearch\n_bad @@\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := Load(Sources{TeamDir: teamDir, UserDir: userDir})

	onES := strings.Join(c.Endpoints(es("9.5.4")), "\n")
	onOS := strings.Join(c.Endpoints(opensearch("2.19.6")), "\n")
	for _, want := range []string{"_cluster/health", "_team/only"} {
		if !strings.Contains(onES, want) || !strings.Contains(onOS, want) {
			t.Errorf("expected %s on both distributions", want)
		}
	}
	if strings.Contains(onES, "_user/only") || !strings.Contains(onOS, "_user/only") {
		t.Error("expected _user/only on OpenSearch only")
	}

	if len(c.Problems) != 1 || c.Problems[0].Line != 2 || !strings.HasSuffix(c.Problems[0].File, endpointsFile) {
		t.Errorf("expected one problem (line 2 of the user's file), got %v", c.Problems)
	}
}

// TestLoadWithoutOptionalFiles checks the normal case of a bare binary:
// directories given, but nothing in them.
func TestLoadWithoutOptionalFiles(t *testing.T) {
	c := Load(Sources{TeamDir: t.TempDir(), UserDir: t.TempDir()})
	if len(c.Problems) != 0 {
		t.Errorf("missing optional files must not be reported, got %v", c.Problems)
	}
	if len(c.Endpoints(Target{})) != len(Load(Sources{}).Endpoints(Target{})) {
		t.Error("expected exactly the embedded endpoints")
	}
}

// TestLoadReportsUnreadableFile checks that a file that exists but can't be
// read is reported instead of silently ignored — here a directory sitting
// where the file is expected.
func TestLoadReportsUnreadableFile(t *testing.T) {
	userDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(userDir, endpointsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	c := Load(Sources{UserDir: userDir})
	if len(c.Problems) != 1 || c.Problems[0].Line != 0 {
		t.Errorf("expected one file-level problem, got %v", c.Problems)
	}
	if len(c.Endpoints(Target{})) == 0 {
		t.Error("the embedded endpoints should still be there")
	}
}

func TestProblemString(t *testing.T) {
	if got := (Problem{File: "f.txt", Line: 3, Message: "boom"}).String(); got != "f.txt:3: boom" {
		t.Errorf("got %q", got)
	}
	if got := (Problem{File: "f.txt", Message: "boom"}).String(); got != "f.txt: boom" {
		t.Errorf("got %q", got)
	}
}

func TestParseCatColumns(t *testing.T) {
	data := `# A general comment, then sections.

# _cat/indices
health
dataset.size @es >=8.19

# _cat/cluster_manager @opensearch
id
host

# _cat/ml/datafeeds @es
id
`
	commands, problems := parseCatColumns("cat_columns.txt", []byte(data))
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	c := &Catalog{catCommands: commands}

	wantES8 := map[string][]string{"indices": {"health"}, "ml/datafeeds": {"id"}}
	if got := c.CatColumns(es("8.0.1")); !reflect.DeepEqual(got, wantES8) {
		t.Errorf("ES 8.0.1: got %v, want %v", got, wantES8)
	}
	wantES9 := map[string][]string{"indices": {"health", "dataset.size"}, "ml/datafeeds": {"id"}}
	if got := c.CatColumns(es("9.5.4")); !reflect.DeepEqual(got, wantES9) {
		t.Errorf("ES 9.5.4: got %v, want %v", got, wantES9)
	}
	wantOS := map[string][]string{"indices": {"health"}, "cluster_manager": {"id", "host"}}
	if got := c.CatColumns(opensearch("2.19.6")); !reflect.DeepEqual(got, wantOS) {
		t.Errorf("OpenSearch: got %v, want %v", got, wantOS)
	}
}

func TestParseCatColumnsReportsMalformedTags(t *testing.T) {
	data := "# _cat/indices @nope\nhealth\n# _cat/shards\nindex @es >=x\nshard\n"
	commands, problems := parseCatColumns("cat_columns.txt", []byte(data))
	if len(problems) != 2 || problems[0].Line != 1 || problems[1].Line != 4 {
		t.Errorf("expected problems at lines 1 and 4, got %v", problems)
	}
	// The broken section is dropped with its columns; the next one loads.
	if len(commands) != 1 || commands[0].Name != "shards" || len(commands[0].Columns) != 1 {
		t.Errorf("got %+v", commands)
	}
}

// TestParseCatHelp runs over a "_cat/health?help" body captured from a real
// Elasticsearch 9.5.4 node.
func TestParseCatHelp(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "cat_health_help_es-9.5.4.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := ParseCatHelp(body)
	want := []string{
		"epoch", "timestamp", "cluster", "status", "node.total", "node.data",
		"shards", "pri", "relo", "init", "unassign", "unassign.pri",
		"pending_tasks", "max_task_wait_time", "active_shards_percent",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestParseCatListing(t *testing.T) {
	// As printed by a real Elasticsearch 8.11.4, glitch included: two
	// routes share the "component_templates" line.
	listing := "=^.^=\n/_cat/allocation\n/_cat/shards\n/_cat/shards/{index}\n/_cat/snapshots/{repository}\n" +
		"/_cat/component_templates/_cat/ml/anomaly_detectors\n/_cat/ml/anomaly_detectors/{job_id}\n" +
		"/_cat/pit_segments\n/_cat/pit_segments/{pit_id}\n/_cat/pit_segments/_all\n"
	got := strings.Join(ParseCatListing([]byte(listing)), " ")
	if want := "allocation component_templates ml/anomaly_detectors pit_segments shards snapshots"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestParseCatHelpIgnoresNonTableBodies(t *testing.T) {
	if got := ParseCatHelp([]byte(`{"error":"no handler found"}`)); len(got) != 0 {
		t.Errorf("expected nothing from a JSON error body, got %v", got)
	}
}

// TestParseCatHelpKeepsOnlyColumnNames checks that a column "name" which
// isn't one — terminal escapes, style tags, spaces, a request line — never
// comes out of an answer: what does is offered by completion and written
// into the editor.
func TestParseCatHelpKeepsOnlyColumnNames(t *testing.T) {
	body := "health | h | fine\n" +
		"\x1b]52;c;aGFjaw==\x07evil | e | clipboard escape\n" +
		"[red]tagged | t | style tag\n" +
		"two words | w | not a name\n" +
		"x&y=1 | q | would add a parameter\n" +
		" | empty | nothing\n" +
		"docs.count | dc | fine\n" +
		"segments.index_writer_memory | siwm | fine\n" +
		"node-role | nr | fine\n"
	got := ParseCatHelp([]byte(body))
	want := []string{"health", "docs.count", "segments.index_writer_memory", "node-role"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
