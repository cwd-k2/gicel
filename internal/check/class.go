package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/internal/types"
)

// ClassInfo stores elaborated class information.
type ClassInfo struct {
	Name         string
	TyParams     []string
	TyParamKinds []types.Kind
	Supers       []SuperInfo  // superclass constraints
	Methods      []MethodInfo // method signatures
	DictTyName   string       // e.g. "Eq$Dict"
	DictConName  string       // e.g. "Eq$Dict"
}

// SuperInfo describes a superclass constraint.
type SuperInfo struct {
	ClassName string
	Args      []types.Type
}

// MethodInfo describes a class method.
type MethodInfo struct {
	Name string
	Type types.Type // the method type (with the class type params free)
}

// processClassDecl elaborates a class declaration into:
// 1. A DataDecl for the dictionary type
// 2. Selector bindings for each method
func (ch *Checker) processClassDecl(d *syntax.DeclClass, prog *core.Program) {
	dictTyName := d.Name + "$Dict"
	dictConName := d.Name + "$Dict"

	// Collect type parameters with their kinds.
	var tyParams []string
	var tyParamKinds []types.Kind
	for _, p := range d.TyParams {
		tyParams = append(tyParams, p.Name)
		tyParamKinds = append(tyParamKinds, ch.resolveKindExpr(p.Kind))
	}

	// Process superclass constraints.
	var supers []SuperInfo
	var superFieldTypes []types.Type
	for _, sup := range d.Supers {
		resolved := ch.resolveTypeExpr(sup)
		head, args := types.UnwindApp(resolved)
		if con, ok := head.(*types.TyCon); ok {
			supers = append(supers, SuperInfo{ClassName: con.Name, Args: args})
			superDictTy := ch.buildDictType(con.Name, args)
			superFieldTypes = append(superFieldTypes, superDictTy)
		}
	}

	// Process method signatures.
	var methods []MethodInfo
	var methodFieldTypes []types.Type
	for _, m := range d.Methods {
		methTy := ch.resolveTypeExpr(m.Type)
		methods = append(methods, MethodInfo{Name: m.Name, Type: methTy})
		methodFieldTypes = append(methodFieldTypes, methTy)
	}

	// Store class info.
	info := &ClassInfo{
		Name:         d.Name,
		TyParams:     tyParams,
		TyParamKinds: tyParamKinds,
		Supers:       supers,
		Methods:      methods,
		DictTyName:   dictTyName,
		DictConName:  dictConName,
	}
	ch.classes[d.Name] = info

	// Build dictionary data declaration.
	allFieldTypes := append(superFieldTypes, methodFieldTypes...)

	// Register the dict type constructor kind.
	var dictKind types.Kind = types.KType{}
	for i := len(tyParamKinds) - 1; i >= 0; i-- {
		dictKind = &types.KArrow{From: tyParamKinds[i], To: dictKind}
	}
	ch.config.RegisteredTypes[dictTyName] = dictKind

	// Build result type: DictTy a b c ...
	var resultType types.Type = &types.TyCon{Name: dictTyName, S: d.S}
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

	// Register constructor.
	ch.conTypes[dictConName] = conType
	ch.ctx.Push(&CtxVar{Name: dictConName, Type: conType})

	dataInfo := &DataTypeInfo{Name: dictTyName}
	dataInfo.Constructors = append(dataInfo.Constructors, ConInfo{Name: dictConName, Arity: len(allFieldTypes)})
	ch.conInfo[dictConName] = dataInfo

	// Core DataDecl.
	coreDecl := core.DataDecl{Name: dictTyName, S: d.S}
	for i, p := range tyParams {
		coreDecl.TyParams = append(coreDecl.TyParams, core.TyParam{Name: p, Kind: tyParamKinds[i]})
	}
	coreDecl.Cons = append(coreDecl.Cons, core.ConDecl{Name: dictConName, Fields: allFieldTypes, S: d.S})
	prog.DataDecls = append(prog.DataDecls, coreDecl)

	// Generate selector bindings for each method.
	for i, m := range methods {
		fieldIdx := len(supers) + i

		tyParamVars := make([]types.Type, len(tyParams))
		for j, p := range tyParams {
			tyParamVars[j] = &types.TyVar{Name: p}
		}
		entry := types.ConstraintEntry{ClassName: d.Name, Args: tyParamVars, S: d.S}
		var selectorTy types.Type = types.MkEvidence([]types.ConstraintEntry{entry}, m.Type)
		for j := len(tyParams) - 1; j >= 0; j-- {
			selectorTy = types.MkForall(tyParams[j], tyParamKinds[j], selectorTy)
		}

		ch.ctx.Push(&CtxVar{Name: m.Name, Type: selectorTy})

		selName := fmt.Sprintf("$sel_%s_%d", m.Name, ch.fresh())
		var patArgs []core.Pattern
		var resultExpr core.Core
		for j := 0; j < len(allFieldTypes); j++ {
			argName := fmt.Sprintf("$f_%d", j)
			patArgs = append(patArgs, &core.PVar{Name: argName})
			if j == fieldIdx {
				resultExpr = &core.Var{Name: argName, S: d.S}
			}
		}

		caseExpr := &core.Case{
			Scrutinee: &core.Var{Name: selName, S: d.S},
			Alts: []core.Alt{{
				Pattern: &core.PCon{Con: dictConName, Args: patArgs, S: d.S},
				Body:    resultExpr,
				S:       d.S,
			}},
			S: d.S,
		}

		var selectorBody core.Core = &core.Lam{
			Param: selName, ParamType: resultType, Body: caseExpr, S: d.S,
		}

		for j := len(tyParams) - 1; j >= 0; j-- {
			selectorBody = &core.TyLam{TyParam: tyParams[j], Kind: tyParamKinds[j], Body: selectorBody, S: d.S}
		}

		prog.Bindings = append(prog.Bindings, core.Binding{
			Name: m.Name,
			Type: selectorTy,
			Expr: selectorBody,
			S:    d.S,
		})
	}
}

// buildDictType constructs the dictionary type for a class applied to arguments.
func (ch *Checker) buildDictType(className string, args []types.Type) types.Type {
	dictTyName := className + "$Dict"
	var ty types.Type = &types.TyCon{Name: dictTyName}
	for _, a := range args {
		ty = &types.TyApp{Fun: ty, Arg: a}
	}
	return ty
}
