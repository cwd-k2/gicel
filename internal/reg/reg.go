// Package reg defines the shared registration interface used by both
// the root package and internal/stdlib, breaking the circular dependency.
package reg

import "github.com/cwd-k2/gicel/internal/eval"

// Registrar is the interface for registering primitives and modules.
type Registrar interface {
	RegisterPrim(name string, impl eval.PrimImpl)
	RegisterModule(name string, source string) error
	// RegisterModuleRec compiles a module with fix/rec enabled, scoped
	// to this single compilation. The recursion gate is saved before
	// and restored after, so subsequent compilations on the same engine
	// are not affected. This preserves the sandbox security boundary.
	RegisterModuleRec(name string, source string) error
}

// Pack configures a Registrar with a coherent set of types, primitives, and modules.
type Pack func(Registrar) error
