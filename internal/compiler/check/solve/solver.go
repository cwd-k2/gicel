package solve

import (
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

	// inaccessible is set to true when a given equality produces a
	// contradiction (e.g. Int ~ Bool). An inaccessible branch is
	// vacuously well-typed: remaining wanteds are skipped.
	inaccessible bool

	// hasStructuralEqError is set when a wanted equality produces a
	// structural failure (skolem rigid, occurs check). Subsequent
	// "no instance" errors involving skolem variables are suppressed
	// to avoid cascading diagnostics from a single root cause.
	hasStructuralEqError bool

	// inTrialOrProbe is true when execution is inside a WithTrial or
	// WithProbe callback. Emit and EmitClassConstraint panic if called
	// in this state, because the worklist has no save/restore mechanism
	// and speculative constraints would persist after rollback.
	inTrialOrProbe bool

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

// RestoreWorklistAppend appends previously saved constraints back onto the
// current worklist. Used when an inner scope's constraint resolution should
// not consume constraints that belong to the outer scope.
func (s *Solver) RestoreWorklistAppend(cts []Ct) {
	for _, ct := range cts {
		s.worklist.Push(ct)
	}
}

// Reactivate kicks out constraints blocked on the given meta and
// re-enqueues them for priority processing.
func (s *Solver) Reactivate(metaID int) {
	kicked := s.inertSet.KickOut(metaID)
	s.worklist.PushFront(kicked...)
}

// Emit pushes a constraint to the worklist for processing.
func (s *Solver) Emit(ct Ct) {
	if s.inTrialOrProbe {
		panic("solve: Emit called inside trial/probe scope — worklist has no rollback")
	}
	s.worklist.Push(ct)
}

// EmitGivenEq pushes a given equality to the front of the worklist
// for priority processing. Given equalities must be processed before
// wanteds so that skolem solutions are available for kick-out.
func (s *Solver) EmitGivenEq(ct *CtEq) {
	s.worklist.PushFront(ct)
}

// trialScope wraps env.WithTrial, setting the inTrialOrProbe flag so
// that Emit panics if accidentally called from the speculative callback.
func (s *Solver) trialScope(fn func() bool) bool {
	prev := s.inTrialOrProbe
	s.inTrialOrProbe = true
	result := s.env.WithTrial(fn)
	s.inTrialOrProbe = prev
	return result
}

// probeScope wraps env.WithProbe, setting the inTrialOrProbe flag.
func (s *Solver) probeScope(fn func() bool) bool {
	prev := s.inTrialOrProbe
	s.inTrialOrProbe = true
	result := s.env.WithProbe(fn)
	s.inTrialOrProbe = prev
	return result
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
//
// Residuals are typed as []*CtPlainClass: quantified constraints never
// defer (they resolve immediately), and constraint variables never
// defer (they are normalized to plain class form during processing).
func (s *Solver) SolveWanteds(
	shouldDefer func(className string, zonkedArgs []types.Type) bool,
) (map[string]ir.Core, []*CtPlainClass) {
	resolutions := make(map[string]ir.Core)
	var residuals []*CtPlainClass
	s.inertSet.Reset()
	s.ambiguityCache = nil // lazily allocated; zero-cost when shouldDefer is nil
	// Only reset the step budget at the top level. Nested SolveWanteds calls
	// (via processCtImplication / CheckWithLocalScope) share the outer budget
	// to prevent N × maxSteps blowup across implication scopes.
	if s.inertSet.ScopeDepth() == 0 {
		s.env.ResetSolverSteps()
	}

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
		case *CtPlainClass:
			s.processCtPlainClass(c, resolutions, &residuals, shouldDefer)
		case *CtVarClass:
			s.processCtVarClass(c, resolutions, &residuals, shouldDefer)
		case *CtQuantifiedClass:
			s.processCtQuantifiedClass(c, resolutions)
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
			s.resolveCtPlainClassKeyed(ct, key, resolutions)
		}
	}

	// Retry deferred residuals: metavariables may have been solved by
	// later constraints (e.g. row unification from put/get sequences).
	// Re-zonk and attempt resolution for any that are no longer ambiguous.
	if shouldDefer != nil && len(residuals) > 0 {
		var stillDeferred []*CtPlainClass
		for _, ct := range residuals {
			ct.Args = s.zonkAll(ct.Args)
			if shouldDefer(ct.ClassName, ct.Args) {
				stillDeferred = append(stillDeferred, ct)
			} else {
				key := constraintKey(ct.ClassName, ct.Args)
				s.resolveCtPlainClassKeyed(ct, key, resolutions)
			}
		}
		residuals = stillDeferred
	}

	s.inertSet.Reset()
	return resolutions, residuals
}

