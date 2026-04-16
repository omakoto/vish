// Package parser provides shell AST types for the vish shell.
package parser

// ─── Node hierarchy ──────────────────────────────────────────────────────────

// File is the root of the parse tree.
type File struct {
	Stmts []*Stmt // top-level list items
}

// Stmt is one element of a list: an and-or chain that may run in background.
type Stmt struct {
	AndOr      *AndOr
	Background bool // terminated by & (async)
	Line       int
}

// AndOr is a chain of pipelines connected by && and ||.
type AndOr struct {
	Pipelines []*AndOrPipeline
}

// AndOrPipeline pairs a logical operator with a pipeline.
type AndOrPipeline struct {
	Op       string    // "" (first item), "&&", "||"
	Pipeline *Pipeline
}

// Pipeline is a sequence of commands connected by |.
type Pipeline struct {
	Negated bool
	Cmds    []*Cmd
}

// Cmd is a single command (simple or compound) with its own redirections.
type Cmd struct {
	// Exactly one of these is set:
	Simple   *SimpleCmd
	If       *IfClause
	While    *WhileClause  // kind="while"
	Until    *WhileClause  // kind="until"
	For      *ForClause
	Case     *CaseClause
	Subshell *Subshell
	Brace    *BraceGroup
	FuncDef  *FuncDef
	// Redirections on this command (compound commands may have them too).
	Redirs []*Redir
	Line   int
}

// ─── Simple Command ───────────────────────────────────────────────────────────

// SimpleCmd is a plain command with optional leading assignments and I/O redirs.
type SimpleCmd struct {
	Assigns []string // raw "NAME=VALUE" strings (before the command name)
	Args    []string // raw word strings; Args[0] is the command name (if any)
	Redirs  []*Redir
}

// ─── Redirections ─────────────────────────────────────────────────────────────

// Redir is an I/O redirection.
type Redir struct {
	Fd        int    // file descriptor number (default depends on Op)
	Op        string // "<", ">", ">>", "<&", ">&", "<>", ">|", "<<", "<<-"
	Word      string // target (raw word); for heredocs, the delimiter word
	HereDoc   string // heredoc body (populated post-parse)
	HereQuote bool   // heredoc delimiter was quoted → no expansion in body
}

// ─── Compound Commands ────────────────────────────────────────────────────────

// IfClause: if cond; then body; [elif cond; then body;]* [else body;] fi
type IfClause struct {
	Cond  []*Stmt
	Then  []*Stmt
	Elifs []ElifClause
	Else  []*Stmt // nil if absent
}

// ElifClause: elif cond; then body;
type ElifClause struct {
	Cond []*Stmt
	Then []*Stmt
}

// WhileClause covers both "while" and "until".
// Kind is "while" or "until".
type WhileClause struct {
	Kind string
	Cond []*Stmt
	Body []*Stmt
}

// ForClause: for Var in Items; do Body; done
// If Items is nil, iterate over positional params.
type ForClause struct {
	Var   string
	Items []string // raw words
	Body  []*Stmt
}

// CaseClause: case Word in Items... esac
type CaseClause struct {
	Word  string // raw word
	Items []*CaseItem
}

// CaseItem: (Patterns) Body ;;
type CaseItem struct {
	Patterns []string // raw words
	Body     []*Stmt
}

// Subshell: ( List )
type Subshell struct {
	Body []*Stmt
}

// BraceGroup: { List }
type BraceGroup struct {
	Body []*Stmt
}

// ─── Function Definition ──────────────────────────────────────────────────────

// FuncDef: name() compound_command [redirect_list]
type FuncDef struct {
	Name string
	Body *Cmd // must be a compound command
}
