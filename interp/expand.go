// Package interp — word expansion for vish shell.
package interp

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ExpandWord expands a single raw word from the parser into a list of strings
// (after tilde, parameter, command, arithmetic expansion, word splitting and
// pathname expansion). If quoted is true (e.g., inside "..."), word splitting
// and pathname expansion are suppressed.
func (sh *Shell) ExpandWord(raw string) ([]string, error) {
	// Phase 1-4: expand the word into a single string, tracking unquoted regions.
	expanded, unquoted, err := sh.expandWordParts(raw)
	if err != nil {
		return nil, err
	}

	// Phase 5: word splitting on unquoted text
	parts := wordSplit(expanded, unquoted, sh.getIFS())

	// Phase 6: pathname expansion on each unquoted word
	var result []string
	for _, p := range parts {
		if p.unquoted && containsGlob(p.val) {
			matches, err := filepath.Glob(p.val)
			if err != nil || len(matches) == 0 {
				result = append(result, p.val) // no matches → literal
			} else {
				sort.Strings(matches)
				result = append(result, matches...)
			}
		} else {
			result = append(result, p.val)
		}
	}

	if len(result) == 0 {
		return nil, nil // empty expansion yields nothing
	}
	return result, nil
}

// ExpandWordNoSplit expands a word without word-splitting or globbing.
// Used for assignment RHS, heredoc bodies, etc.
func (sh *Shell) ExpandWordNoSplit(raw string) (string, error) {
	expanded, _, err := sh.expandWordParts(raw)
	if err != nil {
		return "", err
	}
	return expanded, nil
}

// ExpandWords expands a list of raw words into a flat list.
func (sh *Shell) ExpandWords(raws []string) ([]string, error) {
	var result []string
	for _, r := range raws {
		words, err := sh.ExpandWord(r)
		if err != nil {
			return nil, err
		}
		result = append(result, words...)
	}
	return result, nil
}

// ─── segment: tracks which characters came from unquoted regions ─────────────

type segment struct {
	val      string
	unquoted bool
}

// expandWordParts expands a raw word, returning the expanded string and a
// parallel boolean slice indicating which bytes are from unquoted regions.
func (sh *Shell) expandWordParts(raw string) (string, []bool, error) {
	src := []rune(raw)
	var result strings.Builder
	var unquoted []bool

	write := func(s string, uq bool) {
		result.WriteString(s)
		for range s {
			unquoted = append(unquoted, uq)
		}
	}

	err := sh.expandRunes(src, &result, &unquoted, write, false)
	if err != nil {
		return "", nil, err
	}
	return result.String(), unquoted, nil
}

// expandRunes is the core expansion loop.
// inDouble: true when inside a double-quoted region.
func (sh *Shell) expandRunes(
	src []rune,
	result *strings.Builder,
	unquoted *[]bool,
	write func(string, bool),
	inDouble bool,
) error {
	i := 0
	for i < len(src) {
		ch := src[i]
		switch {
		case ch == '\'' && !inDouble:
			// Single quote: read until matching '
			i++
			start := i
			for i < len(src) && src[i] != '\'' {
				i++
			}
			write(string(src[start:i]), false)
			if i < len(src) {
				i++ // closing '
			}

		case ch == '"':
			if inDouble {
				// End of double-quote
				return nil
			}
			i++
			// Expand inside double quotes (no word-split, no glob)
			for i < len(src) && src[i] != '"' {
				c := src[i]
				switch c {
				case '$':
					i++
					s, n, err := sh.expandDollar(src, i, true)
					if err != nil {
						return err
					}
					write(s, false)
					i += n
				case '`':
					s, n, err := sh.expandBacktick(src, i)
					if err != nil {
						return err
					}
					write(s, false)
					i += n
				case '\\':
					i++
					if i < len(src) {
						nc := src[i]
						// Inside double quotes, \ only escapes $, `, ", \, newline
						if nc == '$' || nc == '`' || nc == '"' || nc == '\\' || nc == '\n' {
							if nc != '\n' {
								write(string(nc), false)
							}
						} else {
							write("\\"+string(nc), false)
						}
						i++
					}
				default:
					write(string(c), false)
					i++
				}
			}
			if i < len(src) {
				i++ // closing "
			}

		case ch == '\\' && !inDouble:
			i++
			if i < len(src) {
				if src[i] == '\n' {
					i++ // line continuation
				} else {
					write(string(src[i]), false)
					i++
				}
			}

		case ch == '$':
			i++
			s, n, err := sh.expandDollar(src, i, inDouble)
			if err != nil {
				return err
			}
			write(s, !inDouble)
			i += n

		case ch == '`':
			s, n, err := sh.expandBacktick(src, i)
			if err != nil {
				return err
			}
			write(s, !inDouble)
			i += n

		case ch == '~' && !inDouble && i == 0:
			// Tilde expansion only at start of word
			end := i + 1
			for end < len(src) && src[end] != '/' && src[end] != ':' {
				end++
			}
			name := string(src[i+1 : end])
			home, err := expandTilde(name)
			if err != nil {
				// Not expandable, keep literal
				write("~"+name, true)
			} else {
				write(home, false)
			}
			i = end

		default:
			write(string(ch), !inDouble)
			i++
		}
	}
	return nil
}

