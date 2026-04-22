// Package parser implements a recursive-descent POSIX shell parser for vish.
package parser

import (
	"fmt"
	"strings"

	"github.com/omakoto/vishc/lexer"
)

// ─── Parser ───────────────────────────────────────────────────────────────────

// Parser parses a token stream into a shell AST.
type Parser struct {
	lex       *lexer.Lexer
	tok       lexer.Token // current look-ahead
	multiLine bool        // allow reading more input lines (interactive)
}

// New creates a Parser from an input string.
func New(input string) *Parser {
	p := &Parser{lex: lexer.New(input)}
	p.advance()
	return p
}

// NewMultiLine creates a Parser that expects multi-line interactive input.
func NewMultiLine(input string) *Parser {
	p := New(input)
	p.multiLine = true
	return p
}

func (p *Parser) advance() {
	p.tok = p.lex.Next()
}

func (p *Parser) eat(tt lexer.TokenType) (lexer.Token, error) {
	t := p.tok
	if t.Type != tt {
		return t, fmt.Errorf("line %d: expected %s, got %s (%q)", t.Line, tt, t.Type, t.Val)
	}
	p.advance()
	return t, nil
}

// skipNewlines skips over newline tokens.
func (p *Parser) skipNewlines() {
	for p.tok.Type == lexer.TokNewline {
		p.advance()
	}
}

// ─── Keyword helpers ──────────────────────────────────────────────────────────

var keywords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "fi": true,
	"while": true, "until": true, "do": true, "done": true,
	"for": true, "in": true, "case": true, "esac": true,
	"function": true, "{": true, "}": true, "!": true,
}

// isWord reports whether the current token is a word (or keyword-as-word).
func (p *Parser) isWord() bool {
	return p.tok.Type == lexer.TokWord
}

// isKeyword reports whether the current token is the word kw.
func (p *Parser) isKeyword(kw string) bool {
	return p.tok.Type == lexer.TokWord && p.tok.Val == kw
}

// isAssignment reports whether a raw word looks like NAME=value.
func isAssignment(w string) bool {
	for i, c := range w {
		if i == 0 {
			if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') {
				return false
			}
		} else if c == '=' {
			return i > 0
		} else if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') {
			return false
		}
	}
	return false
}

// isUnquoted reports whether raw word w contains no quoting characters.
// Used for keyword recognition: a quoted "if" is not the if keyword.
func isUnquoted(w string) bool {
	for _, c := range w {
		switch c {
		case '\'', '"', '\\', '$', '`':
			return false
		}
	}
	return true
}

// ─── Top-level parse ──────────────────────────────────────────────────────────

// ParseFile parses the complete input and returns a File AST.
func ParseFile(input string) (*File, error) {
	p := New(input)
	return p.parseFile()
}

// ParseFileMultiLine parses a complete file; set multiLine for interactive use.
func ParseFileMultiLine(input string) (*File, error) {
	p := NewMultiLine(input)
	return p.parseFile()
}

func (p *Parser) parseFile() (*File, error) {
	p.skipNewlines()
	f := &File{}
	for p.tok.Type != lexer.TokEOF {
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			f.Stmts = append(f.Stmts, stmt)
		}
		// consume optional separator after last item
		p.skipNewlines()
	}
	return f, nil
}

// ─── Statement / list parsing ─────────────────────────────────────────────────

// parseStmt parses one list item (an and-or chain with an optional trailing sep).
func (p *Parser) parseStmt() (*Stmt, error) {
	line := p.tok.Line
	ao, err := p.parseAndOr()
	if err != nil {
		return nil, err
	}
	if ao == nil {
		return nil, nil
	}
	stmt := &Stmt{AndOr: ao, Line: line}

	// Consume separator: & / ; / newline
	switch p.tok.Type {
	case lexer.TokAmp:
		stmt.Background = true
		p.advance()
	case lexer.TokSemi:
		p.advance()
	case lexer.TokNewline:
		p.advance()
	}
	return stmt, nil
}

// parseList parses a list of statements terminated by one of the stop words.
// stopWords are keyword strings that indicate the list has ended.
func (p *Parser) parseList(stopWords ...string) ([]*Stmt, error) {
	var stmts []*Stmt
	p.skipNewlines()
	for {
		// Check for EOF or stop keyword
		if p.tok.Type == lexer.TokEOF || p.tok.Type == lexer.TokRParen {
			break
		}
		if p.tok.Type == lexer.TokWord && isUnquoted(p.tok.Val) {
			stop := false
			for _, sw := range stopWords {
				if p.tok.Val == sw {
					stop = true
					break
				}
			}
			if stop {
				break
			}
		}

		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if stmt == nil {
			break
		}
		stmts = append(stmts, stmt)
		p.skipNewlines()
	}
	return stmts, nil
}

