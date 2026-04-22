// Package lexer provides tokenization for the vish shell.
package lexer

import "strings"

// Lexer tokenizes shell source text.
// Words preserve all quoting/expansion syntax verbatim; the interpreter
// expands them at runtime.
type Lexer struct {
	src   []rune
	pos   int
	line  int
	col   int
	queue []Token // look-ahead buffer
	// Pending heredocs: after scanning a newline that follows a << operator,
	// fill these heredoc structures from the input.
	pendingHeredocs []*HeredocPending
}

// HeredocPending holds a heredoc redir waiting for its body to be read.
type HeredocPending struct {
	Delim string
	Strip bool    // <<-
	Out   *string // pointer to Redir.HereDoc
}

// New returns a Lexer for the given input string.
func New(input string) *Lexer {
	return &Lexer{src: []rune(input), pos: 0, line: 1, col: 1}
}

// ---- character helpers -------------------------------------------------------

func (l *Lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) advance() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func isWhitespace(c rune) bool { return c == ' ' || c == '\t' || c == '\r' }
func isDigit(c rune) bool      { return c >= '0' && c <= '9' }
func isNameStart(c rune) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isNameCont(c rune) bool { return isNameStart(c) || isDigit(c) }
func isSpecialParam(c rune) bool {
	return c == '@' || c == '*' || c == '#' || c == '?' || c == '-' || c == '$' || c == '!' || c == '0'
}
func isOperatorStart(c rune) bool {
	switch c {
	case '|', '&', ';', '<', '>', '(', ')':
		return true
	}
	return false
}

// ---- public API --------------------------------------------------------------

// Next returns the next token from input, consuming it.
func (l *Lexer) Next() Token {
	if len(l.queue) > 0 {
		t := l.queue[0]
		l.queue = l.queue[1:]
		return t
	}
	return l.scan()
}

// Peek returns the next token without consuming it.
func (l *Lexer) Peek() Token {
	if len(l.queue) == 0 {
		l.queue = append(l.queue, l.scan())
	}
	return l.queue[0]
}

// RegisterHeredoc registers a pending heredoc to be read after the next newline.
func (l *Lexer) RegisterHeredoc(delim string, strip bool, out *string) {
	l.pendingHeredocs = append(l.pendingHeredocs, &HeredocPending{
		Delim: delim,
		Strip: strip,
		Out:   out,
	})
}

// fillPendingHeredocs reads all queued heredoc bodies from the current position.
func (l *Lexer) fillPendingHeredocs() {
	for _, h := range l.pendingHeredocs {
		*h.Out = l.ReadHeredoc(h.Delim, h.Strip)
	}
	l.pendingHeredocs = nil
}

// ReadLine reads one raw line from the current position (used for heredocs).
// It does NOT lex tokens; it reads character-by-character up to (and including) \n.
func (l *Lexer) ReadLine() (string, bool) {
	if l.pos >= len(l.src) {
		return "", true
	}
	var sb strings.Builder
	for l.pos < len(l.src) {
		ch := l.advance()
		if ch == '\n' {
			break
		}
		sb.WriteRune(ch)
	}
	return sb.String(), false
}

// ReadHeredoc reads lines from the current position until it finds `delim`
// alone on a line (possibly after stripping leading tabs when strip==true).
// Returns the heredoc body (without the final delimiter line).
func (l *Lexer) ReadHeredoc(delim string, strip bool) string {
	var sb strings.Builder
	for {
		if l.pos >= len(l.src) {
			break
		}
		line, eof := l.ReadLine()
		if eof {
			break
		}
		// Strip leading tabs for <<-
		check := line
		if strip {
			check = strings.TrimLeft(line, "\t")
		}
		if check == delim {
			break
		}
		if strip {
			sb.WriteString(check)
		} else {
			sb.WriteString(line)
		}
		sb.WriteRune('\n')
	}
	return sb.String()
}

// ---- scanner -----------------------------------------------------------------

func (l *Lexer) tok(typ TokenType, val string, line, col int) Token {
	return Token{Type: typ, Val: val, Line: line, Col: col}
}

