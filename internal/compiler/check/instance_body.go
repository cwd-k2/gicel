package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processInstanceBody type-checks instance method implementations and generates the dictionary binding.
func (ch *Checker) processInstanceBody(inst *InstanceInfo, methods map[string]syntax.Expr, prog *ir.Program) {
	classInfo, ok := ch.reg.LookupClass(inst.ClassName)
	if !ok {
		ch.addCodedError(diagnostic.ErrNoInstance, inst.S,
			fmt.Sprintf("class %s not found for instance", inst.ClassName))
		return
	}

	// Build substitution: class type params -> instance type args.
	subst := make(map[string]types.Type)
	for i, p := range classInfo.TyParams {
		if i < len(inst.TypeArgs) {
			subst[p] = inst.TypeArgs[i]
		}
	}

	// Push instance context dict params into scope BEFORE checking methods.
	type ctxParam struct {
		name string
		ty   types.Type
	}
	var ctxParams []ctxParam
	for _, ctx := range inst.Context {
		paramName := fmt.Sprintf("%s_%s_%d", prefixDict, ctx.ClassName, ch.fresh())
		paramTy := ch.buildDictType(ctx.ClassName, ctx.Args)
		ctxParams = append(ctxParams, ctxParam{name: paramName, ty: paramTy})
		ch.ctx.Push(&CtxVar{Name: paramName, Type: paramTy})
	}

	// Build the dictionary constructor arguments.
	var dictArgs []ir.Core
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
		methExpr, ok := methods[m.Name]
		if !ok {
			continue
		}
		expectedTy := types.SubstMany(m.Type, subst)
		methCore := ch.check(methExpr, expectedTy)
		methCore = ch.resolveDeferredConstraints(methCore)
		dictArgs = append(dictArgs, methCore)
		dictArgTypes = append(dictArgTypes, expectedTy)
	}

	// Pop context dict params.
	for range inst.Context {
		ch.ctx.Pop()
	}

	// Build the dictionary value: DictCon @types... arg1 arg2 ...
	// The dict constructor comes from the module that defined the class.
	dictConMod, _ := ch.reg.LookupConModule(classInfo.DictName)
	var dictExpr ir.Core = &ir.Con{Name: classInfo.DictName, Module: dictConMod, S: inst.S}
	for _, ta := range inst.TypeArgs {
		dictExpr = &ir.TyApp{Expr: dictExpr, TyArg: ta, S: inst.S}
	}
	for _, arg := range dictArgs {
		dictExpr = &ir.App{Fun: dictExpr, Arg: arg, S: inst.S}
	}

	// Build dictionary type: DictTy TypeArg1 TypeArg2 ...
	dictTy := ch.buildDictType(inst.ClassName, inst.TypeArgs)

	// If instance has context, wrap in lambda(s) for each context constraint.
	if len(inst.Context) > 0 {
		for i := len(ctxParams) - 1; i >= 0; i-- {
			dictExpr = &ir.Lam{
				Param: ctxParams[i].name, ParamType: ctxParams[i].ty,
				Body: dictExpr, Generated: true, S: inst.S,
			}
			dictTy = types.MkArrow(ctxParams[i].ty, dictTy)
		}
	}

	// Register binding. Private instances are solver-invisible: they are only
	// accessible via explicit evidence injection (value => expr).
	ch.ctx.Push(&CtxVar{Name: inst.DictBindName, Type: dictTy, Module: ch.scope.CurrentModule(), SolverInvisible: inst.Private})
	prog.Bindings = append(prog.Bindings, ir.Binding{
		Name: inst.DictBindName,
		Type: dictTy,
		Expr: dictExpr,
		S:    inst.S,
	})

	// If the instance has a user-visible name (impl name :: ...),
	// register an alias binding so the name can be used in value => expr.
	// SolverInvisible prevents automatic resolution from picking up this binding;
	// it is only used when explicitly referenced by name.
	if inst.UserName != "" {
		ch.ctx.Push(&CtxVar{Name: inst.UserName, Type: dictTy, Module: ch.scope.CurrentModule(), SolverInvisible: true})
		prog.Bindings = append(prog.Bindings, ir.Binding{
			Name: inst.UserName,
			Type: dictTy,
			Expr: &ir.Var{Name: inst.DictBindName, Module: ch.scope.CurrentModule(), S: inst.S},
			S:    inst.S,
		})
	}
}

