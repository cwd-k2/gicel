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
	return normalizeCompApp(u.TypeOps, t)
}

// normalizeCompApp converts fully-applied TyApp chains to their TyCBPV
// representation. Handles both surface forms:
//
//	4-arg (graded):   TyApp(TyApp(TyApp(TyApp(TyCon("Computation"), grade), pre), post), result)
//	3-arg (ungraded): TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
//
// Both forms are first-class — see "CBPV grade duality" in package types
// doc.go for the semantics. The 3-arg form arises when a class type
// parameter is substituted with Computation, or when the resolver left a
// raw TyApp chain because its row-literal heuristic failed (e.g., for
// `Computation pre post a` where pre is a TyVar). At normalization time,
// the depth-3 chain with Computation/Thunk head is unambiguously the
// ungraded form.
func normalizeCompApp(ops *types.TypeOps, t types.Type) types.Type {
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
				return ops.Comp(app3.Arg, app2.Arg, app1.Arg, app4.Arg, t.Span())
			case types.TyConThunk:
				return ops.ThunkGraded(app3.Arg, app2.Arg, app1.Arg, app4.Arg, t.Span())
			}
		}
	}
	// 3-arg ungraded form: Computation pre post result (grade omitted).
	// normalizeCompApp runs during unification/zonking, where the full chain
	// is visible. Safe to normalize without Row restriction because depth-3
	// with Computation head can only be the ungraded form at this point
	// (4-arg would have been caught above, which requires depth-4).
	con, ok := app3.Fun.(*types.TyCon)
	if !ok {
		return t
	}
	switch con.Name {
	case types.TyConComputation:
		return ops.Comp(app3.Arg, app2.Arg, app1.Arg, nil, t.Span())
	case types.TyConThunk:
		return ops.Thunk(app3.Arg, app2.Arg, app1.Arg, t.Span())
	}
	return t
}

// normalizeCompApp4Only normalizes only 4-arg Computation/Thunk TyApp chains.
// Used in Zonk where 3-arg normalization would be premature.
func normalizeCompApp4Only(ops *types.TypeOps, t types.Type) types.Type {
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
		return ops.Comp(app3.Arg, app2.Arg, app1.Arg, app4.Arg, t.Span())
	case types.TyConThunk:
		return ops.ThunkGraded(app3.Arg, app2.Arg, app1.Arg, app4.Arg, t.Span())
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
