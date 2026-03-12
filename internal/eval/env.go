package eval

// Env is a lexically-scoped variable environment.
// Immutable: Extend returns a new Env with the additional binding.
type Env struct {
	bindings map[string]Value
	parent   *Env
}

// EmptyEnv creates an empty environment.
func EmptyEnv() *Env {
	return &Env{bindings: make(map[string]Value)}
}

// Extend returns a new Env with an additional binding.
func (e *Env) Extend(name string, val Value) *Env {
	m := map[string]Value{name: val}
	return &Env{bindings: m, parent: e}
}

// ExtendMany returns a new Env with multiple bindings.
func (e *Env) ExtendMany(bindings map[string]Value) *Env {
	if len(bindings) == 0 {
		return e
	}
	m := make(map[string]Value, len(bindings))
	for k, v := range bindings {
		m[k] = v
	}
	return &Env{bindings: m, parent: e}
}

// Lookup searches for a variable, walking up the parent chain.
func (e *Env) Lookup(name string) (Value, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if v, ok := cur.bindings[name]; ok {
			return v, true
		}
	}
	return nil, false
}
