package ir

import "fmt"

// Walk visits every Core node in depth-first order.
// The visitor returns false to stop traversal.
// Walk returns true if the full tree was visited, false if the traversal
// was truncated by the depth limit. Callers that depend on completeness
// (e.g. node counting for size guards) must check the return value.
func Walk(c Core, visit func(Core) bool) bool {
	return walkRec(c, visit, 0)
}

func walkRec(c Core, visit func(Core) bool, depth int) bool {
	if depth > maxTraversalDepth {
		return false
	}
	if !visit(c) {
		return true // visitor stopped — not a truncation
	}
	switch n := c.(type) {
	case *Var:
		// leaf
	case *Lam:
		if !walkRec(n.Body, visit, depth+1) {
			return false
		}
	case *App:
		if !walkLeftSpine(n, visit, depth, true) {
			return false
		}
	case *TyApp:
		if !walkRec(n.Expr, visit, depth+1) {
			return false
		}
	case *TyLam:
		if !walkRec(n.Body, visit, depth+1) {
			return false
		}
	case *Con:
		for _, arg := range n.Args {
			if !walkRec(arg, visit, depth+1) {
				return false
			}
		}
	case *Case:
		if !walkRec(n.Scrutinee, visit, depth+1) {
			return false
		}
		for _, alt := range n.Alts {
			if !walkRec(alt.Body, visit, depth+1) {
				return false
			}
		}
	case *Fix:
		if !walkRec(n.Body, visit, depth+1) {
			return false
		}
	case *Pure:
		if !walkRec(n.Expr, visit, depth+1) {
			return false
		}
	case *Bind:
		if !walkRec(n.Comp, visit, depth+1) {
			return false
		}
		if !walkRec(n.Body, visit, depth+1) {
			return false
		}
	case *Thunk:
		if !walkRec(n.Comp, visit, depth+1) {
			return false
		}
	case *Force:
		if !walkRec(n.Expr, visit, depth+1) {
			return false
		}
	case *Merge:
		if !walkRec(n.Left, visit, depth+1) {
			return false
		}
		if !walkRec(n.Right, visit, depth+1) {
			return false
		}
	case *PrimOp:
		for _, arg := range n.Args {
			if !walkRec(arg, visit, depth+1) {
				return false
			}
		}
	case *Lit:
		// leaf
	case *Error:
		// leaf
	case *RecordLit:
		for _, f := range n.Fields {
			if !walkRec(f.Value, visit, depth+1) {
				return false
			}
		}
	case *RecordProj:
		if !walkRec(n.Record, visit, depth+1) {
			return false
		}
	case *RecordUpdate:
		if !walkRec(n.Record, visit, depth+1) {
			return false
		}
		for _, f := range n.Updates {
			if !walkRec(f.Value, visit, depth+1) {
				return false
			}
		}
	case *VariantLit:
		if !walkRec(n.Value, visit, depth+1) {
			return false
		}
	default:
		panic(fmt.Sprintf("Walk: unhandled Core node %T", c))
	}
	return true
}

// walkLeftSpine iteratively descends the left spine of App nodes (and
// transparent wrappers TyApp/TyLam), visiting right-side children
// recursively. This prevents Go stack overflow on deeply left-nested
// operator chains while preserving the depth budget for non-spine nodes.
func walkLeftSpine(app *App, visit func(Core) bool, depth int, skipRoot bool) bool {
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
					return true // visitor stopped — not truncation
				}
			}
			first = false
			rights = append(rights, rightChild{n.Arg, depth + 1})
			cur = n.Fun
			continue
		case *TyApp:
			if !visit(n) {
				return true
			}
			cur = n.Expr
			continue
		case *TyLam:
			if !visit(n) {
				return true
			}
			cur = n.Body
			continue
		default:
			if !walkRec(n, visit, depth+1) {
				return false
			}
		}
		break
	}

	for i := len(rights) - 1; i >= 0; i-- {
		if !walkRec(rights[i].node, visit, rights[i].depth) {
			return false
		}
	}
	return true
}

// Clone creates a deep copy of a Core tree. All nodes including Var and Lit
// are freshly allocated, so the clone shares no mutable state with the original.
// This is necessary when the same subtree is inserted into multiple positions
// in the IR (e.g., by substitution), because AssignIndices mutates Var.Index
// and Lam.FVIndices in place.
//
// Var.Index is reset to -1 (unassigned) in the clone. The caller must run
// AssignIndices on the cloned subtree after insertion into its new scope.
func Clone(c Core) Core {
	return Transform(c, func(n Core) Core {
		switch v := n.(type) {
		case *Var:
			return &Var{Name: v.Name, Module: v.Module, Index: -1, S: v.S}
		case *Lit:
			return &Lit{Value: v.Value, S: v.S}
		default:
			return n
		}
	})
}

