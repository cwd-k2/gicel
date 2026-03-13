package core

import "github.com/cwd-k2/gomputation/internal/types"

// FreeVars returns term-level free variables in a Core expression.
func FreeVars(c Core) map[string]struct{} {
	fv := make(map[string]struct{})
	freeVarsRec(c, nil, fv)
	return fv
}

func freeVarsRec(c Core, bound map[string]bool, fv map[string]struct{}) {
	switch n := c.(type) {
	case *Var:
		if bound == nil || !bound[n.Name] {
			fv[n.Name] = struct{}{}
		}
	case *Lam:
		newBound := extendBound(bound, n.Param)
		freeVarsRec(n.Body, newBound, fv)
	case *App:
		freeVarsRec(n.Fun, bound, fv)
		freeVarsRec(n.Arg, bound, fv)
	case *TyApp:
		freeVarsRec(n.Expr, bound, fv)
	case *TyLam:
		freeVarsRec(n.Body, bound, fv)
	case *Con:
		for _, arg := range n.Args {
			freeVarsRec(arg, bound, fv)
		}
	case *Case:
		freeVarsRec(n.Scrutinee, bound, fv)
		for _, alt := range n.Alts {
			altBound := bound
			for _, name := range alt.Pattern.Bindings() {
				altBound = extendBound(altBound, name)
			}
			freeVarsRec(alt.Body, altBound, fv)
		}
	case *LetRec:
		recBound := bound
		for _, b := range n.Bindings {
			recBound = extendBound(recBound, b.Name)
		}
		for _, b := range n.Bindings {
			freeVarsRec(b.Expr, recBound, fv)
		}
		freeVarsRec(n.Body, recBound, fv)
	case *Pure:
		freeVarsRec(n.Expr, bound, fv)
	case *Bind:
		freeVarsRec(n.Comp, bound, fv)
		newBound := extendBound(bound, n.Var)
		freeVarsRec(n.Body, newBound, fv)
	case *Thunk:
		freeVarsRec(n.Comp, bound, fv)
	case *Force:
		freeVarsRec(n.Expr, bound, fv)
	case *PrimOp:
		for _, arg := range n.Args {
			freeVarsRec(arg, bound, fv)
		}
	case *Lit:
		// leaf — no free variables
	case *RecordLit:
		for _, f := range n.Fields {
			freeVarsRec(f.Value, bound, fv)
		}
	case *RecordProj:
		freeVarsRec(n.Record, bound, fv)
	case *RecordUpdate:
		freeVarsRec(n.Record, bound, fv)
		for _, f := range n.Updates {
			freeVarsRec(f.Value, bound, fv)
		}
	}
}

func extendBound(bound map[string]bool, name string) map[string]bool {
	newBound := make(map[string]bool, len(bound)+1)
	for k, v := range bound {
		newBound[k] = v
	}
	newBound[name] = true
	return newBound
}

// FreeTypeVars returns type-level free variables in Core.
func FreeTypeVars(c Core) map[string]struct{} {
	fv := make(map[string]struct{})
	Walk(c, func(n Core) bool {
		switch node := n.(type) {
		case *Lam:
			if node.ParamType != nil {
				for k := range types.FreeVars(node.ParamType) {
					fv[k] = struct{}{}
				}
			}
		case *TyApp:
			if node.TyArg != nil {
				for k := range types.FreeVars(node.TyArg) {
					fv[k] = struct{}{}
				}
			}
		}
		return true
	})
	return fv
}
