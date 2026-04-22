#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

PASS=0
FAIL=0

run() {
    local label="$1"; shift
    printf "%-30s" "$label"
    if output=$("$@" 2>&1); then
        if [ -z "$output" ]; then
            echo "OK"
        else
            echo "WARN"
            printf '%s\n' "$output"
        fi
        PASS=$((PASS + 1))
    else
        echo "FAIL"
        printf '%s\n' "$output"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== Go presubmit checks ==="

# Format check (gofmt exits 0 even when files need reformatting; check output instead)
fmt_output=$(gofmt -l . 2>&1)
printf "%-30s" "gofmt"
if [ -z "$fmt_output" ]; then
    echo "OK"
    PASS=$((PASS + 1))
else
    echo "FAIL (run: gofmt -w ./...)"
    printf '%s\n' "$fmt_output"
    FAIL=$((FAIL + 1))
fi

run "go vet"        go vet ./...

if command -v staticcheck >/dev/null 2>&1; then
    run "staticcheck" staticcheck ./...
fi

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
