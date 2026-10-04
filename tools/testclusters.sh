#!/usr/bin/env bash
# Starts/stops the single-node Elasticsearch and OpenSearch containers the
# reference data is generated from and checked against (security disabled,
# plain HTTP, throwaway — hence published on this machine's loopback
# interface only). Needs Docker and about 12 GB of free memory.
#
#   testclusters.sh up      start every cluster and wait until it answers
#   testclusters.sh down    remove every cluster
#   testclusters.sh env     print the list as a TDT_IT_TARGETS value
#
# The list below is mirrored in internal/testclusters (a test keeps the two
# in step): for each distribution, the first and last minor of every major
# the reference data covers, plus one in the middle of the longest lines.
# Under WSL, keep a "wsl.exe -- sleep" running meanwhile: the VM, and the
# containers with it, stops once no wsl.exe process is left.
set -u

# name|image|host port
targets=(
  "es-7.17.29|docker.elastic.co/elasticsearch/elasticsearch:7.17.29|19217"
  "es-8.0.1|docker.elastic.co/elasticsearch/elasticsearch:8.0.1|19280"
  "es-8.11.4|docker.elastic.co/elasticsearch/elasticsearch:8.11.4|19281"
  "es-8.19.22|docker.elastic.co/elasticsearch/elasticsearch:8.19.22|19289"
  "es-9.0.8|docker.elastic.co/elasticsearch/elasticsearch:9.0.8|19290"
  "es-9.5.4|docker.elastic.co/elasticsearch/elasticsearch:9.5.4|19295"
  "os-2.0.1|opensearchproject/opensearch:2.0.1|19320"
  "os-2.11.1|opensearchproject/opensearch:2.11.1|19321"
  "os-2.19.6|opensearchproject/opensearch:2.19.6|19329"
  "os-3.0.0|opensearchproject/opensearch:3.0.0|19330"
  "os-3.9.0|opensearchproject/opensearch:3.9.0|19339"
)

heap="-Xms768m -Xmx768m"
# Where a snapshot repository may be registered inside each container (the
# snapshot recipes are run for real by the integration tests).
snapshots="/tmp/tdt-snapshots"

up() {
  for t in "${targets[@]}"; do
    IFS='|' read -r name image port <<<"$t"
    if docker ps --format '{{.Names}}' | grep -qx "tdt-$name"; then
      continue
    fi
    docker rm -f "tdt-$name" >/dev/null 2>&1
    case "$name" in
      es-*)
        docker run -d --quiet --name "tdt-$name" -p "127.0.0.1:$port:9200" \
          -e discovery.type=single-node \
          -e xpack.security.enabled=false \
          -e path.repo="$snapshots" \
          -e ES_JAVA_OPTS="$heap" \
          "$image" >/dev/null
        ;;
      os-*)
        docker run -d --quiet --name "tdt-$name" -p "127.0.0.1:$port:9200" \
          -e discovery.type=single-node \
          -e path.repo="$snapshots" \
          -e DISABLE_SECURITY_PLUGIN=true \
          -e DISABLE_INSTALL_DEMO_CONFIG=true \
          -e OPENSEARCH_JAVA_OPTS="$heap" \
          "$image" >/dev/null
        ;;
    esac
    echo "started tdt-$name on :$port"
  done

  failed=0
  for t in "${targets[@]}"; do
    IFS='|' read -r name image port <<<"$t"
    ok=0
    for _ in $(seq 1 90); do
      if curl -s -m 2 "http://localhost:$port/_cluster/health" | grep -q '"status"'; then
        ok=1
        break
      fi
      sleep 2
    done
    if [ "$ok" = 1 ]; then
      echo "ready   tdt-$name  $(curl -s -m 2 "http://localhost:$port/" | tr -d '\n ' | grep -o '"number":"[^"]*"')"
    else
      echo "FAILED  tdt-$name"
      docker logs --tail 15 "tdt-$name" 2>&1 | sed 's/^/    /'
      failed=1
    fi
  done
  # So that "testclusters.sh up && go test ..." stops here.
  return "$failed"
}

down() {
  for t in "${targets[@]}"; do
    IFS='|' read -r name image port <<<"$t"
    docker rm -f "tdt-$name" >/dev/null 2>&1 && echo "removed tdt-$name"
  done
}

env_line() {
  out=""
  for t in "${targets[@]}"; do
    IFS='|' read -r name image port <<<"$t"
    out="$out${out:+,}$name=http://localhost:$port"
  done
  echo "$out"
}

case "${1:-}" in
  up) up ;;
  down) down ;;
  env) env_line ;;
  *) echo "usage: $0 up|down|env" >&2; exit 2 ;;
esac
