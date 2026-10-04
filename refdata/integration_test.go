//go:build integration

// Integration tests: the embedded reference data against real clusters.
//
//	wsl.exe -- bash tools/testclusters.sh up      (or: bash tools/testclusters.sh up)
//	go test -tags integration ./refdata/
//
// The clusters default to those tools/testclusters.sh starts; set
// TDT_IT_TARGETS ("es-8.19.22=http://host:9200,...") to use others.
//
// THESE TESTS ARE DESTRUCTIVE. They run every recipe for real — cluster
// settings are changed, indices and snapshots created, restored and deleted,
// a node drained — and probe state-changing endpoints. They are meant for
// throwaway single-node clusters, and refuse any cluster that isn't on this
// machine unless TDT_IT_ALLOW_REMOTE=1 says it is one.
package refdata_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"termdevtools/internal/testclusters"
	"termdevtools/parser"
	"termdevtools/refdata"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// request sends method on url, with a JSON body if one is given, and
// returns the status and body of the response.
func request(t *testing.T, method, url string, jsonBody ...string) (int, string) {
	t.Helper()
	var payload io.Reader
	if len(jsonBody) > 0 {
		payload = strings.NewReader(jsonBody[0])
	}
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v — are the test clusters up? (tools/testclusters.sh up)", method, url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: reading the response: %v", method, url, err)
	}
	return resp.StatusCode, string(body)
}

// allowRemoteEnv, set to 1, lifts the guard below: the clusters named by
// TDT_IT_TARGETS are throwaway ones even though they run elsewhere.
const allowRemoteEnv = "TDT_IT_ALLOW_REMOTE"

func clusters(t *testing.T) []testclusters.Cluster {
	t.Helper()
	list, err := testclusters.FromEnv()
	if err != nil {
		t.Fatalf("%s: %v", testclusters.EnvVar, err)
	}
	if os.Getenv(allowRemoteEnv) == "1" {
		return list
	}
	for _, cl := range list {
		u, err := url.Parse(cl.URL)
		if err != nil {
			t.Fatalf("%s: %v", cl.Name, err)
		}
		host := u.Hostname()
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			t.Fatalf("%s (%s) is not on this machine. These tests are destructive (see the top of this file): never point them at a cluster that matters. Set %s=1 if it really is a throwaway cluster.",
				cl.Name, cl.URL, allowRemoteEnv)
		}
	}
	return list
}

func target(t *testing.T, cl testclusters.Cluster) refdata.Target {
	t.Helper()
	tg, err := cl.Target()
	if err != nil {
		t.Fatal(err)
	}
	return tg
}

// TestDetectionOnRealClusters checks that what each cluster answers to
// "GET /" is detected as the distribution and version it actually runs.
func TestDetectionOnRealClusters(t *testing.T) {
	for _, cl := range clusters(t) {
		status, body := request(t, "GET", cl.URL+"/")
		if status != 200 {
			t.Errorf("%s: GET / returned HTTP %d", cl.Name, status)
			continue
		}
		if got, want := refdata.DetectTarget([]byte(body)), target(t, cl); got != want {
			t.Errorf("%s: detected %+v, want %+v", cl.Name, got, want)
		}
	}
}

// unknownToCluster reports whether a response means "this cluster has no
// such endpoint", as opposed to an endpoint that exists but rejected this
// particular bare request.
func unknownToCluster(path string, status int, body string) bool {
	if strings.Contains(body, "no handler found for uri") {
		return true
	}
	// A single unknown segment is taken for an index name.
	first := strings.SplitN(strings.SplitN(path, "?", 2)[0], "/", 2)[0]
	if strings.Contains(body, "invalid_index_name_exception") {
		return true
	}
	if status == 404 && strings.Contains(body, "index_not_found_exception") && strings.Contains(body, "["+first+"]") {
		return true
	}
	return false
}

// typedRoutes reports whether a cluster still has the typed-document routes
// (POST /{index}/{type}), removed in Elasticsearch 8.0 and never part of
// OpenSearch 2.
func typedRoutes(tg refdata.Target) bool {
	return tg.Distribution == refdata.Elasticsearch && tg.Version.Major < 8
}

