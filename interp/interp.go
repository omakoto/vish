// Package interp — main shell interpreter for vish.
package interp

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/omakoto/vishc/parser"
)

// ─── exitSignal is used to implement the exit builtin via panic/recover ───────

type exitSignal struct{ code int }

// ─── Job tracks a background process ─────────────────────────────────────────

type Job struct {
	Pid  int
	proc *os.Process
}

func (j *Job) Wait() {
	if j.proc != nil {
		j.proc.Wait()
	}
}

// ─── Shell is the main interpreter state ─────────────────────────────────────

type Shell struct {
	Env         *Env
	Positionals []string // $0, $1, ...
	Funcs       map[string]*parser.FuncDef
	Aliases     map[string]string
	Traps       map[string]string // signal/spec → handler code
	Jobs        []*Job

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	CWD           string
	IsSubshell    bool
	IsInteractive bool

	// Shell options
	OptExit    bool // -e
	OptNounset bool // -u
	OptXtrace  bool // -x
	OptNoexec  bool // -n
	OptNoglob  bool // -f

	// Loop control
	BreakLevel    int
	ContinueLevel int

	// Function return
	ReturnValue int
	Returning   bool

	LastStatus int
	LastBgPID  int
}

// New creates a new top-level Shell.
func New() *Shell {
	cwd, _ := os.Getwd()
	sh := &Shell{
		Env:         newEnv(),
		Positionals: []string{"vish"},
		Funcs:       make(map[string]*parser.FuncDef),
		Aliases:     make(map[string]string),
		Traps:       make(map[string]string),
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		CWD:         cwd,
	}
	// Set $PWD
	_ = sh.Env.Set("PWD", cwd)

	// Handle signals
	go sh.handleSignals()
	return sh
}

// subshell creates a copy of the shell for subshell execution.
func (sh *Shell) subshell() *Shell {
	sub := &Shell{
		Env:         sh.Env.Clone(),
		Positionals: append([]string{}, sh.Positionals...),
		Funcs:       make(map[string]*parser.FuncDef),
		Aliases:     make(map[string]string),
		Traps:       make(map[string]string),
		Stdin:       sh.Stdin,
		Stdout:      sh.Stdout,
		Stderr:      sh.Stderr,
		CWD:         sh.CWD,
		IsSubshell:  true,
		OptExit:     sh.OptExit,
		OptNounset:  sh.OptNounset,
		OptXtrace:   sh.OptXtrace,
		OptNoexec:   sh.OptNoexec,
		OptNoglob:   sh.OptNoglob,
	}
	// Copy functions
	for k, v := range sh.Funcs {
		sub.Funcs[k] = v
	}
	// Copy aliases
	for k, v := range sh.Aliases {
		sub.Aliases[k] = v
	}
	return sub
}

// handleSignals sets up signal handling.
func (sh *Shell) handleSignals() {
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	for sig := range sigs {
		sigName := sigToName(sig)
		if handler, ok := sh.Traps[sigName]; ok && handler != "" {
			sh.RunString(handler)
		}
		if handler, ok := sh.Traps["0"]; ok && handler != "" && sig == syscall.SIGTERM {
			sh.RunString(handler)
		}
	}
}

func sigToName(sig os.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "INT"
	case syscall.SIGTERM:
		return "TERM"
	case syscall.SIGHUP:
		return "HUP"
	default:
		return fmt.Sprintf("%d", sig)
	}
}

// ─── Entry points ─────────────────────────────────────────────────────────────

// RunString parses and executes a shell script string. Returns the exit status.
func (sh *Shell) RunString(input string) (status int) {
	defer func() {
		if r := recover(); r != nil {
			if es, ok := r.(exitSignal); ok {
				if sh.IsSubshell {
					// In subshells, exitSignal sets the exit status rather than exiting the process
					sh.LastStatus = es.code
					status = es.code
					return
				}
				runExitTrap(sh)
				os.Exit(es.code)
			}
			panic(r)
		}
	}()

	f, err := parser.ParseFile(input)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "vish: parse error: %v\n", err)
		return 1
	}

	for _, stmt := range f.Stmts {
		sh.execStmt(stmt)
		if sh.Returning {
			break
		}
	}
	return sh.LastStatus
}

