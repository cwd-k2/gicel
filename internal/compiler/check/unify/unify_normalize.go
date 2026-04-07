// Unifier — type normalization: alias expansion, type family reduction,
// CBPV TyApp collapse to TyCBPV, and adjacent universe cumulativity.
// Does NOT cover: core unification (unify.go), snapshot/trail (unify_trail.go).

package unify

import (
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// normalize applies alias expansion, type family reduction, and special
// type normalization. Type family reduction is eager here for compatibility:
// many inference paths depend on TyFamilyApp being reduced before unification.
// The solver's CtFunEq path (L2-b) handles deferred reduction for stuck
// applications whose args contain unsolved metas.
func (u *Unifier) normalize(t types.Type) types.Type {
	if u.AliasExpander != nil {
		t = u.AliasExpander(t)
	}
	if u.FamilyReducer != nil {
		t = u.FamilyReducer(t)
	}
	return normalizeCompApp(t)
}

// normalizeCompApp converts fully-applied TyApp chains to their special type
// representations. Handles both 4-arg (graded) and 3-arg (legacy) forms:
//
//	4-arg: TyApp(TyApp(TyApp(TyApp(TyCon("Computation"), grade), pre), post), result)
//	3-arg: TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
//
// This arises when a class type parameter is substituted with Computation.

// normalizeCompApp converts fully-applied TyApp chains to their special type
// representations. Handles both 4-arg (graded) and 3-arg (legacy) forms:
//
//	4-arg: TyApp(TyApp(TyApp(TyApp(TyCon("Computation"), grade), pre), post), result)
//	3-arg: TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
//
// This arises when a class type parameter is substituted with Computation.
func normalizeCompApp(t types.Type) types.Type {
	app1, ok := t.(*types.TyApp)
	if !ok {
		return t
	}
	app2, ok := app1.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	app3, ok := app2.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	// Try 4-arg form: Computation grade pre post result
	if app4, ok := app3.Fun.(*types.TyApp); ok {
		if con, ok := app4.Fun.(*types.TyCon); ok {
			switch con.Name {
			case types.TyConComputation:
				return &types.TyCBPV{Tag: types.TagComp, Grade: app4.Arg, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, Flags: types.MetaFreeFlags(app4.Arg, app3.Arg, app2.Arg, app1.Arg), S: t.Span()}
			case types.TyConThunk:
				return &types.TyCBPV{Tag: types.TagThunk, Grade: app4.Arg, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, Flags: types.MetaFreeFlags(app4.Arg, app3.Arg, app2.Arg, app1.Arg), S: t.Span()}
			}
		}
	}
	// 3-arg legacy: Computation pre post result (grade omitted).
	// normalizeCompApp runs during unification/zonking, where the full chain
	// is visible. Safe to normalize without Row restriction because depth-3
	// with Computation head can only be 3-arg at this point (4-arg would
	// have been caught by the 4-arg check above which requires depth-4).
	con, ok := app3.Fun.(*types.TyCon)
	if !ok {
		return t
	}
	switch con.Name {
	case types.TyConComputation:
		return &types.TyCBPV{Tag: types.TagComp, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, S: t.Span()}
	case types.TyConThunk:
		return &types.TyCBPV{Tag: types.TagThunk, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, S: t.Span()}
	}
	return t
}

// normalizeCompApp4Only normalizes only 4-arg Computation/Thunk TyApp chains.
// Used in Zonk where 3-arg normalization would be premature.

// normalizeCompApp4Only normalizes only 4-arg Computation/Thunk TyApp chains.
// Used in Zonk where 3-arg normalization would be premature.
func normalizeCompApp4Only(t types.Type) types.Type {
	app1, ok := t.(*types.TyApp)
	if !ok {
		return t
	}
	app2, ok := app1.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	app3, ok := app2.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	app4, ok := app3.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	con, ok := app4.Fun.(*types.TyCon)
	if !ok {
		return t
	}
	switch con.Name {
	case types.TyConComputation:
		return &types.TyCBPV{Tag: types.TagComp, Grade: app4.Arg, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, Flags: types.MetaFreeFlags(app4.Arg, app3.Arg, app2.Arg, app1.Arg), S: t.Span()}
	case types.TyConThunk:
		return &types.TyCBPV{Tag: types.TagThunk, Grade: app4.Arg, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, Flags: types.MetaFreeFlags(app4.Arg, app3.Arg, app2.Arg, app1.Arg), S: t.Span()}
	}
	return t
}

// levelAdjacentCumulativity returns true if level a is exactly one level
// below level b: a + 1 == b. This captures Russell-style cumulativity
// where a kind at level ℓ inhabits the sort at level ℓ+1.
// Conservative: returns false if either side contains a LevelMeta.

// levelAdjacentCumulativity returns true if level a is exactly one level
// below level b: a + 1 == b. This captures Russell-style cumulativity
// where a kind at level ℓ inhabits the sort at level ℓ+1.
// Conservative: returns false if either side contains a LevelMeta.
func (u *Unifier) levelAdjacentCumulativity(a, b types.LevelExpr) bool {
	a = u.zonkLevel(a)
	b = u.zonkLevel(b)
	la, okA := a.(*types.LevelLit)
	lb, okB := b.(*types.LevelLit)
	if okA && okB {
		return la.N+1 == lb.N
	}
	return false
}

// Unify solves the constraint a ~ b.
