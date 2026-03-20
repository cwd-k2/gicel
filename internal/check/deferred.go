package check

import (
	"slices"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// emitClassConstraint records a class constraint by pushing it to the worklist.
func (ch *Checker) emitClassConstraint(
	placeholder, className string,
	args []types.Type,
	s span.Span,
	quantified *types.QuantifiedConstraint,
	constraintVar types.Type,
) {
	ch.solver.worklist.Push(&CtClass{
		Placeholder:   placeholder,
		ClassName:     className,
		Args:          args,
		S:             s,
		Quantified:    quantified,
		ConstraintVar: constraintVar,
	})
}

// resolveDeferredConstraints discharges all worklist constraints eagerly.
// Used in check mode where the type is already known and every constraint
// must be resolved immediately.
func (ch *Checker) resolveDeferredConstraints(expr core.Core) core.Core {
	resolutions, _ := ch.solveWanteds(nil)
	return ch.substitutePlaceholders(expr, resolutions)
}

// resolveDeferredConstraintsDeferrable resolves worklist constraints but
// returns ambiguous plain-args constraints as residuals instead of forcing
// them. Used in infer mode so let-generalization can lift residuals into
// \-qualified types.
func (ch *Checker) resolveDeferredConstraintsDeferrable(expr core.Core) (core.Core, []*CtClass) {
	shouldDefer := func(className string, zonkedArgs []types.Type) bool {
		return sliceHasMeta(zonkedArgs) && ch.isAmbiguousInstance(className, zonkedArgs)
	}
	resolutions, residuals := ch.solveWanteds(shouldDefer)
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

// sliceHasMeta returns true if any type in the slice contains an unsolved TyMeta.
func sliceHasMeta(tys []types.Type) bool {
	return slices.ContainsFunc(tys, typeHasMeta)
}

func typeHasMeta(ty types.Type) bool {
	return types.AnyType(ty, func(t types.Type) bool {
		_, ok := t.(*types.TyMeta)
		return ok
	})
}

// isAmbiguousInstance checks whether a class constraint with the given args
// could match more than one instance. All trial unifications are rolled back
// to avoid committing any solutions. Results are cached per-solveWanteds scope.
func (ch *Checker) isAmbiguousInstance(className string, args []types.Type) bool {
	key := constraintKey(className, args)
	if ch.solver.ambiguityCache != nil {
		if cached, ok := ch.solver.ambiguityCache[key]; ok {
			return cached
		}
	}

	matchCount := 0
	seen := make(map[*InstanceInfo]bool)
	for _, inst := range ch.reg.instancesByClass[className] {
		if seen[inst] {
			continue
		}
		seen[inst] = true
		if len(inst.TypeArgs) != len(args) {
			continue
		}
		freshSubst := ch.freshInstanceSubst(inst)
		saved := ch.saveState()
		ok := true
		for i := range args {
			instArg := types.SubstMany(inst.TypeArgs[i], freshSubst)
			if err := ch.unifier.Unify(instArg, args[i]); err != nil {
				ok = false
				break
			}
		}
		ch.restoreState(saved)
		if ok {
			matchCount++
			if matchCount > 1 {
				break
			}
		}
	}

	result := matchCount > 1
	if ch.solver.ambiguityCache == nil {
		ch.solver.ambiguityCache = make(map[string]bool)
	}
	ch.solver.ambiguityCache[key] = result
	return result
}

// substitutePlaceholders replaces Var nodes matching placeholders via core.Transform.
func (ch *Checker) substitutePlaceholders(expr core.Core, resolutions map[string]core.Core) core.Core {
	if len(resolutions) == 0 {
		return expr
	}
	return core.Transform(expr, func(c core.Core) core.Core {
		if v, ok := c.(*core.Var); ok {
			if resolved, ok := resolutions[v.Name]; ok {
				return resolved
			}
		}
		return c
	})
}
