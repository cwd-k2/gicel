package types

// PreparedSubst pre-computes state for applying the same substitution to
// multiple types, avoiding repeated fvUnion computation across calls.
//
// PreparedSubst handles type substitutions only; callers needing level
// substitution should use SubstMany directly per call.
type PreparedSubst struct {
	subs    map[string]Type
	fvUnion map[string]bool
	fvDone  bool
}

// PrepareSubst creates a PreparedSubst for batch application.
func PrepareSubst(subs map[string]Type) *PreparedSubst {
	return &PreparedSubst{subs: subs}
}

// Apply applies the prepared substitution to a type.
// The fvUnion is computed lazily on the first TyForall encounter and
// shared across all subsequent Apply calls on the same PreparedSubst.
func (ps *PreparedSubst) Apply(t Type) Type {
	if len(ps.subs) == 0 {
		return t
	}
	return substManyOpt(t, ps.subs, nil, &ps.fvUnion, 0)
}

// substManyFVUnion computes the free variable union of all substitution
// values. Called lazily when capture avoidance is needed (TyForall).
func substManyFVUnion(subs map[string]Type) map[string]bool {
	fvUnion := make(map[string]bool)
	for _, v := range subs {
		for name := range FreeVars(v) {
			fvUnion[name] = true
		}
	}
	return fvUnion
}

func substManyOpt(t Type, subs map[string]Type, levelSubs map[string]LevelExpr, fvUnion *map[string]bool, depth int) Type {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	switch ty := t.(type) {
	case *TyVar:
		if repl, ok := subs[ty.Name]; ok {
			return repl
		}
		return ty
	case *TyCon:
		if len(levelSubs) == 0 {
			return ty
		}
		newLevel := substLevelExprMany(ty.Level, levelSubs)
		if newLevel == ty.Level {
			return ty
		}
		return &TyCon{Name: ty.Name, Level: newLevel, IsLabel: ty.IsLabel, S: ty.S}
	case *TyApp:
		newFun := substManyOpt(ty.Fun, subs, levelSubs, fvUnion, depth+1)
		newArg := substManyOpt(ty.Arg, subs, levelSubs, fvUnion, depth+1)
		if newFun == ty.Fun && newArg == ty.Arg {
			return ty
		}
		return &TyApp{Fun: newFun, Arg: newArg, IsGrade: ty.IsGrade, Flags: MetaFreeFlags(newFun, newArg), S: ty.S}
	case *TyArrow:
		newFrom := substManyOpt(ty.From, subs, levelSubs, fvUnion, depth+1)
		newTo := substManyOpt(ty.To, subs, levelSubs, fvUnion, depth+1)
		if newFrom == ty.From && newTo == ty.To {
			return ty
		}
		return &TyArrow{From: newFrom, To: newTo, Flags: MetaFreeFlags(newFrom, newTo), S: ty.S}
	case *TyForall:
		// Lazy-compute fvUnion on first TyForall encounter when type subs
		// are present. Level subs do not require fvUnion (no level capture
		// avoidance is performed; see SubstMany godoc).
		if len(subs) > 0 && *fvUnion == nil {
			*fvUnion = substManyFVUnion(subs)
		}
		var fv map[string]bool
		if *fvUnion != nil {
			fv = *fvUnion
		}
		newKind := substManyOpt(ty.Kind, subs, levelSubs, fvUnion, depth+1)
		// Remove shadowed variable from substitution.
		if _, shadowed := subs[ty.Var]; shadowed {
			reduced := make(map[string]Type, len(subs)-1)
			for k, v := range subs {
				if k != ty.Var {
					reduced[k] = v
				}
			}
			if len(reduced) == 0 && len(levelSubs) == 0 {
				if newKind == ty.Kind {
					return ty
				}
				return &TyForall{Var: ty.Var, Kind: newKind, Body: ty.Body, S: ty.S}
			}
			// Capture avoidance: use FV union for O(1) check.
			if fv[ty.Var] {
				fresh := freshName(ty.Var)
				body := substDepth(ty.Body, ty.Var, &TyVar{Name: fresh}, depth+1)
				body = substManyOpt(body, reduced, levelSubs, fvUnion, depth+1)
				return &TyForall{Var: fresh, Kind: newKind, Body: body, S: ty.S}
			}
			newBody := substManyOpt(ty.Body, reduced, levelSubs, fvUnion, depth+1)
			if newKind == ty.Kind && newBody == ty.Body {
				return ty
			}
			return &TyForall{Var: ty.Var, Kind: newKind, Body: newBody, S: ty.S}
		}
		// Not shadowed: capture avoidance via FV union (type subs only).
		if fv[ty.Var] {
			fresh := freshName(ty.Var)
			body := substDepth(ty.Body, ty.Var, &TyVar{Name: fresh}, depth+1)
			body = substManyOpt(body, subs, levelSubs, fvUnion, depth+1)
			return &TyForall{Var: fresh, Kind: newKind, Body: body, S: ty.S}
		}
		newBody := substManyOpt(ty.Body, subs, levelSubs, fvUnion, depth+1)
		if newKind == ty.Kind && newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: newKind, Body: newBody, S: ty.S}
	case *TyCBPV:
		newPre := substManyOpt(ty.Pre, subs, levelSubs, fvUnion, depth+1)
		newPost := substManyOpt(ty.Post, subs, levelSubs, fvUnion, depth+1)
		newResult := substManyOpt(ty.Result, subs, levelSubs, fvUnion, depth+1)
		newGrade := ty.Grade
		if newGrade != nil {
			newGrade = substManyOpt(newGrade, subs, levelSubs, fvUnion, depth+1)
		}
		if newPre == ty.Pre && newPost == ty.Post && newResult == ty.Result && newGrade == ty.Grade {
			return ty
		}
		return &TyCBPV{Tag: ty.Tag, Pre: newPre, Post: newPost, Result: newResult, Grade: newGrade, Flags: MetaFreeFlags(newPre, newPost, newResult, newGrade), S: ty.S}
	case *TyEvidenceRow:
		return substManyEvidenceRow(ty, subs, levelSubs, fvUnion, depth+1)
	case *TyEvidence:
		newConstraints := substManyEvidenceRow(ty.Constraints, subs, levelSubs, fvUnion, depth+1)
		newBody := substManyOpt(ty.Body, subs, levelSubs, fvUnion, depth+1)
		if newConstraints == ty.Constraints && newBody == ty.Body {
			return ty
		}
		return &TyEvidence{Constraints: newConstraints, Body: newBody, S: ty.S}
	case *TyFamilyApp:
		var newArgs []Type // nil until first change
		for i, a := range ty.Args {
			sa := substManyOpt(a, subs, levelSubs, fvUnion, depth+1)
			if newArgs == nil && sa != a {
				newArgs = make([]Type, len(ty.Args))
				copy(newArgs[:i], ty.Args[:i])
			}
			if newArgs != nil {
				newArgs[i] = sa
			}
		}
		if newArgs == nil {
			return ty
		}
		return &TyFamilyApp{Name: ty.Name, Args: newArgs, Kind: ty.Kind, Flags: metaFreeSlice(ty.Kind, newArgs) &^ FlagNoFamilyApp, S: ty.S}
	default:
		return ty
	}
}

