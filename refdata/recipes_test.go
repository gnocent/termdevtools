package refdata

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"termdevtools/parser"
)

func titles(recipes []Recipe) []string {
	var out []string
	for _, r := range recipes {
		out = append(out, r.Group+" / "+r.Title)
	}
	return out
}

func TestParseRecipes(t *testing.T) {
	data := `# A file header, ignored.

# @group Shards and allocation

# @recipe Why is a shard unassigned?
# @tags Unassigned, red yellow
# @es >=7.17
# @opensearch
# Explains the first unassigned shard.
GET _cluster/allocation/explain

# @recipe Retry failed shards
POST _cluster/reroute?retry_failed=true

# @group Indices
# @recipe Count documents
# @timestamp is the usual time field: an ordinary comment, kept.
GET ${index}/_count
{
  "query": { "match_all": {} }
}
`
	recipes, problems := parseRecipes("recipes/demo.txt", []byte(data), false)
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	want := []string{
		"Shards and allocation / Why is a shard unassigned?",
		"Shards and allocation / Retry failed shards",
		"Indices / Count documents",
	}
	if got := titles(recipes); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	first := recipes[0]
	if !reflect.DeepEqual(first.Tags, []string{"unassigned", "red", "yellow"}) {
		t.Errorf("tags: got %v", first.Tags)
	}
	if got := first.Constraint.String(); got != "@es >=7.17 @opensearch" {
		t.Errorf("constraint: got %q", got)
	}
	// Directives are metadata, not content; the explanation is content.
	if want := "# Explains the first unassigned shard.\nGET _cluster/allocation/explain"; first.Body != want {
		t.Errorf("body: got %q, want %q", first.Body, want)
	}
	if first.Source != "" {
		t.Errorf("an embedded recipe has no source, got %q", first.Source)
	}
	if got := first.FirstRequestLine(); got != 1 {
		t.Errorf("FirstRequestLine: got %d, want 1", got)
	}

	last := recipes[2]
	if !strings.Contains(last.Body, "# @timestamp is the usual time field") {
		t.Errorf("a comment starting with an unknown @word must stay in the body, got %q", last.Body)
	}
	if !strings.HasSuffix(last.Body, "}") {
		t.Errorf("the JSON body must be kept whole, got %q", last.Body)
	}
}

func TestParseRecipesFileLevelConstraint(t *testing.T) {
	data := `# @opensearch
# @recipe Lifecycle policies
GET _plugins/_ism/policies

# @recipe With its own constraint
# @opensearch >=2.18
GET _list/indices?v
`
	recipes, problems := parseRecipes("ism.txt", []byte(data), false)
	if len(problems) != 0 || len(recipes) != 2 {
		t.Fatalf("got %v, %v", recipes, problems)
	}
	if got := recipes[0].Constraint.String(); got != "@opensearch" {
		t.Errorf("the file-level constraint should apply, got %q", got)
	}
	if got := recipes[1].Constraint.String(); got != "@opensearch >=2.18" {
		t.Errorf("a recipe's own constraint should win, got %q", got)
	}
	// Without @group, recipes are filed under the file's name.
	if recipes[0].Group != "ism" {
		t.Errorf("group: got %q, want the file's name", recipes[0].Group)
	}
}

// TestParseRecipesWholeFile checks the forgiving case: a plain file of
// requests, with no directive at all, is one recipe named after the file.
func TestParseRecipesWholeFile(t *testing.T) {
	data := "# my favourites\nGET _cat/indices?v\n\nGET _cat/shards?v\n"
	recipes, problems := parseRecipes(filepath.Join("some", "dir", "favourites.txt"), []byte(data), true)
	if len(problems) != 0 || len(recipes) != 1 {
		t.Fatalf("got %v, %v", recipes, problems)
	}
	r := recipes[0]
	if r.Title != "favourites" || r.Group != "favourites" {
		t.Errorf("got title %q, group %q", r.Title, r.Group)
	}
	if r.Body != "# my favourites\nGET _cat/indices?v\n\nGET _cat/shards?v" {
		t.Errorf("body: got %q", r.Body)
	}
	if r.Source != filepath.Join("some", "dir", "favourites.txt") {
		t.Errorf("a user's recipe records its file, got %q", r.Source)
	}
}

func TestParseRecipesIgnoresCommentOnlyFile(t *testing.T) {
	recipes, problems := parseRecipes("template.txt", []byte("# Nothing but comments.\n# # @recipe An example\n# GET _cluster/health\n"), true)
	if len(recipes) != 0 || len(problems) != 0 {
		t.Errorf("got %v, %v", recipes, problems)
	}
}

func TestParseRecipesReportsMistakes(t *testing.T) {
	data := `# @recipe
GET _a

# @recipe Empty one
# only a comment

# @recipe Bad tag
# @es >=nope
GET _b

# @recipe Fine
GET _c
`
	recipes, problems := parseRecipes("bad.txt", []byte(data), true)
	if got := titles(recipes); !reflect.DeepEqual(got, []string{"bad / Bad tag", "bad / Fine"}) {
		t.Errorf("recipes: got %v", got)
	}
	var lines []int
	for _, p := range problems {
		lines = append(lines, p.Line)
	}
	// Missing title (1), recipe without request (4), malformed tag (8).
	if !reflect.DeepEqual(lines, []int{1, 4, 8}) {
		t.Errorf("problem lines: got %v (%v)", lines, problems)
	}
}