// ─── And-Or ───────────────────────────────────────────────────────────────────

func (p *Parser) parseAndOr() (*AndOr, error) {
	first, err := p.parsePipeline()
	if err != nil {
		return nil, err
	}
	if first == nil {
		return nil, nil
	}

	ao := &AndOr{}
	ao.Pipelines = append(ao.Pipelines, &AndOrPipeline{Op: "", Pipeline: first})

	for p.tok.Type == lexer.TokAndIf || p.tok.Type == lexer.TokOrIf {
		op := p.tok.Val
		p.advance()
		p.skipNewlines()
		pl, err := p.parsePipeline()
		if err != nil {
			return nil, err
		}
		if pl == nil {
			return nil, fmt.Errorf("expected pipeline after %s", op)
		}
		ao.Pipelines = append(ao.Pipelines, &AndOrPipeline{Op: op, Pipeline: pl})
	}
	return ao, nil
}

// ─── Pipeline ─────────────────────────────────────────────────────────────────

func (p *Parser) parsePipeline() (*Pipeline, error) {
	negated := false
	if p.isKeyword("!") {
		negated = true
		p.advance()
	}

	cmd, err := p.parseCmd()
	if err != nil {
		return nil, err
	}
	if cmd == nil {
		if negated {
			return nil, fmt.Errorf("expected command after '!'")
		}
		return nil, nil
	}

	pl := &Pipeline{Negated: negated}
	pl.Cmds = append(pl.Cmds, cmd)

	for p.tok.Type == lexer.TokPipe {
		p.advance()
		p.skipNewlines()
		next, err := p.parseCmd()
		if err != nil {
			return nil, err
		}
		if next == nil {
			return nil, fmt.Errorf("expected command after |")
		}
		pl.Cmds = append(pl.Cmds, next)
	}
	return pl, nil
}

// ─── Command ──────────────────────────────────────────────────────────────────

func (p *Parser) parseCmd() (*Cmd, error) {
	// Check for compound commands
	if p.tok.Type == lexer.TokWord && isUnquoted(p.tok.Val) {
		switch p.tok.Val {
		case "if":
			return p.parseIf()
		case "while":
			return p.parseWhile("while")
		case "until":
			return p.parseWhile("until")
		case "for":
			return p.parseFor()
		case "case":
			return p.parseCase()
		case "{":
			return p.parseBrace()
		}
	}
	if p.tok.Type == lexer.TokLParen {
		return p.parseSubshell()
	}

	// Check for function definition: NAME()
	if p.tok.Type == lexer.TokWord && !keywords[p.tok.Val] {
		if p.looksLikeFuncDef() {
			return p.parseFuncDef()
		}
	}

	// Simple command
	return p.parseSimpleCmd()
}

// looksLikeFuncDef peeks ahead: WORD ( ) — a function definition.
func (p *Parser) looksLikeFuncDef() bool {
	// We need WORD followed by ( then ). Use a saved position.
	// The lexer queues one token; we need two peeks. Use a temp copy.
	// Simple heuristic: push into lexer queue. We do it properly below.
	saved := p.tok
	_ = saved
	// Peek the next token in the lexer
	next := p.lex.Peek()
	return next.Type == lexer.TokLParen
}

// ─── Function definition ──────────────────────────────────────────────────────

func (p *Parser) parseFuncDef() (*Cmd, error) {
	line := p.tok.Line
	name := p.tok.Val
	p.advance() // consume name
	if _, err := p.eat(lexer.TokLParen); err != nil {
		return nil, err
	}
	p.skipNewlines()
	if _, err := p.eat(lexer.TokRParen); err != nil {
		return nil, err
	}
	p.skipNewlines()

	body, err := p.parseCompoundCmd()
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, fmt.Errorf("line %d: function %s: expected compound command body", line, name)
	}

	// Optional redirections
	redirs, err := p.parseRedirList()
	if err != nil {
		return nil, err
	}
	body.Redirs = append(body.Redirs, redirs...)

	return &Cmd{
		FuncDef: &FuncDef{Name: name, Body: body},
		Line:    line,
	}, nil
}

