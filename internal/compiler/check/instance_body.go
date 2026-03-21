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
func (ch *Checker) processInstanceBody(inst *InstanceInfo, prog *ir.Program) {
	classInfo := ch.reg.classes[inst.ClassName]

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
		methExpr, ok := inst.Methods[m.Name]
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
	dictConMod := ch.reg.conModules[classInfo.DictName]
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

	// Register binding.
	ch.ctx.Push(&CtxVar{Name: inst.DictBindName, Type: dictTy, Module: ch.scope.CurrentModule()})
	prog.Bindings = append(prog.Bindings, ir.Binding{
		Name: inst.DictBindName,
		Type: dictTy,
		Expr: dictExpr,
		S:    inst.S,
	})
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
func (ch *Checker) processAssocDataDef(add syntax.AssocDataDef, className string, instSpan span.Span) {
	fam, ok := ch.reg.families[add.Name]
	if !ok {
		ch.addCodedError(diagnostic.ErrBadInstance, instSpan,
			fmt.Sprintf("instance %s: associated data %s not declared in class %s",
				className, add.Name, className))
		return
	}
	if !fam.IsAssoc || fam.ClassName != className {
		ch.addCodedError(diagnostic.ErrBadInstance, instSpan,
			fmt.Sprintf("instance %s: %s is not an associated data of class %s",
				className, add.Name, className))
		return
	}
	if len(add.Patterns) != len(fam.Params) {
		ch.addCodedError(diagnostic.ErrTypeFamilyEquation, add.S,
			fmt.Sprintf("associated data %s expects %d argument(s), got %d",
				add.Name, len(fam.Params), len(add.Patterns)))
		return
	}

	// Resolve patterns.
	resolvedPats := make([]types.Type, len(add.Patterns))
	for i, pat := range add.Patterns {
		resolvedPats[i] = ch.resolveTypeExpr(pat)
	}

	// Build the mangled data type name: FamilyName$Pattern1$Pattern2
	mangledName := ch.mangledDataFamilyName(add.Name, resolvedPats)

	// Register the mangled data type as a type constructor.
	ch.reg.RegisterTypeKind(mangledName, types.KType{})

	// Build result type for the mangled data type.
	var mangledResultType types.Type = &types.TyCon{Name: mangledName, S: add.S}

	// Collect free vars from patterns (they become type params of the mangled data type).
	patVars := collectPatternVars(resolvedPats)

	// For each pattern var, wrap the mangled result type with a type application.
	for _, pv := range patVars {
		mangledResultType = &types.TyApp{
			Fun: mangledResultType,
			Arg: &types.TyVar{Name: pv, S: add.S},
			S:   add.S,
		}
		// Update the registered kind to accept this parameter.
		existingKind := ch.reg.typeKinds[mangledName]
		ch.reg.RegisterTypeKind(mangledName, &types.KArrow{From: types.KType{}, To: existingKind})
	}

	dataInfo := &DataTypeInfo{Name: mangledName}
	ch.reg.RegisterDataType(mangledName, dataInfo)

	// Register each constructor.
	for _, con := range add.Cons {
		var conType types.Type = mangledResultType
		var fieldTypes []types.Type
		for i := len(con.Fields) - 1; i >= 0; i-- {
			fieldTy := ch.resolveTypeExpr(con.Fields[i])
			fieldTypes = append([]types.Type{fieldTy}, fieldTypes...)
			conType = types.MkArrow(fieldTy, conType)
		}
		// Wrap in forall for pattern vars.
		for i := len(patVars) - 1; i >= 0; i-- {
			conType = types.MkForall(patVars[i], types.KType{}, conType)
		}
		// Guard against constructor name collision with existing constructors.
		if existing, dup := ch.reg.conTypes[con.Name]; dup {
			ch.addCodedError(diagnostic.ErrDuplicateDecl, con.S,
				fmt.Sprintf("data family instance %s: constructor %s conflicts with existing constructor (type: %s)",
					add.Name, con.Name, types.Pretty(existing)))
			continue
		}
		ch.ctx.Push(&CtxVar{Name: con.Name, Type: conType, Module: ch.scope.CurrentModule()})
		dataInfo.Constructors = append(dataInfo.Constructors, ConstructorInfo{Name: con.Name, Arity: len(fieldTypes)})
		ch.reg.RegisterConstructor(con.Name, conType, ch.scope.CurrentModule(), dataInfo)
	}

	// Add type family equation: Family patterns =: MangledType patVars
	fam.Equations = append(fam.Equations, tfEquation{
		Patterns: resolvedPats,
		RHS:      mangledResultType,
		S:        add.S,
	})
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
		liftKind := ch.kindOfType(&types.TyCon{Name: "Lift"})
		if liftKind != nil {
			if ka, ok := liftKind.(*types.KArrow); ok && ka.From.Equal(argKind) {
				lifted := &types.TyApp{Fun: &types.TyCon{Name: "Lift"}, Arg: typeArgs[i]}
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
	declMethods []syntax.InstMethod,
	methodExprs map[string]syntax.Expr,
	s span.Span,
) bool {
	hasMissing := false
	for _, m := range classInfo.Methods {
		if _, ok := methodExprs[m.Name]; !ok {
			ch.addCodedError(diagnostic.ErrMissingMethod, s,
				fmt.Sprintf("instance %s: missing method %s", className, m.Name))
			hasMissing = true
		}
	}
	if hasMissing {
		return false
	}

	classMethodSet := make(map[string]bool, len(classInfo.Methods))
	for _, m := range classInfo.Methods {
		classMethodSet[m.Name] = true
	}
	hasExtra := false
	for _, m := range declMethods {
		if !classMethodSet[m.Name] {
			ch.addCodedError(diagnostic.ErrBadInstance, s,
				fmt.Sprintf("instance %s: extra method %s not declared in class",
					className, m.Name))
			hasExtra = true
		}
	}
	return !hasExtra
}
