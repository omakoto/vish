#!/usr/bin/env bash
# e2e/test_posix_compliance.sh — POSIX compliance regression tests

source "$(dirname "$0")/lib.sh"

begin_suite "POSIX Compliance"

# Bug 1: IFS non-whitespace characters must create empty fields between consecutive separators.
# POSIX: each non-whitespace IFS char always delimits a field, even creating empty ones.
expect_output "ifs-nonwhite-consecutive-empty-field" \
  'IFS=: x="a::b"; for w in $x; do printf "[%s]\n" "$w"; done' \
  "[a]
[]
[b]"

# Bug 2: read without -r must consume (remove) backslash before non-newline characters.
# POSIX: without -r, backslash escapes the next character — backslash is discarded.
# Use subshell grouping so both read and echo share the same environment.
expect_output "read-no-r-backslash-consumed" \
  'printf "%s\n" "a\\b" | (read x; echo "$x")' \
  "ab"

# Bug 3: ${#var} must return the number of characters, not bytes.
# POSIX: "The length in characters of the value of parameter."
expect_output "param-length-unicode-chars" \
  'x="café"; echo ${#x}' \
  "4"

# Bug 4: case patterns must use shell glob semantics where * matches across /.
# POSIX: pattern matching in case uses shell patterns, not filename patterns.
expect_output "case-pattern-star-matches-slash" \
  'case "/usr/bin" in /*) echo yes;; *) echo no;; esac' \
  "yes"

# Also test non-trivial path matching across multiple slashes
expect_output "case-pattern-star-matches-multiple-slashes" \
  'case "/usr/local/bin" in */bin) echo yes;; *) echo no;; esac' \
  "yes"

# Bug 5: unset on a readonly variable must fail and leave the variable intact.
# POSIX: "If the readonly attribute is set for a variable ... it shall not be unset."
expect_output "unset-readonly-preserves-value" \
  'readonly x=5; unset x 2>/dev/null; echo ${x:-gone}' \
  "5"

expect_status "unset-readonly-returns-nonzero" \
  'readonly x=5; unset x' \
  1

end_suite