// parseCompoundCmd parses a compound command (if/while/for/case/brace/subshell).
func (p *Parser) parseCompoundCmd() (*Cmd, error) {
	if p.tok.Type == lexer.TokWord && isUnquoted(p.tok.Val) {
		switch p.tok.Val {
		case "if":
			return p.parseIf()
		case "while":
			return p.parseWhile("while")
		case "until":
			return p.parseWhile("until")
		case "for":
			return p.parseFor()
		case "case":
			return p.parseCase()
		case "{":
			return p.parseBrace()
		}
	}
	if p.tok.Type == lexer.TokLParen {
		return p.parseSubshell()
	}
	return nil, nil
}

// ─── Compound commands ────────────────────────────────────────────────────────

func (p *Parser) parseIf() (*Cmd, error) {
	line := p.tok.Line
	p.advance() // consume "if"
	p.skipNewlines()

	cond, err := p.parseList("then")
	if err != nil {
		return nil, err
	}
	if _, err := p.eatKeyword("then"); err != nil {
		return nil, err
	}
	p.skipNewlines()
	then, err := p.parseList("fi", "else", "elif")
	if err != nil {
		return nil, err
	}

	ic := &IfClause{Cond: cond, Then: then}

	for p.isKeyword("elif") {
		p.advance()
		p.skipNewlines()
		econd, err := p.parseList("then")
		if err != nil {
			return nil, err
		}
		if _, err := p.eatKeyword("then"); err != nil {
			return nil, err
		}
		p.skipNewlines()
		ethen, err := p.parseList("fi", "else", "elif")
		if err != nil {
			return nil, err
		}
		ic.Elifs = append(ic.Elifs, ElifClause{Cond: econd, Then: ethen})
	}

	if p.isKeyword("else") {
		p.advance()
		p.skipNewlines()
		elsebody, err := p.parseList("fi")
		if err != nil {
			return nil, err
		}
		ic.Else = elsebody
	}

	if _, err := p.eatKeyword("fi"); err != nil {
		return nil, err
	}

	return &Cmd{If: ic, Line: line}, nil
}

func (p *Parser) parseWhile(kind string) (*Cmd, error) {
	line := p.tok.Line
	p.advance() // consume "while" or "until"
	p.skipNewlines()

	cond, err := p.parseList("do")
	if err != nil {
		return nil, err
	}
	if _, err := p.eatKeyword("do"); err != nil {
		return nil, err
	}
	p.skipNewlines()

	body, err := p.parseList("done")
	if err != nil {
		return nil, err
	}
	if _, err := p.eatKeyword("done"); err != nil {
		return nil, err
	}

	wc := &WhileClause{Kind: kind, Cond: cond, Body: body}
	if kind == "while" {
		return &Cmd{While: wc, Line: line}, nil
	}
	return &Cmd{Until: wc, Line: line}, nil
}

func (p *Parser) parseFor() (*Cmd, error) {
	line := p.tok.Line
	p.advance() // consume "for"
	p.skipNewlines()

	if !p.isWord() {
		return nil, fmt.Errorf("line %d: expected variable name after for", line)
	}
	varName := p.tok.Val
	p.advance()

	var items []string
	// Optional: in word...
	if p.isKeyword("in") {
		p.advance()
		for p.isWord() {
			items = append(items, p.tok.Val)
			p.advance()
		}
	}
	// Consume newlines/semicolons before do
	for p.tok.Type == lexer.TokNewline || p.tok.Type == lexer.TokSemi {
		p.advance()
	}
	p.skipNewlines()

	if _, err := p.eatKeyword("do"); err != nil {
		return nil, err
	}
	p.skipNewlines()

	body, err := p.parseList("done")
	if err != nil {
		return nil, err
	}
	if _, err := p.eatKeyword("done"); err != nil {
		return nil, err
	}

	fc := &ForClause{Var: varName, Items: items, Body: body}
	return &Cmd{For: fc, Line: line}, nil
}

func (p *Parser) parseCase() (*Cmd, error) {
	line := p.tok.Line
	p.advance() // consume "case"
	p.skipNewlines()

	if !p.isWord() {
		return nil, fmt.Errorf("line %d: expected word after case", line)
	}
	word := p.tok.Val
	p.advance()

	// Skip newlines then "in"
	p.skipNewlines()
	if _, err := p.eatKeyword("in"); err != nil {
		return nil, err
	}
	p.skipNewlines()

	cc := &CaseClause{Word: word}

	for !p.isKeyword("esac") && p.tok.Type != lexer.TokEOF {
		item, err := p.parseCaseItem()
		if err != nil {
			return nil, err
		}
		if item == nil {
			break
		}
		cc.Items = append(cc.Items, item)
		p.skipNewlines()
	}

	if _, err := p.eatKeyword("esac"); err != nil {
		return nil, err
	}
	return &Cmd{Case: cc, Line: line}, nil
}

