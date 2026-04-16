#!/usr/bin/env bash
# e2e/test_io.sh — I/O redirection and heredoc tests

source "$(dirname "$0")/lib.sh"

begin_suite "I/O Redirection & Heredoc"

TMPDIR_TEST="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# --- Output redirection ---

expect_output "redirect-stdout-to-file" \
  "echo hello > $TMPDIR_TEST/out.txt; cat $TMPDIR_TEST/out.txt" \
  "hello"

expect_output "redirect-append" \
  "echo line1 > $TMPDIR_TEST/app.txt; echo line2 >> $TMPDIR_TEST/app.txt; cat $TMPDIR_TEST/app.txt" \
  "line1
line2"

expect_output "redirect-stderr" \
  'echo "err" >&2' \
  "err"

# --- Input redirection ---

expect_output "redirect-stdin" \
  "echo 'hello from file' > $TMPDIR_TEST/in.txt; cat < $TMPDIR_TEST/in.txt" \
  "hello from file"

# --- Pipeline chaining ---

expect_output "pipeline-3-stage" \
  'printf "banana\napple\ncherry\n" | sort | head -2' \
  "apple
banana"

expect_output "pipeline-with-grep" \
  'printf "foo\nbar\nbaz\n" | grep "ba"' \
  "bar
baz"

expect_output "pipeline-preserves-exit-status" \
  'echo hello | cat | cat; echo $?' \
  "hello
0"

# --- Command substitution complex ---

expect_output "cmdsub-multiline" \
  'x=$(printf "a\nb\nc"); echo "$x"' \
  "a
b
c"

expect_output "cmdsub-strips-trailing-newline" \
  'x=$(printf "hello\n\n"); printf "[%s]" "$x"' \
  "[hello]"

expect_output "cmdsub-nested" \
  'echo $(echo $(echo innermost))' \
  "innermost"

expect_output "cmdsub-in-assign" \
  'files=$(ls /); echo "$files" | grep -q "usr" && echo "has usr"' \
  "has usr"

# --- Heredoc ---

expect_output "heredoc-basic" \
  'cat <<EOF
line one
line two
EOF' \
  "line one
line two"

expect_output "heredoc-variable-expansion" \
  'name=Alice
   cat <<EOF
Hello, $name!
EOF' \
  "Hello, Alice!"

expect_output "heredoc-no-expand-quoted" \
  'cat <<'"'"'EOF'"'"'
no $expansion
EOF' \
  'no $expansion'

expect_output "heredoc-strip-tabs" \
  'cat <<-EOF
	line one
	line two
	EOF' \
  "line one
line two"

expect_output "heredoc-to-var" \
  'x=$(cat <<EOF
hello heredoc
EOF
)
echo "$x"' \
  "hello heredoc"

expect_output "heredoc-multiple" \
  'cat <<A; cat <<B
first
A
second
B' \
  "first
second"

# --- /dev/null suppression ---

expect_output "suppress-stderr" \
  'echo hello 2>/dev/null; ls /nonexistent 2>/dev/null; echo done' \
  "hello
done"

# --- fd duplication ---

expect_output "stdout-to-stderr-and-back" \
  'echo "to stdout"' \
  "to stdout"

end_suite
