// Package interp — POSIX built-in commands for vish.
package interp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// ─── Built-in dispatch ────────────────────────────────────────────────────────

// isBuiltin returns true if name is a built-in command.
func isBuiltin(name string) bool {
	_, ok := builtins[name]
	return ok
}

type builtinFunc func(sh *Shell, args []string) int

var builtins map[string]builtinFunc

func init() {
	builtins = map[string]builtinFunc{
		":":        builtinColon,
		".":        builtinDot,
		"source":   builtinDot,
		"break":    builtinBreak,
		"continue": builtinContinue,
		"cd":       builtinCd,
		"eval":     builtinEval,
		"exec":     builtinExec,
		"exit":     builtinExit,
		"export":   builtinExport,
		"readonly": builtinReadonly,
		"false":    builtinFalse,
		"true":     builtinTrue,
		"pwd":      builtinPwd,
		"read":     builtinRead,
		"return":   builtinReturn,
		"set":      builtinSet,
		"shift":    builtinShift,
		"test":     builtinTest,
		"[":        builtinTestBracket,
		"unset":    builtinUnset,
		"echo":     builtinEcho,
		"printf":   builtinPrintf,
		"type":     builtinType,
		"command":  builtinCommand,
		"hash":     builtinHash,
		"getopts":  builtinGetopts,
		"umask":    builtinUmask,
		"wait":     builtinWait,
		"jobs":     builtinJobs,
		"kill":     builtinKill,
		"trap":     builtinTrap,
		"local":    builtinLocal,
		"declare":  builtinDeclare,
		"typeset":  builtinDeclare,
		"alias":    builtinAlias,
		"unalias":  builtinUnalias,
		"ulimit":   builtinUlimit,
		"times":    builtinTimes,
	}
}

// runBuiltin runs a built-in command and returns its exit status.
func (sh *Shell) runBuiltin(name string, args []string) int {
	fn := builtins[name]
	return fn(sh, args)
}

// ─── Individual builtins ──────────────────────────────────────────────────────

// : — always succeeds
func builtinColon(_ *Shell, _ []string) int { return 0 }

// false — always fails
func builtinFalse(_ *Shell, _ []string) int { return 1 }

// true — always succeeds
func builtinTrue(_ *Shell, _ []string) int { return 0 }

// . / source — execute a file in the current shell context
func builtinDot(sh *Shell, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(sh.Stderr, ".: filename argument required")
		return 1
	}
	path := args[1]
	data, err := os.ReadFile(path)
	if err != nil {
		// Try PATH lookup
		found, lerr := exec.LookPath(path)
		if lerr != nil {
			fmt.Fprintf(sh.Stderr, ".: %s: %v\n", path, err)
			return 1
		}
		data, err = os.ReadFile(found)
		if err != nil {
			fmt.Fprintf(sh.Stderr, ".: %s: %v\n", path, err)
			return 1
		}
	}
	oldPos := sh.Positionals
	if len(args) > 2 {
		sh.Positionals = append([]string{args[1]}, args[2:]...)
	}
	sh.RunString(string(data))
	sh.Positionals = oldPos
	return sh.LastStatus
}

// break
func builtinBreak(sh *Shell, args []string) int {
	n := 1
	if len(args) > 1 {
		if v, err := strconv.Atoi(args[1]); err == nil && v >= 1 {
			n = v
		}
	}
	sh.BreakLevel = n
	return 0
}

// continue
func builtinContinue(sh *Shell, args []string) int {
	n := 1
	if len(args) > 1 {
		if v, err := strconv.Atoi(args[1]); err == nil && v >= 1 {
			n = v
		}
	}
	sh.ContinueLevel = n
	return 0
}

// return
func builtinReturn(sh *Shell, args []string) int {
	n := sh.LastStatus
	if len(args) > 1 {
		if v, err := strconv.Atoi(args[1]); err == nil {
			n = v
		}
	}
	sh.ReturnValue = n
	sh.Returning = true
	return n
}

// exit
func builtinExit(sh *Shell, args []string) int {
	n := sh.LastStatus
	if len(args) > 1 {
		if v, err := strconv.Atoi(args[1]); err == nil {
			n = v
		}
	}
	panic(exitSignal{code: n})
}

