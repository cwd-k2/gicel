// De Bruijn index assignment — converts named variable references to integer indices.
//
// This pass runs AFTER AnnotateFreeVars (which computes the FV name lists)
// and BEFORE evaluation. The checker and optimizer are unaffected.
//
// Convention: index 0 = innermost (most recently bound) variable.
// The evaluator uses env.locals[len(locals)-1-index] for lookup.
//
// Variables NOT found in the local scope are global (Index = -1) and
// use Key-based lookup in the global map. This hybrid design avoids
// needing the global environment layout at compile time.
//
// FV metadata (names and per-closure index lists) is read from and written
// into a *FVAnnotations side table supplied by the caller. The IR nodes
// themselves carry no FV state, so assignIndices is phase-neutral with
// respect to the node structure.

package ir

import "fmt"

// AssignIndices assigns de Bruijn indices to a single Core expression,
// populating FVInfo.Indices in annots for every Lam/Thunk/Merge reached.
// annots must already have been populated by AnnotateFreeVars for the
// same tree.
func AssignIndices(c Core, annots *FVAnnotations) {
	assignIndices(c, nil, 0, annots)
}

// AssignIndicesProgram assigns de Bruijn indices to all bindings in a
// Program. Each top-level binding is global (no local scope), so variable
// references within each binding are either to other globals (Index = -1)
// or to locally-bound variables (Index >= 0).
func AssignIndicesProgram(p *Program, annots *FVAnnotations) {
	for i := range p.Bindings {
		assignIndices(p.Bindings[i].Expr, nil, 0, annots)
	}
}

// assignIndicesLeftSpine iteratively descends the left spine of App nodes,
// processing right children recursively. Prevents Go stack overflow on
// deeply left-nested operator chains (e.g., 500-operator expressions).
func assignIndicesLeftSpine(c Core, localScope map[string]int, annots *FVAnnotations) {
	head, rights := unwindLeftSpine(c)
	assignIndices(head, localScope, 0, annots)
	for i := len(rights) - 1; i >= 0; i-- {
		assignIndices(rights[i], localScope, 0, annots)
	}
}

