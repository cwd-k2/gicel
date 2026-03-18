package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

func (ch *Checker) checkDecls(decls []syntax.Decl) *core.Program {
	prog := &core.Program{}

	// 1. Process data declarations (register constructors first).
	for _, d := range decls {
		if data, ok := d.(*syntax.DeclData); ok {
			ch.processDataDecl(data, prog)
		}
	}

	// 2. Process type aliases.
	for _, d := range decls {
		if alias, ok := d.(*syntax.DeclTypeAlias); ok {
			ch.processTypeAlias(alias)
		}
	}

	// 2.5. Process type family declarations.
	for _, d := range decls {
		if tf, ok := d.(*syntax.DeclTypeFamily); ok {
			ch.processTypeFamily(tf)
		}
	}

	// 3. Detect cyclic aliases.
	hasCyclicAlias := ch.validateAliasGraph()

	// 3.5. Install alias expander in unifier for transparent alias handling.
	// Skip installation if cyclic aliases were found to prevent infinite expansion.
	if !hasCyclicAlias {
		ch.installAliasExpander()
	}

	// 4. Process class declarations (generates dict types + selectors).
	for _, d := range decls {
		if cls, ok := d.(*syntax.DeclClass); ok {
			ch.processClassDecl(cls, prog)
		}
	}

	// 5. Process instance headers (validates, registers).
	var instanceDecls []*InstanceInfo
	for _, d := range decls {
		if inst, ok := d.(*syntax.DeclInstance); ok {
			info := ch.processInstanceHeader(inst)
			if info != nil {
				instanceDecls = append(instanceDecls, info)
			}
		}
	}

	// 5.5. Install type family reducer in unifier.
	// Placed after class (phase 4) and instance headers (phase 5) because
	// associated type families are registered in class processing and their
	// equations are collected from instances.
	ch.installFamilyReducer()

	// 5.6. Enable strict type name validation now that all declarations are registered.
	if ch.config.StrictTypeNames {
		ch.strictTypeNames = true
	}

	// 6. Collect type annotations.
	// Free type variables are implicitly universally quantified (implicit forall).
	annotations := make(map[string]types.Type)
	for _, d := range decls {
		if ann, ok := d.(*syntax.DeclTypeAnn); ok {
			ty := ch.resolveTypeExpr(ann.Type)
			annotations[ann.Name] = quantifyFreeVars(ty)
		}
	}

	// 7. Process assumption declarations first (needed by instance bodies).
	for _, d := range decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if v, ok := def.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
				ch.processValueDef(def, annotations, prog)
			}
		}
	}

	// 7.5. Pre-register annotated non-assumption bindings into the context.
	// Only the type is registered; bodies are checked in phase 9.
	// This allows instance methods (phase 8) to reference these bindings,
	// matching the open-scope semantics of Wadler & Blott type classes.
	for _, d := range decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if v, ok := def.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
				continue
			}
			if annTy, hasAnn := annotations[def.Name]; hasAnn {
				ch.ctx.Push(&CtxVar{Name: def.Name, Type: annTy, Module: ch.currentModule})
			}
		}
	}

	// 8. Process instance bodies (type-checks methods, generates dict bindings).
	for _, inst := range instanceDecls {
		ch.processInstanceBody(inst, prog)
	}

	// 9. Process remaining value definitions (non-assumption).
	for _, d := range decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if v, ok := def.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
				continue // already processed
			}
			ch.processValueDef(def, annotations, prog)
		}
	}

	return prog
}

