package types

import (
	"fmt"
	"sync/atomic"
)

// freshCounter provides globally unique suffixes for capture-avoiding
// substitution in type-level operations. Namespace: "$" suffix.
//
// Other fresh-name generators use different namespaces to avoid collisions:
//   - optimize/subst.go: "$r" suffix (term-level Core IR substitution)
//   - check/checker.go: "_" suffix (per-checker, non-global)
var freshCounter atomic.Uint64

func freshName(base string) string {
	n := freshCounter.Add(1)
	return fmt.Sprintf("%s$%d", base, n)
}

// ResetFreshCounter resets the global fresh name counter to zero.
// Use in tests to ensure deterministic type variable naming.
func ResetFreshCounter() {
	freshCounter.Store(0)
}

// --- TypeOps methods (public API) ---

// Subst applies a substitution [varName := replacement] throughout a type.
// Fast-path: if varName does not occur in t, the type is returned unchanged.
func (o *TypeOps) Subst(t Type, varName string, replacement Type) Type {
	if !occursIn(varName, t, nil, 0) {
		return t
	}
	return substDepth(o, t, varName, replacement, 0)
}

// SubstLevel replaces a level variable inside TyCon.Level fields throughout a type.
// This is separate from Subst because level variables live in LevelExpr positions
// inside TyCon (and TyCBPV grade), not in Type positions.
func (o *TypeOps) SubstLevel(t Type, levelVarName string, replacement LevelExpr) Type {
	return substLevel(o, t, levelVarName, replacement, 0)
}

// SubstMany applies a parallel substitution: every variable in typeSubs
// (TyVar → Type) and levelSubs (LevelVar → LevelExpr) is replaced in a
// single tree walk. Either map may be nil. Substitutions are simultaneous:
// substitution values do not interfere with each other.
//
// Capture avoidance: TyForall bound variables are alpha-renamed against the
// free TyVar union of typeSubs values when needed. Level capture avoidance
// is NOT performed; callers passing levelSubs must ensure replacements have
// no free LevelVars (in practice, callers use fresh LevelMetas).
func (o *TypeOps) SubstMany(t Type, typeSubs map[string]Type, levelSubs map[string]LevelExpr) Type {
	if len(typeSubs) == 0 && len(levelSubs) == 0 {
		return t
	}
	var fvUnion map[string]bool
	return substManyOpt(o, t, typeSubs, levelSubs, &fvUnion, 0)
}

// PeelForalls instantiates a chain of TyForall binders by repeatedly calling
// visit for each peeled binder. visit returns the replacement type for the
// binder; for level-kinded binders it also returns a LevelExpr that targets
// level positions of the same name (the type half is still required because
// the same variable name can flow into both type and level positions).
//
// Substitution is applied lazily: when the chain has exactly one binder, a
// single Subst (plus optional SubstLevel) is used with no map allocation.
// Two or more binders allocate the substitution map at the second binder.
//
// Returns the body type after all substitutions, or t unchanged when t is
// not a TyForall.
func (o *TypeOps) PeelForalls(t Type, visit func(f *TyForall) (typeRepl Type, levelRepl LevelExpr)) Type {
	var (
		firstVar   string
		firstRepl  Type
		firstLevel LevelExpr
		hasFirst   bool
		typeSubs   map[string]Type
		levelSubs  map[string]LevelExpr
	)
	for {
		f, ok := t.(*TyForall)
		if !ok {
			break
		}
		visited := f
		if hasFirst {
			var adjustedKind Type
			if typeSubs != nil {
				adjustedKind = o.SubstMany(f.Kind, typeSubs, levelSubs)
			} else {
				adjustedKind = o.Subst(f.Kind, firstVar, firstRepl)
				if firstLevel != nil {
					adjustedKind = o.SubstLevel(adjustedKind, firstVar, firstLevel)
				}
			}
			if adjustedKind != f.Kind {
				visited = &TyForall{Var: f.Var, Kind: adjustedKind, Body: f.Body}
			}
		}
		repl, lvl := visit(visited)
		if !hasFirst {
			firstVar = f.Var
			firstRepl = repl
			firstLevel = lvl
			hasFirst = true
		} else {
			if typeSubs == nil {
				typeSubs = map[string]Type{firstVar: firstRepl}
				if firstLevel != nil {
					levelSubs = map[string]LevelExpr{firstVar: firstLevel}
				}
			}
			typeSubs[f.Var] = repl
			if lvl != nil {
				if levelSubs == nil {
					levelSubs = map[string]LevelExpr{}
				}
				levelSubs[f.Var] = lvl
			}
		}
		t = f.Body
	}
	if !hasFirst {
		return t
	}
	if typeSubs != nil {
		return o.SubstMany(t, typeSubs, levelSubs)
	}
	if firstLevel != nil {
		t = o.SubstLevel(t, firstVar, firstLevel)
	}
	return o.Subst(t, firstVar, firstRepl)
}