// cd
func builtinCd(sh *Shell, args []string) int {
	target := ""
	switch len(args) {
	case 1:
		target = os.Getenv("HOME")
		if target == "" {
			fmt.Fprintln(sh.Stderr, "cd: HOME not set")
			return 1
		}
	case 2:
		if args[1] == "-" {
			target, _ = sh.Env.Get("OLDPWD")
			if target == "" {
				fmt.Fprintln(sh.Stderr, "cd: OLDPWD not set")
				return 1
			}
			fmt.Fprintln(sh.Stdout, target)
		} else {
			target = args[1]
		}
	default:
		fmt.Fprintln(sh.Stderr, "cd: too many arguments")
		return 1
	}

	old := sh.CWD
	newDir := target
	if !filepath.IsAbs(newDir) {
		newDir = filepath.Join(sh.CWD, newDir)
	}
	newDir = filepath.Clean(newDir)

	if !sh.IsSubshell {
		if err := os.Chdir(newDir); err != nil {
			fmt.Fprintf(sh.Stderr, "cd: %s: %v\n", newDir, err)
			return 1
		}
	} else {
		// Verify directory exists without changing the OS cwd
		if _, err := os.Stat(newDir); err != nil {
			fmt.Fprintf(sh.Stderr, "cd: %s: %v\n", newDir, err)
			return 1
		}
	}

	sh.CWD = newDir
	_ = sh.Env.Set("OLDPWD", old)
	_ = sh.Env.Set("PWD", newDir)
	return 0
}

// pwd
func builtinPwd(sh *Shell, args []string) int {
	fmt.Fprintln(sh.Stdout, sh.CWD)
	return 0
}

// echo
func builtinEcho(sh *Shell, args []string) int {
	noNewline := false
	interpret := false
	a := args[1:]
	for len(a) > 0 && strings.HasPrefix(a[0], "-") {
		switch a[0] {
		case "-n":
			noNewline = true
			a = a[1:]
		case "-e":
			interpret = true
			a = a[1:]
		case "-E":
			interpret = false
			a = a[1:]
		default:
			goto doneFlags
		}
	}
doneFlags:
	out := strings.Join(a, " ")
	if interpret {
		out = interpretEscapes(out)
	}
	if noNewline {
		fmt.Fprint(sh.Stdout, out)
	} else {
		fmt.Fprintln(sh.Stdout, out)
	}
	return 0
}

func interpretEscapes(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '\\' || i+1 >= len(runes) {
			sb.WriteRune(runes[i])
			continue
		}
		i++
		switch runes[i] {
		case 'n':
			sb.WriteByte('\n')
		case 't':
			sb.WriteByte('\t')
		case 'r':
			sb.WriteByte('\r')
		case '\\':
			sb.WriteByte('\\')
		case 'a':
			sb.WriteByte('\a')
		case 'b':
			sb.WriteByte('\b')
		case 'f':
			sb.WriteByte('\f')
		case 'v':
			sb.WriteByte('\v')
		case '0':
			// Octal \0NNN
			var oct []rune
			for j := 0; j < 3 && i+1 < len(runes) && runes[i+1] >= '0' && runes[i+1] <= '7'; j++ {
				i++
				oct = append(oct, runes[i])
			}
			if len(oct) > 0 {
				n, _ := strconv.ParseInt(string(oct), 8, 32)
				sb.WriteRune(rune(n))
			} else {
				sb.WriteByte(0)
			}
		default:
			sb.WriteByte('\\')
			sb.WriteRune(runes[i])
		}
	}
	return sb.String()
}

// printf
func builtinPrintf(sh *Shell, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(sh.Stderr, "printf: usage: printf format [args...]")
		return 1
	}
	format := interpretEscapes(args[1])
	fmtArgs := args[2:]

	// POSIX: if more args than format consumes, cycle the format string
	if len(fmtArgs) == 0 {
		fmt.Fprint(sh.Stdout, sprintfShell(format, fmtArgs))
	} else {
		// Count how many args one pass of the format consumes
		for len(fmtArgs) > 0 {
			consumed := countFormatArgs(format)
			if consumed == 0 {
				consumed = 1 // prevent infinite loop
			}
			batch := fmtArgs
			if consumed < len(fmtArgs) {
				batch = fmtArgs[:consumed]
			}
			fmt.Fprint(sh.Stdout, sprintfShell(format, batch))
			if consumed >= len(fmtArgs) {
				break
			}
			fmtArgs = fmtArgs[consumed:]
		}
	}
	return 0
}