// expandTilde expands ~ or ~user.
func expandTilde(name string) (string, error) {
	if name == "" {
		if h := os.Getenv("HOME"); h != "" {
			return h, nil
		}
		u, err := user.Current()
		if err != nil {
			return "", err
		}
		return u.HomeDir, nil
	}
	u, err := user.Lookup(name)
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

// expandDollar handles everything after $ at position i in src.
// Returns the expanded string and number of runes consumed (not including the $).
func (sh *Shell) expandDollar(src []rune, i int, inDouble bool) (string, int, error) {
	if i >= len(src) {
		return "$", 0, nil
	}

	ch := src[i]

	switch {
	case ch == '{':
		// ${...}
		return sh.expandParamBrace(src, i+1)

	case ch == '(' && i+1 < len(src) && src[i+1] == '(':
		// $(( arith ))
		return sh.expandArith(src, i+2)

	case ch == '(':
		// $( cmd )
		return sh.expandCmdSubst(src, i+1)

	case isNameStartRune(ch):
		// $NAME
		end := i
		for end < len(src) && isNameContRune(src[end]) {
			end++
		}
		name := string(src[i:end])
		val, _ := sh.Env.Get(name)
		return val, end - i, nil

	case ch >= '0' && ch <= '9':
		// Positional parameter $0..$9
		n := int(ch - '0')
		val := sh.getPositional(n)
		return val, 1, nil

	case ch == '@':
		if inDouble {
			// "$@" expands to separate quoted words — join with \x00 sentinel;
			// word splitter will see this as a separator
			val := strings.Join(sh.Positionals[1:], "\x00")
			return val, 1, nil
		}
		val := strings.Join(sh.Positionals[1:], " ")
		return val, 1, nil

	case ch == '*':
		ifs := sh.getIFS()
		// POSIX: "$*" joins with first IFS char; with null IFS, joins with no separator.
		sep := ""
		if len(ifs) > 0 {
			sep = string(ifs[0])
		}
		val := strings.Join(sh.Positionals[1:], sep)
		return val, 1, nil

	case ch == '#':
		val := strconv.Itoa(len(sh.Positionals) - 1)
		return val, 1, nil

	case ch == '?':
		return strconv.Itoa(sh.LastStatus), 1, nil

	case ch == '$':
		return strconv.Itoa(os.Getpid()), 1, nil

	case ch == '!':
		return strconv.Itoa(sh.LastBgPID), 1, nil

	case ch == '-':
		return sh.shellFlags(), 1, nil
	}

	// Just a literal $
	return "$", 0, nil
}

// expandParamBrace handles ${...}. i points right after the opening {.
func (sh *Shell) expandParamBrace(src []rune, i int) (string, int, error) {
	start := i // position inside braces

	// Find matching }
	depth := 1
	j := i
	for j < len(src) && depth > 0 {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				break
			}
		case '\'':
			j++
			for j < len(src) && src[j] != '\'' {
				j++
			}
		case '"':
			j++
			for j < len(src) && src[j] != '"' {
				j++
			}
		case '\\':
			j++
		}
		j++
	}
	if depth != 0 {
		return "", 0, fmt.Errorf("missing }")
	}

	inner := string(src[start : j-1]) // content between { and }
	// j now points past the closing }

	val, err := sh.evalParamExpr(inner)
	if err != nil {
		return "", 0, err
	}
	// consumed: "{ ... }" = (j - start + 1) runes after the $. We consumed start-1 to j
	// but expandDollar was called with i = position of '{', so consumed = j - (start-1)
	return val, j - (start - 1), nil
}

