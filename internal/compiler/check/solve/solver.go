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

	// genScope, when non-nil, collects constraints emitted during DK
	// body checking for later wrapping as CtImplication wanteds.
	// Infrastructure for future OutsideIn(X) phase separation; not yet
	// activated (Emit does not divert to genScope). The checker will
	// use EnterGenerationScope / ExitGenerationScope to bracket body
	// checking once processCtImplication is fully integrated.
	genScope *generationScope

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
	s.worklist.Push(ct)
}

// EmitGivenEq pushes a given equality to the front of the worklist
// for priority processing. Given equalities must be processed before
// wanteds so that skolem solutions are available for kick-out.
func (s *Solver) EmitGivenEq(ct *CtEq) {
	s.worklist.PushFront(ct)
}

// IsInaccessible reports whether a contradictory given equality was
// detected (e.g. Int ~ Bool in a GADT branch), making the branch
// vacuously well-typed.
func (s *Solver) IsInaccessible() bool { return s.inaccessible }

// SetInaccessible sets or clears the inaccessible flag.
func (s *Solver) SetInaccessible(v bool) { s.inaccessible = v }

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
				"cannot resolve constraint variable "+types.Pretty(cv))
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
	lhs := s.env.Zonk(ct.Lhs)
	rhs := s.env.Zonk(ct.Rhs)
	if err := s.env.Unify(lhs, rhs); err != nil {
		// If both heads are known (not metas), this is a genuine structural
		// mismatch — report immediately even if decomposition metas exist.
		if !headIsMeta(lhs) && !headIsMeta(rhs) {
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
	head, _ := types.UnwindApp(t)
	_, ok := head.(*types.TyMeta)
	return ok
}

// reportEqError reports a type equality error using the constraint's Origin
// context when available, falling back to a generic message.
func (s *Solver) reportEqError(ct *CtEq, lhs, rhs types.Type) {
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

// generationScope collects constraints emitted during DK body checking.
// Used by EnterGenerationScope / ExitGenerationScope to bracket
// constraint collection for future CtImplication wrapping.
type generationScope struct {
	collected []Ct
}

// EnterGenerationScope begins collecting constraints for a DK body.
// Constraints emitted after this call (when the scope is eventually
// activated via Emit diversion) will be collected rather than pushed
// to the main worklist. Currently infrastructure-only: Emit is not
// yet modified to divert; the checker can call these methods to test
// the scope lifecycle without affecting constraint flow.
func (s *Solver) EnterGenerationScope() {
	s.genScope = &generationScope{}
}

// ExitGenerationScope ends constraint collection and returns all
// constraints gathered since the matching EnterGenerationScope.
// Returns nil if no generation scope was active.
func (s *Solver) ExitGenerationScope() []Ct {
	if s.genScope == nil {
		return nil
	}
	cts := s.genScope.collected
	s.genScope = nil
	return cts
}

// GenerationScopeActive reports whether a generation scope is active.
func (s *Solver) GenerationScopeActive() bool {
	return s.genScope != nil
}

// zonkAll applies Zonk to each type in the slice.
func (s *Solver) zonkAll(tys []types.Type) []types.Type {
	result := make([]types.Type, len(tys))
	for i, t := range tys {
		result[i] = s.env.Zonk(t)
	}
	return result
}
