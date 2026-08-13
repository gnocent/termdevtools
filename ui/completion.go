package ui

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// knownEndpoints is the default list (compiled into the binary) of the most
// common Elasticsearch endpoints for administration/operations, offered by
// Tab auto-completion (SPEC.md §3.2). Only used when there's no
// endpoints.txt file next to the binary (LoadEndpointsFile), which the team
// can maintain without recompiling on every new Elasticsearch version.
//
// Extracted from the official OpenAPI spec (elastic/elasticsearch-specification,
// branch 9.5, output/openapi/elasticsearch-openapi.json), filtered to
// endpoints with no path parameter (`/{index}/...` ones need a real index
// name — no dynamic index discovery, see SPEC.md §7) and to "core admin"
// domains: `_cat` (all commands, `?v` systematically for column headers),
// `_cluster`, `_nodes`, index/search/snapshot/ILM/SLM/license. Deliberately
// left out: ML, security, watcher, transform, rollup, SQL/ES|QL, CCR,
// connectors, inference, enrich — outside this tool's core admin scope.
var knownEndpoints = []string{
	// _cat/* — verbose (?v) systematically, for column headers.
	"_cat/aliases?v",
	"_cat/allocation?v",
	"_cat/circuit_breaker?v",
	"_cat/component_templates?v",
	"_cat/count?v",
	"_cat/fielddata?v",
	"_cat/health?v",
	"_cat/indices?v",
	"_cat/master?v",
	"_cat/ml/anomaly_detectors?v",
	"_cat/ml/data_frame/analytics?v",
	"_cat/ml/datafeeds?v",
	"_cat/ml/trained_models?v",
	"_cat/nodeattrs?v",
	"_cat/nodes?v",
	"_cat/pending_tasks?v",
	"_cat/plugins?v",
	"_cat/recovery?v",
	"_cat/repositories?v",
	"_cat/segments?v",
	"_cat/shards?v",
	"_cat/snapshots?v",
	"_cat/tasks?v",
	"_cat/templates?v",
	"_cat/thread_pool?v",
	"_cat/transforms?v",

	// _cluster/*
	"_cluster/allocation/explain",
	"_cluster/health",
	"_cluster/pending_tasks",
	"_cluster/reroute",
	"_cluster/settings",
	"_cluster/state",
	"_cluster/stats",
	"_cluster/voting_config_exclusions",

	// _nodes/*
	"_nodes",
	"_nodes/hot_threads",
	"_nodes/reload_secure_settings",
	"_nodes/stats",
	"_nodes/usage",

	// Index, document, search, snapshot, ILM/SLM, license.
	"_alias",
	"_aliases",
	"_analyze",
	"_bulk",
	"_cache/clear",
	"_component_template",
	"_count",
	"_data_stream",
	"_data_stream/_modify",
	"_data_stream/_stats",
	"_features",
	"_features/_reset",
	"_field_caps",
	"_flush",
	"_forcemerge",
	"_health_report",
	"_ilm/migrate_to_data_tiers",
	"_ilm/policy",
	"_ilm/start",
	"_ilm/status",
	"_ilm/stop",
	"_index_template",
	"_index_template/_simulate",
	"_license",
	"_license/basic_status",
	"_license/start_basic",
	"_license/start_trial",
	"_license/trial_status",
	"_mapping",
	"_mget",
	"_msearch",
	"_msearch/template",
	"_mtermvectors",
	"_recovery",
	"_refresh",
	"_reindex",
	"_remote/info",
	"_resolve/cluster",
	"_search",
	"_search/scroll",
	"_search/template",
	"_search_shards",
	"_segments",
	"_settings",
	"_shard_stores",
	"_slm/_execute_retention",
	"_slm/policy",
	"_slm/start",
	"_slm/stats",
	"_slm/status",
	"_slm/stop",
	"_snapshot",
	"_snapshot/_status",
	"_stats",
	"_tasks",
	"_tasks/_cancel",
	"_template",
	"_validate/query",
}