// evalParamExpr evaluates the expression inside ${...}.
func (sh *Shell) evalParamExpr(inner string) (string, error) {
	// ${#name} — length
	if strings.HasPrefix(inner, "#") && len(inner) > 1 {
		name := inner[1:]
		if name == "@" || name == "*" {
			return strconv.Itoa(len(sh.Positionals) - 1), nil
		}
		val, _ := sh.getVarOrSpecial(name)
		return strconv.Itoa(len([]rune(val))), nil
	}

	// Two-character operators must be checked before one-character operators
	for _, op := range []string{":-", ":=", ":?", ":+", "##", "%%"} {
		if idx := strings.Index(inner, op); idx > 0 {
			name := inner[:idx]
			word := inner[idx+len(op):]
			return sh.applyParamOp(name, op, word)
		}
	}
	// One-character operators
	for _, op := range []string{"-", "=", "?", "+", "#", "%"} {
		if idx := strings.Index(inner, op); idx > 0 {
			name := inner[:idx]
			word := inner[idx+len(op):]
			return sh.applyParamOp(name, op, word)
		}
	}


	// Just ${name}
	val, _ := sh.getVarOrSpecial(inner)
	return val, nil
}

// getVarOrSpecial looks up a variable, handling special names like @, *, #, ?, $, !, -, and positionals.
func (sh *Shell) getVarOrSpecial(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	if len(name) == 1 {
		switch name[0] {
		case '@':
			return strings.Join(sh.Positionals[1:], " "), true
		case '*':
			ifs := sh.getIFS()
			// POSIX: "$*" joins with first IFS char; with null IFS, joins with no separator.
			sep := ""
			if len(ifs) > 0 {
				sep = string(ifs[0])
			}
			return strings.Join(sh.Positionals[1:], sep), true
		case '#':
			return strconv.Itoa(len(sh.Positionals) - 1), true
		case '?':
			return strconv.Itoa(sh.LastStatus), true
		case '$':
			return strconv.Itoa(os.Getpid()), true
		case '!':
			return strconv.Itoa(sh.LastBgPID), true
		case '-':
			return sh.shellFlags(), true
		}
	}
	
	isNumeric := true
	for _, r := range name {
		if r < '0' || r > '9' {
			isNumeric = false
			break
		}
	}
	if isNumeric {
		n, _ := strconv.Atoi(name)
		val := sh.getPositional(n)
		if n < len(sh.Positionals) {
			return val, true
		}
		return "", false
	}
	
	return sh.Env.Get(name)
}

func (sh *Shell) applyParamOp(name, op, word string) (string, error) {
	val, set := sh.getVarOrSpecial(name)

	switch op {
	case ":-", "-":
		// :- Use default if unset or empty
		// - Use default if unset
		if !set || (op == ":-" && val == "") {
			return sh.ExpandWordNoSplit(word)
		}
		return val, nil

	case ":=", "=":
		// := Assign default if unset or empty
		// = Assign default if unset
		if !set || (op == ":=" && val == "") {
			def, err := sh.ExpandWordNoSplit(word)
			if err != nil {
				return "", err
			}
			_ = sh.Env.Set(name, def)
			return def, nil
		}
		return val, nil

	case ":?", "?":
		// :? Error if unset or empty
		// ? Error if unset
		if !set || (op == ":?" && val == "") {
			msg, err := sh.ExpandWordNoSplit(word)
			if err != nil {
				return "", err
			}
			if msg == "" {
				msg = name + ": parameter null or not set"
			}
			return "", fmt.Errorf("%s", msg)
		}
		return val, nil

	case ":+", "+":
		// :+ Use alternate if set and non-empty
		// + Use alternate if set
		if set && (op == "+" || val != "") {
			return sh.ExpandWordNoSplit(word)
		}
		return "", nil

	case "#":
		// Remove shortest prefix matching word (as glob)
		pattern, err := sh.ExpandWordNoSplit(word)
		if err != nil {
			return "", err
		}
		return removePrefix(val, pattern, false), nil

	case "##":
		// Remove longest prefix
		pattern, err := sh.ExpandWordNoSplit(word)
		if err != nil {
			return "", err
		}
		return removePrefix(val, pattern, true), nil

	case "%":
		// Remove shortest suffix
		pattern, err := sh.ExpandWordNoSplit(word)
		if err != nil {
			return "", err
		}
		return removeSuffix(val, pattern, false), nil

	case "%%":
		// Remove longest suffix
		pattern, err := sh.ExpandWordNoSplit(word)
		if err != nil {
			return "", err
		}
		return removeSuffix(val, pattern, true), nil
	}

	return val, nil
}

// removePrefix removes the prefix from val matching pattern (shortest or longest).
func removePrefix(val, pattern string, longest bool) string {
	if longest {
		// Try from longest possible match down to shortest
		for i := len(val); i >= 0; i-- {
			if shellGlobMatch(pattern, val[:i]) {
				return val[i:]
			}
		}
	} else {
		// Shortest match
		for i := 0; i <= len(val); i++ {
			if shellGlobMatch(pattern, val[:i]) {
				return val[i:]
			}
		}
	}
	return val
}

