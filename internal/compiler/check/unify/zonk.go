package unify

import "github.com/cwd-k2/gicel/internal/lang/types"

// Zonk replaces all solved metavariables in a type.
// Optimizations:
//   - HasMeta fast path: types known to be meta-free skip the entire
//     budget infrastructure and return immediately. Profile (cold path)
//     showed the budget Nest/Unnest pair was a measurable share of the
//     Zonk wrapper cost on the meta-free fast path that dominates calls.
//   - Path compression: meta chains (m1 -> m2 -> Int) are compressed so
//     soln[m1] points directly to the final answer.
//   - Structural identity: if all children are unchanged (pointer-equal),
//     the original node is returned (avoids allocation).
//
// Budget.Nest/Unnest tracks the call-site nesting depth (how deeply the
// checker's own recursion invokes Zonk), not the internal type depth.
// Internal recursion uses zonkInner which is budget-free — the type's
// structural depth is bounded by the allocation limit, not the nesting limit.
func (u *Unifier) Zonk(t types.Type) types.Type {
	// Fast path: meta-free types need no zonking and no budget tracking.
	// HasMeta is O(1) for composites (FlagMetaFree check) and constant-
	// time for leaves, so this is strictly cheaper than the budget path.
	if !types.HasMeta(t) {
		return t
	}
	if u.Budget != nil {
		if err := u.Budget.Nest(); err != nil {
			return t // bail out, preserving the unzonked type
		}
		defer u.Budget.Unnest()
	}
	return u.zonkInner(t)
}

