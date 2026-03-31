package stdlib

import "github.com/cwd-k2/gicel/internal/lang/ir"

// List fusion rules — IR-level rewrite rules for list primitive composition.
// Separated from list.go (runtime primitives) to isolate IR dependency.

// Primitive names for list operations used in registration and fusion rules.
const (
	primListFromSlice = "_listFromSlice"
	primListToSlice   = "_listToSlice"
	primListMap       = "_listMap"
	primListFoldr     = "_listFoldr"
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
