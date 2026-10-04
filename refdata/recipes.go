package refdata

import (
	"path/filepath"
	"regexp"
	"strings"

	"termdevtools/parser"
)

// Recipe is a ready-made request, or short sequence of requests, with the
// comments explaining it — what the recipe palette offers to insert into
// the editor (SPEC.md §3.2).
type Recipe struct {
	// Group is the theme the recipe is filed under ("Shards and allocation").
	Group string
	Title string
	// Tags are extra search keywords, lowercase.
	Tags       []string
	Constraint Constraint
	// Body is what gets inserted: comment lines and requests, in the
	// editor's own syntax.
	Body string
	// Source is the file a team's or user's recipe comes from, empty for
	// the recipes built into the binary.
	Source string
}

// key identifies a recipe across layers: same group and title (ignoring
// case) means "the same recipe, redefined".
func (r Recipe) key() string {
	return strings.ToLower(r.Group) + "\x00" + strings.ToLower(r.Title)
}

// directiveRe recognizes a "# @name argument" comment line. No space is
// required after the name: "# @es>=8.0" is read as the constraint it was
// meant to be — and reported if it can't be — rather than silently taken for
// a comment, which would offer the recipe everywhere.
var directiveRe = regexp.MustCompile(`^#\s*@([A-Za-z_]+)\s*(.*)$`)

// parseRecipes reads a recipe file. Its format is the editor's own —
// comments and requests — plus a few directives written as comments:
//
//	# @group Shards and allocation     theme of the recipes that follow
//	# @recipe Why is a shard unassigned?   starts a recipe, up to the next one
//	# @tags unassigned red yellow      extra search keywords (optional)
//	# @es >=8.0                        clusters it applies to (optional, see
//	# @opensearch                      Constraint); before the first @recipe,
//	                                   the default for the whole file
//
// Everything else between two @recipe lines is the recipe's body. Without
// @group, recipes are filed under the file's name; a file with no @recipe
// at all is one recipe, named after the file. Any other "# @word" comment
// is an ordinary comment.
//
// What a file with @recipe directives holds outside of them would be
// dropped: a request, @tags or a constraint written there (other than the
// file-wide constraint before the first @recipe) is reported instead.
//
// fromUser tells apart the files of a team or user (their Source is
// recorded) from the embedded ones.
func parseRecipes(file string, data []byte, fromUser bool) (recipes []Recipe, problems []Problem) {
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	source := ""
	if fromUser {
		source = file
	}

	group := name
	var fileConstraint Constraint
	// What belongs to no recipe — before the first @recipe, or between an
	// @group and the next @recipe — with the line number of each line.
	var outside []string
	var outsideLines []int

	var current *Recipe
	var body []string
	startLine := 0
	explicit := false // an @recipe directive was seen
	// discarding is set after an @recipe that couldn't be opened (no title):
	// its content, already reported with it, isn't reported again line by
	// line.
	discarding := false
	keep := func(line string, number int) {
		switch {
		case current != nil:
			body = append(body, line)
		case !discarding:
			outside = append(outside, line)
			outsideLines = append(outsideLines, number)
		}
	}

	flush := func() {
		if current == nil {
			return
		}
		current.Body = trimBlankLines(body)
		switch {
		case hasRequest(body):
			if current.Constraint.IsZero() {
				current.Constraint = fileConstraint
			}
			recipes = append(recipes, *current)
		case explicit:
			problems = append(problems, Problem{File: file, Line: startLine, Message: "recipe \"" + current.Title + "\" has no request"})
		}
		current, body = nil, nil
	}

	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r \t")
		m := directiveRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			keep(line, i+1)
			continue
		}

		directive, arg := strings.ToLower(m[1]), strings.TrimSpace(m[2])
		switch directive {
		case "group":
			flush()
			discarding = false
			if arg == "" {
				problems = append(problems, Problem{File: file, Line: i + 1, Message: "@group needs a name"})
				continue
			}
			group = arg
		case "recipe":
			flush()
			explicit = true
			discarding = arg == ""
			if discarding {
				problems = append(problems, Problem{File: file, Line: i + 1, Message: "@recipe needs a title"})
				continue
			}
			current = &Recipe{Group: group, Title: arg, Source: source}
			startLine = i + 1
		case "tags":
			if current == nil {
				if !discarding {
					problems = append(problems, Problem{File: file, Line: i + 1, Message: "@tags outside a recipe: write it after the recipe's @recipe line"})
				}
				continue
			}
			current.Tags = append(current.Tags, strings.FieldsFunc(strings.ToLower(arg), func(r rune) bool {
				return r == ',' || r == ' ' || r == '\t'
			})...)
		case "es", "elasticsearch", "opensearch", "os":
			c, err := ParseConstraint(strings.Fields("@" + directive + " " + arg))
			if err != nil {
				problems = append(problems, Problem{File: file, Line: i + 1, Message: err.Error()})
				continue
			}
			target := &fileConstraint
			switch {
			case current != nil:
				target = &current.Constraint
			case explicit:
				// Between two recipes (after an @group): it would silently
				// widen the whole file's default.
				if !discarding {
					problems = append(problems, Problem{File: file, Line: i + 1, Message: "@" + directive + " outside a recipe: write it before the first @recipe (whole file) or after a recipe's @recipe line"})
				}
				continue
			}
			target.ES = append(target.ES, c.ES...)
			target.OS = append(target.OS, c.OS...)
		default:
			// Not a directive of ours ("# @timestamp is the event time"):
			// an ordinary comment.
			keep(line, i+1)
		}
	}
	flush()

	switch requests := parser.ParseAll(strings.Join(outside, "\n")); {
	case len(requests) == 0:
		// Comments only (like the template created in the user's directory):
		// nothing.
	case explicit:
		problems = append(problems, Problem{File: file, Line: outsideLines[requests[0].StartLine], Message: "request outside a recipe: start one with \"# @recipe <title>\" above it"})
	default:
		// No @recipe anywhere: the file as a whole is one recipe.
		recipes = append(recipes, Recipe{
			Group: group, Title: name, Constraint: fileConstraint,
			Body: trimBlankLines(outside), Source: source,
		})
	}
	return recipes, problems
}

