#!/usr/bin/env bash
# e2e/test_functions.sh — Complex function tests

source "$(dirname "$0")/lib.sh"

begin_suite "Functions"

# --- Recursion ---

expect_output "fibonacci" \
  'fib() { [ $1 -le 1 ] && echo $1 && return; a=$(fib $((1-1))); b=$(fib $((1-0))); echo $((a+b)); }
   fib() {
     n=$1
     if [ $n -le 1 ]; then echo $n; return; fi
     a=$(fib $((n-1)))
     b=$(fib $((n-2)))
     echo $((a+b))
   }
   fib 7' \
  "13"

expect_output "factorial" \
  'fact() { if [ $1 -le 1 ]; then echo 1; return; fi; echo $(( $1 * $(fact $(($1-1))) )); }
   fact 6' \
  "720"

expect_output "mutual-recursion" \
  'is_even() { [ $1 -eq 0 ] && echo yes && return; is_odd $(($1-1)); }
   is_odd()  { [ $1 -eq 0 ] && echo no  && return; is_even $(($1-1)); }
   is_even 4' \
  "yes"

# --- Return values ---

expect_output "return-value-propagated" \
  'f() { return 7; }; f; echo $?' \
  "7"

expect_output "nested-return" \
  'inner() { return 3; }
   outer() { inner; return $?; }
   outer; echo $?' \
  "3"

expect_output "function-sets-global" \
  'x=0; set_x() { x=42; }; set_x; echo $x' \
  "42"

# --- Argument handling ---

expect_output "args-count" \
  'f() { echo $#; }; f a b c' \
  "3"

expect_output "all-args-dollar-at" \
  'f() { for a in "$@"; do echo "[$a]"; done; }; f one two three' \
  "[one]
[two]
[three]"

expect_output "all-args-star" \
  'f() { echo $*; }; f one two three' \
  "one two three"

expect_output "shift" \
  'f() { shift; echo $1; }; f first second third' \
  "second"

expect_output "local-does-not-leak" \
  'x=outer
   f() { local x=inner; echo $x; }
   f
   echo $x' \
  "inner
outer"

# --- Closures / env manipulation ---

expect_output "function-sees-caller-env" \
  'x=hello; f() { echo $x; }; f' \
  "hello"

expect_output "function-modifies-local-not-caller" \
  'x=before
   f() { local x=after; echo $x; }
   f; echo $x' \
  "after
before"

end_suite
