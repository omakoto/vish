#!/usr/bin/env bash
# e2e/lib.sh — Shared test harness for vish e2e tests

# Global counters (shared across sourced files via the runner)
PASS=${PASS:-0}
FAIL=${FAIL:-0}
SKIP=${SKIP:-0}
CURRENT_SUITE=""
SHELL_NAME="${SHELL_NAME:-vish}"

# VISH_CMD: array holding the shell command + any flags (e.g. ("bash" "--posix"))
# Defaults to the vish binary in the repo root.
if [ -z "${VISH_CMD+x}" ]; then
  VISH_CMD=("$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/vish")
fi

begin_suite() {
  CURRENT_SUITE="$1"
  echo ""
  echo "--- Suite: $CURRENT_SUITE ---"
}

end_suite() {
  : # stats are accumulated globally; summary is printed by the runner
}

# _run_shell INPUT — runs the input string with the configured shell
_run_shell() {
  "${VISH_CMD[@]}" -c "$1"
}

# expect_output NAME INPUT EXPECTED_STDOUT [SKIP_FOR_SHELLS...]
# Runs INPUT via the configured shell, merges stderr+stdout, compares to EXPECTED.
# If the current SHELL_NAME matches any SKIP_FOR_SHELLS pattern, the test is skipped.
expect_output() {
  local desc="$1"
  local input="$2"
  local expected="$3"
  shift 3
  # Check if this test should be skipped for the current shell
  local skip_pat
  for skip_pat in "$@"; do
    if [[ "$SHELL_NAME" == *"$skip_pat"* ]]; then
      printf "  [\033[33mSKIP\033[0m] %s  (not applicable for %s)\n" "$desc" "$SHELL_NAME"
      SKIP=$((SKIP+1))
      return
    fi
  done
  local actual

  actual=$(_run_shell "$input" 2>&1) || true

  if [ "$actual" = "$expected" ]; then
    printf "  [\033[32mPASS\033[0m] %s\n" "$desc"
    PASS=$((PASS+1))
  else
    printf "  [\033[31mFAIL\033[0m] %s\n" "$desc"
    printf "    expected: %s\n" "$(printf '%s' "$expected" | head -5 | sed 's/^/|/')"
    printf "    actual:   %s\n" "$(printf '%s' "$actual"   | head -5 | sed 's/^/|/')"
    FAIL=$((FAIL+1))
  fi
}

# expect_status NAME INPUT EXPECTED_STATUS
expect_status() {
  local desc="$1"
  local input="$2"
  local expected_status="$3"
  local actual_status

  _run_shell "$input" >/dev/null 2>&1
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
