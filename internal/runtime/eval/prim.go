package eval

import "maps"

import "context"

// Applier provides function application callbacks for host primitives.
// Apply handles single-argument application (the common case).
// ApplyN handles multi-argument application, eliminating intermediate
// partial application values and reducing VM boundary crossings.
type Applier struct {
	Apply  func(fn Value, arg Value, capEnv CapEnv) (Value, CapEnv, error)
	ApplyN func(fn Value, args []Value, capEnv CapEnv) (Value, CapEnv, error)
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

// Clone returns a shallow copy of the registry, decoupled from the original.
func (r *PrimRegistry) Clone() *PrimRegistry {
	c := &PrimRegistry{impls: make(map[string]PrimImpl, len(r.impls))}
	maps.Copy(c.impls, r.impls)
	return c
}