func (ch *Checker) processDataDecl(d *syntax.DeclData, prog *core.Program) {
	// Resolve parameter kinds.
	paramKinds := make([]types.Kind, len(d.Params))
	for i, p := range d.Params {
		paramKinds[i] = ch.resolveKindExpr(p.Kind)
	}

	// Register type constructor kind.
	var kind types.Kind = types.KType{}
	for i := len(d.Params) - 1; i >= 0; i-- {
		kind = &types.KArrow{From: paramKinds[i], To: kind}
	}
	ch.config.RegisteredTypes[d.Name] = kind

	dataInfo := &DataTypeInfo{Name: d.Name}
	ch.dataTypeByName[d.Name] = dataInfo

	// Build result type: T a b c ...
	var resultType types.Type = &types.TyCon{Name: d.Name, S: d.S}
	for _, p := range d.Params {
		resultType = &types.TyApp{Fun: resultType, Arg: &types.TyVar{Name: p.Name, S: p.S}, S: d.S}
	}

	// Register each constructor.
	coreDecl := core.DataDecl{Name: d.Name, S: d.S}
	for i, p := range d.Params {
		coreDecl.TyParams = append(coreDecl.TyParams, core.TyParam{Name: p.Name, Kind: paramKinds[i]})
	}

	for _, con := range d.Cons {
		var conType types.Type = resultType
		var fieldTypes []types.Type
		var constraintEntries []types.ConstraintEntry
		for i := len(con.Fields) - 1; i >= 0; i-- {
			fieldTy := ch.resolveTypeExpr(con.Fields[i])
			// Check if this field is a Constraint-kinded type variable.
			// If so, treat it as an evidence constraint rather than a regular field.
			if ch.isConstraintKindedField(fieldTy, d.Params, paramKinds) {
				entry := ch.constraintFromField(fieldTy)
				if entry != nil {
					constraintEntries = append([]types.ConstraintEntry{*entry}, constraintEntries...)
					continue
				}
			}
			fieldTypes = append([]types.Type{fieldTy}, fieldTypes...)
			conType = types.MkArrow(fieldTy, conType)
		}
		// Wrap with evidence constraints if any fields were Constraint-kinded.
		if len(constraintEntries) > 0 {
			conType = &types.TyEvidence{
				Constraints: &types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: constraintEntries}},
				Body:        conType,
			}
		}
		// Wrap in forall for type params.
		for i := len(d.Params) - 1; i >= 0; i-- {
			conType = types.MkForall(d.Params[i].Name, paramKinds[i], conType)
		}

		ch.conTypes[con.Name] = conType
		ch.ctx.Push(&CtxVar{Name: con.Name, Type: conType, Module: ch.currentModule})
		ch.conModules[con.Name] = ch.currentModule
		dataInfo.Constructors = append(dataInfo.Constructors, ConInfo{Name: con.Name, Arity: len(fieldTypes)})
		ch.conInfo[con.Name] = dataInfo
		coreDecl.Cons = append(coreDecl.Cons, core.ConDecl{Name: con.Name, Fields: fieldTypes, S: con.S})
	}

	// GADT constructors.
	for _, gcon := range d.GADTCons {
		ch.processGADTCon(gcon, d.Params, dataInfo, &coreDecl)
	}

	prog.DataDecls = append(prog.DataDecls, coreDecl)

	// DataKinds: promote nullary constructors to type level.
	dataKind := types.KData{Name: d.Name}
	ch.promotedKinds[d.Name] = dataKind
	for _, con := range d.Cons {
		if len(con.Fields) == 0 {
			ch.promotedCons[con.Name] = dataKind
		}
	}
	for _, gcon := range d.GADTCons {
		fieldTypes, _ := decomposeConSig(ch.resolveTypeExpr(gcon.Type))
		if len(fieldTypes) == 0 {
			ch.promotedCons[gcon.Name] = dataKind
		}
	}
}

// processGADTCon registers a single GADT constructor: resolves its type,
// wraps unquantified data params, and registers it into conTypes/conInfo/coreDecl.
func (ch *Checker) processGADTCon(gcon syntax.GADTConDecl, dataParams []syntax.TyBinder, dataInfo *DataTypeInfo, coreDecl *core.DataDecl) {
	conTy := ch.resolveTypeExpr(gcon.Type)

	// Wrap data type params that appear free in the constructor type
	// but aren't already quantified.
	existingForalls := collectForallNames(conTy)
	for i := len(dataParams) - 1; i >= 0; i-- {
		p := dataParams[i].Name
		if _, already := existingForalls[p]; !already {
			if types.OccursIn(p, conTy) {
				conTy = types.MkForall(p, types.KType{}, conTy)
			}
		}
	}

	fieldTypes, retTy := decomposeConSig(conTy)

	ch.conTypes[gcon.Name] = conTy
	ch.ctx.Push(&CtxVar{Name: gcon.Name, Type: conTy, Module: ch.currentModule})
	ch.conModules[gcon.Name] = ch.currentModule
	dataInfo.Constructors = append(dataInfo.Constructors, ConInfo{
		Name:       gcon.Name,
		Arity:      len(fieldTypes),
		ReturnType: retTy,
	})
	ch.conInfo[gcon.Name] = dataInfo
	coreDecl.Cons = append(coreDecl.Cons, core.ConDecl{
		Name:       gcon.Name,
		Fields:     fieldTypes,
		ReturnType: retTy,
		S:          gcon.S,
	})
}

// isConstraintKindedField checks if a field type references a Constraint-kinded type variable.
func (ch *Checker) isConstraintKindedField(fieldTy types.Type, params []syntax.TyBinder, paramKinds []types.Kind) bool {
	if tv, ok := fieldTy.(*types.TyVar); ok {
		for i, p := range params {
			if p.Name == tv.Name {
				if _, isConstraint := paramKinds[i].(types.KConstraint); isConstraint {
					return true
				}
			}
		}
	}
	return false
}

// constraintFromField converts a Constraint-kinded field type variable into a
// ConstraintEntry. The type variable `c` is stored as ConstraintVar; when
// substituted (e.g., c = Eq Bool), it decomposes into className + args.
func (ch *Checker) constraintFromField(fieldTy types.Type) *types.ConstraintEntry {
	return &types.ConstraintEntry{
		ConstraintVar: fieldTy,
	}
}