// probe asks a cluster about an endpoint without knowing its method, with a
// GET. "405, this path takes other methods" settles it — the endpoint exists
// — except where typed-document routes remain (typedRoutes): any unknown
// two-segment path matches POST /{index}/{type} there, and answers 405 to a
// GET just like a real POST-only endpoint. Only then is the request sent
// again, as POST or PUT with an empty JSON object: the typed route goes on
// to reject the "index" name ("_cat", "_ssl"...), which unknownToCluster
// recognizes, while a real endpoint runs — some change the cluster's state
// (see restartLifecycle). DELETE is never sent.
func probe(t *testing.T, tg refdata.Target, baseURL, path string) (status int, body string) {
	t.Helper()
	status, body = request(t, "GET", baseURL+"/"+path)
	if status != 405 || !typedRoutes(tg) {
		return status, body
	}
	for _, method := range []string{"POST", "PUT"} {
		if strings.Contains(body, method) {
			return request(t, method, baseURL+"/"+path, "{}")
		}
	}
	return status, body
}

// restartLifecycle undoes what probing "_ilm/stop" and "_slm/stop" with a
// POST leaves behind. Best-effort: the answers don't matter.
func restartLifecycle(t *testing.T, baseURL string) {
	t.Helper()
	request(t, "POST", baseURL+"/_ilm/start")
	request(t, "POST", baseURL+"/_slm/start")
}

// Endpoints for which the checks below can't be applied as is, each with
// the reason — verified by hand on the clusters.
var (
	// Offered, but the test clusters can't show it: they run with security
	// disabled, and these APIs are only registered when it is enabled.
	needsSecurity = map[string]bool{
		"_ssl/certificates": true,
	}

	// Hidden from a distribution, yet a GET answers 200 there.
	hiddenButAnswering = map[refdata.Distribution]map[string]string{
		refdata.OpenSearch: {
			"_cat/master?v":   "deprecated alias of _cat/cluster_manager since OpenSearch 2.0: deliberately not offered",
			"_nodes/shutdown": "not the shutdown API (Elasticsearch only): read as _nodes/{node_id}, it lists zero node",
		},
	}
)

// TestEndpointsOnRealClusters checks every embedded endpoint against every
// cluster, both ways: one offered for a cluster must exist there, and one
// hidden from a cluster must not turn out to work there.
func TestEndpointsOnRealClusters(t *testing.T) {
	catalog := refdata.Load(refdata.Sources{})
	all := catalog.Endpoints(refdata.Target{})
	sort.Strings(all)

	for _, cl := range clusters(t) {
		cl := cl
		t.Run(cl.Name, func(t *testing.T) {
			t.Parallel() // the clusters are independent
			tg := target(t, cl)
			offered := make(map[string]bool)
			for _, e := range catalog.Endpoints(tg) {
				offered[e] = true
			}
			if typedRoutes(tg) {
				t.Cleanup(func() { restartLifecycle(t, cl.URL) })
			}

			for _, path := range all {
				if offered[path] {
					if needsSecurity[path] {
						continue
					}
					status, body := probe(t, tg, cl.URL, path)
					if unknownToCluster(path, status, body) {
						t.Errorf("%s is offered but unknown to the cluster: HTTP %d %.160s", path, status, body)
					}
					continue
				}
				// Hidden: GET only — no need to poke at an endpoint that
				// isn't supposed to be there.
				status, body := request(t, "GET", cl.URL+"/"+path)
				if status == 200 && hiddenButAnswering[tg.Distribution][path] == "" {
					t.Errorf("%s is hidden but works: HTTP 200 %.120s", path, body)
				}
			}
		})
	}
}

// The names every fixture is created under, on each cluster.
const (
	fixtureIndex      = "tdt-it-index"
	fixtureAlias      = "tdt-it-alias"
	fixtureRepository = "tdt-it-repository"
	fixtureSnapshot   = "tdt-it-snapshot"
)

// mustSucceed fails the test unless the request answers with a 2xx.
func mustSucceed(t *testing.T, method, url string, jsonBody ...string) string {
	t.Helper()
	status, body := request(t, method, url, jsonBody...)
	if status < 200 || status >= 300 {
		t.Fatalf("fixture: %s %s: HTTP %d %.300s", method, url, status, body)
	}
	return body
}

