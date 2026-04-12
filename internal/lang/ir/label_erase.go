package ir

import "github.com/cwd-k2/gicel/internal/lang/types"

// EraseLabelArgs converts type applications of label-kinded arguments
// into term-level string literal applications. This is the label erasure
// step for Named Capabilities.
//
// Two forms are handled:
//  1. Concrete label literal: TyApp{expr, TyCon{name, L1, IsLabel=true}}
//     → App{expr, Lit{name}}
//  2. Label type variable: TyApp{expr, TyVar{name}} where name is bound
//     by an enclosing TyLam with Label kind
//     → App{expr, Var{Name: name}}
//
// For case (2), the enclosing TyLam is also lowered to a term-level Lam
// so the label flows as a runtime value.
//
// Must run before optimization and free-variable annotation, since the
// transformation changes the Core IR structure (TyApp → App + Lit/Var,
// TyLam → Lam).
func EraseLabelArgs(c Core) Core {
	return eraseLabelRec(c, nil)
}

// EraseLabelArgsProgram applies label erasure to all bindings.
func EraseLabelArgsProgram(prog *Program) {
	for i, b := range prog.Bindings {
		prog.Bindings[i].Expr = EraseLabelArgs(b.Expr)
	}
}

// bodyHasLabelTyVar reports whether c contains a TyApp whose TyArg is
// TyVar with the given name. This distinguishes explicit @l usage
// (TyVar) from implicit instantiation via polymorphic wrapper (TyMeta).
func bodyHasLabelTyVar(c Core, name string) bool {
	found := false
	Walk(c, func(n Core) bool {
		if found {
			return false
		}
		if ta, ok := n.(*TyApp); ok {
			if tv, ok := ta.TyArg.(*types.TyVar); ok && tv.Name == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// eraseLabelRec recursively walks Core IR, tracking label-kinded TyLam
// bindings in scope to enable variable label erasure.
//
// This pass uses in-place parent-pointer mutation: when a child changes
// but the parent node type stays the same, the parent's field is updated
// directly instead of allocating a new parent. New node allocation occurs
// only when the node type itself changes (TyApp → App, TyLam → Lam).
// This is safe because the IR is freshly produced by the type checker and
// is not yet cached or shared.
func eraseLabelRec(c Core, labelVars map[string]bool) Core {
	if c == nil {
		return nil
	}
	switch n := c.(type) {
	case *TyLam:
		if types.Equal(n.Kind, types.TypeOfLabels) && bodyHasLabelTyVar(n.Body, n.TyParam) {
			if labelVars == nil {
				labelVars = make(map[string]bool)
			}
			labelVars[n.TyParam] = true
			body := eraseLabelRec(n.Body, labelVars)
			delete(labelVars, n.TyParam)
			// Type change: TyLam → Lam. Must allocate.
			return &Lam{Param: n.TyParam, Body: body, S: n.S}
		}
		body := eraseLabelRec(n.Body, labelVars)
		if body != n.Body {
			n.Body = body
		}
		return n

	case *TyApp:
		expr := eraseLabelRec(n.Expr, labelVars)
		// Case 1: concrete label literal. Type change: TyApp → App.
		if con, ok := n.TyArg.(*types.TyCon); ok && types.IsKindLevel(con.Level) && con.IsLabel {
			return &App{Fun: expr, Arg: &Lit{Value: "#" + con.Name}, S: n.S}
		}
		// Case 2: label type variable. Type change: TyApp → App.
		if tv, ok := n.TyArg.(*types.TyVar); ok && labelVars[tv.Name] {
			return &App{Fun: expr, Arg: &Var{Name: tv.Name, S: n.S}, S: n.S}
		}
		if expr != n.Expr {
			n.Expr = expr
		}
		return n

	case *Lam:
		body := eraseLabelRec(n.Body, labelVars)
		if body != n.Body {
			n.Body = body
		}
		return n

	case *App:
		fun := eraseLabelRec(n.Fun, labelVars)
		arg := eraseLabelRec(n.Arg, labelVars)
		if fun != n.Fun {
			n.Fun = fun
		}
		if arg != n.Arg {
			n.Arg = arg
		}
		return n

	case *Case:
		scr := eraseLabelRec(n.Scrutinee, labelVars)
		if scr != n.Scrutinee {
			n.Scrutinee = scr
		}
		for i, a := range n.Alts {
			b := eraseLabelRec(a.Body, labelVars)
			if b != a.Body {
				n.Alts[i].Body = b
			}
		}
		return n

	case *Bind:
		comp := eraseLabelRec(n.Comp, labelVars)
		body := eraseLabelRec(n.Body, labelVars)
		if comp != n.Comp {
			n.Comp = comp
		}
		if body != n.Body {
			n.Body = body
		}
		return n

	case *Pure:
		expr := eraseLabelRec(n.Expr, labelVars)
		if expr != n.Expr {
			n.Expr = expr
		}
		return n

	case *Thunk:
		comp := eraseLabelRec(n.Comp, labelVars)
		if comp != n.Comp {
			n.Comp = comp
		}
		return n

	case *Force:
		expr := eraseLabelRec(n.Expr, labelVars)
		if expr != n.Expr {
			n.Expr = expr
		}
		return n

	case *Merge:
		left := eraseLabelRec(n.Left, labelVars)
		right := eraseLabelRec(n.Right, labelVars)
		if left != n.Left {
			n.Left = left
		}
		if right != n.Right {
			n.Right = right
		}
		return n

	case *Fix:
		body := eraseLabelRec(n.Body, labelVars)
		if body != n.Body {
			n.Body = body
		}
		return n

	case *RecordLit:
		for i, f := range n.Fields {
			v := eraseLabelRec(f.Value, labelVars)
			if v != f.Value {
				n.Fields[i].Value = v
			}
		}
		return n

	case *RecordProj:
		rec := eraseLabelRec(n.Record, labelVars)
		if rec != n.Record {
			n.Record = rec
		}
		return n

	case *RecordUpdate:
		rec := eraseLabelRec(n.Record, labelVars)
		if rec != n.Record {
			n.Record = rec
		}
		for i, f := range n.Updates {
			v := eraseLabelRec(f.Value, labelVars)
			if v != f.Value {
				n.Updates[i].Value = v
			}
		}
		return n

	default:
		// Var, Lit, Con, PrimOp, Field, Error — leaf nodes or not label-relevant.
		return c
	}
}
