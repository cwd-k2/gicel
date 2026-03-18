package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// InstanceInfo stores elaborated instance information.
type InstanceInfo struct {
	ClassName    string
	TypeArgs     []types.Type     // concrete type arguments
	Context      []ConstraintInfo // instance context constraints
	Methods      map[string]syntax.Expr
	DictBindName string // e.g. "Eq$Bool" or "Eq$Maybe"
	Module       string // source module that defined this instance
	S            span.Span
}

// ConstraintInfo represents a constraint in instance context.
type ConstraintInfo struct {
	ClassName string
	Args      []types.Type
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

	ch.autoLiftTypeArgs(typeArgs, classInfo.TyParamKinds)

	// Resolve context constraints.
	var context []ConstraintInfo
	for _, ctx := range d.Context {
		resolved := ch.resolveTypeExpr(ctx)
		head, args := types.UnwindApp(resolved)
		if con, ok := head.(*types.TyCon); ok {
			context = append(context, ConstraintInfo{ClassName: con.Name, Args: args})
		}
	}

	// Arity check: number of type arguments must match class parameter count.
	if len(typeArgs) != len(classInfo.TyParams) {
		ch.addCodedError(errs.ErrBadInstance, d.S,
			fmt.Sprintf("instance %s: expected %d type argument(s), got %d",
				d.ClassName, len(classInfo.TyParams), len(typeArgs)))
		return nil
	}

	// Context well-formedness: each constraint in the instance context must reference a known class.
	for _, ctx := range context {
		if _, ok := ch.classes[ctx.ClassName]; !ok {
			ch.addCodedError(errs.ErrBadInstance, d.S,
				fmt.Sprintf("instance %s: context references unknown class %s",
					d.ClassName, ctx.ClassName))
			return nil
		}
	}

	// Self-cycle detection: instance context must not require itself.
	// e.g., `instance Eq a => Eq a` would cause infinite recursion.
	for _, ctx := range context {
		if ctx.ClassName == d.ClassName && len(ctx.Args) == len(typeArgs) {
			selfCycle := true
			for i := range ctx.Args {
				if !types.Equal(ctx.Args[i], typeArgs[i]) {
					selfCycle = false
					break
				}
			}
			if selfCycle {
				ch.addCodedError(errs.ErrBadInstance, d.S,
					fmt.Sprintf("instance %s: self-referential context (instance requires itself)",
						d.ClassName))
				return nil
			}
		}
	}

	// Build dict binding name: ClassName$TypeName (simplified naming)
	dictName := ch.instanceDictName(d.ClassName, typeArgs)

	// Validate method set (missing / extra).
	methodExprs := make(map[string]syntax.Expr)
	for _, m := range d.Methods {
		methodExprs[m.Name] = m.Expr
	}
	if !ch.validateInstanceMethods(d.ClassName, classInfo, d.Methods, methodExprs, d.S) {
		return nil
	}

	inst := &InstanceInfo{
		ClassName:    d.ClassName,
		TypeArgs:     typeArgs,
		Context:      context,
		Methods:      methodExprs,
		DictBindName: dictName,
		Module:       ch.currentModule,
		S:            d.S,
	}

	// Overlap check: verify no existing local instance for this class matches the same types.
	// Imported instances are excluded: user source is allowed to shadow module instances.
	for _, existing := range ch.instancesByClass[d.ClassName] {
		if existing == inst {
			continue // same pointer (re-exported via module)
		}
		if ch.importedInstances[existing] {
			continue // imported from a module; shadowing is allowed
		}
		if len(existing.TypeArgs) != len(typeArgs) {
			continue
		}
		if ch.instancesOverlap(existing, inst) {
			ch.addCodedError(errs.ErrOverlap, d.S,
				fmt.Sprintf("overlapping instances for class %s: %s and %s",
					d.ClassName, existing.DictBindName, dictName))
			return nil
		}
	}

	// Process associated type definitions: convert to equations and append
	// to the corresponding TypeFamilyInfo registered during class processing.
	for _, atd := range d.AssocTypeDefs {
		fam, ok := ch.families[atd.Name]
		if !ok {
			ch.addCodedError(errs.ErrBadInstance, d.S,
				fmt.Sprintf("instance %s: associated type %s not declared in class %s",
					d.ClassName, atd.Name, d.ClassName))
			continue
		}
		if !fam.IsAssoc || fam.ClassName != d.ClassName {
			ch.addCodedError(errs.ErrBadInstance, d.S,
				fmt.Sprintf("instance %s: %s is not an associated type of class %s",
					d.ClassName, atd.Name, d.ClassName))
			continue
		}
		// Arity check.
		if len(atd.Patterns) != len(fam.Params) {
			ch.addCodedError(errs.ErrTypeFamilyEquation, atd.S,
				fmt.Sprintf("associated type %s expects %d argument(s), got %d",
					atd.Name, len(fam.Params), len(atd.Patterns)))
			continue
		}
		// Resolve patterns and RHS.
		resolvedPats := make([]types.Type, len(atd.Patterns))
		for i, pat := range atd.Patterns {
			resolvedPats[i] = ch.resolveTypeExpr(pat)
		}
		resolvedRHS := ch.resolveTypeExpr(atd.RHS)
		fam.Equations = append(fam.Equations, tfEquation{
			Patterns: resolvedPats,
			RHS:      resolvedRHS,
			S:        atd.S,
		})
	}

	// Process associated data family definitions: register constructors
	// and create type family equations mapping the family to its mangled data type.
	for _, add := range d.AssocDataDefs {
		ch.processAssocDataDef(add, d.ClassName, d.S)
	}

	ch.instances = append(ch.instances, inst)
	ch.instancesByClass[inst.ClassName] = append(ch.instancesByClass[inst.ClassName], inst)
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
	dictConMod := ch.conModules[classInfo.DictName]
	var dictExpr core.Core = &core.Con{Name: classInfo.DictName, Module: dictConMod, S: inst.S}
	for _, ta := range inst.TypeArgs {
		dictExpr = &core.TyApp{Expr: dictExpr, TyArg: ta, S: inst.S}
	}
	for _, arg := range dictArgs {
		dictExpr = &core.App{Fun: dictExpr, Arg: arg, S: inst.S}
	}

	// Build dictionary type: DictTy TypeArg1 TypeArg2 ...
	dictTy := ch.buildDictType(inst.ClassName, inst.TypeArgs)

	// If instance has context, wrap in lambda(s) for each context constraint.
	if len(inst.Context) > 0 {
		for i := len(ctxParams) - 1; i >= 0; i-- {
			dictExpr = &core.Lam{
				Param: ctxParams[i].name, ParamType: ctxParams[i].ty,
				Body: dictExpr, S: inst.S,
			}
			dictTy = types.MkArrow(ctxParams[i].ty, dictTy)
		}
	}

	// Register binding.
	ch.ctx.Push(&CtxVar{Name: inst.DictBindName, Type: dictTy, Module: ch.currentModule})
	prog.Bindings = append(prog.Bindings, core.Binding{
		Name: inst.DictBindName,
		Type: dictTy,
		Expr: dictExpr,
		S:    inst.S,
	})
}

