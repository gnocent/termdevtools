package refdata

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"termdevtools/parser"
)

// Tests of what a team's or user's hand-written file may get wrong: every
// mistake must be reported, none silently change what is offered.

// TestRecipeRequestsAreTheEditors checks that a recipe only counts as
// holding a request if the editor will see one: a line the editor's parser
// rejects (here, a trailing comment) would otherwise be inserted as a recipe
// that Ctrl+E can't run — it would run the request before it instead.
func TestRecipeRequestsAreTheEditors(t *testing.T) {
	recipes, problems := parseRecipes("mine.txt", []byte("# @recipe Health\nGET _cluster/health   # quick check\n"), true)
	if len(recipes) != 0 || len(problems) != 1 || !strings.Contains(problems[0].Message, "has no request") {
		t.Errorf("expected the recipe to be reported as holding no request, got %v, %v", recipes, problems)
	}

	recipes, _ = parseRecipes("mine.txt", []byte("# @recipe Two\n# intro\nGET _a  # not a request\nGET _b\n"), true)
	if len(recipes) != 1 {
		t.Fatalf("expected one recipe, got %v", recipes)
	}
	first := parser.ParseAll(recipes[0].Body)[0]
	if got := recipes[0].FirstRequestLine(); got != first.StartLine || first.Path != "_b" {
		t.Errorf("expected the cursor on the first request the editor sees (_b, line %d), got line %d", first.StartLine, got)
	}
}

func TestParseRecipesReportsWhatFallsOutsideRecipes(t *testing.T) {
	data := `# A comment before anything: fine.
GET _lost/before

# @recipe First
GET _a

# @group Other theme
# @tags lost
# @opensearch
GET _lost/between

# @recipe Second
GET _b
`
	recipes, problems := parseRecipes("mine.txt", []byte(data), true)
	if got := titles(recipes); !reflect.DeepEqual(got, []string{"mine / First", "Other theme / Second"}) {
		t.Errorf("recipes: got %v", got)
	}
	// The stray constraint must not have narrowed (or widened) anything.
	for _, r := range recipes {
		if !r.Constraint.IsZero() {
			t.Errorf("%q: expected no constraint, got %s", r.Title, r.Constraint)
		}
	}
	var lines []int
	for _, p := range problems {
		lines = append(lines, p.Line)
	}
	// @tags (8) and @opensearch (9) outside a recipe, then the first of the
	// requests that belong to none (2).
	if !reflect.DeepEqual(lines, []int{8, 9, 2}) {
		t.Errorf("problem lines: got %v (%v)", lines, problems)
	}
}

// TestRecipeConstraintWithoutSpace checks that "# @es>=8.0" restricts the
// recipe instead of being read as a comment, which would offer it
// everywhere.
func TestRecipeConstraintWithoutSpace(t *testing.T) {
	recipes, problems := parseRecipes("mine.txt", []byte("# @recipe Recent only\n# @es>=8.0\nGET _a\n"), true)
	if len(problems) != 0 || len(recipes) != 1 {
		t.Fatalf("got %v, %v", recipes, problems)
	}
	if got := recipes[0].Constraint.String(); got != "@es >=8.0" {
		t.Errorf("expected the constraint to be read, got %q", got)
	}
	if strings.Contains(recipes[0].Body, "@es") {
		t.Errorf("expected the directive to stay out of the body, got %q", recipes[0].Body)
	}
}

func TestParseConstraintRejectsEmptyInterval(t *testing.T) {
	for _, tags := range []string{"@es >=9.0 <8.0", "@opensearch >=2.4 <2.4", "@es @opensearch >=3.0 <2.0"} {
		if _, err := ParseConstraint(strings.Fields(tags)); err == nil || !strings.Contains(err.Error(), "empty interval") {
			t.Errorf("%q: expected an empty-interval error, got %v", tags, err)
		}
	}
	if _, err := ParseConstraint(strings.Fields("@es >=8.0 <9.0 @es >=9.2")); err != nil {
		t.Errorf("expected several valid intervals to be accepted, got %v", err)
	}
}

// TestUserFilesWithByteOrderMark checks files saved as "UTF-8 with BOM" by
// a Windows editor: the mark must not hide the first line.
func TestUserFilesWithByteOrderMark(t *testing.T) {
	const bom = "\xEF\xBB\xBF"
	userDir := t.TempDir()
	writeFile(t, filepath.Join(userDir, recipesDir, "mine.txt"), bom+"# @group Mine\n# @recipe First\nGET _first\n")
	writeFile(t, filepath.Join(userDir, endpointsFile), bom+"_my/endpoint\n")

	catalog := Load(Sources{UserDir: userDir})
	if len(catalog.Problems) != 0 {
		t.Fatalf("unexpected problems: %v", catalog.Problems)
	}
	found := false
	for _, r := range catalog.Recipes(Target{}) {
		found = found || (r.Group == "Mine" && r.Title == "First")
	}
	if !found {
		t.Error("expected the recipe of the file's first lines to be loaded")
	}
	if !contains(catalog.Endpoints(Target{}), "_my/endpoint") {
		t.Error("expected the endpoint of the file's first line to be loaded")
	}
}

