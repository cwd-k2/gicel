package check

import (
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
		ch.addDiag(diagnostic.ErrNoInstance, inst.S,
			diagFmt{Format: "class %s not found for instance", Args: []any{inst.ClassName}})
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
		paramName := ch.freshDictName(ctx.ClassName)
		paramTy := ch.buildDictType(ctx.ClassName, ctx.Args)
		ctxParams = append(ctxParams, ctxParam{name: paramName, ty: paramTy})
		ch.ctx.Push(&CtxVar{Name: paramName, Type: paramTy, DictClassName: ctx.ClassName})
	}

	// Build the dictionary constructor arguments.
	var dictArgs []ir.Core
	var dictArgTypes []types.Type

	// Superclass dictionaries.
	ps := ch.typeOps.PrepareSubst(subst)
	for _, sup := range classInfo.Supers {
		superArgs := make([]types.Type, len(sup.Args))
		for j, a := range sup.Args {
			superArgs[j] = ps.Apply(a)
		}
		superDictExpr := ch.solver.ResolveInstance(sup.ClassName, superArgs, inst.S)
		dictArgs = append(dictArgs, superDictExpr)
		dictArgTypes = append(dictArgTypes, ch.buildDictType(sup.ClassName, superArgs))
	}

	// Build associated type family saturation map.
	// Associated type families in method types use implicit class parameters;
	// after substitution the TyCon remains bare. We convert them to TyFamilyApp
	// with the substituted class args so the family reducer can process them.
	familyArgs := ch.buildAssocFamilyArgs(classInfo, ps)

	// Method implementations.
	for _, m := range classInfo.Methods {
		methExpr, ok := methods[m.Name]
		if !ok {
			continue
		}
		expectedTy := ps.Apply(m.Type)
		if len(familyArgs) > 0 {
			expectedTy = ch.saturateAssocFamilies(expectedTy, familyArgs)
		}
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
	var dictExpr ir.Core = &ir.Con{Name: classInfo.DictName, Module: dictConMod, IsDict: true, S: inst.S}
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
				Body: dictExpr, Generated: ir.GenDict, S: inst.S,
			}
			dictTy = ch.typeOps.Arrow(ctxParams[i].ty, dictTy, span.Span{})
		}
	}

	// Register binding. Private instances are solver-invisible: they are only
	// accessible via explicit evidence injection (value => expr).
	ch.ctx.Push(&CtxVar{Name: inst.DictBindName, Type: dictTy, Module: ch.scope.CurrentModule(), IsSolverInvisible: inst.IsPrivate, DictClassName: inst.ClassName})
	prog.Bindings = append(prog.Bindings, ir.Binding{
		Name:      inst.DictBindName,
		Type:      dictTy,
		Expr:      dictExpr,
		Generated: ir.GenDict,
		S:         inst.S,
	})

	// If the instance has a user-visible name (impl name :: ...),
	// register an alias binding so the name can be used in value => expr.
	// IsSolverInvisible prevents automatic resolution from picking up this binding;
	// it is only used when explicitly referenced by name.
	if inst.IsNamed() {
		ch.ctx.Push(&CtxVar{Name: inst.UserName, Type: dictTy, Module: ch.scope.CurrentModule(), IsSolverInvisible: true})
		prog.Bindings = append(prog.Bindings, ir.Binding{
			Name: inst.UserName,
			Type: dictTy,
			Expr: &ir.Var{Name: inst.DictBindName, Module: ch.scope.CurrentModule(), S: inst.S},
			S:    inst.S,
		})
	}
}

// instanceDictName generates a dictionary binding name for an instance.
// Uses types.TypeListKey for collision-free structural encoding of type arguments.
func (ch *Checker) instanceDictName(className string, typeArgs []types.Type) string {
	return ch.typeOps.TypeListKey(className, '$', typeArgs)
}

