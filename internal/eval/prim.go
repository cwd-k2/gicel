package eval

import "context"

// PrimImpl is the signature for host-provided primitive operations.
type PrimImpl func(ctx context.Context, capEnv CapEnv, args []Value) (Value, CapEnv, error)

// PrimRegistry maps assumption names to their implementations.
type PrimRegistry struct {
	impls map[string]PrimImpl
}

// NewPrimRegistry creates an empty primitive registry.
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