// scan reads the next token.
func (l *Lexer) scan() Token {
	// Skip non-newline whitespace
	for isWhitespace(l.peek()) {
		l.advance()
	}

	if l.pos >= len(l.src) {
		return l.tok(TokEOF, "", l.line, l.col)
	}

	line, col := l.line, l.col
	ch := l.peek()

	// Comments
	if ch == '#' {
		for l.peek() != '\n' && l.peek() != 0 {
			l.advance()
		}
		return l.scan()
	}

	// Newline — fill any pending heredocs before returning the newline token
	if ch == '\n' {
		l.advance()
		tok := l.tok(TokNewline, "\n", line, col)
		// Fill pending heredoc bodies immediately after the newline
		if len(l.pendingHeredocs) > 0 {
			l.fillPendingHeredocs()
		}
		return tok
	}

	// IO Number: digit(s) immediately followed (no space) by < or >
	if isDigit(ch) {
		j := l.pos
		for j < len(l.src) && isDigit(l.src[j]) {
			j++
		}
		if j < len(l.src) && (l.src[j] == '<' || l.src[j] == '>') {
			var sb strings.Builder
			for isDigit(l.peek()) {
				sb.WriteRune(l.advance())
			}
			return l.tok(TokIONum, sb.String(), line, col)
		}
	}

	// Operators
	switch ch {
	case '(':
		l.advance()
		return l.tok(TokLParen, "(", line, col)
	case ')':
		l.advance()
		return l.tok(TokRParen, ")", line, col)
	case ';':
		l.advance()
		if l.peek() == ';' {
			l.advance()
			return l.tok(TokDSemi, ";;", line, col)
		}
		return l.tok(TokSemi, ";", line, col)
	case '&':
		l.advance()
		if l.peek() == '&' {
			l.advance()
			return l.tok(TokAndIf, "&&", line, col)
		}
		return l.tok(TokAmp, "&", line, col)
	case '|':
		l.advance()
		if l.peek() == '|' {
			l.advance()
			return l.tok(TokOrIf, "||", line, col)
		}
		return l.tok(TokPipe, "|", line, col)
	case '<':
		l.advance()
		switch l.peek() {
		case '<':
			l.advance()
			if l.peek() == '-' {
				l.advance()
				return l.tok(TokDLessDash, "<<-", line, col)
			}
			return l.tok(TokDLess, "<<", line, col)
		case '&':
			l.advance()
			return l.tok(TokLessAnd, "<&", line, col)
		case '>':
			l.advance()
			return l.tok(TokLessGreat, "<>", line, col)
		}
		return l.tok(TokLess, "<", line, col)
	case '>':
		l.advance()
		switch l.peek() {
		case '>':
			l.advance()
			return l.tok(TokDGreat, ">>", line, col)
		case '&':
			l.advance()
			return l.tok(TokGreatAnd, ">&", line, col)
		case '|':
			l.advance()
			return l.tok(TokClobber, ">|", line, col)
		}
		return l.tok(TokGreat, ">", line, col)
	}

	// General word
	return l.scanWord(line, col)
}

// scanWord reads a shell word, preserving all internal quoting/expansion syntax.
func (l *Lexer) scanWord(line, col int) Token {
	var sb strings.Builder
	for {
		ch := l.peek()
		if ch == 0 || isWhitespace(ch) || ch == '\n' {
			break
		}
		if isOperatorStart(ch) {
			break
		}
		switch ch {
		case '\\':
			l.advance()
			if l.peek() == '\n' {
				l.advance() // line continuation — skip both
				continue
			}
			sb.WriteRune('\\')
			if l.peek() != 0 {
				sb.WriteRune(l.advance())
			}
		case '\'':
			l.scanSingleQuote(&sb)
		case '"':
			l.scanDoubleQuote(&sb)
		case '`':
			l.scanBacktick(&sb)
		case '$':
			sb.WriteRune(l.advance())
			l.scanDollar(&sb)
		default:
			sb.WriteRune(l.advance())
		}
	}
	return l.tok(TokWord, sb.String(), line, col)
}

