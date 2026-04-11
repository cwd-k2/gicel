package stdlib

import "github.com/cwd-k2/gicel/internal/lang/ir"

// injectToVariantLit rewrites saturated inject calls to VariantLit nodes.
// After label erasure, inject @#tag payload becomes:
//
//	App(App(Var{inject}, Lit{tag}), payload)
//
// This rewrite produces:
//
//	VariantLit{Tag: tag, Value: payload}
//
// which is compiled to OpVariant (direct VariantVal construction) instead
// of going through the PrimOp dispatch path.
func injectToVariantLit(c ir.Core) ir.Core {
	outer, ok := c.(*ir.App)
	if !ok {
		return c
	}
	inner, ok := outer.Fun.(*ir.App)
	if !ok {
		return c
	}
	v, ok := inner.Fun.(*ir.Var)
	if !ok || v.Name != "inject" {
		return c
	}
	tagLit, ok := inner.Arg.(*ir.Lit)
	if !ok {
		return c
	}
	tag, ok := tagLit.Value.(string)
	if !ok {
		return c
	}
	return &ir.VariantLit{Tag: tag, Value: outer.Arg, S: outer.S}
}
