package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/internal/types"
)

// InstanceInfo stores elaborated instance information.
type InstanceInfo struct {
	ClassName    string
	TypeArgs     []types.Type     // concrete type arguments
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

	// Auto-lift: if a type argument's kind doesn't match the class parameter kind
	// but wrapping with Lift makes it match, do so automatically.
	// e.g. instance IxMonad Maybe → instance IxMonad (Lift Maybe)
	for i := 0; i < len(typeArgs) && i < len(classInfo.TyParamKinds); i++ {
		argKind := ch.kindOfType(typeArgs[i])
		paramKind := classInfo.TyParamKinds[i]
		if argKind != nil && !argKind.Equal(paramKind) {
			// Check if Lift wrapping fixes the kind mismatch.
			liftKind := ch.kindOfType(&types.TyCon{Name: "Lift"})
			if liftKind != nil {
				if ka, ok := liftKind.(*types.KArrow); ok && ka.From.Equal(argKind) {
					lifted := &types.TyApp{Fun: &types.TyCon{Name: "Lift"}, Arg: typeArgs[i]}
					liftedKind := ka.To
					if liftedKind.Equal(paramKind) {
						typeArgs[i] = lifted
					}
				}
			}
		}
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
			break
		}
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
		paramName := fmt.Sprintf("$d_%s_%d", ctx.ClassName, ch.fresh())
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
	var dictExpr core.Core = &core.Con{Name: classInfo.DictConName, S: inst.S}
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
	ch.ctx.Push(&CtxVar{Name: inst.DictBindName, Type: dictTy})
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

// instancesOverlap checks if two instances can match the same type arguments
// using trial unification with fresh metavariables. The unifier state is saved
// and restored so no side effects persist.
func (ch *Checker) instancesOverlap(a, b *InstanceInfo) bool {
	// Save unifier state.
	savedSoln := make(map[int]types.Type)
	for k, v := range ch.unifier.Solutions() {
		savedSoln[k] = v
	}
	defer func() {
		// Roll back unification changes.
		for k := range ch.unifier.Solutions() {
			if _, existed := savedSoln[k]; !existed {
				delete(ch.unifier.Solutions(), k)
			}
		}
		for k, v := range savedSoln {
			ch.unifier.Solutions()[k] = v
		}
	}()

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

// aliasParamKind returns the kind of the i-th parameter of a type alias.
func (ch *Checker) aliasParamKind(aliasName string, i int) types.Kind {
	info, ok := ch.aliases[aliasName]
	if !ok || i >= len(info.paramKinds) {
		return types.KType{}
	}
	return info.paramKinds[i]
}

// kindOfType returns the kind of a resolved type, or nil if unknown.
func (ch *Checker) kindOfType(ty types.Type) types.Kind {
	switch t := ty.(type) {
	case *types.TyCon:
		if k, ok := ch.config.RegisteredTypes[t.Name]; ok {
			return k
		}
		// Type aliases: compute kind from parameter kinds.
		if info, ok := ch.aliases[t.Name]; ok {
			var kind types.Kind = types.KType{}
			for i := len(info.params) - 1; i >= 0; i-- {
				paramKind := ch.aliasParamKind(t.Name, i)
				kind = &types.KArrow{From: paramKind, To: kind}
			}
			return kind
		}
		// Well-known built-in type constructors.
		switch t.Name {
		case "Computation", "Thunk":
			return &types.KArrow{From: types.KRow{}, To: &types.KArrow{From: types.KRow{}, To: &types.KArrow{From: types.KType{}, To: types.KType{}}}}
		}
		return types.KType{}
	case *types.TyApp:
		funKind := ch.kindOfType(t.Fun)
		if ka, ok := funKind.(*types.KArrow); ok {
			return ka.To
		}
		return nil
	case *types.TyMeta:
		return t.Kind
	case *types.TySkolem:
		return t.Kind
	case *types.TyVar:
		if k, ok := ch.ctx.LookupTyVar(t.Name); ok {
			return k
		}
		return types.KType{}
	default:
		return types.KType{}
	}
}