// countFormatArgs counts the number of % format specifiers in a format string.
func countFormatArgs(format string) int {
	count := 0
	runes := []rune(format)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '%' {
			i++
			if i < len(runes) && runes[i] == '%' {
				continue // %% is literal
			}
			// skip flags, width, precision
			for i < len(runes) && strings.ContainsRune("-+ 0#", runes[i]) {
				i++
			}
			for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
				i++
			}
			if i < len(runes) && runes[i] == '.' {
				i++
				for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
					i++
				}
			}
			if i < len(runes) {
				count++
			}
		}
	}
	return count
}


func sprintfShell(format string, args []string) string {
	var sb strings.Builder
	runes := []rune(format)
	argIdx := 0
	nextArg := func() string {
		if argIdx < len(args) {
			s := args[argIdx]
			argIdx++
			return s
		}
		return ""
	}
	nextInt := func() int64 {
		n, _ := strconv.ParseInt(nextArg(), 10, 64)
		return n
	}
	nextFloat := func() float64 {
		f, _ := strconv.ParseFloat(nextArg(), 64)
		return f
	}

	for i := 0; i < len(runes); i++ {
		if runes[i] != '%' {
			sb.WriteRune(runes[i])
			continue
		}
		i++
		if i >= len(runes) {
			break
		}
		if runes[i] == '%' {
			sb.WriteByte('%')
			continue
		}

		// Collect full format spec: [flags][width][.prec]verb
		specStart := i - 1 // position of '%'
		// flags: -, +, ' ', 0, #
		for i < len(runes) && strings.ContainsRune("-+ 0#", runes[i]) {
			i++
		}
		// width
		for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
			i++
		}
		// .precision
		if i < len(runes) && runes[i] == '.' {
			i++
			for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
				i++
			}
		}
		if i >= len(runes) {
			break
		}
		verb := runes[i]
		spec := string(runes[specStart : i+1]) // e.g. "%-10s"

		switch verb {
		case 's':
			sb.WriteString(fmt.Sprintf(spec, nextArg()))
		case 'd', 'i':
			goSpec := strings.Replace(spec, "i", "d", 1)
			sb.WriteString(fmt.Sprintf(goSpec, nextInt()))
		case 'u':
			sb.WriteString(fmt.Sprintf(strings.Replace(spec, "u", "d", 1), uint64(nextInt())))
		case 'f', 'e', 'E', 'g', 'G':
			sb.WriteString(fmt.Sprintf(spec, nextFloat()))
		case 'x':
			sb.WriteString(fmt.Sprintf(spec, nextInt()))
		case 'X':
			sb.WriteString(fmt.Sprintf(spec, nextInt()))
		case 'o':
			sb.WriteString(fmt.Sprintf(spec, nextInt()))
		case 'b':
			sb.WriteString(fmt.Sprintf("%b", nextInt()))
		case 'c':
			s := nextArg()
			if len(s) > 0 {
				sb.WriteString(fmt.Sprintf(strings.Replace(spec, "c", "c", 1), rune(s[0])))
			}
		default:
			sb.WriteString(spec)
		}
	}
	return sb.String()
}


// export
func builtinExport(sh *Shell, args []string) int {
	if len(args) == 1 {
		// List all exported vars
		vars := sh.Env.AllVars()
		keys := make([]string, 0, len(vars))
		for k := range vars {
			if sh.Env.IsExported(k) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(sh.Stdout, "export %s=%s\n", k, vars[k])
		}
		return 0
	}

	status := 0
	for _, arg := range args[1:] {
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			name := arg[:idx]
			val := arg[idx+1:]
			if err := sh.Env.Set(name, val); err != nil {
				fmt.Fprintf(sh.Stderr, "export: %v\n", err)
				status = 1
				continue
			}
			sh.Env.Export(name)
		} else {
			sh.Env.Export(arg)
		}
	}
	return status
}