// RunFile reads and executes a script file.
func (sh *Shell) RunFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
		return 1
	}
	return sh.RunString(string(data))
}

// ─── Statement execution ──────────────────────────────────────────────────────

// execStmts executes a list of statements.
func (sh *Shell) execStmts(stmts []*parser.Stmt) {
	for _, s := range stmts {
		sh.execStmt(s)
		if sh.BreakLevel > 0 || sh.ContinueLevel > 0 || sh.Returning {
			return
		}
		if sh.OptExit && sh.LastStatus != 0 {
			runExitTrap(sh)
			panic(exitSignal{code: sh.LastStatus}) // caught by RunString/RunInteractive
		}
	}
}

// execCond executes a condition list (if/elif/while/until).
// POSIX: the errexit (-e) flag must not apply to compound-command conditions.
func (sh *Shell) execCond(stmts []*parser.Stmt) {
	saved := sh.OptExit
	sh.OptExit = false
	sh.execStmts(stmts)
	sh.OptExit = saved
}

func runExitTrap(sh *Shell) {
	if handler, ok := sh.Traps["EXIT"]; ok && handler != "" {
		delete(sh.Traps, "EXIT") // prevent infinite recursion
		sh.RunString(handler)
	}
}

func (sh *Shell) execStmt(stmt *parser.Stmt) {
	if stmt == nil || stmt.AndOr == nil {
		return
	}
	if stmt.Background {
		sh.execBackground(stmt.AndOr)
		return
	}
	sh.execAndOr(stmt.AndOr)
}

func (sh *Shell) execBackground(ao *parser.AndOr) {
	// Run an and-or list in the background using a goroutine + subshell
	sub := sh.subshell()
	sub.Stdin = openNull()

	pr, pw, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(sh.Stderr, "vish: pipe: %v\n", err)
		return
	}
	_ = pr // not used for simple background

	sub.Stdout = pw

	// We need to get PID for $!. Since we're using goroutines (not fork),
	// we use the parent process PID as a placeholder. True job control
	// requires fork; we approximate here.
	pid := os.Getpid() // approximate
	sh.LastBgPID = pid
	_ = sh.Env.Set("!", fmt.Sprintf("%d", pid))

	go func() {
		defer pw.Close()
		defer pr.Close()
		sub.execAndOr(ao)
	}()

	// Register as a job (with no real process handle)
	sh.Jobs = append(sh.Jobs, &Job{Pid: pid, proc: nil})
}

func openNull() io.Reader {
	f, _ := os.Open(os.DevNull)
	return f
}

func (sh *Shell) execAndOr(ao *parser.AndOr) {
	for i, item := range ao.Pipelines {
		if i > 0 {
			if item.Op == "&&" && sh.LastStatus != 0 {
				continue
			}
			if item.Op == "||" && sh.LastStatus == 0 {
				continue
			}
		}
		sh.execPipeline(item.Pipeline)
	}
}

// ─── Pipeline ─────────────────────────────────────────────────────────────────

func (sh *Shell) execPipeline(pl *parser.Pipeline) {
	if pl == nil {
		return
	}
	if len(pl.Cmds) == 1 {
		// Fast path: single command
		sh.execCmd(pl.Cmds[0])
		if pl.Negated {
			if sh.LastStatus == 0 {
				sh.LastStatus = 1
			} else {
				sh.LastStatus = 0
			}
		}
		return
	}

	// Multi-command pipeline: connect with pipes
	procs := make([]*exec.Cmd, 0, len(pl.Cmds))
	readers := make([]*io.PipeReader, len(pl.Cmds)-1)
	writers := make([]*io.PipeWriter, len(pl.Cmds)-1)
	for i := range readers {
		readers[i], writers[i] = io.Pipe()
	}

	// We need to exec commands with pipes but also handle builtins.
	// Simple approach: for pipelines, run each segment in a goroutine.
	// Non-builtin commands get exec'd; builtins run in goroutines.
	// The pipeline exits when all goroutines finish.

	type result struct {
		idx    int
		status int
	}
	results := make(chan result, len(pl.Cmds))

	for idx, cmd := range pl.Cmds {
		cmd := cmd
		idx := idx

		// Determine stdin/stdout
		var stdin io.Reader = sh.Stdin
		var stdout io.Writer = sh.Stdout
		if idx > 0 {
			stdin = readers[idx-1]
		}
		if idx < len(pl.Cmds)-1 {
			stdout = writers[idx]
		}

		go func() {
			sub := sh.subshell()
			sub.Stdin = stdin
			sub.Stdout = stdout
			sub.execCmd(cmd)
			// Close our write end
			if idx < len(writers) {
				writers[idx].Close()
			}
			results <- result{idx: idx, status: sub.LastStatus}
		}()
	}

	// Collect results
	_ = procs
	last := 0
	for i := 0; i < len(pl.Cmds); i++ {
		r := <-results
		if r.idx == len(pl.Cmds)-1 {
			last = r.status
		}
	}
	sh.LastStatus = last
	if pl.Negated {
		if sh.LastStatus == 0 {
			sh.LastStatus = 1
		} else {
			sh.LastStatus = 0
		}
	}
}