// processCtPlainClass handles a plain class constraint: zonk args,
// consult the resolution cache, defer on ambiguity, or resolve against
// the available instances. This is the canonical path — CtVarClass
// normalizes into this after zonking.
func (s *Solver) processCtPlainClass(
	ct *CtPlainClass,
	resolutions map[string]ir.Core,
	residuals *[]*CtPlainClass,
	shouldDefer func(className string, zonkedArgs []types.Type) bool,
) {
	ct.Args = s.zonkAll(ct.Args)

	// Build canonical key for cache lookup.
	key := constraintKey(ct.ClassName, ct.Args)

	// Check if the inert set already has an identical resolved constraint.
	if cachedPlaceholder := s.inertSet.LookupResolution(key); cachedPlaceholder != "" {
		if cachedExpr, ok := resolutions[cachedPlaceholder]; ok {
			resolutions[ct.Placeholder] = cachedExpr
			return
		}
	}

	// Defer if ambiguous at this point — let-generalization may lift
	// the constraint into the enclosing type later.
	if shouldDefer != nil && shouldDefer(ct.ClassName, ct.Args) {
		*residuals = append(*residuals, ct)
		return
	}

	s.resolveCtPlainClassKeyed(ct, key, resolutions)
}

// processCtVarClass normalizes an unresolved constraint variable into a
// plain class constraint by zonking and decomposing into (className, args),
// then delegates to processCtPlainClass. If the variable cannot be
// decomposed into a class head, ErrNoInstance is reported and the
// placeholder gets a sentinel resolution.
func (s *Solver) processCtVarClass(
	ct *CtVarClass,
	resolutions map[string]ir.Core,
	residuals *[]*CtPlainClass,
	shouldDefer func(className string, zonkedArgs []types.Type) bool,
) {
	cv := s.env.Zonk(ct.ConstraintVar)
	cn, cArgs, ok := types.DecomposeConstraintType(cv)
	if !ok {
		s.env.AddCodedError(diagnostic.ErrNoInstance, ct.S,
			"cannot resolve constraint variable "+types.Pretty(cv))
		resolutions[ct.Placeholder] = &ir.Var{Name: "<no-instance>", S: ct.S}
		return
	}
	plain := &CtPlainClass{
		Placeholder: ct.Placeholder,
		ClassName:   cn,
		Args:        cArgs,
		S:           ct.S,
	}
	s.processCtPlainClass(plain, resolutions, residuals, shouldDefer)
}

// processCtQuantifiedClass discharges a quantified constraint immediately
// via resolveQuantifiedConstraint. Quantified constraints never enter
// the inert set and never become residuals — evidence is a function
// from context dicts to a head dict, which is always constructible.
func (s *Solver) processCtQuantifiedClass(
	ct *CtQuantifiedClass,
	resolutions map[string]ir.Core,
) {
	resolutions[ct.Placeholder] = s.resolveQuantifiedConstraint(ct.Quantified, ct.S)
}

// resolveCtPlainClassKeyed resolves a plain class constraint, records
// the resolution, and inserts it into the inert set with a canonical
// cache key.
func (s *Solver) resolveCtPlainClassKeyed(ct *CtPlainClass, key string, resolutions map[string]ir.Core) {
	resolved := s.resolveInstance(ct.ClassName, ct.Args, ct.S)
	resolutions[ct.Placeholder] = resolved
	s.inertSet.InsertClass(ct, key)
}

// constraintKey builds a canonical key for a class constraint.
// Injective: distinct (className, zonked args) produce distinct keys.
func constraintKey(className string, args []types.Type) string {
	return types.TypeListKey(className, ' ', args)
}

