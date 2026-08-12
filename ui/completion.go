package ui

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// knownEndpoints est la liste par défaut (compilée dans le binaire) des
// endpoints Elasticsearch les plus courants en administration/exploitation,
// proposée par l'auto-complétion Tab (SPEC.md §3.2). Utilisée seulement en
// l'absence du fichier endpoints.txt à côté du binaire (LoadEndpointsFile),
// que l'équipe peut maintenir sans recompiler à chaque nouvelle version
// d'Elasticsearch.
//
// Extraite de la spec OpenAPI officielle (elastic/elasticsearch-specification,
// branche 9.5, output/openapi/elasticsearch-openapi.json), filtrée aux
// endpoints sans paramètre de chemin (les `/{index}/...` nécessitent un nom
// d'index réel — pas de découverte dynamique des index, cf. SPEC.md §7) et
// aux domaines "administration de base" : `_cat` (toutes les commandes,
// `?v` systématique pour les en-têtes de colonnes), `_cluster`, `_nodes`,
// index/recherche/snapshot/ILM/SLM/licence. Volontairement écartés : ML,
// sécurité, watcher, transform, rollup, SQL/ES|QL, CCR, connectors,
// inference, enrich — hors du périmètre admin de base de cet outil.
var knownEndpoints = []string{
	// _cat/* — verbose (?v) systématique pour les en-têtes de colonnes.
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

	// Index, document, recherche, snapshot, ILM/SLM, licence.
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

// catColumns est la table par défaut (compilée dans le binaire) des
// colonnes disponibles pour h= (colonnes affichées) et s= (tri) de chaque
// commande _cat/*, proposée par l'auto-complétion Tab (SPEC.md §3.2).
// Utilisée seulement en l'absence du fichier cat_columns.txt à côté du
// binaire (LoadCatColumnsFile).
//
// Seuls les noms complets de colonne sont listés (pas les alias courts
// comme "dc" pour "docs.count") : plus parlants dans la liste de
// suggestions, et ça limite le nombre de propositions pour les commandes
// qui ont beaucoup de colonnes (ex. _cat/indices, _cat/nodes).
//
// Générée le 2026-08-12 à partir d'un cluster Elasticsearch 9.5.0 réel
// (GET _cat/<commande>?help pour chacune des commandes de knownEndpoints) —
// voir SPEC.md §3.2 pour la méthode.
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

// matchPrefix renvoie, parmi candidates, ceux commençant par prefix
// (insensible à la casse), triés. Un prefix vide renvoie la liste complète.
// Utilisée aussi bien pour les endpoints que pour les colonnes _cat (h=/s=)
// et les directions de tri (asc/desc).
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

// LoadEndpointsFile lit un fichier d'endpoints — un par ligne, lignes
// vides et commentaires ('#') ignorés — pour permettre à l'équipe de
// maintenir la liste proposée par l'auto-complétion sans recompiler
// (SPEC.md §3.2, §9.1). Renvoie (nil, nil) si le fichier n'existe pas :
// l'appelant se rabat alors sur knownEndpoints.
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

// catColumnSectionRe reconnaît une ligne d'en-tête de section dans un
// fichier de colonnes _cat (ex. "# _cat/indices") — voir LoadCatColumnsFile.
var catColumnSectionRe = regexp.MustCompile(`^#\s*_cat/(\S+)\s*$`)

// LoadCatColumnsFile lit un fichier de colonnes _cat organisé en sections
// "# _cat/commande" suivies d'une colonne (nom complet ou alias) par ligne,
// pour permettre à l'équipe de maintenir la liste proposée par
// l'auto-complétion h=/s= sans recompiler (SPEC.md §3.2, §9.1). Les autres
// lignes commençant par '#' sont de simples commentaires. Renvoie (nil,
// nil) si le fichier n'existe pas : l'appelant se rabat alors sur
// catColumns.
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

// sortDirections sont les valeurs possibles pour la direction de tri d'une
// colonne dans le paramètre s= (ex. "s=docs.count:desc").
var sortDirections = []string{"asc", "desc"}

// matchCatCommand cherche, parmi les clés de columns, la commande _cat la
// plus longue (la plus "couvrante") qui préfixe path — soit une
// correspondance exacte, soit suivie d'un '/' (jamais une correspondance
// partielle de mot : "shardsxyz" ne doit pas matcher "shards"). Nécessaire
// car de nombreuses commandes _cat acceptent un filtre en fin de chemin
// avant les paramètres, ex. "_cat/shards/monindex?h=..." (filtre sur
// l'index "monindex") — sans ça, "shards/monindex" ne correspondrait à
// aucune commande connue et aucune colonne ne serait proposée.
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

// catColumnCompletion tente d'interpréter prefix (le texte tapé après la
// méthode HTTP, cf. Editor.CompletionPrefix) comme une frappe en cours dans
// le paramètre h= (colonnes affichées) ou s= (tri) d'une commande _cat/*,
// ex. "_cat/indices?h=health,st" ou "_cat/shards?s=docs.count:de". Renvoie
// ok=false si prefix ne correspond pas à ce cas (endpoint classique, pas
// une commande _cat, ou pas dans un paramètre h=/s=) — l'appelant se rabat
// alors sur la complétion d'endpoint habituelle. subPrefixLen (en runes)
// permet de calculer la portion à remplacer : seule la colonne (ou la
// direction) en cours de frappe, pas tout ce qui précède.
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

	// De nombreuses commandes _cat acceptent un filtre en fin de chemin
	// avant les paramètres (ex. "_cat/shards/monindex?h=..." filtre sur
	// l'index "monindex"). On reconnaît donc la commande _cat la plus
	// longue (la plus "couvrante") qui préfixe path à une frontière de
	// '/' — pas une correspondance partielle de mot (ex. "shardsxyz" ne
	// doit pas matcher "shards").
	command, ok := matchCatCommand(path, columns)
	if !ok {
		return nil, 0, false
	}

	// Le paramètre en cours de frappe : ce qui suit le dernier '&' (ou
	// toute la query string s'il n'y en a pas).
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

	// Colonnes séparées par des virgules : seule la dernière, en cours de
	// frappe, doit être complétée.
	segment := value
	if comma := strings.LastIndexByte(value, ','); comma >= 0 {
		segment = value[comma+1:]
	}

	// s=colonne:asc|desc — une fois le ':' tapé, on complète la direction,
	// pas le nom de colonne (déjà entièrement tapé à ce stade).
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
