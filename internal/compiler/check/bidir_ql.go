// bidir_ql.go — Quick Look impredicativity pre-pass.
//
// Implements a shallow structural pre-unification (qlUnify) that permits
// solving metavariables with polytypes (TyForall). This enables impredicative
// instantiation for multi-argument constructors and functions.
//
// Reference: Serrano et al., "A Quick Look at Impredicativity" (ICFP 2020).
package check

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// qlUnify performs shallow structural matching between two types,
// permitting meta → polytype solutions. Runs inside withTrial;
// returns true if all attempted unifications succeeded.
//
// Key differences from Unify:
//   - Solves meta with TyForall (impredicative).
//   - Does NOT recurse under TyForall bodies.
//   - Does NOT emit constraints or errors.
//   - Skips TyCBPV and TyEvidenceRow (conservative).
func (ch *Checker) qlUnify(a, b types.Type) bool {
	a = ch.unifier.Zonk(a)
	b = ch.unifier.Zonk(b)

	if a == b {
		return true
	}

	// Meta on either side: solve (including polytype solutions).
	if m, ok := a.(*types.TyMeta); ok {
		return ch.unifier.SolveFreshMeta(m, b)
	}
	if m, ok := b.(*types.TyMeta); ok {
		return ch.unifier.SolveFreshMeta(m, a)
	}

	// Structural recursion for type constructors.
	switch at := a.(type) {
	case *types.TyApp:
		if bt, ok := b.(*types.TyApp); ok {
			return ch.qlUnify(at.Fun, bt.Fun) && ch.qlUnify(at.Arg, bt.Arg)
		}
	case *types.TyArrow:
		if bt, ok := b.(*types.TyArrow); ok {
			return ch.qlUnify(at.From, bt.From) && ch.qlUnify(at.To, bt.To)
		}
	case *types.TyCon:
		if bt, ok := b.(*types.TyCon); ok {
			return at.Name == bt.Name
		}
	}

	// Conservative: do not attempt to match forall bodies, CBPV, rows, etc.
	return false
}

// spineArg is a collected argument from an application spine.
type spineArg struct {
	expr syntax.Expr
}

// collectSpine flattens a left-associative App tree into (head, args).
// TyApp nodes are skipped (they don't contribute value-level arguments).
func collectSpine(expr syntax.Expr) (syntax.Expr, []spineArg) {
	var args []spineArg
	for {
		switch e := expr.(type) {
		case *syntax.ExprApp:
			args = append(args, spineArg{expr: e.Arg})
			expr = e.Fun
		case *syntax.ExprTyApp:
			expr = e.Expr
		default:
			// Reverse to get left-to-right order.
			for i, j := 0, len(args)-1; i < j; i, j = i+1, j-1 {
				args[i], args[j] = args[j], args[i]
			}
			return expr, args
		}
	}
}

// checkAppQL checks a multi-argument application using the Quick Look strategy.
// It infers the head, instantiates foralls, uses qlUnify to propagate expected
// type info into instantiation metas, then checks each argument and builds Core.
//
// Returns (coreExpr, true) if QL handled the application.
// Returns (nil, false) if QL cannot apply (falls back to normal path).
func (ch *Checker) checkAppQL(head syntax.Expr, args []spineArg, expected types.Type, s span.Span) (ir.Core, bool) {
	// Step 1: Infer head with instantiation (standard path).
	headTy, headCore := ch.infer(head)

	// Step 2: Peel foralls and decompose arrows for each argument position.
	argTypes := make([]types.Type, 0, len(args))
	ty := headTy
	for range args {
		ty = ch.unifier.Zonk(ty)
		// Peel foralls (standard instantiation).
		ty = types.PeelForalls(ty, func(f *types.TyForall) (types.Type, types.LevelExpr) {
			if isLevelKind(f.Kind) {
				return ch.freshMeta(types.SortZero), ch.unifier.FreshLevelMeta()
			}
			meta := ch.freshMeta(f.Kind)
			headCore = &ir.TyApp{Expr: headCore, TyArg: meta, S: s}
			return meta, nil
		})
		// Peel evidence.
		for {
			if ev, ok := ty.(*types.TyEvidence); ok {
				for _, entry := range ev.Constraints.ConEntries() {
					placeholder := ch.freshName(prefixDictDefer)
					ch.emitClassConstraint(placeholder, entry, s)
					headCore = &ir.App{Fun: headCore, Arg: &ir.Var{Name: placeholder, S: s}, S: s}
				}
				ty = ev.Body
			} else {
				break
			}
		}
		// Decompose arrow.
		if arr, ok := ty.(*types.TyArrow); ok {
			argTypes = append(argTypes, arr.From)
			ty = arr.To
		} else {
			// Fall back: cannot decompose (type family, meta, etc.).
			return nil, false
		}
	}
	retTy := ty

	// Step 3: Quick Look — qlUnify retTy with expected to solve metas impredicatively.
	ch.withTrial(func() bool {
		return ch.qlUnify(retTy, expected)
	})

	// Step 4: Check each argument against the (now enriched) arg types. Build Core.
	core := headCore
	for i, arg := range args {
		argTy := ch.unifier.Zonk(argTypes[i])
		argCore := ch.check(arg.expr, argTy)
		argCore = ch.wrapAutoThunk(core, argCore, arg.expr.Span())
		core = &ir.App{Fun: core, Arg: argCore, S: arg.expr.Span()}
	}

	// Step 5: Final subsumption check.
	return ch.subsCheck(retTy, expected, core, s), true
}
