package check

import (
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processDataDeclParts registers a data type from pre-decomposed body parts.
// Class-like data declarations must be filtered out by the caller.
func (ch *Checker) processDataDeclParts(d *syntax.DeclData, parts dataBodyParts, prog *ir.Program) {
	// Resolve parameter kinds.
	paramKinds := make([]types.Kind, len(parts.Params))
	for i, p := range parts.Params {
		paramKinds[i] = ch.resolveKindExpr(p.Kind)
	}

	// Register type constructor kind.
	var kind types.Kind = types.KType{}
	for i := len(parts.Params) - 1; i >= 0; i-- {
		kind = &types.KArrow{From: paramKinds[i], To: kind}
	}
	ch.reg.RegisterTypeKind(d.Name, kind)

	dataInfo := &DataTypeInfo{Name: d.Name}
	ch.reg.RegisterDataType(d.Name, dataInfo)

	// Build result type: T a b c ...
	var resultType types.Type = &types.TyCon{Name: d.Name, S: d.S}
	for _, p := range parts.Params {
		resultType = &types.TyApp{Fun: resultType, Arg: &types.TyVar{Name: p.Name, S: p.S}, S: d.S}
	}

	// Register each constructor from row fields.
	// In unified syntax, constructors are uppercase fields in the body row:
	//   data Maybe := \a. { Nothing: (); Just: a; };
	coreDecl := ir.DataDecl{Name: d.Name, S: d.S}
	for i, p := range parts.Params {
		coreDecl.TyParams = append(coreDecl.TyParams, ir.TyParam{Name: p.Name, Kind: paramKinds[i]})
	}

	for _, field := range parts.Fields {
		conName := field.Label
		fieldTy := ch.resolveTypeExpr(field.Type)

		// Constructor type is the full GADT-style type.
		// The field type IS the constructor's full type:
		//   Nil:  List a                → nullary, return = List a
		//   Cons: a -> List a -> List a → binary, fields = [a, List a], return = List a
		//   Just: a -> Maybe a          → unary, fields = [a], return = Maybe a
		//   Lit:  Int -> Expr Int       → GADT, fields = [Int], return = Expr Int (refined)
		//
		// The checker peels arrows to extract field types; the last type is the return.
		conType := fieldTy
		fieldTypes, retTy := decomposeConSig(fieldTy)

		// Detect GADT: if the constructor's return type differs from the
		// generic result type (T a b c ...), this is a refined return type.
		var gadtReturnType types.Type
		if !types.Equal(retTy, resultType) {
			gadtReturnType = retTy
		}

		// Wrap in forall for type params.
		for i := len(parts.Params) - 1; i >= 0; i-- {
			conType = types.MkForall(parts.Params[i].Name, paramKinds[i], conType)
		}

		ch.ctx.Push(&CtxVar{Name: conName, Type: conType, Module: ch.scope.CurrentModule()})
		dataInfo.Constructors = append(dataInfo.Constructors, ConstructorInfo{Name: conName, Arity: len(fieldTypes), ReturnType: gadtReturnType})
		ch.reg.RegisterConstructor(conName, conType, ch.scope.CurrentModule(), dataInfo)
		coreDecl.Cons = append(coreDecl.Cons, ir.ConDecl{Name: conName, Fields: fieldTypes, ReturnType: gadtReturnType, S: field.S})
	}

	prog.DataDecls = append(prog.DataDecls, coreDecl)

	// DataKinds: promote nullary constructors to type level.
	dataKind := types.KData{Name: d.Name}
	ch.reg.RegisterPromotedKind(d.Name, dataKind)
	for _, field := range parts.Fields {
		fieldTy := ch.resolveTypeExpr(field.Type)
		fields, _ := decomposeConSig(fieldTy)
		if len(fields) == 0 {
			ch.reg.RegisterPromotedCon(field.Label, dataKind)
		}
	}
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
