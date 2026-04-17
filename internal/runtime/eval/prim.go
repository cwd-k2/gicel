package eval

import (
	"context"
	"maps"
	"sort"
)

// Applier provides function application callbacks for host primitives.
// Apply handles single-argument application (the common case).
// ApplyN handles multi-argument application, eliminating intermediate
// partial application values and reducing VM boundary crossings.
// ForceEffectful drives a deferred effectful value (saturated effectful
// PrimVal or auto-force thunk) to completion without adding arguments.
// Required by handler primitives that receive raw suspended computations
// and need to execute the final deferred effectful step.
type Applier struct {
	Apply          func(fn Value, arg Value, capEnv CapEnv) (Value, CapEnv, error)
	ApplyN         func(fn Value, args []Value, capEnv CapEnv) (Value, CapEnv, error)
	ForceEffectful func(v Value, capEnv CapEnv) (Value, CapEnv, error)
}

// ApplierFrom constructs an Applier from a single-argument function.
// ApplyN is implemented by sequentially applying each argument.
// Intended for tests; the VM provides a native multi-argument implementation.
func ApplierFrom(apply func(fn Value, arg Value, capEnv CapEnv) (Value, CapEnv, error)) Applier {
	a := Applier{Apply: apply}
	a.ApplyN = func(fn Value, args []Value, capEnv CapEnv) (Value, CapEnv, error) {
		var err error
		for _, arg := range args {
			fn, capEnv, err = apply(fn, arg, capEnv)
			if err != nil {
				return nil, capEnv, err
			}
		}
		return fn, capEnv, nil
	}
	return a
}

// PrimImpl is the signature for host-provided primitive operations.
// The apply parameter enables higher-order primitives (e.g. foldl) to
// call back into the evaluator to apply function arguments.
type PrimImpl func(ctx context.Context, capEnv CapEnv, args []Value, apply Applier) (Value, CapEnv, error)

// PrimRegistry maps assumption names to their implementations.
//
// sortedNames is a lazy, mutation-invalidated cache of the lexicographically
// ordered keys of impls. The engine's fingerprint path calls SortedNames
// once per fresh Engine; without caching, each call re-allocates the slice
// and re-sorts. Register nils the cache so the next read rebuilds.
//
// Callers MUST NOT mutate the returned slice — it is shared with the
// registry. The contract is "read-only snapshot of sorted names".
type PrimRegistry struct {
	impls       map[string]PrimImpl
	sortedNames []string // nil = stale; rebuilt lazily on next SortedNames
}

// NewPrimRegistry creates an empty primitive registry.
func NewPrimRegistry() *PrimRegistry {
	return &PrimRegistry{impls: make(map[string]PrimImpl)}
}

// Register adds a primitive implementation.
func (r *PrimRegistry) Register(name string, impl PrimImpl) {
	r.impls[name] = impl
	r.sortedNames = nil // invalidate sorted-names cache
}

// Lookup retrieves a primitive by name.
func (r *PrimRegistry) Lookup(name string) (PrimImpl, bool) {
	impl, ok := r.impls[name]
	return impl, ok
}

// Clone returns a shallow copy of the registry, decoupled from the original.
// The sorted-names cache is not copied; the clone rebuilds on first access.
func (r *PrimRegistry) Clone() *PrimRegistry {
	c := &PrimRegistry{impls: make(map[string]PrimImpl, len(r.impls))}
	maps.Copy(c.impls, r.impls)
	return c
}

// SortedNames returns the registered prim names in lexicographic order.
// Used by callers (e.g. the engine's runtime cache fingerprint) that need
// a deterministic traversal of the registry.
//
// The returned slice is cached and shared; callers MUST NOT mutate it.
// Register invalidates the cache, so subsequent calls see the new name set.
func (r *PrimRegistry) SortedNames() []string {
	if r.sortedNames != nil {
		return r.sortedNames
	}
	names := make([]string, 0, len(r.impls))
	for name := range r.impls {
		names = append(names, name)
	}
	sort.Strings(names)
	r.sortedNames = names
	return names
}
