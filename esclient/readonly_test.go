package esclient

import (
	"strings"
	"testing"
)

// TestIsRead checks the classification protected clusters rely on (SPEC.md
// §5): the cases of the specification, reads first, then writes.
func TestIsRead(t *testing.T) {
	cases := []struct {
		method, path string
		read         bool
	}{
		{"GET", "_cluster/health", true},
		{"GET", "my-index/_doc/42", true},
		{"GET", "/_cat/indices?v", true},
		{"POST", "_search", true},
		{"POST", "/my-index/_search?size=0", true},
		{"POST", "_all/_search", true},
		{"POST", "logs-*,metrics-*/_count", true},
		{"POST", "my-index/_explain/42", true},
		{"POST", "_ingest/pipeline/my-pipe/_simulate", true},
		{"POST", "_search/scroll", true},
		{"POST", "_search/", true},
		{"post", "_search", true},
		{"get", "_cluster/health", true},
		{"POST", "my-index/_search#top", true},
		{"GET", "_plugins/_sql/stats", true},
		{"GET", "my-index/_doc/a%2Fb", true},

		{"POST", "my-index/_doc", false},
		{"POST", "my-index/_doc/_search", false},
		{"POST", "my-index/_update/42", false},
		{"POST", "_bulk", false},
		{"POST", "my-index/_delete_by_query", false},
		{"POST", "my-index/_update_by_query", false},
		{"POST", "_reindex", false},
		{"POST", "_search/foo", false},
		{"POST", "my-index/_explain", false},
		{"POST", "my-index/_refresh", false},
		{"POST", "my-index/_flush", false},
		{"POST", "_cache/clear", false},
		{"POST", "my-index/_forcemerge", false},
		{"POST", "_cluster/reroute", false},
		{"POST", "my-index/_close", false},
		{"POST", "_plugins/_sql", false},
		{"POST", "_plugins/_ppl", false},
		{"POST", "my-index", false},
		{"POST", "_all", false},
		{"POST", "", false},
		{"POST", "my-index/%5Fsearch", false},
		{"PUT", "my-index/_settings", false},
		{"PUT", "my-index", false},
		{"PUT", "_search", false},
		{"DELETE", "my-index", false},
		{"DELETE", "_search/scroll", false},
		{"HEAD", "my-index", false},
		{"", "_search", false},

		// What follows a "#" is never sent: "POST _security/user/bob" is.
		{"POST", "_security/user/bob#/_has_privileges", false},
		{"POST", "my-index/_doc#/_search", false},
		{"POST", "my-index/_doc#?x=/_search", false},
		// A proxy normalizing the path would send "logs/_bulk".
		{"POST", "my-index/_mvt/../../logs/_bulk", false},
		{"POST", "my-index/_mvt/%2E%2E/%2e%2e/logs/_bulk", false},
		{"POST", "logs/../_search", false},
		{"GET", "my-index/../_flush", false},
		// A "*" stands for one segment, not an empty one or one holding "/".
		{"POST", "my-index/_explain/", false},
		{"POST", "my-index/_explain/a%2F_bulk", false},
		// GETs that act on the cluster.
		{"GET", "my-index/_refresh", false},
		{"get", "_refresh", false},
		{"GET", "_flush", false},
		{"GET", "my-index/_flush/synced", false},
		{"GET", "_plugins/_sql?sql=DELETE%20FROM%20my-index", false},
		{"GET", "_opendistro/_sql?sql=x", false},
		{"GET", "_plugins/_ppl?query=x", false},
		{"GET", "_opendistro/_ppl?query=x", false},
	}
	for _, c := range cases {
		if got := IsRead(c.method, c.path); got != c.read {
			t.Errorf("IsRead(%q, %q) = %v, expected %v", c.method, c.path, got, c.read)
		}
	}
}

// readExamples holds a POST request matching each pattern of readPatterns,
// as it would be written in the editor.
var readExamples = []string{
	"my-index/_search",
	"_search/scroll",
	"_search/scroll/DXF1ZXJ5QW5kRmV0Y2gBAAAAAAAAAD4WYm9laVYtZndUQlNsdDcwakFMNjU1QQ==",
	"my-index/_search/template",
	"_msearch",
	"my-index/_msearch/template",
	"_render/template",
	"_render/template/my-template",
	"my-index/_count",
	"my-index/_mget",
	"my-index/_termvectors",
	"my-index/_termvectors/42",
	"my-index/_mtermvectors",
	"logs-*/_field_caps?fields=*",
	"my-index/_validate/query?explain",
	"my-index/_explain/42",
	"my-index/_rank_eval",
	"my-index/_search_shards",
	"my-index/_analyze",
	"_cluster/allocation/explain",
	"_ingest/pipeline/_simulate",
	"_ingest/pipeline/my-pipe/_simulate?verbose",
	"_index_template/_simulate",
	"_index_template/_simulate/my-template",
	"_index_template/_simulate_index/my-index",
	"my-index/_terms_enum",
	"my-index/_knn_search",
	"my-index/_eql/search",
	"my-index/_async_search?wait_for_completion_timeout=2s",
	"_sql?format=txt",
	"_sql/translate",
	"_sql/close",
	"_query",
	"_query/async",
	"my-index/_pit?keep_alive=1m",
	"my-index/_disk_usage?run_expensive_tasks=true",
	"_ingest/_simulate",
	"_ingest/my-index/_simulate",
	"_security/user/_has_privileges",
	"_security/user/jdoe/_has_privileges",
	"my-index/_mvt/location/6/32/22",
	"my-index/_search/point_in_time?keep_alive=1m",
	"_plugins/_asynchronous_search?index=my-index",
}

// TestIsReadEveryPattern checks that every pattern has an example among
// readExamples — so that one added later can't go untested — and that each
// example is classified as a read.
func TestIsReadEveryPattern(t *testing.T) {
	for _, pattern := range readPatterns {
		found := false
		for _, example := range readExamples {
			if matchSegments(pattern, apiSegments(example)) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no example in readExamples for pattern %q", strings.Join(pattern, "/"))
		}
	}
	for _, example := range readExamples {
		if !IsRead("POST", example) {
			t.Errorf("IsRead(POST, %q) = false, expected a read", example)
		}
	}
}