// processAssocDataDef registers constructors for an associated data family definition
// and creates the type family equation mapping the family to its mangled data type.
//
// The field is a syntax.ImplField with IsData=true; patterns come from the
// instance's type arguments (already decomposed by the caller).
func (ch *Checker) processAssocDataDef(field syntax.ImplField, patterns []syntax.TypeExpr, className string, instSpan span.Span) {
	fam, ok := ch.reg.LookupFamily(field.Name)
	if !ok {
		ch.addDiag(diagnostic.ErrBadInstance, instSpan,
			diagFmt{Format: "instance %s: associated form %s not declared in class %s", Args: []any{className, field.Name, className}})
		return
	}
	if !fam.IsAssocFor(className) {
		ch.addDiag(diagnostic.ErrBadInstance, instSpan,
			diagFmt{Format: "instance %s: %s is not an associated form of class %s", Args: []any{className, field.Name, className}})
		return
	}
	if len(patterns) != len(fam.Params) {
		ch.addDiag(diagnostic.ErrTypeFamilyEquation, field.S,
			diagFmt{Format: "associated form %s expects %d argument(s), got %d",
				Args: []any{field.Name, len(fam.Params), len(patterns)}})
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
	ch.reg.RegisterTypeKind(mangledName, types.TypeOfTypes)

	// Build result type for the mangled data type.
	var mangledResultType types.Type = ch.typeOps.Con(mangledName, field.S)

	// Collect free vars from patterns (they become type params of the mangled data type).
	patVars := collectPatternVars(resolvedPats)

	// For each pattern var, wrap the mangled result type with a type application.
	for _, pv := range patVars {
		mangledResultType = ch.typeOps.App(
			mangledResultType,
			ch.typeOps.Var(pv, field.S),
			field.S,
		)
		// Update the registered kind to accept this parameter.
		existingKind, _ := ch.reg.LookupTypeKind(mangledName)
		ch.reg.RegisterTypeKind(mangledName, ch.typeOps.Arrow(types.TypeOfTypes, existingKind, span.Span{}))
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
		conType = ch.typeOps.Arrow(fieldTy, conType, span.Span{})
	}
	// Wrap in forall for pattern vars.
	for i := len(patVars) - 1; i >= 0; i-- {
		conType = ch.typeOps.Forall(patVars[i], types.TypeOfTypes, conType, span.Span{})
	}
	// Guard against constructor name collision with existing constructors.
	if existing, dup := ch.reg.LookupConType(conName); dup {
		ch.addDiag(diagnostic.ErrDuplicateDecl, field.S,
			diagFmt{Format: "form family instance %s: constructor %s conflicts with existing constructor (type: %s)", Args: []any{field.Name, conName, ch.typeOps.Pretty(existing)}})
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
// e.g. instance GIMonad g Maybe → instance GIMonad g (Lift Maybe)
func (ch *Checker) autoLiftTypeArgs(typeArgs []types.Type, paramKinds []types.Type) {
	for i := 0; i < len(typeArgs) && i < len(paramKinds); i++ {
		argKind := ch.kindOfType(typeArgs[i])
		paramKind := paramKinds[i]
		if argKind == nil {
			continue
		}
		if ch.withTrial(func() bool {
			return ch.unifier.Unify(argKind, paramKind) == nil
		}) {
			continue
		}
		liftKind := ch.kindOfType(ch.typeOps.Con("Lift", span.Span{}))
		if liftKind != nil {
			if ka, ok := liftKind.(*types.TyArrow); ok && ch.typeOps.Equal(ka.From, argKind) {
				lifted := ch.typeOps.App(ch.typeOps.Con("Lift", span.Span{}), typeArgs[i], span.Span{})
				liftedKind := ka.To
				if ch.withTrial(func() bool {
					return ch.unifier.Unify(liftedKind, paramKind) == nil
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
			ch.addDiag(diagnostic.ErrMissingMethod, s,
				diagFmt{Format: "instance %s: missing method %s", Args: []any{className, name}})
			hasMissing = true
		}
	}
	if hasMissing {
		return false
	}

	hasExtra := false
	for name := range methods {
		if !classMethodSet[name] {
			ch.addDiag(diagnostic.ErrBadInstance, s,
				diagFmt{Format: "instance %s: extra method %s not declared in class", Args: []any{className, name}})
			hasExtra = true
		}
	}
	return !hasExtra
}

// Associated type family saturation lives in instance_assoc_family.go.
