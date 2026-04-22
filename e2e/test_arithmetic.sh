#!/usr/bin/env bash
# e2e/test_arithmetic.sh — Arithmetic expansion tests

source "$(dirname "$0")/lib.sh"

begin_suite "Arithmetic"

# --- Basic operators ---

expect_output "add"        'echo $((3+4))'       "7"
expect_output "subtract"   'echo $((10-3))'      "7"
expect_output "multiply"   'echo $((3*4))'       "12"
expect_output "divide"     'echo $((10/3))'      "3"
expect_output "modulo"     'echo $((10%3))'      "1"

# --- Operator precedence ---

expect_output "precedence-mul-before-add"   'echo $((2+3*4))'    "14"
expect_output "precedence-parens"           'echo $(((2+3)*4))'  "20"
expect_output "precedence-div-before-sub"   'echo $((10-6/2))'   "7"

# --- Unary operators ---

expect_output "unary-minus"    'echo $((-5))'          "-5"
expect_output "unary-plus"     'echo $((+5))'          "5"
expect_output "logical-not"    'echo $((! 0))'         "1"
expect_output "logical-not-2"  'echo $((! 1))'         "0"
expect_output "bitwise-not"    'echo $((~0))'          "-1"

# --- Comparison operators ---

expect_output "cmp-lt-true"    'echo $((3 < 4))'       "1"
expect_output "cmp-lt-false"   'echo $((4 < 3))'       "0"
expect_output "cmp-gt-true"    'echo $((4 > 3))'       "1"
expect_output "cmp-eq-true"    'echo $((3 == 3))'      "1"
expect_output "cmp-ne-true"    'echo $((3 != 4))'      "1"
expect_output "cmp-le-true"    'echo $((3 <= 3))'      "1"
expect_output "cmp-ge-true"    'echo $((4 >= 3))'      "1"

# --- Logical operators ---

expect_output "logical-and-tt"   'echo $((1 && 1))'    "1"
expect_output "logical-and-tf"   'echo $((1 && 0))'    "0"
expect_output "logical-or-ff"    'echo $((0 || 0))'    "0"
expect_output "logical-or-tf"    'echo $((1 || 0))'    "1"

# --- Bitwise operators ---

expect_output "bitwise-and"   'echo $((6 & 3))'   "2"
expect_output "bitwise-or"    'echo $((6 | 3))'   "7"
expect_output "bitwise-xor"   'echo $((6 ^ 3))'   "5"
expect_output "shift-left"    'echo $((1 << 3))'  "8"
expect_output "shift-right"   'echo $((8 >> 2))'  "2"

# --- Ternary operator ---

expect_output "ternary-true"    'echo $((1 ? 42 : 99))'   "42"
expect_output "ternary-false"   'echo $((0 ? 42 : 99))'   "99"

# --- Variables in arithmetic ---

expect_output "var-in-arith"      'x=5; echo $((x+3))'             "8"
expect_output "var-expr"          'x=3; y=4; echo $((x*x + y*y))'  "25"
expect_output "var-increment"     'x=0; x=$((x+1)); echo $x'       "1"

# --- Hex and octal constants ---

expect_output "hex-constant"    'echo $((0xFF))'    "255"
expect_output "octal-constant"  'echo $((010))'     "8"

# --- Nested arithmetic ---

expect_output "nested-arith"  'echo $(($(echo 3) + $(echo 4)))'  "7"

# --- Arithmetic assignment operators (POSIX required) ---

expect_output "arith-assign"          'x=0; echo $((x = 5)); echo $x'     "5
5"
expect_output "arith-assign-plus"     'x=10; echo $((x += 3)); echo $x'   "13
13"
expect_output "arith-assign-minus"    'x=10; echo $((x -= 3)); echo $x'   "7
7"
expect_output "arith-assign-mul"      'x=4; echo $((x *= 3)); echo $x'    "12
12"
expect_output "arith-assign-div"      'x=12; echo $((x /= 3)); echo $x'   "4
4"
expect_output "arith-assign-mod"      'x=10; echo $((x %= 3)); echo $x'   "1
1"

end_suite
