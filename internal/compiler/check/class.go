package check

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// collectKindVars scans a kind annotation (represented as TypeExpr) for
// unbound lowercase names (implicit kind variables), registers them in
// kindVars, and appends to params.
func collectKindVars(k syntax.TypeExpr, kindVars map[string]bool, params *[]string) {
	if k == nil {
		return
	}
	switch ke := k.(type) {
	case *syntax.TyExprArrow:
		collectKindVars(ke.From, kindVars, params)
		collectKindVars(ke.To, kindVars, params)
	case *syntax.TyExprVar:
		if len(ke.Name) > 0 && unicode.IsLower(rune(ke.Name[0])) && !kindVars[ke.Name] {
			kindVars[ke.Name] = true
			*params = append(*params, ke.Name)
		}
	case *syntax.TyExprParen:
		collectKindVars(ke.Inner, kindVars, params)
	}
}

// processClassLikeForm elaborates a class-like form declaration into:
// 1. A DataDecl for the dictionary type
// 2. Selector bindings for each method
func (ch *Checker) processClassLikeForm(d *syntax.DeclForm, parts formBodyParts, prog *ir.Program) {
	dn := env.DictName(d.Name)

	// Reject default method implementations (not yet supported).
	for _, f := range parts.Fields {
		if f.Default != nil {
			ch.addDiag(diagnostic.ErrBadClass, f.S,
				diagMsg("default method implementations are not yet supported in unified syntax"))
		}
	}

	// Phase 1: collect type/kind parameters.
	kindParams, tyParams, tyParamKinds := ch.collectClassParams(parts)

	// Phase 2: resolve superclass constraints.
	supers, superFieldTypes := ch.resolveSupers(parts)

	// Phase 3: register associated type families.
	assocTypeNames := ch.registerAssocTypes(parts, d.Name)

	// Phase 4: resolve method signatures.
	methods, methodFieldTypes := ch.resolveMethods(parts)

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
	var dictKind types.Type = types.TypeOfTypes
	for i := len(tyParamKinds) - 1; i >= 0; i-- {
		dictKind = &types.TyArrow{From: tyParamKinds[i], To: dictKind}
	}
	ch.reg.RegisterTypeKind(dn, dictKind)

	// Build result type: DictTy a b c ...
	var resultType types.Type = types.ConAt(dn, d.S)
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
		conType = types.MkForall(kindParams[i], types.SortZero, conType)
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
	dict := dictLayout{resultType: resultType, fieldTypes: allFieldTypes, prog: prog, s: d.S}
	for i, m := range methods {
		ch.buildMethodSelector(info, m, i, dict)
	}
}

// collectClassParams collects implicit kind variables from type parameter
// annotations and resolves type parameters with their kinds.
func (ch *Checker) collectClassParams(parts formBodyParts) (kindParams, tyParams []string, tyParamKinds []types.Type) {
	// Collect implicit kind variables from type parameter kind annotations.
	// e.g., class Functor (f: k -> Type) -> kindParams = ["k"]
	for _, p := range parts.Params {
		collectKindVars(p.Kind, ch.reg.kindVars, &kindParams)
	}

	// Collect type parameters with their kinds (kind vars now in scope).
	for _, p := range parts.Params {
		tyParams = append(tyParams, p.Name)
		tyParamKinds = append(tyParamKinds, ch.resolveKindExpr(p.Kind))
	}
	return
}

// resolveSupers resolves superclass constraints into SuperInfo and their
// corresponding dictionary field types.
func (ch *Checker) resolveSupers(parts formBodyParts) ([]SuperInfo, []types.Type) {
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
	return supers, superFieldTypes
}

