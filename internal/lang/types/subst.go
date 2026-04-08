package types

import (
	"fmt"
	"sync/atomic"
)

var freshCounter int64

func freshName(base string) string {
	n := atomic.AddInt64(&freshCounter, 1)
	return fmt.Sprintf("%s$%d", base, n)
}

// ResetFreshCounter resets the global fresh name counter to zero.
// Use in tests to ensure deterministic type variable naming.
func ResetFreshCounter() {
	atomic.StoreInt64(&freshCounter, 0)
}

// Subst applies a substitution [varName := replacement] throughout a type.
// Fast-path: if varName does not occur in t, the type is returned unchanged.
func Subst(t Type, varName string, replacement Type) Type {
	if !OccursIn(varName, t) {
		return t
	}
	return substDepth(t, varName, replacement, 0)
}

func substDepth(t Type, varName string, replacement Type, depth int) Type {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	switch ty := t.(type) {
	case *TyVar:
		if ty.Name == varName {
			return replacement
		}
		return ty

	case *TyCon:
		return ty

	case *TyApp:
		newFun := substDepth(ty.Fun, varName, replacement, depth+1)
		newArg := substDepth(ty.Arg, varName, replacement, depth+1)
		if newFun == ty.Fun && newArg == ty.Arg {
			return ty
		}
		return &TyApp{Fun: newFun, Arg: newArg, IsGrade: ty.IsGrade, Flags: MetaFreeFlags(newFun, newArg), S: ty.S}

	case *TyArrow:
		newFrom := substDepth(ty.From, varName, replacement, depth+1)
		newTo := substDepth(ty.To, varName, replacement, depth+1)
		if newFrom == ty.From && newTo == ty.To {
			return ty
		}
		return &TyArrow{From: newFrom, To: newTo, Flags: MetaFreeFlags(newFrom, newTo), S: ty.S}

	case *TyForall:
		if ty.Var == varName {
			return ty // shadowed
		}
		newKind := substDepth(ty.Kind, varName, replacement, depth+1)
		// Capture avoidance.
		if OccursIn(ty.Var, replacement) {
			fresh := freshName(ty.Var)
			body := substDepth(ty.Body, ty.Var, &TyVar{Name: fresh}, depth+1)
			body = substDepth(body, varName, replacement, depth+1)
			return &TyForall{Var: fresh, Kind: newKind, Body: body, S: ty.S}
		}
		newBody := substDepth(ty.Body, varName, replacement, depth+1)
		if newKind == ty.Kind && newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: newKind, Body: newBody, S: ty.S}

	case *TyCBPV:
		newPre := substDepth(ty.Pre, varName, replacement, depth+1)
		newPost := substDepth(ty.Post, varName, replacement, depth+1)
		newResult := substDepth(ty.Result, varName, replacement, depth+1)
		newGrade := ty.Grade
		if newGrade != nil {
			newGrade = substDepth(newGrade, varName, replacement, depth+1)
		}
		if newPre == ty.Pre && newPost == ty.Post && newResult == ty.Result && newGrade == ty.Grade {
			return ty
		}
		return &TyCBPV{Tag: ty.Tag, Pre: newPre, Post: newPost, Result: newResult, Grade: newGrade, Flags: MetaFreeFlags(newPre, newPost, newResult, newGrade), S: ty.S}

	case *TyEvidence:
		newConstraints := substDepth(ty.Constraints, varName, replacement, depth+1)
		newBody := substDepth(ty.Body, varName, replacement, depth+1)
		if newConstraints == ty.Constraints && newBody == ty.Body {
			return ty
		}
		cr, ok := newConstraints.(*TyEvidenceRow)
		if !ok {
			// Subst produced a non-evidence-row; preserve original to avoid nil.
			return &TyEvidence{Constraints: ty.Constraints, Body: newBody, S: ty.S}
		}
		return &TyEvidence{Constraints: cr, Body: newBody, S: ty.S}

	case *TySkolem:
		return ty

	case *TyEvidenceRow:
		switch entries := ty.Entries.(type) {
		case *CapabilityEntries:
			changed := false
			var fields []RowField // nil until first change (lazy alloc)
			for i, f := range entries.Fields {
				newT := substDepth(f.Type, varName, replacement, depth+1)
				// Label substitution: when a row field label matches the
				// variable being substituted and the replacement is a label
				// literal (L1 TyCon), replace the label string. This enables
				// \(l: Label) s r. { l: s | r } to concretize to { counter: s | r }
				// when l is substituted with #counter.
				newLabel := f.Label
				isLabelVar := f.IsLabelVar
				labelChanged := false
				if f.IsLabelVar && f.Label == varName {
					if lc, ok := replacement.(*TyCon); ok && IsKindLevel(lc.Level) {
						newLabel = lc.Name
						isLabelVar = false // now a concrete label
						labelChanged = true
					}
				}
				// Grades — also lazy alloc.
				var newGrades []Type
				for j, g := range f.Grades {
					ng := substDepth(g, varName, replacement, depth+1)
					if ng != g {
						if newGrades == nil {
							newGrades = make([]Type, len(f.Grades))
							copy(newGrades[:j], f.Grades[:j])
						}
						newGrades[j] = ng
					} else if newGrades != nil {
						newGrades[j] = g
					}
				}
				fieldChanged := newT != f.Type || labelChanged || newGrades != nil
				if fieldChanged {
					if fields == nil {
						fields = make([]RowField, len(entries.Fields))
						copy(fields[:i], entries.Fields[:i])
					}
					grades := f.Grades
					if newGrades != nil {
						grades = newGrades
					}
					fields[i] = RowField{Label: newLabel, Type: newT, Grades: grades, IsLabelVar: isLabelVar, S: f.S}
					changed = true
				} else if fields != nil {
					fields[i] = f
				}
			}
			newTail := ty.Tail
			if ty.Tail != nil {
				nt := substDepth(ty.Tail, varName, replacement, depth+1)
				if nt != ty.Tail {
					newTail = nt
					changed = true
				}
			}
			if !changed {
				return ty
			}
			var newEntries EvidenceEntries = entries
			if fields != nil {
				newEntries = &CapabilityEntries{Fields: fields}
			}
			return &TyEvidenceRow{Entries: newEntries, Tail: newTail, Flags: EvidenceRowFlags(newEntries, newTail), S: ty.S}
		case *ConstraintEntries:
			changed := false
			var ces []ConstraintEntry // nil until first change (lazy alloc)
			for i, e := range entries.Entries {
				newE, entryChanged := substConstraintEntry(e, varName, replacement, depth+1)
				if entryChanged {
					if ces == nil {
						ces = make([]ConstraintEntry, len(entries.Entries))
						copy(ces[:i], entries.Entries[:i])
					}
					ces[i] = newE
					changed = true
				} else if ces != nil {
					ces[i] = e
				}
			}
			newTail := ty.Tail
			if ty.Tail != nil {
				nt := substDepth(ty.Tail, varName, replacement, depth+1)
				if nt != ty.Tail {
					newTail = nt
					changed = true
				}
			}
			if !changed {
				return ty
			}
			var newEntries EvidenceEntries = entries
			if ces != nil {
				newEntries = &ConstraintEntries{Entries: ces}
			}
			return &TyEvidenceRow{Entries: newEntries, Tail: newTail, Flags: EvidenceRowFlags(newEntries, newTail), S: ty.S}
		default:
			return ty
		}

	case *TyFamilyApp:
		var args []Type // nil until first change (lazy-init)
		for i, a := range ty.Args {
			newA := substDepth(a, varName, replacement, depth+1)
			if args == nil && newA != a {
				args = make([]Type, len(ty.Args))
				copy(args[:i], ty.Args[:i])
			}
			if args != nil {
				args[i] = newA
			}
		}
		if args == nil {
			return ty
		}
		return &TyFamilyApp{Name: ty.Name, Args: args, Kind: ty.Kind, Flags: metaFreeSlice(ty.Kind, args) &^ FlagNoFamilyApp, S: ty.S}

	case *TyMeta:
		return ty

	case *TyError:
		return ty

	default:
		return ty
	}
}