// instanceDictName generates a dictionary binding name for an instance.
// Uses types.TypeKey for collision-free structural encoding of type arguments.
func (ch *Checker) instanceDictName(className string, typeArgs []types.Type) string {
	if len(typeArgs) == 0 {
		return className + "$"
	}
	var parts []string
	for _, ta := range typeArgs {
		parts = append(parts, types.TypeKey(ta))
	}
	return className + "$" + strings.Join(parts, "$")
}

// processAssocDataDef registers constructors for an associated data family definition
// and creates the type family equation mapping the family to its mangled data type.
//
// The field is a syntax.ImplField with IsData=true; patterns come from the
// instance's type arguments (already decomposed by the caller).
func (ch *Checker) processAssocDataDef(field syntax.ImplField, patterns []syntax.TypeExpr, className string, instSpan span.Span) {
	fam, ok := ch.reg.LookupFamily(field.Name)
	if !ok {
		ch.addCodedError(diagnostic.ErrBadInstance, instSpan,
			fmt.Sprintf("instance %s: associated form %s not declared in class %s",
				className, field.Name, className))
		return
	}
	if !fam.IsAssoc || fam.ClassName != className {
		ch.addCodedError(diagnostic.ErrBadInstance, instSpan,
			fmt.Sprintf("instance %s: %s is not an associated form of class %s",
				className, field.Name, className))
		return
	}
	if len(patterns) != len(fam.Params) {
		ch.addCodedError(diagnostic.ErrTypeFamilyEquation, field.S,
			fmt.Sprintf("associated form %s expects %d argument(s), got %d",
				field.Name, len(fam.Params), len(patterns)))
		return
	}

	// Resolve patterns.
	resolvedPats := make([]types.Type, len(patterns))
	for i, pat := range patterns {
		resolvedPats[i] = ch.resolveTypeExpr(pat)
	}

	// Build the mangled data type name: FamilyName$Pattern1$Pattern2
	mangledName := ch.mangledDataFamilyName(field.Name, resolvedPats)

	// Register the mangled data type as a type constructor.
	ch.reg.RegisterTypeKind(mangledName, types.KType{})

	// Build result type for the mangled data type.
	var mangledResultType types.Type = types.ConAt(mangledName, field.S)

	// Collect free vars from patterns (they become type params of the mangled data type).
	patVars := collectPatternVars(resolvedPats)

	// For each pattern var, wrap the mangled result type with a type application.
	for _, pv := range patVars {
		mangledResultType = &types.TyApp{
			Fun: mangledResultType,
			Arg: &types.TyVar{Name: pv, S: field.S},
			S:   field.S,
		}
		// Update the registered kind to accept this parameter.
		existingKind, _ := ch.reg.LookupTypeKind(mangledName)
		ch.reg.RegisterTypeKind(mangledName, &types.KArrow{From: types.KType{}, To: existingKind})
	}

	dataInfo := &DataTypeInfo{Name: mangledName}
	ch.reg.RegisterDataType(mangledName, dataInfo)

	// Extract constructors directly from the TypeDef expression.
	// The RHS is a type application chain: e.g., "ListElem a" → TyExprApp(TyExprCon("ListElem"), TyExprVar("a"))
	conName, conFields := unwindImplDataCon(field.TypeDef)
	if conName == "" {
		return
	}

	// Register the constructor.
	var conType types.Type = mangledResultType
	var fieldTypes []types.Type
	for i := len(conFields) - 1; i >= 0; i-- {
		fieldTy := ch.resolveTypeExpr(conFields[i])
		fieldTypes = append([]types.Type{fieldTy}, fieldTypes...)
		conType = types.MkArrow(fieldTy, conType)
	}
	// Wrap in forall for pattern vars.
	for i := len(patVars) - 1; i >= 0; i-- {
		conType = types.MkForall(patVars[i], types.KType{}, conType)
	}
	// Guard against constructor name collision with existing constructors.
	if existing, dup := ch.reg.LookupConType(conName); dup {
		ch.addCodedError(diagnostic.ErrDuplicateDecl, field.S,
			fmt.Sprintf("form family instance %s: constructor %s conflicts with existing constructor (type: %s)",
				field.Name, conName, types.Pretty(existing)))
		return
	}
	ch.ctx.Push(&CtxVar{Name: conName, Type: conType, Module: ch.scope.CurrentModule()})
	dataInfo.Constructors = append(dataInfo.Constructors, ConstructorInfo{Name: conName, Arity: len(fieldTypes)})
	ch.reg.RegisterConstructor(conName, conType, ch.scope.CurrentModule(), dataInfo)

	// Add type family equation: Family patterns =: MangledType patVars
	fam.Equations = append(fam.Equations, tfEquation{
		Patterns: resolvedPats,
		RHS:      mangledResultType,
		S:        field.S,
	})
}

