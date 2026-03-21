package ir

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
		// Flatten left-spine of App to avoid stack overflow on deeply
		// left-associative operator chains. visit(c) was already called
		// above, so pass skipRoot=true.
		walkLeftSpine(n, visit, depth, true)
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
	case *Fix:
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

// walkLeftSpine iteratively descends the left spine of App nodes (and
// transparent wrappers TyApp/TyLam), visiting right-side children
// recursively. This prevents Go stack overflow on deeply left-nested
// operator chains while preserving the depth budget for non-spine nodes.
func walkLeftSpine(app *App, visit func(Core) bool, depth int, skipRoot bool) {
	// Collect right-side children during spine descent; visit them after.
	type rightChild struct {
		node  Core
		depth int
	}
	var rights []rightChild

	cur := Core(app)
	first := true
	for {
		switch n := cur.(type) {
		case *App:
			if !(first && skipRoot) {
				if !visit(n) {
					return
				}
			}
			first = false
			rights = append(rights, rightChild{n.Arg, depth + 1})
			cur = n.Fun
			continue
		case *TyApp:
			if !visit(n) {
				return
			}
			cur = n.Expr
			continue
		case *TyLam:
			if !visit(n) {
				return
			}
			cur = n.Body
			continue
		default:
			walkRec(n, visit, depth+1)
		}
		break
	}

	for i := len(rights) - 1; i >= 0; i-- {
		walkRec(rights[i].node, visit, rights[i].depth)
	}
}

// Transform applies a function to every Core node bottom-up.
func Transform(c Core, f func(Core) Core) Core {
	return transformRec(c, f, 0)
}

func transformRec(c Core, f func(Core) Core, depth int) Core {
	if depth > maxTraversalDepth {
		// For left-spine App chains, use iterative descent.
		if _, ok := c.(*App); ok {
			return transformLeftSpine(c, f, depth)
		}
		return c
	}
	switch n := c.(type) {
	case *Var:
		return f(n)
	case *Lam:
		return f(&Lam{Param: n.Param, ParamType: n.ParamType, Body: transformRec(n.Body, f, depth+1), FV: n.FV, Generated: n.Generated, S: n.S})
	case *App:
		return transformLeftSpine(c, f, depth)
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
			alts[i] = Alt{Pattern: alt.Pattern, Body: transformRec(alt.Body, f, depth+1), Generated: alt.Generated, S: alt.S}
		}
		return f(&Case{Scrutinee: transformRec(n.Scrutinee, f, depth+1), Alts: alts, S: n.S})
	case *Fix:
		return f(&Fix{Name: n.Name, Body: transformRec(n.Body, f, depth+1), S: n.S})
	case *Pure:
		return f(&Pure{Expr: transformRec(n.Expr, f, depth+1), S: n.S})
	case *Bind:
		return f(&Bind{Comp: transformRec(n.Comp, f, depth+1), Var: n.Var, Body: transformRec(n.Body, f, depth+1), Generated: n.Generated, S: n.S})
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

// transformLeftSpine iteratively processes the left spine of App nodes
// (including TyApp/TyLam wrappers), applying f bottom-up. This allows
// Transform to handle arbitrarily deep left-associative operator chains
// without exceeding the Go call stack.
func transformLeftSpine(c Core, f func(Core) Core, baseDepth int) Core {
	type spineNode struct {
		app *App // original App node (for span)
		arg Core // right child (already transformed or pending)
	}
	var spine []spineNode

	// Phase 1: unwind the left spine, transforming right children.
	cur := c
	for {
		switch n := cur.(type) {
		case *App:
			arg := transformRec(n.Arg, f, baseDepth+1)
			spine = append(spine, spineNode{app: n, arg: arg})
			cur = n.Fun
			continue
		case *TyApp:
			inner := transformLeftSpineOrRec(n.Expr, f, baseDepth)
			cur = f(&TyApp{Expr: inner, TyArg: n.TyArg, S: n.S})
			goto rebuild
		case *TyLam:
			inner := transformLeftSpineOrRec(n.Body, f, baseDepth)
			cur = f(&TyLam{TyParam: n.TyParam, Kind: n.Kind, Body: inner, S: n.S})
			goto rebuild
		default:
			cur = transformRec(n, f, baseDepth+1)
			goto rebuild
		}
	}

rebuild:
	// Phase 2: rebuild the spine from the root outward.
	for i := len(spine) - 1; i >= 0; i-- {
		sn := spine[i]
		cur = f(&App{Fun: cur, Arg: sn.arg, S: sn.app.S})
	}
	return cur
}

// transformLeftSpineOrRec continues with left-spine processing if the
// node is an App, otherwise falls back to regular recursion.
func transformLeftSpineOrRec(c Core, f func(Core) Core, depth int) Core {
	if _, ok := c.(*App); ok {
		return transformLeftSpine(c, f, depth)
	}
	return transformRec(c, f, depth+1)
}
