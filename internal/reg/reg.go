// Package reg defines the shared registration interface used by both
// the root package and internal/stdlib, breaking the circular dependency.
package reg

import "github.com/cwd-k2/gicel/internal/eval"

// Registrar is the interface for registering primitives and modules.
type Registrar interface {
	RegisterPrim(name string, impl eval.PrimImpl)
	RegisterModule(name string, source string) error
	// EnableRecursion enables rec/fix builtins for subsequent module compilation.
	// Trusted stdlib packs call this to use recursion in their GICEL source.
	EnableRecursion()
}

// Pack configures a Registrar with a coherent set of types, primitives, and modules.
type Pack func(Registrar) error
