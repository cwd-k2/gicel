// De Bruijn index assignment — converts named variable references to integer indices.
//
// This pass runs AFTER AnnotateFreeVars (which populates FV name lists)
// and BEFORE evaluation. The checker and optimizer are unaffected.
//
// Convention: index 0 = innermost (most recently bound) variable.
// The evaluator uses env.locals[len(locals)-1-index] for lookup.
//
// Variables NOT found in the local scope are global (Index = -1) and
// use Key-based lookup in the global map. This hybrid design avoids
// needing the global environment layout at compile time.

package ir

// AssignIndices assigns de Bruijn indices to a single Core expression.
// Variables not in any local scope get Index = -1 (global lookup).
func AssignIndices(c Core) {
	assignIndices(c, nil, 0)
}

// AssignIndicesProgram assigns de Bruijn indices to all bindings in a Program.
// Each top-level binding is global (no local scope), so all variable
// references within each binding are either to other globals (Index = -1)
// or to locally-bound variables (Index >= 0).
func AssignIndicesProgram(p *Program) {
	for i := range p.Bindings {
		assignIndices(p.Bindings[i].Expr, nil, 0)
	}
}

// EncodeGlobalSlot returns the Var.Index encoding for a global slot.
// Slot 0 → -2, slot 1 → -3, etc. Index -1 is reserved for unassigned globals.
func EncodeGlobalSlot(slot int) int { return -(slot + 2) }

// DecodeGlobalSlot extracts the slot number from a Var.Index.
// Precondition: IsGlobalIndex(idx).
func DecodeGlobalSlot(idx int) int { return -(idx + 2) }

// IsGlobalIndex returns true if the Var.Index encodes an assigned global slot.
func IsGlobalIndex(idx int) bool { return idx <= -2 }

// assignGlobalSlotsSingle converts unassigned global variables (Index == -1) to
// array-indexed globals using the provided slot mapping. After this pass,
// global Var nodes have Index = -(slot+2): -2 → slot 0, -3 → slot 1, etc.
// Index -1 remains the sentinel for "unassigned global" (test/fallback path).
//
// This completes the de Bruijn unification: all variables (local and global)
// are resolved to integer indices. The evaluator dispatches:
//
//	Index >= 0  → locals[len-1-Index]   (de Bruijn)
//	Index <= -2 → globals[-(Index+2)]   (slot)
//	Index == -1 → named map fallback    (tests only)
func assignGlobalSlotsSingle(c Core, slots map[string]int) {
	assignGlobalSlots(c, slots, 0)
}

// AssignGlobalSlotsProgram assigns global slots to all bindings in a Program.
func AssignGlobalSlotsProgram(p *Program, slots map[string]int) {
	for i := range p.Bindings {
		assignGlobalSlots(p.Bindings[i].Expr, slots, 0)
	}
}

func assignGlobalSlots(c Core, slots map[string]int, depth int) {
	if depth > maxTraversalDepth {
		// For left-spine App chains (e.g., long operator chains),
		// use iterative descent to avoid hitting the depth limit.
		if _, ok := c.(*App); ok {
			assignGlobalSlotsLeftSpine(c, slots)
			return
		}
		return
	}
	switch n := c.(type) {
	case *Var:
		if n.Index == -1 {
			key := n.Key
			if key == "" {
				key = VarKey(n)
			}
			if slot, ok := slots[key]; ok {
				n.Index = EncodeGlobalSlot(slot)
			}
		}
	case *Lam:
		assignGlobalSlots(n.Body, slots, depth+1)
	case *App:
		assignGlobalSlotsLeftSpine(c, slots)
		return
	case *TyApp:
		assignGlobalSlots(n.Expr, slots, depth+1)
	case *TyLam:
		assignGlobalSlots(n.Body, slots, depth+1)
	case *Con:
		for _, arg := range n.Args {
			assignGlobalSlots(arg, slots, depth+1)
		}
	case *Case:
		assignGlobalSlots(n.Scrutinee, slots, depth+1)
		for _, alt := range n.Alts {
			assignGlobalSlots(alt.Body, slots, depth+1)
		}
	case *Fix:
		assignGlobalSlots(n.Body, slots, depth+1)
	case *Pure:
		assignGlobalSlots(n.Expr, slots, depth+1)
	case *Bind:
		assignGlobalSlots(n.Comp, slots, depth+1)
		assignGlobalSlots(n.Body, slots, depth+1)
	case *Thunk:
		assignGlobalSlots(n.Comp, slots, depth+1)
	case *Force:
		assignGlobalSlots(n.Expr, slots, depth+1)
	case *PrimOp:
		for _, arg := range n.Args {
			assignGlobalSlots(arg, slots, depth+1)
		}
	case *Lit:
		// leaf
	case *RecordLit:
		for _, f := range n.Fields {
			assignGlobalSlots(f.Value, slots, depth+1)
		}
	case *RecordProj:
		assignGlobalSlots(n.Record, slots, depth+1)
	case *RecordUpdate:
		assignGlobalSlots(n.Record, slots, depth+1)
		for _, f := range n.Updates {
			assignGlobalSlots(f.Value, slots, depth+1)
		}
	}
}