// readonly
func builtinReadonly(sh *Shell, args []string) int {
	if len(args) == 1 {
		vars := sh.Env.AllVars()
		keys := make([]string, 0, len(vars))
		for k := range vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if sh.Env.AllAttrs(k)&attrReadonly != 0 {
				fmt.Fprintf(sh.Stdout, "readonly %s=%s\n", k, vars[k])
			}
		}
		return 0
	}
	status := 0
	for _, arg := range args[1:] {
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			name := arg[:idx]
			val := arg[idx+1:]
			if err := sh.Env.Readonly(name, &val); err != nil {
				fmt.Fprintf(sh.Stderr, "readonly: %v\n", err)
				status = 1
			}
		} else {
			if err := sh.Env.Readonly(arg, nil); err != nil {
				fmt.Fprintf(sh.Stderr, "readonly: %v\n", err)
				status = 1
			}
		}
	}
	return status
}

// local — declare a local variable (only meaningful inside functions)
func builtinLocal(sh *Shell, args []string) int {
	status := 0
	for _, arg := range args[1:] {
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			name := arg[:idx]
			val := arg[idx+1:]
			sh.Env.vars[name] = val // local to current scope
		} else {
			if _, exists := sh.Env.vars[arg]; !exists {
				sh.Env.vars[arg] = "" // create in current scope
			}
		}
	}
	return status
}

// declare / typeset
func builtinDeclare(sh *Shell, args []string) int {
	export := false
	readonly := false
	a := args[1:]
	for len(a) > 0 && strings.HasPrefix(a[0], "-") {
		switch a[0] {
		case "-x":
			export = true
		case "-r":
			readonly = true
		}
		a = a[1:]
	}
	status := 0
	for _, arg := range a {
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			name := arg[:idx]
			val := arg[idx+1:]
			if readonly {
				if err := sh.Env.Readonly(name, &val); err != nil {
					fmt.Fprintf(sh.Stderr, "declare: %v\n", err)
					status = 1
					continue
				}
			} else {
				if err := sh.Env.Set(name, val); err != nil {
					fmt.Fprintf(sh.Stderr, "declare: %v\n", err)
					status = 1
					continue
				}
			}
			if export {
				sh.Env.Export(name)
			}
		}
	}
	return status
}

// unset
func builtinUnset(sh *Shell, args []string) int {
	funcsOnly := false
	varsOnly := false
	a := args[1:]
	for len(a) > 0 && strings.HasPrefix(a[0], "-") {
		switch a[0] {
		case "-f":
			funcsOnly = true
		case "-v":
			varsOnly = true
		}
		a = a[1:]
	}
	for _, name := range a {
		if !varsOnly {
			delete(sh.Funcs, name)
		}
		if !funcsOnly {
			sh.Env.Unset(name)
		}
	}
	return 0
}

// set
func builtinSet(sh *Shell, args []string) int {
	if len(args) == 1 {
		// Print all variables
		vars := sh.Env.AllVars()
		keys := make([]string, 0, len(vars))
		for k := range vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(sh.Stdout, "%s=%s\n", k, vars[k])
		}
		return 0
	}

	// Parse options
	i := 1
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if arg == "-" {
			i++
			break
		}
		if arg == "-o" || arg == "+o" {
			enable := arg == "-o"
			i++
			if i < len(args) {
				setNamedOption(sh, args[i], enable)
				i++
			}
			continue
		}
		if !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "+") {
			break
		}
		enable := strings.HasPrefix(arg, "-")
		for _, c := range arg[1:] {
			setOption(sh, c, enable)
		}
		i++
	}

	// Remaining args are positional parameters
	if i < len(args) {
		sh.Positionals = append([]string{sh.Positionals[0]}, args[i:]...)
	}
	return 0
}

func setOption(sh *Shell, c rune, enable bool) {
	switch c {
	case 'e':
		sh.OptExit = enable
	case 'u':
		sh.OptNounset = enable
	case 'x':
		sh.OptXtrace = enable
	case 'n':
		sh.OptNoexec = enable
	case 'f':
		sh.OptNoglob = enable
	}
}

func setNamedOption(sh *Shell, name string, enable bool) {
	switch name {
	case "errexit":
		sh.OptExit = enable
	case "nounset":
		sh.OptNounset = enable
	case "xtrace":
		sh.OptXtrace = enable
	case "noexec":
		sh.OptNoexec = enable
	case "noglob":
		sh.OptNoglob = enable
	}
}

// shift
func builtinShift(sh *Shell, args []string) int {
	n := 1
	if len(args) > 1 {
		if v, err := strconv.Atoi(args[1]); err == nil && v >= 0 {
			n = v
		}
	}
	if len(sh.Positionals) > 1 {
		if n > len(sh.Positionals)-1 {
			n = len(sh.Positionals) - 1
		}
		sh.Positionals = append(sh.Positionals[:1], sh.Positionals[1+n:]...)
	}
	return 0
}