// ─── Command dispatch ─────────────────────────────────────────────────────────

func (sh *Shell) execCmd(cmd *parser.Cmd) {
	if cmd == nil {
		return
	}

	// Apply redirections and restore afterwards
	restore, err := sh.applyRedirs(cmd.Redirs)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
		sh.LastStatus = 1
		return
	}
	defer restore()

	if cmd.FuncDef != nil {
		sh.Funcs[cmd.FuncDef.Name] = cmd.FuncDef
		sh.LastStatus = 0
		return
	}
	if cmd.Simple != nil {
		sh.execSimple(cmd.Simple)
		return
	}
	if cmd.If != nil {
		sh.execIf(cmd.If)
		return
	}
	if cmd.While != nil {
		sh.execWhile(cmd.While)
		return
	}
	if cmd.Until != nil {
		sh.execWhile(cmd.Until)
		return
	}
	if cmd.For != nil {
		sh.execFor(cmd.For)
		return
	}
	if cmd.Case != nil {
		sh.execCase(cmd.Case)
		return
	}
	if cmd.Brace != nil {
		sh.execStmts(cmd.Brace.Body)
		return
	}
	if cmd.Subshell != nil {
		sub := sh.subshell()
		func() {
			defer func() {
				if r := recover(); r != nil {
					if es, ok := r.(exitSignal); ok {
						sub.LastStatus = es.code
						return
					}
					panic(r)
				}
			}()
			sub.execStmts(cmd.Subshell.Body)
		}()
		sh.LastStatus = sub.LastStatus
		return
	}
}

// ─── Simple command ───────────────────────────────────────────────────────────