func Transform(c Core, f func(Core) Core) Core {
	return transformRec(c, f, 0)
}

func transformRec(c Core, f func(Core) Core, depth int) Core {
	if depth > maxTraversalDepth {
		switch c.(type) {
		case *App:
			return transformLeftSpine(c, f)
		case *Bind:
			return transformBindChain(c, f)
		}
		return c
	}
	switch n := c.(type) {
	case *Var:
		return f(n)
	case *Lam:
		newBody := transformRec(n.Body, f, depth+1)
		if newBody == n.Body {
			return f(n)
		}
		return f(&Lam{Param: n.Param, ParamType: n.ParamType, Body: newBody, Generated: n.Generated, S: n.S})
	case *App:
		return transformLeftSpine(c, f)
	case *TyApp:
		newExpr := transformRec(n.Expr, f, depth+1)
		if newExpr == n.Expr {
			return f(n)
		}
		return f(&TyApp{Expr: newExpr, TyArg: n.TyArg, S: n.S})
	case *TyLam:
		newBody := transformRec(n.Body, f, depth+1)
		if newBody == n.Body {
			return f(n)
		}
		return f(&TyLam{TyParam: n.TyParam, Kind: n.Kind, Body: newBody, S: n.S})
	case *Con:
		args, changed := transformSlice(n.Args, f, depth)
		if !changed {
			return f(n)
		}
		return f(&Con{Name: n.Name, Module: n.Module, Args: args, S: n.S})
	case *Case:
		newScrutinee := transformRec(n.Scrutinee, f, depth+1)
		changed := newScrutinee != n.Scrutinee
		var alts []Alt
		for i, alt := range n.Alts {
			newBody := transformRec(alt.Body, f, depth+1)
			if newBody != alt.Body {
				if alts == nil {
					alts = make([]Alt, len(n.Alts))
					copy(alts[:i], n.Alts[:i])
				}
				alts[i] = Alt{Pattern: alt.Pattern, Body: newBody, Generated: alt.Generated, S: alt.S}
				changed = true
			} else if alts != nil {
				alts[i] = alt
			}
		}
		if !changed {
			return f(n)
		}
		if alts == nil {
			alts = n.Alts
		}
		return f(&Case{Scrutinee: newScrutinee, Alts: alts, S: n.S})
	case *Fix:
		newBody := transformRec(n.Body, f, depth+1)
		if newBody == n.Body {
			return f(n)
		}
		return f(&Fix{Name: n.Name, Body: newBody, S: n.S})
	case *Pure:
		newExpr := transformRec(n.Expr, f, depth+1)
		if newExpr == n.Expr {
			return f(n)
		}
		return f(&Pure{Expr: newExpr, S: n.S})
	case *Bind:
		return transformBindChain(c, f)
	case *Thunk:
		newComp := transformRec(n.Comp, f, depth+1)
		if newComp == n.Comp {
			return f(n)
		}
		return f(&Thunk{Comp: newComp, S: n.S})
	case *Force:
		newExpr := transformRec(n.Expr, f, depth+1)
		if newExpr == n.Expr {
			return f(n)
		}
		return f(&Force{Expr: newExpr, S: n.S})
	case *Merge:
		newLeft := transformRec(n.Left, f, depth+1)
		newRight := transformRec(n.Right, f, depth+1)
		if newLeft == n.Left && newRight == n.Right {
			return f(n)
		}
		return f(&Merge{Left: newLeft, Right: newRight, LeftLabels: n.LeftLabels, RightLabels: n.RightLabels, S: n.S})
	case *PrimOp:
		args, changed := transformSlice(n.Args, f, depth)
		if !changed {
			return f(n)
		}
		return f(&PrimOp{Name: n.Name, Arity: n.Arity, IsEffectful: n.IsEffectful, Args: args, S: n.S})
	case *Lit:
		return f(n)
	case *Error:
		return f(n)
	case *RecordLit:
		fields, changed := transformFields(n.Fields, f, depth)
		if !changed {
			return f(n)
		}
		return f(&RecordLit{Fields: fields, S: n.S})
	case *RecordProj:
		newRecord := transformRec(n.Record, f, depth+1)
		if newRecord == n.Record {
			return f(n)
		}
		return f(&RecordProj{Record: newRecord, Label: n.Label, S: n.S})
	case *RecordUpdate:
		newRecord := transformRec(n.Record, f, depth+1)
		updates, updChanged := transformFields(n.Updates, f, depth)
		if newRecord == n.Record && !updChanged {
			return f(n)
		}
		if !updChanged {
			updates = n.Updates
		}
		return f(&RecordUpdate{Record: newRecord, Updates: updates, S: n.S})
	case *VariantLit:
		newValue := transformRec(n.Value, f, depth+1)
		if newValue == n.Value {
			return f(n)
		}
		return f(&VariantLit{Tag: n.Tag, Value: newValue, S: n.S})
	default:
		panic(fmt.Sprintf("Transform: unhandled Core node %T", c))
	}
}