// collectForallNames returns the set of names bound by outer foralls.
func collectForallNames(ty types.Type) map[string]struct{} {
	names := make(map[string]struct{})
	for {
		if f, ok := ty.(*types.TyForall); ok {
			names[f.Var] = struct{}{}
			ty = f.Body
		} else {
			break
		}
	}
	return names
}

// decomposeConSig strips outer foralls and qualifications, then peels arrow arguments.
// Returns the list of field types and the final return type.
func decomposeConSig(ty types.Type) (fields []types.Type, ret types.Type) {
	for {
		if f, ok := ty.(*types.TyForall); ok {
			ty = f.Body
		} else {
			break
		}
	}
	for {
		switch t := ty.(type) {
		case *types.TyArrow:
			fields = append(fields, t.From)
			ty = t.To
			continue
		case *types.TyEvidence:
			// Evidence constraints become implicit dict fields at runtime.
			ty = t.Body
			continue
		}
		break
	}
	return fields, ty
}

// isComputationType checks whether a type's return position (after stripping
// foralls, arrows, and qualified constraints) is a Computation type.
func isComputationType(ty types.Type) bool {
	for {
		switch t := ty.(type) {
		case *types.TyForall:
			ty = t.Body
		case *types.TyArrow:
			ty = t.To
		case *types.TyEvidence:
			ty = t.Body
		case *types.TyCBPV:
			return t.Tag == types.TagComp
		default:
			return false
		}
	}
}

// typeArity counts the number of arrow arguments in a type,
// stripping outer foralls. E.g. \ a. A -> B -> C has arity 2.
func typeArity(ty types.Type) int {
	for {
		switch t := ty.(type) {
		case *types.TyForall:
			ty = t.Body
		case *types.TyArrow:
			return 1 + typeArity(t.To)
		default:
			return 0
		}
	}
}

func (ch *Checker) processTypeAlias(d *syntax.DeclTypeAlias) {
	var params []string
	var paramKinds []types.Kind
	for _, p := range d.Params {
		params = append(params, p.Name)
		paramKinds = append(paramKinds, ch.resolveKindExpr(p.Kind))
	}
	body := ch.resolveTypeExpr(d.Body)
	ch.aliases[d.Name] = &AliasInfo{Params: params, ParamKinds: paramKinds, Body: body}
}

func (ch *Checker) processValueDef(d *syntax.DeclValueDef, annotations map[string]types.Type, prog *core.Program) {
	annTy, hasAnn := annotations[d.Name]

	// Check if it's an assumption.
	if v, ok := d.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
		// Try AST annotation first, then config assumptions.
		aTy := annTy
		if !hasAnn {
			if ch.config.Assumptions != nil {
				aTy, hasAnn = ch.config.Assumptions[d.Name]
			}
		}
		if !hasAnn {
			ch.addCodedError(errs.ErrAssumption, d.S, fmt.Sprintf("assumption %s requires a type annotation", d.Name))
			return
		}
		// Note: assumptions without a corresponding RegisterPrim are caught at
		// runtime with "missing primitive" error. Compile-time validation is not
		// feasible because stdlib modules use RegisterPrim (not DeclareAssumption).
		ch.ctx.Push(&CtxVar{Name: d.Name, Type: aTy, Module: ch.currentModule})
		prog.Bindings = append(prog.Bindings, core.Binding{
			Name: d.Name,
			Type: aTy,
			Expr: &core.PrimOp{Name: d.Name, Arity: typeArity(aTy), Effectful: isComputationType(aTy), S: d.S},
			S:    d.S,
		})
		return
	}

	var coreExpr core.Core
	var ty types.Type
	if hasAnn {
		coreExpr = ch.check(d.Expr, annTy)
		ty = annTy
	} else {
		ty, coreExpr = ch.infer(d.Expr)
	}

	// Process stuck type family re-activations: metas solved during
	// type checking may have unblocked previously stuck family applications.
	ch.ProcessRework()

	// Resolve deferred constraints now that metas are solved.
	// For unannotated bindings, defer constraints on unsolved metas
	// so they can be lifted into qualified types by generalization.
	var unresolvedConstraints []deferredConstraint
	if hasAnn {
		coreExpr = ch.resolveDeferredConstraints(coreExpr)
	} else {
		coreExpr, unresolvedConstraints = ch.resolveDeferredConstraintsDeferrable(coreExpr)
	}

	// Zonk the type.
	ty = ch.unifier.Zonk(ty)

	// Let-generalization: for unannotated bindings, replace unsolved
	// metavariables with universally quantified type variables,
	// and lift unresolved constraints into qualified types.
	if !hasAnn {
		ty, coreExpr = ch.generalizeConstrained(ty, coreExpr, unresolvedConstraints)
	}

	ch.ctx.Push(&CtxVar{Name: d.Name, Type: ty, Module: ch.currentModule})
	prog.Bindings = append(prog.Bindings, core.Binding{
		Name: d.Name,
		Type: ty,
		Expr: coreExpr,
		S:    d.S,
	})
}