// processCtEq handles a type equality constraint: Lhs ~ Rhs.
// Given equalities (from GADT refinement) are dispatched to processGivenEq.
// Wanted equalities are zonked and unified; stuck constraints are inserted
// into the inert set for re-activation when blocking metas are solved.
//
// Stuck detection distinguishes structural head mismatch from genuinely
// stuck constraints. When both heads are known (not metas), unification
// failure is a real type error — reported immediately even if decomposition
// metas exist deeper in the types. Only when a head is an unsolved meta
// (meaning the structural comparison cannot yet be performed) is the
// constraint deferred to the inert set.
func (s *Solver) processCtEq(ct *CtEq) {
	if ct.Flavor == CtGiven {
		s.processGivenEq(ct)
		return
	}
	// Unify handles zonking internally. Only zonk on the error path
	// (uncommon) to avoid redundant double-zonk on the success path.
	if err := s.env.Unify(ct.Lhs, ct.Rhs); err != nil {
		lhs := s.env.Zonk(ct.Lhs)
		rhs := s.env.Zonk(ct.Rhs)
		// If both heads are known (not metas), this is a genuine structural
		// mismatch — report immediately even if decomposition metas exist.
		if !headIsMeta(lhs) && !headIsMeta(rhs) {
			// CBPV adjunction: opposite-tag TyCBPV types (Computation vs Thunk)
			// are coercible when their structural components match. subsCheck
			// handles this at expression sites (inserting ir.Thunk / ir.Force);
			// here we handle the same coercion at the constraint level where
			// no IR wrapping is needed.
			if s.tryCBPVComponentUnify(lhs, rhs) {
				return
			}
			s.reportEqError(ct, lhs, rhs)
			return
		}
		// Potentially stuck: a head is an unsolved meta.
		lhsMetas := collectMetaIDs([]types.Type{lhs})
		rhsMetas := collectMetaIDs([]types.Type{rhs})
		blocking := append(lhsMetas, rhsMetas...)
		if len(blocking) > 0 {
			ct.Lhs = lhs
			ct.Rhs = rhs
			s.inertSet.InsertEq(ct, blocking)
			return
		}
		s.reportEqError(ct, lhs, rhs)
	}
}

// tryCBPVComponentUnify handles the CBPV adjunction at the constraint level.
// When an equality constraint involves opposite-tag TyCBPV types
// (Computation vs Thunk), the structural components (Pre, Post, Result,
// Grade) are unified in a trial scope. This mirrors tryCBPVCoercion in
// subsCheck but operates purely on types — no IR wrapping is needed
// because the bidirectional checker inserts Thunk/Force at expression sites.
func (s *Solver) tryCBPVComponentUnify(lhs, rhs types.Type) bool {
	l, ok1 := lhs.(*types.TyCBPV)
	r, ok2 := rhs.(*types.TyCBPV)
	if !ok1 || !ok2 || l.Tag == r.Tag {
		return false
	}
	return s.trialScope(func() bool {
		if err := s.env.Unify(l.Pre, r.Pre); err != nil {
			return false
		}
		if err := s.env.Unify(l.Post, r.Post); err != nil {
			return false
		}
		if err := s.env.Unify(l.Result, r.Result); err != nil {
			return false
		}
		// Grade duality: nil grade (ungraded form) is compatible with any grade.
		if l.Grade != nil && r.Grade != nil {
			if err := s.env.Unify(l.Grade, r.Grade); err != nil {
				return false
			}
		}
		return true
	})
}

// headIsMeta returns true if the type's outermost head constructor is an
// unsolved meta. For example:
//
//	?m         → true   (bare meta)
//	?m Int     → true   (meta applied to args)
//	Int        → false  (concrete head)
//	Int -> Bool → false (concrete head)
//	(?a -> ?b) → false  (arrow head, not meta)
func headIsMeta(t types.Type) bool {
	if _, ok := t.(*types.TyMeta); ok {
		return true
	}
	head, _ := types.AppSpineHead(t)
	_, ok := head.(*types.TyMeta)
	return ok
}

// reportEqError reports a type equality error using the constraint's Origin
// context when available, falling back to a generic message.
func (s *Solver) reportEqError(ct *CtEq, lhs, rhs types.Type) {
	s.hasStructuralEqError = true
	if ct.Origin != nil && ct.Origin.Code != 0 {
		s.env.AddCodedError(ct.Origin.Code, ct.S, ct.Origin.GetContext())
	} else if ct.Origin != nil && ct.Origin.GetContext() != "" {
		s.env.AddCodedError(diagnostic.ErrTypeMismatch, ct.S, ct.Origin.GetContext())
	} else {
		s.env.AddCodedError(diagnostic.ErrTypeMismatch, ct.S,
			"unsatisfiable type equality: "+types.Pretty(lhs)+" ~ "+types.Pretty(rhs))
	}
}

