package core

import "github.com/cwd-k2/gomputation/internal/types"

// FreeVars returns term-level free variables in a Core expression.
func FreeVars(c Core) map[string]struct{} {
	fv := make(map[string]struct{})
	bound := make(map[string]int)
	freeVarsRec(c, bound, fv)
	return fv
}

// bind/unbind use a depth counter to handle shadowing without map copies.
func bind(bound map[string]int, name string)   { bound[name]++ }
func unbind(bound map[string]int, name string) { bound[name]--; if bound[name] == 0 { delete(bound, name) } }

func freeVarsRec(c Core, bound map[string]int, fv map[string]struct{}) {
	switch n := c.(type) {
	case *Var:
		if bound[n.Name] == 0 {
			fv[n.Name] = struct{}{}
		}
	case *Lam:
		bind(bound, n.Param)
		freeVarsRec(n.Body, bound, fv)
		unbind(bound, n.Param)
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
			names := alt.Pattern.Bindings()
			for _, name := range names {
				bind(bound, name)
			}
			freeVarsRec(alt.Body, bound, fv)
			for _, name := range names {
				unbind(bound, name)
			}
		}
	case *LetRec:
		for _, b := range n.Bindings {
			bind(bound, b.Name)
		}
		for _, b := range n.Bindings {
			freeVarsRec(b.Expr, bound, fv)
		}
		freeVarsRec(n.Body, bound, fv)
		for _, b := range n.Bindings {
			unbind(bound, b.Name)
		}
	case *Pure:
		freeVarsRec(n.Expr, bound, fv)
	case *Bind:
		freeVarsRec(n.Comp, bound, fv)
		bind(bound, n.Var)
		freeVarsRec(n.Body, bound, fv)
		unbind(bound, n.Var)
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

// AnnotateFreeVars populates FV fields on Lam and Thunk nodes in a single
// bottom-up pass (O(n)). For each Lam, FV = free vars of body ∖ {param}.
// For each Thunk, FV = free vars of comp.
func AnnotateFreeVars(c Core) {
	annotateFV(c)
}

// AnnotateFreeVarsProgram annotates all bindings in a Program.
func AnnotateFreeVarsProgram(p *Program) {
	for _, b := range p.Bindings {
		AnnotateFreeVars(b.Expr)
	}
}

// annotateFV computes free variables bottom-up, annotating Lam and Thunk nodes.
// Returns the set of free variables in the expression.
// Unlike FreeVars/freeVarsRec, this does NOT propagate Lam params into bound —
// outer Lam params are free from an inner closure's perspective (they are captured).
// Only LetRec names, Case alt bindings, and Bind vars are propagated as bound,
// since they are resolved within the same scope.
func annotateFV(c Core) map[string]struct{} {
	switch n := c.(type) {
	case *Var:
		return map[string]struct{}{n.Name: {}}
	case *Lam:
		bodyFV := annotateFV(n.Body)
		// Remove the param — it comes from application, not from captured env.
		delete(bodyFV, n.Param)
		n.FV = setToSlice(bodyFV)
		return bodyFV
	case *App:
		return mergeFV(annotateFV(n.Fun), annotateFV(n.Arg))
	case *TyApp:
		return annotateFV(n.Expr)
	case *TyLam:
		return annotateFV(n.Body)
	case *Con:
		var result map[string]struct{}
		for _, arg := range n.Args {
			result = mergeFV(result, annotateFV(arg))
		}
		return result
	case *Case:
		result := annotateFV(n.Scrutinee)
		for _, alt := range n.Alts {
			altFV := annotateFV(alt.Body)
			// Remove pattern-bound vars — they are local to each alt.
			for _, name := range alt.Pattern.Bindings() {
				delete(altFV, name)
			}
			result = mergeFV(result, altFV)
		}
		return result
	case *LetRec:
		// LetRec names are mutually visible — remove them from the result.
		var result map[string]struct{}
		for _, b := range n.Bindings {
			result = mergeFV(result, annotateFV(b.Expr))
		}
		result = mergeFV(result, annotateFV(n.Body))
		for _, b := range n.Bindings {
			delete(result, b.Name)
		}
		return result
	case *Pure:
		return annotateFV(n.Expr)
	case *Bind:
		compFV := annotateFV(n.Comp)
		bodyFV := annotateFV(n.Body)
		// Bind var is local to the body.
		delete(bodyFV, n.Var)
		return mergeFV(compFV, bodyFV)
	case *Thunk:
		compFV := annotateFV(n.Comp)
		n.FV = setToSlice(compFV)
		return compFV
	case *Force:
		return annotateFV(n.Expr)
	case *PrimOp:
		var result map[string]struct{}
		for _, arg := range n.Args {
			result = mergeFV(result, annotateFV(arg))
		}
		return result
	case *Lit:
		return nil
	case *RecordLit:
		var result map[string]struct{}
		for _, f := range n.Fields {
			result = mergeFV(result, annotateFV(f.Value))
		}
		return result
	case *RecordProj:
		return annotateFV(n.Record)
	case *RecordUpdate:
		result := annotateFV(n.Record)
		for _, f := range n.Updates {
			result = mergeFV(result, annotateFV(f.Value))
		}
		return result
	default:
		return nil
	}
}

func mergeFV(a, b map[string]struct{}) map[string]struct{} {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	for k := range b {
		a[k] = struct{}{}
	}
	return a
}

func setToSlice(s map[string]struct{}) []string {
	if len(s) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(s))
	for k := range s {
		result = append(result, k)
	}
	return result
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