func (sh *Shell) execSimple(sc *parser.SimpleCmd) {
	if sh.OptNoexec {
		return
	}

	// Expand assignments
	var assigns []string
	for _, raw := range sc.Assigns {
		idx := strings.IndexByte(raw, '=')
		if idx < 0 {
			assigns = append(assigns, raw)
			continue
		}
		name := raw[:idx]
		rhs, err := sh.ExpandWordNoSplit(raw[idx+1:])
		if err != nil {
			fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
			sh.LastStatus = 1
			return
		}
		assigns = append(assigns, name+"="+rhs)
	}

	// Expand arguments
	args, err := sh.ExpandWords(sc.Args)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
		sh.LastStatus = 1
		return
	}

	// Apply redirections from simple command
	restore, err := sh.applyRedirs(sc.Redirs)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
		sh.LastStatus = 1
		return
	}
	defer restore()

	// If only assignments (no command)
	if len(args) == 0 {
		// Xtrace: print assignments
		if sh.OptXtrace {
			fmt.Fprintf(sh.Stderr, "+ %s\n", strings.Join(assigns, " "))
		}
		for _, a := range assigns {
			idx := strings.IndexByte(a, '=')
			name, val := a[:idx], a[idx+1:]
			if err := sh.Env.Set(name, val); err != nil {
				fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
				sh.LastStatus = 1
				return
			}
		}
		sh.LastStatus = 0
		return
	}

	name := args[0]

	// Alias expansion
	if expanded, ok := sh.expandAlias(name); ok {
		// Re-parse with expanded alias
		full := expanded + " " + strings.Join(args[1:], " ")
		sh.RunString(full)
		return
	}

	// Xtrace
	if sh.OptXtrace {
		fmt.Fprintf(sh.Stderr, "+ %s\n", strings.Join(args, " "))
	}

	// Check for nounset
	if sh.OptNounset {
		for _, arg := range sc.Args {
			if err := sh.checkNounset(arg); err != nil {
				fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
				sh.LastStatus = 1
				return
			}
		}
	}

	// Prepare env for subprocess: merge assigns + current exported env
	subEnv := sh.Env.Environ()
	for _, a := range assigns {
		subEnv = appendOrReplace(subEnv, a)
	}

	// Is it a builtin?
	if isBuiltin(name) {
		// Apply temporary assignments to env for builtins
		if len(assigns) > 0 {
			saved := make(map[string]string)
			for _, a := range assigns {
				idx := strings.IndexByte(a, '=')
				k, v := a[:idx], a[idx+1:]
				old, _ := sh.Env.Get(k)
				saved[k] = old
				_ = sh.Env.Set(k, v)
			}
			sh.LastStatus = sh.runBuiltin(name, args)
			for k, v := range saved {
				_ = sh.Env.Set(k, v)
			}
		} else {
			sh.LastStatus = sh.runBuiltin(name, args)
		}
		return
	}

	// Is it a function?
	if fn, ok := sh.Funcs[name]; ok {
		sh.callFunc(fn, args, assigns)
		return
	}

	// External command
	path, err := exec.LookPath(name)
	if err != nil {
		// Try relative to CWD
		if !strings.ContainsRune(name, '/') {
			fmt.Fprintf(sh.Stderr, "%s: command not found\n", name)
			sh.LastStatus = 127
			return
		}
		path = filepath.Join(sh.CWD, name)
	}

	cmd := &exec.Cmd{
		Path:   path,
		Args:   args,
		Env:    subEnv,
		Dir:    sh.CWD,
		Stdin:  sh.Stdin,
		Stdout: sh.Stdout,
		Stderr: sh.Stderr,
	}

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			sh.LastStatus = exitErr.ExitCode()
			return
		}
		fmt.Fprintf(sh.Stderr, "vish: %s: %v\n", name, err)
		sh.LastStatus = 126
		return
	}
	sh.LastStatus = 0
}

func appendOrReplace(env []string, kv string) []string {
	key := kv[:strings.IndexByte(kv, '=')]
	for i, e := range env {
		if strings.HasPrefix(e, key+"=") {
			env[i] = kv
			return env
		}
	}
	return append(env, kv)
}

func (sh *Shell) expandAlias(name string) (string, bool) {
	if v, ok := sh.Aliases[name]; ok {
		return v, true
	}
	return "", false
}

func (sh *Shell) checkNounset(raw string) error {
	// Check if any $VAR in raw is unset
	// Simplified: check at expansion time — handled in expandDollar
	return nil
}

// ─── Function calls ───────────────────────────────────────────────────────────

func (sh *Shell) callFunc(fn *parser.FuncDef, args []string, assigns []string) {
	// Create a new scope for function execution
	funcEnv := sh.Env.child()
	saved := &Shell{
		Env:         sh.Env,
		Positionals: sh.Positionals,
	}

	// Apply temporary assignments
	for _, a := range assigns {
		idx := strings.IndexByte(a, '=')
		funcEnv.vars[a[:idx]] = a[idx+1:]
	}

	sh.Env = funcEnv
	sh.Positionals = args

	defer func() {
		sh.Env = saved.Env
		sh.Positionals = saved.Positionals
		sh.Returning = false
	}()

	sh.execCmd(fn.Body)

	if sh.Returning {
		sh.LastStatus = sh.ReturnValue
	}
}

// ─── Compound commands ────────────────────────────────────────────────────────

func (sh *Shell) execIf(ic *parser.IfClause) {
	sh.execCond(ic.Cond)
	if sh.LastStatus == 0 {
		sh.execStmts(ic.Then)
		return
	}
	for _, elif := range ic.Elifs {
		sh.execCond(elif.Cond)
		if sh.LastStatus == 0 {
			sh.execStmts(elif.Then)
			return
		}
	}
	if ic.Else != nil {
		sh.execStmts(ic.Else)
	}
}

