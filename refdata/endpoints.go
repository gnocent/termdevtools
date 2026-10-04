package refdata

import (
	"strings"
)

// Endpoint is one completion candidate and the clusters it applies to.
type Endpoint struct {
	Path       string
	Constraint Constraint
}

// parseEndpoints reads an endpoints file: one endpoint per line, optionally
// followed by constraint tags (see Constraint); empty lines and lines
// starting with '#' are ignored. A malformed line is skipped and reported
// rather than failing the whole file: one typo in a hand-edited file
// shouldn't cost the user every other line of it.
func parseEndpoints(file string, data []byte) (endpoints []Endpoint, problems []Problem) {
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, constraint, err := splitAnnotated(line)
		if err != nil {
			problems = append(problems, Problem{File: file, Line: i + 1, Message: err.Error()})
			continue
		}
		endpoints = append(endpoints, Endpoint{Path: path, Constraint: constraint})
	}
	return endpoints, problems
}

// mergeEndpoints stacks layers, lowest priority first (embedded, then the
// team's file, then the user's). A higher layer adds endpoints; for one that
// already exists below, it replaces the constraint only if it states one —
// an untagged duplicate changes nothing. That last rule matters for the
// endpoints.txt older releases shipped next to the binary: every line of it
// is untagged, and must not wipe out the embedded entry's version interval.
func mergeEndpoints(layers ...[]Endpoint) []Endpoint {
	var merged []Endpoint
	index := make(map[string]int)
	for _, layer := range layers {
		for _, e := range layer {
			i, exists := index[e.Path]
			switch {
			case !exists:
				index[e.Path] = len(merged)
				merged = append(merged, e)
			case !e.Constraint.IsZero():
				merged[i].Constraint = e.Constraint
			}
		}
	}
	return merged
}
