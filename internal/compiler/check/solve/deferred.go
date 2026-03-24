package solve

import (
	"slices"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// EmitClassConstraint records a class constraint by pushing it to the worklist.
// The entry carries the constraint's class, args, and optional quantifier/variable;
// placeholder is the deferred dictionary name and s is the source span for errors.
func (s *Solver) EmitClassConstraint(placeholder string, entry types.ConstraintEntry, sp span.Span) {
	s.Emit(&CtClass{
		Placeholder:   placeholder,
		ClassName:     entry.ClassName,
		Args:          entry.Args,
		S:             sp,
		Quantified:    entry.Quantified,
		ConstraintVar: entry.ConstraintVar,
	})
}

// ResolveDeferredConstraints discharges all worklist constraints eagerly.
// Used in check mode where the type is already known and every constraint
// must be resolved immediately.
func (s *Solver) ResolveDeferredConstraints(expr ir.Core) ir.Core {
	resolutions, _ := s.SolveWanteds(nil)
	return SubstitutePlaceholders(expr, resolutions)
}

// ResolveDeferredConstraintsDeferrable resolves worklist constraints but
// returns ambiguous plain-args constraints as residuals instead of forcing
// them. Used in infer mode so let-generalization can lift residuals into
// \-qualified types.
func (s *Solver) ResolveDeferredConstraintsDeferrable(expr ir.Core) (ir.Core, []*CtClass) {
	shouldDefer := func(className string, zonkedArgs []types.Type) bool {
		return SliceHasMeta(zonkedArgs) && s.isAmbiguousInstance(className, zonkedArgs)
	}
	resolutions, residuals := s.SolveWanteds(shouldDefer)
	return SubstitutePlaceholders(expr, resolutions), residuals
}

// SliceHasMeta returns true if any type in the slice contains an unsolved TyMeta.
func SliceHasMeta(tys []types.Type) bool {
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
func (s *Solver) isAmbiguousInstance(className string, args []types.Type) bool {
	key := constraintKey(className, args)
	if cached, found := s.LookupAmbiguity(key); found {
		return cached
	}

	matchCount := 0
	seen := make(map[*env.InstanceInfo]bool)
	for _, inst := range s.env.InstancesForClass(className) {
		if seen[inst] {
			continue
		}
		seen[inst] = true
		if len(inst.TypeArgs) != len(args) {
			continue
		}
		freshSubst := s.FreshInstanceSubst(inst)
		saved := s.env.SaveState()
		ok := true
		for i := range args {
			instArg := types.SubstMany(inst.TypeArgs[i], freshSubst)
			if err := s.env.Unify(instArg, args[i]); err != nil {
				ok = false
				break
			}
		}
		s.env.RestoreState(saved)
		if ok {
			matchCount++
			if matchCount > 1 {
				break
			}
		}
	}

	result := matchCount > 1
	s.CacheAmbiguity(key, result)
	return result
}

// SubstitutePlaceholders replaces Var nodes matching placeholders via ir.Transform.
func SubstitutePlaceholders(expr ir.Core, resolutions map[string]ir.Core) ir.Core {
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
