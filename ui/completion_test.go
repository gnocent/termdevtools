package ui

import (
	"sort"
	"testing"

	"termdevtools/refdata"
)

// es95 is the cluster the tests in this package select reference data for
// when they assert on exact contents: what is offered depends on the
// distribution and version (OpenSearch has its own _cat commands, for one).
var es95 = refdata.Target{Distribution: refdata.Elasticsearch, Version: refdata.Version{Major: 9, Minor: 5, Patch: 4}, HasVersion: true}

// builtinEndpoints is the endpoints built into the binary for es95.
func builtinEndpoints() []string {
	return refdata.Load(refdata.Sources{}).Endpoints(es95)
}

func TestMatchEndpointsPrefix(t *testing.T) {
	got := matchPrefix("_cat/s", builtinEndpoints())
	want := []string{"_cat/segments?v", "_cat/shards?v", "_cat/snapshots?v"}
	if !sort.StringsAreSorted(got) {
		t.Errorf("expected sorted results, got %v", got)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %v, got %v", want, got)
			break
		}
	}
}

func TestMatchEndpointsEmptyPrefixReturnsAll(t *testing.T) {
	all := builtinEndpoints()
	got := matchPrefix("", all)
	if len(got) != len(all) {
		t.Errorf("expected all %d endpoints, got %d", len(all), len(got))
	}
}

func TestMatchEndpointsNoMatch(t *testing.T) {
	got := matchPrefix("does_not_exist", builtinEndpoints())
	if len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

func TestMatchEndpointsCaseInsensitive(t *testing.T) {
	got := matchPrefix("_CAT/SH", builtinEndpoints())
	if len(got) == 0 {
		t.Error("expected case-insensitive matching to find results")
	}
}
