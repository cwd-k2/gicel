package solve

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Solver holds mutable state for the constraint solver.
//
// Access to level and worklist should go through the methods below rather
// than direct field access. This ensures invariants (level non-negative,
// worklist save/restore matching) are maintained at API boundaries.
type Solver struct {
	worklist       Worklist
	inertSet       InertSet
	ambiguityCache map[string]bool // per-solveWanteds cache; lazily allocated

	// level tracks implication nesting depth for OutsideIn(X) touchability.
	// Incremented on entry to GADT branches (bidir_case.go) and higher-rank
	// forall scopes (bidir.go). freshMeta uses this to stamp metas with
	// their birth level. The companion field unifier.SolverLevel controls
	// touchability enforcement: metas with Level < SolverLevel are untouchable.
	level int

	env Env
}

// New creates a Solver backed by the given environment.
func New(e Env) *Solver {
	return &Solver{
		inertSet: NewInertSet(),
		env:      e,
	}
}

// EnterScope increments the solver level. Call ExitScope when leaving.
func (s *Solver) EnterScope() { s.level++ }

// ExitScope decrements the solver level.
// Panics if level would go negative (unpaired EnterScope/ExitScope).
func (s *Solver) ExitScope() {
	if s.level <= 0 {
		panic("internal: Solver.ExitScope called at level 0 (unpaired EnterScope/ExitScope)")
	}
	s.level--
}

// SaveWorklist drains the current worklist and returns its contents.
// The caller must eventually call RestoreWorklist to put constraints back.
func (s *Solver) SaveWorklist() []Ct { return s.worklist.Drain() }

// RestoreWorklist replaces the worklist contents.
func (s *Solver) RestoreWorklist(cts []Ct) { s.worklist.Load(cts) }

// Reactivate kicks out constraints blocked on the given meta and
// re-enqueues them for priority processing.
func (s *Solver) Reactivate(metaID int) {
	kicked := s.inertSet.KickOut(metaID)
	s.worklist.PushFront(kicked...)
}

// Emit pushes a constraint to the worklist for processing.
func (s *Solver) Emit(ct Ct) {
	s.worklist.Push(ct)
}

// Level returns the current implication nesting depth for touchability.
func (s *Solver) Level() int {
	return s.level
}

// RegisterStuckFunEq inserts a stuck type family equation into the inert
// set for later re-activation when blocking metas are solved.
func (s *Solver) RegisterStuckFunEq(ct *CtFunEq) {
	s.inertSet.InsertFunEq(ct)
}

// LookupAmbiguity checks the per-solve ambiguity cache.
func (s *Solver) LookupAmbiguity(key string) (result bool, found bool) {
	if s.ambiguityCache == nil {
		return false, false
	}
	result, found = s.ambiguityCache[key]
	return
}

// CacheAmbiguity stores an ambiguity result in the per-solve cache.
func (s *Solver) CacheAmbiguity(key string, result bool) {
	if s.ambiguityCache == nil {
		s.ambiguityCache = make(map[string]bool)
	}
	s.ambiguityCache[key] = result
}

// InertSet returns a pointer to the solver's inert set for direct inspection.
// Exposed for test access from the parent package.
func (s *Solver) InertSet() *InertSet {
	return &s.inertSet
}

// solveWanteds processes all constraints in the worklist, producing
// a placeholder → Core resolution map and residual constraints.
//
// shouldDefer governs whether a plain class constraint is deferred
// (returned as residual) or discharged. Nil means discharge all.
func (s *Solver) SolveWanteds(
	shouldDefer func(className string, zonkedArgs []types.Type) bool,
) (map[string]ir.Core, []*CtClass) {
	resolutions := make(map[string]ir.Core)
	var residuals []*CtClass
	s.inertSet.Reset()
	s.ambiguityCache = nil // lazily allocated; zero-cost when shouldDefer is nil
	s.env.ResetSolverSteps()

	for {
		ct, ok := s.worklist.Pop()
		if !ok {
			break
		}
		if err := s.env.SolverStep(); err != nil {
			s.env.AddCodedError(diagnostic.ErrSolverLimit, ct.ctSpan(),
				"constraint solver step limit exceeded (possible infinite loop or exponential blowup)")
			break
		}
		if s.env.CheckCancelled() {
			break
		}
		switch c := ct.(type) {
		case *CtClass:
			s.processCtClass(c, resolutions, &residuals, shouldDefer)
		case *CtFunEq:
			s.processCtFunEq(c)
		case *CtEq:
			s.processCtEq(c)
		case *CtImplication:
			s.processCtImplication(c, resolutions)
		}
	}

	// Collect remaining class constraints from inert set as residuals.
	for _, ct := range s.inertSet.CollectClassResiduals() {
		if shouldDefer != nil {
			zonkedArgs := s.zonkAll(ct.Args)
			if shouldDefer(ct.ClassName, zonkedArgs) {
				residuals = append(residuals, ct)
				continue
			}
		}
		// Discharge from inert set: resolve if not already resolved.
		if _, exists := resolutions[ct.Placeholder]; !exists {
			key := constraintKey(ct.ClassName, ct.Args)
			s.resolveCtClassKeyed(ct, key, resolutions)
		}
	}

	// Retry deferred residuals: metavariables may have been solved by
	// later constraints (e.g. row unification from put/get sequences).
	// Re-zonk and attempt resolution for any that are no longer ambiguous.
	if shouldDefer != nil && len(residuals) > 0 {
		var stillDeferred []*CtClass
		for _, ct := range residuals {
			ct.Args = s.zonkAll(ct.Args)
			if shouldDefer(ct.ClassName, ct.Args) {
				stillDeferred = append(stillDeferred, ct)
			} else {
				key := constraintKey(ct.ClassName, ct.Args)
				s.resolveCtClassKeyed(ct, key, resolutions)
			}
		}
		residuals = stillDeferred
	}

	s.inertSet.Reset()
	return resolutions, residuals
}

