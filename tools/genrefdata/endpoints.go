package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"termdevtools/refdata"
)

// Elasticsearch publishes one specification per release branch. Every minor
// from the oldest supported version on is read: an endpoint's "since"
// annotation says when it was added (when it has one), the branches it
// appears in say the rest — where an endpoint without annotation first shows
// up, and whether it is still there in the latest release. Neither source is
// fully reliable (annotations are sometimes wrong, and an endpoint may only
// be described several releases after it shipped), hence the overrides
// below and the check against real clusters. Branches that don't exist are
// skipped. Raise esLatestMinor when regenerating for a new release.
const (
	esOldestBranch = "7.17"
	esLatestMajor  = 9
	esLatestMinor  = 5
)

// Last minor of each major before the latest.
var esLastMinor = map[int]int{8: 19}

const (
	esSpecURL = "https://raw.githubusercontent.com/elastic/elasticsearch-specification/%s/output/schema/schema.json"
	osSpecURL = "https://github.com/opensearch-project/opensearch-api-specification/releases/download/main-latest/opensearch-openapi.yaml"
)

// The oldest version of each distribution the data is meant for: an "added
// in" at or below it carries no information and isn't written.
var (
	esFloor = refdata.Version{Major: 7, Minor: 17}
	osFloor = refdata.Version{Major: 2}
)

// Which parts of each API make it into completion: cluster administration
// and the everyday document/search calls. Deliberately left out: machine
// learning, security, watcher, transforms, rollups, SQL/ES|QL, cross-cluster
// replication, connectors, inference, enrich (SPEC.md §3.2).
var (
	esNamespaces = set(
		"cat", "cluster", "nodes", "indices", "snapshot", "ilm", "slm", "license",
		"tasks", "features", "ingest", "migration", "ssl", "xpack",
		"dangling_indices", "shutdown", "searchable_snapshots",
	)
	osGroups = set(
		"cat", "cluster", "nodes", "indices", "snapshot", "tasks", "ingest",
		"dangling_indices", "ism", "sm", "list", "insights", "search_pipeline",
		"remote_store",
	)
	// Endpoints outside any namespace ("search", "bulk"...), common to both.
	coreEndpoints = set(
		"bulk", "count", "field_caps", "health_report", "mget", "msearch",
		"msearch_template", "mtermvectors", "reindex", "search", "scroll",
		"clear_scroll", "search_template", "search_shards",
	)
)

// overrides corrects what the specifications get wrong, as observed on the
// real clusters the integration tests run against (tools/testclusters.sh).
// Keyed by the path as offered; the value is the constraint to use instead
// ("" for none, "-" to drop the endpoint). Each entry says what was
// observed. Where the true version lies between two clusters queried, the
// later one is used: better not to offer than to offer wrongly.
var overrides = map[string]string{
	// Annotated "since 5.1" (copied from _cat/templates), but unknown to a
	// real 8.0.1; the first branch describing it is 8.2.
	"_cat/component_templates?v": "@es >=8.2",

	// Dropped from the specification after 9.1 and only described from 8.9
	// on, yet real 7.17.29, 8.0.1 and 9.5.4 nodes all answer it.
	"_nodes/shutdown": "@es",

	// Absent from the Elasticsearch specification (deprecated since 7.0),
	// but a real 7.17.29 still answers it; gone in 8.0.1. Kept by OpenSearch.
	"_upgrade": "@es <8.0 @opensearch",

	// All four are annotated "added in 1.0" by the OpenSearch specification,
	// and all four are unknown to a real 2.0.1.
	"_cluster/decommission/awareness":    "@opensearch >=2.11", // answers on 2.11.1
	"_cluster/routing/awareness/weights": "@opensearch >=2.11", // answers on 2.11.1
	"_remotestore/_restore":              "@opensearch >=2.11", // answers on 2.11.1
	"_insights/top_queries":              "@opensearch >=2.19", // unknown to 2.11.1, answers on 2.19.6
}

// specs is everything read from the specifications.
type specs struct {
	// esBranches are the Elasticsearch branches actually found, ascending.
	esBranches []string
	esVersions []refdata.Version
	paths      map[string]*endpointInfo
}

