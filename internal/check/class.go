package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/pkg/types"
)

// ClassInfo stores elaborated class information.
type ClassInfo struct {
	Name        string
	TyParams    []string
	TyParamKinds []types.Kind
	Supers      []SuperInfo  // superclass constraints
	Methods     []MethodInfo // method signatures
	DictTyName  string       // e.g. "Eq$Dict"
	DictConName string       // e.g. "Eq$Dict"
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

// InstanceInfo stores elaborated instance information.
type InstanceInfo struct {
	ClassName    string
	TypeArgs     []types.Type   // concrete type arguments
	Context      []ConstraintInfo // instance context constraints
	Methods      map[string]syntax.Expr
	DictBindName string // e.g. "Eq$Bool" or "Eq$Maybe"
	S            span.Span
}

// ConstraintInfo represents a constraint in instance context.
type ConstraintInfo struct {
	ClassName string
	Args      []types.Type
}

// processClassDecl elaborates a class declaration into:
// 1. A DataDecl for the dictionary type
// 2. Selector bindings for each method
func (ch *Checker) processClassDecl(d *syntax.DeclClass, prog *core.Program) {
	dictTyName := d.Name + "$Dict"
	dictConName := d.Name + "$Dict"

	// Collect type parameters.
	var tyParams []string
	var tyParamKinds []types.Kind
	for _, p := range d.TyParams {
		tyParams = append(tyParams, p.Name)
		tyParamKinds = append(tyParamKinds, types.KType{})
	}

	// Process superclass constraints.
	var supers []SuperInfo
	var superFieldTypes []types.Type
	for _, sup := range d.Supers {
		resolved := ch.resolveTypeExpr(sup)
		head, args := types.UnwindApp(resolved)
		if con, ok := head.(*types.TyCon); ok {
			supers = append(supers, SuperInfo{ClassName: con.Name, Args: args})
			// The field type is the superclass dictionary: SuperClass$Dict args...
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

	// Build dictionary data declaration: data ClassName$Dict params = ClassName$Dict fields...
	allFieldTypes := append(superFieldTypes, methodFieldTypes...)

	// Register the dict type constructor kind.
	var dictKind types.Kind = types.KType{}
	for i := len(tyParams) - 1; i >= 0; i-- {
		dictKind = &types.KArrow{From: types.KType{}, To: dictKind}
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
	// Wrap in foralls.
	for i := len(tyParams) - 1; i >= 0; i-- {
		conType = types.MkForall(tyParams[i], types.KType{}, conType)
	}

	// Register constructor.
	ch.conTypes[dictConName] = conType
	ch.ctx.Push(&CtxVar{Name: dictConName, Type: conType})

	dataInfo := &DataTypeInfo{Name: dictTyName}
	dataInfo.Constructors = append(dataInfo.Constructors, ConInfo{Name: dictConName, Arity: len(allFieldTypes)})
	ch.conInfo[dictConName] = dataInfo

	// Core DataDecl.
	coreDecl := core.DataDecl{Name: dictTyName, S: d.S}
	for _, p := range tyParams {
		coreDecl.TyParams = append(coreDecl.TyParams, core.TyParam{Name: p, Kind: types.KType{}})
	}
	coreDecl.Cons = append(coreDecl.Cons, core.ConDecl{Name: dictConName, Fields: allFieldTypes, S: d.S})
	prog.DataDecls = append(prog.DataDecls, coreDecl)

	// Generate selector bindings for each method.
	// eq :: forall a. Eq$Dict a -> a -> a -> Bool
	// Elaborated as: \d -> case d of { Eq$Dict sel -> sel }
	for i, m := range methods {
		fieldIdx := len(supers) + i // skip superclass fields

		// Build selector type: forall params. ClassName params => methodType
		// Uses TyQual so that instantiate can resolve the dict from context.
		tyParamVars := make([]types.Type, len(tyParams))
		for j, p := range tyParams {
			tyParamVars[j] = &types.TyVar{Name: p}
		}
		var selectorTy types.Type = types.MkQual(d.Name, tyParamVars, m.Type)
		for j := len(tyParams) - 1; j >= 0; j-- {
			selectorTy = types.MkForall(tyParams[j], types.KType{}, selectorTy)
		}

		ch.ctx.Push(&CtxVar{Name: m.Name, Type: selectorTy})

		// Build selector Core: TyLam(params..., Lam($d, Case($d, {DictCon args... -> field})))
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

		// Wrap in TyLams.
		for j := len(tyParams) - 1; j >= 0; j-- {
			selectorBody = &core.TyLam{TyParam: tyParams[j], Kind: types.KType{}, Body: selectorBody, S: d.S}
		}

		prog.Bindings = append(prog.Bindings, core.Binding{
			Name: m.Name,
			Type: selectorTy,
			Expr: selectorBody,
			S:    d.S,
		})
	}
}

// processInstanceHeader validates and registers an instance.
func (ch *Checker) processInstanceHeader(d *syntax.DeclInstance) *InstanceInfo {
	classInfo, ok := ch.classes[d.ClassName]
	if !ok {
		ch.addCodedError(errs.ErrBadClass, d.S, fmt.Sprintf("unknown class: %s", d.ClassName))
		return nil
	}

	// Resolve type arguments.
	var typeArgs []types.Type
	for _, ta := range d.TypeArgs {
		typeArgs = append(typeArgs, ch.resolveTypeExpr(ta))
	}

	// Resolve context constraints.
	var context []ConstraintInfo
	for _, ctx := range d.Context {
		resolved := ch.resolveTypeExpr(ctx)
		head, args := types.UnwindApp(resolved)
		if con, ok := head.(*types.TyCon); ok {
			context = append(context, ConstraintInfo{ClassName: con.Name, Args: args})
		}
	}

	// Build dict binding name: ClassName$TypeName (simplified naming)
	dictName := ch.instanceDictName(d.ClassName, typeArgs)

	// Check for missing methods.
	methodExprs := make(map[string]syntax.Expr)
	for _, m := range d.Methods {
		methodExprs[m.Name] = m.Expr
	}
	for _, m := range classInfo.Methods {
		if _, ok := methodExprs[m.Name]; !ok {
			ch.addCodedError(errs.ErrMissingMethod, d.S,
				fmt.Sprintf("instance %s: missing method %s", d.ClassName, m.Name))
		}
	}

	inst := &InstanceInfo{
		ClassName:    d.ClassName,
		TypeArgs:     typeArgs,
		Context:      context,
		Methods:      methodExprs,
		DictBindName: dictName,
		S:            d.S,
	}
	ch.instances = append(ch.instances, inst)
	return inst
}

// processInstanceBody type-checks instance method implementations and generates the dictionary binding.
func (ch *Checker) processInstanceBody(inst *InstanceInfo, prog *core.Program) {
	classInfo := ch.classes[inst.ClassName]

	// Build substitution: class type params -> instance type args.
	subst := make(map[string]types.Type)
	for i, p := range classInfo.TyParams {
		if i < len(inst.TypeArgs) {
			subst[p] = inst.TypeArgs[i]
		}
	}

	// Build the dictionary constructor arguments.
	var dictArgs []core.Core
	var dictArgTypes []types.Type

	// Superclass dictionaries.
	for _, sup := range classInfo.Supers {
		superArgs := make([]types.Type, len(sup.Args))
		for j, a := range sup.Args {
			superArgs[j] = types.SubstMany(a, subst)
		}
		superDictExpr := ch.resolveInstance(sup.ClassName, superArgs, inst.S)
		dictArgs = append(dictArgs, superDictExpr)
		dictArgTypes = append(dictArgTypes, ch.buildDictType(sup.ClassName, superArgs))
	}

	// Method implementations.
	for _, m := range classInfo.Methods {
		methExpr, ok := inst.Methods[m.Name]
		if !ok {
			continue
		}
		expectedTy := types.SubstMany(m.Type, subst)
		methCore := ch.check(methExpr, expectedTy)
		dictArgs = append(dictArgs, methCore)
		dictArgTypes = append(dictArgTypes, expectedTy)
	}

	// Build the dictionary value: DictCon @types... arg1 arg2 ...
	var dictExpr core.Core = &core.Con{Name: classInfo.DictConName, S: inst.S}
	// Apply type arguments.
	for _, ta := range inst.TypeArgs {
		dictExpr = &core.TyApp{Expr: dictExpr, TyArg: ta, S: inst.S}
	}
	// Apply dictionary field arguments.
	for _, arg := range dictArgs {
		dictExpr = &core.App{Fun: dictExpr, Arg: arg, S: inst.S}
	}

	// Build dictionary type: DictTy TypeArg1 TypeArg2 ...
	dictTy := ch.buildDictType(inst.ClassName, inst.TypeArgs)

	// If instance has context, wrap in lambda(s) for each context constraint.
	if len(inst.Context) > 0 {
		for i := len(inst.Context) - 1; i >= 0; i-- {
			ctx := inst.Context[i]
			paramName := fmt.Sprintf("$d_%s_%d", ctx.ClassName, ch.fresh())
			paramTy := ch.buildDictType(ctx.ClassName, ctx.Args)
			dictExpr = &core.Lam{Param: paramName, ParamType: paramTy, Body: dictExpr, S: inst.S}
			dictTy = types.MkArrow(paramTy, dictTy)

			// Push dict param into context for resolveInstance within the body.
			ch.ctx.Push(&CtxVar{Name: paramName, Type: paramTy})
		}
		// Pop context dict params.
		for range inst.Context {
			ch.ctx.Pop()
		}
	}

	// Register binding.
	ch.ctx.Push(&CtxVar{Name: inst.DictBindName, Type: dictTy})
	prog.Bindings = append(prog.Bindings, core.Binding{
		Name: inst.DictBindName,
		Type: dictTy,
		Expr: dictExpr,
		S:    inst.S,
	})
}

// buildDictType constructs the dictionary type for a class applied to arguments.
// E.g., buildDictType("Eq", [Int]) → TyApp(TyCon("Eq$Dict"), Int)
func (ch *Checker) buildDictType(className string, args []types.Type) types.Type {
	dictTyName := className + "$Dict"
	var ty types.Type = &types.TyCon{Name: dictTyName}
	for _, a := range args {
		ty = &types.TyApp{Fun: ty, Arg: a}
	}
	return ty
}

// resolveInstance finds a dictionary expression for a given class constraint.
// Returns a Core expression that evaluates to the dictionary value.
func (ch *Checker) resolveInstance(className string, args []types.Type, s span.Span) core.Core {
	// 1. Search context for dictionary variables (from Eq a => parameters).
	for i := len(ch.ctx.entries) - 1; i >= 0; i-- {
		if v, ok := ch.ctx.entries[i].(*CtxVar); ok {
			if ch.matchesDictVar(v, className, args) {
				return &core.Var{Name: v.Name, S: s}
			}
		}
	}

	// 2. Search global instances.
	for _, inst := range ch.instances {
		if inst.ClassName != className {
			continue
		}
		if len(inst.TypeArgs) != len(args) {
			continue
		}
		// Try to match instance type args against requested args.
		matched := true
		for i := range args {
			if err := ch.unifier.Unify(inst.TypeArgs[i], args[i]); err != nil {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		var dictExpr core.Core = &core.Var{Name: inst.DictBindName, S: s}
		// If the instance has context, apply resolved context dictionaries.
		for _, ctx := range inst.Context {
			ctxArgs := make([]types.Type, len(ctx.Args))
			for j, a := range ctx.Args {
				ctxArgs[j] = ch.unifier.Zonk(a)
			}
			ctxDict := ch.resolveInstance(ctx.ClassName, ctxArgs, s)
			dictExpr = &core.App{Fun: dictExpr, Arg: ctxDict, S: s}
		}
		return dictExpr
	}

	ch.addCodedError(errs.ErrNoInstance, s,
		fmt.Sprintf("no instance for %s %s", className, ch.prettyTypeArgs(args)))
	return &core.Var{Name: "<no-instance>", S: s}
}

// matchesDictVar checks if a context variable is a dictionary for the given class and args.
func (ch *Checker) matchesDictVar(v *CtxVar, className string, args []types.Type) bool {
	dictTyName := className + "$Dict"
	ty := ch.unifier.Zonk(v.Type)
	head, tyArgs := types.UnwindApp(ty)
	if con, ok := head.(*types.TyCon); ok && con.Name == dictTyName {
		if len(tyArgs) != len(args) {
			return false
		}
		for i := range args {
			if err := ch.unifier.Unify(tyArgs[i], args[i]); err != nil {
				return false
			}
		}
		return true
	}
	return false
}

func (ch *Checker) prettyTypeArgs(args []types.Type) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = types.Pretty(ch.unifier.Zonk(a))
	}
	return strings.Join(parts, " ")
}

// instanceDictName generates a dictionary binding name for an instance.
func (ch *Checker) instanceDictName(className string, typeArgs []types.Type) string {
	if len(typeArgs) == 0 {
		return className + "$"
	}
	// Use head type constructor name.
	var parts []string
	for _, ta := range typeArgs {
		head, _ := types.UnwindApp(ta)
		switch h := head.(type) {
		case *types.TyCon:
			parts = append(parts, h.Name)
		case *types.TyVar:
			parts = append(parts, h.Name)
		default:
			parts = append(parts, "?")
		}
	}
	return className + "$" + strings.Join(parts, "$")
}
