package types

// substConstraintEntry substitutes within a single ConstraintEntry,
// handling QuantifiedConstraint with proper variable shadowing.
// Returns the (possibly new) entry and whether anything changed.
// When the second return value is false, the first value is e itself.
func substConstraintEntry(e ConstraintEntry, varName string, replacement Type, depth int) (ConstraintEntry, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	switch e := e.(type) {
	case *ClassEntry:
		args, changed := substTypeSlice(e.Args, varName, replacement, depth+1)
		if !changed {
			return e, false
		}
		return &ClassEntry{ClassName: e.ClassName, Args: args, S: e.S}, true
	case *EqualityEntry:
		newLhs := substDepth(e.Lhs, varName, replacement, depth+1)
		newRhs := substDepth(e.Rhs, varName, replacement, depth+1)
		if newLhs == e.Lhs && newRhs == e.Rhs {
			return e, false
		}
		return &EqualityEntry{Lhs: newLhs, Rhs: newRhs, S: e.S}, true
	case *VarEntry:
		newVar := substDepth(e.Var, varName, replacement, depth+1)
		if newVar == e.Var {
			return e, false
		}
		return &VarEntry{Var: newVar, S: e.S}, true
	case *QuantifiedConstraint:
		// Check if varName is shadowed by any quantified variable.
		for _, v := range e.Vars {
			if v.Name == varName {
				return e, false // no substitution inside
			}
		}
		newQC, changed := substQuantifiedConstraint(e, varName, replacement, depth+1)
		if !changed {
			return e, false
		}
		return newQC, true
	}
	return e, false
}

// substManyConstraintEntry applies a parallel substitution to a single
// ConstraintEntry. Multi-variable counterpart of substConstraintEntry:
// same per-variant dispatch, children are traversed with substManyOpt,
// and QuantifiedConstraint is delegated to substManyQuantifiedConstraint
// which handles shadowing and capture avoidance uniformly.
func substManyConstraintEntry(e ConstraintEntry, subs map[string]Type, levelSubs map[string]LevelExpr, fvUnion *map[string]bool, depth int) (ConstraintEntry, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	switch e := e.(type) {
	case *ClassEntry:
		args, changed := substManyTypeSlice(e.Args, subs, levelSubs, fvUnion, depth+1)
		if !changed {
			return e, false
		}
		return &ClassEntry{ClassName: e.ClassName, Args: args, S: e.S}, true
	case *EqualityEntry:
		newLhs := substManyOpt(e.Lhs, subs, levelSubs, fvUnion, depth+1)
		newRhs := substManyOpt(e.Rhs, subs, levelSubs, fvUnion, depth+1)
		if newLhs == e.Lhs && newRhs == e.Rhs {
			return e, false
		}
		return &EqualityEntry{Lhs: newLhs, Rhs: newRhs, S: e.S}, true
	case *VarEntry:
		newVar := substManyOpt(e.Var, subs, levelSubs, fvUnion, depth+1)
		if newVar == e.Var {
			return e, false
		}
		return &VarEntry{Var: newVar, S: e.S}, true
	case *QuantifiedConstraint:
		newQC, changed := substManyQuantifiedConstraint(e, subs, levelSubs, fvUnion, depth+1)
		if !changed {
			return e, false
		}
		return newQC, true
	}
	return e, false
}

// substQuantifiedConstraint substitutes within a QuantifiedConstraint, handling
// capture avoidance by renaming bound variables that appear free in replacement.
// Returns the (possibly new) constraint and whether anything changed.
func substQuantifiedConstraint(qc *QuantifiedConstraint, varName string, replacement Type, depth int) (*QuantifiedConstraint, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	changed := false
	// Capture avoidance: rename bound vars that appear free in replacement
	// BEFORE substituting, so the rename does not corrupt the replacement.
	var vars []ForallBinder   // nil until first rename
	var ctx []ConstraintEntry // nil until first change in ctx OR first rename
	head := qc.Head

	for i, v := range qc.Vars {
		if OccursIn(v.Name, replacement) {
			fresh := freshName(v.Name)
			if vars == nil {
				vars = make([]ForallBinder, len(qc.Vars))
				copy(vars, qc.Vars)
			}
			vars[i] = ForallBinder{Name: fresh, Kind: v.Kind}
			changed = true
			// Rename the bound variable in context and head BEFORE substitution.
			if ctx == nil {
				ctx = make([]ConstraintEntry, len(qc.Context))
				copy(ctx, qc.Context)
			}
			for j := range ctx {
				ctx[j], _ = renameInConstraintEntry(ctx[j], v.Name, fresh, depth+1)
			}
			if head != nil {
				renamed, _ := renameClassEntry(head, v.Name, fresh, depth+1)
				head = renamed
			}
		}
	}

	// Step 2: Now substitute in context — the renamed body no longer shadows the replacement.
	for i, c := range qc.Context {
		var src ConstraintEntry
		if ctx != nil {
			src = ctx[i]
		} else {
			src = c
		}
		newC, cChanged := substConstraintEntry(src, varName, replacement, depth+1)
		if cChanged {
			if ctx == nil {
				ctx = make([]ConstraintEntry, len(qc.Context))
				copy(ctx, qc.Context)
			}
			ctx[i] = newC
			changed = true
		} else if ctx != nil {
			ctx[i] = src
		}
	}
	if head != nil {
		newArgs, headChanged := substTypeSlice(head.Args, varName, replacement, depth+1)
		if headChanged {
			head = &ClassEntry{ClassName: head.ClassName, Args: newArgs, S: head.S}
			changed = true
		}
	}

	if !changed {
		return qc, false
	}
	finalVars := qc.Vars
	if vars != nil {
		finalVars = vars
	}
	finalCtx := qc.Context
	if ctx != nil {
		finalCtx = ctx
	}
	return &QuantifiedConstraint{Vars: finalVars, Context: finalCtx, Head: head, S: qc.S}, true
}

