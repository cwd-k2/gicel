package check

import (
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
func (ch *Checker) walkValidateLabel(c ir.Core, bindingType types.Type, labelLamDepth int) {
	if c == nil {
		return
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
				if types.Equal(meta.Kind, types.TypeOfLabels) && typeContainsMeta(bindingType, meta.ID) {
					ch.addCodedError(diagnostic.ErrMissingLabel, n.S,
						"named capability requires explicit @#label argument")
				}
			}
		}
		ch.walkValidateLabel(n.Expr, bindingType, labelLamDepth)

	case *ir.Lam:
		ch.walkValidateLabel(n.Body, bindingType, labelLamDepth)
	case *ir.App:
		ch.walkValidateLabel(n.Fun, bindingType, labelLamDepth)
		ch.walkValidateLabel(n.Arg, bindingType, labelLamDepth)
	case *ir.Case:
		ch.walkValidateLabel(n.Scrutinee, bindingType, labelLamDepth)
		for _, a := range n.Alts {
			ch.walkValidateLabel(a.Body, bindingType, labelLamDepth)
		}
	case *ir.Bind:
		ch.walkValidateLabel(n.Comp, bindingType, labelLamDepth)
		ch.walkValidateLabel(n.Body, bindingType, labelLamDepth)
	case *ir.Pure:
		ch.walkValidateLabel(n.Expr, bindingType, labelLamDepth)
	case *ir.Thunk:
		ch.walkValidateLabel(n.Comp, bindingType, labelLamDepth)
	case *ir.Force:
		ch.walkValidateLabel(n.Expr, bindingType, labelLamDepth)
	case *ir.Fix:
		ch.walkValidateLabel(n.Body, bindingType, labelLamDepth)
	}
}

// typeContainsMeta reports whether a type contains TyMeta with the given ID.
func typeContainsMeta(ty types.Type, metaID int) bool {
	matches := types.CollectTypes(ty, func(t types.Type) (struct{}, bool) {
		if m, ok := t.(*types.TyMeta); ok && m.ID == metaID {
			return struct{}{}, true
		}
		return struct{}{}, false
	})
	return len(matches) > 0
}
