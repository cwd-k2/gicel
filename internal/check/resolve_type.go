package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

func (ch *Checker) resolveTypeExpr(texpr syntax.TypeExpr) types.Type {
	switch t := texpr.(type) {
	case *syntax.TyExprVar:
		return &types.TyVar{Name: t.Name, S: t.S}
	case *syntax.TyExprCon:
		if info, ok := ch.aliases[t.Name]; ok && len(info.params) == 0 {
			return info.body
		}
		// DataKinds: if the name is a promoted constructor, treat it as a TyCon
		// (it will be kind-checked later; for now it's just a name in type position).
		return &types.TyCon{Name: t.Name, S: t.S}
	case *syntax.TyExprApp:
		fun := ch.resolveTypeExpr(t.Fun)
		arg := ch.resolveTypeExpr(t.Arg)
		// Recognize Computation and Thunk constructor application.
		result := ch.tryExpandApp(fun, arg, t.S)
		if result != nil {
			return result
		}
		ch.checkTypeAppKind(fun, arg, t.S)
		return &types.TyApp{Fun: fun, Arg: arg, S: t.S}
	case *syntax.TyExprArrow:
		return &types.TyArrow{
			From: ch.resolveTypeExpr(t.From),
			To:   ch.resolveTypeExpr(t.To),
			S:    t.S,
		}
	case *syntax.TyExprForall:
		// Register kind variables (binders with Kind sort) before resolving the body,
		// so that kind variable references in inner kind annotations resolve correctly.
		var kindVarNames []string
		for _, b := range t.Binders {
			if _, ok := b.Kind.(*syntax.KindExprSort); ok {
				ch.kindVars[b.Name] = true
				kindVarNames = append(kindVarNames, b.Name)
			}
		}
		ty := ch.resolveTypeExpr(t.Body)
		for i := len(t.Binders) - 1; i >= 0; i-- {
			kind := ch.resolveKindExpr(t.Binders[i].Kind)
			ty = &types.TyForall{Var: t.Binders[i].Name, Kind: kind, Body: ty, S: t.S}
		}
		for _, name := range kindVarNames {
			delete(ch.kindVars, name)
		}
		return ty
	case *syntax.TyExprRow:
		fields := make([]types.RowField, len(t.Fields))
		seen := make(map[string]bool, len(t.Fields))
		for i, f := range t.Fields {
			if seen[f.Label] {
				ch.addCodedError(errs.ErrDuplicateLabel, f.S, fmt.Sprintf("duplicate label %q in record type", f.Label))
			}
			seen[f.Label] = true
			fields[i] = types.RowField{Label: f.Label, Type: ch.resolveTypeExpr(f.Type), S: f.S}
		}
		var tail types.Type
		if t.Tail != nil {
			tail = &types.TyVar{Name: t.Tail.Name, S: t.Tail.S}
		}
		// Skip normalization if duplicates were found (NormalizeRow panics on duplicates).
		if len(seen) < len(fields) {
			return &types.TyEvidenceRow{
				Entries: &types.CapabilityEntries{Fields: fields[:len(seen)]},
				Tail:    tail,
				S:       t.S,
			}
		}
		return &types.TyEvidenceRow{
			Entries: &types.CapabilityEntries{Fields: fields},
			Tail:    tail,
			S:       t.S,
		}
	case *syntax.TyExprQual:
		body := ch.resolveTypeExpr(t.Body)
		constraint := ch.resolveTypeExpr(t.Constraint)
		// Quantified constraint: (forall a. C1 a => C2 (f a)) => T
		if qc := ch.decomposeQuantifiedConstraint(constraint); qc != nil {
			entry := types.ConstraintEntry{
				ClassName:  qc.Head.ClassName,
				Args:       qc.Head.Args,
				Quantified: qc,
				S:          t.S,
			}
			return qualifyBody(entry, body, t.S)
		}
		// Simple constraint: C a => T
		head, args := types.UnwindApp(constraint)
		if con, ok := head.(*types.TyCon); ok {
			entry := types.ConstraintEntry{ClassName: con.Name, Args: args, S: t.S}
			return qualifyBody(entry, body, t.S)
		}
		ch.addCodedError(errs.ErrNoInstance, t.S, fmt.Sprintf("invalid constraint: %s", types.Pretty(constraint)))
		return body
	case *syntax.TyExprParen:
		return ch.resolveTypeExpr(t.Inner)
	default:
		return &types.TyError{}
	}
}

// qualifyBody prepends a constraint entry to a body type, folding into an
// existing TyEvidence if the body is already qualified.
func qualifyBody(entry types.ConstraintEntry, body types.Type, s span.Span) *types.TyEvidence {
	if ev, ok := body.(*types.TyEvidence); ok {
		old := ev.Constraints.ConEntries()
		entries := make([]types.ConstraintEntry, 0, 1+len(old))
		entries = append(entries, entry)
		entries = append(entries, old...)
		return &types.TyEvidence{
			Constraints: &types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: entries}},
			Body:        ev.Body,
			S:           s,
		}
	}
	return &types.TyEvidence{
		Constraints: &types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: []types.ConstraintEntry{entry}}},
		Body:        body,
		S:           s,
	}
}

