// Package interp — arithmetic expression evaluator for vish.
package interp

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// EvalArith evaluates a shell arithmetic expression string and returns the result.
// Variables are looked up in env. The expression is first word-expanded (for
// $(...), ${VAR}, etc.) before arithmetic parsing.
func EvalArith(expr string, sh *Shell) (int64, error) {
	// Pre-expand the expression to resolve $(cmd), $var, ${var}, etc.
	expanded, err := sh.ExpandWordNoSplit(expr)
	if err != nil {
		return 0, fmt.Errorf("arithmetic: %v", err)
	}
	e := &arithEval{src: []rune(strings.TrimSpace(expanded)), sh: sh}
	val, err := e.expr()
	if err != nil {
		return 0, err
	}
	return val, nil
}

type arithEval struct {
	src []rune
	pos int
	sh  *Shell
}

func (e *arithEval) peek() rune {
	if e.pos >= len(e.src) {
		return 0
	}
	return e.src[e.pos]
}

func (e *arithEval) advance() rune {
	if e.pos >= len(e.src) {
		return 0
	}
	ch := e.src[e.pos]
	e.pos++
	return ch
}

func (e *arithEval) skipWS() {
	for e.pos < len(e.src) && unicode.IsSpace(e.src[e.pos]) {
		e.pos++
	}
}

// expr parses the full expression (assignment or ternary).
func (e *arithEval) expr() (int64, error) {
	return e.assignment()
}

// assignment handles variable assignment and compound assignment operators.
// POSIX requires: =, +=, -=, *=, /=, %=, &=, |=, ^=, <<=, >>=
func (e *arithEval) assignment() (int64, error) {
	savedPos := e.pos
	e.skipWS()

	if isNameStartRune(e.peek()) {
		nameStart := e.pos
		for isNameContRune(e.peek()) {
			e.advance()
		}
		name := string(e.src[nameStart:e.pos])
		e.skipWS()

		op := ""
		switch {
		case e.peek() == '<' && e.peekAt(1) == '<' && e.peekAt(2) == '=':
			e.advance(); e.advance(); e.advance()
			op = "<<="
		case e.peek() == '>' && e.peekAt(1) == '>' && e.peekAt(2) == '=':
			e.advance(); e.advance(); e.advance()
			op = ">>="
		case e.peek() == '+' && e.peekAt(1) == '=':
			e.advance(); e.advance()
			op = "+="
		case e.peek() == '-' && e.peekAt(1) == '=':
			e.advance(); e.advance()
			op = "-="
		case e.peek() == '*' && e.peekAt(1) == '=':
			e.advance(); e.advance()
			op = "*="
		case e.peek() == '/' && e.peekAt(1) == '=':
			e.advance(); e.advance()
			op = "/="
		case e.peek() == '%' && e.peekAt(1) == '=':
			e.advance(); e.advance()
			op = "%="
		case e.peek() == '&' && e.peekAt(1) == '=':
			e.advance(); e.advance()
			op = "&="
		case e.peek() == '|' && e.peekAt(1) == '=':
			e.advance(); e.advance()
			op = "|="
		case e.peek() == '^' && e.peekAt(1) == '=':
			e.advance(); e.advance()
			op = "^="
		case e.peek() == '=' && e.peekAt(1) != '=':
			e.advance()
			op = "="
		}

		if op != "" {
			rhs, err := e.assignment()
			if err != nil {
				return 0, err
			}
			var result int64
			if op == "=" {
				result = rhs
			} else {
				valStr, _ := e.sh.Env.Get(name)
				var lhs int64
				if valStr != "" {
					lhs, err = strconv.ParseInt(strings.TrimSpace(valStr), 10, 64)
					if err != nil {
						return 0, fmt.Errorf("%s: not a valid integer", name)
					}
				}
				switch op {
				case "+=":
					result = lhs + rhs
				case "-=":
					result = lhs - rhs
				case "*=":
					result = lhs * rhs
				case "/=":
					if rhs == 0 {
						return 0, fmt.Errorf("division by zero")
					}
					result = lhs / rhs
				case "%=":
					if rhs == 0 {
						return 0, fmt.Errorf("modulo by zero")
					}
					result = lhs % rhs
				case "&=":
					result = lhs & rhs
				case "|=":
					result = lhs | rhs
				case "^=":
					result = lhs ^ rhs
				case "<<=":
					result = lhs << uint(rhs)
				case ">>=":
					result = lhs >> uint(rhs)
				}
			}
			if err := e.sh.Env.Set(name, strconv.FormatInt(result, 10)); err != nil {
				return 0, fmt.Errorf("%s: %v", name, err)
			}
			return result, nil
		}

		// Not an assignment — backtrack and fall through to ternary
		e.pos = savedPos
	}

	return e.ternary()
}

