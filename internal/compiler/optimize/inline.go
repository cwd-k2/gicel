package optimize

import "github.com/cwd-k2/gicel/internal/lang/ir"

// maxInlineSize is the maximum node count for a binding to be considered
// an inline candidate.
const maxInlineSize = 20

// maxEvidenceInlineSize is the maximum node count for evidence bindings
// (instance dictionaries and class method selectors). Evidence is larger
// than typical inline candidates because dictionary constructors carry
// all method implementations, but inlining them exposes case-of-known-
// constructor reductions that dissolve the dictionary entirely.
const maxEvidenceInlineSize = 60

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
// imported-module bindings) for small, non-recursive definitions
// that can be safely inlined at call sites. Candidates are keyed by
// ir.VarKey so the inliner matches references regardless of whether
// they are local (`Name`) or module-qualified (`Module\x00Name`).
//
// Three categories of candidates:
//   - User bindings: small lambdas (existing behavior)
//   - External bindings: small lambdas from imported modules
//   - Evidence bindings: Generated instance dictionaries and class
//     method selectors. Inlining these exposes case-of-known-constructor
//     (R1) which dissolves dictionary dispatch at compile time.
func collectInlineCandidates(prog *ir.Program, userBindings map[string]bool, external []ExternalBinding) map[string]*inlineCandidate {
	candidates := make(map[string]*inlineCandidate)

	// Evidence bindings: always scan, regardless of userBindings.
	// Instance dictionaries (Con applications) and method selectors
	// (TyLam/Lam/Case) are compile-time constants whose inlining
	// enables algebraic simplification.
	for _, b := range prog.Bindings {
		if !b.Generated {
			continue
		}
		if !eligibleEvidenceBody(b.Expr, b.Name) {
			continue
		}
		candidates[b.Name] = &inlineCandidate{body: b.Expr}
	}

	// User bindings: small lambdas.
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

	// External bindings: small lambdas and transparent aliases from
	// imported modules.
	for _, eb := range external {
		key := ir.QualifiedKey(eb.Module, eb.Name)
		if _, exists := candidates[key]; exists {
			continue
		}
		if !eligibleInlineBody(eb.Expr, eb.Name) && !eligibleTransparentAlias(eb.Expr) && !eligibleEvidenceBody(eb.Expr, eb.Name) {
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

// eligibleTransparentAlias reports whether an expression is a pure
// forwarding reference that can always be inlined. Peels TyLam
// wrappers (from polymorphic elaboration) and checks that the core
// is a single Var. These have minimal code bloat (the TyLam/TyApp
// pairs are eliminated by tyAppBeta after inlining).
func eligibleTransparentAlias(expr ir.Core) bool {
	core := expr
	for {
		if tl, ok := core.(*ir.TyLam); ok {
			core = tl.Body
			continue
		}
		break
	}
	switch n := core.(type) {
	case *ir.Var:
		return true
	case *ir.PrimOp:
		return len(n.Args) == 0 && !n.Effectful
	}
	return false
}

// eligibleEvidenceBody checks whether a Generated binding is an evidence
// candidate (instance dictionary or method selector) eligible for inlining.
//
// Evidence takes two forms:
//   - Dictionary: Con(DictName, args...) optionally wrapped in Lam/TyLam
//     for context parameters. Inlining exposes case-of-known-constructor.
//   - Selector: TyLam* → Lam $d → Case $d { DictCon ... → field }
//     Inlining exposes beta-reduction then case-of-known-constructor.
//
// Unlike user bindings, evidence need not start with *ir.Lam — it may
// start with *ir.TyLam, *ir.Con, or *ir.App. The size limit is larger
// (maxEvidenceInlineSize) because dictionaries carry all fields but
// simplify away after R1 fires.
func eligibleEvidenceBody(expr ir.Core, name string) bool {
	if nodeSize(expr, maxEvidenceInlineSize+1) > maxEvidenceInlineSize {
		return false
	}
	fv := ir.FreeVars(expr)
	if _, recursive := fv[name]; recursive {
		return false
	}
	// Must have evidence structure: peel TyLam/Lam wrappers and check
	// that the core is a Con application (dictionary) or Case (selector).
	core := expr
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
	case *ir.Con, *ir.App, *ir.Case:
		return true
	}
	return false
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
