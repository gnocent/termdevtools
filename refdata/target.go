// Package refdata holds the reference data shipped inside the binary —
// completion endpoints, _cat columns, recipes — and selects what applies to
// the cluster actually connected to (its distribution and version). See
// SPEC.md §3.2 and §9.1.
package refdata

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Distribution is the product a cluster runs.
type Distribution int

const (
	// UnknownDistribution means detection failed or wasn't attempted:
	// nothing is filtered out for such a cluster (see Constraint.Matches).
	UnknownDistribution Distribution = iota
	Elasticsearch
	OpenSearch
)

// Version is a product version; missing parts are 0 ("8.19" is 8.19.0).
type Version struct {
	Major, Minor, Patch int
}

// ParseVersion reads "9", "8.19" or "8.19.22", ignoring any suffix after
// the numeric part ("9.6.0-SNAPSHOT").
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return Version{}, false
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return Version{}, false
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, false
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, true
}

// Compare returns -1, 0 or 1 as v is lower than, equal to or higher than o.
func (v Version) Compare(o Version) int {
	for _, d := range [3]int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		switch {
		case d < 0:
			return -1
		case d > 0:
			return 1
		}
	}
	return 0
}

// String renders v the way constraints are written: "8.19", or "8.19.22"
// when the patch matters.
func (v Version) String() string {
	if v.Patch == 0 {
		return fmt.Sprintf("%d.%d", v.Major, v.Minor)
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Target is what reference data gets selected for: the connected cluster's
// distribution and version.
type Target struct {
	Distribution Distribution
	Version      Version
	// HasVersion is false when the version is unknown or not meaningful
	// (Elasticsearch Serverless, OpenSearch reporting a fake 7.10.2 in
	// compatibility mode): version bounds are then not applied.
	HasVersion bool
	Serverless bool
}

// DetectTarget reads the body of a cluster's "GET /" response. Anything it
// can't recognize yields the zero Target, for which nothing is filtered.
//
// OpenSearch is recognized by version.distribution, or failing that by its
// tagline: with compatibility.override_main_response_version enabled (1.x
// and 2.x), OpenSearch drops the distribution field and reports 7.10.2 —
// checked against a real 2.19.6 node — so the number is discarded there.
func DetectTarget(rootBody []byte) Target {
	var root struct {
		Version struct {
			Distribution string `json:"distribution"`
			Number       string `json:"number"`
			BuildFlavor  string `json:"build_flavor"`
		} `json:"version"`
		Tagline string `json:"tagline"`
	}
	if err := json.Unmarshal(rootBody, &root); err != nil {
		return Target{}
	}

	version, hasVersion := ParseVersion(root.Version.Number)
	tagline := strings.ToLower(root.Tagline)

	switch {
	case strings.EqualFold(root.Version.Distribution, "opensearch"):
		return Target{Distribution: OpenSearch, Version: version, HasVersion: hasVersion}
	case root.Version.Distribution != "":
		// Some other fork announcing itself: not ours to guess at.
		return Target{}
	case strings.Contains(tagline, "opensearch"):
		return Target{Distribution: OpenSearch}
	case strings.Contains(tagline, "you know, for search"):
		if strings.EqualFold(root.Version.BuildFlavor, "serverless") {
			return Target{Distribution: Elasticsearch, Serverless: true}
		}
		return Target{Distribution: Elasticsearch, Version: version, HasVersion: hasVersion}
	}
	return Target{}
}

// WithOverride applies a per-cluster override from config.yaml: distribution
// ("elasticsearch" or "opensearch") and/or version, each optional. An
// overridden distribution without a version keeps the detected version only
// if detection had found that same distribution.
func (t Target) WithOverride(distribution, version string) (Target, error) {
	distribution = strings.ToLower(strings.TrimSpace(distribution))
	version = strings.TrimSpace(version)

	switch distribution {
	case "":
	case "elasticsearch", "es":
		if t.Distribution != Elasticsearch {
			t = Target{Distribution: Elasticsearch}
		}
	case "opensearch", "os":
		if t.Distribution != OpenSearch {
			t = Target{Distribution: OpenSearch}
		}
	default:
		return t, fmt.Errorf("unknown distribution %q (expected elasticsearch or opensearch)", distribution)
	}

	if version != "" {
		v, ok := ParseVersion(version)
		if !ok {
			return t, fmt.Errorf("invalid version %q", version)
		}
		t.Version, t.HasVersion, t.Serverless = v, true, false
	}
	return t, nil
}

// Label is the short form shown in the status bar ("ES 9.5.4", "OS 2.19.6"),
// empty when the distribution is unknown.
func (t Target) Label() string {
	return t.describe("ES", "OS")
}

// Name is the long form ("Elasticsearch 9.5.4"), empty when the distribution
// is unknown.
func (t Target) Name() string {
	return t.describe("Elasticsearch", "OpenSearch")
}

func (t Target) describe(es, os string) string {
	var name string
	switch t.Distribution {
	case Elasticsearch:
		name = es
	case OpenSearch:
		name = os
	default:
		return ""
	}
	switch {
	case t.Serverless:
		return name + " Serverless"
	case t.HasVersion:
		return fmt.Sprintf("%s %d.%d.%d", name, t.Version.Major, t.Version.Minor, t.Version.Patch)
	}
	return name
}
