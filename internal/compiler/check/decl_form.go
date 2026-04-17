package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processFormDeclParts registers a form declaration from pre-decomposed body parts.
// Class-like form declarations must be filtered out by the caller.
func (ch *Checker) processFormDeclParts(d *syntax.DeclForm, parts formBodyParts, prog *ir.Program) {
	// Resolve parameter kinds.
	paramKinds := make([]types.Type, len(parts.Params))
	for i, p := range parts.Params {
		paramKinds[i] = ch.resolveKindExpr(p.Kind)
	}

	// Register type constructor kind.
	// The result kind is Type at a level computed from parameter kinds:
	// if all params are Type l_i, result is Type (max l_1 ... l_n).
	// If any param is not Type-kinded, result defaults to TypeOfTypes (L1).
	var kind types.Type = formResultKind(paramKinds)
	for i := len(parts.Params) - 1; i >= 0; i-- {
		kind = &types.TyArrow{From: paramKinds[i], To: kind, Flags: types.MetaFreeFlags(paramKinds[i], kind)}
	}
	if d.KindAnn != nil {
		annKind := ch.resolveKindExpr(d.KindAnn)
		resultKind := types.ResultKind(annKind)
		if isSortKind(resultKind) {
			ch.addDiag(diagnostic.ErrKindMismatch, d.S,
				diagFmt{Format: "form %s has result kind %s, but form declarations must have result kind Type", Args: []any{d.Name, resultKind}})
			return // halt: do not register invalid form
		}
	}
	ch.reg.RegisterTypeKind(d.Name, kind)

	dataInfo := &DataTypeInfo{Name: d.Name, IsLazy: d.IsLazy}
	ch.reg.RegisterDataType(d.Name, dataInfo)

	// Build result type: T a b c ...
	var resultType types.Type = ch.typeOps.Con(d.Name, d.S)
	for _, p := range parts.Params {
		arg := &types.TyVar{Name: p.Name, S: p.S}
		resultType = &types.TyApp{Fun: resultType, Arg: arg, Flags: types.MetaFreeFlags(resultType, arg), S: d.S}
	}

	// Register each constructor from row fields.
	// Constructors are uppercase fields with GADT-style full type signatures:
	//   form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; };
	coreDecl := ir.DataDecl{Name: d.Name, S: d.S}
	for i, p := range parts.Params {
		coreDecl.TyParams = append(coreDecl.TyParams, ir.TyParam{Name: p.Name, Kind: paramKinds[i]})
	}

	seenCons := make(map[string]bool, len(parts.Fields))
	for _, field := range parts.Fields {
		conName := field.Label
		if seenCons[conName] {
			ch.addDiag(diagnostic.ErrDuplicateDecl, field.S,
				diagFmt{Format: "duplicate constructor %q in form %s", Args: []any{conName, d.Name}})
			continue
		}
		seenCons[conName] = true

		fieldTy := ch.resolveTypeExpr(field.Type)
		conType := fieldTy
		fieldTypes, retTy := decomposeConSig(fieldTy)

		// Detect GADT: if the constructor's return type differs from the
		// generic result type (T a b c ...), this is a refined return type.
		var gadtReturnType types.Type
		if !ch.typeOps.Equal(retTy, resultType) {
			gadtReturnType = retTy
		}

		// Wrap in forall for type params.
		for i := len(parts.Params) - 1; i >= 0; i-- {
			conType = ch.typeOps.Forall(parts.Params[i].Name, paramKinds[i], conType, span.Span{})
		}

		ch.ctx.Push(&CtxVar{Name: conName, Type: conType, Module: ch.scope.CurrentModule()})
		dataInfo.Constructors = append(dataInfo.Constructors, ConstructorInfo{Name: conName, Arity: len(fieldTypes), ReturnType: gadtReturnType})
		ch.reg.RegisterConstructor(conName, conType, ch.scope.CurrentModule(), dataInfo)
		coreDecl.Cons = append(coreDecl.Cons, ir.ConDecl{Name: conName, Fields: fieldTypes, ReturnType: gadtReturnType, FullType: conType, S: field.S})
	}

	prog.DataDecls = append(prog.DataDecls, coreDecl)

	// DataKinds: promote all constructors to type level.
	// Nullary constructors (e.g., True, False) get PromotedDataKind(Name).
	// Non-nullary constructors (e.g., Just: a -> Maybe a) get a kind arrow:
	//   Just :: Type -> PromotedDataKind(Maybe)
	// This enables type-level application of promoted constructors.
	dataKind := types.PromotedDataKind(d.Name)
	ch.reg.RegisterPromotedKind(d.Name, dataKind)
	for _, con := range coreDecl.Cons {
		var conKind types.Type = dataKind
		// Build kind arrow from field types (right to left).
		for i := len(con.Fields) - 1; i >= 0; i-- {
			fieldKind := ch.promotedFieldKind(con.Fields[i])
			conKind = &types.TyArrow{From: fieldKind, To: conKind, Flags: types.MetaFreeFlags(fieldKind, conKind)}
		}
		ch.reg.RegisterPromotedCon(con.Name, conKind)
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
func typeArity(ops *types.TypeOps, ty types.Type) int {
	for {
		switch t := ty.(type) {
		case *types.TyForall:
			// Label-kinded foralls become value-level parameters after label
			// erasure (TyLam → Lam at call sites). Count them in PrimOp arity
			// so the VM collects the label string as an arg.
			if ops.Equal(t.Kind, types.TypeOfLabels) {
				return 1 + typeArity(ops, t.Body)
			}
			ty = t.Body
		case *types.TyArrow:
			return 1 + typeArity(ops, t.To)
		default:
			return 0
		}
	}
}

// formResultKind computes the result kind for a form declaration from its
// parameter kinds. If all parameters are Type l_i, the result is Type (max l_1 ... l_n).
// Otherwise, defaults to TypeOfTypes (Type at level 1).
func formResultKind(paramKinds []types.Type) types.Type {
	if len(paramKinds) == 0 {
		return types.TypeOfTypes
	}
	level, ok := extractTypeLevel(paramKinds[0])
	if !ok {
		return types.TypeOfTypes
	}
	for _, pk := range paramKinds[1:] {
		l, ok := extractTypeLevel(pk)
		if !ok {
			return types.TypeOfTypes
		}
		level = joinLevel(level, l)
	}
	return &types.TyCon{Name: "Type", Level: level}
}
