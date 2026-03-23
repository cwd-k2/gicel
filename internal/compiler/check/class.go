package check

import (
	"fmt"
	"unicode"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// collectKindVars scans a kind expression for unbound lowercase names
// (implicit kind variables), registers them in kindVars, and appends to params.
func collectKindVars(k syntax.KindExpr, kindVars map[string]bool, params *[]string) {
	if k == nil {
		return
	}
	switch ke := k.(type) {
	case *syntax.KindExprArrow:
		collectKindVars(ke.From, kindVars, params)
		collectKindVars(ke.To, kindVars, params)
	case *syntax.KindExprName:
		if len(ke.Name) > 0 && unicode.IsLower(rune(ke.Name[0])) && !kindVars[ke.Name] {
			kindVars[ke.Name] = true
			*params = append(*params, ke.Name)
		}
	}
}

// dictName returns the dictionary type/constructor name for a class.
func dictName(className string) string { return className + "$Dict" }

// ClassInfo stores elaborated class information.
// processClassDecl elaborates a class-like data declaration into:
// 1. A DataDecl for the dictionary type
// 2. Selector bindings for each method
func (ch *Checker) processClassDecl(d *syntax.DeclForm, parts formBodyParts, prog *ir.Program) {
	dn := dictName(d.Name)

	// Reject default method implementations (not yet supported).
	for _, f := range parts.Fields {
		if f.Default != nil {
			ch.addCodedError(diagnostic.ErrBadClass, f.S,
				"default method implementations are not yet supported in unified syntax")
		}
	}

	// Collect implicit kind variables from type parameter kind annotations.
	// e.g., class Functor (f: k -> Type) → kindParams = ["k"]
	var kindParams []string
	for _, p := range parts.Params {
		collectKindVars(p.Kind, ch.reg.kindVars, &kindParams)
	}

	// Collect type parameters with their kinds (kind vars now in scope).
	var tyParams []string
	var tyParamKinds []types.Kind
	for _, p := range parts.Params {
		tyParams = append(tyParams, p.Name)
		tyParamKinds = append(tyParamKinds, ch.resolveKindExpr(p.Kind))
	}

	// Process superclass constraints.
	var supers []SuperInfo
	var superFieldTypes []types.Type
	for _, sup := range parts.Supers {
		resolved := ch.resolveTypeExpr(sup)
		head, args := types.UnwindApp(resolved)
		if con, ok := head.(*types.TyCon); ok {
			supers = append(supers, SuperInfo{ClassName: con.Name, Args: args})
			superDictTy := ch.buildDictType(con.Name, args)
			superFieldTypes = append(superFieldTypes, superDictTy)
		}
	}

	// Process associated type declarations before method signatures,
	// so that associated type names are available during method type resolution.
	// In unified syntax, associated types don't repeat class params — inject
	// the class params so that the family is registered with the correct arity.
	var assocTypeNames []string
	for _, td := range parts.TypeDecls {
		assocTypeNames = append(assocTypeNames, td.Name)
		// Register as a type family with no equations yet (equations come from instances).
		var atParams []TFParam
		for _, p := range parts.Params {
			atParams = append(atParams, TFParam{Name: p.Name, Kind: ch.resolveKindExpr(p.Kind)})
		}
		resultKind := ch.resolveKindExpr(td.KindAnn)
		ch.reg.RegisterFamily(td.Name, &TypeFamilyInfo{
			Name:       td.Name,
			Params:     atParams,
			ResultKind: resultKind,
			IsAssoc:    true,
			ClassName:  d.Name,
		})
	}

	// Process method signatures (after associated types are registered).
	var methods []MethodInfo
	var methodFieldTypes []types.Type
	for _, f := range parts.Fields {
		methTy := ch.resolveTypeExpr(f.Type)
		methods = append(methods, MethodInfo{Name: f.Label, Type: methTy})
		methodFieldTypes = append(methodFieldTypes, methTy)
	}

	// Clean up kind variable scope.
	for _, kv := range kindParams {
		ch.reg.UnsetKindVar(kv)
	}

	// Store class info.
	info := &ClassInfo{
		Name:         d.Name,
		TyParams:     tyParams,
		TyParamKinds: tyParamKinds,
		KindParams:   kindParams,
		Supers:       supers,
		Methods:      methods,
		DictName:     dn,
		AssocTypes:   assocTypeNames,
	}
	ch.reg.RegisterClass(d.Name, info)

	// Build dictionary data declaration.
	allFieldTypes := append(superFieldTypes, methodFieldTypes...)

	// Register the dict type constructor kind.
	var dictKind types.Kind = types.KType{}
	for i := len(tyParamKinds) - 1; i >= 0; i-- {
		dictKind = &types.KArrow{From: tyParamKinds[i], To: dictKind}
	}
	ch.reg.RegisterTypeKind(dn, dictKind)

	// Build result type: DictTy a b c ...
	var resultType types.Type = &types.TyCon{Name: dn, S: d.S}
	for _, p := range tyParams {
		resultType = &types.TyApp{Fun: resultType, Arg: &types.TyVar{Name: p}, S: d.S}
	}

	// Build constructor type: field1 -> field2 -> ... -> DictTy a b...
	conType := resultType
	for i := len(allFieldTypes) - 1; i >= 0; i-- {
		conType = types.MkArrow(allFieldTypes[i], conType)
	}
	for i := len(tyParams) - 1; i >= 0; i-- {
		conType = types.MkForall(tyParams[i], tyParamKinds[i], conType)
	}
	// Wrap kind parameters as outermost foralls (kind-level quantification).
	for i := len(kindParams) - 1; i >= 0; i-- {
		conType = types.MkForall(kindParams[i], types.KSort{}, conType)
	}

	// Register constructor.
	ch.ctx.Push(&CtxVar{Name: dn, Type: conType, Module: ch.scope.CurrentModule()})
	dataInfo := &DataTypeInfo{Name: dn}
	dataInfo.Constructors = append(dataInfo.Constructors, ConstructorInfo{Name: dn, Arity: len(allFieldTypes)})
	ch.reg.RegisterConstructor(dn, conType, ch.scope.CurrentModule(), dataInfo)

	// Core DataDecl.
	coreDecl := ir.DataDecl{Name: dn, S: d.S}
	for i, p := range tyParams {
		coreDecl.TyParams = append(coreDecl.TyParams, ir.TyParam{Name: p, Kind: tyParamKinds[i]})
	}
	coreDecl.Cons = append(coreDecl.Cons, ir.ConDecl{Name: dn, Fields: allFieldTypes, S: d.S})
	prog.DataDecls = append(prog.DataDecls, coreDecl)

	// Generate selector bindings for each method.
	dict := dictLayout{resultType: resultType, fieldTypes: allFieldTypes}
	for i, m := range methods {
		ch.buildMethodSelector(info, m, i, dict, prog, d.S)
	}
}

// dictLayout groups the dictionary type representation for buildMethodSelector.
type dictLayout struct {
	resultType types.Type   // D a b c ...
	fieldTypes []types.Type // superclass dicts ++ method types
}

// buildMethodSelector generates a selector binding for a single class method.
// The selector pattern-matches on the dictionary constructor to extract the method
// at position fieldIdx (supers count + method index within methods).
func (ch *Checker) buildMethodSelector(cls *ClassInfo, m MethodInfo, methodIdx int, dict dictLayout, prog *ir.Program, s span.Span) {
	fieldIdx := len(cls.Supers) + methodIdx

	tyParamVars := make([]types.Type, len(cls.TyParams))
	for j, p := range cls.TyParams {
		tyParamVars[j] = &types.TyVar{Name: p}
	}
	entry := types.ConstraintEntry{ClassName: cls.Name, Args: tyParamVars, S: s}
	var selectorTy types.Type = types.MkEvidence([]types.ConstraintEntry{entry}, m.Type)
	for j := len(cls.TyParams) - 1; j >= 0; j-- {
		selectorTy = types.MkForall(cls.TyParams[j], cls.TyParamKinds[j], selectorTy)
	}
	for j := len(cls.KindParams) - 1; j >= 0; j-- {
		selectorTy = types.MkForall(cls.KindParams[j], types.KSort{}, selectorTy)
	}

	ch.ctx.Push(&CtxVar{Name: m.Name, Type: selectorTy, Module: ch.scope.CurrentModule()})

	selName := fmt.Sprintf("%s_%s_%d", prefixSel, m.Name, ch.fresh())
	var patArgs []ir.Pattern
	var resultExpr ir.Core
	for j := 0; j < len(dict.fieldTypes); j++ {
		argName := fmt.Sprintf("%s_%d", prefixField, j)
		patArgs = append(patArgs, &ir.PVar{Name: argName})
		if j == fieldIdx {
			resultExpr = &ir.Var{Name: argName, S: s}
		}
	}

	caseExpr := &ir.Case{
		Scrutinee: &ir.Var{Name: selName, S: s},
		Alts: []ir.Alt{{
			Pattern:   &ir.PCon{Con: cls.DictName, Args: patArgs, S: s},
			Body:      resultExpr,
			Generated: true,
			S:         s,
		}},
		S: s,
	}

	var selectorBody ir.Core = &ir.Lam{
		Param: selName, ParamType: dict.resultType, Body: caseExpr, Generated: true, S: s,
	}

	for j := len(cls.TyParams) - 1; j >= 0; j-- {
		selectorBody = &ir.TyLam{TyParam: cls.TyParams[j], Kind: cls.TyParamKinds[j], Body: selectorBody, S: s}
	}
	for j := len(cls.KindParams) - 1; j >= 0; j-- {
		selectorBody = &ir.TyLam{TyParam: cls.KindParams[j], Kind: types.KSort{}, Body: selectorBody, S: s}
	}

	prog.Bindings = append(prog.Bindings, ir.Binding{
		Name: m.Name,
		Type: selectorTy,
		Expr: selectorBody,
		S:    s,
	})
}

// buildDictType constructs the dictionary type for a class applied to arguments.
func (ch *Checker) buildDictType(className string, args []types.Type) types.Type {
	var ty types.Type = &types.TyCon{Name: dictName(className)}
	for _, a := range args {
		ty = &types.TyApp{Fun: ty, Arg: a}
	}
	return ty
}

// buildQuantifiedDictType constructs the evidence type for a quantified constraint.
// \ a. Eq a => Eq (f a) → \ a. Eq$Dict a -> Eq$Dict (f a)
func (ch *Checker) buildQuantifiedDictType(qc *types.QuantifiedConstraint) types.Type {
	headDictTy := ch.buildDictType(qc.Head.ClassName, qc.Head.Args)
	// Build function type from context dicts to head dict.
	var ty types.Type = headDictTy
	for i := len(qc.Context) - 1; i >= 0; i-- {
		ctxDictTy := ch.buildDictType(qc.Context[i].ClassName, qc.Context[i].Args)
		ty = types.MkArrow(ctxDictTy, ty)
	}
	// Wrap in foralls.
	for i := len(qc.Vars) - 1; i >= 0; i-- {
		ty = types.MkForall(qc.Vars[i].Name, qc.Vars[i].Kind, ty)
	}
	return ty
}