func TestParseRecipesNormalizesCRLF(t *testing.T) {
	recipes, _ := parseRecipes("crlf.txt", []byte("# @recipe Health\r\n# comment\r\nGET _cluster/health\r\n"), true)
	if len(recipes) != 1 || strings.ContainsRune(recipes[0].Body, '\r') || recipes[0].Title != "Health" {
		t.Errorf("got %+v", recipes)
	}
}

func TestMergeRecipes(t *testing.T) {
	parse := func(file, s string) []Recipe {
		r, problems := parseRecipes(file, []byte(s), file != "builtin.txt")
		if len(problems) != 0 {
			t.Fatalf("setup: %v", problems)
		}
		return r
	}
	builtin := parse("builtin.txt", `# @group Overview
# @recipe Nodes
# @es
GET _cat/nodes?v&h=name,master
# @recipe Nodes
# @opensearch
GET _cat/nodes?v&h=name,cluster_manager
# @recipe Health
GET _cluster/health
# @group Indices
# @recipe Largest
GET _cat/indices?v&s=store.size:desc
`)
	user := parse("mine.txt", `# @group overview
# @recipe nodes
GET _cat/nodes?v&h=name,ip
# @recipe My check
GET _cat/health?v
# @group Mine
# @recipe Favourite
GET _cat/count?v
`)

	merged := mergeRecipes(builtin, user)
	want := []string{
		"Overview / Health",
		// Both built-in variants of "Nodes" are replaced by the user's one
		// (group and title compare without case).
		"overview / nodes",
		"overview / My check",
		"Indices / Largest",
		"Mine / Favourite",
	}
	if got := titles(merged); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// TestLoadMergesRecipeDirectories checks the three layers end to end.
func TestLoadMergesRecipeDirectories(t *testing.T) {
	teamDir, userDir := t.TempDir(), t.TempDir()
	write := func(dir, name, content string) {
		t.Helper()
		path := filepath.Join(dir, recipesDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(teamDir, "team.txt", "# @group Team\n# @recipe Team check\nGET _team\n")
	write(userDir, "mine.txt", "# @group Team\n# @recipe Mine\n# @opensearch\nGET _mine\n# @recipe Broken\n")
	write(userDir, "notes.md", "# @recipe Not a recipe file\nGET _ignored\n")

	c := Load(Sources{TeamDir: teamDir, UserDir: userDir})

	builtin := len(Load(Sources{}).Recipes(es("9.5.4")))
	onES := c.Recipes(es("9.5.4"))
	if len(onES) != builtin+1 || onES[len(onES)-1].Title != "Team check" {
		t.Errorf("Elasticsearch: expected the built-in recipes plus the team's, got %d (built-in: %d)", len(onES), builtin)
	}
	onOS := titles(c.Recipes(opensearch("2.19.6")))
	if n := len(onOS); n < 2 || onOS[n-2] != "Team / Team check" || onOS[n-1] != "Team / Mine" {
		t.Errorf("OpenSearch: expected the team's then the user's recipe at the end, got %v", onOS)
	}

	if len(c.Problems) != 1 || !strings.HasSuffix(c.Problems[0].File, "mine.txt") {
		t.Errorf("expected one problem in mine.txt, got %v", c.Problems)
	}
}

// Variables the built-in recipes may refer to: a closed vocabulary, so that
// the same few names serve across the whole catalog (and so that the
// integration tests can give each a value).
var recipeVariables = map[string]string{
	"index":      "my-index",
	"node":       "node-1",
	"repository": "my-repository",
	"snapshot":   "my-snapshot",
	"field":      "status",
	"task_id":    "oTUltX4IQMOUUVeiohTt8A:12345",
}

var variableRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// TestEmbeddedRecipes checks every recipe built into the binary, as far as
// it can be checked without a cluster: at least one request, readable by
// the editor's own parser, with a valid JSON body, and only known variables.
// (Whether the requests actually work is the integration tests' job.)
func TestEmbeddedRecipes(t *testing.T) {
	recipes := Load(Sources{}).Recipes(Target{})
	if len(recipes) < 50 {
		t.Fatalf("expected a rich catalog, got only %d recipes", len(recipes))
	}

	seen := make(map[string]bool)
	for _, r := range recipes {
		name := r.Group + " / " + r.Title
		// The same title may come in one variant per distribution, never
		// twice for the same clusters.
		id := name + " " + r.Constraint.String()
		if seen[id] {
			t.Errorf("%s: defined twice with the same constraint", name)
		}
		seen[id] = true

		if strings.ContainsRune(r.Body, '\r') {
			t.Errorf("%s: body contains CR characters", name)
		}
		for _, m := range variableRe.FindAllStringSubmatch(r.Body, -1) {
			if _, known := recipeVariables[m[1]]; !known {
				t.Errorf("%s: unknown variable ${%s}", name, m[1])
			}
		}

		resolved := variableRe.ReplaceAllStringFunc(r.Body, func(v string) string {
			return recipeVariables[v[2:len(v)-1]]
		})
		requests := parser.ParseAll(resolved)
		if len(requests) == 0 {
			t.Errorf("%s: no request found by the editor's parser", name)
		}
		for _, req := range requests {
			if err := parser.ValidateBody(req.Body); err != nil {
				t.Errorf("%s: %s %s: invalid body: %v", name, req.Method, req.Path, err)
			}
		}
	}
}

func TestStarter(t *testing.T) {
	starter := Starter()
	if strings.ContainsRune(starter, '\r') {
		t.Error("the starter contains CR characters")
	}
	requests := parser.ParseAll(starter)
	if len(requests) < 3 {
		t.Errorf("expected a few requests in the starter, got %d", len(requests))
	}
	if !strings.Contains(starter, "F8") {
		t.Error("the starter should point at the recipe catalog (F8)")
	}
}
