#!/usr/bin/env bash
# e2e/test_control_flow.sh — Complex control flow tests

source "$(dirname "$0")/lib.sh"

begin_suite "Control Flow"

# --- Nested if/elif/else ---

expect_output "elif" \
  'x=2
   if [ $x -eq 1 ]; then echo one
   elif [ $x -eq 2 ]; then echo two
   elif [ $x -eq 3 ]; then echo three
   else echo other
   fi' \
  "two"

expect_output "nested-if" \
  'x=1; y=2
   if [ $x -eq 1 ]; then
     if [ $y -eq 2 ]; then echo "1 and 2"; fi
   fi' \
  "1 and 2"

# --- Nested loops ---

expect_output "nested-for" \
  'for i in 1 2; do
     for j in a b; do
       echo ${i}${j}
     done
   done' \
  "1a
1b
2a
2b"

expect_output "nested-while" \
  'i=1
   while [ $i -le 2 ]; do
     j=1
     while [ $j -le 2 ]; do
       echo ${i}x${j}
       j=$((j+1))
     done
     i=$((i+1))
   done' \
  "1x1
1x2
2x1
2x2"

# --- Break and continue in nested loops ---

expect_output "break-inner-loop" \
  'for i in 1 2 3; do
     for j in a b c; do
       [ "$j" = "b" ] && break
       echo ${i}${j}
     done
   done' \
  "1a
2a
3a"

expect_output "break-2" \
  'for i in 1 2 3; do
     for j in a b c; do
       [ "$j" = "b" ] && break 2
       echo ${i}${j}
     done
   done' \
  "1a"

expect_output "continue-inner" \
  'for i in 1 2; do
     for j in a b c; do
       [ "$j" = "b" ] && continue
       echo ${i}${j}
     done
   done' \
  "1a
1c
2a
2c"

# --- Case statement ---

expect_output "case-multiple-patterns" \
  'for x in cat dog bird; do
     case $x in
       cat|dog) echo "mammal: $x";;
       bird)    echo "avian: $x";;
       *)       echo "unknown: $x";;
     esac
   done' \
  "mammal: cat
mammal: dog
avian: bird"

expect_output "case-glob-patterns" \
  'for f in foo.c foo.h bar.sh baz; do
     case $f in
       *.c|*.h) echo "C source: $f";;
       *.sh)    echo "shell:    $f";;
       *)       echo "other:    $f";;
     esac
   done' \
  "C source: foo.c
C source: foo.h
shell:    bar.sh
other:    baz"

# --- For without 'in' (iterates $@) ---

expect_output "for-positionals" \
  'set -- alpha beta gamma
   for x; do echo $x; done' \
  "alpha
beta
gamma"

# --- Subshell isolation ---

expect_output "subshell-no-leak" \
  'x=outer
   (x=inner; echo $x)
   echo $x' \
  "inner
outer"

expect_output "subshell-exit-code" \
  '(exit 42); echo $?' \
  "42"

expect_output "subshell-pipeline" \
  '(echo a; echo b) | tr a-z A-Z' \
  "A
B"

# --- Brace group ---

expect_output "brace-shared-env" \
  'x=0; { x=1; echo $x; }; echo $x' \
  "1
1"

expect_output "brace-redirect" \
  '{ echo hello; echo world; } | cat' \
  "hello
world"

# --- And-or lists ---

expect_output "and-or-chain" \
  'true && true && echo "all good"' \
  "all good"

expect_output "and-or-short-circuit" \
  'false && echo skip || echo "reached"' \
  "reached"

expect_output "complex-and-or" \
  'x=5
   [ $x -gt 3 ] && [ $x -lt 10 ] && echo "in range"' \
  "in range"

# --- Pipelines determinism ---

expect_output "pipeline-false-true" 'false | true; echo $?' "0"
expect_output "pipeline-true-false" 'true | false; echo $?' "1"

# --- Return from nested context ---

expect_output "return-from-loop" \
  'f() {
     for x in 1 2 3; do
       [ $x -eq 2 ] && return 99
     done
     return 0
   }
   f; echo $?' \
  "99"

# POSIX: set -e (errexit) must NOT trigger when a command is used as the condition
# of an if, elif, while, or until statement (even if it returns non-zero).
expect_output "set-e-if-condition-exempt" \
  'set -e; if false; then echo yes; fi; echo done' \
  "done"

expect_output "set-e-while-condition-exempt" \
  'set -e; i=3; while [ $i -gt 0 ]; do i=$((i-1)); done; echo done' \
  "done"

end_suite