// TestUserFileNotUTF8IsReported checks a file written by ">" in Windows
// PowerShell (UTF-16): one clear problem, and nothing loaded from it.
func TestUserFileNotUTF8IsReported(t *testing.T) {
	userDir := t.TempDir()
	path := filepath.Join(userDir, endpointsFile)
	data := []byte{0xFF, 0xFE}
	for _, unit := range utf16.Encode([]rune("_my/endpoint\r\n")) {
		data = append(data, byte(unit), byte(unit>>8))
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	builtIn := len(Load(Sources{}).Endpoints(Target{}))

	catalog := Load(Sources{UserDir: userDir})
	if len(catalog.Problems) != 1 || catalog.Problems[0].File != path || !strings.Contains(catalog.Problems[0].Message, "UTF-8") {
		t.Errorf("expected one problem naming the file's encoding, got %v", catalog.Problems)
	}
	if got := len(catalog.Endpoints(Target{})); got != builtIn {
		t.Errorf("expected nothing to be loaded from the file, got %d endpoints instead of %d", got, builtIn)
	}
}

// TestUserFileWithControlCharactersIsRefused checks that a file hiding a
// terminal escape sequence — which no editor shows — is turned down as a
// whole, with the line it is on.
func TestUserFileWithControlCharactersIsRefused(t *testing.T) {
	userDir := t.TempDir()
	recipes := filepath.Join(userDir, recipesDir, "mine.txt")
	writeFile(t, recipes, "# @recipe Fine\nGET _fine\n\n# @recipe Hidden\nGET _cluster/health\x1b[8m\n")
	writeFile(t, filepath.Join(userDir, endpointsFile), "_ok\t\r\n_fine\n")
	builtIn := len(Load(Sources{}).Recipes(Target{}))

	catalog := Load(Sources{UserDir: userDir})
	if len(catalog.Problems) != 1 {
		t.Fatalf("expected the recipe file alone to be a problem, got %v", catalog.Problems)
	}
	if refused := catalog.Problems[0]; refused.File != recipes || refused.Line != 5 || !strings.Contains(refused.Message, "control character") {
		t.Errorf("expected the recipe file to be refused for its line 5, got %v", refused)
	}
	if got := len(catalog.Recipes(Target{})); got != builtIn {
		t.Errorf("expected nothing to be loaded from the refused file, got %d recipes instead of %d", got, builtIn)
	}
	// Tabs and Windows line endings are ordinary text.
	if !contains(catalog.Endpoints(Target{}), "_fine") {
		t.Error("expected the file with a tab and a CRLF to be read")
	}
}

func TestSourcesReads(t *testing.T) {
	teamDir, userDir, elsewhere := t.TempDir(), t.TempDir(), t.TempDir()
	sources := Sources{TeamDir: teamDir, UserDir: userDir}

	for dir, want := range map[string]bool{
		teamDir:                                  true,
		userDir:                                  true,
		filepath.Join(userDir, ".", "sub", ".."): true, // the same directory, written differently
		elsewhere:                                false,
		filepath.Join(userDir, recipesDir):       false, // a subdirectory isn't read as a source itself
		filepath.Join(elsewhere, "not-yet"):      false,
	} {
		if dir == filepath.Join(userDir, ".", "sub", "..") {
			if err := os.Mkdir(filepath.Join(userDir, "sub"), 0o700); err != nil {
				t.Fatalf("Mkdir: %v", err)
			}
		}
		if got := sources.Reads(dir); got != want {
			t.Errorf("Reads(%q): got %v, want %v", dir, got, want)
		}
	}
	if (Sources{}).Reads(elsewhere) {
		t.Error("expected no source directory to mean nothing is read")
	}
}

// TestExportDefaultsLeavesNothingBehindOnFailure checks that an export that
// fails midway removes what it had written: a second attempt would otherwise
// be refused because of the first one's own files.
func TestExportDefaultsLeavesNothingBehindOnFailure(t *testing.T) {
	dir := t.TempDir()
	// A file where the export needs a directory.
	writeFile(t, filepath.Join(dir, recipesDir), "in the way\n")

	written, err := ExportDefaults(dir)
	if err == nil {
		t.Fatal("expected the export to fail")
	}
	if len(written) != 0 {
		t.Errorf("expected no file to be reported as written, got %v", written)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("ReadDir: %v", readErr)
	}
	if len(entries) != 1 || entries[0].Name() != recipesDir {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected only the pre-existing file to remain, got %v", names)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
