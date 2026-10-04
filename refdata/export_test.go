package refdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUserTemplateAddsNothing guards the template's main property: written
// as is into the user's directory, it must yield no recipe and no problem —
// including through a comment line that would happen to read as a directive.
func TestUserTemplateAddsNothing(t *testing.T) {
	data := mustReadEmbedded(userTemplateFile)
	recipes, problems := parseRecipes(userTemplateFile, data, true)
	if len(recipes) != 0 || len(problems) != 0 {
		t.Errorf("expected the template to be inert, got %d recipes and problems %v", len(recipes), problems)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if directiveRe.MatchString(strings.TrimSpace(line)) {
			t.Errorf("line %d reads as a directive: %q", i+1, line)
		}
		if strings.Contains(line, "\r") {
			t.Errorf("line %d has a carriage return", i+1)
		}
	}
}

// TestUserTemplateExampleWorks checks the example the template ends with:
// uncommented the way the template says, it must be a valid recipe.
func TestUserTemplateExampleWorks(t *testing.T) {
	text := string(mustReadEmbedded(userTemplateFile))
	_, example, found := strings.Cut(text, "to try it:\n")
	if !found {
		t.Fatal("the template's example marker has changed: update this test")
	}
	var lines []string
	for _, line := range strings.Split(example, "\n") {
		lines = append(lines, strings.TrimPrefix(line, "# "))
	}

	recipes, problems := parseRecipes(userTemplateFile, []byte(strings.Join(lines, "\n")), true)
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if len(recipes) != 1 {
		t.Fatalf("expected the example to be one recipe, got %d", len(recipes))
	}
	r := recipes[0]
	if r.Group != "My team" || r.Title != "Orders still pending" || len(r.Tags) != 2 {
		t.Errorf("unexpected recipe: group %q, title %q, tags %v", r.Group, r.Title, r.Tags)
	}
	if !strings.Contains(r.Body, "GET orders-*/_search") || r.FirstRequestLine() != 1 {
		t.Errorf("unexpected body (first request at line %d):\n%s", r.FirstRequestLine(), r.Body)
	}
}

func TestCreateUserTemplateOnlyTheFirstTime(t *testing.T) {
	userDir := t.TempDir()
	path := filepath.Join(userDir, recipesDir, userTemplateFile)

	if err := CreateUserTemplate(userDir); err != nil {
		t.Fatalf("CreateUserTemplate: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the template to be created: %v", err)
	}
	if problems := Load(Sources{UserDir: userDir}).Problems; len(problems) != 0 {
		t.Errorf("expected the freshly created directory to load cleanly, got %v", problems)
	}

	// Deleted by the user: not written again, since the directory remains.
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := CreateUserTemplate(userDir); err != nil {
		t.Fatalf("CreateUserTemplate (second call): %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected the template not to come back once deleted (stat: %v)", err)
	}

	if err := CreateUserTemplate(""); err != nil {
		t.Errorf("expected no directory to mean nothing to do, got %v", err)
	}
}

func TestExportDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "defaults")
	written, err := ExportDefaults(dir)
	if err != nil {
		t.Fatalf("ExportDefaults: %v", err)
	}

	for _, name := range []string{endpointsFile, catColumnsFile, starterFile, userTemplateFile, filepath.Join(recipesDir, "10-overview.txt")} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("expected %s to be exported: %v", name, err)
			continue
		}
		if want := mustReadEmbedded(filepath.ToSlash(name)); string(got) != string(want) {
			t.Errorf("%s: exported content differs from the embedded one", name)
		}
	}
	if len(written) < 10 {
		t.Errorf("expected every embedded file to be reported, got %v", written)
	}

	// The exported recipes are valid team/user files as they are.
	exported := Load(Sources{UserDir: dir})
	if len(exported.Problems) != 0 {
		t.Errorf("expected the exported files to load cleanly, got %v", exported.Problems)
	}
}

// TestExportDefaultsNeverOverwrites checks that an existing file — possibly
// edited since an earlier export — blocks the whole export, untouched.
func TestExportDefaultsNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, endpointsFile)
	if err := os.WriteFile(mine, []byte("_mine\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	written, err := ExportDefaults(dir)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected an \"already exists\" error, got %v", err)
	}
	if len(written) != 0 {
		t.Errorf("expected nothing to be written, got %v", written)
	}
	if got, _ := os.ReadFile(mine); string(got) != "_mine\n" {
		t.Errorf("expected the existing file to be left alone, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, starterFile)); !os.IsNotExist(err) {
		t.Errorf("expected no other file to be written (stat: %v)", err)
	}
}
