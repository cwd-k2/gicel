package check

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
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

// classFromDict recovers the class name from a dictionary name.
func classFromDict(name string) string { return strings.TrimSuffix(name, "$Dict") }

// isDictName reports whether name is a dictionary type/constructor name.
func isDictName(name string) bool { return strings.HasSuffix(name, "$Dict") }

// ClassInfo stores elaborated class information.
type ClassInfo struct {
	Name         string
	TyParams     []string
	TyParamKinds []types.Kind
	KindParams   []string      // implicit kind variables (e.g., "k" in f : k -> Type)
	Supers       []SuperInfo   // superclass constraints
	Methods      []MethodInfo  // method signatures
	DictName     string        // e.g. "Eq$Dict" — used as both type and constructor name
	AssocTypes   []string      // associated type family names
	FunDeps      []ClassFunDep // functional dependencies: | a -> b
}

// ClassFunDep is an elaborated functional dependency on a class.
// From params determine To params: | a -> b means knowing a determines b.
type ClassFunDep struct {
	From []int // indices into TyParams
	To   []int // indices into TyParams
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
	dn := dictName(d.Name)

	// Collect implicit kind variables from type parameter kind annotations.
	// e.g., class Functor (f : k -> Type) → kindParams = ["k"]
	var kindParams []string
	for _, p := range d.TyParams {
		collectKindVars(p.Kind, ch.kindVars, &kindParams)
	}

	// Collect type parameters with their kinds (kind vars now in scope).
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

	// Clean up kind variable scope.
	for _, kv := range kindParams {
		delete(ch.kindVars, kv)
	}

	// Process associated type declarations.
	var assocTypeNames []string
	for _, atd := range d.AssocTypes {
		assocTypeNames = append(assocTypeNames, atd.Name)
		// Register as a type family with no equations yet (equations come from instances).
		var atParams []TFParam
		for _, p := range atd.Params {
			atParams = append(atParams, TFParam{Name: p.Name, Kind: ch.resolveKindExpr(p.Kind)})
		}
		resultKind := ch.resolveKindExpr(atd.ResultKind)
		var deps []tfDep
		for _, fd := range atd.Deps {
			deps = append(deps, tfDep{From: fd.From, To: fd.To})
		}
		ch.families[atd.Name] = &TypeFamilyInfo{
			Name:       atd.Name,
			Params:     atParams,
			ResultKind: resultKind,
			ResultName: atd.ResultName,
			Deps:       deps,
			IsAssoc:    true,
			ClassName:  d.Name,
		}
	}

	// Process associated data family declarations.
	// Data families are registered as type families (for Elem reduction)
	// AND as data type placeholders (for constructor resolution).
	for _, add := range d.AssocDataDecls {
		assocTypeNames = append(assocTypeNames, add.Name)
		var dfParams []TFParam
		for _, p := range add.Params {
			dfParams = append(dfParams, TFParam{Name: p.Name, Kind: ch.resolveKindExpr(p.Kind)})
		}
		resultKind := ch.resolveKindExpr(add.ResultKind)
		ch.families[add.Name] = &TypeFamilyInfo{
			Name:       add.Name,
			Params:     dfParams,
			ResultKind: resultKind,
			IsAssoc:    true,
			ClassName:  d.Name,
		}
		// Register the data family name as a type constructor.
		var dfKind types.Kind = resultKind
		for i := len(dfParams) - 1; i >= 0; i-- {
			dfKind = &types.KArrow{From: dfParams[i].Kind, To: dfKind}
		}
		ch.config.RegisteredTypes[add.Name] = dfKind
	}

	// Elaborate functional dependencies: convert param names to indices.
	paramIndex := make(map[string]int, len(tyParams))
	for i, p := range tyParams {
		paramIndex[p] = i
	}
	var funDeps []ClassFunDep
	for _, fd := range d.FunDeps {
		fromIdx, ok := paramIndex[fd.From]
		if !ok {
			ch.addCodedError(errs.ErrBadClass, d.S,
				fmt.Sprintf("class %s: functional dependency references unknown parameter %s", d.Name, fd.From))
			continue
		}
		var toIdxs []int
		for _, to := range fd.To {
			toIdx, ok := paramIndex[to]
			if !ok {
				ch.addCodedError(errs.ErrBadClass, d.S,
					fmt.Sprintf("class %s: functional dependency references unknown parameter %s", d.Name, to))
				continue
			}
			toIdxs = append(toIdxs, toIdx)
		}
		funDeps = append(funDeps, ClassFunDep{From: []int{fromIdx}, To: toIdxs})
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
		FunDeps:      funDeps,
	}
	ch.classes[d.Name] = info

	// Build dictionary data declaration.
	allFieldTypes := append(superFieldTypes, methodFieldTypes...)

	// Register the dict type constructor kind.
	var dictKind types.Kind = types.KType{}
	for i := len(tyParamKinds) - 1; i >= 0; i-- {
		dictKind = &types.KArrow{From: tyParamKinds[i], To: dictKind}
	}
	ch.config.RegisteredTypes[dn] = dictKind

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
	ch.conTypes[dn] = conType
	ch.ctx.Push(&CtxVar{Name: dn, Type: conType})

	dataInfo := &DataTypeInfo{Name: dn}
	dataInfo.Constructors = append(dataInfo.Constructors, ConInfo{Name: dn, Arity: len(allFieldTypes)})
	ch.conInfo[dn] = dataInfo

	// Core DataDecl.
	coreDecl := core.DataDecl{Name: dn, S: d.S}
	for i, p := range tyParams {
		coreDecl.TyParams = append(coreDecl.TyParams, core.TyParam{Name: p, Kind: tyParamKinds[i]})
	}
	coreDecl.Cons = append(coreDecl.Cons, core.ConDecl{Name: dn, Fields: allFieldTypes, S: d.S})
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
		// Wrap kind parameters as outermost foralls.
		for j := len(kindParams) - 1; j >= 0; j-- {
			selectorTy = types.MkForall(kindParams[j], types.KSort{}, selectorTy)
		}

		ch.ctx.Push(&CtxVar{Name: m.Name, Type: selectorTy})

		selName := fmt.Sprintf("%s_%s_%d", prefixSel, m.Name, ch.fresh())
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
				Pattern: &core.PCon{Con: dn, Args: patArgs, S: d.S},
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
		// Wrap kind parameters as outermost TyLams.
		for j := len(kindParams) - 1; j >= 0; j-- {
			selectorBody = &core.TyLam{TyParam: kindParams[j], Kind: types.KSort{}, Body: selectorBody, S: d.S}
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