// zonkInner is the budget-free recursive core of Zonk.
func (u *Unifier) zonkInner(t types.Type) types.Type {
	// Fast path: types known to be meta-free need no walking.
	if !types.HasMeta(t) {
		return t
	}
	switch ty := t.(type) {
	case *types.TyMeta:
		soln, ok := u.lookupSoln(ty.ID)
		if !ok {
			return ty
		}
		result := u.zonkInner(soln)
		if result != soln && u.tempSoln == nil {
			// Path compression: shorten meta→…→value chains in the
			// permanent solution map. Suppressed when the temp overlay
			// is active (generalization) because the resolved value may
			// contain transient TyVar nodes that must not leak into soln.
			if u.snapshotDepth > 0 {
				u.trailSolnWrite(ty.ID)
			}
			u.soln[ty.ID] = result
		}
		return result
	case *types.TyApp:
		zFun := u.zonkInner(ty.Fun)
		zArg := u.zonkInner(ty.Arg)
		var result types.Type
		if zFun == ty.Fun && zArg == ty.Arg {
			result = ty
		} else {
			result = &types.TyApp{Fun: zFun, Arg: zArg, IsGrade: ty.IsGrade, Flags: types.MetaFreeFlags(zFun, zArg), S: ty.S}
		}
		// Try 4-arg (graded) normalization only — depth-4 Computation/Thunk
		// chains. The 3-arg ungraded form is deferred to normalizeCompApp
		// during unification, where the depth-3 chain is unambiguous; doing
		// it here would prematurely commit a partial 4-arg application
		// (still missing its result arg) to the ungraded interpretation.
		if norm := normalizeCompApp4Only(result); norm != result {
			return norm
		}
		return result
	case *types.TyArrow:
		zFrom := u.zonkInner(ty.From)
		zTo := u.zonkInner(ty.To)
		if zFrom == ty.From && zTo == ty.To {
			return ty
		}
		return &types.TyArrow{From: zFrom, To: zTo, Flags: types.MetaFreeFlags(zFrom, zTo), S: ty.S}
	case *types.TyForall:
		zKind := u.zonkInner(ty.Kind)
		zBody := u.zonkInner(ty.Body)
		if zKind == ty.Kind && zBody == ty.Body {
			return ty
		}
		return &types.TyForall{Var: ty.Var, Kind: zKind, Body: zBody, Flags: types.MetaFreeFlags(zKind, zBody), S: ty.S}
	case *types.TyCBPV:
		zPre := u.zonkInner(ty.Pre)
		zPost := u.zonkInner(ty.Post)
		zResult := u.zonkInner(ty.Result)
		zGrade := ty.Grade
		if zGrade != nil {
			zGrade = u.zonkInner(zGrade)
		}
		if zPre == ty.Pre && zPost == ty.Post && zResult == ty.Result && zGrade == ty.Grade {
			return ty
		}
		return &types.TyCBPV{Tag: ty.Tag, Pre: zPre, Post: zPost, Result: zResult, Grade: zGrade, Flags: types.MetaFreeFlags(zPre, zPost, zResult, zGrade), S: ty.S}
	case *types.TyEvidenceRow:
		// Use the lazily-bound zonkEntriesFn callback (allocated on the
		// first TyEvidenceRow zonk and cached on the Unifier) instead
		// of creating a fresh method-value closure on every visit.
		// Lazy binding keeps trial unifiers that never zonk evidence
		// rows allocation-free (Tier 4 micro benches noticed the
		// per-NewUnifier alloc when the binding was eager).
		if u.zonkEntriesFn == nil {
			u.zonkEntriesFn = u.zonkInner
		}
		newEntries, changed := ty.Entries.ZonkEntries(u.zonkEntriesFn)
		var tail types.Type
		if ty.Tail != nil {
			tail = u.zonkInner(ty.Tail)
			if tail != ty.Tail {
				changed = true
			}
		}
		if !changed {
			return ty
		}
		return &types.TyEvidenceRow{Entries: newEntries, Tail: tail, Flags: types.EvidenceRowFlags(newEntries, tail), S: ty.S}
	case *types.TyEvidence:
		zConstraints := u.zonkInner(ty.Constraints)
		zBody := u.zonkInner(ty.Body)
		if zConstraints == ty.Constraints && zBody == ty.Body {
			return ty
		}
		cr, ok := zConstraints.(*types.TyEvidenceRow)
		if !ok {
			// Zonk produced a non-evidence-row (e.g., solved meta);
			// preserve original constraints to avoid nil dereference.
			return &types.TyEvidence{Constraints: ty.Constraints, Body: zBody, Flags: types.MetaFreeFlags(ty.Constraints, zBody), S: ty.S}
		}
		return &types.TyEvidence{Constraints: cr, Body: zBody, Flags: types.MetaFreeFlags(cr, zBody), S: ty.S}
	case *types.TyFamilyApp:
		var args []types.Type // nil until first change (lazy-init)
		for i, a := range ty.Args {
			zA := u.zonkInner(a)
			if args == nil && zA != a {
				args = make([]types.Type, len(ty.Args))
				copy(args[:i], ty.Args[:i])
			}
			if args != nil {
				args[i] = zA
			}
		}
		zKind := u.zonkInner(ty.Kind)
		if args == nil && zKind == ty.Kind {
			return ty
		}
		if args == nil {
			args = ty.Args
		}
		// After resolving metas in arguments, the family application may
		// now be reducible. Try single-node reduction to avoid leaving
		// unreduced TyFamilyApp nodes that cause soundness issues.
		if u.TryReduceFamily != nil {
			if result, ok := u.TryReduceFamily(ty.Name, args, ty.S); ok {
				return u.zonkInner(result)
			}
		}
		return &types.TyFamilyApp{Name: ty.Name, Args: args, Kind: zKind, Flags: types.MetaFreeFlags(append(args, zKind)...) &^ types.FlagNoFamilyApp, S: ty.S}
	case *types.TyCon:
		// TyCon is usually a leaf, but Level may contain LevelMeta.
		if ty.Level == nil {
			return ty
		}
		zLevel := u.zonkLevel(ty.Level)
		if zLevel == ty.Level {
			return ty
		}
		return &types.TyCon{Name: ty.Name, Level: zLevel, S: ty.S}
	case *types.TySkolem:
		if u.skolemSoln != nil {
			if soln, ok := u.skolemSoln[ty.ID]; ok {
				return u.zonkInner(soln)
			}
		}
		return ty
	default:
		return t
	}
}
