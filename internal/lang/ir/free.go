package ir

import (
	"fmt"
	"sort"
	"strings"
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
		// Cache the qualified key on the Var node so subsequent walks
		// (index assignment, traverseFV, evaluator) reuse the same string
		// without re-concatenating module + name. Cold-path profile showed
		// varKey allocating ~240K objects from this single line; caching
		// drops it to one alloc per unique Var.
		key := n.Key
		if key == "" {
			key = varKey(n)
			n.Key = key
		}
		if bound[key] == 0 {
			fv[key] = struct{}{}
		}
	case *Lam:
		bind(bound, n.Param)
		freeVarsRec(n.Body, bound, fv, depth+1)
		unbind(bound, n.Param)
	case *App:
		// Flatten left-spine of App to avoid stack overflow on deeply
		// left-associative operator chains.
		freeVarsLeftSpine(n, bound, fv, depth)
		return
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
	case *Merge:
		freeVarsRec(n.Left, bound, fv, depth+1)
		freeVarsRec(n.Right, bound, fv, depth+1)
	case *PrimOp:
		for _, arg := range n.Args {
			freeVarsRec(arg, bound, fv, depth+1)
		}
	case *Lit:
		// leaf — no free variables
	case *Error:
		// error placeholder — no free variables
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
	default:
		panic(fmt.Sprintf("freeVarsRec: unhandled Core node %T", c))
	}
}

// freeVarsLeftSpine iteratively descends the left spine of App nodes
// (and transparent TyApp/TyLam wrappers), collecting free variables from
// right-side children. Prevents Go stack overflow on deeply left-nested
// operator chains.
func freeVarsLeftSpine(app *App, bound map[string]int, fv map[string]struct{}, depth int) {
	head, rights := unwindLeftSpine(app)
	freeVarsRec(head, bound, fv, depth+1)
	for i := len(rights) - 1; i >= 0; i-- {
		freeVarsRec(rights[i], bound, fv, depth+1)
	}
}

// AnnotateFreeVars populates FV fields on Lam and Thunk nodes in a single
// bottom-up pass (O(n)). For each Lam, FV = free vars of body ∖ {param}.
// For each Thunk, FV = free vars of comp.
func AnnotateFreeVars(c Core) {
	traverseFV(c, 0, annotateObs{})
}

// AnnotateFreeVarsProgram annotates all bindings in a Program.
func AnnotateFreeVarsProgram(p *Program) {
	for _, b := range p.Bindings {
		AnnotateFreeVars(b.Expr)
	}
}

// fvObserver receives computed free variables at annotation points during
// bottom-up FV traversal. The traversal calls the observer at each Lam,
// Thunk, and Merge node with the computed FV result.
type fvObserver interface {
	OnLam(lam *Lam, bodyFV fvResult)
	OnThunk(th *Thunk, compFV fvResult)
	OnMerge(m *Merge, leftFV, rightFV fvResult)
}

// annotateObs sets FV fields on Lam, Thunk, and Merge nodes.
type annotateObs struct{}

func (annotateObs) OnLam(lam *Lam, bodyFV fvResult) {
	if bodyFV.overflow {
		lam.FV = nil
	} else {
		lam.FV = setToSlice(bodyFV.vars)
	}
}

func (annotateObs) OnThunk(th *Thunk, compFV fvResult) {
	if compFV.overflow {
		th.FV = nil
	} else {
		th.FV = setToSlice(compFV.vars)
	}
}

func (annotateObs) OnMerge(m *Merge, leftFV, rightFV fvResult) {
	if leftFV.overflow {
		m.LeftFV = nil
	} else {
		m.LeftFV = setToSlice(leftFV.vars)
	}
	if rightFV.overflow {
		m.RightFV = nil
	} else {
		m.RightFV = setToSlice(rightFV.vars)
	}
}

// fvResult carries the free variable set and an overflow flag.
// When overflow is true, the FV computation was truncated by the depth
// limit; ancestor Lam/Thunk nodes must disable environment trimming
// rather than silently losing deep free variables.
type fvResult struct {
	vars     map[string]struct{}
	overflow bool
}

var fvOverflow = fvResult{overflow: true}

func (r fvResult) delete(name string) {
	delete(r.vars, name)
}

// traverseFVLeftSpine iteratively descends the left spine of App nodes,
// merging free variable sets from right children.
func traverseFVLeftSpine(app *App, depth int, obs fvObserver) fvResult {
	head, rights := unwindLeftSpine(app)
	result := traverseFV(head, depth+1, obs)
	for i := len(rights) - 1; i >= 0; i-- {
		result = mergeFV(result, traverseFV(rights[i], depth+1, obs))
	}
	return result
}

