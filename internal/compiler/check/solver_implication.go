package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processCtImplication solves an implication constraint: enter inner level,
// install given equalities, solve inner wanteds, then partition residuals
// into floatable (promoted to outer) and stuck (local skolem/meta → error).
func (ch *Checker) processCtImplication(ct *CtImplication, outerResolutions map[string]ir.Core) {
	savedWorklist := ch.solver.worklist.Drain()

	// Build local skolem ID set before entering inner scope.
	localSkolems := make(map[int]bool, len(ct.Skolems))
	for _, sk := range ct.Skolems {
		localSkolems[sk.ID] = true
	}

	// SolverLevel is set immediately here (unlike bidir_case.go which defers
	// it until after body check). This is safe because CtImplication carries
	// pre-collected Wanteds — no DK eager unification occurs inside.
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
				return ok && localSkolems[sk.ID]
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

	floatable := ch.partitionResiduals(innerResiduals, localSkolems, ch.solver.level)

	for skolemID := range ct.GivenEqs {
		ch.unifier.RemoveGivenEq(skolemID)
	}
	ch.solver.level--
	ch.unifier.SolverLevel = savedSolverLevel
	ch.solver.worklist.Load(append(savedWorklist, floatable...))
}

// checkWithLocalScope checks expr against expected inside an implication scope.
// Increments solver.level so new metas are at inner level, then solves
// constraints with touchability enforced. Residuals are partitioned: stuck
// constraints produce errors, floatable ones are merged into the outer worklist.
//
// SolverLevel is deferred until after body check — DK eager unification
// during check must be free to solve outer metas (e.g. case result type).
func (ch *Checker) checkWithLocalScope(expr syntax.Expr, expected types.Type, skolemIDs map[int]string) ir.Core {
	savedWorklist := ch.solver.worklist.Drain()

	ch.solver.level++

	bodyCore := ch.check(expr, expected)

	// Enable touchability for constraint solving only.
	savedSolverLevel := ch.unifier.SolverLevel
	ch.unifier.SolverLevel = ch.solver.level

	resolutions, residuals := ch.solveWanteds(nil)
	bodyCore = ch.substitutePlaceholders(bodyCore, resolutions)

	localSkolems := make(map[int]bool, len(skolemIDs))
	for id := range skolemIDs {
		localSkolems[id] = true
	}
	floatable := ch.partitionResiduals(residuals, localSkolems, ch.solver.level)

	ch.unifier.SolverLevel = savedSolverLevel
	ch.solver.level--
	ch.solver.worklist.Load(append(savedWorklist, floatable...))
	return bodyCore
}

// partitionResiduals splits residual constraints into floatable (promoted
// to the outer scope) and stuck (mentions local skolems or inner-level
// metas → error). Stuck constraints are reported as ErrNoInstance.
func (ch *Checker) partitionResiduals(residuals []*CtClass, localSkolems map[int]bool, level int) []Ct {
	var floatable []Ct
	for _, r := range residuals {
		if constraintMentionsLocal(ch, r, localSkolems, level) {
			ch.addCodedError(diagnostic.ErrNoInstance, r.S,
				fmt.Sprintf("cannot resolve %s (mentions GADT-local type variables)",
					constraintKey(r.ClassName, ch.zonkAll(r.Args))))
		} else {
			floatable = append(floatable, r)
		}
	}
	return floatable
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
