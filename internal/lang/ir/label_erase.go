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
func eraseLabelRec(c Core, labelVars map[string]bool) Core {
	if c == nil {
		return nil
	}
	switch n := c.(type) {
	case *TyLam:
		if types.Equal(n.Kind, types.TypeOfLabels) && bodyHasLabelTyVar(n.Body, n.TyParam) {
			// Lower TyLam with Label kind to term-level Lam, but only when
			// the body actually contains TyApp{_, TyVar{name}} referencing
			// this parameter (i.e., explicit @l usage in source code).
			// Polymorphic wrappers (e.g., newAt := _primNamed) have TyMeta
			// in TyApp.TyArg, not TyVar, so they are left as TyLam (erased
			// at runtime, letting the label pass through to the primitive).
			if labelVars == nil {
				labelVars = make(map[string]bool)
			}
			labelVars[n.TyParam] = true
			body := eraseLabelRec(n.Body, labelVars)
			delete(labelVars, n.TyParam)
			return &Lam{Param: n.TyParam, Body: body, S: n.S}
		}
		body := eraseLabelRec(n.Body, labelVars)
		if body == n.Body {
			return n
		}
		return &TyLam{TyParam: n.TyParam, Kind: n.Kind, Body: body, S: n.S}

	case *TyApp:
		expr := eraseLabelRec(n.Expr, labelVars)
		// Case 1: concrete label literal.
		if con, ok := n.TyArg.(*types.TyCon); ok && types.IsKindLevel(con.Level) && con.IsLabel {
			return &App{Fun: expr, Arg: &Lit{Value: con.Name}, S: n.S}
		}
		// Case 2: label type variable bound by an enclosing TyLam.
		if tv, ok := n.TyArg.(*types.TyVar); ok && labelVars[tv.Name] {
			return &App{Fun: expr, Arg: &Var{Name: tv.Name, S: n.S}, S: n.S}
		}
		if expr == n.Expr {
			return n
		}
		return &TyApp{Expr: expr, TyArg: n.TyArg, S: n.S}

	case *Lam:
		body := eraseLabelRec(n.Body, labelVars)
		if body == n.Body {
			return n
		}
		return &Lam{Param: n.Param, Body: body, Generated: n.Generated, S: n.S}

	case *App:
		fun := eraseLabelRec(n.Fun, labelVars)
		arg := eraseLabelRec(n.Arg, labelVars)
		if fun == n.Fun && arg == n.Arg {
			return n
		}
		return &App{Fun: fun, Arg: arg, S: n.S}

	case *Case:
		scr := eraseLabelRec(n.Scrutinee, labelVars)
		alts := n.Alts
		changed := scr != n.Scrutinee
		for i, a := range alts {
			b := eraseLabelRec(a.Body, labelVars)
			if b != a.Body {
				if !changed {
					alts = make([]Alt, len(n.Alts))
					copy(alts, n.Alts)
					changed = true
				}
				alts[i] = Alt{Pattern: a.Pattern, Body: b}
			}
		}
		if !changed {
			return n
		}
		return &Case{Scrutinee: scr, Alts: alts, S: n.S}

	case *Bind:
		comp := eraseLabelRec(n.Comp, labelVars)
		body := eraseLabelRec(n.Body, labelVars)
		if comp == n.Comp && body == n.Body {
			return n
		}
		return &Bind{Var: n.Var, Comp: comp, Body: body, S: n.S}

	case *Pure:
		expr := eraseLabelRec(n.Expr, labelVars)
		if expr == n.Expr {
			return n
		}
		return &Pure{Expr: expr, S: n.S}

	case *Thunk:
		comp := eraseLabelRec(n.Comp, labelVars)
		if comp == n.Comp {
			return n
		}
		return &Thunk{Comp: comp, S: n.S}

	case *Force:
		expr := eraseLabelRec(n.Expr, labelVars)
		if expr == n.Expr {
			return n
		}
		return &Force{Expr: expr, S: n.S}

	case *Merge:
		left := eraseLabelRec(n.Left, labelVars)
		right := eraseLabelRec(n.Right, labelVars)
		if left == n.Left && right == n.Right {
			return n
		}
		return &Merge{Left: left, Right: right, LeftLabels: n.LeftLabels, RightLabels: n.RightLabels, S: n.S}

	case *Fix:
		body := eraseLabelRec(n.Body, labelVars)
		if body == n.Body {
			return n
		}
		return &Fix{Name: n.Name, Body: body, S: n.S}

	case *RecordLit:
		fields := n.Fields
		changed := false
		for i, f := range fields {
			v := eraseLabelRec(f.Value, labelVars)
			if v != f.Value {
				if !changed {
					fields = make([]Field, len(n.Fields))
					copy(fields, n.Fields)
					changed = true
				}
				fields[i] = Field{Label: f.Label, Value: v}
			}
		}
		if !changed {
			return n
		}
		return &RecordLit{Fields: fields, S: n.S}

	case *RecordProj:
		rec := eraseLabelRec(n.Record, labelVars)
		if rec == n.Record {
			return n
		}
		return &RecordProj{Record: rec, Label: n.Label, S: n.S}

	case *RecordUpdate:
		rec := eraseLabelRec(n.Record, labelVars)
		updates := n.Updates
		changed := rec != n.Record
		for i, f := range updates {
			v := eraseLabelRec(f.Value, labelVars)
			if v != f.Value {
				if !changed {
					updates = make([]Field, len(n.Updates))
					copy(updates, n.Updates)
					changed = true
				}
				updates[i] = Field{Label: f.Label, Value: v}
			}
		}
		if !changed {
			return n
		}
		return &RecordUpdate{Record: rec, Updates: updates, S: n.S}

	default:
		// Var, Lit, Con, PrimOp, Field, Error — leaf nodes or not label-relevant.
		return c
	}
}
