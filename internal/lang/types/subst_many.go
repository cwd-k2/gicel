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
	case *TyForall:
		if len(subs) > 0 && *fvUnion == nil {
			*fvUnion = substManyFVUnion(subs)
		}
		var fv map[string]bool
		if *fvUnion != nil {
			fv = *fvUnion
		}
		newKind := substManyOpt(ty.Kind, subs, levelSubs, fvUnion, depth+1)
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
				return &TyForall{Var: ty.Var, Kind: newKind, Body: ty.Body, Flags: MetaFreeFlags(newKind, ty.Body), S: ty.S}
			}
			if fv[ty.Var] {
				fresh := freshName(ty.Var)
				body := substDepth(ty.Body, ty.Var, &TyVar{Name: fresh}, depth+1)
				body = substManyOpt(body, reduced, levelSubs, fvUnion, depth+1)
				return &TyForall{Var: fresh, Kind: newKind, Body: body, Flags: MetaFreeFlags(newKind, body), S: ty.S}
			}
			newBody := substManyOpt(ty.Body, reduced, levelSubs, fvUnion, depth+1)
			if newKind == ty.Kind && newBody == ty.Body {
				return ty
			}
			return &TyForall{Var: ty.Var, Kind: newKind, Body: newBody, Flags: MetaFreeFlags(newKind, newBody), S: ty.S}
		}
		if fv[ty.Var] {
			fresh := freshName(ty.Var)
			body := substDepth(ty.Body, ty.Var, &TyVar{Name: fresh}, depth+1)
			body = substManyOpt(body, subs, levelSubs, fvUnion, depth+1)
			return &TyForall{Var: fresh, Kind: newKind, Body: body, Flags: MetaFreeFlags(newKind, body), S: ty.S}
		}
		newBody := substManyOpt(ty.Body, subs, levelSubs, fvUnion, depth+1)
		if newKind == ty.Kind && newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: newKind, Body: newBody, Flags: MetaFreeFlags(newKind, newBody), S: ty.S}
	case *TyEvidenceRow:
		return substManyEvidenceRow(ty, subs, levelSubs, fvUnion, depth+1)
	case *TyEvidence:
		newConstraints := substManyEvidenceRow(ty.Constraints, subs, levelSubs, fvUnion, depth+1)
		newBody := substManyOpt(ty.Body, subs, levelSubs, fvUnion, depth+1)
		if newConstraints == ty.Constraints && newBody == ty.Body {
			return ty
		}
		return &TyEvidence{Constraints: newConstraints, Body: newBody, Flags: MetaFreeFlags(newConstraints, newBody), S: ty.S}
	default:
		return MapType(t, func(child Type) Type {
			return substManyOpt(child, subs, levelSubs, fvUnion, depth+1)
		})
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
	if row.IsOpen() {
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
