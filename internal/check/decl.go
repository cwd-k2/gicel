package check

import (
	"fmt"
	"sort"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/internal/types"
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
		ch.ctx.Push(&CtxVar{Name: con.Name, Type: conType})
		dataInfo.Constructors = append(dataInfo.Constructors, ConInfo{Name: con.Name, Arity: len(fieldTypes)})
		ch.conInfo[con.Name] = dataInfo
		coreDecl.Cons = append(coreDecl.Cons, core.ConDecl{Name: con.Name, Fields: fieldTypes, S: con.S})
	}

	// GADT constructors.
	for _, gcon := range d.GADTCons {
		conTy := ch.resolveTypeExpr(gcon.Type)

		// Wrap data type params that appear free in the constructor type
		// but aren't already quantified. This makes `data F f = { MkF :: forall a. f a -> F f }`
		// work correctly by wrapping f in an outer forall.
		existingForalls := collectForallNames(conTy)
		for i := len(d.Params) - 1; i >= 0; i-- {
			p := d.Params[i].Name
			if _, already := existingForalls[p]; !already {
				if types.OccursIn(p, conTy) {
					conTy = types.MkForall(p, types.KType{}, conTy)
				}
			}
		}

		// Decompose the resolved type into (field types, return type),
		// skipping any outer foralls and qualifications.
		fieldTypes, retTy := decomposeConSig(conTy)

		ch.conTypes[gcon.Name] = conTy
		ch.ctx.Push(&CtxVar{Name: gcon.Name, Type: conTy})
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
		case *types.TyComp:
			return true
		default:
			return false
		}
	}
}

// typeArity counts the number of arrow arguments in a type,
// stripping outer foralls. E.g. forall a. A -> B -> C has arity 2.
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
	ch.aliases[d.Name] = &aliasInfo{params: params, paramKinds: paramKinds, body: body}
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
		ch.ctx.Push(&CtxVar{Name: d.Name, Type: aTy})
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

	// Resolve deferred constraints now that metas are solved.
	coreExpr = ch.resolveDeferredConstraints(coreExpr)

	// Zonk the type.
	ty = ch.unifier.Zonk(ty)

	ch.ctx.Push(&CtxVar{Name: d.Name, Type: ty})
	prog.Bindings = append(prog.Bindings, core.Binding{
		Name: d.Name,
		Type: ty,
		Expr: coreExpr,
		S:    d.S,
	})
}

// quantifyFreeVars wraps free type variables in implicit forall quantifiers.
// This implements Haskell-style implicit universal quantification for type annotations:
// `f :: List a -> Int` is treated as `f :: forall a. List a -> Int`.
func quantifyFreeVars(ty types.Type) types.Type {
	fv := types.FreeVars(ty)
	if len(fv) == 0 {
		return ty
	}
	vars := make([]string, 0, len(fv))
	for v := range fv {
		vars = append(vars, v)
	}
	sort.Strings(vars)
	for i := len(vars) - 1; i >= 0; i-- {
		ty = types.MkForall(vars[i], types.KType{}, ty)
	}
	return ty
}
