package core

// Walk visits every Core node in depth-first order.
// The visitor returns false to stop traversal.
func Walk(c Core, visit func(Core) bool) {
	if !visit(c) {
		return
	}
	switch n := c.(type) {
	case *Var:
		// leaf
	case *Lam:
		Walk(n.Body, visit)
	case *App:
		Walk(n.Fun, visit)
		Walk(n.Arg, visit)
	case *TyApp:
		Walk(n.Expr, visit)
	case *TyLam:
		Walk(n.Body, visit)
	case *Con:
		for _, arg := range n.Args {
			Walk(arg, visit)
		}
	case *Case:
		Walk(n.Scrutinee, visit)
		for _, alt := range n.Alts {
			Walk(alt.Body, visit)
		}
	case *LetRec:
		for _, b := range n.Bindings {
			Walk(b.Expr, visit)
		}
		Walk(n.Body, visit)
	case *Pure:
		Walk(n.Expr, visit)
	case *Bind:
		Walk(n.Comp, visit)
		Walk(n.Body, visit)
	case *Thunk:
		Walk(n.Comp, visit)
	case *Force:
		Walk(n.Expr, visit)
	case *PrimOp:
		for _, arg := range n.Args {
			Walk(arg, visit)
		}
	}
}

// Transform applies a function to every Core node bottom-up.
func Transform(c Core, f func(Core) Core) Core {
	switch n := c.(type) {
	case *Var:
		return f(n)
	case *Lam:
		return f(&Lam{Param: n.Param, ParamType: n.ParamType, Body: Transform(n.Body, f), S: n.S})
	case *App:
		return f(&App{Fun: Transform(n.Fun, f), Arg: Transform(n.Arg, f), S: n.S})
	case *TyApp:
		return f(&TyApp{Expr: Transform(n.Expr, f), TyArg: n.TyArg, S: n.S})
	case *TyLam:
		return f(&TyLam{TyParam: n.TyParam, Kind: n.Kind, Body: Transform(n.Body, f), S: n.S})
	case *Con:
		args := make([]Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = Transform(a, f)
		}
		return f(&Con{Name: n.Name, Args: args, S: n.S})
	case *Case:
		alts := make([]Alt, len(n.Alts))
		for i, alt := range n.Alts {
			alts[i] = Alt{Pattern: alt.Pattern, Body: Transform(alt.Body, f), S: alt.S}
		}
		return f(&Case{Scrutinee: Transform(n.Scrutinee, f), Alts: alts, S: n.S})
	case *LetRec:
		bindings := make([]Binding, len(n.Bindings))
		for i, b := range n.Bindings {
			bindings[i] = Binding{Name: b.Name, Type: b.Type, Expr: Transform(b.Expr, f), S: b.S}
		}
		return f(&LetRec{Bindings: bindings, Body: Transform(n.Body, f), S: n.S})
	case *Pure:
		return f(&Pure{Expr: Transform(n.Expr, f), S: n.S})
	case *Bind:
		return f(&Bind{Comp: Transform(n.Comp, f), Var: n.Var, Body: Transform(n.Body, f), S: n.S})
	case *Thunk:
		return f(&Thunk{Comp: Transform(n.Comp, f), S: n.S})
	case *Force:
		return f(&Force{Expr: Transform(n.Expr, f), S: n.S})
	case *PrimOp:
		args := make([]Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = Transform(a, f)
		}
		return f(&PrimOp{Name: n.Name, Args: args, S: n.S})
	default:
		return f(c)
	}
}