func (p *Parser) parseCaseItem() (*CaseItem, error) {
	p.skipNewlines()
	if p.isKeyword("esac") || p.tok.Type == lexer.TokEOF {
		return nil, nil
	}

	// Optional leading (
	if p.tok.Type == lexer.TokLParen {
		p.advance()
	}

	var patterns []string
	for p.isWord() {
		patterns = append(patterns, p.tok.Val)
		p.advance()
		if p.tok.Type == lexer.TokPipe {
			p.advance() // | between patterns
		} else {
			break
		}
	}
	if _, err := p.eat(lexer.TokRParen); err != nil {
		return nil, err
	}
	p.skipNewlines()

	body, err := p.parseList("esac")
	if err != nil {
		return nil, err
	}

	// ;; terminates case items
	if p.tok.Type == lexer.TokDSemi {
		p.advance()
	}

	return &CaseItem{Patterns: patterns, Body: body}, nil
}

func (p *Parser) parseBrace() (*Cmd, error) {
	line := p.tok.Line
	p.advance() // consume "{"
	p.skipNewlines()

	body, err := p.parseList("}")
	if err != nil {
		return nil, err
	}
	if _, err := p.eatKeyword("}"); err != nil {
		return nil, err
	}

	return &Cmd{Brace: &BraceGroup{Body: body}, Line: line}, nil
}

func (p *Parser) parseSubshell() (*Cmd, error) {
	line := p.tok.Line
	p.advance() // consume (
	p.skipNewlines()

	body, err := p.parseList()
	if err != nil {
		return nil, err
	}
	if _, err := p.eat(lexer.TokRParen); err != nil {
		return nil, err
	}

	return &Cmd{Subshell: &Subshell{Body: body}, Line: line}, nil
}

// ─── Simple command ───────────────────────────────────────────────────────────

func (p *Parser) parseSimpleCmd() (*Cmd, error) {
	line := p.tok.Line
	sc := &SimpleCmd{}

	// Leading redirections and assignments
	for {
		if p.tok.Type == lexer.TokIONum || p.tok.Type == lexer.TokLess ||
			p.tok.Type == lexer.TokGreat || p.tok.Type == lexer.TokDLess ||
			p.tok.Type == lexer.TokDGreat || p.tok.Type == lexer.TokLessAnd ||
			p.tok.Type == lexer.TokGreatAnd || p.tok.Type == lexer.TokLessGreat ||
			p.tok.Type == lexer.TokDLessDash || p.tok.Type == lexer.TokClobber {
			r, err := p.parseRedir()
			if err != nil {
				return nil, err
			}
			sc.Redirs = append(sc.Redirs, r)
			continue
		}
		if p.isWord() && isAssignment(p.tok.Val) && len(sc.Args) == 0 {
			sc.Assigns = append(sc.Assigns, p.tok.Val)
			p.advance()
			continue
		}
		break
	}

	// Command name and arguments
	for {
		if p.tok.Type == lexer.TokIONum || p.tok.Type == lexer.TokLess ||
			p.tok.Type == lexer.TokGreat || p.tok.Type == lexer.TokDLess ||
			p.tok.Type == lexer.TokDGreat || p.tok.Type == lexer.TokLessAnd ||
			p.tok.Type == lexer.TokGreatAnd || p.tok.Type == lexer.TokLessGreat ||
			p.tok.Type == lexer.TokDLessDash || p.tok.Type == lexer.TokClobber {
			r, err := p.parseRedir()
			if err != nil {
				return nil, err
			}
			sc.Redirs = append(sc.Redirs, r)
			continue
		}
		if !p.isWord() {
			break
		}
		// Stop at keywords (unless we have at least one arg already or assignment)
		if len(sc.Args) == 0 && isUnquoted(p.tok.Val) && keywords[p.tok.Val] {
			break
		}
		sc.Args = append(sc.Args, p.tok.Val)
		p.advance()
	}

	if len(sc.Assigns) == 0 && len(sc.Args) == 0 && len(sc.Redirs) == 0 {
		return nil, nil
	}

	return &Cmd{Simple: sc, Line: line}, nil
}

