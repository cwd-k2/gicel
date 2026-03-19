package types

import (
	"fmt"
	"sync/atomic"
)

var freshCounter int64

func freshName(base string) string {
	n := atomic.AddInt64(&freshCounter, 1)
	return fmt.Sprintf("%s$%d", base, n)
}

// ResetFreshCounter resets the global fresh name counter to zero.
// Use in tests to ensure deterministic type variable naming.
func ResetFreshCounter() {
	atomic.StoreInt64(&freshCounter, 0)
}

// Subst applies a substitution [varName := replacement] throughout a type.
func Subst(t Type, varName string, replacement Type) Type {
	return substDepth(t, varName, replacement, 0)
}

func substDepth(t Type, varName string, replacement Type, depth int) Type {
	if depth > maxTraversalDepth {
		return t
	}
	switch ty := t.(type) {
	case *TyVar:
		if ty.Name == varName {
			return replacement
		}
		return ty

	case *TyCon:
		return ty

	case *TyApp:
		newFun := substDepth(ty.Fun, varName, replacement, depth+1)
		newArg := substDepth(ty.Arg, varName, replacement, depth+1)
		if newFun == ty.Fun && newArg == ty.Arg {
			return ty
		}
		return &TyApp{Fun: newFun, Arg: newArg, S: ty.S}

	case *TyArrow:
		newFrom := substDepth(ty.From, varName, replacement, depth+1)
		newTo := substDepth(ty.To, varName, replacement, depth+1)
		if newFrom == ty.From && newTo == ty.To {
			return ty
		}
		return &TyArrow{From: newFrom, To: newTo, S: ty.S}

	case *TyForall:
		if ty.Var == varName {
			return ty // shadowed
		}
		// Capture avoidance.
		if OccursIn(ty.Var, replacement) {
			fresh := freshName(ty.Var)
			body := substDepth(ty.Body, ty.Var, &TyVar{Name: fresh}, depth+1)
			body = substDepth(body, varName, replacement, depth+1)
			return &TyForall{Var: fresh, Kind: ty.Kind, Body: body, S: ty.S}
		}
		newBody := substDepth(ty.Body, varName, replacement, depth+1)
		if newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: ty.Kind, Body: newBody, S: ty.S}

	case *TyCBPV:
		newPre := substDepth(ty.Pre, varName, replacement, depth+1)
		newPost := substDepth(ty.Post, varName, replacement, depth+1)
		newResult := substDepth(ty.Result, varName, replacement, depth+1)
		if newPre == ty.Pre && newPost == ty.Post && newResult == ty.Result {
			return ty
		}
		return &TyCBPV{Tag: ty.Tag, Pre: newPre, Post: newPost, Result: newResult, S: ty.S}

	case *TyEvidence:
		newConstraints := substDepth(ty.Constraints, varName, replacement, depth+1)
		newBody := substDepth(ty.Body, varName, replacement, depth+1)
		if newConstraints == ty.Constraints && newBody == ty.Body {
			return ty
		}
		cr, ok := newConstraints.(*TyEvidenceRow)
		if !ok {
			// Subst produced a non-evidence-row; preserve original to avoid nil.
			return &TyEvidence{Constraints: ty.Constraints, Body: newBody, S: ty.S}
		}
		return &TyEvidence{Constraints: cr, Body: newBody, S: ty.S}

	case *TySkolem:
		return ty

	case *TyEvidenceRow:
		switch entries := ty.Entries.(type) {
		case *CapabilityEntries:
			changed := false
			fields := make([]RowField, len(entries.Fields))
			for i, f := range entries.Fields {
				newT := substDepth(f.Type, varName, replacement, depth+1)
				if newT != f.Type {
					changed = true
				}
				var newMult Type
				if f.Mult != nil {
					newMult = substDepth(f.Mult, varName, replacement, depth+1)
					if newMult != f.Mult {
						changed = true
					}
				}
				fields[i] = RowField{Label: f.Label, Type: newT, Mult: newMult, S: f.S}
			}
			var newTail Type
			if ty.Tail != nil {
				newTail = substDepth(ty.Tail, varName, replacement, depth+1)
				if newTail != ty.Tail {
					changed = true
				}
			}
			if !changed {
				return ty
			}
			return &TyEvidenceRow{Entries: &CapabilityEntries{Fields: fields}, Tail: newTail, S: ty.S}
		case *ConstraintEntries:
			changed := false
			ces := make([]ConstraintEntry, len(entries.Entries))
			for i, e := range entries.Entries {
				ces[i] = substConstraintEntry(e, varName, replacement, &changed, depth+1)
			}
			var newTail Type
			if ty.Tail != nil {
				newTail = substDepth(ty.Tail, varName, replacement, depth+1)
				if newTail != ty.Tail {
					changed = true
				}
			}
			if !changed {
				return ty
			}
			return &TyEvidenceRow{Entries: &ConstraintEntries{Entries: ces}, Tail: newTail, S: ty.S}
		default:
			return ty
		}

	case *TyFamilyApp:
		changed := false
		args := make([]Type, len(ty.Args))
		for i, a := range ty.Args {
			newA := substDepth(a, varName, replacement, depth+1)
			if newA != a {
				changed = true
			}
			args[i] = newA
		}
		if !changed {
			return ty
		}
		return &TyFamilyApp{Name: ty.Name, Args: args, Kind: ty.Kind, S: ty.S}

	case *TyMeta:
		return ty

	case *TyError:
		return ty

	default:
		return ty
	}
}