func substManyEvidenceRow(row *TyEvidenceRow, subs map[string]Type, levelSubs map[string]LevelExpr, fvUnion *map[string]bool, depth int) *TyEvidenceRow {
	if row == nil {
		return nil
	}
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	newEntries, changed := row.Entries.SubstEntriesMany(subs, levelSubs, fvUnion, depth+1)
	var newTail Type
	if row.Tail != nil {
		newTail = substManyOpt(row.Tail, subs, levelSubs, fvUnion, depth+1)
		if newTail != row.Tail {
			changed = true
		}
	}
	if !changed {
		return row
	}
	return &TyEvidenceRow{Entries: newEntries, Tail: newTail, Flags: EvidenceRowFlags(newEntries, newTail), S: row.S}
}

// substTypeSlice applies substDepth to every element of ts, returning the
// original slice unchanged (and false) when no element was modified.
func substTypeSlice(ts []Type, varName string, replacement Type, depth int) ([]Type, bool) {
	var out []Type // nil until first change (lazy alloc)
	for j, t := range ts {
		newT := substDepth(t, varName, replacement, depth)
		if out == nil && newT != t {
			out = make([]Type, len(ts))
			copy(out[:j], ts[:j])
		}
		if out != nil {
			out[j] = newT
		}
	}
	if out == nil {
		return ts, false
	}
	return out, true
}

// substManyTypeSlice applies substManyOpt to every element of ts, returning
// the original slice unchanged (and false) when no element was modified.
// Multi-variable counterpart of substTypeSlice.
func substManyTypeSlice(ts []Type, subs map[string]Type, levelSubs map[string]LevelExpr, fvUnion *map[string]bool, depth int) ([]Type, bool) {
	var out []Type // nil until first change (lazy alloc)
	for j, t := range ts {
		newT := substManyOpt(t, subs, levelSubs, fvUnion, depth)
		if out == nil && newT != t {
			out = make([]Type, len(ts))
			copy(out[:j], ts[:j])
		}
		if out != nil {
			out[j] = newT
		}
	}
	if out == nil {
		return ts, false
	}
	return out, true
}
