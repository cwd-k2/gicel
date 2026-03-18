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
		if info, ok := ch.aliases[t.Name]; ok && len(info.Params) == 0 {
			return info.Body
		}
		// Zero-arity type family: immediate TyFamilyApp.
		if fam, ok := ch.families[t.Name]; ok && len(fam.Params) == 0 {
			return &types.TyFamilyApp{Name: t.Name, Args: nil, Kind: fam.ResultKind, S: t.S}
		}
		// Validate that the type constructor is known when strict mode is active.
		if ch.strictTypeNames && !ch.isKnownTypeName(t.Name) {
			ch.addCodedError(errs.ErrUnboundCon, t.S, fmt.Sprintf("unknown type: %s", t.Name))
			return &types.TyError{S: t.S}
		}
		return &types.TyCon{Name: t.Name, S: t.S}
	case *syntax.TyExprQualCon:
		qs, ok := ch.qualifiedScopes[t.Qualifier]
		if !ok {
			ch.addCodedError(errs.ErrImport, t.S, fmt.Sprintf("unknown qualifier: %s", t.Qualifier))
			return &types.TyError{S: t.S}
		}
		// Check qualified aliases (zero-arity: expand inline; parameterized: inject into local scope for TyApp expansion)
		if info, ok := qs.exports.Aliases[t.Name]; ok {
			if len(info.Params) == 0 {
				return info.Body
			}
			ch.aliases[t.Name] = info
			return &types.TyCon{Name: t.Name, S: t.S}
		}
		// Check qualified type families (zero-arity: immediate; parameterized: inject for TyApp expansion)
		if fam, ok := qs.exports.TypeFamilies[t.Name]; ok {
			if len(fam.Params) == 0 {
				return &types.TyFamilyApp{Name: t.Name, Args: nil, Kind: fam.ResultKind, S: t.S}
			}
			ch.families[t.Name] = fam.Clone()
			return &types.TyCon{Name: t.Name, S: t.S}
		}
		// Check qualified types — only types defined by this module's data declarations,
		// not inherited built-in types (Int, String, etc.).
		if isModuleDefinedType(qs.exports, t.Name) {
			return &types.TyCon{Name: t.Name, S: t.S}
		}
		// Check promoted kinds/constructors
		if _, ok := qs.exports.PromotedKinds[t.Name]; ok {
			return &types.TyCon{Name: t.Name, S: t.S}
		}
		if _, ok := qs.exports.PromotedCons[t.Name]; ok {
			return &types.TyCon{Name: t.Name, S: t.S}
		}
		ch.addCodedError(errs.ErrImport, t.S,
			fmt.Sprintf("module %s does not export type: %s", qs.moduleName, t.Name))
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
		seen := make(map[string]bool, len(t.Fields))
		var fields []types.RowField
		for _, f := range t.Fields {
			if seen[f.Label] {
				ch.addCodedError(errs.ErrDuplicateLabel, f.S, fmt.Sprintf("duplicate label %q in record type", f.Label))
				continue
			}
			seen[f.Label] = true
			var mult types.Type
			if f.Mult != nil {
				mult = ch.resolveTypeExpr(f.Mult)
			}
			fields = append(fields, types.RowField{Label: f.Label, Type: ch.resolveTypeExpr(f.Type), Mult: mult, S: f.S})
		}
		var tail types.Type
		if t.Tail != nil {
			tail = &types.TyVar{Name: t.Tail.Name, S: t.Tail.S}
		}
		return &types.TyEvidenceRow{
			Entries: &types.CapabilityEntries{Fields: fields},
			Tail:    tail,
			S:       t.S,
		}
	case *syntax.TyExprQual:
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
	// General alias/family expansion: collect the TyApp spine and check if the
	// head is an alias or type family with matching parameter count.
	result := &types.TyApp{Fun: fun, Arg: arg, S: s}
	head, args := types.UnwindApp(result)
	if con, ok := head.(*types.TyCon); ok {
		if info, ok := ch.aliases[con.Name]; ok && len(info.Params) == len(args) {
			body := info.Body
			for i, p := range info.Params {
				body = types.Subst(body, p, args[i])
			}
			return body
		}
		// Type family: saturated application → TyFamilyApp.
		if fam, ok := ch.families[con.Name]; ok && len(fam.Params) == len(args) {
			return &types.TyFamilyApp{Name: con.Name, Args: args, Kind: fam.ResultKind, S: s}
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

// isModuleDefinedType checks if a type name was defined by the module itself
// (via data declarations or class declarations), as opposed to being inherited
// from built-in types or open imports.
func isModuleDefinedType(exports *ModuleExports, name string) bool {
	for _, dd := range exports.DataDecls {
		if dd.Name == name {
			return true
		}
	}
	// Classes are already checked separately, but class-defined types
	// (dict types) might appear in Types.
	if _, ok := exports.Classes[name]; ok {
		return true
	}
	return false
}

// builtinTypeNames are type constructor names that are intrinsic to the checker
// (used in TyComp/TyThunk expansion) but not registered in RegisteredTypes.
var builtinTypeNames = map[string]bool{
	"Computation": true,
	"Thunk":       true,
}

// isKnownTypeName returns true if name refers to a known type: registered type,
// parameterized alias, parameterized type family, class, promoted kind/constructor,
// or checker-intrinsic type (Computation, Thunk).
func (ch *Checker) isKnownTypeName(name string) bool {
	if builtinTypeNames[name] {
		return true
	}
	if _, ok := ch.config.RegisteredTypes[name]; ok {
		return true
	}
	if _, ok := ch.aliases[name]; ok {
		return true
	}
	if _, ok := ch.families[name]; ok {
		return true
	}
	if _, ok := ch.classes[name]; ok {
		return true
	}
	if _, ok := ch.promotedKinds[name]; ok {
		return true
	}
	if _, ok := ch.promotedCons[name]; ok {
		return true
	}
	return false
}

// aliasParamKind returns the kind of the i-th parameter of a type alias.
func (ch *Checker) aliasParamKind(aliasName string, i int) types.Kind {
	info, ok := ch.aliases[aliasName]
	if !ok || i >= len(info.ParamKinds) {
		return types.KType{}
	}
	return info.ParamKinds[i]
}

// kindOfType returns the kind of a resolved type, or nil if unknown.
func (ch *Checker) kindOfType(ty types.Type) types.Kind {
	switch t := ty.(type) {
	case *types.TyCon:
		if k, ok := ch.config.RegisteredTypes[t.Name]; ok {
			return k
		}
		// Type aliases: compute kind from parameter kinds.
		if info, ok := ch.aliases[t.Name]; ok {
			var kind types.Kind = types.KType{}
			for i := len(info.Params) - 1; i >= 0; i-- {
				paramKind := ch.aliasParamKind(t.Name, i)
				kind = &types.KArrow{From: paramKind, To: kind}
			}
			return kind
		}
		if k, ok := ch.promotedCons[t.Name]; ok {
			return k
		}
		return types.KType{}
	case *types.TyApp:
		funKind := ch.kindOfType(t.Fun)
		if ka, ok := funKind.(*types.KArrow); ok {
			return ka.To
		}
		return nil
	case *types.TyMeta:
		return t.Kind
	case *types.TySkolem:
		return t.Kind
	case *types.TyVar:
		if k, ok := ch.ctx.LookupTyVar(t.Name); ok {
			return k
		}
		return types.KType{}
	default:
		return types.KType{}
	}
}
