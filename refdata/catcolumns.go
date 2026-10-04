package refdata

import (
	"regexp"
	"sort"
	"strings"
)

// CatCommand is one _cat command ("indices", "ml/datafeeds"...) with the
// columns its h= and s= parameters accept.
type CatCommand struct {
	Name       string
	Constraint Constraint
	Columns    []CatColumn
}

// CatColumn is one column of a _cat command and the clusters it exists on.
type CatColumn struct {
	Name       string
	Constraint Constraint
}

// catSectionRe recognizes a section header in the _cat columns file:
// "# _cat/indices", optionally followed by constraint tags.
var catSectionRe = regexp.MustCompile(`^#\s*_cat/(\S+)(.*)$`)

// parseCatColumns reads the _cat columns file: "# _cat/command" sections,
// each followed by one column per line; a section or a column may carry
// constraint tags (see Constraint). Any other line starting with '#' is a
// comment.
//
// This table is only a fallback: the columns actually offered are asked from
// the connected cluster ("_cat/command?help"), which is exact for any
// version. It is embedded, never read from a user file.
func parseCatColumns(file string, data []byte) (commands []CatCommand, problems []Problem) {
	current := -1
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := catSectionRe.FindStringSubmatch(line); m != nil {
			constraint, err := ParseConstraint(strings.Fields(m[2]))
			if err != nil {
				problems = append(problems, Problem{File: file, Line: i + 1, Message: err.Error()})
				current = -1
				continue
			}
			commands = append(commands, CatCommand{Name: m[1], Constraint: constraint})
			current = len(commands) - 1
			continue
		}
		if strings.HasPrefix(line, "#") || current < 0 {
			continue
		}
		name, constraint, err := splitAnnotated(line)
		if err != nil {
			problems = append(problems, Problem{File: file, Line: i + 1, Message: err.Error()})
			continue
		}
		commands[current].Columns = append(commands[current].Columns, CatColumn{Name: name, Constraint: constraint})
	}
	return commands, problems
}

// ParseCatListing extracts the command names from the body of a "GET _cat"
// response, which lists the cluster's _cat routes one per line
// ("/_cat/shards", "/_cat/shards/{index}"...): each command once, without
// its parametric variants, sorted.
//
// Routes are located by their "/_cat/" prefix rather than line by line:
// some Elasticsearch 8.x releases print two of them on the same line
// ("/_cat/component_templates/_cat/ml/anomaly_detectors").
func ParseCatListing(body []byte) []string {
	seen := make(map[string]bool)
	var commands []string
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Split(strings.TrimSpace(line), "/_cat/")
		for _, cmd := range parts[1:] {
			if i := strings.Index(cmd, "/{"); i >= 0 {
				cmd = cmd[:i]
			}
			// "{...}": nothing but a parameter; ".../_all": a fixed-value
			// variant of another route (pit_segments/_all).
			if cmd == "" || strings.HasPrefix(cmd, "{") || strings.HasSuffix(cmd, "/_all") || seen[cmd] {
				continue
			}
			seen[cmd] = true
			commands = append(commands, cmd)
		}
	}
	sort.Strings(commands)
	return commands
}

// ParseCatHelp extracts the column names from the body of a
// "_cat/command?help" response: one column per line, as
// "name | aliases | description". Only full names are kept, not the short
// aliases ("dc" for "docs.count"): more descriptive in a suggestion list,
// and fewer entries for commands with a hundred columns.
//
// A name that isn't made of the characters column names are made of is
// dropped: what is kept is offered in a list and, once chosen, written into
// the editor — the answer of a cluster is not to be trusted with that.
func ParseCatHelp(body []byte) []string {
	var columns []string
	for _, line := range strings.Split(string(body), "\n") {
		name, _, found := strings.Cut(line, "|")
		if !found {
			continue
		}
		if name = strings.TrimSpace(name); catColumnNameRe.MatchString(name) {
			columns = append(columns, name)
		}
	}
	return columns
}

// catColumnNameRe is what a _cat column name looks like ("docs.count",
// "node.role", "segments.index_writer_memory").
var catColumnNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.\-]*$`)
