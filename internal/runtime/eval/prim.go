package eval

import "context"

// Applier is a callback that applies a function value to an argument.
// It allows primitives to invoke closures and other callable values,
// threading through the capability environment.
type Applier func(fn Value, arg Value, capEnv CapEnv) (Value, CapEnv, error)

// PrimImpl is the signature for host-provided primitive operations.
// The apply parameter enables higher-order primitives (e.g. foldl) to
// call back into the evaluator to apply function arguments.
type PrimImpl func(ctx context.Context, capEnv CapEnv, args []Value, apply Applier) (Value, CapEnv, error)

// PrimRegistry maps assumption names to their implementations.
type PrimRegistry struct {
	impls map[string]PrimImpl
}

// NewPrimRegistry creates an empty primitive reg.
func NewPrimRegistry() *PrimRegistry {
	return &PrimRegistry{impls: make(map[string]PrimImpl)}
}

// Register adds a primitive implementation.
func (r *PrimRegistry) Register(name string, impl PrimImpl) {
	r.impls[name] = impl
}

// Lookup retrieves a primitive by name.
func (r *PrimRegistry) Lookup(name string) (PrimImpl, bool) {
	impl, ok := r.impls[name]
	return impl, ok
}

// Clone returns a shallow copy of the registry, decoupled from the original.
func (r *PrimRegistry) Clone() *PrimRegistry {
	c := &PrimRegistry{impls: make(map[string]PrimImpl, len(r.impls))}
	for k, v := range r.impls {
		c.impls[k] = v
	}
	return c
}
