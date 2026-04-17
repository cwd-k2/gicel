package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func (r *typeResolver) resolveTypeExpr(texpr syntax.TypeExpr) types.Type {
	switch t := texpr.(type) {
	case *syntax.TyExprVar:
		return &types.TyVar{Name: t.Name, S: t.S}
	case *syntax.TyExprCon:
		return r.resolveUnqualifiedTypeCon(t.Name, t.S)
	case *syntax.TyExprQualCon:
		return r.resolveQualifiedTypeCon(t.Qualifier, t.Name, t.S)
	case *syntax.TyExprApp:
		fun := r.resolveTypeExpr(t.Fun)
		arg := r.resolveTypeExpr(t.Arg)
		// Recognize Computation and Thunk constructor application.
		result := r.tryExpandApp(fun, arg, t.S)
		if result != nil {
			return result
		}
		// @ grade annotation: wrap in TyApp but mark for 4-arg detection.
		// When fun is TyCon("Computation"/"Thunk") and IsGrade, this is the
		// first arg (grade) of a 4-arg form. Store as tagged TyApp so the
		// next tryExpandApp iteration can detect the 4-arg pattern.
		if t.IsGrade {
			r.checkTypeAppKind(fun, arg, t.S)
			return &types.TyApp{Fun: fun, Arg: arg, IsGrade: true, S: t.S}
		}
		r.checkTypeAppKind(fun, arg, t.S)
		return &types.TyApp{Fun: fun, Arg: arg, S: t.S}
	case *syntax.TyExprArrow:
		return &types.TyArrow{
			From: r.resolveTypeExpr(t.From),
			To:   r.resolveTypeExpr(t.To),
			S:    t.S,
		}
	case *syntax.TyExprForall:
		// Register kind variables (binders with Kind sort) before resolving the body,
		// so that kind variable references in inner kind annotations resolve correctly.
		var kindVarNames []string
		var labelVarNames []string
		var levelVarNames []string
		for _, b := range t.Binders {
			if con, ok := b.Kind.(*syntax.TyExprCon); ok {
				switch con.Name {
				case "Kind":
					r.reg.SetKindVar(b.Name)
					kindVarNames = append(kindVarNames, b.Name)
				case "Level":
					r.reg.SetLevelVar(b.Name)
					levelVarNames = append(levelVarNames, b.Name)
				case "Label":
					if r.labelVars == nil {
						r.labelVars = make(map[string]bool, 4)
					}
					r.labelVars[b.Name] = true
					labelVarNames = append(labelVarNames, b.Name)
				}
			}
		}
		// Register explicitly-kinded binder kinds so that checkTypeAppKind
		// can determine the kind of forall-bound type variables during body
		// resolution. Implicit binders (Kind == nil) are skipped — their kind
		// defaults to Type, which checkTypeAppKind handles conservatively.
		for _, b := range t.Binders {
			if b.Kind != nil {
				if r.forallKinds == nil {
					r.forallKinds = make(map[string]types.Type, 4)
				}
				r.forallKinds[b.Name] = r.resolveKindExpr(b.Kind)
			}
		}
		ty := r.resolveTypeExpr(t.Body)
		for i := len(t.Binders) - 1; i >= 0; i-- {
			kind := r.resolveKindExpr(t.Binders[i].Kind)
			ty = &types.TyForall{Var: t.Binders[i].Name, Kind: kind, Body: ty, S: t.S}
		}
		for _, name := range kindVarNames {
			r.reg.UnsetKindVar(name)
		}
		for _, name := range levelVarNames {
			r.reg.UnsetLevelVar(name)
		}
		for _, name := range labelVarNames {
			delete(r.labelVars, name)
		}
		for _, b := range t.Binders {
			delete(r.forallKinds, b.Name)
		}
		return ty
	case *syntax.TyExprRow:
		seen := make(map[string]bool, len(t.Fields))
		var fields []types.RowField
		for _, f := range t.Fields {
			if seen[f.Label] {
				r.addDiag(diagnostic.ErrDuplicateLabel, f.S, diagLabel{Label: f.Label, Context: "duplicate label %q in record type"})
				continue
			}
			seen[f.Label] = true
			var grades []types.Type
			if f.Mult != nil {
				grades = []types.Type{r.resolveTypeExpr(f.Mult)}
			}
			fields = append(fields, types.RowField{
				Label: f.Label, Type: r.resolveTypeExpr(f.Type), Grades: grades,
				IsLabelVar: r.labelVars[f.Label],
				S:          f.S,
			})
		}
		var tail types.Type
		if t.Tail != nil {
			if v, ok := t.Tail.(*syntax.TyExprVar); ok {
				tail = &types.TyVar{Name: v.Name, S: v.S}
			} else {
				tail = r.resolveTypeExpr(t.Tail)
			}
		}
		// Use ClosedRow/OpenRow to ensure sorted field order.
		if tail == nil {
			return types.ClosedRow(fields...)
		}
		return types.OpenRow(fields, tail)
	case *syntax.TyExprQual:
		// Equality constraint: a ~ T => Body
		// Embedded in TyEvidence as an EqualityEntry variant. No evidence
		// dictionary is generated; the CtEq is emitted when the constraint
		// is instantiated (forall variables → metas).
		// Unwrap parentheses so that (a ~ Int) => is handled identically
		// to bare a ~ Int =>.
		rawConstraint := t.Constraint
		if p, ok := rawConstraint.(*syntax.TyExprParen); ok {
			rawConstraint = p.Inner
		}
		if eq, ok := rawConstraint.(*syntax.TyExprEq); ok {
			body := r.resolveTypeExpr(t.Body)
			lhs := r.resolveTypeExpr(eq.Lhs)
			rhs := r.resolveTypeExpr(eq.Rhs)
			entry := &types.EqualityEntry{Lhs: lhs, Rhs: rhs, S: eq.S}
			return qualifyBody(entry, body, t.S)
		}
		body := r.resolveTypeExpr(t.Body)
		constraint := r.resolveTypeExpr(t.Constraint)
		// Quantified constraint: (\ a. C1 a => C2 (f a)) => T
		if qc := r.decomposeQuantifiedConstraint(constraint); qc != nil {
			qc.S = t.S
			return qualifyBody(qc, body, t.S)
		}
		// Simple constraint: C a => T
		head, args := types.UnwindApp(constraint)
		if con, ok := head.(*types.TyCon); ok {
			entry := &types.ClassEntry{ClassName: con.Name, Args: args, S: t.S}
			return qualifyBody(entry, body, t.S)
		}
		r.addDiag(diagnostic.ErrNoInstance, t.S, diagWithType{Context: "invalid constraint: ", Type: constraint})
		return body
	case *syntax.TyExprEq:
		// Equality constraint outside of a qualified type position.
		// Resolve both sides; the checker will process it contextually.
		lhs := r.resolveTypeExpr(t.Lhs)
		rhs := r.resolveTypeExpr(t.Rhs)
		// Emit immediately — this handles edge cases where ~ appears
		// outside constraint position (e.g. in standalone type expressions).
		r.emitEq(lhs, rhs, t.S, nil)
		return r.unifier.TypeOps.Con("()", span.Span{})
	case *syntax.TyExprParen:
		return r.resolveTypeExpr(t.Inner)
	case *syntax.TyExprLabelLit:
		// Label literals are type-level constants of kind Label.
		return &types.TyCon{Name: t.Label, Level: types.L1, IsLabel: true, S: t.S}
	case *syntax.TyExprError:
		return &types.TyError{S: t.S}
	default:
		r.addDiag(diagnostic.ErrTypeMismatch, texpr.Span(), diagFmt{Format: "unsupported type expression: %T", Args: []any{texpr}})
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
		ce := &types.ConstraintEntries{Entries: entries}
		cr := &types.TyEvidenceRow{Entries: ce, Flags: types.EvidenceRowFlags(ce, nil)}
		return &types.TyEvidence{
			Constraints: cr,
			Body:        ev.Body,
			Flags:       types.MetaFreeFlags(cr, ev.Body),
			S:           s,
		}
	}
	ce := &types.ConstraintEntries{Entries: []types.ConstraintEntry{entry}}
	cr := &types.TyEvidenceRow{Entries: ce, Flags: types.EvidenceRowFlags(ce, nil)}
	return &types.TyEvidence{
		Constraints: cr,
		Body:        body,
		Flags:       types.MetaFreeFlags(cr, body),
		S:           s,
	}
}

