// Package stdlib provides standard library packs for Gomputation.
package stdlib

import "github.com/cwd-k2/gomputation/internal/eval"

// Host is the registration interface that Engine implements.
// Stdlib packs use this to register primitives and modules without
// importing the root package (which would create a circular dependency).
type Host interface {
	RegisterPrim(name string, impl eval.PrimImpl)
	RegisterModule(name string, source string) error
}

// Pack configures a Host with a coherent set of types, primitives, and modules.
type Pack func(Host) error
