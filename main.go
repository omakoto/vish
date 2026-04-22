// Command vish is a POSIX-compatible shell.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/omakoto/vishc/interp"
	"github.com/omakoto/vishc/readline"
	"golang.org/x/term"
)

func main() {
	var (
		cmdFlag     = flag.String("c", "", "Execute command string")
		interFlag   = flag.Bool("i", false, "Force interactive mode")
		loginFlag   = flag.Bool("l", false, "Login shell")
		noexecFlag  = flag.Bool("n", false, "Read commands but don't execute")
		verboseFlag = flag.Bool("v", false, "Print input lines as they are read")
		xtraceFlag  = flag.Bool("x", false, "Print commands before executing")
		exitFlag    = flag.Bool("e", false, "Exit on error")
		nounsetFlag = flag.Bool("u", false, "Error on unset variable use")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: vish [options] [script [args...]]\n")
		fmt.Fprintf(os.Stderr, "       vish -c command [args...]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	sh := interp.New()
	sh.OptNoexec = *noexecFlag
	sh.OptXtrace = *xtraceFlag
	sh.OptExit = *exitFlag
	sh.OptNounset = *nounsetFlag
	_ = *verboseFlag
	_ = *loginFlag

	// Set $0
	sh.Positionals = []string{os.Args[0]}

	// -c: execute command string
	if *cmdFlag != "" {
		args := flag.Args()
		if len(args) > 0 {
			sh.Positionals = append([]string{args[0]}, args[1:]...)
		}
		os.Exit(sh.RunString(*cmdFlag))
	}

	// Script file
	if flag.NArg() > 0 {
		scriptFile := flag.Arg(0)
		args := flag.Args()
		sh.Positionals = args // $0=script, $1...=args
		os.Exit(sh.RunFile(scriptFile))
	}

	// Interactive or stdin mode
	isInteractive := *interFlag || term.IsTerminal(int(os.Stdin.Fd()))
	sh.IsInteractive = isInteractive

	if isInteractive {
		runInteractive(sh)
	} else {
		runStdin(sh)
	}
}

// runInteractive runs the interactive REPL loop.
func runInteractive(sh *interp.Shell) {
	rl, err := readline.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vish: readline: %v\n", err)
		runStdin(sh)
		return
	}
	defer rl.Close()

	sh.SetupSignals()

	// Source ~/.vishrc
	if home := os.Getenv("HOME"); home != "" {
		rc := home + "/.vishrc"
		if _, err := os.Stat(rc); err == nil {
			sh.RunFile(rc)
		}
	}

	// Set a default PS1 if unset
	if _, ok := sh.Env.Get("PS1"); !ok {
		_ = sh.Env.Set("PS1", `\u@\h:\w\$ `)
	}

	var pendingLines []string // for multi-line commands

	for {
		prompt := sh.PS1()
		if len(pendingLines) > 0 {
			ps2, _ := sh.Env.Get("PS2")
			if ps2 == "" {
				ps2 = "> "
			}
			prompt = ps2
		}

		line, err := rl.Readline(prompt)
		if err == io.EOF {
			fmt.Fprintln(os.Stdout)
			break
		}
		if err != nil {
			break
		}

		// Accumulate multi-line commands (line continuation or open constructs)
		if strings.HasSuffix(line, "\\") {
			pendingLines = append(pendingLines, line[:len(line)-1])
			continue
		}

		pendingLines = append(pendingLines, line)
		full := strings.Join(pendingLines, "\n")
		pendingLines = nil

		if strings.TrimSpace(full) == "" {
			continue
		}

		// Check for incomplete input (open quotes/parens)
		if isIncomplete(full) {
			pendingLines = []string{full}
			continue
		}

		sh.RunInteractive(full)
	}

	// Run EXIT trap
	if handler, ok := sh.Traps["EXIT"]; ok && handler != "" {
		sh.RunString(handler)
	}
	os.Exit(sh.LastStatus)
}

// runStdin reads and executes commands from stdin.
func runStdin(sh *interp.Shell) {
	scanner := bufio.NewScanner(os.Stdin)
	var pendingLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasSuffix(line, "\\") {
			pendingLines = append(pendingLines, line[:len(line)-1])
			continue
		}

		pendingLines = append(pendingLines, line)
		full := strings.Join(pendingLines, "\n")

		if isIncomplete(full) {
			pendingLines = append(pendingLines[:0], full)
			continue
		}

		pendingLines = nil

		if strings.TrimSpace(full) == "" {
			continue
		}

		sh.RunString(full)
		if sh.OptExit && sh.LastStatus != 0 {
			os.Exit(sh.LastStatus)
		}
	}

	// Flush any remaining
	if len(pendingLines) > 0 {
		full := strings.Join(pendingLines, "\n")
		if strings.TrimSpace(full) != "" {
			sh.RunString(full)
		}
	}

	if handler, ok := sh.Traps["EXIT"]; ok && handler != "" {
		sh.RunString(handler)
	}
	os.Exit(sh.LastStatus)
}

// isIncomplete reports whether the input looks like an incomplete shell
// command (open quotes, heredoc, open if/while/for/case/function/brace/paren).
func isIncomplete(input string) bool {
	runes := []rune(input)
	inSingle := false
	inDouble := false

	// Track open compound command keywords
	depth := 0      // ( ) depth
	braceDepth := 0 // { } depth
	backtick := 0

	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '\\' && i+1 < len(runes) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '`':
			backtick++
			if backtick%2 == 0 {
				backtick = 0
			}
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '\\':
			if i+1 < len(runes) && runes[i+1] == '\n' {
				i++ // line continuation
			}
		}
	}

	if inSingle || inDouble || depth > 0 || backtick > 0 {
		return true
	}

	// Check for open compound commands by counting keywords
	// This is a simplified heuristic
	tokens := tokenizeForKeywords(input)
	opens := 0
	i := 0
	for i < len(tokens) {
		switch tokens[i] {
		case "if":
			opens++
		case "fi":
			opens--
		case "while", "until", "for":
			opens++
		case "done":
			opens--
		case "case":
			opens++
		case "esac":
			opens--
		case "function":
			// function name { } — handled by braceDepth
		}
		i++
	}

	return opens > 0 || braceDepth > 0
}

// tokenizeForKeywords does a minimal keyword scan for completeness checking.
func tokenizeForKeywords(input string) []string {
	var tokens []string
	inSingle, inDouble := false, false
	var cur strings.Builder

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}

	for _, c := range input {
		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			cur.WriteRune(c)
			continue
		}
		if inDouble {
			if c == '"' {
				inDouble = false
			}
			cur.WriteRune(c)
			continue
		}
		if c == '\'' {
			inSingle = true
			cur.WriteRune(c)
			continue
		}
		if c == '"' {
			inDouble = true
			cur.WriteRune(c)
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == ';' || c == '|' || c == '&' {
			flush()
			continue
		}
		cur.WriteRune(c)
	}
	flush()
	return tokens
}
