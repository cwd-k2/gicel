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
