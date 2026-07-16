#!/bin/sh
set -eu

profile=${1:-coverage.out}
packages=$(go list ./internal/... | grep -Ev '/internal/(openapi/ogen|store/dbgen)$' | paste -sd, -)

go test -covermode=atomic -coverpkg="$packages" -coverprofile="$profile" ./internal/...
total=$(go tool cover -func="$profile" | awk '/^total:/ {gsub("%", "", $3); print $3}')
awk -v total="$total" 'BEGIN { if (total + 0 < 70.0) { printf "aggregate coverage %.1f%% is below 70.0%%\n", total; exit 1 } }'
printf 'aggregate coverage %s%% meets 70.0%% gate\n' "$total"