// SubstLevel replaces a level variable inside TyCon.Level fields throughout a type.
// This is separate from Subst because level variables live in LevelExpr positions
// inside TyCon (and TyCBPV grade), not in Type positions.
func SubstLevel(t Type, levelVarName string, replacement LevelExpr) Type {
	return substLevel(t, levelVarName, replacement, 0)
}

func substLevel(t Type, name string, repl LevelExpr, depth int) Type {
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
	case *TyApp:
		newFun := substLevel(ty.Fun, name, repl, depth+1)
		newArg := substLevel(ty.Arg, name, repl, depth+1)
		if newFun == ty.Fun && newArg == ty.Arg {
			return ty
		}
		return &TyApp{Fun: newFun, Arg: newArg, IsGrade: ty.IsGrade, Flags: MetaFreeFlags(newFun, newArg), S: ty.S}
	case *TyArrow:
		newFrom := substLevel(ty.From, name, repl, depth+1)
		newTo := substLevel(ty.To, name, repl, depth+1)
		if newFrom == ty.From && newTo == ty.To {
			return ty
		}
		return &TyArrow{From: newFrom, To: newTo, Flags: MetaFreeFlags(newFrom, newTo), S: ty.S}
	case *TyForall:
		if ty.Var == name {
			return ty // shadowed
		}
		newKind := substLevel(ty.Kind, name, repl, depth+1)
		newBody := substLevel(ty.Body, name, repl, depth+1)
		if newKind == ty.Kind && newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: newKind, Body: newBody, S: ty.S}
	case *TyCBPV:
		newPre := substLevel(ty.Pre, name, repl, depth+1)
		newPost := substLevel(ty.Post, name, repl, depth+1)
		newResult := substLevel(ty.Result, name, repl, depth+1)
		newGrade := ty.Grade
		if newGrade != nil {
			newGrade = substLevel(newGrade, name, repl, depth+1)
		}
		if newPre == ty.Pre && newPost == ty.Post && newResult == ty.Result && newGrade == ty.Grade {
			return ty
		}
		return &TyCBPV{Tag: ty.Tag, Pre: newPre, Post: newPost, Result: newResult, Grade: newGrade, Flags: MetaFreeFlags(newPre, newPost, newResult, newGrade), S: ty.S}
	case *TyFamilyApp:
		var args []Type
		for i, a := range ty.Args {
			newA := substLevel(a, name, repl, depth+1)
			if args == nil && newA != a {
				args = make([]Type, len(ty.Args))
				copy(args[:i], ty.Args[:i])
			}
			if args != nil {
				args[i] = newA
			}
		}
		if args == nil {
			return ty
		}
		return &TyFamilyApp{Name: ty.Name, Args: args, Kind: ty.Kind, S: ty.S}
	default:
		return ty
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

