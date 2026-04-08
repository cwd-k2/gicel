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
// placeholder is the deferred dictionary name and sp is the source span for errors.
//
// Equality entries are not routed through this path — the checker emits them
// directly as CtEq constraints. Passing a non-class variant here is a checker
// bug, so it panics to surface the issue loudly during development.
func (s *Solver) EmitClassConstraint(placeholder string, entry types.ConstraintEntry, sp span.Span) {
	ct := &CtClass{Placeholder: placeholder, S: sp}
	switch e := entry.(type) {
	case *types.ClassEntry:
		ct.ClassName = e.ClassName
		ct.Args = e.Args
	case *types.VarEntry:
		ct.ConstraintVar = e.Var
	case *types.QuantifiedConstraint:
		ct.Quantified = e
		if e.Head != nil {
			ct.ClassName = e.Head.ClassName
			ct.Args = e.Head.Args
		}
	default:
		panic("EmitClassConstraint: unexpected entry variant")
	}
	s.Emit(ct)
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
		ps := types.PrepareSubst(freshSubst)
		matched := s.env.WithProbe(func() bool {
			for i := range args {
				instArg := ps.Apply(inst.TypeArgs[i])
				if err := s.env.Unify(instArg, args[i]); err != nil {
					return false
				}
			}
			return true
		})
		if matched {
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
				// Clone to prevent shared mutable nodes: AssignIndices
				// mutates Var.Index in place, so each substitution site
				// must have an independent copy.
				return ir.Clone(resolved)
			}
		}
		return c
	})
}