// assignGlobalSlotsLeftSpine iteratively descends the left spine of App nodes
// (and TyApp/TyLam wrappers), assigning global slots to right children.
func assignGlobalSlotsLeftSpine(c Core, slots map[string]int) {
	var rights []Core
	cur := c
	for {
		switch n := cur.(type) {
		case *App:
			rights = append(rights, n.Arg)
			cur = n.Fun
			continue
		case *TyApp:
			cur = n.Expr
			continue
		case *TyLam:
			cur = n.Body
			continue
		default:
			assignGlobalSlots(n, slots, 0)
		}
		break
	}
	for i := len(rights) - 1; i >= 0; i-- {
		assignGlobalSlots(rights[i], slots, 0)
	}
}

// assignIndicesLeftSpine iteratively descends the left spine of App nodes,
// processing right children recursively. Prevents Go stack overflow on
// deeply left-nested operator chains (e.g., 500-operator expressions).
func assignIndicesLeftSpine(c Core, localScope map[string]int) {
	var rights []Core
	cur := c
	for {
		switch n := cur.(type) {
		case *App:
			rights = append(rights, n.Arg)
			cur = n.Fun
			continue
		case *TyApp:
			cur = n.Expr
			continue
		case *TyLam:
			cur = n.Body
			continue
		default:
			assignIndices(n, localScope, 0)
		}
		break
	}
	for i := len(rights) - 1; i >= 0; i-- {
		assignIndices(rights[i], localScope, 0)
	}
}

// assignIndices walks the Core IR top-down, maintaining a local scope
// that maps variable keys to their de Bruijn indices.
// Variables found in localScope get Index >= 0 (local lookup).
// Variables NOT found get Index = -1 (global lookup by Key).
func assignIndices(c Core, localScope map[string]int, depth int) {
	if depth > maxTraversalDepth {
		if _, ok := c.(*App); ok {
			assignIndicesLeftSpine(c, localScope)
			return
		}
		return
	}
	switch n := c.(type) {
	case *Var:
		key := n.Key
		if key == "" {
			key = varKey(n)
			n.Key = key
		}
		if localScope != nil {
			if idx, ok := localScope[key]; ok {
				n.Index = idx
				return
			}
		}
		n.Index = -1 // global variable

	case *Lam:
		assignLam(n, localScope, depth, "", false)

	case *App:
		assignIndicesLeftSpine(c, localScope)
		return

	case *TyApp:
		assignIndices(n.Expr, localScope, depth+1)

	case *TyLam:
		assignIndices(n.Body, localScope, depth+1)

	case *Con:
		for _, arg := range n.Args {
			assignIndices(arg, localScope, depth+1)
		}

	case *Case:
		assignIndices(n.Scrutinee, localScope, depth+1)
		for _, alt := range n.Alts {
			bindings := alt.Pattern.Bindings()
			altScope := shiftScope(localScope, len(bindings))
			for i, name := range bindings {
				altScope[name] = len(bindings) - 1 - i
			}
			assignIndices(alt.Body, altScope, depth+1)
		}

	case *Fix:
		// Fix introduces a self-referential name. The inner Lam's body
		// sees the fix name at a known position (index 1, between captured
		// FVs and the param). FVIndices does NOT include the fix name;
		// evalFix adds it after Capture via Push.
		lam, ok := PeelTyLam(n.Body).(*Lam)
		if !ok {
			assignIndices(n.Body, localScope, depth+1)
			return
		}
		assignLam(lam, localScope, depth+1, n.Name, true)

	case *Pure:
		assignIndices(n.Expr, localScope, depth+1)

	case *Bind:
		assignIndices(n.Comp, localScope, depth+1)
		bodyScope := shiftScope(localScope, 1)
		bodyScope[n.Var] = 0
		assignIndices(n.Body, bodyScope, depth+1)

	case *Thunk:
		assignThunk(n, localScope, depth)

	case *Force:
		assignIndices(n.Expr, localScope, depth+1)

	case *PrimOp:
		for _, arg := range n.Args {
			assignIndices(arg, localScope, depth+1)
		}

	case *Lit:
		// No sub-expressions.

	case *RecordLit:
		for _, f := range n.Fields {
			assignIndices(f.Value, localScope, depth+1)
		}

	case *RecordProj:
		assignIndices(n.Record, localScope, depth+1)

	case *RecordUpdate:
		assignIndices(n.Record, localScope, depth+1)
		for _, f := range n.Updates {
			assignIndices(f.Value, localScope, depth+1)
		}
	}
}