// ─── Redirections ─────────────────────────────────────────────────────────────

func (p *Parser) parseRedirList() ([]*Redir, error) {
	var redirs []*Redir
	for {
		if p.tok.Type != lexer.TokIONum && !p.tok.IsRedirOp() {
			break
		}
		r, err := p.parseRedir()
		if err != nil {
			return nil, err
		}
		redirs = append(redirs, r)
	}
	return redirs, nil
}

func (p *Parser) parseRedir() (*Redir, error) {
	r := &Redir{}

	// Optional IO Number
	if p.tok.Type == lexer.TokIONum {
		n := 0
		for _, c := range p.tok.Val {
			n = n*10 + int(c-'0')
		}
		r.Fd = n
		p.advance()
	} else {
		r.Fd = -1 // will be defaulted based on op
	}

	op := p.tok.Val
	r.Op = op
	p.advance()

	// Set default FD
	if r.Fd < 0 {
		switch op {
		case "<", "<<", "<<-", "<&", "<>":
			r.Fd = 0
		default:
			r.Fd = 1
		}
	}

	if !p.isWord() {
		return nil, fmt.Errorf("line %d: expected word after %s", p.tok.Line, op)
	}
	word := p.tok.Val

	switch op {
	case "<<", "<<-":
		// Heredoc: register BEFORE consuming the delimiter token.
		// This ensures the lexer's next scan (which will see '\n') fills the body.
		r.Word = word
		r.HereQuote = isHeredocQuoted(word)
		delim := stripHeredocQuotes(word)
		p.lex.RegisterHeredoc(delim, op == "<<-", &r.HereDoc)
		p.advance() // consume delimiter word; next scan will hit '\n' → fill heredoc
	default:
		r.Word = word
		p.advance()
	}

	return r, nil
}

// isHeredocQuoted reports whether a heredoc delimiter word contains any quoting.
func isHeredocQuoted(word string) bool {
	for _, c := range word {
		if c == '\'' || c == '"' || c == '\\' {
			return true
		}
	}
	return false
}

// ReadHeredocs reads heredoc bodies from the lexer for all pending heredocs
// in the command tree. Call this after each line-complete parsing.
func ReadHeredocs(lex *lexer.Lexer, stmts []*Stmt) {
	for _, s := range stmts {
		readHeredocsAndOr(lex, s.AndOr)
	}
}

func readHeredocsAndOr(lex *lexer.Lexer, ao *AndOr) {
	if ao == nil {
		return
	}
	for _, ap := range ao.Pipelines {
		readHeredocsPipeline(lex, ap.Pipeline)
	}
}

func readHeredocsPipeline(lex *lexer.Lexer, pl *Pipeline) {
	if pl == nil {
		return
	}
	for _, cmd := range pl.Cmds {
		readHeredocsCmd(lex, cmd)
	}
}

func readHeredocsCmd(lex *lexer.Lexer, cmd *Cmd) {
	if cmd == nil {
		return
	}
	// Collect heredoc redirs from the command's own redir list
	for _, r := range cmd.Redirs {
		fillHeredoc(lex, r)
	}
	if cmd.Simple != nil {
		for _, r := range cmd.Simple.Redirs {
			fillHeredoc(lex, r)
		}
	}
}

func fillHeredoc(lex *lexer.Lexer, r *Redir) {
	if r.Op != "<<" && r.Op != "<<-" {
		return
	}
	if r.HereDoc != "" {
		return // already filled
	}
	delim := stripHeredocQuotes(r.Word)
	r.HereDoc = lex.ReadHeredoc(delim, r.Op == "<<-")
}

// stripHeredocQuotes removes quoting characters from a heredoc delimiter.
func stripHeredocQuotes(word string) string {
	var sb strings.Builder
	inSingle := false
	for _, c := range word {
		if inSingle {
			if c == '\'' {
				inSingle = false
			} else {
				sb.WriteRune(c)
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"', '\\':
			// skip quoting chars
		default:
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// eatKeyword consumes a keyword word token.
func (p *Parser) eatKeyword(kw string) (lexer.Token, error) {
	t := p.tok
	if t.Type != lexer.TokWord || t.Val != kw {
		return t, fmt.Errorf("line %d: expected %q, got %q", t.Line, kw, t.Val)
	}
	p.advance()
	return t, nil
}
