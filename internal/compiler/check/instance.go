package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processImplHeader processes an impl declaration, validates and registers
// the instance. Returns the instance info and a map of unevaluated method
// expressions. The method map is consumed by processInstanceBody; it is not
// stored in InstanceInfo.
func (ch *Checker) processImplHeader(impl *syntax.DeclImpl) (*InstanceInfo, map[string]syntax.Expr) {
	// Decompose the type annotation into class name and type args.
	// impl Eq Bool := { ... }  →  Ann = TyExprApp(TyExprCon("Eq"), TyExprCon("Bool"))

	// Peel off context constraints: impl (Eq a) => Ord a := { ... }
	ann := impl.Ann
	var contextExprs []syntax.TypeExpr
	for {
		if qual, ok := ann.(*syntax.TyExprQual); ok {
			contextExprs = append(contextExprs, desugarConstraints(qual.Constraint)...)
			ann = qual.Body
		} else {
			break
		}
	}

	// Unwind application to get class name and type args.
	className, syntaxTypeArgs := unwindTypeApp(ann)
	if className == "" {
		return nil, nil
	}

	// Extract methods and associated type definitions from the body.
	bodyParts := extractImplBody(impl.Body)

	// --- Instance validation and registration ---

	classInfo, ok := ch.reg.LookupClass(className)
	if !ok {
		ch.addCodedError(diagnostic.ErrBadClass, impl.S, fmt.Sprintf("unknown class: %s", className))
		return nil, nil
	}

	// Resolve type arguments.
	var typeArgs []types.Type
	for _, ta := range syntaxTypeArgs {
		typeArgs = append(typeArgs, ch.resolveTypeExpr(ta))
	}

	ch.autoLiftTypeArgs(typeArgs, classInfo.TyParamKinds)

	// Resolve context constraints.
	var context []ConstraintInfo
	for _, ctx := range contextExprs {
		resolved := ch.resolveTypeExpr(ctx)
		head, args := types.UnwindApp(resolved)
		if con, ok := head.(*types.TyCon); ok {
			context = append(context, ConstraintInfo{ClassName: con.Name, Args: args})
		}
	}

	// Arity check: number of type arguments must match class parameter count.
	if len(typeArgs) != len(classInfo.TyParams) {
		ch.addCodedError(diagnostic.ErrBadInstance, impl.S,
			fmt.Sprintf("instance %s: expected %d type argument(s), got %d",
				className, len(classInfo.TyParams), len(typeArgs)))
		return nil, nil
	}

	// Context well-formedness: each constraint in the instance context must reference a known class.
	for _, ctx := range context {
		if _, ok := ch.reg.LookupClass(ctx.ClassName); !ok {
			ch.addCodedError(diagnostic.ErrBadInstance, impl.S,
				fmt.Sprintf("instance %s: context references unknown class %s",
					className, ctx.ClassName))
			return nil, nil
		}
	}

	// Self-cycle detection: instance context must not require itself.
	// e.g., `instance Eq a => Eq a` would cause infinite recursion.
	for _, ctx := range context {
		if ctx.ClassName == className && len(ctx.Args) == len(typeArgs) {
			selfCycle := true
			for i := range ctx.Args {
				if !types.Equal(ctx.Args[i], typeArgs[i]) {
					selfCycle = false
					break
				}
			}
			if selfCycle {
				ch.addCodedError(diagnostic.ErrBadInstance, impl.S,
					fmt.Sprintf("instance %s: self-referential context (instance requires itself)",
						className))
				return nil, nil
			}
		}
	}

	// Build dict binding name: ClassName$TypeName (simplified naming).
	// Private instances get a unique suffix to avoid colliding with the
	// public instance dict in the context.
	dictName := ch.instanceDictName(className, typeArgs)
	if env.IsPrivateName(impl.Name) {
		dictName = fmt.Sprintf("%s$%d", dictName, ch.fresh())
	}

	// Validate method set (missing / extra).
	if !ch.validateInstanceMethods(className, classInfo, bodyParts.Methods, impl.S) {
		return nil, nil
	}

	inst := &InstanceInfo{
		ClassName:    className,
		TypeArgs:     typeArgs,
		Context:      context,
		DictBindName: dictName,
		UserName:     impl.Name,
		Module:       ch.scope.CurrentModule(),
		Private:      env.IsPrivateName(impl.Name),
		FreeVarNames: collectInstanceFreeVars(typeArgs, context),
		S:            impl.S,
	}

	// Overlap check: verify no existing local instance for this class matches the same types.
	// Imported instances are excluded: user source is allowed to shadow module instances.
	// Private instances (impl _name) are solver-invisible outside their defining module
	// and occupy a separate namespace — they never overlap with public instances.
	for _, existing := range ch.reg.InstancesForClass(className) {
		if existing == inst {
			continue // same pointer (re-exported via module)
		}
		if ch.reg.IsImportedInstance(existing) {
			continue // imported from a module; shadowing is allowed
		}
		if inst.Private || existing.Private {
			continue // private instances don't participate in public overlap checks
		}
		if len(existing.TypeArgs) != len(typeArgs) {
			continue
		}
		if ch.instancesOverlap(existing, inst) {
			ch.addCodedError(diagnostic.ErrOverlap, impl.S,
				fmt.Sprintf("overlapping instances for class %s: %s and %s",
					className, existing.DictBindName, dictName))
			return nil, nil
		}
	}

	// Process associated type definitions: convert to equations and append
	// to the corresponding TypeFamilyInfo registered during class processing.
	for _, td := range bodyParts.TypeDefs {
		if td.IsData || td.TypeDef == nil {
			continue
		}
		fam, ok := ch.reg.LookupFamily(td.Name)
		if !ok {
			ch.addCodedError(diagnostic.ErrBadInstance, impl.S,
				fmt.Sprintf("instance %s: associated type %s not declared in class %s",
					className, td.Name, className))
			continue
		}
		if !fam.IsAssoc || fam.ClassName != className {
			ch.addCodedError(diagnostic.ErrBadInstance, impl.S,
				fmt.Sprintf("instance %s: %s is not an associated type of class %s",
					className, td.Name, className))
			continue
		}
		// Arity check.
		if len(syntaxTypeArgs) != len(fam.Params) {
			ch.addCodedError(diagnostic.ErrTypeFamilyEquation, td.S,
				fmt.Sprintf("associated type %s expects %d argument(s), got %d",
					td.Name, len(fam.Params), len(syntaxTypeArgs)))
			continue
		}
		// Resolve patterns and RHS.
		resolvedPats := make([]types.Type, len(syntaxTypeArgs))
		for i, pat := range syntaxTypeArgs {
			resolvedPats[i] = ch.resolveTypeExpr(pat)
		}
		resolvedRHS := ch.resolveTypeExpr(td.TypeDef)
		fam.Equations = append(fam.Equations, tfEquation{
			Patterns: resolvedPats,
			RHS:      resolvedRHS,
			S:        td.S,
		})
	}

	// Process associated data family definitions: register constructors
	// and create type family equations mapping the family to its mangled data type.
	for _, td := range bodyParts.TypeDefs {
		if !td.IsData || td.TypeDef == nil {
			continue
		}
		ch.processAssocDataDef(td, syntaxTypeArgs, className, impl.S)
	}

	ch.reg.RegisterInstance(inst)
	return inst, bodyParts.Methods
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

// collectInstanceFreeVars computes the deduplicated list of free type variable
// names across an instance's type arguments and context. Called once at
// registration time to avoid repeated FreeVars calls during resolution.
func collectInstanceFreeVars(typeArgs []types.Type, context []env.ConstraintInfo) []string {
	seen := make(map[string]bool)
	var names []string
	add := func(ty types.Type) {
		for v := range types.FreeVars(ty) {
			if !seen[v] {
				seen[v] = true
				names = append(names, v)
			}
		}
	}
	for _, ta := range typeArgs {
		add(ta)
	}
	for _, ctx := range context {
		for _, a := range ctx.Args {
			add(a)
		}
	}
	return names
}