// read
func builtinRead(sh *Shell, args []string) int {
	raw := false
	silent := false
	promptStr := ""
	a := args[1:]
	for len(a) > 0 && strings.HasPrefix(a[0], "-") {
		switch a[0] {
		case "-r":
			raw = true
		case "-s":
			silent = true
		case "-p":
			a = a[1:]
			if len(a) > 0 {
				promptStr = a[0]
			}
		}
		a = a[1:]
	}

	if promptStr != "" && sh.IsInteractive {
		fmt.Fprint(sh.Stderr, promptStr)
	}
	_ = silent

	var line string
	var buf [1]byte
	var sb strings.Builder
	for {
		n, err := sh.Stdin.Read(buf[:])
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if buf[0] == '\\' && !raw {
				// Check for continuation
				n2, _ := sh.Stdin.Read(buf[:])
				if n2 > 0 && buf[0] == '\n' {
					continue // line continuation
				}
				sb.WriteByte('\\')
				if n2 > 0 {
					sb.WriteByte(buf[0])
				}
				continue
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			line = sb.String()
			if line == "" {
				return 1 // EOF
			}
			break
		}
	}
	line = sb.String()

	ifs := sh.getIFS()
	if len(a) == 0 {
		_ = sh.Env.Set("REPLY", line)
		return 0
	}

	fields := splitByIFS(line, ifs)
	for i, name := range a {
		var val string
		if i < len(a)-1 {
			// Not the last variable: take exactly one field (or empty if no more fields)
			if i < len(fields) {
				val = fields[i]
			}
		} else {
			// Last variable: take all remaining fields joined by first IFS char
			if i < len(fields) {
				sep := " "
				if len(ifs) > 0 && ifs[0] != ' ' && ifs[0] != '\t' && ifs[0] != '\n' {
					sep = string(ifs[0])
				}
				val = strings.Join(fields[i:], sep)
			}
		}
		_ = sh.Env.Set(name, val)
	}
	return 0
}