// decomposeQuantifiedConstraint checks if a resolved type is a quantified constraint
// (\ vars. context => head) and decomposes it into a QuantifiedConstraint.
// Returns nil if the type is not a quantified constraint.
func (r *typeResolver) decomposeQuantifiedConstraint(ty types.Type) *types.QuantifiedConstraint {
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
	head := &types.ClassEntry{ClassName: headCon.Name, Args: headArgs}
	// All entries in the evidence are context (premise) constraints.
	return &types.QuantifiedConstraint{
		Vars:    vars,
		Context: conEntries,
		Head:    head,
	}
}

// tryExpandApp recognizes fully-saturated Computation and Thunk applications
// and produces the dedicated TyCBPV nodes, and expands type aliases.
func (r *typeResolver) tryExpandApp(fun types.Type, arg types.Type, s span.Span) types.Type {
	// Try 4-arg: Computation grade pre post result
	if app3, ok := fun.(*types.TyApp); ok {
		if app2, ok := app3.Fun.(*types.TyApp); ok {
			if app1, ok := app2.Fun.(*types.TyApp); ok {
				if con, ok := app1.Fun.(*types.TyCon); ok {
					switch con.Name {
					case types.TyConComputation:
						return &types.TyCBPV{Tag: types.TagComp, Grade: app1.Arg, Pre: app2.Arg, Post: app3.Arg, Result: arg, Flags: types.MetaFreeFlags(app1.Arg, app2.Arg, app3.Arg, arg), S: s}
					case types.TyConThunk:
						return &types.TyCBPV{Tag: types.TagThunk, Grade: app1.Arg, Pre: app2.Arg, Post: app3.Arg, Result: arg, Flags: types.MetaFreeFlags(app1.Arg, app2.Arg, app3.Arg, arg), S: s}
					}
				}
			}
		}
	}
	// 3-arg ungraded form: Computation pre post result (no @ grade
	// annotation). This branch is a fast-path heuristic that fires only
	// when the first arg is a row literal (TyEvidenceRow), so a partial
	// 4-arg application — whose first arg is a grade (TyCon, TyVar) —
	// is not misclassified. For ungraded uses where the first arg is a
	// TyVar (e.g., the Prelude `merge` signature), this branch does not
	// fire and the resolver leaves a raw TyApp chain; the unifier then
	// converts it via normalizeCompApp at unification time.
	if app2, ok := fun.(*types.TyApp); ok {
		if app1, ok := app2.Fun.(*types.TyApp); ok {
			if con, ok := app1.Fun.(*types.TyCon); ok {
				if _, isRow := app1.Arg.(*types.TyEvidenceRow); isRow {
					switch con.Name {
					case types.TyConComputation:
						return &types.TyCBPV{Tag: types.TagComp, Pre: app1.Arg, Post: app2.Arg, Result: arg, Flags: types.MetaFreeFlags(app1.Arg, app2.Arg, arg), S: s}
					case types.TyConThunk:
						return &types.TyCBPV{Tag: types.TagThunk, Pre: app1.Arg, Post: app2.Arg, Result: arg, Flags: types.MetaFreeFlags(app1.Arg, app2.Arg, arg), S: s}
					}
				}
			}
		}
	}
	// General alias/family expansion: collect the TyApp spine and check if the
	// head is an alias or type family with matching parameter count.
	result := &types.TyApp{Fun: fun, Arg: arg, S: s}
	head, args := types.UnwindApp(result)
	if con, ok := head.(*types.TyCon); ok {
		if info, ok := r.lookupAlias(con.Name); ok && len(info.Params) == len(args) {
			// Guard against cyclic alias expansion: if the body references
			// the alias itself, substitution produces an infinite loop.
			if aliasBodyRefsSelf(con.Name, info.Body) {
				return nil
			}
			var body types.Type
			if len(info.Params) == 1 {
				body = r.unifier.TypeOps.Subst(info.Body, info.Params[0], args[0])
			} else {
				subs := make(map[string]types.Type, len(info.Params))
				for i, p := range info.Params {
					subs[p] = args[i]
				}
				body = r.unifier.TypeOps.SubstMany(info.Body, subs, nil)
			}
			// Re-check the expanded result: alias expansion may produce a
			// saturated Computation/Thunk TyApp chain that needs TyCBPV conversion.
			if app, ok := body.(*types.TyApp); ok {
				if expanded := r.tryExpandApp(app.Fun, app.Arg, s); expanded != nil {
					return expanded
				}
			}
			return body
		}
		// Type family: saturated application → TyFamilyApp.
		if fam, ok := r.lookupFamily(con.Name); ok && len(fam.Params) == len(args) {
			return &types.TyFamilyApp{Name: con.Name, Args: args, Kind: fam.ResultKind, Flags: types.MetaFreeFlags(append(args, fam.ResultKind)...) &^ types.FlagNoFamilyApp, S: s}
		}
	}
	return nil
}

// aliasBodyRefsSelf checks if a type alias body contains a direct reference
// to the alias itself, indicating a cyclic definition.
func aliasBodyRefsSelf(name string, ty types.Type) bool {
	switch t := ty.(type) {
	case *types.TyCon:
		return t.Name == name
	case *types.TyApp:
		return aliasBodyRefsSelf(name, t.Fun) || aliasBodyRefsSelf(name, t.Arg)
	case *types.TyArrow:
		return aliasBodyRefsSelf(name, t.From) || aliasBodyRefsSelf(name, t.To)
	case *types.TyForall:
		return aliasBodyRefsSelf(name, t.Body)
	}
	return false
}