func (sh *Shell) execWhile(wc *parser.WhileClause) {
	for {
		sh.execCond(wc.Cond)
		condOk := sh.LastStatus == 0
		if wc.Kind == "until" {
			condOk = !condOk
		}
		if !condOk {
			break
		}
		sh.execStmts(wc.Body)
		if sh.BreakLevel > 0 {
			sh.BreakLevel--
			break
		}
		if sh.ContinueLevel > 0 {
			sh.ContinueLevel--
			if sh.ContinueLevel > 0 {
				break
			}
			continue
		}
		if sh.Returning {
			break
		}
	}
}

func (sh *Shell) execFor(fc *parser.ForClause) {
	var items []string
	if fc.Items == nil {
		// Iterate over "$@"
		items = sh.Positionals[1:]
	} else {
		var err error
		items, err = sh.ExpandWords(fc.Items)
		if err != nil {
			fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
			sh.LastStatus = 1
			return
		}
	}

	for _, item := range items {
		_ = sh.Env.Set(fc.Var, item)
		sh.execStmts(fc.Body)
		if sh.BreakLevel > 0 {
			sh.BreakLevel--
			break
		}
		if sh.ContinueLevel > 0 {
			sh.ContinueLevel--
			if sh.ContinueLevel > 0 {
				break
			}
			continue
		}
		if sh.Returning {
			break
		}
	}
}

func (sh *Shell) execCase(cc *parser.CaseClause) {
	word, err := sh.ExpandWordNoSplit(cc.Word)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
		sh.LastStatus = 1
		return
	}

	for _, item := range cc.Items {
		matched := false
		for _, pat := range item.Patterns {
			expandedPat, _ := sh.ExpandWordNoSplit(pat)
			if shellGlobMatch(expandedPat, word) {
				matched = true
				break
			}
		}
		if matched {
			sh.execStmts(item.Body)
			return
		}
	}
	sh.LastStatus = 0
}

// ─── I/O Redirection ──────────────────────────────────────────────────────────

// applyRedirs sets up I/O redirections and returns a restore function.
func (sh *Shell) applyRedirs(redirs []*parser.Redir) (func(), error) {
	if len(redirs) == 0 {
		return func() {}, nil
	}

	savedStdin := sh.Stdin
	savedStdout := sh.Stdout
	savedStderr := sh.Stderr

	restore := func() {
		sh.Stdin = savedStdin
		sh.Stdout = savedStdout
		sh.Stderr = savedStderr
	}

	for _, r := range redirs {
		if err := sh.applyRedir(r); err != nil {
			restore()
			return func() {}, err
		}
	}
	return restore, nil
}

