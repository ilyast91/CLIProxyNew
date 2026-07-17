#!/bin/sh
set -eu

require_test() {
	package=$1
	name=$2
	if ! go test -list "^${name}$" "$package" | grep -qx "$name"; then
		printf 'required chaos test %s is missing in %s\n' "$name" "$package" >&2
		exit 1
	fi
}

require_test ./internal/watcher TestIntegrationAdvisoryLeaderFailover
require_test ./internal/e2e TestIntegrationRuntimeReplicaFailover

go test -count=1 -run '^TestIntegrationAdvisoryLeaderFailover$' -timeout 5m ./internal/watcher
go test -count=1 -run '^TestIntegrationRuntimeReplicaFailover$' -timeout 10m ./internal/e2e
