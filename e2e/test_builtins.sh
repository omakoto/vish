#!/usr/bin/env bash
# e2e/test_builtins.sh — Built-in command tests

source "$(dirname "$0")/lib.sh"

begin_suite "Built-in Commands"

TMPDIR_TEST="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# --- echo ---

expect_output "echo-n"      'echo -n hello'              "hello"
expect_output "echo-e"      'echo -e "a\tb"'             "$(printf 'a\tb')"
expect_output "echo-empty"  'echo'                       ""

# --- printf ---

expect_output "printf-string"  'printf "%s\n" hello world'   "hello
world"
expect_output "printf-int"     'printf "%d + %d = %d\n" 3 4 7'  "3 + 4 = 7"
expect_output "printf-padded"  'printf "%-10s|\n" hi'           "hi        |"
expect_output "printf-hex"     'printf "0x%x\n" 255'            "0xff"
expect_output "printf-oct"     'printf "%o\n" 8'                "10"

# --- test / [ ] ---

expect_output "test-file-exists"    "echo x > $TMPDIR_TEST/f; [ -f $TMPDIR_TEST/f ] && echo yes"  "yes"
expect_output "test-file-not-exist" '[ -f /no/such/file ] || echo missing'  "missing"
expect_output "test-dir"            "[ -d $TMPDIR_TEST ] && echo yes"  "yes"
expect_output "test-readable"       "echo x > $TMPDIR_TEST/r; [ -r $TMPDIR_TEST/r ] && echo yes"  "yes"
expect_output "test-string-empty"   '[ -z "" ] && echo yes'            "yes"
expect_output "test-string-nonempty" '[ -n "x" ] && echo yes'          "yes"
expect_output "test-string-eq"      '[ "abc" = "abc" ] && echo yes'   "yes"
expect_output "test-string-ne"      '[ "abc" != "xyz" ] && echo yes'  "yes"
expect_output "test-int-lt"         '[ 3 -lt 5 ] && echo yes'         "yes"
expect_output "test-int-ge"         '[ 5 -ge 5 ] && echo yes'         "yes"
expect_output "test-compound-and"   '[ 1 -eq 1 -a 2 -eq 2 ] && echo yes'  "yes"
expect_output "test-compound-or"    '[ 1 -eq 2 -o 2 -eq 2 ] && echo yes'  "yes"
expect_output "test-not"            '[ ! 1 -eq 2 ] && echo yes'            "yes"

# --- read ---

expect_output "read-basic" \
  'echo "hello world" | (read x; echo "got: $x")' \
  "got: hello world"

expect_output "read-two-vars" \
  'echo "hello world" | (read a b; echo "$a|$b")' \
  "hello|world"

expect_output "read-rest-in-last" \
  'echo "a b c d" | (read a rest; echo "$a :: $rest")' \
  "a :: b c d"

# --- set and $@ ---

expect_output "set-positionals" \
  'set -- one two three; echo $1 $2 $3' \
  "one two three"

expect_output "set-e-stops-on-error" \
  '(set -e; false; echo unreachable) || echo "stopped"' \
  "stopped" \
  "bash"  # bash --posix: set -e does not exit subshell when guarded by ||

# set -x sends trace to stderr; capture it and verify the assignment appears
expect_output "set-x-traces" \
  'set -x; x=hello 2>&1 1>/dev/null' \
  "+ x=hello"

# --- shift ---

expect_output "shift-1" \
  'set -- a b c; shift; echo $@' \
  "b c"

expect_output "shift-n" \
  'set -- a b c d; shift 2; echo $@' \
  "c d"

expect_output "shift-out-of-bounds" \
  'set -- a b c; shift 5 2>/dev/null; echo "Status: $?, Count: $#"' \
  "Status: 1, Count: 3"

# --- unset ---

expect_output "unset-var" \
  'x=hello; unset x; echo ${x:-gone}' \
  "gone"

# --- export ---

expect_output "export-visible-to-child" \
  'export VISH_TEST_VAR=testval; env | grep "^VISH_TEST_VAR="' \
  "VISH_TEST_VAR=testval"

# --- readonly ---

expect_output "readonly-prevents-assign" \
  'readonly x=5; x=6 2>/dev/null; echo $x' \
  "5" \
  "bash"  # bash --posix: readonly error bleeds through even with 2>/dev/null in this context

# --- eval ---

expect_output "eval-basic" \
  'cmd="echo hello eval"; eval "$cmd"' \
  "hello eval"

expect_output "eval-assign" \
  'eval "x=42"; echo $x' \
  "42"

expect_output "eval-complex" \
  'var=greeting; eval "$var=hello"; echo $greeting' \
  "hello"

# --- alias ---

expect_output "alias-define-and-use" \
  'alias greet="echo hi"; greet there' \
  "hi there" \
  "bash"  # bash --posix does not expand aliases in non-interactive mode

# --- type ---

expect_output "type-builtin" \
  'type echo | grep -q builtin && echo yes' \
  "yes"

expect_output "type-function" \
  'f() { echo hi; }; type f | grep -q function && echo yes' \
  "yes"

end_suite