// traverseFV computes free variables bottom-up, notifying the observer at
// each annotation point (Lam, Thunk, Merge).
// Unlike FreeVars/freeVarsRec, this does NOT propagate Lam params into bound —
// outer Lam params are free from an inner closure's perspective (they are captured).
// Only Fix names, Case alt bindings, and Bind vars are propagated as bound,
// since they are resolved within the same scope.
//
// When the depth limit is reached, returns fvOverflow so that ancestor
// Lam/Thunk nodes detect the truncation and disable environment trimming
// rather than silently losing deep free variables.
func traverseFV(c Core, depth int, obs fvObserver) fvResult {
	if depth > maxTraversalDepth {
		return fvOverflow
	}
	switch n := c.(type) {
	case *Var:
		// Pre-compute the environment lookup key to avoid string concat at eval time.
		if n.Key == "" {
			n.Key = varKey(n)
		}
		return fvResult{vars: map[string]struct{}{n.Key: {}}}
	case *Lam:
		bodyFV := traverseFV(n.Body, depth+1, obs)
		// Remove the param — it comes from application, not from captured env.
		bodyFV.delete(n.Param)
		if obs != nil {
			obs.OnLam(n, bodyFV)
		}
		if bodyFV.overflow {
			return fvOverflow
		}
		return bodyFV
	case *App:
		return traverseFVLeftSpine(n, depth, obs)
	case *TyApp:
		return traverseFV(n.Expr, depth+1, obs)
	case *TyLam:
		return traverseFV(n.Body, depth+1, obs)
	case *Con:
		var result fvResult
		for _, arg := range n.Args {
			result = mergeFV(result, traverseFV(arg, depth+1, obs))
		}
		return result
	case *Case:
		result := traverseFV(n.Scrutinee, depth+1, obs)
		for _, alt := range n.Alts {
			altFV := traverseFV(alt.Body, depth+1, obs)
			// Remove pattern-bound vars — they are local to each alt.
			for _, name := range alt.Pattern.Bindings() {
				altFV.delete(name)
			}
			result = mergeFV(result, altFV)
		}
		return result
	case *Fix:
		// Fix name is visible in Body — remove it from the result.
		result := traverseFV(n.Body, depth+1, obs)
		result.delete(n.Name)
		return result
	case *Pure:
		return traverseFV(n.Expr, depth+1, obs)
	case *Bind:
		compFV := traverseFV(n.Comp, depth+1, obs)
		bodyFV := traverseFV(n.Body, depth+1, obs)
		// Bind var is local to the body.
		bodyFV.delete(n.Var)
		return mergeFV(compFV, bodyFV)
	case *Thunk:
		compFV := traverseFV(n.Comp, depth+1, obs)
		if obs != nil {
			obs.OnThunk(n, compFV)
		}
		if compFV.overflow {
			return fvOverflow
		}
		return compFV
	case *Force:
		return traverseFV(n.Expr, depth+1, obs)
	case *Merge:
		leftFV := traverseFV(n.Left, depth+1, obs)
		rightFV := traverseFV(n.Right, depth+1, obs)
		if obs != nil {
			obs.OnMerge(n, leftFV, rightFV)
		}
		return mergeFV(leftFV, rightFV)
	case *PrimOp:
		var result fvResult
		for _, arg := range n.Args {
			result = mergeFV(result, traverseFV(arg, depth+1, obs))
		}
		return result
	case *Lit:
		return fvResult{}
	case *Error:
		return fvResult{}
	case *RecordLit:
		var result fvResult
		for _, f := range n.Fields {
			result = mergeFV(result, traverseFV(f.Value, depth+1, obs))
		}
		return result
	case *RecordProj:
		return traverseFV(n.Record, depth+1, obs)
	case *RecordUpdate:
		result := traverseFV(n.Record, depth+1, obs)
		for _, f := range n.Updates {
			result = mergeFV(result, traverseFV(f.Value, depth+1, obs))
		}
		return result
	default:
		panic(fmt.Sprintf("traverseFV: unhandled Core node %T", c))
	}
}

func mergeFV(a, b fvResult) fvResult {
	if a.overflow || b.overflow {
		return fvOverflow
	}
	if len(a.vars) == 0 {
		return b
	}
	if len(b.vars) == 0 {
		return a
	}
	for k := range b.vars {
		a.vars[k] = struct{}{}
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
	sort.Strings(result)
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
// Uses the pre-computed Key field when available.
func VarKey(v *Var) string {
	if v.Key != "" {
		return v.Key
	}
	return varKey(v)
}

// QualifiedKey builds a qualified environment key from module and name.
// This is the canonical constructor for the "module\x00name" key format.
func QualifiedKey(module, name string) string {
	return module + "\x00" + name
}

// SplitQualifiedKey decomposes a qualified key into (module, name).
// For unqualified keys, module is "" and name is the key itself.
func SplitQualifiedKey(key string) (module, name string) {
	if before, after, ok := strings.Cut(key, "\x00"); ok {
		return before, after
	}
	return "", key
}