// assignIndices walks the Core IR top-down, maintaining a local scope
// that maps variable keys to their de Bruijn indices.
// Variables found in localScope get Index >= 0 (local lookup).
// Variables NOT found get Index = -1 (global lookup by Key).
func assignIndices(c Core, localScope map[string]int, depth int, annots *FVAnnotations) {
	if depth > maxTraversalDepth {
		if _, ok := c.(*App); ok {
			assignIndicesLeftSpine(c, localScope, annots)
			return
		}
		return
	}
	switch n := c.(type) {
	case *Var:
		if n.Key == "" {
			n.Key = varKey(n)
		}
		sk := string(n.Key)
		if localScope != nil {
			if idx, ok := localScope[sk]; ok {
				n.Index = idx
				return
			}
		}
		n.Index = -1 // global variable

	case *Lam:
		assignLam(n, localScope, depth, "", false, annots)

	case *App:
		assignIndicesLeftSpine(c, localScope, annots)
		return

	case *TyApp:
		assignIndices(n.Expr, localScope, depth+1, annots)

	case *TyLam:
		assignIndices(n.Body, localScope, depth+1, annots)

	case *Con:
		for _, arg := range n.Args {
			assignIndices(arg, localScope, depth+1, annots)
		}

	case *Case:
		assignIndices(n.Scrutinee, localScope, depth+1, annots)
		for _, alt := range n.Alts {
			bindings := alt.Pattern.Bindings()
			altScope := shiftScope(localScope, len(bindings))
			for i, name := range bindings {
				altScope[name] = len(bindings) - 1 - i
			}
			assignIndices(alt.Body, altScope, depth+1, annots)
		}

	case *Fix:
		// Fix introduces a self-referential name. The inner body's
		// scope sees the fix name at a known position. FVInfo.Indices
		// does NOT include the fix name; evalFix adds it after Capture
		// via Push.
		body := PeelTyLam(n.Body)
		switch b := body.(type) {
		case *Lam:
			assignLam(b, localScope, depth+1, n.Name, true, annots)
		case *Thunk:
			assignFixThunk(b, localScope, depth+1, n.Name, annots)
		default:
			assignIndices(n.Body, localScope, depth+1, annots)
		}

	case *Pure:
		assignIndices(n.Expr, localScope, depth+1, annots)

	case *Bind:
		assignIndices(n.Comp, localScope, depth+1, annots)
		bodyScope := shiftScope(localScope, 1)
		bodyScope[n.Var] = 0
		// Iteratively handle sequential Bind chains: mutate bodyScope
		// in-place for subsequent Binds (we own it from shiftScope).
		// This reduces D allocations to 1 for a chain of D Binds.
		cur := n.Body
		d := depth + 1
		for b, ok := cur.(*Bind); ok; b, ok = cur.(*Bind) {
			assignIndices(b.Comp, bodyScope, d+1, annots)
			for k := range bodyScope {
				bodyScope[k]++
			}
			bodyScope[b.Var] = 0
			d++
			cur = b.Body
		}
		assignIndices(cur, bodyScope, d+1, annots)

	case *Thunk:
		assignThunk(n, localScope, depth, annots)

	case *Force:
		assignIndices(n.Expr, localScope, depth+1, annots)

	case *Merge:
		info := annots.LookupMerge(n)
		assignMergeChild(n.Left, &info.Left, localScope, depth, annots)
		assignMergeChild(n.Right, &info.Right, localScope, depth, annots)

	case *PrimOp:
		for _, arg := range n.Args {
			assignIndices(arg, localScope, depth+1, annots)
		}

	case *Lit:
		// No sub-expressions.

	case *Error:
		// No sub-expressions.

	case *RecordLit:
		for _, f := range n.Fields {
			assignIndices(f.Value, localScope, depth+1, annots)
		}

	case *RecordProj:
		assignIndices(n.Record, localScope, depth+1, annots)

	case *RecordUpdate:
		assignIndices(n.Record, localScope, depth+1, annots)
		for _, f := range n.Updates {
			assignIndices(f.Value, localScope, depth+1, annots)
		}

	case *VariantLit:
		assignIndices(n.Value, localScope, depth+1, annots)

	default:
		panic(fmt.Sprintf("assignIndices: unhandled Core node %T", c))
	}
}

// assignLam assigns indices for a Lam node, building the captured scope
// from its FV list. fixName is non-empty when the Lam is inside a Fix.
func assignLam(n *Lam, enclosingScope map[string]int, depth int, fixName string, isFix bool, annots *FVAnnotations) {
	info := annots.LookupLam(n)
	if info.Overflow {
		// FV overflow — no trimming. Body sees all enclosing locals + param.
		bodyScope := shiftScope(enclosingScope, 1)
		bodyScope[n.Param] = 0
		if isFix {
			bodyScope = shiftScope(bodyScope, 1)
			bodyScope[fixName] = 0
			// param shifts to 1
		}
		assignIndices(n.Body, bodyScope, depth+1, annots)
		info.Indices = nil // signal: capture entire env
		return
	}

	// Collect local FVs (skip globals and fix name).
	// Indices starts non-nil (empty) to distinguish "no local captures"
	// from "FV overflow" (info.Overflow == true). The evaluator uses:
	//   Indices != nil → Capture(Indices)
	//   Indices == nil && !Overflow → should not happen (invariant)
	//   Overflow → CaptureAll
	var localFVNames []string
	indices := []int{}
	for _, name := range info.Vars {
		if isFix && name == fixName {
			continue // fix name is added by evalFix, not captured
		}
		if enclosingScope == nil {
			continue // no local scope → all FVs are global
		}
		if idx, ok := enclosingScope[name]; ok {
			localFVNames = append(localFVNames, name)
			indices = append(indices, idx)
		}
		// else: global FV, always accessible via globals map
	}
	info.Indices = indices

	// Build captured scope for the body.
	// Runtime env layout after Capture + (Fix Push) + param Push:
	//   [localFV0, localFV1, ..., (fix_self,) param]
	// Indices count from innermost (param = 0).
	extra := 0
	if isFix {
		extra = 1
	}
	capturedScope := make(map[string]int, len(localFVNames)+extra+1)
	capturedScope[n.Param] = 0
	if isFix {
		capturedScope[fixName] = 1
	}
	for i, name := range localFVNames {
		capturedScope[name] = extra + 1 + len(localFVNames) - 1 - i
	}
	assignIndices(n.Body, capturedScope, depth+1, annots)
}