// removeSuffix removes the suffix from val matching pattern.
func removeSuffix(val, pattern string, longest bool) string {
	if longest {
		for i := 0; i <= len(val); i++ {
			if shellGlobMatch(pattern, val[i:]) {
				return val[:i]
			}
		}
	} else {
		for i := len(val); i >= 0; i-- {
			if shellGlobMatch(pattern, val[i:]) {
				return val[:i]
			}
		}
	}
	return val
}

// shellGlobMatch matches a shell glob pattern against a string.
// Unlike filepath.Match, '*' matches any sequence including '/'.
func shellGlobMatch(pattern, s string) bool {
	// Convert shell glob pattern to a simple recursive matcher
	return shellGlobMatchRunes([]rune(pattern), []rune(s))
}

func shellGlobMatchRunes(pat, s []rune) bool {
	for len(pat) > 0 {
		switch pat[0] {
		case '*':
			pat = pat[1:]
			// * matches zero or more characters (including /)
			if len(pat) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if shellGlobMatchRunes(pat, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pat = pat[1:]
			s = s[1:]
		case '[':
			if len(s) == 0 {
				return false
			}
			// Use filepath.Match for bracket expressions (limited)
			ok, err := filepath.Match(string(pat[:2]), string(s[:1]))
			if err != nil || !ok {
				return false
			}
			pat = pat[1:]
			s = s[1:]
		case '\\':
			if len(pat) < 2 {
				return false
			}
			if len(s) == 0 || pat[1] != s[0] {
				return false
			}
			pat = pat[2:]
			s = s[1:]
		default:
			if len(s) == 0 || pat[0] != s[0] {
				return false
			}
			pat = pat[1:]
			s = s[1:]
		}
	}
	return len(s) == 0
}


// expandCmdSubst handles $( ... ). i points past the opening '('.
func (sh *Shell) expandCmdSubst(src []rune, i int) (string, int, error) {
	depth := 1
	j := i
	for j < len(src) && depth > 0 {
		switch src[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				break
			}
		case '\'':
			j++
			for j < len(src) && src[j] != '\'' {
				j++
			}
		case '"':
			j++
			for j < len(src) && src[j] != '"' {
				j++
			}
		case '\\':
			j++
		}
		j++
	}
	inner := string(src[i : j-1]) // content between ( and )
	// consumed: from opening ( through closing ) = j - i + 1
	// But expandDollar was called with i = position of '('; consumed includes the '('
	consumed := j - (i - 1)

	output, err := sh.runCmdSubst(inner)
	if err != nil {
		return "", 0, err
	}
	// Strip trailing newlines per POSIX
	output = strings.TrimRight(output, "\n")
	return output, consumed, nil
}

// expandArith handles $(( ... )). i points past the second '('.
func (sh *Shell) expandArith(src []rune, i int) (string, int, error) {
	depth := 2
	j := i
	for j < len(src) && depth > 0 {
		if src[j] == '(' {
			depth++
		} else if src[j] == ')' {
			depth--
		}
		j++
	}
	inner := string(src[i : j-2]) // between (( and ))
	// consumed from '((' through '))': j - (i-2)
	consumed := j - (i - 2)

	result, err := EvalArith(inner, sh)
	if err != nil {
		return "", 0, fmt.Errorf("arithmetic: %w", err)
	}
	return strconv.FormatInt(result, 10), consumed, nil
}

// expandBacktick handles `...`. i points at the opening backtick.
func (sh *Shell) expandBacktick(src []rune, i int) (string, int, error) {
	j := i + 1
	for j < len(src) && src[j] != '`' {
		if src[j] == '\\' {
			j++
		}
		j++
	}
	inner := string(src[i+1 : j])
	// Unescape \` inside backticks
	inner = strings.ReplaceAll(inner, "\\`", "`")
	consumed := j - i + 1 // from ` to `

	output, err := sh.runCmdSubst(inner)
	if err != nil {
		return "", 0, err
	}
	output = strings.TrimRight(output, "\n")
	return output, consumed, nil
}

// runCmdSubst runs a command string and captures its stdout.
func (sh *Shell) runCmdSubst(cmdStr string) (string, error) {
	var buf bytes.Buffer
	sub := sh.subshell()
	sub.Stdout = &buf
	sub.RunString(cmdStr) // exit status is accessible via sub.LastStatus; we ignore it here
	return buf.String(), nil
}

