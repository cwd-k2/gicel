package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processCtImplication solves an implication constraint: enter inner level,
// install given equalities, solve inner wanteds, then partition residuals
// into floatable (promoted to outer) and stuck (local skolem/meta → error).
func (ch *Checker) processCtImplication(ct *CtImplication, outerResolutions map[string]ir.Core) {
	savedWorklist := ch.solver.worklist.Drain()

	// Build local skolem ID set before entering inner scope.
	skolemIDs := make(map[int]bool, len(ct.Skolems))
	for _, sk := range ct.Skolems {
		skolemIDs[sk.ID] = true
	}

	ch.solver.level++
	savedSolverLevel := ch.unifier.SolverLevel
	ch.unifier.SolverLevel = ch.solver.level

	for skolemID, ty := range ct.GivenEqs {
		ch.unifier.InstallGivenEq(skolemID, ty)
	}

	ch.solver.worklist.Load(ct.Wanteds)

	// Defer constraints that mention local skolems or unsolved metas —
	// these cannot be resolved at the inner level and need partitioning.
	shouldDefer := func(_ string, zonkedArgs []types.Type) bool {
		if sliceHasMeta(zonkedArgs) {
			return true
		}
		for _, arg := range zonkedArgs {
			if types.AnyType(arg, func(ty types.Type) bool {
				sk, ok := ty.(*types.TySkolem)
				return ok && skolemIDs[sk.ID]
			}) {
				return true
			}
		}
		return false
	}
	innerResolutions, innerResiduals := ch.solveWanteds(shouldDefer)
	for k, v := range innerResolutions {
		outerResolutions[k] = v
	}

	// Partition residuals: constraints mentioning local skolems or
	// inner-level metas are stuck (error); others float to outer scope.
	var floatable []Ct
	for _, r := range innerResiduals {
		if constraintMentionsLocal(ch, r, skolemIDs, ch.solver.level) {
			ch.addCodedError(diagnostic.ErrNoInstance, r.S,
				fmt.Sprintf("cannot resolve %s (mentions GADT-local type variables)",
					constraintKey(r.ClassName, ch.zonkAll(r.Args))))
		} else {
			floatable = append(floatable, r)
		}
	}

	for skolemID := range ct.GivenEqs {
		ch.unifier.RemoveGivenEq(skolemID)
	}
	ch.solver.level--
	ch.unifier.SolverLevel = savedSolverLevel
	ch.solver.worklist.Load(append(savedWorklist, floatable...))
}

// constraintMentionsLocal returns true if the constraint's type arguments
// mention any local skolem ID or any meta created at the given inner level.
func constraintMentionsLocal(ch *Checker, ct *CtClass, localSkolems map[int]bool, innerLevel int) bool {
	for _, arg := range ct.Args {
		zonked := ch.unifier.Zonk(arg)
		if typeHasLocal(zonked, localSkolems, innerLevel) {
			return true
		}
	}
	return false
}

// typeHasLocal walks a type and returns true if it contains a skolem
// with an ID in localSkolems, or an unsolved meta at the given level.
func typeHasLocal(t types.Type, localSkolems map[int]bool, level int) bool {
	return types.AnyType(t, func(ty types.Type) bool {
		switch x := ty.(type) {
		case *types.TySkolem:
			return localSkolems[x.ID]
		case *types.TyMeta:
			return x.Level >= level
		}
		return false
	})
}
