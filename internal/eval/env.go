package eval

import "maps"

// Env is a lexically-scoped variable environment.
// Flat representation: each Env holds a complete map of bindings.
// Extend creates a new Env by copying and adding a binding.
// This avoids parent-chain retention that causes GC pressure in
// long-lived closures and thunks.
type Env struct {
	bindings map[string]Value
}

// EmptyEnv creates an empty environment.
func EmptyEnv() *Env {
	return &Env{bindings: make(map[string]Value)}
}

// Extend returns a new Env with an additional binding.
func (e *Env) Extend(name string, val Value) *Env {
	m := make(map[string]Value, len(e.bindings)+1)
	maps.Copy(m, e.bindings)
	m[name] = val
	return &Env{bindings: m}
}

// ExtendMany returns a new Env with multiple bindings.
func (e *Env) ExtendMany(bindings map[string]Value) *Env {
	if len(bindings) == 0 {
		return e
	}
	m := make(map[string]Value, len(e.bindings)+len(bindings))
	maps.Copy(m, e.bindings)
	maps.Copy(m, bindings)
	return &Env{bindings: m}
}

// Lookup searches for a variable.
func (e *Env) Lookup(name string) (Value, bool) {
	v, ok := e.bindings[name]
	return v, ok
}

// Len returns the number of bindings.
func (e *Env) Len() int {
	return len(e.bindings)
}
