package main

import (
	"strings"
	"testing"

	"termdevtools/refdata"
)

func versions(list ...string) []refdata.Version {
	var out []refdata.Version
	for _, s := range list {
		v, _ := refdata.ParseVersion(s)
		out = append(out, v)
	}
	return out
}

func render(ranges []refdata.Range) string {
	return refdata.Constraint{ES: ranges}.String()
}

func TestRangesFromPresence(t *testing.T) {
	points := versions("7.17", "8.0", "8.1", "8.19", "9.0", "9.1")
	cases := []struct {
		name string
		seen []bool
		want string
	}{
		{"everywhere", []bool{true, true, true, true, true, true}, "@es"},
		{"nowhere", []bool{false, false, false, false, false, false}, ""},
		{"added in a minor", []bool{false, false, true, true, true, true}, "@es >=8.1"},
		{"added in a major", []bool{false, true, true, true, true, true}, "@es >=8.0"},
		{"removed at a major", []bool{true, false, false, false, false, false}, "@es <8.0"},
		{"removed at the next major", []bool{true, true, true, true, false, false}, "@es <9.0"},
		{"removed in a minor", []bool{true, true, false, false, false, false}, "@es <8.1"},
		{"backported", []bool{false, false, false, true, false, true}, "@es >=8.19 <9.0 @es >=9.1"},
		{"only the latest", []bool{false, false, false, false, false, true}, "@es >=9.1"},
	}
	for _, c := range cases {
		if got := render(rangesFromPresence(points, c.seen)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestRangesFromPresenceWithGaps checks the conservative bounds when the
// observed points are far apart (real clusters rather than dense release
// branches): nothing is claimed about the versions in between.
func TestRangesFromPresenceWithGaps(t *testing.T) {
	points := versions("7.17.29", "8.0.1", "8.19.22", "9.0.8", "9.5.4")

	// Seen on 8.19.22 only among the 8.x: dated from 8.19, not from 8.1.
	if got := render(rangesFromPresence(points, []bool{false, false, true, true, true})); got != "@es >=8.19" {
		t.Errorf("got %q", got)
	}
	// Last seen on 8.0.1: ends right after it, not at 8.19.
	if got := render(rangesFromPresence(points, []bool{true, true, false, false, false})); got != "@es <8.1" {
		t.Errorf("got %q", got)
	}
}

func TestUnbounded(t *testing.T) {
	if !unbounded([]refdata.Range{{}}) {
		t.Error("a single open interval is unbounded")
	}
	if unbounded(nil) {
		t.Error("no interval at all means absent, not unbounded")
	}
	min := refdata.Version{Major: 8}
	if unbounded([]refdata.Range{{Min: &min}}) {
		t.Error("a bounded interval is not unbounded")
	}
}

func TestCandidateBranches(t *testing.T) {
	got := strings.Join(candidateBranches(), " ")
	for _, want := range []string{"7.17 8.0 ", " 8.19 9.0 ", " 9.5"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in %q", want, got)
		}
	}
	if strings.Contains(got, "8.20") || strings.Contains(got, "7.18") {
		t.Errorf("unexpected branch in %q", got)
	}
}

func TestDisplay(t *testing.T) {
	cases := map[string]string{
		"/_cluster/health": "_cluster/health",
		"/_cat/indices":    "_cat/indices?v",
		"/_list/shards":    "_list/shards?v",
	}
	for in, want := range cases {
		if got := display(in); got != want {
			t.Errorf("display(%q) = %q, want %q", in, got, want)
		}
	}
}
