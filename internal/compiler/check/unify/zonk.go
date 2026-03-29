package unify

import "github.com/cwd-k2/gicel/internal/lang/types"

// Zonk replaces all solved metavariables in a type.
// Optimizations:
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
	switch ty := t.(type) {
	case *types.TyMeta:
		soln, ok := u.soln[ty.ID]
		if !ok {
			return ty
		}
		result := u.zonkInner(soln)
		if result != soln {
			// Path compression: only trail when a snapshot is active
			// (trail entries outside snapshot scopes are never restored
			// and would leak memory over long compilations).
			if u.snapshotDepth > 0 {
				u.trailSolnWrite(ty.ID)
			}
			u.soln[ty.ID] = result
		}
		return result
	case *types.TyApp:
		zFun := u.zonkInner(ty.Fun)
		zArg := u.zonkInner(ty.Arg)
		if zFun == ty.Fun && zArg == ty.Arg {
			return ty
		}
		return &types.TyApp{Fun: zFun, Arg: zArg, S: ty.S}
	case *types.TyArrow:
		zFrom := u.zonkInner(ty.From)
		zTo := u.zonkInner(ty.To)
		if zFrom == ty.From && zTo == ty.To {
			return ty
		}
		return &types.TyArrow{From: zFrom, To: zTo, S: ty.S}
	case *types.TyForall:
		zKind := u.zonkInner(ty.Kind)
		zBody := u.zonkInner(ty.Body)
		if zKind == ty.Kind && zBody == ty.Body {
			return ty
		}
		return &types.TyForall{Var: ty.Var, Kind: zKind, Body: zBody, S: ty.S}
	case *types.TyCBPV:
		zPre := u.zonkInner(ty.Pre)
		zPost := u.zonkInner(ty.Post)
		zResult := u.zonkInner(ty.Result)
		if zPre == ty.Pre && zPost == ty.Post && zResult == ty.Result {
			return ty
		}
		return &types.TyCBPV{Tag: ty.Tag, Pre: zPre, Post: zPost, Result: zResult, S: ty.S}
	case *types.TyEvidenceRow:
		newEntries, changed := ty.Entries.ZonkEntries(u.zonkInner)
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
		return &types.TyEvidenceRow{Entries: newEntries, Tail: tail, S: ty.S}
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
			return &types.TyEvidence{Constraints: ty.Constraints, Body: zBody, S: ty.S}
		}
		return &types.TyEvidence{Constraints: cr, Body: zBody, S: ty.S}
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
		return &types.TyFamilyApp{Name: ty.Name, Args: args, Kind: zKind, S: ty.S}
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