func splitByIFS(s, ifs string) []string {
	if ifs == "" {
		return []string{s}
	}
	var fields []string
	start := -1
	for i, c := range s {
		if strings.ContainsRune(ifs, c) {
			if start >= 0 {
				fields = append(fields, s[start:i])
				start = -1
			}
		} else {
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		fields = append(fields, s[start:])
	}
	return fields
}

// eval
func builtinEval(sh *Shell, args []string) int {
	if len(args) < 2 {
		return 0
	}
	cmd := strings.Join(args[1:], " ")
	return sh.RunString(cmd)
}

// exec
func builtinExec(sh *Shell, args []string) int {
	if len(args) == 1 {
		// exec with no args: apply redirections only (already handled)
		return 0
	}
	path, err := exec.LookPath(args[1])
	if err != nil {
		fmt.Fprintf(sh.Stderr, "exec: %s: %v\n", args[1], err)
		return 127
	}
	env := sh.Env.Environ()
	if err := syscall.Exec(path, args[1:], env); err != nil {
		fmt.Fprintf(sh.Stderr, "exec: %v\n", err)
		return 1
	}
	return 0 // unreachable
}

// type
func builtinType(sh *Shell, args []string) int {
	if len(args) < 2 {
		return 0
	}
	status := 0
	for _, name := range args[1:] {
		if isBuiltin(name) {
			fmt.Fprintf(sh.Stdout, "%s is a shell builtin\n", name)
		} else if _, ok := sh.Funcs[name]; ok {
			fmt.Fprintf(sh.Stdout, "%s is a function\n", name)
		} else if path, err := exec.LookPath(name); err == nil {
			fmt.Fprintf(sh.Stdout, "%s is %s\n", name, path)
		} else {
			fmt.Fprintf(sh.Stderr, "type: %s: not found\n", name)
			status = 1
		}
	}
	return status
}

// command
func builtinCommand(sh *Shell, args []string) int {
	if len(args) < 2 {
		return 0
	}
	verbose := false
	a := args[1:]
	for len(a) > 0 && strings.HasPrefix(a[0], "-") {
		switch a[0] {
		case "-v":
			verbose = true
		case "-V":
			verbose = true
		}
		a = a[1:]
	}

	if len(a) == 0 {
		return 0
	}

	if verbose {
		name := a[0]
		if isBuiltin(name) {
			fmt.Fprintf(sh.Stdout, "%s\n", name)
		} else if path, err := exec.LookPath(name); err == nil {
			fmt.Fprintf(sh.Stdout, "%s\n", path)
		} else {
			return 1
		}
		return 0
	}

	// Run command bypassing functions
	return sh.runExternal(a[0], a, sh.Env.Environ())
}

// hash — no-op (we don't maintain a hash table but don't error)
func builtinHash(_ *Shell, _ []string) int { return 0 }

// getopts
func builtinGetopts(sh *Shell, args []string) int {
	if len(args) < 3 {
		fmt.Fprintln(sh.Stderr, "getopts: usage: getopts optstring name [arg...]")
		return 1
	}
	optstring := args[1]
	varName := args[2]

	params := sh.Positionals[1:]
	if len(args) > 3 {
		params = args[3:]
	}

	// Get or initialize OPTIND
	optind := 1
	if v, ok := sh.Env.Get("OPTIND"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			optind = n
		}
	}

	if optind > len(params) {
		_ = sh.Env.Set(varName, "?")
		return 1
	}

	arg := params[optind-1]
	if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
		_ = sh.Env.Set(varName, "?")
		_ = sh.Env.Set("OPTIND", strconv.Itoa(optind+1))
		return 1
	}

	optchar := rune(arg[1])
	optIdx := 2 // Next char in arg

	// Find option in optstring
	optstringRunes := []rune(optstring)
	found := false
	needsArg := false
	for i, c := range optstringRunes {
		if c == optchar {
			found = true
			if i+1 < len(optstringRunes) && optstringRunes[i+1] == ':' {
				needsArg = true
			}
			break
		}
	}

	if !found {
		_ = sh.Env.Set(varName, "?")
		_ = sh.Env.Set("OPTARG", string(optchar))
		if !strings.HasPrefix(optstring, ":") {
			fmt.Fprintf(sh.Stderr, "getopts: illegal option -- %c\n", optchar)
		}
		if optIdx > len(arg)-1 {
			optind++
		}
		_ = sh.Env.Set("OPTIND", strconv.Itoa(optind))
		return 0
	}

	_ = sh.Env.Set(varName, string(optchar))
	if needsArg {
		optArg := ""
		if optIdx <= len(arg)-1 {
			optArg = arg[optIdx:]
			optind++
		} else {
			optind++
			if optind <= len(params) {
				optArg = params[optind-1]
				optind++
			} else {
				fmt.Fprintf(sh.Stderr, "getopts: option requires an argument -- %c\n", optchar)
				_ = sh.Env.Set(varName, "?")
				_ = sh.Env.Set("OPTIND", strconv.Itoa(optind))
				return 0
			}
		}
		_ = sh.Env.Set("OPTARG", optArg)
	} else {
		_ = sh.Env.Set("OPTARG", "")
		if optIdx > len(arg)-1 {
			optind++
		} else {
			// More options in same arg: update params conceptually
			// We simplify: advance OPTIND and put remaining in OPTARG fake
			optind++
			// Re-insert remaining options as next arg (simplified)
		}
	}
	_ = sh.Env.Set("OPTIND", strconv.Itoa(optind))
	return 0
}

// umask
func builtinUmask(sh *Shell, args []string) int {
	if len(args) == 1 {
		mask := syscall.Umask(0)
		syscall.Umask(mask) // restore
		fmt.Fprintf(sh.Stdout, "%04o\n", mask)
		return 0
	}
	n, err := strconv.ParseInt(args[1], 8, 64)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "umask: invalid mask: %s\n", args[1])
		return 1
	}
	syscall.Umask(int(n))
	return 0
}

// wait
func builtinWait(sh *Shell, args []string) int {
	if len(args) == 1 {
		// Wait for all background jobs
		for _, job := range sh.Jobs {
			job.Wait()
		}
		sh.Jobs = nil
		return 0
	}
	// Wait for specific PID
	pid, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Fprintf(sh.Stderr, "wait: invalid PID: %s\n", args[1])
		return 1
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "wait: %v\n", err)
		return 1
	}
	state, err := proc.Wait()
	if err != nil {
		return 1
	}
	return state.ExitCode()
}