// assignLam assigns indices for a Lam node, building the captured scope
// from its FV list. fixName is non-empty when the Lam is inside a Fix.
func assignLam(n *Lam, enclosingScope map[string]int, depth int, fixName string, isFix bool) {
	if n.FV == nil {
		// FV overflow — no trimming. Body sees all enclosing locals + param.
		bodyScope := shiftScope(enclosingScope, 1)
		bodyScope[n.Param] = 0
		if isFix {
			bodyScope = shiftScope(bodyScope, 1)
			bodyScope[fixName] = 0
			// param shifts to 1
		}
		assignIndices(n.Body, bodyScope, depth+1)
		n.FVIndices = nil // signal: capture entire env
		return
	}

	// Collect local FVs (skip globals and fix name).
	// FVIndices starts non-nil (empty) to distinguish "no local captures"
	// from "FV overflow" (FV == nil). The evaluator uses:
	//   FVIndices != nil → Capture(FVIndices)
	//   FVIndices == nil && FV != nil → CaptureAll (overflow)
	var localFVNames []string
	n.FVIndices = []int{}
	for _, name := range n.FV {
		if isFix && name == fixName {
			continue // fix name is added by evalFix, not captured
		}
		if enclosingScope == nil {
			continue // no local scope → all FVs are global
		}
		if idx, ok := enclosingScope[name]; ok {
			localFVNames = append(localFVNames, name)
			n.FVIndices = append(n.FVIndices, idx)
		}
		// else: global FV, always accessible via globals map
	}

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
	assignIndices(n.Body, capturedScope, depth+1)
}

// assignThunk assigns indices for a Thunk node (like Lam but no param).
func assignThunk(n *Thunk, enclosingScope map[string]int, depth int) {
	if n.FV == nil {
		// FV overflow — no trimming.
		assignIndices(n.Comp, enclosingScope, depth+1)
		n.FVIndices = nil
		return
	}

	var localFVNames []string
	n.FVIndices = []int{}
	for _, name := range n.FV {
		if enclosingScope == nil {
			continue
		}
		if idx, ok := enclosingScope[name]; ok {
			localFVNames = append(localFVNames, name)
			n.FVIndices = append(n.FVIndices, idx)
		}
	}

	// Captured env: [localFV0, localFV1, ...]
	// No param, so FV[i] is at index len(localFVNames)-1-i.
	capturedScope := make(map[string]int, len(localFVNames))
	for i, name := range localFVNames {
		capturedScope[name] = len(localFVNames) - 1 - i
	}
	assignIndices(n.Comp, capturedScope, depth+1)
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