// SubstKindInType substitutes a kind variable throughout all kind annotations
// embedded in a type. Used when instantiating kind-polymorphic quantifiers
// (e.g., \ (k: Kind). ... where k appears in kind positions).
func SubstKindInType(t Type, varName string, replacement Kind) Type {
	return substKindInTypeDepth(t, varName, replacement, 0)
}

func substKindInTypeDepth(t Type, varName string, replacement Kind, depth int) Type {
	if depth > maxTraversalDepth {
		return t
	}
	switch ty := t.(type) {
	case *TyForall:
		newKind := KindSubst(ty.Kind, varName, replacement)
		newBody := substKindInTypeDepth(ty.Body, varName, replacement, depth+1)
		if newKind == ty.Kind && newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: newKind, Body: newBody, S: ty.S}
	case *TyApp:
		newFun := substKindInTypeDepth(ty.Fun, varName, replacement, depth+1)
		newArg := substKindInTypeDepth(ty.Arg, varName, replacement, depth+1)
		if newFun == ty.Fun && newArg == ty.Arg {
			return ty
		}
		return &TyApp{Fun: newFun, Arg: newArg, S: ty.S}
	case *TyArrow:
		newFrom := substKindInTypeDepth(ty.From, varName, replacement, depth+1)
		newTo := substKindInTypeDepth(ty.To, varName, replacement, depth+1)
		if newFrom == ty.From && newTo == ty.To {
			return ty
		}
		return &TyArrow{From: newFrom, To: newTo, S: ty.S}
	case *TyCBPV:
		newPre := substKindInTypeDepth(ty.Pre, varName, replacement, depth+1)
		newPost := substKindInTypeDepth(ty.Post, varName, replacement, depth+1)
		newResult := substKindInTypeDepth(ty.Result, varName, replacement, depth+1)
		if newPre == ty.Pre && newPost == ty.Post && newResult == ty.Result {
			return ty
		}
		return &TyCBPV{Tag: ty.Tag, Pre: newPre, Post: newPost, Result: newResult, S: ty.S}
	case *TyMeta:
		newKind := KindSubst(ty.Kind, varName, replacement)
		if newKind == ty.Kind {
			return ty
		}
		return &TyMeta{ID: ty.ID, Kind: newKind}
	case *TySkolem:
		newKind := KindSubst(ty.Kind, varName, replacement)
		if newKind == ty.Kind {
			return ty
		}
		return &TySkolem{ID: ty.ID, Name: ty.Name, Kind: newKind}
	case *TyFamilyApp:
		changed := false
		args := make([]Type, len(ty.Args))
		for i, a := range ty.Args {
			newA := substKindInTypeDepth(a, varName, replacement, depth+1)
			if newA != a {
				changed = true
			}
			args[i] = newA
		}
		if !changed {
			return ty
		}
		return &TyFamilyApp{Name: ty.Name, Args: args, Kind: KindSubst(ty.Kind, varName, replacement), S: ty.S}
	default:
		// TyVar, TyCon, TyEvidenceRow, TyEvidence,
		// TyError, TyLit — no embedded kind annotations to substitute.
		return ty
	}
}

// SubstMany applies multiple substitutions simultaneously.
// Unlike sequential Subst calls, this performs a single pass:
// all variables are replaced in one traversal, so substitution
// values do not interfere with each other.
func SubstMany(t Type, subs map[string]Type) Type {
	if len(subs) == 0 {
		return t
	}
	return substMany(t, subs, 0)
}