// instanceDictName generates a dictionary binding name for an instance.
func (ch *Checker) instanceDictName(className string, typeArgs []types.Type) string {
	if len(typeArgs) == 0 {
		return className + "$"
	}
	var parts []string
	for _, ta := range typeArgs {
		parts = append(parts, typeNameForDict(ta))
	}
	return className + "$" + strings.Join(parts, "$")
}

// typeNameForDict recursively constructs a name component from a type,
// including concrete type constructor arguments (e.g., Lift Maybe → "Lift$Maybe")
// but omitting type variables (parametric instances share one dictionary function).
func typeNameForDict(ty types.Type) string {
	head, args := types.UnwindApp(ty)
	var parts []string
	switch h := head.(type) {
	case *types.TyCon:
		parts = append(parts, h.Name)
	case *types.TyVar, *types.TySkolem, *types.TyMeta:
		// Type variables are omitted from dict names.
	case *types.TyEvidenceRow:
		// Encode row structure into the name to distinguish e.g. {} from {_1, _2}.
		parts = append(parts, evidenceRowName(h))
	default:
		parts = append(parts, "?")
	}
	for _, a := range args {
		if sub := typeNameForDict(a); sub != "" {
			parts = append(parts, sub)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "$")
}

// evidenceRowName produces a stable name component for an evidence row.
// Empty row → "R0", row with labels → "R2$_1$_2", etc.
func evidenceRowName(row *types.TyEvidenceRow) string {
	switch entries := row.Entries.(type) {
	case *types.CapabilityEntries:
		if len(entries.Fields) == 0 {
			return "R0"
		}
		parts := []string{fmt.Sprintf("R%d", len(entries.Fields))}
		for _, f := range entries.Fields {
			parts = append(parts, f.Label)
		}
		return strings.Join(parts, "$")
	case *types.ConstraintEntries:
		if len(entries.Entries) == 0 {
			return "C0"
		}
		parts := []string{fmt.Sprintf("C%d", len(entries.Entries))}
		for _, e := range entries.Entries {
			parts = append(parts, e.ClassName)
		}
		return strings.Join(parts, "$")
	default:
		return "?"
	}
}

// instancesOverlap checks if two instances can match the same type arguments
// using trial unification with fresh metavariables. The unifier state is saved
// and restored so no side effects persist.
func (ch *Checker) instancesOverlap(a, b *InstanceInfo) bool {
	saved := ch.saveUnifierState()
	defer ch.restoreUnifierState(saved)

	substA := ch.freshInstanceSubst(a)
	substB := ch.freshInstanceSubst(b)
	for i := range a.TypeArgs {
		argA := types.SubstMany(a.TypeArgs[i], substA)
		argB := types.SubstMany(b.TypeArgs[i], substB)
		if err := ch.unifier.Unify(argA, argB); err != nil {
			return false
		}
	}
	return true
}

// processAssocDataDef registers constructors for an associated data family definition
// and creates the type family equation mapping the family to its mangled data type.
func (ch *Checker) processAssocDataDef(add syntax.AssocDataDef, className string, instSpan span.Span) {
	fam, ok := ch.families[add.Name]
	if !ok {
		ch.addCodedError(errs.ErrBadInstance, instSpan,
			fmt.Sprintf("instance %s: associated data %s not declared in class %s",
				className, add.Name, className))
		return
	}
	if !fam.IsAssoc || fam.ClassName != className {
		ch.addCodedError(errs.ErrBadInstance, instSpan,
			fmt.Sprintf("instance %s: %s is not an associated data of class %s",
				className, add.Name, className))
		return
	}
	if len(add.Patterns) != len(fam.Params) {
		ch.addCodedError(errs.ErrTypeFamilyEquation, add.S,
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
	ch.config.RegisteredTypes[mangledName] = types.KType{}

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
		existingKind := ch.config.RegisteredTypes[mangledName]
		ch.config.RegisteredTypes[mangledName] = &types.KArrow{From: types.KType{}, To: existingKind}
	}

	dataInfo := &DataTypeInfo{Name: mangledName}
	ch.dataTypeByName[mangledName] = dataInfo

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
		if existing, dup := ch.conTypes[con.Name]; dup {
			ch.addCodedError(errs.ErrDuplicateDecl, con.S,
				fmt.Sprintf("data family instance %s: constructor %s conflicts with existing constructor (type: %s)",
					add.Name, con.Name, types.Pretty(existing)))
			continue
		}
		ch.conTypes[con.Name] = conType
		ch.ctx.Push(&CtxVar{Name: con.Name, Type: conType, Module: ch.currentModule})
		ch.conModules[con.Name] = ch.currentModule
		dataInfo.Constructors = append(dataInfo.Constructors, ConInfo{Name: con.Name, Arity: len(fieldTypes)})
		ch.conInfo[con.Name] = dataInfo
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
			ch.addCodedError(errs.ErrMissingMethod, s,
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
			ch.addCodedError(errs.ErrBadInstance, s,
				fmt.Sprintf("instance %s: extra method %s not declared in class",
					className, m.Name))
			hasExtra = true
		}
	}
	return !hasExtra
}
