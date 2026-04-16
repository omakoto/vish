#!/usr/bin/env bash
# e2e/test_scripts.sh — Tests that run vish with multi-line vish script files

source "$(dirname "$0")/lib.sh"

begin_suite "Script Execution"

TMPDIR_TEST="$(mktemp -d)"
VISH="${VISH:-$(dirname "$0")/../vish}"
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# Helper: write a script file and run it
run_script() {
  local name="$1"
  local body="$2"
  local expected="$3"
  local scriptfile="$TMPDIR_TEST/${name}.sh"
  printf '%s\n' "$body" > "$scriptfile"
  local actual
  actual=$("$VISH" "$scriptfile" 2>&1)
  if [ "$actual" = "$expected" ]; then
    printf "[\033[32mPASS\033[0m] script: %s\n" "$name"
    PASS=$((PASS+1))
  else
    printf "[\033[31mFAIL\033[0m] script: %s\n" "$name"
    echo "  expected: |$expected|"
    echo "  actual:   |$actual|"
    FAIL=$((FAIL+1))
  fi
}

# --- Count words ---

run_script "word-count" \
'count=0
for word in the quick brown fox; do
  count=$((count + 1))
done
echo "words: $count"' \
"words: 4"

# --- FizzBuzz ---

run_script "fizzbuzz" \
'i=1
while [ $i -le 15 ]; do
  if [ $((i % 15)) -eq 0 ]; then echo FizzBuzz
  elif [ $((i % 3)) -eq 0 ]; then echo Fizz
  elif [ $((i % 5)) -eq 0 ]; then echo Buzz
  else echo $i
  fi
  i=$((i+1))
done' \
"1
2
Fizz
4
Buzz
Fizz
7
8
Fizz
Buzz
11
Fizz
13
14
FizzBuzz"

# --- Bubble sort ---

run_script "bubble-sort" \
'set -- 5 3 8 1 4
n=$#
# Convert to array-like sequential vars
i=1
for val in "$@"; do eval "a$i=$val"; i=$((i+1)); done

swapped=1
while [ $swapped -eq 1 ]; do
  swapped=0
  i=1
  while [ $i -lt $n ]; do
    j=$((i+1))
    eval "vi=\$a$i; vj=\$a$j"
    if [ $vi -gt $vj ]; then
      eval "a$i=$vj; a$j=$vi"
      swapped=1
    fi
    i=$((i+1))
  done
done

i=1
while [ $i -le $n ]; do
  eval "echo \$a$i"
  i=$((i+1))
done' \
"1
3
4
5
8"

# --- String processing ---

run_script "string-reverse-words" \
'input="one two three four"
result=""
for word in $input; do
  if [ -z "$result" ]; then
    result="$word"
  else
    result="$word $result"
  fi
done
echo "$result"' \
"four three two one"

# --- Simple stack implementation ---

run_script "stack" \
'sp=0
push() { eval "stack_$sp=$1"; sp=$((sp+1)); }
pop()  { sp=$((sp-1)); eval "echo \$stack_$sp"; }

push alpha
push beta
push gamma
pop
pop
pop' \
"gamma
beta
alpha"

# --- Generator pattern (command substitution loop) ---

run_script "generate-squares" \
'squares() {
  i=1
  while [ $i -le 5 ]; do
    echo $((i*i))
    i=$((i+1))
  done
}
sum=0
for n in $(squares); do
  sum=$((sum + n))
done
echo "sum of squares 1..5 = $sum"' \
"sum of squares 1..5 = 55"

# --- GCD ---

run_script "gcd" \
'gcd() {
  a=$1; b=$2
  while [ $b -ne 0 ]; do
    r=$((a % b))
    a=$b; b=$r
  done
  echo $a
}
gcd 48 18' \
"6"

# --- Heredoc in script ---

run_script "heredoc-in-script" \
'name=World
cat <<EOF
Hello, $name!
This is a heredoc.
EOF' \
"Hello, World!
This is a heredoc."

# --- exit code propagation ---

run_script "exit-code" \
'true
false
echo done' \
"done"

# --- Script with functions that call each other ---

run_script "mutual-functions" \
'is_even() {
  [ $1 -eq 0 ] && echo "even" && return
  is_odd $(($1 - 1))
}
is_odd() {
  [ $1 -eq 0 ] && echo "odd" && return
  is_even $(($1 - 1))
}
is_even 8
is_odd 7' \
"even
odd"

end_suite
