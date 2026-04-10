package stdlib

import "github.com/cwd-k2/gicel/internal/lang/ir"

// List fusion rules — IR-level rewrite rules for list primitive composition.
// Separated from list.go (runtime primitives) to isolate IR dependency.
//
// DESIGN NOTE: Fusion rules live in host/stdlib (not compiler/optimize)
// because each pack owns its own algebraic laws. The tradeoff is that
// stdlib imports lang/ir for Core pattern matching, creating a cross-
// layer dependency. This is acknowledged and intentional — the
// alternative (optimizer importing domain-specific pack knowledge)
// would be worse for modularity. See registry.RewriteRule.

// Primitive names for list operations used in registration and fusion rules.
const (
	primListFromSlice = "_listFromSlice"
	primListToSlice   = "_listToSlice"
	primListMap       = "_listMap"
	primListFoldr     = "_listFoldr"
	primListFoldl     = "_listFoldl"
	primListFilter    = "_listFilter"
)

// R15: _listMap f (_listMap g xs) → _listMap (\$x -> f (g $x)) xs
func listMapMapFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primListMap || len(po.Args) != 2 {
		return c
	}
	inner, ok := po.Args[1].(*ir.PrimOp)
	if !ok || inner.Name != primListMap || len(inner.Args) != 2 {
		return c
	}
	f, g, xs := po.Args[0], inner.Args[0], inner.Args[1]
	x := optVarX
	if freeIn(x, f) || freeIn(x, g) {
		return c
	}
	composed := &ir.Lam{Param: x, Body: &ir.App{
		Fun: f, Arg: &ir.App{Fun: g, Arg: &ir.Var{Name: x}},
	}}
	return &ir.PrimOp{Name: primListMap, Arity: 2, Args: []ir.Core{composed, xs}, S: po.S}
}

// R16: _listFoldr k z (_listMap f xs) → _listFoldr (\$x $acc -> k (f $x) $acc) z xs
func listFoldrMapFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primListFoldr || len(po.Args) != 3 {
		return c
	}
	inner, ok := po.Args[2].(*ir.PrimOp)
	if !ok || inner.Name != primListMap || len(inner.Args) != 2 {
		return c
	}
	k, z, f, xs := po.Args[0], po.Args[1], inner.Args[0], inner.Args[1]
	x, acc := optVarX, optVarAcc
	if freeIn(x, k) || freeIn(x, f) || freeIn(acc, k) || freeIn(acc, f) {
		return c
	}
	fused := &ir.Lam{Param: x, Body: &ir.Lam{Param: acc, Body: &ir.App{
		Fun: &ir.App{Fun: k, Arg: &ir.App{Fun: f, Arg: &ir.Var{Name: x}}},
		Arg: &ir.Var{Name: acc},
	}}}
	return &ir.PrimOp{Name: primListFoldr, Arity: 3, Args: []ir.Core{fused, z, xs}, S: po.S}
}

// R17: _listFoldr k z (_listFilter p xs) → _listFoldr (\$x $acc -> case p $x of { True -> k $x $acc; False -> $acc }) z xs
// Eliminates the intermediate filtered list.
func listFoldrFilterFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primListFoldr || len(po.Args) != 3 {
		return c
	}
	inner, ok := po.Args[2].(*ir.PrimOp)
	if !ok || inner.Name != primListFilter || len(inner.Args) != 2 {
		return c
	}
	k, z, p, xs := po.Args[0], po.Args[1], inner.Args[0], inner.Args[1]
	x, acc := optVarX, optVarAcc
	if freeIn(x, k) || freeIn(x, p) || freeIn(acc, k) || freeIn(acc, p) {
		return c
	}
	// \$x $acc -> case p $x of { True -> k $x $acc; False -> $acc }
	fused := &ir.Lam{Param: x, Body: &ir.Lam{Param: acc, Body: &ir.Case{
		Scrutinee: &ir.App{Fun: p, Arg: &ir.Var{Name: x}},
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "True"}, Body: &ir.App{
				Fun: &ir.App{Fun: k, Arg: &ir.Var{Name: x}},
				Arg: &ir.Var{Name: acc},
			}},
			{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Var{Name: acc}},
		},
	}}}
	return &ir.PrimOp{Name: primListFoldr, Arity: 3, Args: []ir.Core{fused, z, xs}, S: po.S}
}