// catColumns is the default table (compiled into the binary) of columns
// available for h= (displayed columns) and s= (sort) of each _cat/*
// command, offered by Tab auto-completion (SPEC.md §3.2). Only used when
// there's no cat_columns.txt file next to the binary (LoadCatColumnsFile).
//
// Only full column names are listed (not short aliases like "dc" for
// "docs.count"): more descriptive in the suggestion list, and it limits the
// number of suggestions for commands with many columns (e.g. _cat/indices,
// _cat/nodes).
//
// Generated on 2026-08-12 from a real Elasticsearch 9.5.0 cluster (GET
// _cat/<command>?help for each of knownEndpoints' commands) — see SPEC.md
// §3.2 for the method.
var catColumns = map[string][]string{
	"aliases": {
		"alias", "index", "filter", "routing.index", "routing.search", "is_write_index",
	},
	"allocation": {
		"shards", "shards.undesired", "write_load.forecast", "disk.indices.forecast", "disk.indices", "disk.used",
		"disk.avail", "disk.total", "disk.percent", "host", "ip", "node",
		"node.role",
	},
	"circuit_breaker": {
		"node_id", "node_name", "breaker", "limit", "limit_bytes", "estimated",
		"estimated_bytes", "tripped", "overhead",
	},
	"component_templates": {
		"name", "version", "alias_count", "mapping_count", "settings_count", "metadata_count",
		"included_in",
	},
	"count": {
		"epoch", "timestamp", "count",
	},
	"fielddata": {
		"id", "host", "ip", "node", "field", "size",
	},
	"health": {
		"epoch", "timestamp", "cluster", "status", "node.total", "node.data",
		"shards", "pri", "relo", "init", "unassign", "unassign.pri",
		"pending_tasks", "max_task_wait_time", "active_shards_percent",
	},
	"indices": {
		"health", "status", "index", "uuid", "pri", "rep",
		"docs.count", "docs.deleted", "creation.date", "creation.date.string", "store.size", "pri.store.size",
		"dataset.size", "completion.size", "pri.completion.size", "fielddata.memory_size", "pri.fielddata.memory_size", "fielddata.evictions",
		"pri.fielddata.evictions", "query_cache.memory_size", "pri.query_cache.memory_size", "query_cache.evictions", "pri.query_cache.evictions", "request_cache.memory_size",
		"pri.request_cache.memory_size", "request_cache.evictions", "pri.request_cache.evictions", "request_cache.hit_count", "pri.request_cache.hit_count", "request_cache.miss_count",
		"pri.request_cache.miss_count", "flush.total", "pri.flush.total", "flush.total_time", "pri.flush.total_time", "get.current",
		"pri.get.current", "get.time", "pri.get.time", "get.total", "pri.get.total", "get.exists_time",
		"pri.get.exists_time", "get.exists_total", "pri.get.exists_total", "get.missing_time", "pri.get.missing_time", "get.missing_total",
		"pri.get.missing_total", "indexing.delete_current", "pri.indexing.delete_current", "indexing.delete_time", "pri.indexing.delete_time", "indexing.delete_total",
		"pri.indexing.delete_total", "indexing.index_current", "pri.indexing.index_current", "indexing.index_time", "pri.indexing.index_time", "indexing.index_total",
		"pri.indexing.index_total", "indexing.index_failed", "pri.indexing.index_failed", "indexing.index_failed_due_to_version_conflict", "pri.indexing.index_failed_due_to_version_conflict", "merges.current",
		"pri.merges.current", "merges.current_docs", "pri.merges.current_docs", "merges.current_size", "pri.merges.current_size", "merges.total",
		"pri.merges.total", "merges.total_docs", "pri.merges.total_docs", "merges.total_size", "pri.merges.total_size", "merges.total_time",
		"pri.merges.total_time", "refresh.total", "pri.refresh.total", "refresh.time", "pri.refresh.time", "refresh.external_total",
		"pri.refresh.external_total", "refresh.external_time", "pri.refresh.external_time", "refresh.listeners", "pri.refresh.listeners", "search.fetch_current",
		"pri.search.fetch_current", "search.fetch_time", "pri.search.fetch_time", "search.fetch_total", "pri.search.fetch_total", "search.open_contexts",
		"pri.search.open_contexts", "search.query_current", "pri.search.query_current", "search.query_time", "pri.search.query_time", "search.query_total",
		"pri.search.query_total", "search.scroll_current", "pri.search.scroll_current", "search.scroll_time", "pri.search.scroll_time", "search.scroll_total",
		"pri.search.scroll_total", "segments.count", "pri.segments.count", "segments.memory", "pri.segments.memory", "segments.index_writer_memory",
		"pri.segments.index_writer_memory", "segments.version_map_memory", "pri.segments.version_map_memory", "segments.fixed_bitset_memory", "pri.segments.fixed_bitset_memory", "warmer.current",
		"pri.warmer.current", "warmer.total", "pri.warmer.total", "warmer.total_time", "pri.warmer.total_time", "suggest.current",
		"pri.suggest.current", "suggest.time", "pri.suggest.time", "suggest.total", "pri.suggest.total", "memory.total",
		"pri.memory.total", "bulk.total_operations", "pri.bulk.total_operations", "bulk.total_time", "pri.bulk.total_time", "bulk.total_size_in_bytes",
		"pri.bulk.total_size_in_bytes", "bulk.avg_time", "pri.bulk.avg_time", "bulk.avg_size_in_bytes", "pri.bulk.avg_size_in_bytes", "dense_vector.value_count",
		"pri.dense_vector.value_count", "sparse_vector.value_count", "pri.sparse_vector.value_count",
	},
	"master": {
		"id", "host", "ip", "node",
	},
	"ml/anomaly_detectors": {
		"id", "state", "opened_time", "assignment_explanation", "data.processed_records", "data.processed_fields",
		"data.input_bytes", "data.input_records", "data.input_fields", "data.invalid_dates", "data.missing_fields", "data.out_of_order_timestamps",
		"data.empty_buckets", "data.sparse_buckets", "data.buckets", "data.earliest_record", "data.latest_record", "data.last",
		"data.last_empty_bucket", "data.last_sparse_bucket", "model.bytes", "model.memory_status", "model.bytes_exceeded", "model.memory_limit",
		"model.by_fields", "model.over_fields", "model.partition_fields", "model.bucket_allocation_failures", "model.output_memory_allocator_bytes", "model.categorization_status",
		"model.categorized_doc_count", "model.total_category_count", "model.frequent_category_count", "model.rare_category_count", "model.dead_category_count", "model.failed_category_count",
		"model.log_time", "model.timestamp", "forecasts.total", "forecasts.memory.min", "forecasts.memory.max", "forecasts.memory.avg",
		"forecasts.memory.total", "forecasts.records.min", "forecasts.records.max", "forecasts.records.avg", "forecasts.records.total", "forecasts.time.min",
		"forecasts.time.max", "forecasts.time.avg", "forecasts.time.total", "node.id", "node.name", "node.ephemeral_id",
		"node.address", "buckets.count", "buckets.time.total", "buckets.time.min", "buckets.time.max", "buckets.time.exp_avg",
		"buckets.time.exp_avg_hour",
	},
	"ml/data_frame/analytics": {
		"id", "type", "create_time", "version", "source_index", "dest_index",
		"description", "model_memory_limit", "state", "failure_reason", "progress", "assignment_explanation",
		"node.id", "node.name", "node.ephemeral_id", "node.address",
	},
	"ml/datafeeds": {
		"id", "state", "assignment_explanation", "buckets.count", "search.count", "search.time",
		"search.bucket_avg", "search.exp_avg_hour", "node.id", "node.name", "node.ephemeral_id", "node.address",
	},
	"ml/trained_models": {
		"id", "created_by", "heap_size", "operations", "license", "create_time",
		"version", "description", "type", "ingest.pipelines", "ingest.count", "ingest.time",
		"ingest.current", "ingest.failed", "data_frame.id", "data_frame.create_time", "data_frame.source_index", "data_frame.analysis",
	},
	"nodeattrs": {
		"node", "id", "pid", "host", "ip", "port",
		"attr", "value",
	},
	"nodes": {
		"id", "pid", "ip", "port", "http_address", "version",
		"type", "build", "jdk", "disk.total", "disk.used", "disk.avail",
		"disk.used_percent", "heap.current", "heap.percent", "heap.max", "ram.current", "ram.percent",
		"ram.max", "file_desc.current", "file_desc.percent", "file_desc.max", "cpu", "load_1m",
		"load_5m", "load_15m", "available_processors", "uptime", "node.role", "master",
		"name", "completion.size", "fielddata.memory_size", "fielddata.evictions", "query_cache.memory_size", "query_cache.evictions",
		"query_cache.hit_count", "query_cache.miss_count", "request_cache.memory_size", "request_cache.evictions", "request_cache.hit_count", "request_cache.miss_count",
		"flush.total", "flush.total_time", "get.current", "get.time", "get.total", "get.exists_time",
		"get.exists_total", "get.missing_time", "get.missing_total", "indexing.delete_current", "indexing.delete_time", "indexing.delete_total",
		"indexing.index_current", "indexing.index_time", "indexing.index_total", "indexing.index_failed", "indexing.index_failed_due_to_version_conflict", "merges.current",
		"merges.current_docs", "merges.current_size", "merges.total", "merges.total_docs", "merges.total_size", "merges.total_time",
		"refresh.total", "refresh.time", "refresh.external_total", "refresh.external_time", "refresh.listeners", "script.compilations",
		"script.cache_evictions", "script.compilation_limit_triggered", "search.fetch_current", "search.fetch_time", "search.fetch_total", "search.open_contexts",
		"search.query_current", "search.query_time", "search.query_total", "search.scroll_current", "search.scroll_time", "search.scroll_total",
		"segments.count", "segments.memory", "segments.index_writer_memory", "segments.version_map_memory", "segments.fixed_bitset_memory", "suggest.current",
		"suggest.time", "suggest.total", "bulk.total_operations", "bulk.total_time", "bulk.total_size_in_bytes", "bulk.avg_time",
		"bulk.avg_size_in_bytes", "shard_stats.total_count", "mappings.total_count", "mappings.total_estimated_overhead_in_bytes",
	},
	"pending_tasks": {
		"insertOrder", "timeInQueue", "priority", "source",
	},
	"plugins": {
		"id", "name", "component", "version", "description",
	},
	"recovery": {
		"index", "shard", "start_time", "start_time_millis", "stop_time", "stop_time_millis",
		"time", "type", "stage", "source_host", "source_node", "target_host",
		"target_node", "repository", "snapshot", "files", "files_recovered", "files_percent",
		"files_total", "bytes", "bytes_recovered", "bytes_percent", "bytes_total", "translog_ops",
		"translog_ops_recovered", "translog_ops_percent",
	},
	"repositories": {
		"id", "type",
	},
	"segments": {
		"index", "shard", "prirep", "ip", "id", "segment",
		"generation", "docs.count", "docs.deleted", "size", "size.memory", "committed",
		"searchable", "version", "compound",
	},
	"shards": {
		"index", "shard", "prirep", "state", "docs", "store",
		"dataset", "ip", "id", "node", "unassigned.reason", "unassigned.at",
		"unassigned.for", "unassigned.details", "recoverysource.type", "completion.size", "fielddata.memory_size", "fielddata.evictions",
		"query_cache.memory_size", "query_cache.evictions", "flush.total", "flush.total_time", "get.current", "get.time",
		"get.total", "get.exists_time", "get.exists_total", "get.missing_time", "get.missing_total", "indexing.delete_current",
		"indexing.delete_time", "indexing.delete_total", "indexing.index_current", "indexing.index_time", "indexing.index_total", "indexing.index_failed",
		"indexing.index_failed_due_to_version_conflict", "merges.current", "merges.current_docs", "merges.current_size", "merges.total", "merges.total_docs",
		"merges.total_size", "merges.total_time", "refresh.total", "refresh.time", "refresh.external_total", "refresh.external_time",
		"refresh.listeners", "search.fetch_current", "search.fetch_time", "search.fetch_total", "search.open_contexts", "search.query_current",
		"search.query_time", "search.query_total", "search.scroll_current", "search.scroll_time", "search.scroll_total", "segments.count",
		"segments.memory", "segments.index_writer_memory", "segments.version_map_memory", "segments.fixed_bitset_memory", "seq_no.max", "seq_no.local_checkpoint",
		"seq_no.global_checkpoint", "warmer.current", "warmer.total", "warmer.total_time", "path.data", "path.state",
		"bulk.total_operations", "bulk.total_time", "bulk.total_size_in_bytes", "bulk.avg_time", "bulk.avg_size_in_bytes", "dense_vector.value_count",
		"sparse_vector.value_count",
	},
	"snapshots": {
		"id", "repository", "status", "start_epoch", "start_time", "end_epoch",
		"end_time", "duration", "indices", "successful_shards", "failed_shards", "total_shards",
		"reason",
	},
	"tasks": {
		"id", "action", "task_id", "parent_task_id", "type", "start_time",
		"timestamp", "running_time_ns", "running_time", "node_id", "ip", "port",
		"node", "version", "x_opaque_id",
	},
	"templates": {
		"name", "index_patterns", "order", "version", "composed_of",
	},
	"thread_pool": {
		"node_name", "node_id", "ephemeral_node_id", "pid", "host", "ip",
		"port", "name", "type", "active", "pool_size", "queue",
		"queue_size", "rejected", "largest", "completed", "core", "max",
		"size", "keep_alive",
	},
	"transforms": {
		"id", "state", "checkpoint", "documents_processed", "checkpoint_progress", "last_search_time",
		"changes_last_detection_time", "create_time", "version", "source_index", "project_routing", "dest_index",
		"pipeline", "description", "transform_type", "frequency", "max_page_search_size", "docs_per_second",
		"reason", "search_total", "search_failure", "search_time", "index_total", "index_failure",
		"index_time", "documents_indexed", "delete_time", "documents_deleted", "trigger_count", "pages_processed",
		"processing_time", "checkpoint_duration_time_exp_avg", "indexed_documents_exp_avg", "processed_documents_exp_avg",
	},
}

