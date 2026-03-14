// Package reg defines the shared registration interface used by both
// the root package and internal/stdlib, breaking the circular dependency.
package reg

import "github.com/cwd-k2/gomputation/internal/eval"

// Registrar is the interface for registering primitives and modules.
type Registrar interface {
	RegisterPrim(name string, impl eval.PrimImpl)
	RegisterModule(name string, source string) error
}

// Pack configures a Registrar with a coherent set of types, primitives, and modules.
type Pack func(Registrar) error