// SubstMany applies a parallel substitution: every variable in typeSubs
// (TyVar → Type) and levelSubs (LevelVar → LevelExpr) is replaced in a
// single tree walk. Either map may be nil. Substitutions are simultaneous:
// substitution values do not interfere with each other.
//
// Capture avoidance: TyForall bound variables are alpha-renamed against the
// free TyVar union of typeSubs values when needed. Level capture avoidance
// is NOT performed; callers passing levelSubs must ensure replacements have
// no free LevelVars (in practice, callers use fresh LevelMetas).
func SubstMany(t Type, typeSubs map[string]Type, levelSubs map[string]LevelExpr) Type {
	if len(typeSubs) == 0 && len(levelSubs) == 0 {
		return t
	}
	var fvUnion map[string]bool
	return substManyOpt(t, typeSubs, levelSubs, &fvUnion, 0)
}

// PeelForalls instantiates a chain of TyForall binders by repeatedly calling
// visit for each peeled binder. visit returns the replacement type for the
// binder; for level-kinded binders it also returns a LevelExpr that targets
// level positions of the same name (the type half is still required because
// the same variable name can flow into both type and level positions).
//
// Substitution is applied lazily: when the chain has exactly one binder, a
// single Subst (plus optional SubstLevel) is used with no map allocation.
// Two or more binders allocate the substitution map at the second binder
// — the "deferred materialization" pattern. This replaces the K=1 / K>=2
// dispatch that previously had to live in line at every call site, where
// it duplicated ~30 lines of bookkeeping each. The performance trade-off
// of the look-ahead `f1.Body.(*TyForall)` type assertion (which leaked into
// Tier 4 micro benches as a +1-5% regression after S2) is gone — the new
// dispatch is a single bool check on the second iteration only.
//
// Returns the body type after all substitutions, or t unchanged when t is
// not a TyForall.
func PeelForalls(t Type, visit func(f *TyForall) (typeRepl Type, levelRepl LevelExpr)) Type {
	var (
		firstVar    string
		firstRepl   Type
		firstLevel  LevelExpr
		hasFirst    bool
		typeSubs    map[string]Type
		levelSubs   map[string]LevelExpr
	)
	for {
		f, ok := t.(*TyForall)
		if !ok {
			break
		}
		repl, lvl := visit(f)
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
		return SubstMany(t, typeSubs, levelSubs)
	}
	// K=1: single binder, fast path with no map.
	if firstLevel != nil {
		t = SubstLevel(t, firstVar, firstLevel)
	}
	return Subst(t, firstVar, firstRepl)
}

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
	newEntries, changed := row.Entries.MapChildren(func(child Type) Type {
		return substManyOpt(child, subs, levelSubs, fvUnion, depth+1)
	})
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