// transformSlice transforms a []Core with lazy-init: returns (original, false) if unchanged.
func transformSlice(elems []Core, f func(Core) Core, depth int) ([]Core, bool) {
	var result []Core // nil until first change
	for i, e := range elems {
		newE := transformRec(e, f, depth+1)
		if result == nil && newE != e {
			result = make([]Core, len(elems))
			copy(result[:i], elems[:i])
		}
		if result != nil {
			result[i] = newE
		}
	}
	if result == nil {
		return elems, false
	}
	return result, true
}

// transformFields transforms a []Field with lazy-init: returns (original, false) if unchanged.
func transformFields(fields []Field, f func(Core) Core, depth int) ([]Field, bool) {
	var result []Field // nil until first change
	for i, fld := range fields {
		newVal := transformRec(fld.Value, f, depth+1)
		if result == nil && newVal != fld.Value {
			result = make([]Field, len(fields))
			copy(result[:i], fields[:i])
		}
		if result != nil {
			result[i] = Field{Label: fld.Label, Value: newVal}
		}
	}
	if result == nil {
		return fields, false
	}
	return result, true
}

// transformLeftSpine iteratively processes the left spine of App nodes
// (including TyApp/TyLam wrappers), applying f bottom-up. This allows
// Transform to handle arbitrarily deep left-associative operator chains
// without exceeding the Go call stack.
//
// Right children branching off the spine are structurally shallow (arguments
// to each application). Their transform depth resets to 0, matching the
// convention used by assignIndicesLeftSpine. Without this reset, operator
// chains exceeding maxTraversalDepth cause right-child Var nodes (e.g.,
// dictionary placeholders) to be returned untransformed, because transformRec
// bails on non-App nodes past the limit.
func transformLeftSpine(c Core, f func(Core) Core) Core {
	type spineNode struct {
		app    *App // original App node
		arg    Core // transformed right child
		argChg bool // true if arg differs from original
	}
	var spine []spineNode

	// Phase 1: unwind the left spine, transforming right children.
	cur := c
	headChanged := false
	for {
		switch n := cur.(type) {
		case *App:
			newArg := transformRec(n.Arg, f, 0)
			spine = append(spine, spineNode{app: n, arg: newArg, argChg: newArg != n.Arg})
			cur = n.Fun
			continue
		case *TyApp:
			inner := transformLeftSpineOrRec(n.Expr, f)
			if inner == n.Expr {
				cur = f(n)
			} else {
				cur = f(&TyApp{Expr: inner, TyArg: n.TyArg, S: n.S})
			}
			headChanged = cur != n
			goto rebuild
		case *TyLam:
			inner := transformLeftSpineOrRec(n.Body, f)
			if inner == n.Body {
				cur = f(n)
			} else {
				cur = f(&TyLam{TyParam: n.TyParam, Kind: n.Kind, Body: inner, S: n.S})
			}
			headChanged = cur != n
			goto rebuild
		default:
			cur = transformRec(n, f, 0)
			headChanged = cur != n
			goto rebuild
		}
	}

rebuild:
	// Phase 2: rebuild the spine from the root outward.
	// Check if anything changed; if not, pass original App nodes to f.
	anyChange := headChanged
	if !anyChange {
		for _, sn := range spine {
			if sn.argChg {
				anyChange = true
				break
			}
		}
	}
	if !anyChange {
		// No structural change — pass original nodes through f.
		for i := len(spine) - 1; i >= 0; i-- {
			r := f(spine[i].app)
			if r != spine[i].app {
				// f wants to rewrite this App; rebuild from here.
				cur = r
				for j := i - 1; j >= 0; j-- {
					cur = f(&App{Fun: cur, Arg: spine[j].arg, S: spine[j].app.S})
				}
				return cur
			}
		}
		return spine[0].app // f returned all originals
	}
	for i := len(spine) - 1; i >= 0; i-- {
		sn := spine[i]
		cur = f(&App{Fun: cur, Arg: sn.arg, S: sn.app.S})
	}
	return cur
}