// substManyQuantifiedConstraint applies a parallel substitution to a
// QuantifiedConstraint. Multi-variable counterpart of
// substQuantifiedConstraint — same rename-then-substitute protocol,
// generalized across multiple replacements.
//
// Shadowing: entries in subs / levelSubs whose key names a binder in
// qc.Vars are removed before descending into context and head. The
// reduced maps are allocated lazily — the common case (no shadowing)
// descends through without allocation.
//
// Capture avoidance: binders in qc.Vars that appear free in any
// replacement (as tracked by fvUnion) are alpha-renamed to fresh names
// before substitution. fvUnion is computed lazily from the ORIGINAL
// subs, matching substManyOpt's TyForall strategy; this is correct but
// slightly conservative (a shadowed entry's replacement may contribute
// to fvUnion and trigger an unnecessary rename) — the alternative would
// require recomputing fvUnion per QC, which is costlier than an
// occasional spurious rename.
func substManyQuantifiedConstraint(qc *QuantifiedConstraint, subs map[string]Type, levelSubs map[string]LevelExpr, fvUnion *map[string]bool, depth int) (*QuantifiedConstraint, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}

	// Shadowing: build reduced subs / levelSubs without any binder names.
	// Allocate new maps only when shadowing actually applies.
	reducedSubs := subs
	reducedLevelSubs := levelSubs
	typeShadowed := false
	levelShadowed := false
	for _, v := range qc.Vars {
		if !typeShadowed {
			if _, ok := subs[v.Name]; ok {
				typeShadowed = true
			}
		}
		if !levelShadowed {
			if _, ok := levelSubs[v.Name]; ok {
				levelShadowed = true
			}
		}
		if typeShadowed && levelShadowed {
			break
		}
	}
	if typeShadowed {
		reducedSubs = make(map[string]Type, len(subs))
		for k, rv := range subs {
			if !qcVarsContain(qc.Vars, k) {
				reducedSubs[k] = rv
			}
		}
	}
	if levelShadowed {
		reducedLevelSubs = make(map[string]LevelExpr, len(levelSubs))
		for k, rv := range levelSubs {
			if !qcVarsContain(qc.Vars, k) {
				reducedLevelSubs[k] = rv
			}
		}
	}

	// Nothing left to substitute after shadowing — the QC is untouched.
	if len(reducedSubs) == 0 && len(reducedLevelSubs) == 0 {
		return qc, false
	}

	// Capture avoidance: lazy-compute fvUnion from the ORIGINAL subs.
	// Matches substManyOpt's TyForall strategy (conservative but cheap).
	var fv map[string]bool
	if len(subs) > 0 {
		if *fvUnion == nil {
			*fvUnion = substManyFVUnion(subs)
		}
		fv = *fvUnion
	}

	changed := false
	var vars []ForallBinder   // nil until first rename
	var ctx []ConstraintEntry // nil until first change in ctx OR first rename
	head := qc.Head

	// Rename any binder that appears free in a replacement, propagating
	// the rename through context and head BEFORE substitution so that
	// the renamed body no longer shadows the replacement's free vars.
	for i, v := range qc.Vars {
		if !fv[v.Name] {
			continue
		}
		fresh := freshName(v.Name)
		if vars == nil {
			vars = make([]ForallBinder, len(qc.Vars))
			copy(vars, qc.Vars)
		}
		vars[i] = ForallBinder{Name: fresh, Kind: v.Kind}
		changed = true
		if ctx == nil {
			ctx = make([]ConstraintEntry, len(qc.Context))
			copy(ctx, qc.Context)
		}
		for j := range ctx {
			ctx[j], _ = renameInConstraintEntry(ctx[j], v.Name, fresh, depth+1)
		}
		if head != nil {
			renamed, _ := renameClassEntry(head, v.Name, fresh, depth+1)
			head = renamed
		}
	}

	// Substitute with reducedSubs / reducedLevelSubs into the (possibly
	// renamed) ctx and head. fvUnion is the outer cell — nested QCs
	// reached from inside will see the same cached union.
	for i, c := range qc.Context {
		var src ConstraintEntry
		if ctx != nil {
			src = ctx[i]
		} else {
			src = c
		}
		newC, cChanged := substManyConstraintEntry(src, reducedSubs, reducedLevelSubs, fvUnion, depth+1)
		if cChanged {
			if ctx == nil {
				ctx = make([]ConstraintEntry, len(qc.Context))
				copy(ctx, qc.Context)
			}
			ctx[i] = newC
			changed = true
		} else if ctx != nil {
			ctx[i] = src
		}
	}
	if head != nil {
		newArgs, headChanged := substManyTypeSlice(head.Args, reducedSubs, reducedLevelSubs, fvUnion, depth+1)
		if headChanged {
			head = &ClassEntry{ClassName: head.ClassName, Args: newArgs, S: head.S}
			changed = true
		}
	}

	if !changed {
		return qc, false
	}
	finalVars := qc.Vars
	if vars != nil {
		finalVars = vars
	}
	finalCtx := qc.Context
	if ctx != nil {
		finalCtx = ctx
	}
	return &QuantifiedConstraint{Vars: finalVars, Context: finalCtx, Head: head, S: qc.S}, true
}

