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
// Aliasing invariant: Push/PushMany use Go's append for amortized O(1) extension.
// Sibling scopes may share a backing array, but this is safe because:
//   - Evaluation is sequential — sibling sub-evaluations run one at a time.
//   - Closures always go through Capture or CaptureAll, which create an
//     independent copy. This is the only way an Env survives beyond its
//     creating scope.
//
// This is analogous to GHC STG's flat closure: the environment itself is
// a transient stack, and closure creation extracts an independent snapshot.
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

// Push appends a single value to the local stack (amortized O(1) via append).
func (e *Env) Push(val Value) *Env {
	return &Env{globals: e.globals, locals: append(e.locals, val)}
}

// PushMany appends multiple values to the local stack.
func (e *Env) PushMany(vals []Value) *Env {
	if len(vals) == 0 {
		return e
	}
	return &Env{globals: e.globals, locals: append(e.locals, vals...)}
}

// Capture creates a closure environment by extracting specific local values
// at the given de Bruijn indices. extraCap pre-allocates additional capacity
// for subsequent Push calls (1 for Lam application, 1 for Fix self-ref),
// so that entering the closure does not trigger a backing-array growth.
//
// Returns an Env with the same globals but a fresh locals slice — breaking
// any backing-array aliasing from prior Push calls.
func (e *Env) Capture(fvIndices []int, extraCap int) *Env {
	n := len(fvIndices)
	if n == 0 && extraCap == 0 {
		return &Env{globals: e.globals}
	}
	locals := make([]Value, n, n+extraCap)
	for i, idx := range fvIndices {
		locals[i] = e.locals[len(e.locals)-1-idx]
	}
	return &Env{globals: e.globals, locals: locals}
}

// CaptureAll creates a closure environment that copies all current locals.
// Used when FV annotation overflowed (FV == nil → FVIndices == nil).
// extraCap pre-allocates capacity for subsequent Push calls.
func (e *Env) CaptureAll(extraCap int) *Env {
	n := len(e.locals)
	if n == 0 && extraCap == 0 {
		return &Env{globals: e.globals}
	}
	cp := make([]Value, n, n+extraCap)
	copy(cp, e.locals)
	return &Env{globals: e.globals, locals: cp}
}

// Globals returns the underlying globals map (for use by runtime assembly).
func (e *Env) Globals() map[string]Value {
	return e.globals
}

// Extend adds a named binding to the globals map. Used during env setup
// (initBuiltinEnv, evalBindingsCore), NOT during evaluation hot path.
func (e *Env) Extend(name string, val Value) *Env {
	e.globals[name] = val
	return e
}

// Len returns the total number of bindings (globals + locals).
func (e *Env) Len() int {
	return len(e.globals) + len(e.locals)
}