// ─── Word splitting ───────────────────────────────────────────────────────────

type splitPart struct {
	val      string
	unquoted bool
}

// wordSplit splits an expanded string using IFS, respecting quoted regions.
//
// POSIX field splitting rules:
//   - Each IFS-whitespace sequence (alone) collapses into one separator; leading/trailing ignored.
//   - Each IFS-non-whitespace character is always a separator, even creating empty fields.
//   - IFS-whitespace adjacent to an IFS-non-whitespace char is merged with it (not a second separator).
func wordSplit(s string, unquoted []bool, ifs string) []splitPart {
	if ifs == "" {
		// No splitting
		return []splitPart{{val: s, unquoted: anyUnquoted(unquoted)}}
	}

	ifsWhiteStr := ifsWhitespace(ifs)
	ifsNonWhiteStr := ifsNonWhitespace(ifs)

	isW := func(r rune) bool { return strings.ContainsRune(ifsWhiteStr, r) }
	isN := func(r rune) bool { return strings.ContainsRune(ifsNonWhiteStr, r) }
	isUnq := func(i int) bool { return i < len(unquoted) && unquoted[i] }

	var parts []splitPart
	var cur strings.Builder
	curUnquoted := false

	// State machine states:
	//   0 = accumulating field characters
	//   1 = after IFS-whitespace (no non-whitespace IFS seen yet; leading/standalone whitespace)
	//   2 = after IFS-non-whitespace (and any adjacent right-side IFS-whitespace)
	state := 0

	emitField := func() {
		parts = append(parts, splitPart{val: cur.String(), unquoted: curUnquoted || cur.Len() == 0})
		cur.Reset()
		curUnquoted = false
	}

	for i, r := range []rune(s) {
		uq := isUnq(i)

		// \x00: internal "$@" field boundary — always split here regardless of IFS
		if r == '\x00' {
			emitField()
			state = 0
			continue
		}

		if !uq {
			// Quoted character — always part of current field
			cur.WriteRune(r)
			curUnquoted = false
			state = 0
			continue
		}

		switch {
		case isN(r):
			switch state {
			case 0: // field → emit current field, enter state 2
				emitField()
				state = 2
			case 1: // IFS-whitespace was adjacent to this non-whitespace IFS; field already emitted
				state = 2
			case 2: // consecutive non-whitespace IFS → emit empty field
				emitField()
				// state stays 2
			}

		case isW(r):
			switch state {
			case 0: // field → emit if non-empty, enter state 1
				if cur.Len() > 0 {
					emitField()
				}
				state = 1
			case 1: // more whitespace — skip (collapse)
			case 2: // right-adjacent whitespace after non-whitespace IFS — skip
			}

		default:
			// Regular (non-IFS) character
			cur.WriteRune(r)
			curUnquoted = true
			state = 0
		}
	}

	// End of string
	switch state {
	case 0:
		if cur.Len() > 0 {
			parts = append(parts, splitPart{val: cur.String(), unquoted: curUnquoted})
		}
	case 1:
		// Trailing IFS-whitespace: ignored per POSIX
	case 2:
		// Trailing IFS-non-whitespace: emit empty trailing field
		parts = append(parts, splitPart{val: "", unquoted: true})
	}

	if len(parts) == 0 {
		return []splitPart{{val: "", unquoted: true}}
	}
	return parts
}

func ifsWhitespace(ifs string) string {
	var b strings.Builder
	for _, c := range ifs {
		if c == ' ' || c == '\t' || c == '\n' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func ifsNonWhitespace(ifs string) string {
	var b strings.Builder
	for _, c := range ifs {
		if c != ' ' && c != '\t' && c != '\n' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func anyUnquoted(flags []bool) bool {
	for _, f := range flags {
		if f {
			return true
		}
	}
	return false
}

// containsGlob reports whether a string contains unescaped glob metacharacters.
func containsGlob(s string) bool {
	for _, c := range s {
		if c == '*' || c == '?' || c == '[' {
			return true
		}
	}
	return false
}

func (sh *Shell) getIFS() string {
	v, ok := sh.Env.Get("IFS")
	if !ok {
		return " \t\n"
	}
	return v
}

func (sh *Shell) getPositional(n int) string {
	if n >= len(sh.Positionals) {
		return ""
	}
	return sh.Positionals[n]
}

func (sh *Shell) shellFlags() string {
	var sb strings.Builder
	if sh.OptExit {
		sb.WriteByte('e')
	}
	if sh.OptNounset {
		sb.WriteByte('u')
	}
	if sh.OptXtrace {
		sb.WriteByte('x')
	}
	return sb.String()
}