// transformLeftSpineOrRec continues with left-spine processing if the
// node is an App, otherwise falls back to regular recursion.
func transformLeftSpineOrRec(c Core, f func(Core) Core) Core {
	if _, ok := c.(*App); ok {
		return transformLeftSpine(c, f)
	}
	return transformRec(c, f, 0)
}

// TransformMut applies f to every Core node bottom-up, mutating parent
// fields in place when a child changes. This avoids allocating new parent
// nodes for the rebuild cascade. New allocation occurs only when f itself
// returns a structurally different node.
//
// Contract: the input tree must be a tree (not a DAG) and exclusively
// owned by the caller. The tree is mutated in place.
//
// Used by the optimizer. Clone and SubstitutePlaceholders use the
// rebuild-semantics Transform instead.
func TransformMut(c Core, f func(Core) Core) Core {
	return transformMutRec(c, f, 0)
}

func transformMutRec(c Core, f func(Core) Core, depth int) Core {
	if depth > maxTraversalDepth {
		switch c.(type) {
		case *App:
			return transformMutLeftSpine(c, f)
		case *Bind:
			return transformMutBindChain(c, f)
		}
		return c
	}
	switch n := c.(type) {
	case *Var:
		return f(n)
	case *Lam:
		n.Body = transformMutRec(n.Body, f, depth+1)
		return f(n)
	case *App:
		return transformMutLeftSpine(c, f)
	case *TyApp:
		n.Expr = transformMutRec(n.Expr, f, depth+1)
		return f(n)
	case *TyLam:
		n.Body = transformMutRec(n.Body, f, depth+1)
		return f(n)
	case *Con:
		for i, a := range n.Args {
			n.Args[i] = transformMutRec(a, f, depth+1)
		}
		return f(n)
	case *Case:
		n.Scrutinee = transformMutRec(n.Scrutinee, f, depth+1)
		for i := range n.Alts {
			n.Alts[i].Body = transformMutRec(n.Alts[i].Body, f, depth+1)
		}
		return f(n)
	case *Fix:
		n.Body = transformMutRec(n.Body, f, depth+1)
		return f(n)
	case *Pure:
		n.Expr = transformMutRec(n.Expr, f, depth+1)
		return f(n)
	case *Bind:
		return transformMutBindChain(c, f)
	case *Thunk:
		n.Comp = transformMutRec(n.Comp, f, depth+1)
		return f(n)
	case *Force:
		n.Expr = transformMutRec(n.Expr, f, depth+1)
		return f(n)
	case *Merge:
		n.Left = transformMutRec(n.Left, f, depth+1)
		n.Right = transformMutRec(n.Right, f, depth+1)
		return f(n)
	case *PrimOp:
		for i, a := range n.Args {
			n.Args[i] = transformMutRec(a, f, depth+1)
		}
		return f(n)
	case *Lit:
		return f(n)
	case *Error:
		return f(n)
	case *RecordLit:
		for i := range n.Fields {
			n.Fields[i].Value = transformMutRec(n.Fields[i].Value, f, depth+1)
		}
		return f(n)
	case *RecordProj:
		n.Record = transformMutRec(n.Record, f, depth+1)
		return f(n)
	case *RecordUpdate:
		n.Record = transformMutRec(n.Record, f, depth+1)
		for i := range n.Updates {
			n.Updates[i].Value = transformMutRec(n.Updates[i].Value, f, depth+1)
		}
		return f(n)
	case *VariantLit:
		n.Value = transformMutRec(n.Value, f, depth+1)
		return f(n)
	default:
		panic(fmt.Sprintf("TransformMut: unhandled Core node %T", c))
	}
}