// endpointInfo accumulates what is known about one path.
type endpointInfo struct {
	// esSeen[i] says whether the path is in esBranches[i].
	esSeen []bool
	// esSince is the "since" annotation, as stated by the most recent
	// branch holding the path; nil when it states none.
	esSince *refdata.Version

	inOS      bool
	osAdded   *refdata.Version
	osRemoved *refdata.Version
}

func (s *specs) info(path string) *endpointInfo {
	if s.paths[path] == nil {
		s.paths[path] = &endpointInfo{}
	}
	return s.paths[path]
}

func generateEndpoints(specsDir, outPath string) error {
	s, err := collectSpecs(specsDir)
	if err != nil {
		return err
	}

	var lines []string
	width := 0
	for path := range s.paths {
		if len(display(path)) > width {
			width = len(display(path))
		}
	}
	applied := make(map[string]bool)
	for path, i := range s.paths {
		constraint := s.constraintFor(i)
		if o, ok := overrides[display(path)]; ok {
			applied[display(path)] = true
			if o == "-" {
				continue
			}
			constraint = o
		}
		lines = append(lines, strings.TrimRight(fmt.Sprintf("%-*s  %s", width, display(path), constraint), " "))
	}
	sort.Strings(lines)

	// An override only corrects a path the specifications still hold: one
	// that no longer matches anything would otherwise make its endpoint
	// vanish from the output without a word.
	for path := range overrides {
		if !applied[path] {
			return fmt.Errorf("override for %q matches no path of the specifications anymore: review it", path)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, endpointsHeader, strings.Join(s.esBranches, ", "))
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	if err := os.WriteFile(outPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: %d endpoints\n", outPath, len(lines))
	return nil
}

// explainEndpoints prints, for each path containing one of the given
// fragments, what the specifications say about it and what constraint
// results — to understand an entry before overriding it.
func explainEndpoints(specsDir string, fragments []string) error {
	s, err := collectSpecs(specsDir)
	if err != nil {
		return err
	}
	var paths []string
	for path := range s.paths {
		for _, f := range fragments {
			if strings.Contains(path, f) {
				paths = append(paths, path)
				break
			}
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		i := s.paths[path]
		var branches []string
		for idx, seen := range i.esSeen {
			if seen {
				branches = append(branches, s.esBranches[idx])
			}
		}
		fmt.Printf("%s\n  Elasticsearch: since %s, in branches %v\n  OpenSearch:    present %v, added %s, removed %s\n  generated:     %q",
			display(path), versionOrNone(i.esSince), branches, i.inOS, versionOrNone(i.osAdded), versionOrNone(i.osRemoved), s.constraintFor(i))
		if o, ok := overrides[display(path)]; ok {
			fmt.Printf(", overridden by %q", o)
		}
		fmt.Println()
	}
	return nil
}

func versionOrNone(v *refdata.Version) string {
	if v == nil {
		return "(none)"
	}
	return v.String()
}

const endpointsHeader = `# Endpoints offered by Tab/F10 completion (SPEC.md §3.2).
#
# One endpoint per line, optionally followed by tags restricting it to a
# distribution and version interval:
#
#   _health_report         @es >=8.7
#   _plugins/_ism/explain  @opensearch
#
# No tag: offered everywhere. Lines starting with # and empty lines are
# ignored. The same format works in your own endpoints.txt (next to the
# binary for a team, in ~/.config/termdevtools/ for yourself), which adds to
# this list.
#
# Generated by "go run ./tools/genrefdata endpoints" from the official API
# specifications — elastic/elasticsearch-specification (branches %s)
# and opensearch-project/opensearch-api-specification — then checked against
# real clusters. Do not edit by hand.

`

// display is how a path is offered: without its leading slash, and with ?v
// for the tabular APIs, whose output is unreadable without column headers.
func display(path string) string {
	p := strings.TrimPrefix(path, "/")
	if strings.HasPrefix(p, "_cat/") || strings.HasPrefix(p, "_list/") {
		p += "?v"
	}
	return p
}

// constraintFor turns what the specifications say about a path into the
// tags written next to it.
func (s *specs) constraintFor(i *endpointInfo) string {
	var c refdata.Constraint

	first, last := -1, -1
	for idx, seen := range i.esSeen {
		if seen {
			if first < 0 {
				first = idx
			}
			last = idx
		}
	}
	if first >= 0 {
		// One interval from the first branch to the last: a gap in between
		// is the specification lagging, not the endpoint coming and going.
		seen := make([]bool, len(s.esBranches))
		for idx := first; idx <= last; idx++ {
			seen[idx] = true
		}
		r := rangesFromPresence(s.esVersions, seen)[0]
		// The annotation, when there is one, is a better lower bound than
		// the first branch describing the endpoint.
		if i.esSince != nil {
			r.Min = nil
			if i.esSince.Compare(esFloor) > 0 {
				r.Min = &refdata.Version{Major: i.esSince.Major, Minor: i.esSince.Minor}
			}
		}
		c.ES = []refdata.Range{r}
	}

	if i.inOS {
		r := refdata.Range{}
		if i.osAdded != nil && i.osAdded.Compare(osFloor) > 0 {
			r.Min = i.osAdded
		}
		r.Max = i.osRemoved
		c.OS = []refdata.Range{r}
	}

	// Both distributions, no bound on either side: no tag at all.
	if unbounded(c.ES) && unbounded(c.OS) {
		return ""
	}
	return c.String()
}

// collectSpecs reads every specification into one record per path.
func collectSpecs(specsDir string) (*specs, error) {
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return nil, err
	}
	s := &specs{paths: make(map[string]*endpointInfo)}

	for _, branch := range candidateBranches() {
		data, err := cachedDownload(filepath.Join(specsDir, "es-"+branch+".json"), fmt.Sprintf(esSpecURL, branch))
		if errors.Is(err, errNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		paths, err := esPaths(data)
		if err != nil {
			return nil, fmt.Errorf("Elasticsearch %s specification: %w", branch, err)
		}
		version, _ := refdata.ParseVersion(branch)
		s.esBranches = append(s.esBranches, branch)
		s.esVersions = append(s.esVersions, version)
		for path, since := range paths {
			i := s.info(path)
			for len(i.esSeen) < len(s.esBranches) {
				i.esSeen = append(i.esSeen, false)
			}
			i.esSeen[len(s.esBranches)-1] = true
			// Branches are read oldest first: the latest one has the last
			// word on the annotation.
			i.esSince = since
		}
	}
	if len(s.esBranches) == 0 || s.esBranches[0] != esOldestBranch {
		return nil, fmt.Errorf("the Elasticsearch %s specification, the base of every interval, could not be read", esOldestBranch)
	}

	data, err := cachedDownload(filepath.Join(specsDir, "os-openapi.yaml"), osSpecURL)
	if err != nil {
		return nil, err
	}
	if err := collectOS(data, s); err != nil {
		return nil, fmt.Errorf("OpenSearch specification: %w", err)
	}
	return s, nil
}

// candidateBranches lists every Elasticsearch branch that may exist, from
// the oldest supported one to the latest release.
func candidateBranches() []string {
	branches := []string{esOldestBranch}
	oldest, _ := refdata.ParseVersion(esOldestBranch)
	for major := oldest.Major + 1; major <= esLatestMajor; major++ {
		last := esLastMinor[major]
		if major == esLatestMajor {
			last = esLatestMinor
		}
		for minor := 0; minor <= last; minor++ {
			branches = append(branches, fmt.Sprintf("%d.%d", major, minor))
		}
	}
	return branches
}

// esPaths returns the paths one branch of the Elasticsearch specification
// (output/schema/schema.json) holds, among those of interest, each with its
// "since" annotation — the oldest one when several endpoints share a path
// (GET and PUT variants), nil as soon as one of them has none (endpoints old
// enough to predate the annotation).
func esPaths(data []byte) (map[string]*refdata.Version, error) {
	var schema struct {
		Endpoints []struct {
			Name         string `json:"name"`
			Since        string `json:"since"`      // 7.17 layout
			Visibility   string `json:"visibility"` // 7.17 layout
			Availability map[string]struct {
				Since      string `json:"since"`
				Visibility string `json:"visibility"`
			} `json:"availability"`
			URLs []struct {
				Path        string          `json:"path"`
				Deprecation json.RawMessage `json:"deprecation"`
			} `json:"urls"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}

	paths := make(map[string]*refdata.Version)
	unannotated := make(map[string]bool)
	for _, e := range schema.Endpoints {
		namespace, _, hasNamespace := strings.Cut(e.Name, ".")
		if hasNamespace && !esNamespaces[namespace] || !hasNamespace && !coreEndpoints[e.Name] {
			continue
		}

		since, visibility := e.Since, e.Visibility
		if e.Availability != nil {
			stack, onStack := e.Availability["stack"]
			if !onStack {
				continue // Serverless only
			}
			since, visibility = stack.Since, stack.Visibility
		}
		if visibility == "private" || visibility == "feature_flag" {
			continue
		}

		for _, u := range e.URLs {
			if strings.Contains(u.Path, "{") || u.Path == "/_cat" || u.Path == "/" || len(u.Deprecation) > 0 && string(u.Deprecation) != "null" {
				continue
			}
			v, annotated := refdata.ParseVersion(since)
			switch current, known := paths[u.Path]; {
			case !annotated:
				unannotated[u.Path] = true
				paths[u.Path] = nil
			case unannotated[u.Path]:
				// stays nil
			case !known || current == nil || v.Compare(*current) < 0:
				paths[u.Path] = &v
			}
		}
	}
	return paths, nil
}

// collectOS reads the OpenSearch specification (a single OpenAPI document
// annotated with x-version-added / x-version-removed).
func collectOS(data []byte, s *specs) error {
	var spec struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return err
	}

	for path, operations := range spec.Paths {
		if strings.Contains(path, "{") || path == "/_cat" || path == "/_list" || path == "/" || strings.HasPrefix(path, "/_opendistro") {
			continue
		}
		// A path exists as long as one of its operations (GET, POST...)
		// does: since the oldest "added" — always, if one operation states
		// none — and until the latest "removed", if every one of them has
		// been removed.
		var added, removed *refdata.Version
		kept, alwaysThere, stillThere := 0, false, false
		for _, node := range operations {
			var op struct {
				Group      string `yaml:"x-operation-group"`
				Added      string `yaml:"x-version-added"`
				Removed    string `yaml:"x-version-removed"`
				Deprecated bool   `yaml:"deprecated"`
				Ignorable  bool   `yaml:"x-ignorable"`
			}
			if err := node.Decode(&op); err != nil || op.Group == "" {
				continue // "parameters", "summary"... not an operation
			}
			namespace, _, hasNamespace := strings.Cut(op.Group, ".")
			if hasNamespace && !osGroups[namespace] || !hasNamespace && !coreEndpoints[op.Group] {
				continue
			}
			if op.Deprecated || op.Ignorable {
				continue
			}

			kept++
			if v, ok := refdata.ParseVersion(op.Added); !ok {
				alwaysThere = true
			} else if added == nil || v.Compare(*added) < 0 {
				added = &v
			}
			if v, ok := refdata.ParseVersion(op.Removed); !ok {
				stillThere = true
			} else if removed == nil || v.Compare(*removed) > 0 {
				removed = &v
			}
		}
		if kept == 0 {
			continue
		}
		i := s.info(path)
		i.inOS = true
		if !alwaysThere {
			i.osAdded = added
		}
		if !stillThere {
			i.osRemoved = removed
		}
	}
	return nil
}

var errNotFound = errors.New("not found")

// cachedDownload returns the content of path, downloading it from url first
// if it isn't there yet — the specifications weigh several megabytes each.
// What was downloaded once is reused as is, however old (its date is
// printed): delete the file, or the whole directory, to fetch it again. A
// URL that doesn't exist is reported as errNotFound and asked again on the
// next run — a branch that isn't published yet will be one day.
func cachedDownload(path, url string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if info, err := os.Stat(path); err == nil {
			fmt.Printf("using %s, downloaded on %s\n", filepath.Base(path), info.ModTime().Format("2006-01-02"))
		}
		return data, nil
	}
	fmt.Printf("downloading %s\n", url)
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("not found, skipped: %s\n", url)
		return nil, errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, os.WriteFile(path, data, 0o644)
}

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}
