// Package interp provides the shell interpreter for vish.
package interp

import (
	"fmt"
	"os"
	"strings"
)

// ─── Variable attributes ──────────────────────────────────────────────────────

const (
	attrExport   = 1 << 0
	attrReadonly = 1 << 1
)

// ─── Env is a layered variable store ─────────────────────────────────────────

type Env struct {
	vars   map[string]string
	attrs  map[string]int // bitfield: attrExport, attrReadonly
	parent *Env
}

// newEnv creates a root environment pre-populated from os.Environ().
func newEnv() *Env {
	e := &Env{
		vars:  make(map[string]string),
		attrs: make(map[string]int),
	}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k, v := kv[:i], kv[i+1:]
			e.vars[k] = v
			e.attrs[k] |= attrExport
		}
	}
	return e
}

// child creates a new scope with this env as parent (for functions).
func (e *Env) child() *Env {
	return &Env{
		vars:   make(map[string]string),
		attrs:  make(map[string]int),
		parent: e,
	}
}

// Clone creates a detached deep copy of the environment (for subshells).
func (e *Env) Clone() *Env {
	c := &Env{
		vars:  make(map[string]string),
		attrs: make(map[string]int),
	}
	for k, v := range e.AllVars() {
		c.vars[k] = v
		if e.IsExported(k) {
			c.attrs[k] |= attrExport
		}
		if e.IsReadonly(k) {
			c.attrs[k] |= attrReadonly
		}
	}
	return c
}

// IsReadonly reports whether name is readonly.
func (e *Env) IsReadonly(name string) bool {
	for cur := e; cur != nil; cur = cur.parent {
		if a, ok := cur.attrs[name]; ok && a&attrReadonly != 0 {
			return true
		}
	}
	return false
}

// Get looks up a variable, walking up the scope chain.
func (e *Env) Get(name string) (string, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if v, ok := cur.vars[name]; ok {
			return v, true
		}
	}
	return "", false
}

// Set sets a variable. If the variable already exists in an ancestor scope
// (and is not locally declared in the current scope), it is updated there.
// This matches POSIX shell behavior where unlocalized assignments propagate.
// New variables are created in the root (global) scope, so that assignments
// inside functions without 'local' are visible globally (POSIX behavior).
// The 'local' builtin bypasses this by writing directly to Env.vars.
func (e *Env) Set(name, value string) error {
	// Check readonly in any scope
	for cur := e; cur != nil; cur = cur.parent {
		if a, ok := cur.attrs[name]; ok && a&attrReadonly != 0 {
			return fmt.Errorf("%s: readonly variable", name)
		}
	}
	// If the variable exists in the current scope, update it there.
	if _, ok := e.vars[name]; ok {
		e.vars[name] = value
		return nil
	}
	// If the variable exists in an ancestor scope, update it there.
	for cur := e.parent; cur != nil; cur = cur.parent {
		if _, ok := cur.vars[name]; ok {
			cur.vars[name] = value
			return nil
		}
	}
	// New variable: create in root (global) scope so that function
	// assignments without 'local' are visible to callers.
	root := e
	for root.parent != nil {
		root = root.parent
	}
	root.vars[name] = value
	return nil
}

// SetGlobal sets a variable at the root scope (for export/readonly).
func (e *Env) SetGlobal(name, value string) error {
	root := e
	for root.parent != nil {
		root = root.parent
	}
	return root.Set(name, value)
}

// Unset removes a variable from the current and parent scopes.
// Returns an error if the variable is readonly (POSIX requires unset to fail on readonly vars).
func (e *Env) Unset(name string) error {
	if e.IsReadonly(name) {
		return fmt.Errorf("%s: cannot unset: readonly variable", name)
	}
	for cur := e; cur != nil; cur = cur.parent {
		delete(cur.vars, name)
	}
	return nil
}

// Export marks a variable as exported (creates it if absent).
func (e *Env) Export(name string) {
	root := e
	for root.parent != nil {
		root = root.parent
	}
	root.attrs[name] |= attrExport
}

// Readonly marks a variable as readonly.
func (e *Env) Readonly(name string, value *string) error {
	root := e
	for root.parent != nil {
		root = root.parent
	}
	if value != nil {
		if err := root.Set(name, *value); err != nil {
			return err
		}
	}
	root.attrs[name] |= attrReadonly
	return nil
}

// IsExported reports whether name is exported.
func (e *Env) IsExported(name string) bool {
	for cur := e; cur != nil; cur = cur.parent {
		if a, ok := cur.attrs[name]; ok && a&attrExport != 0 {
			return true
		}
	}
	return false
}

// Environ returns a []string slice of NAME=VALUE for all exported variables.
func (e *Env) Environ() []string {
	exported := make(map[string]string)
	// Walk from root to current so inner scopes override outer.
	var walk func(*Env)
	walk = func(env *Env) {
		if env.parent != nil {
			walk(env.parent)
		}
		for k, v := range env.vars {
			if env.IsExported(k) {
				exported[k] = v
			}
		}
	}
	walk(e)
	var out []string
	for k, v := range exported {
		out = append(out, k+"="+v)
	}
	return out
}

// AllVars returns all visible variables (for set/declare listing).
func (e *Env) AllVars() map[string]string {
	all := make(map[string]string)
	var walk func(*Env)
	walk = func(env *Env) {
		if env.parent != nil {
			walk(env.parent)
		}
		for k, v := range env.vars {
			all[k] = v
		}
	}
	walk(e)
	return all
}

// AllAttrs returns attribute bits for a variable (searching all scopes).
func (e *Env) AllAttrs(name string) int {
	for cur := e; cur != nil; cur = cur.parent {
		if a, ok := cur.attrs[name]; ok {
			return a
		}
	}
	return 0
}
