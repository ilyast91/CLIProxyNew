#!/bin/sh
set -eu

fail() {
	printf '%s\n' "$1" >&2
	exit 1
}

fail_on_git_grep() {
	pattern=$1
	message=$2
	shift 2
	if matches=$(git grep -nE -e "$pattern" -- "$@"); then
		printf '%s\n' "$matches" >&2
		fail "$message"
	else
		status=$?
		if [ "$status" -ne 1 ]; then
			fail "security audit git grep failed with status $status"
		fi
	fi
}

tracked=$(git ls-files)

if artifacts=$(printf '%s\n' "$tracked" | grep -E '(^|/)\.env($|\.)' | grep -Ev '(^|/)\.env(\.[^/]*)?\.example$'); then
	printf '%s\n' "$artifacts" >&2
	fail 'tracked environment secret artifact detected'
fi

if artifacts=$(printf '%s\n' "$tracked" | grep -E '\.(pem|key|p12|pfx)$' | grep -Ev '(^|/)[^/]*\.example\.(pem|key|p12|pfx)$'); then
	printf '%s\n' "$artifacts" >&2
	fail 'tracked key artifact detected'
fi

if artifacts=$(printf '%s\n' "$tracked" | grep -E '\.(exe|dll|dylib|so(\.[0-9]+)*|a|o|bin|class|jar|war)$'); then
	printf '%s\n' "$artifacts" >&2
	fail 'tracked binary artifact detected'
fi

if binary_candidates=$(git grep -IL '' -- .); then
	binary_artifacts=$(printf '%s\n' "$binary_candidates" | while IFS= read -r path; do
		if [ -n "$path" ] && [ -s "$path" ]; then
			printf '%s\n' "$path"
		fi
	done)
	if [ -n "$binary_artifacts" ]; then
		printf '%s\n' "$binary_artifacts" >&2
		fail 'tracked binary content detected'
	fi
else
	status=$?
	if [ "$status" -ne 1 ]; then
		fail "security audit binary scan failed with status $status"
	fi
fi

fail_on_git_grep '-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----' 'private key material detected' . ':!scripts/security-audit.sh'
fail_on_git_grep '(^|[^[:alnum:]_])(fmt|log)\.Print(f|ln)?\(' 'unstructured runtime printing detected' ':(glob)cmd/**/*.go' ':(glob)internal/**/*.go' ':(exclude,glob)**/*_test.go'
fail_on_git_grep 'slog\.(Debug|Info|Warn|Error)\(.*(os\.Getenv|StaticPassword|BindPassword|Credentials|RefreshToken|AccessToken|APIKey)' 'potential sensitive slog payload detected' ':(glob)cmd/**/*.go' ':(glob)internal/**/*.go' ':(exclude,glob)**/*_test.go'

printf 'security source audit passed\n'
