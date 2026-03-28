package solve

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processCtImplication solves an implication constraint: enter inner level,
// install given equalities, solve inner wanteds, then partition residuals
// into floatable (promoted to outer) and stuck (local skolem/meta → error).
//
// GADT branches currently use CheckWithLocalScope which handles DK body
// checking inline. This function processes pre-collected Wanteds (no DK
// interleaving), so SolverLevel is set immediately. It will be activated
// once EnterGenerationScope / ExitGenerationScope collect body constraints
// and the checker wraps them as CtImplication wanteds.
func (s *Solver) processCtImplication(ct *CtImplication, outerResolutions map[string]ir.Core) {
	savedWorklist := s.SaveWorklist()

	// Build local skolem ID set before entering inner scope.
	localSkolems := make(map[int]bool, len(ct.Skolems))
	for _, sk := range ct.Skolems {
		localSkolems[sk.ID] = true
	}

	// SolverLevel is set immediately here (unlike CheckWithLocalScope which defers
	// it until after body check). This is safe because CtImplication carries
	// pre-collected Wanteds — no DK eager unification occurs inside.
	s.EnterScope()
	s.inertSet.EnterScope()
	savedSolverLevel := s.env.SolverLevel()
	s.env.SetSolverLevel(s.Level())

	for skolemID, ty := range ct.GivenEqs {
		s.env.InstallGivenEq(skolemID, ty)
		s.EmitGivenEq(&CtEq{
			Lhs:    &types.TySkolem{ID: skolemID},
			Rhs:    ty,
			Flavor: CtGiven,
			S:      ct.S,
		})
	}

	s.RestoreWorklist(ct.Wanteds)

	innerResolutions, innerResiduals := s.SolveWanteds(localShouldDefer())
	for k, v := range innerResolutions {
		outerResolutions[k] = v
	}

	floatable := s.partitionResiduals(innerResiduals, localSkolems, s.Level())

	for skolemID := range ct.GivenEqs {
		s.env.RemoveGivenEq(skolemID)
	}
	s.inertSet.LeaveScope()
	s.ExitScope()
	s.env.SetSolverLevel(savedSolverLevel)
	s.RestoreWorklist(append(savedWorklist, floatable...))
}

// CheckWithLocalScope checks expr against expected inside an implication scope.
// Increments solver.level so new metas are at inner level, then solves
// constraints with touchability enforced. Residuals are partitioned: stuck
// constraints produce errors, floatable ones are merged into the outer worklist.
//
// SolverLevel is deferred until after body check — DK eager unification
// during check must be free to solve outer metas (e.g. case result type).
//
// Known limitation: if check() triggers constraint solving internally
// (e.g. via checkWithEvidence → resolveDeferredConstraints), that solving
// runs before SolverLevel is raised. This is acceptable because such
// constraints are scoped to the evidence body, not the GADT result type.
// A full separation of unification and solving (OutsideIn-style) would
// eliminate this gap but conflicts with DK interleaving.
func (s *Solver) CheckWithLocalScope(checkBody func(types.Type) ir.Core, expected types.Type, skolemIDs map[int]string) ir.Core {
	savedWorklist := s.SaveWorklist()

	s.EnterScope()
	s.inertSet.EnterScope()

	bodyCore := checkBody(expected)

	// Enable touchability for constraint solving only.
	// Deferred until after body check — DK eager unification during check
	// must be free to solve outer metas (e.g. case result type).
	savedSolverLevel := s.env.SolverLevel()
	s.env.SetSolverLevel(s.Level())

	// Defer constraints with unsolved metas or local skolems — these
	// cannot be resolved at the inner level (instance resolution via
	// withTrial would bypass touchability). Deferred constraints are
	// partitioned below: local ones produce errors, others float out.
	localSkolems := make(map[int]bool, len(skolemIDs))
	for id := range skolemIDs {
		localSkolems[id] = true
	}
	resolutions, residuals := s.SolveWanteds(localShouldDefer())
	bodyCore = SubstitutePlaceholders(bodyCore, resolutions)

	floatable := s.partitionResiduals(residuals, localSkolems, s.Level())

	s.env.SetSolverLevel(savedSolverLevel)
	s.inertSet.LeaveScope()
	s.ExitScope()
	s.RestoreWorklist(append(savedWorklist, floatable...))
	return bodyCore
}

// localShouldDefer returns a shouldDefer predicate that defers constraints
// whose args contain unsolved metas. Used by processCtImplication and
// CheckWithLocalScope to prevent inner-level resolution of constraints
// that belong to the outer scope. Constraints with local skolems are NOT
// deferred — they may be resolvable via given evidence from the GADT
// pattern. Unresolvable local-skolem constraints become residuals and
// are caught by partitionResiduals.
func localShouldDefer() func(string, []types.Type) bool {
	return func(_ string, zonkedArgs []types.Type) bool {
		return SliceHasMeta(zonkedArgs)
	}
}

// partitionResiduals splits residual constraints into floatable (promoted
// to the outer scope) and stuck (mentions local skolems or inner-level
// metas → error). Stuck constraints are reported as ErrNoInstance.
func (s *Solver) partitionResiduals(residuals []*CtClass, localSkolems map[int]bool, level int) []Ct {
	var floatable []Ct
	for _, r := range residuals {
		if s.constraintMentionsLocal(r, localSkolems, level) {
			s.env.AddCodedError(diagnostic.ErrNoInstance, r.S,
				"cannot resolve "+constraintKey(r.ClassName, s.zonkAll(r.Args))+" (mentions GADT-local type variables)")
		} else {
			floatable = append(floatable, r)
		}
	}
	return floatable
}

// constraintMentionsLocal returns true if the constraint's type arguments
// mention any local skolem ID or any meta created at the given inner level.
func (s *Solver) constraintMentionsLocal(ct *CtClass, localSkolems map[int]bool, innerLevel int) bool {
	for _, arg := range ct.Args {
		zonked := s.env.Zonk(arg)
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
