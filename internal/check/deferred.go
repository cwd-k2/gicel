package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
)

// resolveDeferredConstraints discharges all deferred constraints eagerly.
// Used in check mode where the type is already known and every constraint
// must be resolved immediately.
func (ch *Checker) resolveDeferredConstraints(expr core.Core) core.Core {
	result, _ := ch.resolveDeferred(expr, nil)
	return result
}

// resolveDeferredConstraintsDeferrable resolves deferred constraints but
// returns ambiguous plain-args constraints as residuals instead of forcing
// them. Used in infer mode so let-generalization can lift residuals into
// forall-qualified types.
func (ch *Checker) resolveDeferredConstraintsDeferrable(expr core.Core) (core.Core, []deferredConstraint) {
	return ch.resolveDeferred(expr, func(className string, zonkedArgs []types.Type) bool {
		return hasMeta(zonkedArgs) && ch.isAmbiguousInstance(className, zonkedArgs)
	})
}

// resolveDeferred is the parameterized fold over ch.deferred.
//
// Each deferred constraint falls into one of three branches:
//
//	(A) quantified     → resolveQuantifiedConstraint
//	(B) constraintVar  → zonk, decompose, resolveInstance / fallback
//	(C) plain args     → zonkArgs, then ask shouldDefer
//
// shouldDefer governs branch (C): when non-nil and returning true, the
// constraint becomes a residual rather than being discharged. This mirrors
// the check/infer duality in bidirectional constraint resolution — check
// mode passes nil (discharge all), infer mode passes a predicate that
// defers ambiguous constraints containing unsolved metavariables.
//
// A single-pass fold preserves the sequential resolution order, which
// matters because earlier resolutions may solve metavariables that later
// constraints depend on.
func (ch *Checker) resolveDeferred(
	expr core.Core,
	shouldDefer func(className string, zonkedArgs []types.Type) bool,
) (core.Core, []deferredConstraint) {
	if len(ch.deferred) == 0 {
		return expr, nil
	}

	resolutions := make(map[string]core.Core)
	var residuals []deferredConstraint

	for _, dc := range ch.deferred {
		if dc.quantified != nil {
			// (A) Quantified constraint: resolve by finding matching evidence.
			resolved := ch.resolveQuantifiedConstraint(dc.quantified, dc.s)
			resolutions[dc.placeholder] = resolved
		} else if dc.constraintVar != nil {
			// (B) Constraint variable: zonk and decompose into className + args.
			cv := ch.unifier.Zonk(dc.constraintVar)
			cn, cArgs, ok := DecomposeConstraintType(cv)
			if ok {
				resolutions[dc.placeholder] = ch.resolveInstance(cn, cArgs, dc.s)
			} else if dc.className != "" {
				zonkedArgs := ch.zonkAll(dc.args)
				resolutions[dc.placeholder] = ch.resolveInstance(dc.className, zonkedArgs, dc.s)
			} else {
				ch.addCodedError(errs.ErrNoInstance, dc.s,
					fmt.Sprintf("cannot resolve constraint variable %s", types.Pretty(cv)))
				resolutions[dc.placeholder] = &core.Var{Name: "<no-instance>", S: dc.s}
			}
		} else {
			// (C) Plain className + args: the only branch where the mode matters.
			zonkedArgs := ch.zonkAll(dc.args)
			if shouldDefer != nil && shouldDefer(dc.className, zonkedArgs) {
				dc.args = zonkedArgs
				residuals = append(residuals, dc)
			} else {
				resolutions[dc.placeholder] = ch.resolveInstance(dc.className, zonkedArgs, dc.s)
			}
		}
	}

	ch.deferred = ch.deferred[:0]
	return ch.substitutePlaceholders(expr, resolutions), residuals
}

// zonkAll applies Zonk to each type in the slice.
func (ch *Checker) zonkAll(tys []types.Type) []types.Type {
	result := make([]types.Type, len(tys))
	for i, t := range tys {
		result[i] = ch.unifier.Zonk(t)
	}
	return result
}

// hasMeta returns true if any type in the slice contains an unsolved TyMeta.
func hasMeta(tys []types.Type) bool {
	for _, ty := range tys {
		if containsMeta(ty) {
			return true
		}
	}
	return false
}

func containsMeta(ty types.Type) bool {
	switch t := ty.(type) {
	case *types.TyMeta:
		return true
	case *types.TyApp:
		return containsMeta(t.Fun) || containsMeta(t.Arg)
	case *types.TyArrow:
		return containsMeta(t.From) || containsMeta(t.To)
	case *types.TyForall:
		return containsMeta(t.Body)
	case *types.TyEvidence:
		if t.Constraints != nil {
			for _, ch := range t.Constraints.Children() {
				if containsMeta(ch) {
					return true
				}
			}
		}
		return containsMeta(t.Body)
	case *types.TyComp:
		return containsMeta(t.Pre) || containsMeta(t.Post) || containsMeta(t.Result)
	case *types.TyEvidenceRow:
		for _, ch := range t.Children() {
			if containsMeta(ch) {
				return true
			}
		}
		return false
	}
	return false
}

// isAmbiguousInstance checks whether a class constraint with the given args
// could match more than one instance. All trial unifications are rolled back
// to avoid committing any solutions.
func (ch *Checker) isAmbiguousInstance(className string, args []types.Type) bool {
	matchCount := 0
	for _, inst := range ch.instancesByClass[className] {
		if len(inst.TypeArgs) != len(args) {
			continue
		}
		freshSubst := ch.freshInstanceSubst(inst)
		saved := ch.saveUnifierState()
		ok := true
		for i := range args {
			instArg := types.SubstMany(inst.TypeArgs[i], freshSubst)
			if err := ch.unifier.Unify(instArg, args[i]); err != nil {
				ok = false
				break
			}
		}
		ch.restoreUnifierState(saved)
		if ok {
			matchCount++
			if matchCount > 1 {
				return true
			}
		}
	}
	return false
}

// substitutePlaceholders replaces Var nodes matching placeholders via core.Transform.
func (ch *Checker) substitutePlaceholders(expr core.Core, resolutions map[string]core.Core) core.Core {
	return core.Transform(expr, func(c core.Core) core.Core {
		if v, ok := c.(*core.Var); ok {
			if resolved, ok := resolutions[v.Name]; ok {
				return resolved
			}
		}
		return c
	})
}