// registerAssocTypes registers associated type family declarations, injecting
// class parameters so each family has the correct arity. Must be called before
// resolveMethods so that associated type names are in scope.
func (ch *Checker) registerAssocTypes(parts formBodyParts, className string) []string {
	var assocTypeNames []string
	for _, td := range parts.TypeDecls {
		assocTypeNames = append(assocTypeNames, td.Name)
		// Register as a type family with no equations yet (equations come from instances).
		var atParams []TFParam
		for _, p := range parts.Params {
			atParams = append(atParams, TFParam{Name: p.Name, Kind: ch.resolveKindExpr(p.Kind)})
		}
		resultKind := ch.resolveKindExpr(td.KindAnn)
		if err := ch.reg.RegisterFamily(td.Name, &TypeFamilyInfo{
			Name:       td.Name,
			Params:     atParams,
			ResultKind: resultKind,
			IsAssoc:    true,
			ClassName:  className,
		}); err != nil {
			ch.addDiag(diagnostic.ErrTypeFamilyEquation, td.S, diagWithErr{Context: "associated type family registration", Err: err})
		}
	}
	return assocTypeNames
}

// resolveMethods resolves method signatures into MethodInfo and their
// corresponding field types for the dictionary constructor.
func (ch *Checker) resolveMethods(parts formBodyParts) ([]MethodInfo, []types.Type) {
	var methods []MethodInfo
	var methodFieldTypes []types.Type
	for _, f := range parts.Fields {
		methTy := ch.resolveTypeExpr(f.Type)
		methods = append(methods, MethodInfo{Name: f.Label, Type: methTy})
		methodFieldTypes = append(methodFieldTypes, methTy)
	}
	return methods, methodFieldTypes
}

// dictLayout groups the dictionary type representation and IR context
// for buildMethodSelector.
type dictLayout struct {
	resultType types.Type   // D a b c ...
	fieldTypes []types.Type // superclass dicts ++ method types
	prog       *ir.Program
	s          span.Span
}

// buildMethodSelector generates a selector binding for a single class method.
// The selector pattern-matches on the dictionary constructor to extract the method
// at position fieldIdx (supers count + method index within methods).
func (ch *Checker) buildMethodSelector(cls *ClassInfo, m MethodInfo, methodIdx int, dict dictLayout) {
	fieldIdx := len(cls.Supers) + methodIdx
	s := dict.s

	tyParamVars := make([]types.Type, len(cls.TyParams))
	for j, p := range cls.TyParams {
		tyParamVars[j] = &types.TyVar{Name: p}
	}
	entry := &types.ClassEntry{ClassName: cls.Name, Args: tyParamVars, S: s}
	var selectorTy types.Type = types.MkEvidence([]types.ConstraintEntry{entry}, m.Type)
	for j := len(cls.TyParams) - 1; j >= 0; j-- {
		selectorTy = types.MkForall(cls.TyParams[j], cls.TyParamKinds[j], selectorTy)
	}
	for j := len(cls.KindParams) - 1; j >= 0; j-- {
		selectorTy = types.MkForall(cls.KindParams[j], types.SortZero, selectorTy)
	}

	ch.ctx.Push(&CtxVar{Name: m.Name, Type: selectorTy, Module: ch.scope.CurrentModule()})

	selName := ch.freshName(prefixSel + "_" + m.Name)
	var patArgs []ir.Pattern
	var resultExpr ir.Core
	for j := 0; j < len(dict.fieldTypes); j++ {
		argName := fmt.Sprintf("%s_%d", prefixField, j)
		patArgs = append(patArgs, &ir.PVar{Name: argName, Generated: ir.GenDictExtract})
		if j == fieldIdx {
			resultExpr = &ir.Var{Name: argName, S: s}
		}
	}

	caseExpr := &ir.Case{
		Scrutinee: &ir.Var{Name: selName, S: s},
		Alts: []ir.Alt{{
			Pattern:   &ir.PCon{Con: cls.DictName, Args: patArgs, S: s},
			Body:      resultExpr,
			Generated: ir.GenDictExtract,
			S:         s,
		}},
		S: s,
	}

	var selectorBody ir.Core = &ir.Lam{
		Param: selName, ParamType: dict.resultType, Body: caseExpr, Generated: ir.GenDictExtract, S: s,
	}

	for j := len(cls.TyParams) - 1; j >= 0; j-- {
		selectorBody = &ir.TyLam{TyParam: cls.TyParams[j], Kind: cls.TyParamKinds[j], Body: selectorBody, S: s}
	}
	for j := len(cls.KindParams) - 1; j >= 0; j-- {
		selectorBody = &ir.TyLam{TyParam: cls.KindParams[j], Kind: types.SortZero, Body: selectorBody, S: s}
	}

	dict.prog.Bindings = append(dict.prog.Bindings, ir.Binding{
		Name: m.Name,
		Type: selectorTy,
		Expr: selectorBody,
		S:    s,
	})
}

