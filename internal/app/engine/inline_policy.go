// Cross-module inlining policy — whitelist and collection of transparent
// wrappers and instance dictionaries eligible for cross-module inlining.
// Separated from pipeline.go to isolate optimization policy from pipeline orchestration.
package engine

import (
	"github.com/cwd-k2/gicel/internal/compiler/optimize"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// collectUserBindings returns the set of non-generated binding names
// eligible for selective inlining.
func collectUserBindings(prog *ir.Program) map[string]bool {
	m := make(map[string]bool)
	for _, b := range prog.Bindings {
		if !b.Generated.IsGenerated() {
			m[b.Name] = true
		}
	}
	return m
}

// transparentInlineWhitelist enumerates Prelude bindings eligible for
// cross-module selective inlining. These are the "transparent wrappers"
// whose inlined forms reduce to simpler IR (e.g. $ x y → App x y).
//
// Inlining these lets the optimizer reduce every call site to the
// corresponding IR primitive (`$`/`&` to a direct `App`, `fix`/`rec`
// to `ir.Fix`, `force` to `ir.Force`, `pure`/`bind` to their
// respective Core nodes via betaReduce/bindPureElim) so that user code
// written against the first-class values compiles to the same bytecode
// as the syntactic special forms.
//
// The list is intentionally small and closed: arbitrary module bindings
// are NOT inlined across module boundaries because that would destroy
// source-attribution invariants that explain/diagnostic code relies on.
// Maintained manually — adding a new transparent primitive to the
// Prelude requires a corresponding entry here.
var transparentInlineWhitelist = map[string]bool{
	"$":     true,
	"&":     true,
	"fix":   true,
	"rec":   true,
	"force": true,
	"pure":  true,
	"bind":  true,
}

// collectExternalInlineBindings gathers the whitelisted transparent
// wrappers from imported modules so the optimizer can reduce their
// applied forms at call sites in the main program. The inliner applies
// its own size / non-recursive / lambda-body filters as a secondary
// guard, but the whitelist is the primary mechanism that keeps the
// scope of cross-module inlining narrow and predictable.
//
// Each ExternalBinding is keyed by (moduleName, bindingName) so the
// inliner's VarKey lookup matches qualified references emitted by the
// checker for imported identifiers.
func (pc *pipelineCtx) collectExternalInlineBindings() []optimize.ExternalBinding {
	if pc.pipelineFlags.noInline {
		return nil
	}
	entries := pc.store.Entries()
	if len(entries) == 0 {
		return nil
	}
	var out []optimize.ExternalBinding
	for _, e := range entries {
		if e.prog == nil {
			continue
		}
		for _, b := range e.prog.Bindings {
			if b.Generated.IsGenerated() {
				continue
			}
			if !transparentInlineWhitelist[b.Name] && !optimize.IsTransparentAlias(b.Expr) && !optimize.IsMethodSelector(b.Expr) {
				continue
			}
			out = append(out, optimize.ExternalBinding{
				Module: e.name,
				Name:   b.Name,
				Expr:   b.Expr,
			})
		}
	}
	return out
}

// collectExternalDictionaries gathers instance dictionaries from imported
// modules for demand-driven inlining at Case scrutinee sites.
func (pc *pipelineCtx) collectExternalDictionaries() map[string]optimize.ExternalBinding {
	if pc.pipelineFlags.noInline {
		return nil
	}
	entries := pc.store.Entries()
	dicts := make(map[string]optimize.ExternalBinding)
	for _, e := range entries {
		if e.prog == nil {
			continue
		}
		for _, b := range e.prog.Bindings {
			if !b.Generated.IsGenerated() {
				continue
			}
			core := b.Expr
			for {
				switch n := core.(type) {
				case *ir.TyLam:
					core = n.Body
					continue
				case *ir.Lam:
					core = n.Body
					continue
				}
				break
			}
			switch core.(type) {
			case *ir.Con, *ir.App:
				// Skip recursive dictionaries (self-referential bindings)
				// to prevent infinite inlining expansion in the optimizer.
				if _, selfRef := ir.FreeVars(b.Expr)[b.Name]; selfRef {
					continue
				}
				key := string(ir.QualifiedKey(e.name, b.Name))
				dicts[key] = optimize.ExternalBinding{
					Module: e.name,
					Name:   b.Name,
					Expr:   b.Expr,
				}
			}
		}
	}
	return dicts
}
