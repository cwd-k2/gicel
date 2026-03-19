package core

import "fmt"

// Walk visits every Core node in depth-first order.
// The visitor returns false to stop traversal.
func Walk(c Core, visit func(Core) bool) {
	walkRec(c, visit, 0)
}

func walkRec(c Core, visit func(Core) bool, depth int) {
	if depth > maxTraversalDepth {
		return
	}
	if !visit(c) {
		return
	}
	switch n := c.(type) {
	case *Var:
		// leaf
	case *Lam:
		walkRec(n.Body, visit, depth+1)
	case *App:
		walkRec(n.Fun, visit, depth+1)
		walkRec(n.Arg, visit, depth+1)
	case *TyApp:
		walkRec(n.Expr, visit, depth+1)
	case *TyLam:
		walkRec(n.Body, visit, depth+1)
	case *Con:
		for _, arg := range n.Args {
			walkRec(arg, visit, depth+1)
		}
	case *Case:
		walkRec(n.Scrutinee, visit, depth+1)
		for _, alt := range n.Alts {
			walkRec(alt.Body, visit, depth+1)
		}
	case *LetRec:
		for _, b := range n.Bindings {
			walkRec(b.Expr, visit, depth+1)
		}
		walkRec(n.Body, visit, depth+1)
	case *Pure:
		walkRec(n.Expr, visit, depth+1)
	case *Bind:
		walkRec(n.Comp, visit, depth+1)
		walkRec(n.Body, visit, depth+1)
	case *Thunk:
		walkRec(n.Comp, visit, depth+1)
	case *Force:
		walkRec(n.Expr, visit, depth+1)
	case *PrimOp:
		for _, arg := range n.Args {
			walkRec(arg, visit, depth+1)
		}
	case *Lit:
		// leaf
	case *RecordLit:
		for _, f := range n.Fields {
			walkRec(f.Value, visit, depth+1)
		}
	case *RecordProj:
		walkRec(n.Record, visit, depth+1)
	case *RecordUpdate:
		walkRec(n.Record, visit, depth+1)
		for _, f := range n.Updates {
			walkRec(f.Value, visit, depth+1)
		}
	default:
		panic(fmt.Sprintf("Walk: unhandled Core node %T", c))
	}
}

// Transform applies a function to every Core node bottom-up.
func Transform(c Core, f func(Core) Core) Core {
	return transformRec(c, f, 0)
}

func transformRec(c Core, f func(Core) Core, depth int) Core {
	if depth > maxTraversalDepth {
		return c
	}
	switch n := c.(type) {
	case *Var:
		return f(n)
	case *Lam:
		return f(&Lam{Param: n.Param, ParamType: n.ParamType, Body: transformRec(n.Body, f, depth+1), S: n.S})
	case *App:
		return f(&App{Fun: transformRec(n.Fun, f, depth+1), Arg: transformRec(n.Arg, f, depth+1), S: n.S})
	case *TyApp:
		return f(&TyApp{Expr: transformRec(n.Expr, f, depth+1), TyArg: n.TyArg, S: n.S})
	case *TyLam:
		return f(&TyLam{TyParam: n.TyParam, Kind: n.Kind, Body: transformRec(n.Body, f, depth+1), S: n.S})
	case *Con:
		args := make([]Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = transformRec(a, f, depth+1)
		}
		return f(&Con{Name: n.Name, Args: args, S: n.S})
	case *Case:
		alts := make([]Alt, len(n.Alts))
		for i, alt := range n.Alts {
			alts[i] = Alt{Pattern: alt.Pattern, Body: transformRec(alt.Body, f, depth+1), S: alt.S}
		}
		return f(&Case{Scrutinee: transformRec(n.Scrutinee, f, depth+1), Alts: alts, S: n.S})
	case *LetRec:
		bindings := make([]Binding, len(n.Bindings))
		for i, b := range n.Bindings {
			bindings[i] = Binding{Name: b.Name, Type: b.Type, Expr: transformRec(b.Expr, f, depth+1), S: b.S}
		}
		return f(&LetRec{Bindings: bindings, Body: transformRec(n.Body, f, depth+1), S: n.S})
	case *Pure:
		return f(&Pure{Expr: transformRec(n.Expr, f, depth+1), S: n.S})
	case *Bind:
		return f(&Bind{Comp: transformRec(n.Comp, f, depth+1), Var: n.Var, Body: transformRec(n.Body, f, depth+1), S: n.S})
	case *Thunk:
		return f(&Thunk{Comp: transformRec(n.Comp, f, depth+1), S: n.S})
	case *Force:
		return f(&Force{Expr: transformRec(n.Expr, f, depth+1), S: n.S})
	case *PrimOp:
		args := make([]Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = transformRec(a, f, depth+1)
		}
		return f(&PrimOp{Name: n.Name, Arity: n.Arity, Effectful: n.Effectful, Args: args, S: n.S})
	case *Lit:
		return f(n)
	case *RecordLit:
		fields := make([]RecordField, len(n.Fields))
		for i, fld := range n.Fields {
			fields[i] = RecordField{Label: fld.Label, Value: transformRec(fld.Value, f, depth+1)}
		}
		return f(&RecordLit{Fields: fields, S: n.S})
	case *RecordProj:
		return f(&RecordProj{Record: transformRec(n.Record, f, depth+1), Label: n.Label, S: n.S})
	case *RecordUpdate:
		updates := make([]RecordField, len(n.Updates))
		for i, fld := range n.Updates {
			updates[i] = RecordField{Label: fld.Label, Value: transformRec(fld.Value, f, depth+1)}
		}
		return f(&RecordUpdate{Record: transformRec(n.Record, f, depth+1), Updates: updates, S: n.S})
	default:
		panic(fmt.Sprintf("Transform: unhandled Core node %T", c))
	}
}