// transformMutLeftSpine processes left-spine App chains with in-place mutation.
func transformMutLeftSpine(c Core, f func(Core) Core) Core {
	type spineNode struct {
		app *App
		arg Core
	}
	var spine []spineNode

	// Phase 1: unwind the spine, mutating right children in place.
	cur := c
	for {
		switch n := cur.(type) {
		case *App:
			n.Arg = transformMutRec(n.Arg, f, 0)
			spine = append(spine, spineNode{app: n, arg: n.Arg})
			cur = n.Fun
			continue
		case *TyApp:
			n.Expr = transformMutLeftSpineOrRec(n.Expr, f)
			cur = f(n)
			goto rebuild
		case *TyLam:
			n.Body = transformMutLeftSpineOrRec(n.Body, f)
			cur = f(n)
			goto rebuild
		default:
			cur = transformMutRec(n, f, 0)
			goto rebuild
		}
	}

rebuild:
	// Phase 2: rebuild the spine with in-place mutation.
	for i := len(spine) - 1; i >= 0; i-- {
		sn := spine[i]
		sn.app.Fun = cur
		cur = f(sn.app)
	}
	return cur
}

func transformMutLeftSpineOrRec(c Core, f func(Core) Core) Core {
	if _, ok := c.(*App); ok {
		return transformMutLeftSpine(c, f)
	}
	return transformMutRec(c, f, 0)
}

// transformBindChain iteratively processes right-leaning Bind chains
// (e.g., do-block sequences: Bind{Comp, Body: Bind{Comp, Body: ...}}).
// This mirrors transformLeftSpine for App chains, preventing stack overflow
// on deeply nested do-blocks that exceed maxTraversalDepth.
func transformBindChain(c Core, f func(Core) Core) Core {
	type bindNode struct {
		orig    *Bind
		comp    Core
		compChg bool
	}
	var chain []bindNode

	// Phase 1: unwind the Bind chain, transforming Comp children.
	cur := c
	for {
		b, ok := cur.(*Bind)
		if !ok {
			break
		}
		newComp := transformRec(b.Comp, f, 0)
		chain = append(chain, bindNode{orig: b, comp: newComp, compChg: newComp != b.Comp})
		cur = b.Body
	}

	// Transform the tail (non-Bind).
	newTail := transformRec(cur, f, 0)
	tailChg := newTail != cur

	// Phase 2: rebuild from the tail outward.
	anyChange := tailChg
	if !anyChange {
		for _, bn := range chain {
			if bn.compChg {
				anyChange = true
				break
			}
		}
	}

	cur = newTail
	if !anyChange {
		// No structural change — pass original nodes through f without mutation.
		// Unlike the previous implementation, we do NOT mutate bn.orig.Body
		// to avoid a data race window when concurrent readers (e.g., LSP hover)
		// access the IR tree.
		for i := len(chain) - 1; i >= 0; i-- {
			bn := chain[i]
			r := f(bn.orig)
			if r != bn.orig {
				// f rewrote this node. Rebuild remaining elements.
				cur = r
				for j := i - 1; j >= 0; j-- {
					bnj := chain[j]
					cur = f(&Bind{Comp: bnj.comp, Var: bnj.orig.Var, IsDiscard: bnj.orig.IsDiscard,
						Body: cur, Generated: bnj.orig.Generated, S: bnj.orig.S})
				}
				return cur
			}
			cur = bn.orig
		}
		return cur
	}
	for i := len(chain) - 1; i >= 0; i-- {
		bn := chain[i]
		cur = f(&Bind{Comp: bn.comp, Var: bn.orig.Var, IsDiscard: bn.orig.IsDiscard,
			Body: cur, Generated: bn.orig.Generated, S: bn.orig.S})
	}
	return cur
}

// transformMutBindChain iteratively processes right-leaning Bind chains
// with in-place mutation.
func transformMutBindChain(c Core, f func(Core) Core) Core {
	type bindNode struct {
		bind *Bind
		comp Core
	}
	var chain []bindNode

	// Phase 1: unwind the Bind chain, mutating Comp children in place.
	cur := c
	for {
		b, ok := cur.(*Bind)
		if !ok {
			break
		}
		b.Comp = transformMutRec(b.Comp, f, 0)
		chain = append(chain, bindNode{bind: b, comp: b.Comp})
		cur = b.Body
	}

	// Transform the tail (non-Bind).
	cur = transformMutRec(cur, f, 0)

	// Phase 2: rebuild from the tail outward with in-place mutation.
	for i := len(chain) - 1; i >= 0; i-- {
		bn := chain[i]
		bn.bind.Body = cur
		cur = f(bn.bind)
	}
	return cur
}