func (sh *Shell) applyRedir(r *parser.Redir) error {
	switch r.Op {
	case "<":
		word, err := sh.ExpandWordNoSplit(r.Word)
		if err != nil {
			return err
		}
		f, err := os.Open(word)
		if err != nil {
			return err
		}
		if r.Fd == 0 {
			sh.Stdin = f
		}

	case ">":
		word, err := sh.ExpandWordNoSplit(r.Word)
		if err != nil {
			return err
		}
		f, err := os.Create(word)
		if err != nil {
			return err
		}
		setFdOutput(sh, r.Fd, f)

	case ">>":
		word, err := sh.ExpandWordNoSplit(r.Word)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(word, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		setFdOutput(sh, r.Fd, f)

	case ">|":
		// Clobber (forced overwrite)
		word, err := sh.ExpandWordNoSplit(r.Word)
		if err != nil {
			return err
		}
		f, err := os.Create(word)
		if err != nil {
			return err
		}
		setFdOutput(sh, r.Fd, f)

	case ">&":
		word, err := sh.ExpandWordNoSplit(r.Word)
		if err != nil {
			return err
		}
		// >&2, >&1: redirect to another fd
		// Our simplified version handles common cases
		switch word {
		case "1":
			setFdOutput(sh, r.Fd, sh.Stdout)
		case "2":
			setFdOutput(sh, r.Fd, sh.Stderr)
		case "-": // close
			// no-op in our model
		default:
			// Try as file open for writing
			f, err := os.Create(word)
			if err != nil {
				return fmt.Errorf(">&%s: %v", word, err)
			}
			setFdOutput(sh, r.Fd, f)
		}

	case "<&":
		word, err := sh.ExpandWordNoSplit(r.Word)
		if err != nil {
			return err
		}
		switch word {
		case "0":
			// stdin from stdin — no-op
		case "-":
			// close stdin
			sh.Stdin = io.NopCloser(strings.NewReader(""))
		}

	case "<>":
		word, err := sh.ExpandWordNoSplit(r.Word)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(word, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			return err
		}
		if r.Fd == 0 {
			sh.Stdin = f
		} else {
			setFdOutput(sh, r.Fd, f)
		}

	case "<<", "<<-":
		var body string
		if r.HereQuote {
			body = r.HereDoc
		} else {
			var err error
			body, err = sh.ExpandWordNoSplit(r.HereDoc)
			if err != nil {
				return err
			}
		}
		sh.Stdin = strings.NewReader(body)
	}
	return nil
}

func setFdOutput(sh *Shell, fd int, w io.Writer) {
	switch fd {
	case 1:
		sh.Stdout = w
	case 2:
		sh.Stderr = w
	}
}

// ─── Nounset checking ─────────────────────────────────────────────────────────

// (placeholder; real checking happens during expansion)

// ─── Utilities ────────────────────────────────────────────────────────────────

// RunInteractive runs a single line in interactive mode.
func (sh *Shell) RunInteractive(line string) {
	defer func() {
		if r := recover(); r != nil {
			if es, ok := r.(exitSignal); ok {
				// Run EXIT trap
				runExitTrap(sh)
				os.Exit(es.code)
			}
			fmt.Fprintf(sh.Stderr, "vish: internal error: %v\n", r)
		}
	}()

	f, err := parser.ParseFile(line)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "vish: %v\n", err)
		sh.LastStatus = 1
		return
	}

	for _, stmt := range f.Stmts {
		sh.execStmt(stmt)
		if sh.Returning {
			sh.Returning = false
		}
		if sh.BreakLevel > 0 {
			sh.BreakLevel = 0
		}
		if sh.ContinueLevel > 0 {
			sh.ContinueLevel = 0
		}
	}
}

// PS1 returns the current prompt string.
func (sh *Shell) PS1() string {
	ps1, ok := sh.Env.Get("PS1")
	if !ok || ps1 == "" {
		if os.Getuid() == 0 {
			ps1 = "# "
		} else {
			ps1 = "$ "
		}
	}
	// Simple prompt expansion (\u, \h, \w, \n, \$)
	ps1 = expandPrompt(ps1, sh)
	return ps1
}

func expandPrompt(ps1 string, sh *Shell) string {
	ps1 = strings.ReplaceAll(ps1, `\n`, "\n")
	ps1 = strings.ReplaceAll(ps1, `\$`, func() string {
		if os.Getuid() == 0 {
			return "#"
		}
		return "$"
	}())
	if host, _ := os.Hostname(); host != "" {
		short := strings.SplitN(host, ".", 2)[0]
		ps1 = strings.ReplaceAll(ps1, `\h`, short)
		ps1 = strings.ReplaceAll(ps1, `\H`, host)
	}
	ps1 = strings.ReplaceAll(ps1, `\u`, os.Getenv("USER"))
	cwd := sh.CWD
	if home := os.Getenv("HOME"); home != "" {
		if strings.HasPrefix(cwd, home) {
			cwd = "~" + cwd[len(home):]
		}
	}
	ps1 = strings.ReplaceAll(ps1, `\w`, cwd)
	ps1 = strings.ReplaceAll(ps1, `\W`, filepath.Base(cwd))
	return ps1
}

// SetupSignals installs signal handlers for the interactive shell.
func (sh *Shell) SetupSignals() {
	// SIGINT should not kill the interactive shell
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT)
	go func() {
		for range c {
			if handler, ok := sh.Traps["INT"]; ok {
				sh.RunString(handler)
			}
			// Otherwise just interrupt the current command (handled by os/exec)
		}
	}()
}
