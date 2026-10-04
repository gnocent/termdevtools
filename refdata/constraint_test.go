package refdata

import (
	"strings"
	"testing"
)

func es(v string) Target {
	version, _ := ParseVersion(v)
	return Target{Distribution: Elasticsearch, Version: version, HasVersion: true}
}

func opensearch(v string) Target {
	version, _ := ParseVersion(v)
	return Target{Distribution: OpenSearch, Version: version, HasVersion: true}
}

func mustConstraint(t *testing.T, s string) Constraint {
	t.Helper()
	c, err := ParseConstraint(strings.Fields(s))
	if err != nil {
		t.Fatalf("ParseConstraint(%q): %v", s, err)
	}
	return c
}

func TestConstraintMatches(t *testing.T) {
	cases := []struct {
		constraint string
		target     Target
		want       bool
	}{
		// No tag: everywhere.
		{"", es("7.17.29"), true},
		{"", opensearch("3.0.0"), true},
		{"", Target{}, true},

		// One distribution tagged: the other is excluded.
		{"@es", es("7.17.29"), true},
		{"@es", opensearch("2.19.6"), false},
		{"@opensearch", es("9.5.4"), false},
		{"@opensearch", opensearch("2.0.1"), true},

		// Bounds: lower inclusive, upper exclusive.
		{"@es >=8.7", es("8.6.2"), false},
		{"@es >=8.7", es("8.7.0"), true},
		{"@es >=8.7", es("9.0.8"), true},
		{"@es <9.0", es("8.19.22"), true},
		{"@es <9.0", es("9.0.0"), false},
		{"@es >=7.17 <9.0", es("8.0.1"), true},
		{"@es >=8.2", es("8.19.0"), true}, // 19 > 2: numeric comparison

		// Both distributions, each with its own bounds.
		{"@es >=8.0 @opensearch >=2.4", opensearch("2.0.1"), false},
		{"@es >=8.0 @opensearch >=2.4", opensearch("2.19.6"), true},
		{"@es >=8.0 @opensearch >=2.4", es("7.17.29"), false},
		{"@es >=8.0 @opensearch", opensearch("2.0.1"), true},

		// Several intervals for one distribution (a backport: in 8.19 and
		// 9.1+, but not in 9.0).
		{"@es >=8.19 <9.0 @es >=9.1", es("8.19.22"), true},
		{"@es >=8.19 <9.0 @es >=9.1", es("9.0.8"), false},
		{"@es >=8.19 <9.0 @es >=9.1", es("9.5.4"), true},

		// Missing information never hides anything.
		{"@es >=8.7", Target{}, true},
		{"@opensearch >=2.4", Target{Distribution: OpenSearch}, true},
		{"@es >=8.7", Target{Distribution: Elasticsearch, Serverless: true}, true},
		// ...but a known distribution still excludes the other's entries.
		{"@es", Target{Distribution: OpenSearch}, false},
	}
	for _, c := range cases {
		got := mustConstraint(t, c.constraint).Matches(c.target)
		if got != c.want {
			t.Errorf("%q vs %s: got %v, want %v", c.constraint, c.target.Name(), got, c.want)
		}
	}
}

func TestParseConstraintRoundTrip(t *testing.T) {
	for _, s := range []string{
		"",
		"@es",
		"@opensearch",
		"@es >=8.7",
		"@es >=7.17 <9.0",
		"@es >=8.19 <9.0 @es >=9.1",
		"@es >=8.0 @opensearch >=2.4 <3.0",
		"@es >=8.19.3",
	} {
		if got := mustConstraint(t, s).String(); got != s {
			t.Errorf("round trip of %q gave %q", s, got)
		}
	}
}

func TestParseConstraintAliases(t *testing.T) {
	if got := mustConstraint(t, "@Elasticsearch >=8 @OS").String(); got != "@es >=8.0 @opensearch" {
		t.Errorf("got %q", got)
	}
}

func TestParseConstraintRejectsMalformedInput(t *testing.T) {
	for _, s := range []string{
		">=8.0",           // bound without a distribution
		"@solr",           // unknown tag
		"@es >=x",         // unparseable version
		"@es >=8.0 >=8.5", // lower bound given twice
		"@es <9 <10",      // upper bound given twice
		"@es <=9.0",       // unsupported operator
		"@es 8.0",         // bare version
	} {
		if _, err := ParseConstraint(strings.Fields(s)); err == nil {
			t.Errorf("ParseConstraint(%q) should have failed", s)
		}
	}
}

func TestSplitAnnotated(t *testing.T) {
	value, c, err := splitAnnotated("  _health_report   @es >=8.7  ")
	if err != nil || value != "_health_report" || c.String() != "@es >=8.7" {
		t.Errorf("got %q, %q, %v", value, c.String(), err)
	}

	value, c, err = splitAnnotated("_cluster/health")
	if err != nil || value != "_cluster/health" || !c.IsZero() {
		t.Errorf("got %q, %q, %v", value, c.String(), err)
	}

	if _, _, err := splitAnnotated("_cluster/health oops"); err == nil {
		t.Error("trailing text that isn't a tag should be rejected")
	}
}