// qcVarsContain reports whether any ForallBinder in vars is named name.
// Used by substManyQuantifiedConstraint to test shadowing during the
// reduced-subs construction pass.
func qcVarsContain(vars []ForallBinder, name string) bool {
	for _, v := range vars {
		if v.Name == name {
			return true
		}
	}
	return false
}

// renameClassEntry rewrites a ClassEntry by replacing free occurrences of
// oldName with a TyVar named newName. Returns (entry, changed).
func renameClassEntry(e *ClassEntry, oldName, newName string, depth int) (*ClassEntry, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	replacement := &TyVar{Name: newName}
	args, changed := substTypeSlice(e.Args, oldName, replacement, depth+1)
	if !changed {
		return e, false
	}
	return &ClassEntry{ClassName: e.ClassName, Args: args, S: e.S}, true
}

// renameInConstraintEntry replaces oldName with a TyVar named newName
// throughout a constraint entry. Returns (entry, changed). When changed
// is false, the returned entry is e itself.
func renameInConstraintEntry(e ConstraintEntry, oldName, newName string, depth int) (ConstraintEntry, bool) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	replacement := &TyVar{Name: newName}
	switch e := e.(type) {
	case *ClassEntry:
		return renameClassEntry(e, oldName, newName, depth)
	case *EqualityEntry:
		newLhs := substDepth(e.Lhs, oldName, replacement, depth+1)
		newRhs := substDepth(e.Rhs, oldName, replacement, depth+1)
		if newLhs == e.Lhs && newRhs == e.Rhs {
			return e, false
		}
		return &EqualityEntry{Lhs: newLhs, Rhs: newRhs, S: e.S}, true
	case *VarEntry:
		newVar := substDepth(e.Var, oldName, replacement, depth+1)
		if newVar == e.Var {
			return e, false
		}
		return &VarEntry{Var: newVar, S: e.S}, true
	case *QuantifiedConstraint:
		// Check if oldName is shadowed by a quantified variable.
		for _, v := range e.Vars {
			if v.Name == oldName {
				return e, false
			}
		}
		ctxChanged := false
		var ctx []ConstraintEntry // nil until first change
		for i, c := range e.Context {
			newC, cChanged := renameInConstraintEntry(c, oldName, newName, depth+1)
			if cChanged {
				if ctx == nil {
					ctx = make([]ConstraintEntry, len(e.Context))
					copy(ctx[:i], e.Context[:i])
				}
				ctx[i] = newC
				ctxChanged = true
			} else if ctx != nil {
				ctx[i] = c
			}
		}
		head := e.Head
		headChanged := false
		if head != nil {
			head, headChanged = renameClassEntry(head, oldName, newName, depth+1)
		}
		if !ctxChanged && !headChanged {
			return e, false
		}
		finalCtx := e.Context
		if ctx != nil {
			finalCtx = ctx
		}
		return &QuantifiedConstraint{Vars: e.Vars, Context: finalCtx, Head: head, S: e.S}, true
	}
	return e, false
}
