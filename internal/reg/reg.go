// Package reg defines the shared registration interface used by both
// the root package and internal/stdlib, breaking the circular dependency.
package reg

import (
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/eval"
)

// RewriteRule is a bottom-up rewrite function applied at each Core IR node
// during optimization. If the rule does not apply, it must return the input
// unchanged. Stdlib packs use this to define domain-specific fusion rules.
type RewriteRule = func(core.Core) core.Core

// Registrar is the interface for registering primitives, modules, and rewrite rules.
type Registrar interface {
	RegisterPrim(name string, impl eval.PrimImpl)
	RegisterModule(name string, source string) error
	// RegisterModuleRec compiles a module with fix/rec enabled, scoped
	// to this single compilation. The recursion gate is saved before
	// and restored after, so subsequent compilations on the same engine
	// are not affected. This preserves the sandbox security boundary.
	RegisterModuleRec(name string, source string) error
	// RegisterRewriteRule adds a fusion rule to the optimization pipeline.
	// Rules are applied bottom-up after algebraic simplifications.
	RegisterRewriteRule(rule RewriteRule)
}

// Pack configures a Registrar with a coherent set of types, primitives, and modules.
type Pack func(Registrar) error