// waitForSnapshots blocks until no snapshot is running on the cluster: the
// snapshot recipes start operations that must not overlap the next request.
func waitForSnapshots(t *testing.T, baseURL string) {
	t.Helper()
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		status, body := request(t, "GET", baseURL+"/_snapshot/_status")
		if status == 200 && strings.Contains(strings.ReplaceAll(body, " ", ""), `"snapshots":[]`) {
			return
		}
	}
	t.Fatalf("snapshots still running on %s after a minute", baseURL)
}

// prepareFixtures gives a cluster what the recipes' variables point at — an
// index with a few documents and a replica that can't be assigned (single
// node: the recipes about unassigned shards then have something to show), a
// snapshot repository, a snapshot — and returns the variables' values.
func prepareFixtures(t *testing.T, cl testclusters.Cluster) map[string]string {
	t.Helper()
	base := cl.URL

	for _, leftover := range []string{fixtureIndex, "restored-" + fixtureIndex} {
		if status, body := request(t, "DELETE", base+"/"+leftover); status != 200 && status != 404 {
			t.Fatalf("fixture: deleting %s: HTTP %d %.300s", leftover, status, body)
		}
	}
	// In case an earlier, interrupted run left them set.
	mustSucceed(t, "PUT", base+"/_cluster/settings",
		`{"persistent": {"cluster.routing.allocation.enable": null, "cluster.routing.allocation.exclude._name": null}}`)

	mustSucceed(t, "PUT", base+"/"+fixtureIndex, `{
		"settings": {"number_of_shards": 1, "number_of_replicas": 1},
		"mappings": {"properties": {
			"@timestamp": {"type": "date"},
			"level": {"type": "keyword"},
			"message": {"type": "text"}
		}}
	}`)
	// An alias, part of the snapshot taken below: what the restore recipe
	// does with it is checked once the recipes have run.
	mustSucceed(t, "POST", base+"/_aliases",
		fmt.Sprintf(`{"actions": [{"add": {"index": %q, "alias": %q}}]}`, fixtureIndex, fixtureAlias))
	for i, level := range []string{"INFO", "WARN", "ERROR"} {
		mustSucceed(t, "PUT", fmt.Sprintf("%s/%s/_doc/%d?refresh=true", base, fixtureIndex, i+1),
			fmt.Sprintf(`{"@timestamp": "2026-01-0%dT10:00:00Z", "level": %q, "message": "fixture document %d"}`, i+1, level, i+1))
	}

	// A block, for the recipe that lists them to have one to show; this one
	// only refuses new documents, which no recipe writes.
	mustSucceed(t, "PUT", base+"/"+fixtureIndex+"/_settings", `{"index.blocks.write": true}`)

	// A new location on every run: a fresh, empty repository, so snapshot
	// names never collide with those of an earlier run.
	location := fmt.Sprintf("/tmp/tdt-snapshots/%d", time.Now().UnixNano())
	mustSucceed(t, "PUT", base+"/_snapshot/"+fixtureRepository,
		fmt.Sprintf(`{"type": "fs", "settings": {"location": %q}}`, location))
	mustSucceed(t, "PUT", fmt.Sprintf("%s/_snapshot/%s/%s?wait_for_completion=true", base, fixtureRepository, fixtureSnapshot),
		fmt.Sprintf(`{"indices": %q, "include_global_state": false}`, fixtureIndex))

	node := strings.TrimSpace(strings.SplitN(mustSucceed(t, "GET", base+"/_cat/nodes?h=name"), "\n", 2)[0])
	if node == "" {
		t.Fatal("fixture: no node name returned by _cat/nodes")
	}

	return map[string]string{
		"index":      fixtureIndex,
		"node":       node,
		"repository": fixtureRepository,
		"snapshot":   fixtureSnapshot,
		"field":      "level",
		// No such task: the cancellation is accepted and reports, in its
		// body, that the node is unknown.
		"task_id": "AAAAAAAAAAAAAAAAAAAAAA:1",
	}
}

var recipeVariableRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expectedFailures lists the recipe requests that can't succeed on the test
// clusters, with the reason — everything else must answer below 400. Keyed
// by recipe title, then distribution; applies to every request of the
// recipe.
var expectedFailures = map[string]map[refdata.Distribution]string{
	"Retry the failed lifecycle step of an index": {
		// (OpenSearch answers 200 with "updated_indices": 0 instead.)
		refdata.Elasticsearch: "the fixture index is not stuck on a lifecycle error",
	},
	"TLS certificates and their expiry": {
		refdata.Elasticsearch: "security is disabled on the test clusters",
	},
}

// noneDefined lists the recipes that list objects of some kind the test
// clusters have none of, where some versions then answer 404 instead of an
// empty list — the recipes' own comments warn about it. The value is the
// text such an answer carries; empty stands for a bare "{}".
var noneDefined = map[string]string{
	"Ingest pipelines":             "",
	"Search pipelines":             "",
	"Snapshot management policies": "config index not found",
}

// emptyOnTestClusters lists the recipes whose answer is a bare "{}" on the
// test clusters, with the reason. Any other recipe answering "{}" is taken
// for a request that selects nothing — a filter that matches no field, say:
// accepted by the cluster, and useless.
var emptyOnTestClusters = map[string]string{
	"Remote clusters":             "none is configured",
	"Snapshot lifecycle policies": "none is defined",
	"Legacy index templates":      "OpenSearch ships none",
}

// answersNoneDefined reports whether an answer is a recipe's documented
// "nothing of that kind yet" (see noneDefined).
func answersNoneDefined(title string, status int, body string) bool {
	marker, listing := noneDefined[title]
	if !listing || status != 404 {
		return false
	}
	if marker == "" {
		return strings.TrimSpace(body) == "{}"
	}
	return strings.Contains(body, marker)
}

// catColumnsUsed returns the _cat command a request path queries and the
// columns its h= and s= parameters name, or ok=false for any other path.
func catColumnsUsed(path string, liveCommands []string) (command string, columns []string, ok bool) {
	path = strings.TrimPrefix(path, "/")
	rest, isCat := strings.CutPrefix(path, "_cat/")
	if !isCat {
		return "", nil, false
	}
	rest, query, _ := strings.Cut(rest, "?")
	for _, cmd := range liveCommands {
		if (rest == cmd || strings.HasPrefix(rest, cmd+"/")) && len(cmd) > len(command) {
			command = cmd
		}
	}
	if command == "" {
		return "", nil, false
	}
	for _, param := range strings.Split(query, "&") {
		name, value, _ := strings.Cut(param, "=")
		if name != "h" && name != "s" {
			continue
		}
		for _, col := range strings.Split(value, ",") {
			col, _, _ = strings.Cut(col, ":") // s=column:desc
			if col != "" {
				columns = append(columns, col)
			}
		}
	}
	return command, columns, true
}

