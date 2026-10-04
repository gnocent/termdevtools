package refdata

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// embedded is the reference data compiled into the binary: what makes the
// binary self-sufficient, with nothing to install next to it.
//
//go:embed data
var embedded embed.FS

// File names, identical in the embedded data and in the two optional
// directories that extend it (see Sources).
const (
	endpointsFile  = "endpoints.txt"
	catColumnsFile = "cat_columns.txt" // embedded only, see parseCatColumns
	starterFile    = "starter.txt"     // embedded only, see Starter
	recipesDir     = "recipes"         // a directory of *.txt files
)

// Sources locates the optional files that extend the embedded data. Either
// directory may be empty (not looked at) or simply lack the files.
type Sources struct {
	// TeamDir is the binary's own directory: content shared by everyone
	// running that installation.
	TeamDir string
	// UserDir is the current user's configuration directory.
	UserDir string
}

// Reads reports whether dir is one of the directories these sources are
// read from: files written there would be loaded on top of the built-in
// data (see ExportDefaults).
func (s Sources) Reads(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false // doesn't exist yet: neither of them, both are in use
	}
	for _, source := range []string{s.TeamDir, s.UserDir} {
		if source == "" {
			continue
		}
		if sourceInfo, err := os.Stat(source); err == nil && os.SameFile(info, sourceInfo) {
			return true
		}
	}
	return false
}

// Problem is something wrong in a reference file that didn't prevent the
// rest of it from loading.
type Problem struct {
	File    string
	Line    int // 0 when the problem concerns the file as a whole
	Message string
}

func (p Problem) String() string {
	if p.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Message)
	}
	return fmt.Sprintf("%s: %s", p.File, p.Message)
}

// Catalog is the reference data in effect: the embedded data extended by
// the team's and the user's files, before any selection by cluster.
type Catalog struct {
	endpoints   []Endpoint
	catCommands []CatCommand
	recipes     []Recipe
	// Problems lists what was skipped while loading the team's and user's
	// files, for the caller to report.
	Problems []Problem
}

// Load builds the catalog: embedded data first, then the team's files, then
// the user's — each layer adding to the ones below (see mergeEndpoints).
// Never fails: a file that can't be read or a line that can't be parsed
// becomes a Problem, and the rest still loads.
func Load(src Sources) *Catalog {
	c := &Catalog{}

	layers := [][]Endpoint{mustParseEmbeddedEndpoints()}
	for _, dir := range []string{src.TeamDir, src.UserDir} {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, endpointsFile)
		data, ok := c.readOptional(path)
		if !ok {
			continue
		}
		endpoints, problems := parseEndpoints(path, data)
		c.Problems = append(c.Problems, problems...)
		layers = append(layers, endpoints)
	}
	c.endpoints = mergeEndpoints(layers...)

	c.catCommands = mustParseEmbeddedCatColumns()

	recipeLayers := [][]Recipe{mustParseEmbeddedRecipes()}
	for _, dir := range []string{src.TeamDir, src.UserDir} {
		if dir == "" {
			continue
		}
		recipeLayers = append(recipeLayers, c.readRecipeDir(filepath.Join(dir, recipesDir)))
	}
	c.recipes = mergeRecipes(recipeLayers...)

	return c
}

// readRecipeDir reads every *.txt file of a team's or user's recipe
// directory, in name order. A missing directory is the normal case.
func (c *Catalog) readRecipeDir(dir string) []Recipe {
	entries, err := os.ReadDir(dir) // sorted by name
	if err != nil {
		if !os.IsNotExist(err) {
			c.Problems = append(c.Problems, Problem{File: dir, Message: err.Error()})
		}
		return nil
	}
	var recipes []Recipe
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".txt") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, ok := c.readOptional(path)
		if !ok {
			continue
		}
		parsed, problems := parseRecipes(path, data, true)
		c.Problems = append(c.Problems, problems...)
		recipes = append(recipes, parsed...)
	}
	return recipes
}

// Recipes returns the recipes that apply to t, grouped by theme in the
// order the palette lists them.
func (c *Catalog) Recipes(t Target) []Recipe {
	var recipes []Recipe
	for _, r := range c.recipes {
		if r.Constraint.Matches(t) {
			recipes = append(recipes, r)
		}
	}
	return recipes
}

