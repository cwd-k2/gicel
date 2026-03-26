package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processFormDeclParts registers a form declaration from pre-decomposed body parts.
// Class-like form declarations must be filtered out by the caller.
func (ch *Checker) processFormDeclParts(d *syntax.DeclForm, parts formBodyParts, prog *ir.Program) {
	// Resolve parameter kinds.
	paramKinds := make([]types.Kind, len(parts.Params))
	for i, p := range parts.Params {
		paramKinds[i] = ch.resolveKindExpr(p.Kind)
	}

	// Register type constructor kind.
	// The result kind of a form declaration is always Type.
	// If a kind annotation is present, validate that it is consistent.
	var kind types.Kind = types.KType{}
	for i := len(parts.Params) - 1; i >= 0; i-- {
		kind = &types.KArrow{From: paramKinds[i], To: kind}
	}
	if d.KindAnn != nil {
		annKind := ch.resolveKindExpr(d.KindAnn)
		resultKind := types.ResultKind(annKind)
		if _, isSort := resultKind.(types.KSort); isSort {
			ch.addCodedError(diagnostic.ErrKindMismatch, d.S,
				fmt.Sprintf("form %s has result kind %s, but form declarations must have result kind Type", d.Name, resultKind))
		}
	}
	ch.reg.RegisterTypeKind(d.Name, kind)

	dataInfo := &DataTypeInfo{Name: d.Name}
	ch.reg.RegisterDataType(d.Name, dataInfo)

	// Build result type: T a b c ...
	var resultType types.Type = types.ConAt(d.Name, d.S)
	for _, p := range parts.Params {
		resultType = &types.TyApp{Fun: resultType, Arg: &types.TyVar{Name: p.Name, S: p.S}, S: d.S}
	}

	// Register each constructor from row fields.
	// In unified syntax, constructors are uppercase fields in the body row:
	//   form Maybe := \a. { Nothing: (); Just: a; };
	coreDecl := ir.DataDecl{Name: d.Name, S: d.S}
	for i, p := range parts.Params {
		coreDecl.TyParams = append(coreDecl.TyParams, ir.TyParam{Name: p.Name, Kind: paramKinds[i]})
	}

	seenCons := make(map[string]bool, len(parts.Fields))
	for _, field := range parts.Fields {
		conName := field.Label
		if seenCons[conName] {
			ch.addCodedError(diagnostic.ErrDuplicateDecl, field.S,
				fmt.Sprintf("duplicate constructor %q in form %s", conName, d.Name))
			continue
		}
		seenCons[conName] = true
		fieldTy := ch.resolveTypeExpr(field.Type)

		// Constructor type is the full GADT-style type.
		// The field type IS the constructor's full type:
		//   Nil:  List a                → nullary, return = List a
		//   Cons: a -> List a -> List a → binary, fields = [a, List a], return = List a
		//   Just: a -> Maybe a          → unary, fields = [a], return = Maybe a
		//   Lit:  Int -> Expr Int       → GADT, fields = [Int], return = Expr Int (refined)
		//
		// The checker peels arrows to extract field types; the last type is the return.
		// ADT shorthand generates unit type (Record {}) for nullary constructors.
		// Replace with resultType so the constructor type is correct.
		conType := fieldTy
		if isUnitType(fieldTy) {
			// Nullary constructor: replace unit with result type.
			conType = resultType
			fieldTy = resultType
		}
		fieldTypes, retTy := decomposeConSig(fieldTy)

		// ADT shorthand: the parser synthesizes () as a sentinel return type.
		// Replace it with the actual result type (e.g., Nat for form Nat := ...).
		if isUnitType(retTy) && len(fieldTypes) > 0 {
			retTy = resultType
			// Rebuild conType: field1 -> field2 -> ... -> resultType
			conType = resultType
			for i := len(fieldTypes) - 1; i >= 0; i-- {
				conType = types.MkArrow(fieldTypes[i], conType)
			}
			fieldTy = conType
		}

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

	// DataKinds: promote all constructors to type level.
	// Both nullary (e.g., True, False) and non-nullary (e.g., Cons, Pi)
	// are promoted, enabling universe decoding patterns at the type level.
	dataKind := types.KData{Name: d.Name}
	ch.reg.RegisterPromotedKind(d.Name, dataKind)
	for _, field := range parts.Fields {
		ch.reg.RegisterPromotedCon(field.Label, dataKind)
	}
}

// isUnitType checks if a type is the unit type: Record {} or bare {}.
func isUnitType(t types.Type) bool {
	if app, ok := t.(*types.TyApp); ok {
		if con, ok := app.Fun.(*types.TyCon); ok && con.Name == "Record" {
			if row, ok := app.Arg.(*types.TyEvidenceRow); ok {
				return row.Entries.EntryCount() == 0
			}
		}
	}
	if row, ok := t.(*types.TyEvidenceRow); ok {
		return row.Entries.EntryCount() == 0
	}
	return false
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
