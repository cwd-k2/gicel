package eval

// Env is a lexically-scoped variable environment with a hybrid representation:
//
//   - globals: shared flat map for module-level bindings (builtins, constructors,
//     top-level definitions). Looked up by string key. Set up once per execution,
//     never modified during evaluation.
//
//   - locals: de Bruijn indexed array for lexically-scoped bindings (lambda params,
//     bind vars, case pattern vars, fix names). Index 0 = innermost binding.
//     Lookup is locals[len(locals)-1-index], O(1) with zero allocation.
//
// This design eliminates map allocations from the evaluation hot path while
// keeping the simplicity of named lookup for global bindings whose layout
// depends on runtime assembly.
type Env struct {
	globals map[string]Value
	locals  []Value
}

// NewGlobalEnv creates an Env with the given globals map and no locals.
func NewGlobalEnv(globals map[string]Value) *Env {
	return &Env{globals: globals}
}

// EmptyEnv returns an Env with empty globals and no locals.
func EmptyEnv() *Env {
	return &Env{globals: map[string]Value{}}
}

// LookupLocal returns the value at the given de Bruijn index in the local stack.
func (e *Env) LookupLocal(index int) Value {
	return e.locals[len(e.locals)-1-index]
}

// LookupGlobal searches for a variable by key in the globals map.
func (e *Env) LookupGlobal(key string) (Value, bool) {
	v, ok := e.globals[key]
	return v, ok
}

// Push appends a single value to the local stack.
// Always copies to prevent aliasing with the parent's backing array.
func (e *Env) Push(val Value) *Env {
	newLocals := make([]Value, len(e.locals)+1)
	copy(newLocals, e.locals)
	newLocals[len(e.locals)] = val
	return &Env{globals: e.globals, locals: newLocals}
}

// PushMany appends multiple values to the local stack.
func (e *Env) PushMany(vals []Value) *Env {
	if len(vals) == 0 {
		return e
	}
	newLocals := make([]Value, len(e.locals)+len(vals))
	copy(newLocals, e.locals)
	copy(newLocals[len(e.locals):], vals)
	return &Env{globals: e.globals, locals: newLocals}
}

// Capture creates a closure environment by extracting specific local values
// at the given de Bruijn indices. The captured values are stored in order
// (FVIndices[0] first, FVIndices[len-1] last). Returns an Env with the
// same globals but a fresh locals slice.
func (e *Env) Capture(fvIndices []int) *Env {
	if len(fvIndices) == 0 {
		return &Env{globals: e.globals}
	}
	cap := make([]Value, len(fvIndices))
	for i, idx := range fvIndices {
		cap[i] = e.locals[len(e.locals)-1-idx]
	}
	return &Env{globals: e.globals, locals: cap}
}

// CaptureAll creates a closure environment that copies all current locals.
// Used when FV annotation overflowed (FVIndices == nil).
func (e *Env) CaptureAll() *Env {
	if len(e.locals) == 0 {
		return &Env{globals: e.globals}
	}
	cp := make([]Value, len(e.locals))
	copy(cp, e.locals)
	return &Env{globals: e.globals, locals: cp}
}

// Globals returns the underlying globals map (for use by runtime assembly).
func (e *Env) Globals() map[string]Value {
	return e.globals
}

// --- Legacy API (used by tests and explain system) ---

// Extend adds a named binding to the globals map. Used during env setup
// (initBuiltinEnv, evalBindingsCore), NOT during evaluation hot path.
func (e *Env) Extend(name string, val Value) *Env {
	e.globals[name] = val
	return e
}

// Lookup searches globals by name. Used by explain system and legacy paths.
func (e *Env) Lookup(name string) (Value, bool) {
	v, ok := e.globals[name]
	return v, ok
}

// Flatten is a no-op (kept for API compatibility during migration).
func (e *Env) Flatten() {}

// Len returns the total number of bindings (globals + locals).
func (e *Env) Len() int {
	return len(e.globals) + len(e.locals)
}
