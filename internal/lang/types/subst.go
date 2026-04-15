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
		newEntries, entriesChanged := ty.Entries.SubstEntries(varName, replacement, depth+1)
		newTail := ty.Tail
		tailChanged := false
		if ty.Tail != nil {
			nt := substDepth(ty.Tail, varName, replacement, depth+1)
			if nt != ty.Tail {
				newTail = nt
				tailChanged = true
			}
		}
		if !entriesChanged && !tailChanged {
			return ty
		}
		return &TyEvidenceRow{Entries: newEntries, Tail: newTail, Flags: EvidenceRowFlags(newEntries, newTail), S: ty.S}

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
		newKind := ty.Kind
		if ty.Kind != nil {
			newKind = substDepth(ty.Kind, varName, replacement, depth+1)
		}
		if args == nil && newKind == ty.Kind {
			return ty
		}
		finalArgs := ty.Args
		if args != nil {
			finalArgs = args
		}
		return &TyFamilyApp{Name: ty.Name, Args: finalArgs, Kind: newKind, Flags: metaFreeSlice(newKind, finalArgs) &^ FlagNoFamilyApp, S: ty.S}

	case *TyMeta:
		return ty

	case *TyError:
		return ty

	default:
		panic(unhandledTypeMsg("substDepth", ty))
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
		newKind := ty.Kind
		if ty.Kind != nil {
			newKind = substLevel(ty.Kind, name, repl, depth+1)
		}
		if args == nil && newKind == ty.Kind {
			return ty
		}
		finalArgs := ty.Args
		if args != nil {
			finalArgs = args
		}
		return &TyFamilyApp{Name: ty.Name, Args: finalArgs, Kind: newKind, Flags: metaFreeSlice(newKind, finalArgs) &^ FlagNoFamilyApp, S: ty.S}
	case *TyVar, *TyMeta, *TySkolem, *TyError:
		// Leaves — no LevelExpr positions.
		return ty
	case *TyEvidence:
		// TyEvidence body may contain level vars, but constraints are row-typed.
		newConstraints := substLevel(ty.Constraints, name, repl, depth+1)
		newBody := substLevel(ty.Body, name, repl, depth+1)
		if newConstraints == ty.Constraints && newBody == ty.Body {
			return ty
		}
		cr, ok := newConstraints.(*TyEvidenceRow)
		if !ok {
			cr = ty.Constraints
		}
		return &TyEvidence{Constraints: cr, Body: newBody, Flags: MetaFreeFlags(cr, newBody), S: ty.S}
	case *TyEvidenceRow:
		// Delegate child traversal via MapType — level vars may occur inside row entries.
		return MapType(ty, func(child Type) Type {
			return substLevel(child, name, repl, depth+1)
		})
	default:
		panic(unhandledTypeMsg("substLevel", ty))
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
		// For K>=2 binders, apply accumulated substitutions to f.Kind
		// before calling the visitor. Without this, the visitor receives
		// stale TyVar references in the Kind field (e.g., TyVar{k} instead
		// of the meta that replaced k in the first iteration).
		visited := f
		if hasFirst {
			var adjustedKind Type
			if typeSubs != nil {
				adjustedKind = SubstMany(f.Kind, typeSubs, levelSubs)
			} else {
				adjustedKind = Subst(f.Kind, firstVar, firstRepl)
				if firstLevel != nil {
					adjustedKind = SubstLevel(adjustedKind, firstVar, firstLevel)
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
		return SubstMany(t, typeSubs, levelSubs)
	}
	// K=1: single binder, fast path with no map.
	if firstLevel != nil {
		t = SubstLevel(t, firstVar, firstLevel)
	}
	return Subst(t, firstVar, firstRepl)
}