// scanSingleQuote reads '...' including the surrounding quotes.
func (l *Lexer) scanSingleQuote(sb *strings.Builder) {
	sb.WriteRune(l.advance()) // opening '
	for l.peek() != 0 && l.peek() != '\'' {
		sb.WriteRune(l.advance())
	}
	if l.peek() == '\'' {
		sb.WriteRune(l.advance())
	}
}

// scanDoubleQuote reads "..." including surrounding quotes.
func (l *Lexer) scanDoubleQuote(sb *strings.Builder) {
	sb.WriteRune(l.advance()) // opening "
	for l.peek() != 0 && l.peek() != '"' {
		ch := l.peek()
		switch ch {
		case '\\':
			sb.WriteRune(l.advance())
			if l.peek() != 0 {
				sb.WriteRune(l.advance())
			}
		case '$':
			sb.WriteRune(l.advance())
			l.scanDollar(sb)
		case '`':
			l.scanBacktick(sb)
		default:
			sb.WriteRune(l.advance())
		}
	}
	if l.peek() == '"' {
		sb.WriteRune(l.advance())
	}
}

// scanBacktick reads `...` including surrounding backticks.
func (l *Lexer) scanBacktick(sb *strings.Builder) {
	sb.WriteRune(l.advance()) // opening `
	for l.peek() != 0 && l.peek() != '`' {
		ch := l.peek()
		if ch == '\\' {
			sb.WriteRune(l.advance())
			if l.peek() != 0 {
				sb.WriteRune(l.advance())
			}
		} else {
			sb.WriteRune(l.advance())
		}
	}
	if l.peek() == '`' {
		sb.WriteRune(l.advance())
	}
}

// scanDollar reads the part after $: ${...}, $(...), $((...)), $NAME, $0, $@, etc.
func (l *Lexer) scanDollar(sb *strings.Builder) {
	ch := l.peek()
	switch {
	case ch == '{':
		// ${...} — balanced braces (may contain nested expansions)
		sb.WriteRune(l.advance())
		depth := 1
		for depth > 0 && l.peek() != 0 {
			c := l.peek()
			switch c {
			case '{':
				depth++
				sb.WriteRune(l.advance())
			case '}':
				depth--
				sb.WriteRune(l.advance())
			case '\'':
				l.scanSingleQuote(sb)
			case '"':
				l.scanDoubleQuote(sb)
			case '`':
				l.scanBacktick(sb)
			case '\\':
				sb.WriteRune(l.advance())
				if l.peek() != 0 {
					sb.WriteRune(l.advance())
				}
			case '$':
				sb.WriteRune(l.advance())
				l.scanDollar(sb)
			default:
				sb.WriteRune(l.advance())
			}
		}

	case ch == '(':
		l.advance()
		sb.WriteRune('(')
		if l.peek() == '(' {
			// $(( arith ))
			sb.WriteRune(l.advance())
			depth := 2
			for depth > 0 && l.peek() != 0 {
				c := l.advance()
				sb.WriteRune(c)
				if c == '(' {
					depth++
				} else if c == ')' {
					depth--
				}
			}
		} else {
			// $( cmd )
			depth := 1
			for depth > 0 && l.peek() != 0 {
				c := l.peek()
				switch c {
				case '(':
					depth++
					sb.WriteRune(l.advance())
				case ')':
					depth--
					sb.WriteRune(l.advance())
				case '\'':
					l.scanSingleQuote(sb)
				case '"':
					l.scanDoubleQuote(sb)
				case '`':
					l.scanBacktick(sb)
				case '\\':
					sb.WriteRune(l.advance())
					if l.peek() != 0 {
						sb.WriteRune(l.advance())
					}
				case '$':
					sb.WriteRune(l.advance())
					l.scanDollar(sb)
				default:
					sb.WriteRune(l.advance())
				}
			}
		}

	case isNameStart(ch):
		for isNameCont(l.peek()) {
			sb.WriteRune(l.advance())
		}

	case isDigit(ch):
		sb.WriteRune(l.advance())

	case isSpecialParam(ch):
		sb.WriteRune(l.advance())
	}
}