// processCtClass handles a single class constraint from the worklist.
func (s *Solver) processCtClass(
	ct *CtClass,
	resolutions map[string]ir.Core,
	residuals *[]*CtClass,
	shouldDefer func(className string, zonkedArgs []types.Type) bool,
) {
	// Branch A: quantified constraint.
	if ct.Quantified != nil {
		resolutions[ct.Placeholder] = s.resolveQuantifiedConstraint(ct.Quantified, ct.S)
		return
	}

	// Branch B: constraint variable.
	if ct.ConstraintVar != nil {
		cv := s.env.Zonk(ct.ConstraintVar)
		cn, cArgs, ok := types.DecomposeConstraintType(cv)
		if ok {
			ct.ClassName = cn
			ct.Args = cArgs
		} else if ct.ClassName != "" {
			ct.Args = s.zonkAll(ct.Args)
		} else {
			s.env.AddCodedError(diagnostic.ErrNoInstance, ct.S,
				fmt.Sprintf("cannot resolve constraint variable %s", types.Pretty(cv)))
			resolutions[ct.Placeholder] = &ir.Var{Name: "<no-instance>", S: ct.S}
			return
		}
	} else {
		// Branch C: plain className + args.
		ct.Args = s.zonkAll(ct.Args)
	}

	// Build canonical key for cache lookup.
	key := constraintKey(ct.ClassName, ct.Args)

	// Check if the inert set already has an identical resolved constraint.
	if cachedPlaceholder := s.inertSet.LookupResolution(key); cachedPlaceholder != "" {
		if cachedExpr, ok := resolutions[cachedPlaceholder]; ok {
			resolutions[ct.Placeholder] = cachedExpr
			return
		}
	}

	// Check defer predicate for non-quantified constraints.
	// Applies to both plain (Branch C) and normalized ConstraintVar (Branch B)
	// constraints — both have ClassName/Args populated at this point.
	if ct.Quantified == nil && shouldDefer != nil {
		if shouldDefer(ct.ClassName, ct.Args) {
			*residuals = append(*residuals, ct)
			return
		}
	}

	// Resolve the constraint.
	s.resolveCtClassKeyed(ct, key, resolutions)
}

// resolveCtClassKeyed resolves a class constraint, records the resolution,
// and inserts it into the inert set with a canonical cache key.
func (s *Solver) resolveCtClassKeyed(ct *CtClass, key string, resolutions map[string]ir.Core) {
	resolved := s.resolveInstance(ct.ClassName, ct.Args, ct.S)
	resolutions[ct.Placeholder] = resolved
	s.inertSet.InsertClass(ct, key)
}

// constraintKey builds a canonical key for a class constraint.
// Injective: distinct (className, zonked args) produce distinct keys.
func constraintKey(className string, args []types.Type) string {
	var b strings.Builder
	b.Grow(len(className) + len(args)*16)
	b.WriteString(className)
	for _, a := range args {
		b.WriteByte(' ')
		types.WriteTypeKey(&b, a)
	}
	return b.String()
}

// processCtEq handles a type equality constraint: Lhs ~ Rhs.
// Zonks both sides and attempts unification. If either side contains
// unsolved metas (stuck), the constraint is inserted into the inert set
// for re-activation when blocking metas are solved.
func (s *Solver) processCtEq(ct *CtEq) {
	lhs := s.env.Zonk(ct.Lhs)
	rhs := s.env.Zonk(ct.Rhs)
	if err := s.env.Unify(lhs, rhs); err != nil {
		// Check if stuck (contains metas) vs genuinely unsatisfiable.
		lhsMetas := collectMetaIDs([]types.Type{lhs})
		rhsMetas := collectMetaIDs([]types.Type{rhs})
		blocking := make([]int, 0, len(lhsMetas)+len(rhsMetas))
		blocking = append(blocking, lhsMetas...)
		blocking = append(blocking, rhsMetas...)
		if len(blocking) > 0 {
			// Stuck: register in inert set for re-activation.
			ct.Lhs = lhs
			ct.Rhs = rhs
			s.inertSet.InsertEq(ct, blocking)
			return
		}
		// Genuinely unsatisfiable equality.
		s.env.AddCodedError(diagnostic.ErrTypeMismatch, ct.S,
			fmt.Sprintf("unsatisfiable type equality: %s ~ %s",
				types.Pretty(lhs), types.Pretty(rhs)))
	}
}

// processCtFunEq handles a stuck type family equation from the worklist.
func (s *Solver) processCtFunEq(ct *CtFunEq) {
	zonked := s.zonkAll(ct.Args)
	result, reduced := s.env.ReduceTyFamily(ct.FamilyName, zonked, ct.S)
	if reduced {
		// Unify the reduced result with the result meta.
		// Failure implies conflicting reductions or a grade violation.
		if err := s.env.Unify(ct.ResultMeta, result); err != nil && ct.OnFailure != nil {
			ct.OnFailure(ct.S, s.env.Zonk(ct.ResultMeta), s.env.Zonk(result))
		}
		return
	}
	// Still stuck: update args and re-register in inert set.
	ct.Args = zonked
	ct.BlockingOn = collectMetaIDs(zonked)
	if len(ct.BlockingOn) > 0 {
		s.inertSet.InsertFunEq(ct)
	}
}

// zonkAll applies Zonk to each type in the slice.
func (s *Solver) zonkAll(tys []types.Type) []types.Type {
	result := make([]types.Type, len(tys))
	for i, t := range tys {
		result[i] = s.env.Zonk(t)
	}
	return result
}
