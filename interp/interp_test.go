// Package interp contains tests for the Vish shell interpreter
package interp_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/omakoto/vishc/interp"
)

// runVishScript runs a script and returns stdout, stderr, and exit code.
func runVishScript(input string) (string, string, int) {
	sh := interp.New()
	
	// Capture I/O
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	
	sh.Stdin = strings.NewReader("")
	sh.Stdout = &stdoutBuf
	sh.Stderr = &stderrBuf
	
	// Standard test environment
	sh.Env.Set("PATH", os.Getenv("PATH"))
	sh.Env.Set("HOME", os.Getenv("HOME"))
	sh.Env.Set("USER", os.Getenv("USER"))
	
	// Execute
	status := sh.RunString(input)
	
	return stdoutBuf.String(), stderrBuf.String(), status
}

func TestCoreFeatures(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		status   int
	}{
		{
			name:     "echo",
			input:    "echo hello world",
			expected: "hello world\n",
			status:   0,
		},
		{
			name:     "variable assignment and expansion",
			input:    "x=world; echo hello $x",
			expected: "hello world\n",
			status:   0,
		},
		{
			name:     "arithmetic expansion",
			input:    "echo $((2+3))",
			expected: "5\n",
			status:   0,
		},
		{
			name:     "if-true",
			input:    "if true; then echo yes; fi",
			expected: "yes\n",
			status:   0,
		},
		{
			name:     "while loop",
			input:    "i=0; while [ $i -lt 3 ]; do echo $i; i=$((i+1)); done",
			expected: "0\n1\n2\n",
			status:   1, // loop ends when condition fails (1)
		},
		{
			name:     "for loop",
			input:    "for x in a b c; do echo $x; done",
			expected: "a\nb\nc\n",
			status:   0,
		},
		{
			name:     "function definition and call",
			input:    "greet() { echo hello $1; }; greet world",
			expected: "hello world\n",
			status:   0,
		},
		{
			name:     "and/or operators",
			input:    "true && echo yes; false || echo no",
			expected: "yes\nno\n",
			status:   0,
		},
		{
			name:     "command substitution",
			input:    "x=$(echo hello); echo $x",
			expected: "hello\n",
			status:   0,
		},
		{
			name:     "single quotes",
			input:    "echo 'hello $world'",
			expected: "hello $world\n",
			status:   0,
		},
		{
			name:     "double quotes",
			input:    "x=world; echo \"hello $x\"",
			expected: "hello world\n",
			status:   0,
		},
		{
			name:     "case statement",
			input:    "x=b; case $x in a) echo A;; b) echo B;; esac",
			expected: "B\n",
			status:   0,
		},
		{
			name:     "subshell",
			input:    "(echo inside)",
			expected: "inside\n",
			status:   0,
		},
		{
			name:     "test equality",
			input:    "[ 1 -eq 1 ] && echo yes",
			expected: "yes\n",
			status:   0,
		},
		{
			name:     "negation",
			input:    "! false && echo yes",
			expected: "yes\n",
			status:   0,
		},
		{
			name:     "parameter default",
			input:    "x=; echo ${x:-default}",
			expected: "default\n",
			status:   0,
		},
		{
			name:     "nested functions",
			input:    "a() { echo a-$1; }; b() { a inner; }; b",
			expected: "a-inner\n",
			status:   0,
		},
		{
			name:     "local variable",
			input:    "x=global; f() { local x=local; echo $x; }; f; echo $x",
			expected: "local\nglobal\n",
			status:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, status := runVishScript(tt.input)
			if status != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, status)
			}
			if stdout != tt.expected {
				t.Errorf("expected stdout %q, got %q", tt.expected, stdout)
			}
		})
	}
}

// TestHeredoc test heredocs separately because they are structurally complex
func TestHeredoc(t *testing.T) {
	input := `cat <<EOF
hello
world
EOF`
	
	stdout, _, status := runVishScript(input)
	if status != 0 {
		t.Errorf("expected status 0, got %d", status)
	}
	
	expected := "hello\nworld\n"
	if stdout != expected {
		t.Errorf("expected stdout %q, got %q", expected, stdout)
	}
}

// TestPipeline tests pipelining which requires real goroutines
func TestPipeline(t *testing.T) {
	input := `echo hello world | tr ' ' '\n' | head -1`
	
	// Use standard sh with access to tr/head from PATH
	stdout, _, status := runVishScript(input)
	if status != 0 {
		t.Errorf("expected status 0, got %d", status)
	}
	
	expected := "hello\n"
	if stdout != expected {
		t.Errorf("expected stdout %q, got %q", expected, stdout)
	}
}