// Starter is the content given to the editor the first time a cluster is
// connected to (nothing saved for it yet): a handful of requests that work
// on any cluster, and where to find the rest.
func Starter() string {
	return string(mustReadEmbedded(starterFile))
}

func mustParseEmbeddedRecipes() []Recipe {
	entries, err := embedded.ReadDir("data/" + recipesDir) // sorted by name
	if err != nil {
		panic(fmt.Sprintf("refdata: embedded %s: %v", recipesDir, err))
	}
	var recipes []Recipe
	for _, entry := range entries {
		name := recipesDir + "/" + entry.Name()
		parsed, problems := parseRecipes(name, mustReadEmbedded(name), false)
		if len(problems) > 0 {
			panic(fmt.Sprintf("refdata: embedded %s", problems[0]))
		}
		recipes = append(recipes, parsed...)
	}
	return recipes
}

// readOptional reads a file that may legitimately not exist. Any other
// failure is recorded as a Problem — including a file that isn't UTF-8 text
// (">" in Windows PowerShell writes UTF-16): read line by line, it would
// only yield garbage entries and misleading problems.
func (c *Catalog) readOptional(path string) ([]byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			c.Problems = append(c.Problems, Problem{File: path, Message: err.Error()})
		}
		return nil, false
	}
	// The byte order mark some Windows editors put in front of UTF-8 text
	// would otherwise stick to the first line and hide what it says.
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		c.Problems = append(c.Problems, Problem{File: path, Message: "not a UTF-8 text file: ignored (save it as UTF-8)"})
		return nil, false
	}
	// Nothing a recipe or an endpoint is made of: a terminal escape sequence
	// hidden in a file of a shared directory would end up, unseen, in the
	// requests it is inserted into.
	if line := controlCharacterLine(data); line > 0 {
		c.Problems = append(c.Problems, Problem{File: path, Line: line, Message: "control character in the file: ignored as a whole"})
		return nil, false
	}
	return data, true
}

// controlCharacterLine returns the number of the first line of data holding
// a control character other than a tab or an end of line, 0 if there is none.
func controlCharacterLine(data []byte) int {
	line := 1
	for _, r := range string(data) {
		switch {
		case r == '\n':
			line++
		case r == '\t', r == '\r':
		case r < 0x20, r == 0x7f, r >= 0x80 && r < 0xa0:
			return line
		}
	}
	return 0
}

// Endpoints returns the completion candidates that apply to t.
func (c *Catalog) Endpoints(t Target) []string {
	var paths []string
	for _, e := range c.endpoints {
		if e.Constraint.Matches(t) {
			paths = append(paths, e.Path)
		}
	}
	return paths
}

// CatColumns returns, for each _cat command that exists on t, the columns
// known for it there — the fallback used when the cluster itself can't be
// asked (see parseCatColumns). The map's keys are also what lets a typed
// path be recognized as a _cat command in the first place.
func (c *Catalog) CatColumns(t Target) map[string][]string {
	table := make(map[string][]string)
	for _, cmd := range c.catCommands {
		if !cmd.Constraint.Matches(t) {
			continue
		}
		columns := make([]string, 0, len(cmd.Columns))
		for _, col := range cmd.Columns {
			if col.Constraint.Matches(t) {
				columns = append(columns, col.Name)
			}
		}
		table[cmd.Name] = columns
	}
	return table
}

func mustParseEmbeddedCatColumns() []CatCommand {
	commands, problems := parseCatColumns(catColumnsFile, mustReadEmbedded(catColumnsFile))
	if len(problems) > 0 {
		panic(fmt.Sprintf("refdata: embedded %s", problems[0]))
	}
	return commands
}

func mustReadEmbedded(name string) []byte {
	data, err := embedded.ReadFile("data/" + name)
	if err != nil {
		// Only possible if the file was removed from the source tree: a
		// build-time mistake, caught by this package's tests.
		panic(fmt.Sprintf("refdata: embedded %s: %v", name, err))
	}
	return data
}

func mustParseEmbeddedEndpoints() []Endpoint {
	endpoints, problems := parseEndpoints(endpointsFile, mustReadEmbedded(endpointsFile))
	if len(problems) > 0 {
		panic(fmt.Sprintf("refdata: embedded %s", problems[0]))
	}
	return endpoints
}
