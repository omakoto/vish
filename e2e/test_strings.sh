#!/usr/bin/env bash
# e2e/test_strings.sh — String / parameter expansion tests

source "$(dirname "$0")/lib.sh"

begin_suite "String & Parameter Expansion"

# --- Basic expansion ---

expect_output "simple-var"           'x=hello; echo $x'           "hello"
expect_output "curly-var"            'x=hello; echo ${x}'         "hello"
expect_output "undefined-is-empty"   'echo ${UNDEFINED_VAR}'      ""

# --- Length ---

expect_output "length-string"  'x=hello;      echo ${#x}'  "5"
expect_output "length-empty"   'x=;           echo ${#x}'  "0"
expect_output "length-number"  'x=12345;      echo ${#x}'  "5"
expect_output "length-at"      'set -- a b c; echo ${#@}'  "3"
expect_output "length-star"    'set -- a b c; echo ${#*}'  "3"
expect_output "length-hash"    'set -- a b c; echo ${#}'   "3"


# --- Default / assign / error ---

expect_output "default-unset"   'unset x; echo ${x:-def}'        "def"
expect_output "default-empty"   'x=; echo ${x:-def}'             "def"
expect_output "default-set"     'x=val; echo ${x:-def}'          "val"
expect_output "assign-unset"    'unset x; echo ${x:=assigned}; echo $x' "assigned
assigned"
expect_output "alt-set"         'x=val; echo ${x:+alt}'          "alt"
expect_output "alt-unset"       'unset x; echo ${x:+alt}'        ""

# --- Non-colon variants (only trigger if unset, not if empty) ---
expect_output "nocolon-default-unset" 'unset x; echo ${x-def}'     "def"
expect_output "nocolon-default-empty" 'x=; echo ${x-def}'          ""
expect_output "nocolon-default-set"   'x=val; echo ${x-def}'       "val"
expect_output "nocolon-assign-unset"  'unset x; echo ${x=assigned}; echo $x' "assigned
assigned"
expect_output "nocolon-assign-empty"  'x=; echo ${x=assigned}; echo $x' ""
expect_output "nocolon-alt-set"       'x=val; echo ${x+alt}'       "alt"
expect_output "nocolon-alt-empty"     'x=; echo ${x+alt}'          "alt"
expect_output "nocolon-alt-unset"     'unset x; echo ${x+alt}'     ""

# --- Prefix stripping ---

expect_output "strip-prefix"           'x=hello_world; echo ${x#hello_}'        "world"
expect_output "strip-prefix-glob"      'x=/usr/local/bin; echo ${x#/*/}'        "local/bin"
expect_output "strip-prefix-greedy"    'x=/usr/local/bin; echo ${x##/*/}'       "bin"

# --- Suffix stripping ---

expect_output "strip-suffix"           'x=hello.tar.gz; echo ${x%.gz}'          "hello.tar"
expect_output "strip-suffix-greedy"    'x=hello.tar.gz; echo ${x%%.tar*}'       "hello"  # note: %% is same as % for suffix in most shells, but let's be specific
expect_output "strip-suffix-glob"      'x=hello_world; echo ${x%_*}'            "hello"

# --- Quoting interactions ---

expect_output "dquote-expands"     'x=world; echo "hello $x"'                   "hello world"
expect_output "squote-literal"     "echo 'no \$expansion here'"                 'no $expansion here'
expect_output "backslash-dollar"   'echo \$x'                                   '$x'
expect_output "dquote-backslash"   'echo "a\"b"'                                'a"b'

# --- Concatenation ---

expect_output "concat-vars"   'a=hello; b=world; echo ${a}_${b}'   "hello_world"
expect_output "concat-lit"    'x=foo; echo ${x}bar'                "foobar"

# --- Nested expansions & Special variables ---

expect_output "nested-cmdsub" 'echo $(echo $(echo deep))'   "deep"
expect_output "arith-in-var"  'x=5; echo $((x * x))'        "25"
expect_output "multidigit-positional" 'set -- a b c d e f g h i j k; echo ${10}' "j"

# --- Word splitting ---

expect_output "word-split-default" \
  'x="a b c"; for w in $x; do echo $w; done' \
  "a
b
c"

expect_output "word-split-suppress-dquote" \
  'x="a b c"; for w in "$x"; do echo $w; done' \
  "a b c"

expect_output "ifs-change" \
  'IFS=:; x="a:b:c"; for w in $x; do echo $w; done; IFS=" "' \
  "a
b
c"

# POSIX: "$*" must use first IFS char as separator, or no separator when IFS is null.
expect_output "dollar-star-ifs-separator" \
  'IFS=":"; set -- a b c; echo "$*"' \
  "a:b:c"

expect_output "dollar-star-null-ifs-no-separator" \
  'IFS=""; set -- a b c; echo "$*"' \
  "abc"

end_suite
