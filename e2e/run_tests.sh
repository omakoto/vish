#!/usr/bin/env bash
# e2e/run_tests.sh — Top-level runner for vish end-to-end test suite
#
# Usage:
#   ./e2e/run_tests.sh               # run all tests against vish and bash --posix
#   ./e2e/run_tests.sh test_io.sh    # run a specific suite file
#   VISH=some_shell ./e2e/run_tests.sh  # run against a specific shell only

set -u

E2E_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$E2E_DIR/.." && pwd)"

# ── Build ──────────────────────────────────────────────────────────────────────
echo "Building vish..."
cd "$ROOT_DIR"
if ! go build -o vish . ; then
  echo "BUILD FAILED" >&2
  exit 2
fi
VISH_BIN="$ROOT_DIR/vish"
echo "Build OK ($VISH_BIN)"

# ── Suite file list ────────────────────────────────────────────────────────────
SUITES=()
if [ $# -gt 0 ]; then
  for arg in "$@"; do
    SUITES+=("$E2E_DIR/$arg")
  done
else
  for f in "$E2E_DIR"/test_*.sh; do
    SUITES+=("$f")
  done
fi

# ── Run all tests for a given shell ───────────────────────────────────────────
run_all_for_shell() {
  local shell_name="$1"
  shift
  # Remaining args are the shell command + flags (passed as separate words)
  local shell_cmd=("$@")

  export VISH_CMD=("${shell_cmd[@]}")
  export SHELL_NAME="$shell_name"
  export PASS=0
  export FAIL=0
  export SKIP=0

  echo ""
  echo "╔══════════════════════════════════════════════════════════════╗"
  printf  "║  Shell: %-54s║\n" "$shell_name"
  echo "╚══════════════════════════════════════════════════════════════╝"

  source "$E2E_DIR/lib.sh"

  # ── Basic inline suite (original smoke tests) ──────────────────────────────
  begin_suite "Basic Smoke Tests"

  expect_output "echo"                 "echo hello"                               "hello"
  expect_output "variable"             "x=world; echo hello \$x"                  "hello world"
  expect_output "arithmetic"           "echo \$((2+3))"                           "5"
  expect_output "arith-complex"        "x=10; echo \$((x*2+5))"                   "25"
  expect_output "arith-nested"         "echo \$((1 + 2 * 3))"                     "7"
  expect_output "if-true"              "if true; then echo yes; fi"               "yes"
  expect_output "if-false-else"        "if false; then echo yes; else echo no; fi" "no"
  expect_output "while"                "i=0; while [ \$i -lt 3 ]; do echo \$i; i=\$((i+1)); done" "$(printf '0\n1\n2')"
  expect_output "until"                "i=0; until [ \$i -ge 3 ]; do echo \$i; i=\$((i+1)); done" "$(printf '0\n1\n2')"
  expect_output "for"                  "for x in a b c; do echo \$x; done"        "$(printf 'a\nb\nc')"
  expect_output "function"             "greet() { echo hello \$1; }; greet world" "hello world"
  expect_output "nested-func"          "a() { echo a-\$1; }; b() { a inner; }; b" "a-inner"
  expect_output "func-return"          "f() { return 42; }; f; echo \$?"          "42"
  expect_output "pipeline"             "echo hello world | tr ' ' '\n' | head -1"  "hello"
  expect_output "and-true"             "true && echo yes"                          "yes"
  expect_output "and-false"            "false && echo yes || echo no"              "no"
  expect_output "or"                   "false || echo fallback"                    "fallback"
  expect_output "cmdsub"               "x=\$(echo hello); echo \$x"               "hello"
  expect_output "squote"               "echo 'hello \$world'"                      "hello \$world"
  expect_output "dquote"               "x=world; echo \"hello \$x\""              "hello world"
  expect_output "case"                 "x=b; case \$x in a) echo A;; b) echo B;; esac" "B"
  expect_output "case-wildcard"        "x=hello; case \$x in hel*) echo yes;; esac" "yes"
  expect_output "subshell"             "(echo inside)"                             "inside"
  expect_output "brace"                "{ echo a; echo b; }"                       "$(printf 'a\nb')"
  expect_output "test-eq"              "[ 1 -eq 1 ] && echo yes"                  "yes"
  expect_output "test-ne"             "[ 1 -ne 2 ] && echo yes"                  "yes"
  expect_output "test-string-eq"       '[ "hello" = "hello" ] && echo yes'        "yes"
  expect_output "test-z"               '[ -z "" ] && echo yes'                    "yes"
  expect_output "test-n"               '[ -n "x" ] && echo yes'                   "yes"
  expect_output "break"                "for x in 1 2 3; do [ \$x -eq 2 ] && break; echo \$x; done" "1"
  expect_output "continue"             "for x in 1 2 3; do [ \$x -eq 2 ] && continue; echo \$x; done" "$(printf '1\n3')"
  expect_output "negation"             "! false && echo yes"                       "yes"
  expect_output "export"               "export E2E_FOO=bar; env | grep ^E2E_FOO=" "E2E_FOO=bar"
  expect_output "local-var"            "x=global; f() { local x=local; echo \$x; }; f; echo \$x" "$(printf 'local\nglobal')"
  expect_output "multi-assign"         "a=1 b=2; echo \$a \$b"                    "1 2"
  expect_output "param-default"        "x=; echo \${x:-default}"                  "default"
  expect_output "param-isset"          "x=val; echo \${x:-default}"               "val"
  expect_output "param-length"         "x=hello; echo \${#x}"                     "5"
  expect_output "param-prefix"         "x=hello; echo \${x#hel}"                  "lo"
  expect_output "param-prefix-glob"    "x=hello; echo \${x#h*l}"                  "lo"
  expect_output "param-suffix"         "x=hello; echo \${x%lo}"                   "hel"
  expect_output "heredoc"              "$(printf 'cat <<EOF\nhello\nworld\nEOF')"  "$(printf 'hello\nworld')"
  expect_output "heredoc-var"          "name=World; cat <<EOF
Hello, \$name!
EOF" "Hello, World!"
  # set -e in subshell: bash --posix does not propagate set -e through ||
  expect_output "set-e-subshell" \
    "(set -e; false; echo skip) || echo caught" \
    "caught" \
    "bash"  # bash --posix: set -e inside subshell doesn't exit via ||

  # ── External suite files ────────────────────────────────────────────────────
  for suite in "${SUITES[@]}"; do
    if [ -f "$suite" ]; then
      source "$suite"
    else
      echo "WARNING: suite not found: $suite" >&2
    fi
  done

  # ── Per-shell summary ────────────────────────────────────────────────────────
  local total=$((PASS + FAIL + SKIP))
  echo ""
  echo "  ── $shell_name results ──────────────────────────────────────────"
  if [ "$FAIL" -eq 0 ]; then
    if [ "$SKIP" -gt 0 ]; then
      printf "  \033[32m✓ %d/%d passed\033[0m  (\033[33m%d skipped\033[0m)\n" "$PASS" "$total" "$SKIP"
    else
      printf "  \033[32m✓ All %d tests passed\033[0m\n" "$total"
    fi
  else
    printf "  \033[31m✗ %d/%d passed  (%d FAILED, %d skipped)\033[0m\n" "$PASS" "$total" "$FAIL" "$SKIP"
  fi
  echo ""

  # Return the failure count
  return "$FAIL"
}

# ── Run for each shell ─────────────────────────────────────────────────────────
OVERALL_FAIL=0

run_all_for_shell "vish" "$VISH_BIN"
OVERALL_FAIL=$((OVERALL_FAIL + $?))

run_all_for_shell "bash --posix" bash --posix
OVERALL_FAIL=$((OVERALL_FAIL + $?))

# ── Final summary ──────────────────────────────────────────────────────────────
echo "══════════════════════════════════════════════════════════════"
if [ "$OVERALL_FAIL" -eq 0 ]; then
  printf "\033[32m✓ All shells passed all tests\033[0m\n"
else
  printf "\033[31m✗ %d failure(s) across all shells\033[0m\n" "$OVERALL_FAIL"
fi
echo "══════════════════════════════════════════════════════════════"

[ "$OVERALL_FAIL" -eq 0 ]