// TestRecipesOnRealClusters runs every embedded recipe, for real, on every
// cluster it is offered for: each request must be accepted (or be a
// documented exception), and every _cat column a recipe names must exist on
// that cluster — an unknown column is silently ignored there, so only this
// check would catch a typo.
func TestRecipesOnRealClusters(t *testing.T) {
	catalog := refdata.Load(refdata.Sources{})

	for _, cl := range clusters(t) {
		cl := cl
		t.Run(cl.Name, func(t *testing.T) {
			t.Parallel() // the clusters are independent
			tg := target(t, cl)
			variables := prepareFixtures(t, cl)
			liveCommands := catCommandsOf(t, cl)
			liveColumns := make(map[string]map[string]bool)

			recipes := catalog.Recipes(tg)
			if len(recipes) < 40 {
				t.Fatalf("only %d recipes offered for this cluster", len(recipes))
			}
			for _, recipe := range recipes {
				text := recipeVariableRe.ReplaceAllStringFunc(recipe.Body, func(v string) string {
					value, known := variables[v[2:len(v)-1]]
					if !known {
						t.Errorf("%q: no fixture for variable %s", recipe.Title, v)
					}
					return value
				})

				for _, req := range parser.ParseAll(text) {
					path := strings.TrimPrefix(req.Path, "/")

					if command, columns, isCat := catColumnsUsed(path, liveCommands); isCat {
						if liveColumns[command] == nil {
							liveColumns[command] = make(map[string]bool)
							_, help := request(t, "GET", fmt.Sprintf("%s/_cat/%s?help", cl.URL, command))
							for _, col := range refdata.ParseCatHelp([]byte(help)) {
								liveColumns[command][col] = true
							}
						}
						for _, col := range columns {
							if !liveColumns[command][col] {
								t.Errorf("%q: _cat/%s has no column %q on this cluster", recipe.Title, command, col)
							}
						}
					}

					var status int
					var body string
					if len(req.Body) > 0 {
						status, body = request(t, req.Method, cl.URL+"/"+path, string(req.Body))
					} else {
						status, body = request(t, req.Method, cl.URL+"/"+path)
					}
					if answersNoneDefined(recipe.Title, status, body) {
						continue
					}
					reason, mayFail := expectedFailures[recipe.Title][tg.Distribution]
					switch {
					case status < 400 && mayFail:
						t.Errorf("%q: %s %s was expected to fail (%s) but answered HTTP %d", recipe.Title, req.Method, path, reason, status)
					case status >= 400 && !mayFail:
						t.Errorf("%q: %s %s: HTTP %d %.300s", recipe.Title, req.Method, path, status, body)
					case req.Method == "GET" && strings.TrimSpace(body) == "{}" && emptyOnTestClusters[recipe.Title] == "":
						t.Errorf("%q: %s %s answers an empty {}: does the request select anything?", recipe.Title, req.Method, path)
					}

					if strings.Contains(path, "_snapshot/") && req.Method != "GET" {
						waitForSnapshots(t, cl.URL)
					}
				}
			}

			// The restore recipe brings an index back next to the live one:
			// the copy must not have joined the live index's aliases, where
			// every search through them would see each document twice.
			if _, aliased := request(t, "GET", cl.URL+"/_alias/"+fixtureAlias); !strings.Contains(aliased, `"`+fixtureIndex+`"`) || strings.Contains(aliased, "restored-"+fixtureIndex) {
				t.Errorf("expected alias %s to name %s alone after the restore recipe, got %.300s", fixtureAlias, fixtureIndex, aliased)
			}
		})
	}
}

// catCommandsOf lists the _cat commands a cluster announces ("GET _cat").
func catCommandsOf(t *testing.T, cl testclusters.Cluster) []string {
	t.Helper()
	status, body := request(t, "GET", cl.URL+"/_cat")
	if status != 200 {
		t.Fatalf("%s: GET _cat returned HTTP %d", cl.Name, status)
	}
	return refdata.ParseCatListing([]byte(body))
}

// TestCatColumnsOnRealClusters checks the embedded _cat table against what
// each cluster reports: the same commands, and — the table being a
// deliberately conservative fallback — no column the cluster doesn't have.
// It also exercises ParseCatHelp on every real "?help" response.
func TestCatColumnsOnRealClusters(t *testing.T) {
	catalog := refdata.Load(refdata.Sources{})

	for _, cl := range clusters(t) {
		cl := cl
		t.Run(cl.Name, func(t *testing.T) {
			t.Parallel() // the clusters are independent
			embedded := catalog.CatColumns(target(t, cl))

			live := make(map[string]bool)
			for _, cmd := range catCommandsOf(t, cl) {
				live[cmd] = true
				if _, known := embedded[cmd]; !known {
					t.Errorf("_cat/%s exists on the cluster but isn't in the embedded table", cmd)
				}
			}

			for cmd, columns := range embedded {
				if !live[cmd] {
					t.Errorf("_cat/%s is in the embedded table but the cluster doesn't announce it", cmd)
					continue
				}
				status, body := request(t, "GET", fmt.Sprintf("%s/_cat/%s?help", cl.URL, cmd))
				if status != 200 {
					t.Errorf("_cat/%s?help returned HTTP %d", cmd, status)
					continue
				}
				has := make(map[string]bool)
				for _, col := range refdata.ParseCatHelp([]byte(body)) {
					has[col] = true
				}
				if len(has) == 0 {
					t.Errorf("_cat/%s?help: no column parsed from %.120s", cmd, body)
				}
				for _, col := range columns {
					if !has[col] {
						t.Errorf("_cat/%s: embedded column %q doesn't exist on the cluster", cmd, col)
					}
				}
			}
		})
	}
}
