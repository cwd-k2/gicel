package core

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/types"
)

// FreeVars returns term-level free variables in a Core expression.
func FreeVars(c Core) map[string]struct{} {
	fv := make(map[string]struct{})
	bound := make(map[string]int)
	freeVarsRec(c, bound, fv, 0)
	return fv
}

// bind/unbind use a depth counter to handle shadowing without map copies.
func bind(bound map[string]int, name string) { bound[name]++ }
func unbind(bound map[string]int, name string) {
	bound[name]--
	if bound[name] == 0 {
		delete(bound, name)
	}
}

func freeVarsRec(c Core, bound map[string]int, fv map[string]struct{}, depth int) {
	if depth > maxTraversalDepth {
		return
	}
	switch n := c.(type) {
	case *Var:
		key := varKey(n)
		if bound[key] == 0 {
			fv[key] = struct{}{}
		}
	case *Lam:
		bind(bound, n.Param)
		freeVarsRec(n.Body, bound, fv, depth+1)
		unbind(bound, n.Param)
	case *App:
		freeVarsRec(n.Fun, bound, fv, depth+1)
		freeVarsRec(n.Arg, bound, fv, depth+1)
	case *TyApp:
		freeVarsRec(n.Expr, bound, fv, depth+1)
	case *TyLam:
		freeVarsRec(n.Body, bound, fv, depth+1)
	case *Con:
		for _, arg := range n.Args {
			freeVarsRec(arg, bound, fv, depth+1)
		}
	case *Case:
		freeVarsRec(n.Scrutinee, bound, fv, depth+1)
		for _, alt := range n.Alts {
			names := alt.Pattern.Bindings()
			for _, name := range names {
				bind(bound, name)
			}
			freeVarsRec(alt.Body, bound, fv, depth+1)
			for _, name := range names {
				unbind(bound, name)
			}
		}
	case *Fix:
		bind(bound, n.Name)
		freeVarsRec(n.Body, bound, fv, depth+1)
		unbind(bound, n.Name)
	case *Pure:
		freeVarsRec(n.Expr, bound, fv, depth+1)
	case *Bind:
		freeVarsRec(n.Comp, bound, fv, depth+1)
		bind(bound, n.Var)
		freeVarsRec(n.Body, bound, fv, depth+1)
		unbind(bound, n.Var)
	case *Thunk:
		freeVarsRec(n.Comp, bound, fv, depth+1)
	case *Force:
		freeVarsRec(n.Expr, bound, fv, depth+1)
	case *PrimOp:
		for _, arg := range n.Args {
			freeVarsRec(arg, bound, fv, depth+1)
		}
	case *Lit:
		// leaf — no free variables
	case *RecordLit:
		for _, f := range n.Fields {
			freeVarsRec(f.Value, bound, fv, depth+1)
		}
	case *RecordProj:
		freeVarsRec(n.Record, bound, fv, depth+1)
	case *RecordUpdate:
		freeVarsRec(n.Record, bound, fv, depth+1)
		for _, f := range n.Updates {
			freeVarsRec(f.Value, bound, fv, depth+1)
		}
	}
}

// AnnotateFreeVars populates FV fields on Lam and Thunk nodes in a single
// bottom-up pass (O(n)). For each Lam, FV = free vars of body ∖ {param}.
// For each Thunk, FV = free vars of comp.
func AnnotateFreeVars(c Core) {
	annotateFV(c, 0)
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
// Only Fix names, Case alt bindings, and Bind vars are propagated as bound,
// since they are resolved within the same scope.
func annotateFV(c Core, depth int) map[string]struct{} {
	if depth > maxTraversalDepth {
		return nil
	}
	switch n := c.(type) {
	case *Var:
		return map[string]struct{}{varKey(n): {}}
	case *Lam:
		bodyFV := annotateFV(n.Body, depth+1)
		// Remove the param — it comes from application, not from captured env.
		delete(bodyFV, n.Param)
		n.FV = setToSlice(bodyFV)
		return bodyFV
	case *App:
		return mergeFV(annotateFV(n.Fun, depth+1), annotateFV(n.Arg, depth+1))
	case *TyApp:
		return annotateFV(n.Expr, depth+1)
	case *TyLam:
		return annotateFV(n.Body, depth+1)
	case *Con:
		var result map[string]struct{}
		for _, arg := range n.Args {
			result = mergeFV(result, annotateFV(arg, depth+1))
		}
		return result
	case *Case:
		result := annotateFV(n.Scrutinee, depth+1)
		for _, alt := range n.Alts {
			altFV := annotateFV(alt.Body, depth+1)
			// Remove pattern-bound vars — they are local to each alt.
			for _, name := range alt.Pattern.Bindings() {
				delete(altFV, name)
			}
			result = mergeFV(result, altFV)
		}
		return result
	case *Fix:
		// Fix name is visible in Body — remove it from the result.
		result := annotateFV(n.Body, depth+1)
		delete(result, n.Name)
		return result
	case *Pure:
		return annotateFV(n.Expr, depth+1)
	case *Bind:
		compFV := annotateFV(n.Comp, depth+1)
		bodyFV := annotateFV(n.Body, depth+1)
		// Bind var is local to the body.
		delete(bodyFV, n.Var)
		return mergeFV(compFV, bodyFV)
	case *Thunk:
		compFV := annotateFV(n.Comp, depth+1)
		n.FV = setToSlice(compFV)
		return compFV
	case *Force:
		return annotateFV(n.Expr, depth+1)
	case *PrimOp:
		var result map[string]struct{}
		for _, arg := range n.Args {
			result = mergeFV(result, annotateFV(arg, depth+1))
		}
		return result
	case *Lit:
		return nil
	case *RecordLit:
		var result map[string]struct{}
		for _, f := range n.Fields {
			result = mergeFV(result, annotateFV(f.Value, depth+1))
		}
		return result
	case *RecordProj:
		return annotateFV(n.Record, depth+1)
	case *RecordUpdate:
		result := annotateFV(n.Record, depth+1)
		for _, f := range n.Updates {
			result = mergeFV(result, annotateFV(f.Value, depth+1))
		}
		return result
	default:
		panic(fmt.Sprintf("annotateFV: unhandled Core node %T", c))
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

// varKey returns a map key for a Var node. Qualified vars use "module\x00name"
// to avoid collisions with local names.
func varKey(v *Var) string {
	if v.Module != "" {
		return v.Module + "\x00" + v.Name
	}
	return v.Name
}

// VarKey returns the map key for a Var node (exported for use in evaluator).
func VarKey(v *Var) string {
	return varKey(v)
}

// QualifiedKey builds a qualified environment key from module and name.
// This is the canonical constructor for the "module\x00name" key format.
func QualifiedKey(module, name string) string {
	return module + "\x00" + name
}

// freeTypeVars returns type-level free variables in Core.
func freeTypeVars(c Core) map[string]struct{} {
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
