package optimize

import "github.com/cwd-k2/gicel/internal/lang/ir"

// maxInlineSize is the maximum node count for a binding to be considered
// an inline candidate.
const maxInlineSize = 20

// inlineCandidate holds a binding eligible for inlining.
type inlineCandidate struct {
	body ir.Core
}

// ExternalBinding names a binding from an imported module that should
// be considered as an inline candidate. The module name is used to
// construct the qualified lookup key (via ir.QualifiedKey) so that
// only references from the same origin receive the inlined body.
type ExternalBinding struct {
	Module string
	Name   string
	Expr   ir.Core
}

// collectInlineCandidates scans program bindings (and optionally
// imported-module bindings) for small, non-recursive lambda definitions
// that can be safely inlined at call sites. Candidates are keyed by
// ir.VarKey so the inliner matches references regardless of whether
// they are local (`Name`) or module-qualified (`Module\x00Name`).
func collectInlineCandidates(prog *ir.Program, userBindings map[string]bool, external []ExternalBinding) map[string]*inlineCandidate {
	if len(userBindings) == 0 && len(external) == 0 {
		return nil
	}
	candidates := make(map[string]*inlineCandidate)
	if userBindings != nil {
		for _, b := range prog.Bindings {
			if !userBindings[b.Name] {
				continue
			}
			if b.Generated {
				continue
			}
			if !eligibleInlineBody(b.Expr, b.Name) {
				continue
			}
			candidates[b.Name] = &inlineCandidate{body: b.Expr}
		}
	}
	for _, eb := range external {
		key := ir.QualifiedKey(eb.Module, eb.Name)
		if _, exists := candidates[key]; exists {
			continue
		}
		if !eligibleInlineBody(eb.Expr, eb.Name) {
			continue
		}
		candidates[key] = &inlineCandidate{body: eb.Expr}
	}
	if len(candidates) == 0 {
		return nil
	}
	return candidates
}

// eligibleInlineBody applies the shared filters: the body must be a
// lambda (so beta-reduction can fire at the call site), small enough
// to avoid code bloat, and non-recursive in the self-name sense.
//
// NOTE: polymorphic Prelude wrappers like `($) :: \a b. ...` elaborate
// into `TyLam @a. TyLam @b. Lam f. Lam x. App f x`, which is NOT a
// direct `*ir.Lam`, so they currently do not qualify for inlining
// through this rule. Peeling the TyLam layers at candidate time and
// reducing the call-site TyApps would require type-level beta
// reduction (to keep ParamType annotations well-formed), which is a
// separate design concern. `$` reaches the checkFix fast path via
// the dedicated transparent-rewrite at the checker level
// (bidir.go:isDollarOp) instead.
func eligibleInlineBody(expr ir.Core, name string) bool {
	if _, ok := expr.(*ir.Lam); !ok {
		return false
	}
	if nodeSize(expr, maxInlineSize+1) > maxInlineSize {
		return false
	}
	fv := ir.FreeVars(expr)
	if _, recursive := fv[name]; recursive {
		return false
	}
	return true
}

// inlineRule returns a rewrite rule that replaces variable references
// with their inlined bodies. Candidates are matched by VarKey so the
// rule fires for both local and module-qualified references.
func inlineRule(candidates map[string]*inlineCandidate) func(ir.Core) ir.Core {
	if len(candidates) == 0 {
		return func(c ir.Core) ir.Core { return c }
	}
	return func(c ir.Core) ir.Core {
		v, ok := c.(*ir.Var)
		if !ok {
			return c
		}
		cand, ok := candidates[ir.VarKey(v)]
		if !ok {
			return c
		}
		return ir.Clone(cand.body)
	}
}