// ternary: e ? e : e
func (e *arithEval) ternary() (int64, error) {
	cond, err := e.logicalOr()
	if err != nil {
		return 0, err
	}
	e.skipWS()
	if e.peek() == '?' {
		e.advance()
		then, err := e.ternary()
		if err != nil {
			return 0, err
		}
		e.skipWS()
		if e.peek() != ':' {
			return 0, fmt.Errorf("expected ':' in ternary")
		}
		e.advance()
		els, err := e.ternary()
		if err != nil {
			return 0, err
		}
		if cond != 0 {
			return then, nil
		}
		return els, nil
	}
	return cond, nil
}

func (e *arithEval) logicalOr() (int64, error) {
	l, err := e.logicalAnd()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		if e.peek() == '|' && e.peekAt(1) == '|' {
			e.advance()
			e.advance()
			r, err := e.logicalAnd()
			if err != nil {
				return 0, err
			}
			if l != 0 || r != 0 {
				l = 1
			} else {
				l = 0
			}
		} else {
			break
		}
	}
	return l, nil
}

func (e *arithEval) logicalAnd() (int64, error) {
	l, err := e.bitwiseOr()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		if e.peek() == '&' && e.peekAt(1) == '&' {
			e.advance()
			e.advance()
			r, err := e.bitwiseOr()
			if err != nil {
				return 0, err
			}
			if l != 0 && r != 0 {
				l = 1
			} else {
				l = 0
			}
		} else {
			break
		}
	}
	return l, nil
}

func (e *arithEval) bitwiseOr() (int64, error) {
	l, err := e.bitwiseXor()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		if e.peek() == '|' && e.peekAt(1) != '|' {
			e.advance()
			r, err := e.bitwiseXor()
			if err != nil {
				return 0, err
			}
			l |= r
		} else {
			break
		}
	}
	return l, nil
}

func (e *arithEval) bitwiseXor() (int64, error) {
	l, err := e.bitwiseAnd()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		if e.peek() == '^' {
			e.advance()
			r, err := e.bitwiseAnd()
			if err != nil {
				return 0, err
			}
			l ^= r
		} else {
			break
		}
	}
	return l, nil
}

func (e *arithEval) bitwiseAnd() (int64, error) {
	l, err := e.equality()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		if e.peek() == '&' && e.peekAt(1) != '&' {
			e.advance()
			r, err := e.equality()
			if err != nil {
				return 0, err
			}
			l &= r
		} else {
			break
		}
	}
	return l, nil
}

func (e *arithEval) equality() (int64, error) {
	l, err := e.relational()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		if e.peek() == '=' && e.peekAt(1) == '=' {
			e.advance()
			e.advance()
			r, err := e.relational()
			if err != nil {
				return 0, err
			}
			if l == r {
				l = 1
			} else {
				l = 0
			}
		} else if e.peek() == '!' && e.peekAt(1) == '=' {
			e.advance()
			e.advance()
			r, err := e.relational()
			if err != nil {
				return 0, err
			}
			if l != r {
				l = 1
			} else {
				l = 0
			}
		} else {
			break
		}
	}
	return l, nil
}

func (e *arithEval) relational() (int64, error) {
	l, err := e.shift()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		if e.peek() == '<' && e.peekAt(1) == '=' {
			e.advance()
			e.advance()
			r, err := e.shift()
			if err != nil {
				return 0, err
			}
			l = boolInt(l <= r)
		} else if e.peek() == '>' && e.peekAt(1) == '=' {
			e.advance()
			e.advance()
			r, err := e.shift()
			if err != nil {
				return 0, err
			}
			l = boolInt(l >= r)
		} else if e.peek() == '<' && e.peekAt(1) != '<' {
			e.advance()
			r, err := e.shift()
			if err != nil {
				return 0, err
			}
			l = boolInt(l < r)
		} else if e.peek() == '>' && e.peekAt(1) != '>' {
			e.advance()
			r, err := e.shift()
			if err != nil {
				return 0, err
			}
			l = boolInt(l > r)
		} else {
			break
		}
	}
	return l, nil
}

