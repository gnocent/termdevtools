package esclient

import (
	"net/url"
	"strings"
)

// readPatterns lists the POST requests that only read from the cluster, as
// the part of the path that starts at the API (see apiSegments): "*" stands
// for exactly one segment of any value. Every other POST, and every PUT or
// DELETE, is a write — including those that lose no data but act on the
// cluster (_refresh, _flush, _cache/clear, _forcemerge, _cluster/reroute):
// what a protected cluster guards against is being acted on, not only being
// written to. An allowlist rather than a denylist: a read missing from it is
// blocked for nothing, whereas a write missing from a denylist would go
// through. See SPEC.md §5.
//
// One list for both distributions: a pattern only Elasticsearch knows,
// sent to OpenSearch, just gets a 404 there. Left out on purpose:
// OpenSearch's _plugins/_sql, which runs a DELETE statement when
// plugins.sql.delete.enabled is set — something that can't be known from
// here — and _plugins/_ppl, whose machine-learning commands make "read only"
// a claim this list can't vouch for.
var readPatterns = splitPatterns(
	// Both distributions.
	"_search",
	"_search/scroll",
	"_search/scroll/*",
	"_search/template",
	"_msearch",
	"_msearch/template",
	"_render/template",
	"_render/template/*",
	"_count",
	"_mget",
	"_termvectors",
	"_termvectors/*",
	"_mtermvectors",
	"_field_caps",
	"_validate/query",
	"_explain/*",
	"_rank_eval",
	"_search_shards",
	"_analyze",
	"_cluster/allocation/explain",
	"_ingest/pipeline/_simulate",
	"_ingest/pipeline/*/_simulate",
	"_index_template/_simulate",
	"_index_template/_simulate/*",
	"_index_template/_simulate_index/*",
	// Elasticsearch.
	"_terms_enum",
	"_knn_search",
	"_eql/search",
	"_async_search",
	"_sql",
	"_sql/translate",
	"_sql/close",
	"_query",
	"_query/async",
	"_pit",
	"_disk_usage",
	"_ingest/_simulate",
	"_ingest/*/_simulate",
	"_security/user/_has_privileges",
	"_security/user/*/_has_privileges",
	"_mvt/*/*/*/*",
	// OpenSearch.
	"_search/point_in_time",
	"_plugins/_asynchronous_search",
)

// writingGets lists the GET requests that act on the cluster all the same,
// as patterns of readPatterns' form: Elasticsearch and OpenSearch take GET as
// well as POST for a refresh or a flush, and OpenSearch's SQL and PPL — left
// out of readPatterns for the statements they may run — read their query
// from the URL too.
var writingGets = splitPatterns(
	"_refresh",
	"_flush",
	"_flush/synced",
	"_plugins/_sql",
	"_opendistro/_sql",
	"_plugins/_ppl",
	"_opendistro/_ppl",
)

func splitPatterns(patterns ...string) [][]string {
	split := make([][]string, len(patterns))
	for i, pattern := range patterns {
		split[i] = strings.Split(pattern, "/")
	}
	return split
}

// IsRead reports whether a request only reads from the cluster: a GET whose
// API isn't in writingGets, and a POST whose API is in readPatterns.
// Everything else is a write, which a protected cluster confirms or refuses
// (SPEC.md §5), and so is a path with a "." or ".." segment: a proxy that
// normalizes paths would send another one. path is the one actually sent,
// variables already substituted: a variable may hold the API itself.
func IsRead(method, path string) bool {
	for _, segment := range strings.Split(sentPath(path), "/") {
		if decoded, err := url.PathUnescape(segment); err == nil && (decoded == "." || decoded == "..") {
			return false
		}
	}
	api := apiSegments(path)
	switch strings.ToUpper(method) {
	case "GET":
		return !matchesAny(writingGets, api)
	case "POST":
		return matchesAny(readPatterns, api)
	}
	return false
}

// sentPath returns the part of path a request sends as its path, the way
// Go's HTTP client parses it out of the URL (url.Parse): without what follows
// a "#", a fragment that is never sent, then without the query string.
// Classifying the whole of "_security/user/bob#/_has_privileges" would take
// for a read the "POST _security/user/bob" that is sent.
func sentPath(path string) string {
	path, _, _ = strings.Cut(path, "#")
	path, _, _ = strings.Cut(path, "?")
	return path
}

// apiSegments returns the segments of the path path sends (sentPath) from
// its API on: from the first segment starting with "_" that isn't the
// special "_all" target, since no index, alias or data stream name may start
// with "_". nil when there is none — not a path any read is sent to. A "_"
// written as "%5F" isn't decoded: such a path is classified as a write,
// which is the safe side to err on.
func apiSegments(path string) []string {
	segments := strings.Split(strings.Trim(sentPath(path), "/"), "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, "_") && segment != "_all" {
			return segments[i:]
		}
	}
	return nil
}

func matchesAny(patterns [][]string, api []string) bool {
	for _, pattern := range patterns {
		if matchSegments(pattern, api) {
			return true
		}
	}
	return false
}

// matchSegments reports whether api matches pattern as a whole — a mere
// prefix doesn't: "_search/foo" isn't "_search". A "*" stands for one
// segment, however it is read: neither an empty one nor one holding an
// encoded "/".
func matchSegments(pattern, api []string) bool {
	if len(pattern) != len(api) {
		return false
	}
	for i, segment := range pattern {
		if segment == "*" {
			decoded, err := url.PathUnescape(api[i])
			if err != nil || decoded == "" || strings.Contains(decoded, "/") {
				return false
			}
		} else if segment != api[i] {
			return false
		}
	}
	return true
}