func substMany(t Type, subs map[string]Type, depth int) Type {
	if depth > maxTraversalDepth {
		return t
	}
	switch ty := t.(type) {
	case *TyVar:
		if repl, ok := subs[ty.Name]; ok {
			return repl
		}
		return ty
	case *TyCon:
		return ty
	case *TyApp:
		newFun := substMany(ty.Fun, subs, depth+1)
		newArg := substMany(ty.Arg, subs, depth+1)
		if newFun == ty.Fun && newArg == ty.Arg {
			return ty
		}
		return &TyApp{Fun: newFun, Arg: newArg, S: ty.S}
	case *TyArrow:
		newFrom := substMany(ty.From, subs, depth+1)
		newTo := substMany(ty.To, subs, depth+1)
		if newFrom == ty.From && newTo == ty.To {
			return ty
		}
		return &TyArrow{From: newFrom, To: newTo, S: ty.S}
	case *TyForall:
		// Remove shadowed variable from substitution.
		if _, shadowed := subs[ty.Var]; shadowed {
			reduced := make(map[string]Type, len(subs)-1)
			for k, v := range subs {
				if k != ty.Var {
					reduced[k] = v
				}
			}
			if len(reduced) == 0 {
				return ty
			}
			// Capture avoidance: check if any replacement contains the bound variable.
			for _, repl := range reduced {
				if OccursIn(ty.Var, repl) {
					fresh := freshName(ty.Var)
					body := substDepth(ty.Body, ty.Var, &TyVar{Name: fresh}, depth+1)
					body = substMany(body, reduced, depth+1)
					return &TyForall{Var: fresh, Kind: ty.Kind, Body: body, S: ty.S}
				}
			}
			newBody := substMany(ty.Body, reduced, depth+1)
			if newBody == ty.Body {
				return ty
			}
			return &TyForall{Var: ty.Var, Kind: ty.Kind, Body: newBody, S: ty.S}
		}
		// Not shadowed: check capture avoidance.
		for _, repl := range subs {
			if OccursIn(ty.Var, repl) {
				fresh := freshName(ty.Var)
				body := substDepth(ty.Body, ty.Var, &TyVar{Name: fresh}, depth+1)
				body = substMany(body, subs, depth+1)
				return &TyForall{Var: fresh, Kind: ty.Kind, Body: body, S: ty.S}
			}
		}
		newBody := substMany(ty.Body, subs, depth+1)
		if newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: ty.Kind, Body: newBody, S: ty.S}
	case *TyCBPV:
		newPre := substMany(ty.Pre, subs, depth+1)
		newPost := substMany(ty.Post, subs, depth+1)
		newResult := substMany(ty.Result, subs, depth+1)
		if newPre == ty.Pre && newPost == ty.Post && newResult == ty.Result {
			return ty
		}
		return &TyCBPV{Tag: ty.Tag, Pre: newPre, Post: newPost, Result: newResult, S: ty.S}
	case *TyEvidenceRow:
		return substManyEvidenceRow(ty, subs, depth+1)
	case *TyEvidence:
		newConstraints := substManyEvidenceRow(ty.Constraints, subs, depth+1)
		newBody := substMany(ty.Body, subs, depth+1)
		if newConstraints == ty.Constraints && newBody == ty.Body {
			return ty
		}
		return &TyEvidence{Constraints: newConstraints, Body: newBody, S: ty.S}
	case *TyFamilyApp:
		changed := false
		newArgs := make([]Type, len(ty.Args))
		for i, a := range ty.Args {
			newArgs[i] = substMany(a, subs, depth+1)
			if newArgs[i] != a {
				changed = true
			}
		}
		if !changed {
			return ty
		}
		return &TyFamilyApp{Name: ty.Name, Args: newArgs, Kind: ty.Kind, S: ty.S}
	default:
		return ty
	}
}

func substManyEvidenceRow(row *TyEvidenceRow, subs map[string]Type, depth int) *TyEvidenceRow {
	if row == nil {
		return nil
	}
	if depth > maxTraversalDepth {
		return row
	}
	changed := false
	newEntries := row.Entries.MapChildren(func(child Type) Type {
		r := substMany(child, subs, depth+1)
		if r != child {
			changed = true
		}
		return r
	})
	var newTail Type
	if row.Tail != nil {
		newTail = substMany(row.Tail, subs, depth+1)
		if newTail != row.Tail {
			changed = true
		}
	}
	if !changed {
		return row
	}
	return &TyEvidenceRow{Entries: newEntries, Tail: newTail, S: row.S}
}