// assignFixThunk assigns indices for a Thunk node inside a Fix (self-referential thunk).
// Layout after CaptureLam(ExtraCapSelf) + Push(self):
//
//	[localFV0, localFV1, ..., self]
//
// self at index 0 (innermost), captures shifted by 1.
func assignFixThunk(n *Thunk, enclosingScope map[string]int, depth int, fixName string, annots *FVAnnotations) {
	info := annots.LookupThunk(n)
	if info.Overflow {
		// FV overflow — no trimming.
		scope := shiftScope(enclosingScope, 1)
		scope[fixName] = 0
		assignIndices(n.Comp, scope, depth+1, annots)
		info.Indices = nil
		return
	}

	var localFVNames []string
	indices := []int{}
	for _, name := range info.Vars {
		if name == fixName {
			continue // fix name is added by evalFix, not captured
		}
		if enclosingScope == nil {
			continue
		}
		if idx, ok := enclosingScope[name]; ok {
			localFVNames = append(localFVNames, name)
			indices = append(indices, idx)
		}
	}
	info.Indices = indices

	capturedScope := make(map[string]int, len(localFVNames)+1)
	capturedScope[fixName] = 0 // self-ref at index 0 after Push
	for i, name := range localFVNames {
		capturedScope[name] = 1 + len(localFVNames) - 1 - i
	}
	assignIndices(n.Comp, capturedScope, depth+1, annots)
}

// assignThunk assigns indices for a Thunk node (like Lam but no param).
func assignThunk(n *Thunk, enclosingScope map[string]int, depth int, annots *FVAnnotations) {
	info := annots.LookupThunk(n)
	if info.Overflow {
		// FV overflow — no trimming.
		assignIndices(n.Comp, enclosingScope, depth+1, annots)
		info.Indices = nil
		return
	}

	var localFVNames []string
	indices := []int{}
	for _, name := range info.Vars {
		if enclosingScope == nil {
			continue
		}
		if idx, ok := enclosingScope[name]; ok {
			localFVNames = append(localFVNames, name)
			indices = append(indices, idx)
		}
	}
	info.Indices = indices

	// Captured env: [localFV0, localFV1, ...]
	// No param, so FV[i] is at index len(localFVNames)-1-i.
	capturedScope := make(map[string]int, len(localFVNames))
	for i, name := range localFVNames {
		capturedScope[name] = len(localFVNames) - 1 - i
	}
	assignIndices(n.Comp, capturedScope, depth+1, annots)
}

// assignMergeChild assigns indices for one side of a Merge. info is the
// matching half (Left or Right) of the Merge's MergeFVInfo entry.
func assignMergeChild(body Core, info *FVInfo, enclosingScope map[string]int, depth int, annots *FVAnnotations) {
	if info.Overflow {
		assignIndices(body, enclosingScope, depth+1, annots)
		info.Indices = nil
		return
	}
	var localFVNames []string
	indices := []int{}
	for _, name := range info.Vars {
		if enclosingScope == nil {
			continue
		}
		if idx, ok := enclosingScope[name]; ok {
			localFVNames = append(localFVNames, name)
			indices = append(indices, idx)
		}
	}
	info.Indices = indices

	capturedScope := make(map[string]int, len(localFVNames))
	for i, name := range localFVNames {
		capturedScope[name] = len(localFVNames) - 1 - i
	}
	assignIndices(body, capturedScope, depth+1, annots)
}

// shiftScope returns a copy of scope with all indices incremented by n.
// Returns nil if scope is nil.
func shiftScope(scope map[string]int, n int) map[string]int {
	if scope == nil && n == 0 {
		return nil
	}
	shifted := make(map[string]int, len(scope)+1) // +1 for the caller to add
	for k, v := range scope {
		shifted[k] = v + n
	}
	return shifted
}

// PeelTyLam strips type abstractions from a Core node.
func PeelTyLam(c Core) Core {
	for {
		if tl, ok := c.(*TyLam); ok {
			c = tl.Body
			continue
		}
		return c
	}
}