// matchPrefix returns, among candidates, those starting with prefix
// (case-insensitive), sorted. An empty prefix returns the full list. Used
// both for endpoints and for _cat columns (h=/s=) and sort directions
// (asc/desc).
func matchPrefix(prefix string, candidates []string) []string {
	lower := strings.ToLower(prefix)
	var matches []string
	for _, c := range candidates {
		if strings.HasPrefix(strings.ToLower(c), lower) {
			matches = append(matches, c)
		}
	}
	sort.Strings(matches)
	return matches
}

// LoadEndpointsFile reads an endpoints file — one per line, blank lines
// and comments ('#') ignored — so the team can maintain the list offered
// by auto-completion without recompiling (SPEC.md §3.2, §9.1). Returns
// (nil, nil) if the file doesn't exist: the caller then falls back to
// knownEndpoints.
func LoadEndpointsFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var endpoints []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		endpoints = append(endpoints, line)
	}
	return endpoints, nil
}

// catColumnSectionRe recognizes a section header line in a _cat columns
// file (e.g. "# _cat/indices") — see LoadCatColumnsFile.
var catColumnSectionRe = regexp.MustCompile(`^#\s*_cat/(\S+)\s*$`)

// LoadCatColumnsFile reads a _cat columns file organized into "# _cat/command"
// sections followed by one column (full name or alias) per line, so the
// team can maintain the list offered by h=/s= auto-completion without
// recompiling (SPEC.md §3.2, §9.1). Other lines starting with '#' are plain
// comments. Returns (nil, nil) if the file doesn't exist: the caller then
// falls back to catColumns.
func LoadCatColumnsFile(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	result := make(map[string][]string)
	var current string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if m := catColumnSectionRe.FindStringSubmatch(trimmed); m != nil {
			current = m[1]
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if current == "" {
			continue
		}
		result[current] = append(result[current], trimmed)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// sortDirections are the possible values for a column's sort direction in
// the s= parameter (e.g. "s=docs.count:desc").
var sortDirections = []string{"asc", "desc"}

// matchCatCommand looks, among the keys of columns, for the longest (most
// "covering") _cat command that prefixes path — either an exact match, or
// followed by a '/' (never a partial word match: "shardsxyz" must not
// match "shards"). Necessary because many _cat commands accept a filter at
// the end of the path before the parameters, e.g. "_cat/shards/myindex?h=..."
// (filtering on index "myindex") — without this, "shards/myindex" would
// match no known command and no column would be suggested.
func matchCatCommand(path string, columns map[string][]string) (command string, ok bool) {
	for cmd := range columns {
		if cmd == "" {
			continue
		}
		if path == cmd || strings.HasPrefix(path, cmd+"/") {
			if len(cmd) > len(command) {
				command = cmd
				ok = true
			}
		}
	}
	return command, ok
}

// catColumnCompletion attempts to interpret prefix (the text typed after
// the HTTP method, see Editor.CompletionPrefix) as an in-progress edit
// inside the h= (displayed columns) or s= (sort) parameter of a _cat/*
// command, e.g. "_cat/indices?h=health,st" or "_cat/shards?s=docs.count:de".
// Returns ok=false if prefix doesn't match this case (a regular endpoint,
// not a _cat command, or not inside an h=/s= parameter) — the caller then
// falls back to regular endpoint completion. subPrefixLen (in runes) allows
// computing the portion to replace: only the column (or direction) being
// typed, not everything before it.
func catColumnCompletion(prefix string, columns map[string][]string) (candidates []string, subPrefixLen int, ok bool) {
	if !strings.HasPrefix(prefix, "_cat/") {
		return nil, 0, false
	}
	qIdx := strings.IndexByte(prefix, '?')
	if qIdx < 0 {
		return nil, 0, false
	}
	path := prefix[len("_cat/"):qIdx]
	query := prefix[qIdx+1:]

	// Many _cat commands accept a filter at the end of the path before the
	// parameters (e.g. "_cat/shards/myindex?h=..." filtering on index
	// "myindex"). So we recognize the longest (most "covering") _cat
	// command that prefixes path at a '/' boundary — not a partial word
	// match (e.g. "shardsxyz" must not match "shards").
	command, ok := matchCatCommand(path, columns)
	if !ok {
		return nil, 0, false
	}

	// The parameter currently being typed: whatever follows the last '&'
	// (or the whole query string if there is none).
	param := query
	if amp := strings.LastIndexByte(query, '&'); amp >= 0 {
		param = query[amp+1:]
	}

	var value string
	var isSortParam bool
	switch {
	case strings.HasPrefix(param, "h="):
		value = param[len("h="):]
	case strings.HasPrefix(param, "s="):
		value = param[len("s="):]
		isSortParam = true
	default:
		return nil, 0, false
	}

	// Comma-separated columns: only the last one, being typed, should be
	// completed.
	segment := value
	if comma := strings.LastIndexByte(value, ','); comma >= 0 {
		segment = value[comma+1:]
	}

	// s=column:asc|desc — once ':' is typed, we complete the direction, not
	// the column name (already fully typed at this point).
	if isSortParam {
		if colon := strings.IndexByte(segment, ':'); colon >= 0 {
			dirPrefix := segment[colon+1:]
			return matchPrefix(dirPrefix, sortDirections), len([]rune(dirPrefix)), true
		}
	}

	cols := columns[command]
	if len(cols) == 0 {
		return nil, 0, false
	}
	return matchPrefix(segment, cols), len([]rune(segment)), true
}