// jobs
func builtinJobs(sh *Shell, args []string) int {
	for i, job := range sh.Jobs {
		fmt.Fprintf(sh.Stdout, "[%d] %d\n", i+1, job.Pid)
	}
	return 0
}

// kill
func builtinKill(sh *Shell, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(sh.Stderr, "kill: usage: kill [-sig] pid...")
		return 1
	}
	sig := syscall.SIGTERM
	a := args[1:]
	if strings.HasPrefix(a[0], "-") {
		sigName := a[0][1:]
		switch strings.ToUpper(sigName) {
		case "TERM", "15":
			sig = syscall.SIGTERM
		case "KILL", "9":
			sig = syscall.SIGKILL
		case "HUP", "1":
			sig = syscall.SIGHUP
		case "INT", "2":
			sig = syscall.SIGINT
		default:
			fmt.Fprintf(sh.Stderr, "kill: unknown signal: %s\n", sigName)
			return 1
		}
		a = a[1:]
	}
	status := 0
	for _, s := range a {
		pid, err := strconv.Atoi(s)
		if err != nil {
			fmt.Fprintf(sh.Stderr, "kill: invalid PID: %s\n", s)
			status = 1
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			fmt.Fprintf(sh.Stderr, "kill: %v\n", err)
			status = 1
			continue
		}
		if err := proc.Signal(sig); err != nil {
			fmt.Fprintf(sh.Stderr, "kill: %v\n", err)
			status = 1
		}
	}
	return status
}

// trap
func builtinTrap(sh *Shell, args []string) int {
	if len(args) == 1 {
		// List traps
		for sig, action := range sh.Traps {
			fmt.Fprintf(sh.Stdout, "trap -- %q %s\n", action, sig)
		}
		return 0
	}

	if len(args) == 2 && args[1] == "-" {
		// Reset all traps
		sh.Traps = make(map[string]string)
		return 0
	}

	action := args[1]
	for _, sig := range args[2:] {
		if action == "-" {
			delete(sh.Traps, sig)
		} else {
			if sh.Traps == nil {
				sh.Traps = make(map[string]string)
			}
			sh.Traps[sig] = action
		}
	}
	return 0
}

// alias
func builtinAlias(sh *Shell, args []string) int {
	if len(args) == 1 {
		keys := make([]string, 0, len(sh.Aliases))
		for k := range sh.Aliases {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(sh.Stdout, "alias %s='%s'\n", k, sh.Aliases[k])
		}
		return 0
	}
	for _, arg := range args[1:] {
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			sh.Aliases[arg[:idx]] = arg[idx+1:]
		} else {
			if v, ok := sh.Aliases[arg]; ok {
				fmt.Fprintf(sh.Stdout, "alias %s='%s'\n", arg, v)
			} else {
				fmt.Fprintf(sh.Stderr, "alias: %s: not found\n", arg)
				return 1
			}
		}
	}
	return 0
}

// unalias
func builtinUnalias(sh *Shell, args []string) int {
	if len(args) > 1 && args[1] == "-a" {
		sh.Aliases = make(map[string]string)
		return 0
	}
	for _, name := range args[1:] {
		delete(sh.Aliases, name)
	}
	return 0
}

// ulimit
func builtinUlimit(sh *Shell, args []string) int {
	// Simplified: just report/set soft limit for various resources
	_ = sh
	_ = args
	fmt.Fprintln(sh.Stdout, "unlimited")
	return 0
}

// times
func builtinTimes(sh *Shell, _ []string) int {
	var usage syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
	userSec := usage.Utime.Sec
	userUSec := usage.Utime.Usec
	sysSec := usage.Stime.Sec
	sysUSec := usage.Stime.Usec
	fmt.Fprintf(sh.Stdout, "%dm%d.%03ds %dm%d.%03ds\n",
		userSec/60, userSec%60, userUSec/1000,
		sysSec/60, sysSec%60, sysUSec/1000)
	return 0
}

// ─── test / [ ────────────────────────────────────────────────────────────────

func builtinTest(sh *Shell, args []string) int {
	return evalTest(sh, args[1:])
}

func builtinTestBracket(sh *Shell, args []string) int {
	// Strip trailing ]
	a := args[1:]
	if len(a) > 0 && a[len(a)-1] == "]" {
		a = a[:len(a)-1]
	}
	return evalTest(sh, a)
}