// substConstraintEntry substitutes within a single ConstraintEntry,
// handling the Quantified field with proper variable shadowing.
func substConstraintEntry(e ConstraintEntry, varName string, replacement Type, changed *bool, depth int) ConstraintEntry {
	if depth > maxTraversalDepth {
		return e
	}
	args := make([]Type, len(e.Args))
	for j, a := range e.Args {
		newA := substDepth(a, varName, replacement, depth+1)
		if newA != a {
			*changed = true
		}
		args[j] = newA
	}
	result := ConstraintEntry{ClassName: e.ClassName, Args: args, S: e.S}
	if e.ConstraintVar != nil {
		newCV := substDepth(e.ConstraintVar, varName, replacement, depth+1)
		if newCV != e.ConstraintVar {
			*changed = true
		}
		result.ConstraintVar = newCV
	}
	if e.Quantified != nil {
		// Check if varName is shadowed by any quantified variable.
		for _, v := range e.Quantified.Vars {
			if v.Name == varName {
				result.Quantified = e.Quantified // shadowed, no substitution inside
				return result
			}
		}
		newQC := substQuantifiedConstraint(e.Quantified, varName, replacement, changed, depth+1)
		result.Quantified = newQC
	}
	return result
}

func substQuantifiedConstraint(qc *QuantifiedConstraint, varName string, replacement Type, changed *bool, depth int) *QuantifiedConstraint {
	if depth > maxTraversalDepth {
		return qc
	}
	// Capture avoidance: check if any bound var appears free in replacement.
	vars := make([]ForallBinder, len(qc.Vars))
	copy(vars, qc.Vars)
	for i, v := range vars {
		if OccursIn(v.Name, replacement) {
			fresh := freshName(v.Name)
			// Rename this bound var in context and head.
			vars[i] = ForallBinder{Name: fresh, Kind: v.Kind}
			*changed = true
		}
	}
	ctx := make([]ConstraintEntry, len(qc.Context))
	for i, c := range qc.Context {
		ctx[i] = substConstraintEntry(c, varName, replacement, changed, depth+1)
	}
	head := substConstraintEntry(qc.Head, varName, replacement, changed, depth+1)
	// Also apply renames from capture avoidance.
	for i, orig := range qc.Vars {
		if vars[i].Name != orig.Name {
			for j := range ctx {
				ctx[j] = renameInConstraintEntry(ctx[j], orig.Name, vars[i].Name, depth+1)
			}
			head = renameInConstraintEntry(head, orig.Name, vars[i].Name, depth+1)
		}
	}
	return &QuantifiedConstraint{Vars: vars, Context: ctx, Head: head}
}

func renameInConstraintEntry(e ConstraintEntry, oldName, newName string, depth int) ConstraintEntry {
	if depth > maxTraversalDepth {
		return e
	}
	replacement := &TyVar{Name: newName}
	changed := false
	args := make([]Type, len(e.Args))
	for j, a := range e.Args {
		args[j] = substDepth(a, oldName, replacement, depth+1)
		if args[j] != a {
			changed = true
		}
	}
	result := ConstraintEntry{ClassName: e.ClassName, Args: args, S: e.S}
	if e.ConstraintVar != nil {
		newCV := substDepth(e.ConstraintVar, oldName, replacement, depth+1)
		if newCV != e.ConstraintVar {
			changed = true
		}
		result.ConstraintVar = newCV
	}
	if e.Quantified != nil {
		// Check if oldName is shadowed by a quantified variable.
		shadowed := false
		for _, v := range e.Quantified.Vars {
			if v.Name == oldName {
				shadowed = true
				break
			}
		}
		if !shadowed {
			ctx := make([]ConstraintEntry, len(e.Quantified.Context))
			for i, c := range e.Quantified.Context {
				ctx[i] = renameInConstraintEntry(c, oldName, newName, depth+1)
			}
			head := renameInConstraintEntry(e.Quantified.Head, oldName, newName, depth+1)
			result.Quantified = &QuantifiedConstraint{Vars: e.Quantified.Vars, Context: ctx, Head: head}
			changed = true
		} else {
			result.Quantified = e.Quantified
		}
	}
	if !changed {
		return e
	}
	return result
}