// substConstraintEntry substitutes within a single ConstraintEntry,
// handling QuantifiedConstraint with proper variable shadowing.
// Returns the (possibly new) entry and whether anything changed.
// When the second return value is false, the first value is e itself.
func substConstraintEntry(e ConstraintEntry, varName string, replacement Type, depth int) (ConstraintEntry, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	switch e := e.(type) {
	case *ClassEntry:
		args, changed := substTypeSlice(e.Args, varName, replacement, depth+1)
		if !changed {
			return e, false
		}
		return &ClassEntry{ClassName: e.ClassName, Args: args, S: e.S}, true
	case *EqualityEntry:
		newLhs := substDepth(e.Lhs, varName, replacement, depth+1)
		newRhs := substDepth(e.Rhs, varName, replacement, depth+1)
		if newLhs == e.Lhs && newRhs == e.Rhs {
			return e, false
		}
		return &EqualityEntry{Lhs: newLhs, Rhs: newRhs, S: e.S}, true
	case *VarEntry:
		newVar := substDepth(e.Var, varName, replacement, depth+1)
		if newVar == e.Var {
			return e, false
		}
		return &VarEntry{Var: newVar, S: e.S}, true
	case *QuantifiedConstraint:
		// Check if varName is shadowed by any quantified variable.
		for _, v := range e.Vars {
			if v.Name == varName {
				return e, false // no substitution inside
			}
		}
		newQC, changed := substQuantifiedConstraint(e, varName, replacement, depth+1)
		if !changed {
			return e, false
		}
		return newQC, true
	}
	return e, false
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

// substQuantifiedConstraint substitutes within a QuantifiedConstraint, handling
// capture avoidance by renaming bound variables that appear free in replacement.
// Returns the (possibly new) constraint and whether anything changed.
func substQuantifiedConstraint(qc *QuantifiedConstraint, varName string, replacement Type, depth int) (*QuantifiedConstraint, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	changed := false
	// Capture avoidance: rename bound vars that appear free in replacement
	// BEFORE substituting, so the rename does not corrupt the replacement.
	var vars []ForallBinder   // nil until first rename
	var ctx []ConstraintEntry // nil until first change in ctx OR first rename
	head := qc.Head

	for i, v := range qc.Vars {
		if OccursIn(v.Name, replacement) {
			fresh := freshName(v.Name)
			if vars == nil {
				vars = make([]ForallBinder, len(qc.Vars))
				copy(vars, qc.Vars)
			}
			vars[i] = ForallBinder{Name: fresh, Kind: v.Kind}
			changed = true
			// Rename the bound variable in context and head BEFORE substitution.
			if ctx == nil {
				ctx = make([]ConstraintEntry, len(qc.Context))
				copy(ctx, qc.Context)
			}
			for j := range ctx {
				ctx[j], _ = renameInConstraintEntry(ctx[j], v.Name, fresh, depth+1)
			}
			if head != nil {
				renamed, _ := renameClassEntry(head, v.Name, fresh, depth+1)
				head = renamed
			}
		}
	}

	// Step 2: Now substitute in context — the renamed body no longer shadows the replacement.
	for i, c := range qc.Context {
		var src ConstraintEntry
		if ctx != nil {
			src = ctx[i]
		} else {
			src = c
		}
		newC, cChanged := substConstraintEntry(src, varName, replacement, depth+1)
		if cChanged {
			if ctx == nil {
				ctx = make([]ConstraintEntry, len(qc.Context))
				copy(ctx, qc.Context)
			}
			ctx[i] = newC
			changed = true
		} else if ctx != nil {
			ctx[i] = src
		}
	}
	if head != nil {
		newArgs, headChanged := substTypeSlice(head.Args, varName, replacement, depth+1)
		if headChanged {
			head = &ClassEntry{ClassName: head.ClassName, Args: newArgs, S: head.S}
			changed = true
		}
	}

	if !changed {
		return qc, false
	}
	finalVars := qc.Vars
	if vars != nil {
		finalVars = vars
	}
	finalCtx := qc.Context
	if ctx != nil {
		finalCtx = ctx
	}
	return &QuantifiedConstraint{Vars: finalVars, Context: finalCtx, Head: head, S: qc.S}, true
}

// renameClassEntry rewrites a ClassEntry by replacing free occurrences of
// oldName with a TyVar named newName. Returns (entry, changed).
func renameClassEntry(e *ClassEntry, oldName, newName string, depth int) (*ClassEntry, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	replacement := &TyVar{Name: newName}
	args, changed := substTypeSlice(e.Args, oldName, replacement, depth+1)
	if !changed {
		return e, false
	}
	return &ClassEntry{ClassName: e.ClassName, Args: args, S: e.S}, true
}

// renameInConstraintEntry replaces oldName with a TyVar named newName
// throughout a constraint entry. Returns (entry, changed). When changed
// is false, the returned entry is e itself.
func renameInConstraintEntry(e ConstraintEntry, oldName, newName string, depth int) (ConstraintEntry, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	replacement := &TyVar{Name: newName}
	switch e := e.(type) {
	case *ClassEntry:
		return renameClassEntry(e, oldName, newName, depth)
	case *EqualityEntry:
		newLhs := substDepth(e.Lhs, oldName, replacement, depth+1)
		newRhs := substDepth(e.Rhs, oldName, replacement, depth+1)
		if newLhs == e.Lhs && newRhs == e.Rhs {
			return e, false
		}
		return &EqualityEntry{Lhs: newLhs, Rhs: newRhs, S: e.S}, true
	case *VarEntry:
		newVar := substDepth(e.Var, oldName, replacement, depth+1)
		if newVar == e.Var {
			return e, false
		}
		return &VarEntry{Var: newVar, S: e.S}, true
	case *QuantifiedConstraint:
		// Check if oldName is shadowed by a quantified variable.
		for _, v := range e.Vars {
			if v.Name == oldName {
				return e, false
			}
		}
		ctxChanged := false
		var ctx []ConstraintEntry // nil until first change
		for i, c := range e.Context {
			newC, cChanged := renameInConstraintEntry(c, oldName, newName, depth+1)
			if cChanged {
				if ctx == nil {
					ctx = make([]ConstraintEntry, len(e.Context))
					copy(ctx[:i], e.Context[:i])
				}
				ctx[i] = newC
				ctxChanged = true
			} else if ctx != nil {
				ctx[i] = c
			}
		}
		head := e.Head
		headChanged := false
		if head != nil {
			head, headChanged = renameClassEntry(head, oldName, newName, depth+1)
		}
		if !ctxChanged && !headChanged {
			return e, false
		}
		finalCtx := e.Context
		if ctx != nil {
			finalCtx = ctx
		}
		return &QuantifiedConstraint{Vars: e.Vars, Context: finalCtx, Head: head, S: e.S}, true
	}
	return e, false
}
