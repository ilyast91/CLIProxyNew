#!/bin/sh
set -eu

fail() {
	printf '%s\n' "$1" >&2
	exit 1
}

tracked=$(git ls-files)

printf '%s\n' "$tracked" | grep -E '(^|/)(\.env|[^/]+\.(pem|key|p12|pfx))$' && fail 'tracked secret/key artifact detected'
printf '%s\n' "$tracked" | grep -E '(^|/)cliproxy$' && fail 'tracked binary artifact detected'

if git grep -nE -- '-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----' -- . ':!scripts/security-audit.sh'; then
	fail 'private key material detected'
fi

if git grep -nE '\b(fmt|log)\.Print(f|ln)?\(' -- ':(glob)cmd/**/*.go' ':(glob)internal/**/*.go' ':(exclude,glob)**/*_test.go'; then
	fail 'unstructured runtime printing detected'
fi

if git grep -nE 'slog\.(Debug|Info|Warn|Error)\([^\n]*(os\.Getenv|StaticPassword|BindPassword|Credentials|RefreshToken|AccessToken|APIKey)' -- ':(glob)cmd/**/*.go' ':(glob)internal/**/*.go' ':(exclude,glob)**/*_test.go'; then
	fail 'potential sensitive slog payload detected'
fi

printf 'security source audit passed\n'
