package check

import (
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

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
	ch.reg.typeKinds[d.Name] = kind

	dataInfo := &DataTypeInfo{Name: d.Name}
	ch.reg.dataTypeByName[d.Name] = dataInfo

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

		ch.reg.conTypes[con.Name] = conType
		ch.ctx.Push(&CtxVar{Name: con.Name, Type: conType, Module: ch.scope.currentModule})
		ch.reg.conModules[con.Name] = ch.scope.currentModule
		dataInfo.Constructors = append(dataInfo.Constructors, ConstructorInfo{Name: con.Name, Arity: len(fieldTypes)})
		ch.reg.conInfo[con.Name] = dataInfo
		coreDecl.Cons = append(coreDecl.Cons, core.ConDecl{Name: con.Name, Fields: fieldTypes, S: con.S})
	}

	// GADT constructors.
	for _, gcon := range d.GADTCons {
		ch.processGADTCon(gcon, d.Params, dataInfo, &coreDecl)
	}

	prog.DataDecls = append(prog.DataDecls, coreDecl)

	// DataKinds: promote nullary constructors to type level.
	dataKind := types.KData{Name: d.Name}
	ch.reg.promotedKinds[d.Name] = dataKind
	for _, con := range d.Cons {
		if len(con.Fields) == 0 {
			ch.reg.promotedCons[con.Name] = dataKind
		}
	}
	for _, gcon := range d.GADTCons {
		fieldTypes, _ := decomposeConSig(ch.resolveTypeExpr(gcon.Type))
		if len(fieldTypes) == 0 {
			ch.reg.promotedCons[gcon.Name] = dataKind
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

	ch.reg.conTypes[gcon.Name] = conTy
	ch.ctx.Push(&CtxVar{Name: gcon.Name, Type: conTy, Module: ch.scope.currentModule})
	ch.reg.conModules[gcon.Name] = ch.scope.currentModule
	dataInfo.Constructors = append(dataInfo.Constructors, ConstructorInfo{
		Name:       gcon.Name,
		Arity:      len(fieldTypes),
		ReturnType: retTy,
	})
	ch.reg.conInfo[gcon.Name] = dataInfo
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

// isBareComputationType checks whether a type is a bare Computation
// after stripping outer foralls and qualified constraints.
// Unlike isComputationType, this does NOT strip arrows:
//
//	Int -> Computation {} {} a  → false (function type, a value)
//	\ a. Computation a a Int    → true  (bare Computation)
//	Thunk {} {} a               → false (Thunk is a value)
func isBareComputationType(ty types.Type) bool {
	for {
		switch t := ty.(type) {
		case *types.TyForall:
			ty = t.Body
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