// processGivenEq handles a given equality from a GADT pattern refinement.
//
// Three cases:
//   - Skolem ~ concrete: install in skolemSoln for Zonk transparency,
//     record in inert set, and kick out any constraints mentioning the skolem.
//   - Concrete ~ concrete: attempt unification to check for contradiction.
//     If both sides are ground and don't match, mark the branch inaccessible.
//   - Skolem ~ skolem: record the equality; may enable transitive reasoning.
func (s *Solver) processGivenEq(ct *CtEq) {
	lhs := s.env.Zonk(ct.Lhs)
	rhs := s.env.Zonk(ct.Rhs)

	// Extract skolem from either side.
	sk, concrete := extractSkolemGiven(lhs, rhs)
	if sk != nil {
		// Check for contradictory given: if this skolem already has a
		// different solution, unify to detect contradiction rather than
		// silently overwriting the existing binding.
		existing := s.env.Zonk(sk)
		if existing != sk {
			// Skolem already solved. Check consistency with new given.
			if err := s.env.Unify(existing, concrete); err != nil {
				s.inaccessible = true
				return
			}
			// Consistent — no need to re-install.
		} else {
			// Fresh skolem: install for Zonk transparency.
			s.env.InstallGivenEq(sk.ID, concrete)
		}
		// Record in inert set for scope-aware cleanup.
		s.inertSet.InsertGiven(&CtEq{Lhs: lhs, Rhs: rhs, Flavor: CtGiven, S: ct.S})
		// Kick out any constraints whose type args mention this skolem.
		kicked := s.inertSet.KickOutMentioningSkolem(sk.ID)
		s.worklist.PushFront(kicked...)
		return
	}

	// Both concrete (or both skolems): check for contradiction.
	if err := s.env.Unify(lhs, rhs); err != nil {
		// If no metas are involved, this is a genuine contradiction.
		lhsMetas := collectMetaIDs([]types.Type{lhs})
		rhsMetas := collectMetaIDs([]types.Type{rhs})
		if len(lhsMetas) == 0 && len(rhsMetas) == 0 {
			s.inaccessible = true
		}
	}
}

// extractSkolemGiven extracts a skolem and the "other side" from a
// given equality. Returns (nil, nil) if neither side is a skolem.
func extractSkolemGiven(lhs, rhs types.Type) (*types.TySkolem, types.Type) {
	if sk, ok := lhs.(*types.TySkolem); ok {
		return sk, rhs
	}
	if sk, ok := rhs.(*types.TySkolem); ok {
		return sk, lhs
	}
	return nil, nil
}

// processCtFunEq handles a stuck type family equation from the worklist.
func (s *Solver) processCtFunEq(ct *CtFunEq) {
	zonked := s.zonkAll(ct.Args)
	result, reduced := s.env.ReduceTyFamily(ct.FamilyName, zonked, ct.S)
	if reduced {
		// Unify the reduced result with the result meta.
		// Failure implies conflicting reductions or a grade violation.
		if err := s.env.Unify(ct.ResultMeta, result); err != nil {
			if ct.OnFailure != nil {
				ct.OnFailure(ct.S, s.env.Zonk(ct.ResultMeta), s.env.Zonk(result))
			} else {
				s.env.AddCodedError(diagnostic.ErrTypeMismatch, ct.S,
					"type family "+ct.FamilyName+": reduced to "+
						types.Pretty(s.env.Zonk(result))+
						", but expected "+types.Pretty(s.env.Zonk(ct.ResultMeta)))
			}
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
// Returns the original slice if no type changed (pointer equality).
func (s *Solver) zonkAll(tys []types.Type) []types.Type {
	var result []types.Type // nil until first change (lazy alloc)
	for i, t := range tys {
		z := s.env.Zonk(t)
		if result == nil && z != t {
			result = make([]types.Type, len(tys))
			copy(result[:i], tys[:i])
		}
		if result != nil {
			result[i] = z
		}
	}
	if result == nil {
		return tys
	}
	return result
}