// decomposeQuantifiedConstraint checks if a resolved type is a quantified constraint
// (forall vars. context => head) and decomposes it into a QuantifiedConstraint.
// Returns nil if the type is not a quantified constraint.
func (ch *Checker) decomposeQuantifiedConstraint(ty types.Type) *types.QuantifiedConstraint {
	// Peel forall binders.
	var vars []types.ForallBinder
	current := ty
	for {
		if f, ok := current.(*types.TyForall); ok {
			vars = append(vars, types.ForallBinder{Name: f.Var, Kind: f.Kind})
			current = f.Body
		} else {
			break
		}
	}
	if len(vars) == 0 {
		return nil // not a quantified constraint
	}
	// Extract evidence: must be TyEvidence with at least one constraint entry for the head.
	ev, ok := current.(*types.TyEvidence)
	if !ok {
		return nil // forall a. T without => is not a quantified constraint
	}
	conEntries := ev.Constraints.ConEntries()
	if len(conEntries) == 0 {
		return nil
	}
	// The body of the evidence is the head constraint (after the last =>).
	headTy := ev.Body
	headHead, headArgs := types.UnwindApp(headTy)
	headCon, ok := headHead.(*types.TyCon)
	if !ok {
		return nil // head is not a class constraint
	}
	head := types.ConstraintEntry{ClassName: headCon.Name, Args: headArgs}
	// All entries in the evidence are context (premise) constraints.
	return &types.QuantifiedConstraint{
		Vars:    vars,
		Context: conEntries,
		Head:    head,
	}
}

// tryExpandApp recognizes fully-saturated Computation and Thunk applications
// and produces the dedicated TyComp/TyThunk nodes, and expands type aliases.
func (ch *Checker) tryExpandApp(fun types.Type, arg types.Type, s span.Span) types.Type {
	// Computation pre post result: TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
	if app2, ok := fun.(*types.TyApp); ok {
		if app1, ok := app2.Fun.(*types.TyApp); ok {
			if con, ok := app1.Fun.(*types.TyCon); ok {
				switch con.Name {
				case "Computation":
					return &types.TyComp{Pre: app1.Arg, Post: app2.Arg, Result: arg, S: s}
				case "Thunk":
					return &types.TyThunk{Pre: app1.Arg, Post: app2.Arg, Result: arg, S: s}
				}
			}
		}
	}
	// General alias expansion: collect the TyApp spine and check if the
	// head is an alias with matching parameter count.
	result := &types.TyApp{Fun: fun, Arg: arg, S: s}
	head, args := types.UnwindApp(result)
	if con, ok := head.(*types.TyCon); ok {
		if info, ok := ch.aliases[con.Name]; ok && len(info.params) == len(args) {
			body := info.body
			for i, p := range info.params {
				body = types.Subst(body, p, args[i])
			}
			return body
		}
	}
	return nil
}

func (ch *Checker) resolveKindExpr(k syntax.KindExpr) types.Kind {
	if k == nil {
		return types.KType{}
	}
	switch ke := k.(type) {
	case *syntax.KindExprType:
		return types.KType{}
	case *syntax.KindExprRow:
		return types.KRow{}
	case *syntax.KindExprConstraint:
		return types.KConstraint{}
	case *syntax.KindExprArrow:
		return &types.KArrow{From: ch.resolveKindExpr(ke.From), To: ch.resolveKindExpr(ke.To)}
	case *syntax.KindExprName:
		if ch.kindVars[ke.Name] {
			return types.KVar{Name: ke.Name}
		}
		if pk, ok := ch.promotedKinds[ke.Name]; ok {
			return pk
		}
		return types.KType{}
	case *syntax.KindExprSort:
		return types.KSort{}
	default:
		return types.KType{}
	}
}

// checkTypeAppKind validates that a type application F A is kind-correct.
// Only checks when:
//   - F has an explicitly annotated parameter kind (not the default KType)
//   - A is a concrete type constructor (TyCon or TyApp) with a deterministic kind
//
// This avoids false positives from type variables whose kind isn't yet in context.
func (ch *Checker) checkTypeAppKind(fun, arg types.Type, s span.Span) {
	// Only check when arg has a deterministic kind (concrete TyCon, not TyVar).
	if !ch.hasDeterministicKind(arg) {
		return
	}
	funKind := ch.kindOfType(fun)
	if funKind == nil {
		return
	}
	funKind = ch.unifier.ZonkKind(funKind)
	ka, ok := funKind.(*types.KArrow)
	if !ok {
		return
	}
	// Skip if the parameter kind is the default KType (unannotated parameter).
	if _, isType := ka.From.(types.KType); isType {
		return
	}
	argKind := ch.kindOfType(arg)
	if argKind == nil {
		return
	}
	argKind = ch.unifier.ZonkKind(argKind)
	if _, isMeta := argKind.(*types.KMeta); isMeta {
		return
	}
	if err := ch.unifier.UnifyKinds(ka.From, argKind); err != nil {
		ch.addCodedError(errs.ErrKindMismatch, s,
			fmt.Sprintf("kind mismatch in type application: expected kind %s, got %s", ka.From, argKind))
	}
}

// hasDeterministicKind returns true if the type's kind is deterministic
// (i.e., derived from a registered type constructor, not a defaulted TyVar).
func (ch *Checker) hasDeterministicKind(ty types.Type) bool {
	switch t := ty.(type) {
	case *types.TyCon:
		_, inReg := ch.config.RegisteredTypes[t.Name]
		_, inProm := ch.promotedCons[t.Name]
		_, isAlias := ch.aliases[t.Name]
		return inReg || inProm || isAlias
	case *types.TyApp:
		// Recurse on the head to check if it's deterministic.
		head, _ := types.UnwindApp(ty)
		if head != ty {
			return ch.hasDeterministicKind(head)
		}
		return false
	case *types.TyMeta:
		return true
	case *types.TySkolem:
		return true
	default:
		return false
	}
}