// PrepareSubst creates a PreparedSubst for batch application.
func (o *TypeOps) PrepareSubst(subs map[string]Type) *PreparedSubst {
	return &PreparedSubst{subs: subs}
}

// --- Internal implementation (package-level, original signatures) ---

func substDepth(ops *TypeOps, t Type, varName string, replacement Type, depth int) Type {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	switch ty := t.(type) {
	case *TyVar:
		if ty.Name == varName {
			return replacement
		}
		return ty

	case *TyForall:
		if ty.Var == varName {
			return ty // shadowed
		}
		newKind := substDepth(ops, ty.Kind, varName, replacement, depth+1)
		// Capture avoidance.
		if occursIn(ty.Var, replacement, nil, 0) {
			fresh := freshName(ty.Var)
			body := substDepth(ops, ty.Body, ty.Var, &TyVar{Name: fresh}, depth+1)
			body = substDepth(ops, body, varName, replacement, depth+1)
			return &TyForall{Var: fresh, Kind: newKind, Body: body, Flags: MetaFreeFlags(newKind, body), S: ty.S}
		}
		newBody := substDepth(ops, ty.Body, varName, replacement, depth+1)
		if newKind == ty.Kind && newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: newKind, Body: newBody, Flags: MetaFreeFlags(newKind, newBody), S: ty.S}

	case *TyEvidenceRow:
		newEntries, entriesChanged := ty.Entries.SubstEntries(ops, varName, replacement, depth+1)
		newTail := ty.Tail
		tailChanged := false
		if ty.IsOpen() {
			nt := substDepth(ops, ty.Tail, varName, replacement, depth+1)
			if nt != ty.Tail {
				newTail = nt
				tailChanged = true
			}
		}
		if !entriesChanged && !tailChanged {
			return ty
		}
		return &TyEvidenceRow{Entries: newEntries, Tail: newTail, Flags: EvidenceRowFlags(newEntries, newTail), S: ty.S}

	default:
		return ops.MapType(t, func(child Type) Type {
			return substDepth(ops, child, varName, replacement, depth+1)
		})
	}
}

func substLevel(ops *TypeOps, t Type, name string, repl LevelExpr, depth int) Type {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	switch ty := t.(type) {
	case *TyCon:
		newLevel := substLevelExpr(ty.Level, name, repl)
		if newLevel == ty.Level {
			return ty
		}
		return &TyCon{Name: ty.Name, Level: newLevel, IsLabel: ty.IsLabel, S: ty.S}
	case *TyForall:
		if ty.Var == name {
			return ty // shadowed
		}
		newKind := substLevel(ops, ty.Kind, name, repl, depth+1)
		newBody := substLevel(ops, ty.Body, name, repl, depth+1)
		if newKind == ty.Kind && newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: newKind, Body: newBody, Flags: MetaFreeFlags(newKind, newBody), S: ty.S}
	default:
		return ops.MapType(t, func(child Type) Type {
			return substLevel(ops, child, name, repl, depth+1)
		})
	}
}

// substLevelExpr replaces a LevelVar by name inside a LevelExpr.
func substLevelExpr(l LevelExpr, name string, repl LevelExpr) LevelExpr {
	if l == nil {
		return nil
	}
	switch le := l.(type) {
	case *LevelVar:
		if le.Name == name {
			return repl
		}
		return le
	case *LevelMax:
		newA := substLevelExpr(le.A, name, repl)
		newB := substLevelExpr(le.B, name, repl)
		if newA == le.A && newB == le.B {
			return le
		}
		return &LevelMax{A: newA, B: newB}
	case *LevelSucc:
		newE := substLevelExpr(le.E, name, repl)
		if newE == le.E {
			return le
		}
		return &LevelSucc{E: newE}
	default:
		return l
	}
}

// substLevelExprMany applies a parallel level-variable substitution to a LevelExpr.
func substLevelExprMany(l LevelExpr, levelSubs map[string]LevelExpr) LevelExpr {
	if l == nil || len(levelSubs) == 0 {
		return l
	}
	switch le := l.(type) {
	case *LevelVar:
		if repl, ok := levelSubs[le.Name]; ok {
			return repl
		}
		return le
	case *LevelMax:
		newA := substLevelExprMany(le.A, levelSubs)
		newB := substLevelExprMany(le.B, levelSubs)
		if newA == le.A && newB == le.B {
			return le
		}
		return &LevelMax{A: newA, B: newB}
	case *LevelSucc:
		newE := substLevelExprMany(le.E, levelSubs)
		if newE == le.E {
			return le
		}
		return &LevelSucc{E: newE}
	default:
		return l
	}
}
