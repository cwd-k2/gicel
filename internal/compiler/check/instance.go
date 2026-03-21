package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processInstanceHeader validates and registers an instance.
// Returns the instance info and a map of unevaluated method expressions.
// The method map is consumed by processInstanceBody; it is not stored in InstanceInfo.
func (ch *Checker) processInstanceHeader(d *syntax.DeclInstance) (*InstanceInfo, map[string]syntax.Expr) {
	classInfo, ok := ch.reg.LookupClass(d.ClassName)
	if !ok {
		ch.addCodedError(diagnostic.ErrBadClass, d.S, fmt.Sprintf("unknown class: %s", d.ClassName))
		return nil, nil
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
		ch.addCodedError(diagnostic.ErrBadInstance, d.S,
			fmt.Sprintf("instance %s: expected %d type argument(s), got %d",
				d.ClassName, len(classInfo.TyParams), len(typeArgs)))
		return nil, nil
	}

	// Context well-formedness: each constraint in the instance context must reference a known class.
	for _, ctx := range context {
		if _, ok := ch.reg.LookupClass(ctx.ClassName); !ok {
			ch.addCodedError(diagnostic.ErrBadInstance, d.S,
				fmt.Sprintf("instance %s: context references unknown class %s",
					d.ClassName, ctx.ClassName))
			return nil, nil
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
				ch.addCodedError(diagnostic.ErrBadInstance, d.S,
					fmt.Sprintf("instance %s: self-referential context (instance requires itself)",
						d.ClassName))
				return nil, nil
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
		return nil, nil
	}

	inst := &InstanceInfo{
		ClassName:    d.ClassName,
		TypeArgs:     typeArgs,
		Context:      context,
		DictBindName: dictName,
		Module:       ch.scope.CurrentModule(),
		S:            d.S,
	}

	// Overlap check: verify no existing local instance for this class matches the same types.
	// Imported instances are excluded: user source is allowed to shadow module instances.
	for _, existing := range ch.reg.InstancesForClass(d.ClassName) {
		if existing == inst {
			continue // same pointer (re-exported via module)
		}
		if ch.reg.IsImportedInstance(existing) {
			continue // imported from a module; shadowing is allowed
		}
		if len(existing.TypeArgs) != len(typeArgs) {
			continue
		}
		if ch.instancesOverlap(existing, inst) {
			ch.addCodedError(diagnostic.ErrOverlap, d.S,
				fmt.Sprintf("overlapping instances for class %s: %s and %s",
					d.ClassName, existing.DictBindName, dictName))
			return nil, nil
		}
	}

	// Process associated type definitions: convert to equations and append
	// to the corresponding TypeFamilyInfo registered during class processing.
	for _, atd := range d.AssocTypeDefs {
		fam, ok := ch.reg.LookupFamily(atd.Name)
		if !ok {
			ch.addCodedError(diagnostic.ErrBadInstance, d.S,
				fmt.Sprintf("instance %s: associated type %s not declared in class %s",
					d.ClassName, atd.Name, d.ClassName))
			continue
		}
		if !fam.IsAssoc || fam.ClassName != d.ClassName {
			ch.addCodedError(diagnostic.ErrBadInstance, d.S,
				fmt.Sprintf("instance %s: %s is not an associated type of class %s",
					d.ClassName, atd.Name, d.ClassName))
			continue
		}
		// Arity check.
		if len(atd.Patterns) != len(fam.Params) {
			ch.addCodedError(diagnostic.ErrTypeFamilyEquation, atd.S,
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

	ch.reg.RegisterInstance(inst)
	return inst, methodExprs
}

// instancesOverlap checks if two instances can match the same type arguments
// using trial unification with fresh metavariables. The unifier state is saved
// and restored so no side effects persist.
func (ch *Checker) instancesOverlap(a, b *InstanceInfo) bool {
	saved := ch.saveState()
	defer ch.restoreState(saved)

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