// R18: _listFoldl k z (_listFilter p xs) → _listFoldl (\$acc $x -> case p $x of { True -> k $acc $x; False -> $acc }) z xs
func listFoldlFilterFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primListFoldl || len(po.Args) != 3 {
		return c
	}
	inner, ok := po.Args[2].(*ir.PrimOp)
	if !ok || inner.Name != primListFilter || len(inner.Args) != 2 {
		return c
	}
	k, z, p, xs := po.Args[0], po.Args[1], inner.Args[0], inner.Args[1]
	x, acc := optVarX, optVarAcc
	if freeIn(x, k) || freeIn(x, p) || freeIn(acc, k) || freeIn(acc, p) {
		return c
	}
	// \$acc $x -> case p $x of { True -> k $acc $x; False -> $acc }
	fused := &ir.Lam{Param: acc, Body: &ir.Lam{Param: x, Body: &ir.Case{
		Scrutinee: &ir.App{Fun: p, Arg: &ir.Var{Name: x}},
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "True"}, Body: &ir.App{
				Fun: &ir.App{Fun: k, Arg: &ir.Var{Name: acc}},
				Arg: &ir.Var{Name: x},
			}},
			{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Var{Name: acc}},
		},
	}}}
	return &ir.PrimOp{Name: primListFoldl, Arity: 3, Args: []ir.Core{fused, z, xs}, S: po.S}
}

// R19: _listMap f (_listFilter p xs) → _listFilter p (_listMap f xs) is NOT safe
// (filter ∘ map changes element types). Instead we fuse directly:
// _listMap f (_listFilter p xs) → _listFoldr (\$x $acc -> case p $x of { True -> Cons (f $x) $acc; False -> $acc }) Nil xs
// Eliminates both the intermediate filtered list AND the intermediate mapped list.
func listMapFilterFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primListMap || len(po.Args) != 2 {
		return c
	}
	inner, ok := po.Args[1].(*ir.PrimOp)
	if !ok || inner.Name != primListFilter || len(inner.Args) != 2 {
		return c
	}
	f, p, xs := po.Args[0], inner.Args[0], inner.Args[1]
	x, acc := optVarX, optVarAcc
	if freeIn(x, f) || freeIn(x, p) || freeIn(acc, f) || freeIn(acc, p) {
		return c
	}
	// \$x $acc -> case p $x of { True -> Cons (f $x) $acc; False -> $acc }
	fused := &ir.Lam{Param: x, Body: &ir.Lam{Param: acc, Body: &ir.Case{
		Scrutinee: &ir.App{Fun: p, Arg: &ir.Var{Name: x}},
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "True"}, Body: &ir.Con{
				Name: "Cons",
				Args: []ir.Core{
					&ir.App{Fun: f, Arg: &ir.Var{Name: x}},
					&ir.Var{Name: acc},
				},
			}},
			{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Var{Name: acc}},
		},
	}}}
	nilVal := &ir.Con{Name: "Nil"}
	return &ir.PrimOp{Name: primListFoldr, Arity: 3, Args: []ir.Core{fused, nilVal, xs}, S: po.S}
}

// R20: _listFoldr k z (_listMap f xs) is already R16.
// R21: _listFoldl k z (_listMap f xs) → _listFoldl (\$acc $x -> k $acc (f $x)) z xs
func listFoldlMapFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primListFoldl || len(po.Args) != 3 {
		return c
	}
	inner, ok := po.Args[2].(*ir.PrimOp)
	if !ok || inner.Name != primListMap || len(inner.Args) != 2 {
		return c
	}
	k, z, f, xs := po.Args[0], po.Args[1], inner.Args[0], inner.Args[1]
	x, acc := optVarX, optVarAcc
	if freeIn(x, k) || freeIn(x, f) || freeIn(acc, k) || freeIn(acc, f) {
		return c
	}
	fused := &ir.Lam{Param: acc, Body: &ir.Lam{Param: x, Body: &ir.App{
		Fun: &ir.App{Fun: k, Arg: &ir.Var{Name: acc}},
		Arg: &ir.App{Fun: f, Arg: &ir.Var{Name: x}},
	}}}
	return &ir.PrimOp{Name: primListFoldl, Arity: 3, Args: []ir.Core{fused, z, xs}, S: po.S}
}

// R14: _listFromSlice (_listToSlice xs) → xs  and  _listToSlice (_listFromSlice xs) → xs
func listPackedRoundtrip(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || len(po.Args) != 1 {
		return c
	}
	inner, ok := po.Args[0].(*ir.PrimOp)
	if !ok || len(inner.Args) != 1 {
		return c
	}
	if po.Name == primListFromSlice && inner.Name == primListToSlice {
		return inner.Args[0]
	}
	if po.Name == primListToSlice && inner.Name == primListFromSlice {
		return inner.Args[0]
	}
	return c
}
