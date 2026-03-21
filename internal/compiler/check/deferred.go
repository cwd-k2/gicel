package check

import (
	"slices"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// emitClassConstraint records a class constraint by pushing it to the worklist.
// The entry carries the constraint's class, args, and optional quantifier/variable;
// placeholder is the deferred dictionary name and s is the source span for errors.
func (ch *Checker) emitClassConstraint(placeholder string, entry types.ConstraintEntry, s span.Span) {
	ch.solver.Emit(&CtClass{
		Placeholder:   placeholder,
		ClassName:     entry.ClassName,
		Args:          entry.Args,
		S:             s,
		Quantified:    entry.Quantified,
		ConstraintVar: entry.ConstraintVar,
	})
}

// resolveDeferredConstraints discharges all worklist constraints eagerly.
// Used in check mode where the type is already known and every constraint
// must be resolved immediately.
func (ch *Checker) resolveDeferredConstraints(expr ir.Core) ir.Core {
	resolutions, _ := ch.solveWanteds(nil)
	return ch.substitutePlaceholders(expr, resolutions)
}

// resolveDeferredConstraintsDeferrable resolves worklist constraints but
// returns ambiguous plain-args constraints as residuals instead of forcing
// them. Used in infer mode so let-generalization can lift residuals into
// \-qualified types.
func (ch *Checker) resolveDeferredConstraintsDeferrable(expr ir.Core) (ir.Core, []*CtClass) {
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
	if cached, found := ch.solver.LookupAmbiguity(key); found {
		return cached
	}

	matchCount := 0
	seen := make(map[*InstanceInfo]bool)
	for _, inst := range ch.reg.InstancesForClass(className) {
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
	ch.solver.CacheAmbiguity(key, result)
	return result
}

// substitutePlaceholders replaces Var nodes matching placeholders via ir.Transform.
func (ch *Checker) substitutePlaceholders(expr ir.Core, resolutions map[string]ir.Core) ir.Core {
	if len(resolutions) == 0 {
		return expr
	}
	return ir.Transform(expr, func(c ir.Core) ir.Core {
		if v, ok := c.(*ir.Var); ok {
			if resolved, ok := resolutions[v.Name]; ok {
				return resolved
			}
		}
		return c
	})
}
