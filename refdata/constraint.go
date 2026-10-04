package refdata

import (
	"fmt"
	"strings"
)

// Range is a version interval: Min inclusive, Max exclusive, nil meaning
// unbounded on that side.
type Range struct {
	Min, Max *Version
}

func (r Range) contains(v Version) bool {
	if r.Min != nil && v.Compare(*r.Min) < 0 {
		return false
	}
	if r.Max != nil && v.Compare(*r.Max) >= 0 {
		return false
	}
	return true
}

// Constraint says which clusters an entry (endpoint, _cat column, recipe)
// applies to. Written as tags after the entry:
//
//	@es                       any Elasticsearch
//	@es >=8.7                 Elasticsearch 8.7 and later
//	@es >=7.17 <9.0           a bounded interval
//	@es >=8.19 <9.0 @es >=9.1 several intervals for one distribution
//	@opensearch >=2.4         OpenSearch 2.4 and later
//
// The zero Constraint (no tag) applies everywhere. As soon as one
// distribution is tagged, the other is excluded unless tagged too.
type Constraint struct {
	ES []Range
	OS []Range
}

// IsZero reports whether c carries no restriction at all.
func (c Constraint) IsZero() bool {
	return len(c.ES) == 0 && len(c.OS) == 0
}

// Matches reports whether an entry applies to t. Deliberately permissive
// where information is missing: an unknown distribution matches everything,
// and an unknown version (see Target.HasVersion) matches every interval of
// its distribution — better to offer something that may not exist than to
// hide what does.
func (c Constraint) Matches(t Target) bool {
	if c.IsZero() {
		return true
	}
	var ranges []Range
	switch t.Distribution {
	case Elasticsearch:
		ranges = c.ES
	case OpenSearch:
		ranges = c.OS
	default:
		return true
	}
	if len(ranges) == 0 {
		return false
	}
	if !t.HasVersion {
		return true
	}
	for _, r := range ranges {
		if r.contains(t.Version) {
			return true
		}
	}
	return false
}

// String renders c in the syntax ParseConstraint reads back.
func (c Constraint) String() string {
	var parts []string
	for _, r := range c.ES {
		parts = append(parts, rangeString("@es", r))
	}
	for _, r := range c.OS {
		parts = append(parts, rangeString("@opensearch", r))
	}
	return strings.Join(parts, " ")
}

func rangeString(tag string, r Range) string {
	s := tag
	if r.Min != nil {
		s += " >=" + r.Min.String()
	}
	if r.Max != nil {
		s += " <" + r.Max.String()
	}
	return s
}

// ParseConstraint reads the tag tokens following an entry (see Constraint).
func ParseConstraint(tokens []string) (Constraint, error) {
	var c Constraint
	var current *Range

	for _, tok := range tokens {
		switch {
		case strings.HasPrefix(tok, "@"):
			switch strings.ToLower(tok) {
			case "@es", "@elasticsearch":
				c.ES = append(c.ES, Range{})
				current = &c.ES[len(c.ES)-1]
			case "@opensearch", "@os":
				c.OS = append(c.OS, Range{})
				current = &c.OS[len(c.OS)-1]
			default:
				return Constraint{}, fmt.Errorf("unknown tag %q (expected @es or @opensearch)", tok)
			}
		case strings.HasPrefix(tok, ">="):
			if current == nil {
				return Constraint{}, fmt.Errorf("%q must follow @es or @opensearch", tok)
			}
			v, ok := ParseVersion(tok[2:])
			if !ok || current.Min != nil {
				return Constraint{}, fmt.Errorf("invalid lower bound %q", tok)
			}
			current.Min = &v
		case strings.HasPrefix(tok, "<") && !strings.HasPrefix(tok, "<="):
			if current == nil {
				return Constraint{}, fmt.Errorf("%q must follow @es or @opensearch", tok)
			}
			v, ok := ParseVersion(tok[1:])
			if !ok || current.Max != nil {
				return Constraint{}, fmt.Errorf("invalid upper bound %q", tok)
			}
			current.Max = &v
		default:
			return Constraint{}, fmt.Errorf("unexpected %q (expected @es, @opensearch, >=version or <version)", tok)
		}
	}
	// An interval no version can be in would hide the entry from every
	// identified cluster — and, matching failing open, still show it on the
	// others: a typo, reported rather than obeyed.
	for _, r := range append(append([]Range(nil), c.ES...), c.OS...) {
		if r.Min != nil && r.Max != nil && r.Min.Compare(*r.Max) >= 0 {
			return Constraint{}, fmt.Errorf("empty interval: >=%s <%s", r.Min, r.Max)
		}
	}
	return c, nil
}

// splitAnnotated splits a data line into its value (first field) and the
// constraint written after it.
func splitAnnotated(line string) (value string, c Constraint, err error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", Constraint{}, nil
	}
	c, err = ParseConstraint(fields[1:])
	return fields[0], c, err
}
