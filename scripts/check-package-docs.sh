#!/bin/sh
set -eu

missing=$(go list -f '{{if not .Doc}}{{.ImportPath}}{{end}}' ./...)
if [ -n "$missing" ]; then
	printf 'packages without godoc comments:\n%s\n' "$missing" >&2
	exit 1
fi

printf 'package godoc check passed\n'