func evalTest(_ *Shell, args []string) int {
	result, _ := testExpr(args, 0)
	if result {
		return 0
	}
	return 1
}

func testExpr(args []string, pos int) (bool, int) {
	if pos >= len(args) {
		return false, pos
	}

	// Handle not (!)
	if args[pos] == "!" {
		val, next := testExpr(args, pos+1)
		return !val, next
	}

	// Handle -a and -o (binary)
	left, next := testPrimary(args, pos)
	for next < len(args) {
		if args[next] == "-a" {
			right, n := testPrimary(args, next+1)
			left = left && right
			next = n
		} else if args[next] == "-o" {
			right, n := testPrimary(args, next+1)
			left = left || right
			next = n
		} else {
			break
		}
	}
	return left, next
}

func testPrimary(args []string, pos int) (bool, int) {
	if pos >= len(args) {
		return false, pos
	}

	// Grouped
	if args[pos] == "(" {
		val, next := testExpr(args, pos+1)
		if next < len(args) && args[next] == ")" {
			next++
		}
		return val, next
	}

	// Unary operators
	if pos+1 < len(args) && strings.HasPrefix(args[pos], "-") && len(args[pos]) == 2 {
		op := args[pos]
		operand := args[pos+1]
		switch op {
		case "-n":
			return len(operand) > 0, pos + 2
		case "-z":
			return len(operand) == 0, pos + 2
		case "-e":
			_, err := os.Stat(operand)
			return err == nil, pos + 2
		case "-f":
			fi, err := os.Stat(operand)
			return err == nil && fi.Mode().IsRegular(), pos + 2
		case "-d":
			fi, err := os.Stat(operand)
			return err == nil && fi.IsDir(), pos + 2
		case "-r":
			_, err := os.Open(operand)
			ok := err == nil
			if ok {
				// close it
			}
			return ok, pos + 2
		case "-w":
			_, err := os.OpenFile(operand, os.O_WRONLY, 0)
			return err == nil, pos + 2
		case "-x":
			fi, err := os.Stat(operand)
			return err == nil && fi.Mode()&0111 != 0, pos + 2
		case "-s":
			fi, err := os.Stat(operand)
			return err == nil && fi.Size() > 0, pos + 2
		case "-L":
			fi, err := os.Lstat(operand)
			return err == nil && fi.Mode()&os.ModeSymlink != 0, pos + 2
		case "-p":
			fi, err := os.Stat(operand)
			return err == nil && fi.Mode()&os.ModeNamedPipe != 0, pos + 2
		case "-t":
			fd, _ := strconv.Atoi(operand)
			fi, err := os.Stdout.Stat()
			if fd == 0 {
				fi, err = os.Stdin.Stat()
			} else if fd == 2 {
				fi, err = os.Stderr.Stat()
			}
			return err == nil && fi.Mode()&os.ModeCharDevice != 0, pos + 2
		}
	}

	// Binary operators
	if pos+2 < len(args) {
		left := args[pos]
		op := args[pos+1]
		right := args[pos+2]
		switch op {
		case "=", "==":
			return left == right, pos + 3
		case "!=":
			return left != right, pos + 3
		case "-eq":
			return parseInt(left) == parseInt(right), pos + 3
		case "-ne":
			return parseInt(left) != parseInt(right), pos + 3
		case "-lt":
			return parseInt(left) < parseInt(right), pos + 3
		case "-le":
			return parseInt(left) <= parseInt(right), pos + 3
		case "-gt":
			return parseInt(left) > parseInt(right), pos + 3
		case "-ge":
			return parseInt(left) >= parseInt(right), pos + 3
		}
	}

	// Single string — true if non-empty
	return len(args[pos]) > 0, pos + 1
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// ─── External command runner (used by command builtin) ────────────────────────

func (sh *Shell) runExternal(name string, args []string, environ []string) int {
	path, err := exec.LookPath(name)
	if err != nil {
		fmt.Fprintf(sh.Stderr, "%s: command not found\n", name)
		return 127
	}
	cmd := exec.Cmd{
		Path:   path,
		Args:   args,
		Env:    environ,
		Dir:    sh.CWD,
		Stdin:  sh.Stdin,
		Stdout: sh.Stdout,
		Stderr: sh.Stderr,
	}
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}
