package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
)

// resolveDeferredConstraints walks a Core expression and replaces
// placeholder dict variables with resolved instance dictionaries.
func (ch *Checker) resolveDeferredConstraints(expr core.Core) core.Core {
	if len(ch.deferred) == 0 {
		return expr
	}

	// Build resolution map: placeholder name → resolved Core expression.
	resolutions := make(map[string]core.Core)
	for _, dc := range ch.deferred {
		if dc.quantified != nil {
			// Resolve quantified constraint by finding matching evidence.
			resolved := ch.resolveQuantifiedConstraint(dc.quantified, dc.s)
			resolutions[dc.placeholder] = resolved
		} else if dc.constraintVar != nil {
			// Constraint variable: zonk and decompose into className + args.
			cv := ch.unifier.Zonk(dc.constraintVar)
			cn, cArgs, ok := DecomposeConstraintType(cv)
			if ok {
				resolved := ch.resolveInstance(cn, cArgs, dc.s)
				resolutions[dc.placeholder] = resolved
			} else {
				// Still unresolved — try using className/args if available.
				if dc.className != "" {
					zonkedArgs := make([]types.Type, len(dc.args))
					for i, a := range dc.args {
						zonkedArgs[i] = ch.unifier.Zonk(a)
					}
					resolved := ch.resolveInstance(dc.className, zonkedArgs, dc.s)
					resolutions[dc.placeholder] = resolved
				} else {
					ch.addCodedError(errs.ErrNoInstance, dc.s,
						fmt.Sprintf("cannot resolve constraint variable %s", types.Pretty(cv)))
					resolutions[dc.placeholder] = &core.Var{Name: "<no-instance>", S: dc.s}
				}
			}
		} else {
			zonkedArgs := make([]types.Type, len(dc.args))
			for i, a := range dc.args {
				zonkedArgs[i] = ch.unifier.Zonk(a)
			}
			resolved := ch.resolveInstance(dc.className, zonkedArgs, dc.s)
			resolutions[dc.placeholder] = resolved
		}
	}
	ch.deferred = ch.deferred[:0]

	return ch.substitutePlaceholders(expr, resolutions)
}

// resolveDeferredConstraintsDeferrable works like resolveDeferredConstraints
// but returns constraints whose zonked args contain unsolved metavariables
// instead of resolving them eagerly. Used by let-generalization to lift
// such constraints into forall-qualified types.
func (ch *Checker) resolveDeferredConstraintsDeferrable(expr core.Core) (core.Core, []deferredConstraint) {
	if len(ch.deferred) == 0 {
		return expr, nil
	}

	resolutions := make(map[string]core.Core)
	var unresolved []deferredConstraint
	for _, dc := range ch.deferred {
		if dc.quantified != nil {
			resolved := ch.resolveQuantifiedConstraint(dc.quantified, dc.s)
			resolutions[dc.placeholder] = resolved
		} else if dc.constraintVar != nil {
			cv := ch.unifier.Zonk(dc.constraintVar)
			cn, cArgs, ok := DecomposeConstraintType(cv)
			if ok {
				resolved := ch.resolveInstance(cn, cArgs, dc.s)
				resolutions[dc.placeholder] = resolved
			} else if dc.className != "" {
				zonkedArgs := make([]types.Type, len(dc.args))
				for i, a := range dc.args {
					zonkedArgs[i] = ch.unifier.Zonk(a)
				}
				resolved := ch.resolveInstance(dc.className, zonkedArgs, dc.s)
				resolutions[dc.placeholder] = resolved
			} else {
				ch.addCodedError(errs.ErrNoInstance, dc.s,
					fmt.Sprintf("cannot resolve constraint variable %s", types.Pretty(cv)))
				resolutions[dc.placeholder] = &core.Var{Name: "<no-instance>", S: dc.s}
			}
		} else {
			zonkedArgs := make([]types.Type, len(dc.args))
			for i, a := range dc.args {
				zonkedArgs[i] = ch.unifier.Zonk(a)
			}
			// If args contain unsolved metas AND the constraint is ambiguous
			// (multiple instances match), defer for let-generalization.
			// Otherwise resolve normally (which may solve metas via unification).
			if hasMeta(zonkedArgs) && ch.isAmbiguousInstance(dc.className, zonkedArgs) {
				dc.args = zonkedArgs
				unresolved = append(unresolved, dc)
			} else {
				resolved := ch.resolveInstance(dc.className, zonkedArgs, dc.s)
				resolutions[dc.placeholder] = resolved
			}
		}
	}
	ch.deferred = ch.deferred[:0]
	return ch.substitutePlaceholders(expr, resolutions), unresolved
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