// buildDictType constructs the dictionary type for a class applied to arguments.
func (ch *Checker) buildDictType(className string, args []types.Type) types.Type {
	var ty types.Type = types.Con(env.DictName(className))
	for _, a := range args {
		ty = &types.TyApp{Fun: ty, Arg: a}
	}
	return ty
}

// validateSuperclassGraph checks for cyclic superclass constraints using
// DFS three-color marking. A cycle exists when class A requires B as a
// superclass and B (directly or transitively) requires A. Such cycles would
// cause infinite recursion during dictionary construction at runtime.
// Returns true if any cycle was found.
func (ch *Checker) validateSuperclassGraph() bool {
	type color int
	const (
		white color = iota
		gray
		black
	)

	classes := ch.reg.AllClasses()
	colors := make(map[string]color, len(classes))
	for name := range classes {
		colors[name] = white
	}

	var path []string

	var visit func(name string) bool
	visit = func(name string) bool {
		switch colors[name] {
		case black:
			return false
		case gray:
			cycleStart := 0
			for i, p := range path {
				if p == name {
					cycleStart = i
					break
				}
			}
			cycle := append(path[cycleStart:], name)
			ch.addDiag(diagnostic.ErrCyclicSuperclass, span.Span{},
				diagMsg("cyclic superclass constraint: "+strings.Join(cycle, " -> ")))
			return true
		}

		colors[name] = gray
		path = append(path, name)

		info, ok := classes[name]
		if ok {
			for _, sup := range info.Supers {
				if _, isClass := classes[sup.ClassName]; isClass {
					if visit(sup.ClassName) {
						return true
					}
				}
			}
		}

		path = path[:len(path)-1]
		colors[name] = black
		// Compute transitive superclass closure (only for classes that
		// don't already have one — imported classes carry their closure
		// from the originating module's compilation and must not be
		// mutated, as the ClassInfo may be shared across goroutines via
		// the module cache).
		if info, ok := classes[name]; ok && info.SuperClosure == nil {
			closure := make(map[string]bool, len(info.Supers))
			for _, sup := range info.Supers {
				closure[sup.ClassName] = true
				if supInfo, ok := classes[sup.ClassName]; ok && supInfo.SuperClosure != nil {
					for k := range supInfo.SuperClosure {
						closure[k] = true
					}
				}
			}
			info.SuperClosure = closure
		}
		return false
	}

	hasCycle := false
	for name := range classes {
		if colors[name] == white {
			if visit(name) {
				hasCycle = true
			}
		}
	}
	return hasCycle
}

// buildQuantifiedDictType constructs the evidence type for a quantified constraint.
// \ a. Eq a => Eq (f a) → \ a. Eq$Dict a -> Eq$Dict (f a)
//
// Only class-headed context entries yield runtime dictionaries; equality or
// other non-class premises in a quantified constraint do not contribute
// arguments to the dict function type, so they are skipped here.
func (ch *Checker) buildQuantifiedDictType(qc *types.QuantifiedConstraint) types.Type {
	headDictTy := ch.buildDictType(qc.Head.ClassName, qc.Head.Args)
	// Build function type from context dicts to head dict.
	var ty types.Type = headDictTy
	for i := len(qc.Context) - 1; i >= 0; i-- {
		ctxCls, ok := qc.Context[i].(*types.ClassEntry)
		if !ok {
			continue
		}
		ctxDictTy := ch.buildDictType(ctxCls.ClassName, ctxCls.Args)
		ty = types.MkArrow(ctxDictTy, ty)
	}
	// Wrap in foralls.
	for i := len(qc.Vars) - 1; i >= 0; i-- {
		ty = types.MkForall(qc.Vars[i].Name, qc.Vars[i].Kind, ty)
	}
	return ty
}
