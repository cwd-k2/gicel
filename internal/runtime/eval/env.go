package eval

// Environment representation:
//
//   - globals: map[string]Value held on the Evaluator, set once before eval,
//     immutable during evaluation. Looked up by string key.
//
//   - locals: []Value (de Bruijn indexed array) for lexically-scoped bindings
//     (lambda params, bind vars, case pattern vars, fix names).
//     Index 0 = innermost binding. Lookup is locals[len(locals)-1-index], O(1).
//
// Aliasing invariant: Push/PushMany use Go's append for amortized O(1) extension.
// Sibling scopes may share a backing array, but this is safe because:
//   - Evaluation is sequential — sibling sub-evaluations run one at a time.
//   - Closures always go through Capture or CaptureAll, which create an
//     independent copy. This is the only way locals survive beyond their
//     creating scope.
//
// This is analogous to GHC STG's flat closure: the environment itself is
// a transient stack, and closure creation extracts an independent snapshot.

// LookupLocal returns the value at the given de Bruijn index in the local stack.
func LookupLocal(locals []Value, index int) Value {
	return locals[len(locals)-1-index]
}

// Push appends a single value to the local stack (amortized O(1) via append).
func Push(locals []Value, val Value) []Value {
	return append(locals, val)
}

// PushMany appends multiple values to the local stack.
func PushMany(locals []Value, vals []Value) []Value {
	if len(vals) == 0 {
		return locals
	}
	return append(locals, vals...)
}

// Capture creates a closure environment by extracting specific local values
// at the given de Bruijn indices. extraCap pre-allocates additional capacity
// for subsequent Push calls (1 for Lam application, 1 for Fix self-ref),
// so that entering the closure does not trigger a backing-array growth.
//
// Returns a fresh locals slice — breaking any backing-array aliasing from
// prior Push calls.
func Capture(locals []Value, fvIndices []int, extraCap int) []Value {
	n := len(fvIndices)
	if n == 0 && extraCap == 0 {
		return nil
	}
	result := make([]Value, n, n+extraCap)
	for i, idx := range fvIndices {
		result[i] = locals[len(locals)-1-idx]
	}
	return result
}

// CaptureAll creates a closure environment that copies all current locals.
// Used when FV annotation overflowed (FV == nil → FVIndices == nil).
// extraCap pre-allocates capacity for subsequent Push calls.
func CaptureAll(locals []Value, extraCap int) []Value {
	n := len(locals)
	if n == 0 && extraCap == 0 {
		return nil
	}
	cp := make([]Value, n, n+extraCap)
	copy(cp, locals)
	return cp
}
