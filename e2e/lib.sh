#!/usr/bin/env bash
# e2e/lib.sh — Shared test harness for vish e2e tests

# Global counters (shared across sourced files via the runner)
PASS=${PASS:-0}
FAIL=${FAIL:-0}
CURRENT_SUITE=""

# VISH binary: default to the one built in the repo root
VISH="${VISH:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/vish}"

begin_suite() {
  CURRENT_SUITE="$1"
  echo ""
  echo "--- Suite: $CURRENT_SUITE ---"
}

end_suite() {
  : # stats are accumulated globally; summary is printed by the runner
}

# expect_output NAME INPUT EXPECTED_STDOUT
# Runs INPUT via `vish -c`, merges stderr into stdout, compares to EXPECTED.
expect_output() {
  local desc="$1"
  local input="$2"
  local expected="$3"
  local actual

  actual=$("$VISH" -c "$input" 2>&1) || true

  if [ "$actual" = "$expected" ]; then
    printf "  [\033[32mPASS\033[0m] %s\n" "$desc"
    PASS=$((PASS+1))
  else
    printf "  [\033[31mFAIL\033[0m] %s\n" "$desc"
    # Show diff-style output
    printf "    expected: %s\n" "$(echo "$expected" | head -5 | sed 's/^/|/')"
    printf "    actual:   %s\n" "$(echo "$actual"   | head -5 | sed 's/^/|/')"
    FAIL=$((FAIL+1))
  fi
}

# expect_status NAME INPUT EXPECTED_STATUS
expect_status() {
  local desc="$1"
  local input="$2"
  local expected_status="$3"
  local actual_status

  "$VISH" -c "$input" >/dev/null 2>&1
  actual_status=$?

  if [ "$actual_status" -eq "$expected_status" ]; then
    printf "  [\033[32mPASS\033[0m] %s\n" "$desc"
    PASS=$((PASS+1))
  else
    printf "  [\033[31mFAIL\033[0m] %s\n" "$desc"
    printf "    expected exit: %d, got: %d\n" "$expected_status" "$actual_status"
    FAIL=$((FAIL+1))
  fi
}