func (e *arithEval) shift() (int64, error) {
	l, err := e.addSub()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		if e.peek() == '<' && e.peekAt(1) == '<' {
			e.advance()
			e.advance()
			r, err := e.addSub()
			if err != nil {
				return 0, err
			}
			l <<= uint(r)
		} else if e.peek() == '>' && e.peekAt(1) == '>' {
			e.advance()
			e.advance()
			r, err := e.addSub()
			if err != nil {
				return 0, err
			}
			l >>= uint(r)
		} else {
			break
		}
	}
	return l, nil
}

func (e *arithEval) addSub() (int64, error) {
	l, err := e.mulDiv()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		if e.peek() == '+' && e.peekAt(1) != '+' {
			e.advance()
			r, err := e.mulDiv()
			if err != nil {
				return 0, err
			}
			l += r
		} else if e.peek() == '-' && e.peekAt(1) != '-' {
			e.advance()
			r, err := e.mulDiv()
			if err != nil {
				return 0, err
			}
			l -= r
		} else {
			break
		}
	}
	return l, nil
}

func (e *arithEval) mulDiv() (int64, error) {
	l, err := e.unary()
	if err != nil {
		return 0, err
	}
	for {
		e.skipWS()
		switch e.peek() {
		case '*':
			if e.peekAt(1) == '*' {
				// exponentiation
				e.advance()
				e.advance()
				r, err := e.unary()
				if err != nil {
					return 0, err
				}
				result := int64(1)
				for i := int64(0); i < r; i++ {
					result *= l
				}
				l = result
			} else {
				e.advance()
				r, err := e.unary()
				if err != nil {
					return 0, err
				}
				l *= r
			}
		case '/':
			e.advance()
			r, err := e.unary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			l /= r
		case '%':
			e.advance()
			r, err := e.unary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			l %= r
		default:
			return l, nil
		}
	}
}

func (e *arithEval) unary() (int64, error) {
	e.skipWS()
	switch e.peek() {
	case '-':
		e.advance()
		v, err := e.unary()
		return -v, err
	case '+':
		e.advance()
		return e.unary()
	case '!':
		e.advance()
		v, err := e.unary()
		if err != nil {
			return 0, err
		}
		return boolInt(v == 0), nil
	case '~':
		e.advance()
		v, err := e.unary()
		if err != nil {
			return 0, err
		}
		return ^v, nil
	}
	return e.primary()
}

func (e *arithEval) primary() (int64, error) {
	e.skipWS()
	ch := e.peek()

	// Grouped expression
	if ch == '(' {
		e.advance()
		v, err := e.expr()
		if err != nil {
			return 0, err
		}
		e.skipWS()
		if e.peek() != ')' {
			return 0, fmt.Errorf("expected ')'")
		}
		e.advance()
		return v, nil
	}

	// Hex / octal literal
	if ch == '0' && (e.peekAt(1) == 'x' || e.peekAt(1) == 'X') {
		e.advance()
		e.advance()
		start := e.pos
		for isHexDigit(e.peek()) {
			e.advance()
		}
		n, err := strconv.ParseInt(string(e.src[start:e.pos]), 16, 64)
		return n, err
	}
	if ch == '0' && isDigitRune(e.peekAt(1)) {
		start := e.pos
		e.advance()
		for isDigitRune(e.peek()) {
			e.advance()
		}
		n, err := strconv.ParseInt(string(e.src[start:e.pos]), 8, 64)
		return n, err
	}

	// Decimal literal
	if isDigitRune(ch) {
		start := e.pos
		for isDigitRune(e.peek()) {
			e.advance()
		}
		n, err := strconv.ParseInt(string(e.src[start:e.pos]), 10, 64)
		return n, err
	}

	// Variable name
	if isNameStartRune(ch) {
		start := e.pos
		for isNameContRune(e.peek()) {
			e.advance()
		}
		name := string(e.src[start:e.pos])
		val, _ := e.sh.Env.Get(name)
		if val == "" {
			return 0, nil
		}
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s: not a valid integer", name)
		}
		return n, nil
	}

	return 0, fmt.Errorf("unexpected character %q in arithmetic", ch)
}

func (e *arithEval) peekAt(offset int) rune {
	i := e.pos + offset
	if i >= len(e.src) {
		return 0
	}
	return e.src[i]
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isDigitRune(c rune) bool { return c >= '0' && c <= '9' }

func isNameStartRune(c rune) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameContRune(c rune) bool { return isNameStartRune(c) || isDigitRune(c) }
