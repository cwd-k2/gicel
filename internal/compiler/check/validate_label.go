package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// validateLabelArgs walks the Core IR program and reports unsolved label
// metavariables in TyApp nodes. A TyApp whose TyArg zonks to a TyMeta
// with Label kind means a named capability function (*At) was called
// without an explicit @#label argument.
//
// Excluded cases (not errors):
//   - Binding type is polymorphic over Label: polymorphic wrappers
//     (e.g., newAt := _arrayNewNamed) or generalized definitions.
//   - Unsolved label meta does not appear in the binding's final type:
//     phantom parameters (e.g., main := g where g :: \(l: Label). Bool).
//
// Must run after type checking (all solvable metas are resolved) and
// before label erasure (which would silently skip the unsolved TyApp).
func (ch *Checker) validateLabelArgs(prog *ir.Program) {
	for _, b := range prog.Bindings {
		zonkedType := ch.unifier.Zonk(b.Type)
		if hasLabelForall(zonkedType) {
			continue
		}
		ch.walkValidateLabel(b.Expr, zonkedType, 0)
	}
}

// hasLabelForall reports whether a type has a top-level forall quantifier
// with Label kind. Peels through nested foralls and evidence.
func hasLabelForall(ty types.Type) bool {
	for {
		switch t := ty.(type) {
		case *types.TyForall:
			if types.Equal(t.Kind, types.TypeOfLabels) {
				return true
			}
			ty = t.Body
		case *types.TyEvidence:
			ty = t.Body
		default:
			return false
		}
	}
}

// walkValidateLabel recursively walks Core IR, tracking the number of
// enclosing TyLam nodes with Label kind.
// All 19 Core node types are handled; default panics on unrecognized nodes
// to catch omissions when new formers are added.
func (ch *Checker) walkValidateLabel(c ir.Core, bindingType types.Type, labelLamDepth int) {
	if c == nil {
		return
	}
	w := func(child ir.Core) {
		ch.walkValidateLabel(child, bindingType, labelLamDepth)
	}
	switch n := c.(type) {
	case *ir.TyLam:
		d := labelLamDepth
		if types.Equal(n.Kind, types.TypeOfLabels) {
			d++
		}
		ch.walkValidateLabel(n.Body, bindingType, d)

	case *ir.TyApp:
		// Zonk TyArg in place so label erasure can see resolved TyVar
		// instead of unsolved TyMeta. Safe: IR is freshly produced and not shared.
		n.TyArg = ch.unifier.Zonk(n.TyArg)
		if labelLamDepth == 0 {
			if meta, ok := n.TyArg.(*types.TyMeta); ok {
				if types.Equal(ch.unifier.Zonk(meta.Kind), types.TypeOfLabels) && typeContainsMeta(bindingType, meta.ID) {
					ch.addDiag(diagnostic.ErrMissingLabel, n.S,
						diagMsg("named capability requires explicit @#label argument"))
				}
			}
		}
		w(n.Expr)

	case *ir.Var, *ir.Lit, *ir.Error:
		// leaf — no children
	case *ir.Lam:
		w(n.Body)
	case *ir.App:
		w(n.Fun)
		w(n.Arg)
	case *ir.Con:
		for _, arg := range n.Args {
			w(arg)
		}
	case *ir.Case:
		w(n.Scrutinee)
		for _, a := range n.Alts {
			w(a.Body)
		}
	case *ir.Fix:
		w(n.Body)
	case *ir.Pure:
		w(n.Expr)
	case *ir.Bind:
		w(n.Comp)
		w(n.Body)
	case *ir.Thunk:
		w(n.Comp)
	case *ir.Force:
		w(n.Expr)
	case *ir.Merge:
		w(n.Left)
		w(n.Right)
	case *ir.PrimOp:
		for _, arg := range n.Args {
			w(arg)
		}
	case *ir.RecordLit:
		for _, f := range n.Fields {
			w(f.Value)
		}
	case *ir.RecordProj:
		w(n.Record)
	case *ir.RecordUpdate:
		w(n.Record)
		for _, f := range n.Updates {
			w(f.Value)
		}
	case *ir.VariantLit:
		w(n.Value)
	default:
		panic(fmt.Sprintf("walkValidateLabel: unhandled Core node %T", c))
	}
}

// typeContainsMeta reports whether a type contains TyMeta with the given ID.
func typeContainsMeta(ty types.Type, metaID int) bool {
	return types.AnyType(ty, func(t types.Type) bool {
		m, ok := t.(*types.TyMeta)
		return ok && m.ID == metaID
	})
}