// unwindImplDataCon extracts a constructor name and field types from an impl
// data definition's RHS type expression. Returns ("", nil) if the head is
// not a type constructor.
func unwindImplDataCon(rhs syntax.TypeExpr) (string, []syntax.TypeExpr) {
	var args []syntax.TypeExpr
	t := rhs
	for {
		if app, ok := t.(*syntax.TyExprApp); ok {
			args = append([]syntax.TypeExpr{app.Arg}, args...)
			t = app.Fun
		} else {
			break
		}
	}
	if con, ok := t.(*syntax.TyExprCon); ok {
		return con.Name, args
	}
	return "", nil
}

// autoLiftTypeArgs applies automatic Lift wrapping to type arguments whose kinds
// don't match the expected class parameter kinds.
// e.g. instance IxMonad Maybe → instance IxMonad (Lift Maybe)
func (ch *Checker) autoLiftTypeArgs(typeArgs []types.Type, paramKinds []types.Kind) {
	for i := 0; i < len(typeArgs) && i < len(paramKinds); i++ {
		argKind := ch.kindOfType(typeArgs[i])
		paramKind := paramKinds[i]
		if argKind == nil {
			continue
		}
		if ch.withTrial(func() bool {
			return ch.unifier.UnifyKinds(argKind, paramKind) == nil
		}) {
			continue
		}
		liftKind := ch.kindOfType(types.Con("Lift"))
		if liftKind != nil {
			if ka, ok := liftKind.(*types.KArrow); ok && ka.From.Equal(argKind) {
				lifted := &types.TyApp{Fun: types.Con("Lift"), Arg: typeArgs[i]}
				liftedKind := ka.To
				if ch.withTrial(func() bool {
					return ch.unifier.UnifyKinds(liftedKind, paramKind) == nil
				}) {
					typeArgs[i] = lifted
				}
			}
		}
	}
}

// validateInstanceMethods checks that the instance defines exactly the methods
// declared in the class (no missing, no extra). Returns true if valid.
func (ch *Checker) validateInstanceMethods(
	className string,
	classInfo *ClassInfo,
	methods map[string]syntax.Expr,
	s span.Span,
) bool {
	classMethodSet := make(map[string]bool, len(classInfo.Methods))
	for _, m := range classInfo.Methods {
		classMethodSet[m.Name] = true
	}

	hasMissing := false
	for name := range classMethodSet {
		if _, ok := methods[name]; !ok {
			ch.addCodedError(diagnostic.ErrMissingMethod, s,
				fmt.Sprintf("instance %s: missing method %s", className, name))
			hasMissing = true
		}
	}
	if hasMissing {
		return false
	}

	hasExtra := false
	for name := range methods {
		if !classMethodSet[name] {
			ch.addCodedError(diagnostic.ErrBadInstance, s,
				fmt.Sprintf("instance %s: extra method %s not declared in class",
					className, name))
			hasExtra = true
		}
	}
	return !hasExtra
}