// hasRequest reports whether lines hold something the editor can run —
// judged by the editor's own parser, so that a recipe accepted here is one
// Ctrl+E will find once inserted.
func hasRequest(lines []string) bool {
	return len(parser.ParseAll(strings.Join(lines, "\n"))) > 0
}

// trimBlankLines joins lines without the blank ones at either end.
func trimBlankLines(lines []string) string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

// mergeRecipes stacks layers, lowest priority first (embedded, team, user).
// A layer's recipes are added at the end of their group — so that a user's
// additions sit with the built-in recipes on the same theme — or as a new
// group after the others. A recipe with the same group and title as recipes
// of a lower layer replaces them: all of them, since one title may exist
// there in several variants (one per distribution).
func mergeRecipes(layers ...[]Recipe) []Recipe {
	var groups []string
	byGroup := make(map[string][]Recipe)

	for _, layer := range layers {
		redefined := make(map[string]bool, len(layer))
		for _, r := range layer {
			redefined[r.key()] = true
		}
		for g, existing := range byGroup {
			kept := make([]Recipe, 0, len(existing))
			for _, r := range existing {
				if !redefined[r.key()] {
					kept = append(kept, r)
				}
			}
			byGroup[g] = kept
		}
		for _, r := range layer {
			g := strings.ToLower(r.Group)
			if _, known := byGroup[g]; !known {
				groups = append(groups, g)
			}
			byGroup[g] = append(byGroup[g], r)
		}
	}

	var merged []Recipe
	for _, g := range groups {
		merged = append(merged, byGroup[g]...)
	}
	return merged
}

// FirstRequestLine returns the index, among the body's lines, of its first
// request — where the cursor goes after insertion, so the recipe can be run
// right away. 0 if there is none.
func (r Recipe) FirstRequestLine() int {
	if requests := parser.ParseAll(r.Body); len(requests) > 0 {
		return requests[0].StartLine
	}
	return 0
}
