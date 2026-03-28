package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// isModuleDefinedType checks if a type name was defined by the module itself
// (via data declarations or class declarations), as opposed to being inherited
// from built-in types or open imports.
func isModuleDefinedType(exports *ModuleExports, name string) bool {
	if exports.OwnedTypeNames[name] {
		return true
	}
	// Classes are already checked separately, but class-defined types
	// (dict types) might appear in Types.
	if _, ok := exports.Classes[name]; ok {
		return true
	}
	return false
}

func (ch *Checker) resolveTypeExpr(texpr syntax.TypeExpr) types.Type {
	switch t := texpr.(type) {
	case *syntax.TyExprVar:
		return &types.TyVar{Name: t.Name, S: t.S}
	case *syntax.TyExprCon:
		if info, ok := ch.lookupAlias(t.Name); ok && len(info.Params) == 0 {
			return info.Body
		}
		// Zero-arity type family: immediate TyFamilyApp.
		if fam, ok := ch.lookupFamily(t.Name); ok && len(fam.Params) == 0 {
			return &types.TyFamilyApp{Name: t.Name, Args: nil, Kind: fam.ResultKind, S: t.S}
		}
		// Validate that the type constructor is known when strict mode is active.
		if ch.strictTypeNames && !ch.isKnownTypeName(t.Name) {
			ch.addCodedError(diagnostic.ErrUnboundCon, t.S, "unknown type: "+t.Name)
			return &types.TyError{S: t.S}
		}
		return types.ConAt(t.Name, t.S)
	case *syntax.TyExprQualCon:
		qs, ok := ch.scope.LookupQualified(t.Qualifier)
		if !ok {
			ch.addCodedError(diagnostic.ErrImport, t.S, "unknown qualifier: "+t.Qualifier)
			return &types.TyError{S: t.S}
		}
		// Check qualified aliases (zero-arity: expand inline; parameterized: inject into Scope for TyApp expansion)
		if info, ok := qs.Exports.Aliases[t.Name]; ok {
			if len(info.Params) == 0 {
				return info.Body
			}
			ch.scope.InjectAlias(t.Name, info)
			return types.ConAt(t.Name, t.S)
		}
		// Check qualified type families (zero-arity: immediate; parameterized: inject into Scope for TyApp expansion)
		if fam, ok := qs.Exports.TypeFamilies[t.Name]; ok {
			if len(fam.Params) == 0 {
				return &types.TyFamilyApp{Name: t.Name, Args: nil, Kind: fam.ResultKind, S: t.S}
			}
			ch.scope.InjectFamily(t.Name, fam.Clone())
			return types.ConAt(t.Name, t.S)
		}
		// Check qualified types — only types defined by this module's data declarations,
		// not inherited built-in types (Int, String, etc.).
		if isModuleDefinedType(qs.Exports, t.Name) {
			return types.ConAt(t.Name, t.S)
		}
		// Check promoted kinds/constructors
		if _, ok := qs.Exports.PromotedKinds[t.Name]; ok {
			return types.ConAt(t.Name, t.S)
		}
		if _, ok := qs.Exports.PromotedCons[t.Name]; ok {
			return types.ConAt(t.Name, t.S)
		}
		ch.addCodedError(diagnostic.ErrImport, t.S,
			"module "+qs.ModuleName+" does not export type: "+t.Name)
		return &types.TyError{S: t.S}
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
			if con, ok := b.Kind.(*syntax.TyExprCon); ok && con.Name == "Kind" {
				ch.reg.SetKindVar(b.Name)
				kindVarNames = append(kindVarNames, b.Name)
			}
		}
		ty := ch.resolveTypeExpr(t.Body)
		for i := len(t.Binders) - 1; i >= 0; i-- {
			kind := ch.resolveKindExpr(t.Binders[i].Kind)
			ty = &types.TyForall{Var: t.Binders[i].Name, Kind: kind, Body: ty, S: t.S}
		}
		for _, name := range kindVarNames {
			ch.reg.UnsetKindVar(name)
		}
		return ty
	case *syntax.TyExprRow:
		seen := make(map[string]bool, len(t.Fields))
		var fields []types.RowField
		for _, f := range t.Fields {
			if seen[f.Label] {
				ch.addCodedError(diagnostic.ErrDuplicateLabel, f.S, fmt.Sprintf("duplicate label %q in record type", f.Label))
				continue
			}
			seen[f.Label] = true
			var grades []types.Type
			if f.Mult != nil {
				grades = []types.Type{ch.resolveTypeExpr(f.Mult)}
			}
			fields = append(fields, types.RowField{Label: f.Label, Type: ch.resolveTypeExpr(f.Type), Grades: grades, S: f.S})
		}
		var tail types.Type
		if t.Tail != nil {
			tail = &types.TyVar{Name: t.Tail.Name, S: t.Tail.S}
		}
		// Use ClosedRow/OpenRow to ensure sorted field order.
		if tail == nil {
			return types.ClosedRow(fields...)
		}
		return types.OpenRow(fields, tail)
	case *syntax.TyExprQual:
		// Equality constraint: a ~ T => Body
		// Embedded in TyEvidence as ConstraintEntry with IsEquality=true.
		// No evidence dictionary is generated; the CtEq is emitted when
		// the constraint is instantiated (forall variables → metas).
		if eq, ok := t.Constraint.(*syntax.TyExprEq); ok {
			body := ch.resolveTypeExpr(t.Body)
			lhs := ch.resolveTypeExpr(eq.Lhs)
			rhs := ch.resolveTypeExpr(eq.Rhs)
			entry := types.ConstraintEntry{IsEquality: true, EqLhs: lhs, EqRhs: rhs, S: eq.S}
			return qualifyBody(entry, body, t.S)
		}
		body := ch.resolveTypeExpr(t.Body)
		constraint := ch.resolveTypeExpr(t.Constraint)
		// Quantified constraint: (\ a. C1 a => C2 (f a)) => T
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
		ch.addCodedError(diagnostic.ErrNoInstance, t.S, "invalid constraint: "+types.Pretty(constraint))
		return body
	case *syntax.TyExprEq:
		// Equality constraint outside of a qualified type position.
		// Resolve both sides; the checker will process it contextually.
		lhs := ch.resolveTypeExpr(t.Lhs)
		rhs := ch.resolveTypeExpr(t.Rhs)
		// Emit immediately — this handles edge cases where ~ appears
		// outside constraint position (e.g. in standalone type expressions).
		ch.emitEq(lhs, rhs, t.S, nil)
		return types.Con("()")
	case *syntax.TyExprParen:
		return ch.resolveTypeExpr(t.Inner)
	case *syntax.TyExprLabelLit:
		// Label literals are type-level constants of kind Label.
		return &types.TyCon{Name: t.Label, Level: types.L1, IsLabel: true, S: t.S}
	default:
		ch.addCodedError(diagnostic.ErrTypeMismatch, texpr.Span(), fmt.Sprintf("unsupported type expression: %T", texpr))
		return &types.TyError{S: texpr.Span()}
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
// (\ vars. context => head) and decomposes it into a QuantifiedConstraint.
// Returns nil if the type is not a quantified constraint.
func (ch *Checker) decomposeQuantifiedConstraint(ty types.Type) *types.QuantifiedConstraint {
	// Peel \ binders.
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
		return nil // \ a. T without => is not a quantified constraint
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
// and produces the dedicated TyCBPV nodes, and expands type aliases.
func (ch *Checker) tryExpandApp(fun types.Type, arg types.Type, s span.Span) types.Type {
	// Computation pre post result: TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
	if app2, ok := fun.(*types.TyApp); ok {
		if app1, ok := app2.Fun.(*types.TyApp); ok {
			if con, ok := app1.Fun.(*types.TyCon); ok {
				switch con.Name {
				case types.TyConComputation:
					return &types.TyCBPV{Tag: types.TagComp, Pre: app1.Arg, Post: app2.Arg, Result: arg, S: s}
				case types.TyConThunk:
					return &types.TyCBPV{Tag: types.TagThunk, Pre: app1.Arg, Post: app2.Arg, Result: arg, S: s}
				}
			}
		}
	}
	// General alias/family expansion: collect the TyApp spine and check if the
	// head is an alias or type family with matching parameter count.
	result := &types.TyApp{Fun: fun, Arg: arg, S: s}
	head, args := types.UnwindApp(result)
	if con, ok := head.(*types.TyCon); ok {
		if info, ok := ch.lookupAlias(con.Name); ok && len(info.Params) == len(args) {
			body := info.Body
			for i, p := range info.Params {
				body = types.Subst(body, p, args[i])
			}
			return body
		}
		// Type family: saturated application → TyFamilyApp.
		if fam, ok := ch.lookupFamily(con.Name); ok && len(fam.Params) == len(args) {
			return &types.TyFamilyApp{Name: con.Name, Args: args, Kind: fam.ResultKind, S: s}
		}
	}
	return nil
}
