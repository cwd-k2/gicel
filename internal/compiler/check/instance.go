package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// processImplHeader processes an impl declaration, validates and registers
// the instance. Returns the instance info and a map of unevaluated method
// expressions. The method map is consumed by processInstanceBody; it is not
// stored in InstanceInfo.
func (ch *Checker) processImplHeader(impl *syntax.DeclImpl) (*InstanceInfo, map[string]syntax.Expr) {
	// Decompose the type annotation into class name and type args.
	// impl Eq Bool := { ... }  ->  Ann = TyExprApp(TyExprCon("Eq"), TyExprCon("Bool"))

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
		ch.addDiag(diagnostic.ErrBadClass, impl.S, diagUnknown{Kind: "class", Name: className})
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
		ch.addDiag(diagnostic.ErrBadInstance, impl.S,
			diagFmt{Format: "instance %s: expected %d type argument(s), got %d",
				Args: []any{className, len(classInfo.TyParams), len(typeArgs)}})
		return nil, nil
	}

	// Validate context constraints (well-formedness + self-cycle).
	if !ch.validateInstanceContext(className, typeArgs, context, impl.S) {
		return nil, nil
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
		FreeVars:     collectInstanceFreeVarsWithKind(typeArgs, context),
		S:            impl.S,
	}

	// Overlap check: verify no existing instance for this class matches the same types.
	// Both local and imported (stdlib) instances are checked — instance coherence
	// requires that no two instances for the same class/type combination coexist.
	// Private instances (impl _name) are solver-invisible outside their defining module
	// and occupy a separate namespace — they never overlap with public instances.
	for _, existing := range ch.reg.InstancesForClass(className) {
		if existing == inst {
			continue // same pointer (re-exported via module)
		}
		if inst.Private || existing.Private {
			continue // private instances don't participate in public overlap checks
		}
		if len(existing.TypeArgs) != len(typeArgs) {
			continue
		}
		if ch.instancesOverlap(existing, inst) {
			ch.addDiag(diagnostic.ErrOverlap, impl.Ann.Span(),
				diagFmt{Format: "overlapping instances for class %s: %s and %s", Args: []any{className, existing.DictBindName, dictName}})
			return nil, nil
		}
	}

	// Inject associated type definitions and data family definitions.
	ch.injectAssocTypes(bodyParts.TypeDefs, syntaxTypeArgs, className, impl.S)

	if ch.config.DeclRecorder != nil {
		// Build the impl type: ClassName TypeArg1 TypeArg2 ...
		var implTy types.Type = &types.TyCon{Name: className}
		for _, arg := range typeArgs {
			implTy = &types.TyApp{Fun: implTy, Arg: arg}
		}
		label := className
		if impl.Name != "" {
			label = impl.Name
		}
		ch.config.DeclRecorder(impl.S, "impl", label, implTy)
	}
	ch.reg.RegisterInstance(inst)
	return inst, bodyParts.Methods
}

// validateInstanceContext checks context well-formedness and detects
// self-referential constraints. Returns false if any error was found.
func (ch *Checker) validateInstanceContext(className string, typeArgs []types.Type, context []ConstraintInfo, s span.Span) bool {
	// Context well-formedness: each constraint in the instance context must reference a known class.
	for _, ctx := range context {
		if _, ok := ch.reg.LookupClass(ctx.ClassName); !ok {
			ch.addDiag(diagnostic.ErrBadInstance, s,
				diagFmt{Format: "instance %s: context references unknown class %s", Args: []any{className, ctx.ClassName}})
			return false
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
				ch.addDiag(diagnostic.ErrBadInstance, s,
					diagFmt{Format: "instance %s: self-referential context (instance requires itself)", Args: []any{className}})
				return false
			}
		}
	}
	return true
}

// injectAssocTypes processes associated type and data family definitions
// from an instance body, converting them to type family equations.
func (ch *Checker) injectAssocTypes(typeDefs []syntax.ImplField, syntaxTypeArgs []syntax.TypeExpr, className string, s span.Span) {
	// Process associated type definitions: convert to equations and append
	// to the corresponding TypeFamilyInfo registered during class processing.
	for _, td := range typeDefs {
		if td.IsData || td.TypeDef == nil {
			continue
		}
		fam, ok := ch.reg.LookupFamily(td.Name)
		if !ok {
			ch.addDiag(diagnostic.ErrBadInstance, s,
				diagFmt{Format: "instance %s: associated type %s not declared in class %s", Args: []any{className, td.Name, className}})
			continue
		}
		if !fam.IsAssoc || fam.ClassName != className {
			ch.addDiag(diagnostic.ErrBadInstance, s,
				diagFmt{Format: "instance %s: %s is not an associated type of class %s", Args: []any{className, td.Name, className}})
			continue
		}
		// Arity check.
		if len(syntaxTypeArgs) != len(fam.Params) {
			ch.addDiag(diagnostic.ErrTypeFamilyEquation, td.S,
				diagFmt{Format: "associated type %s expects %d argument(s), got %d",
					Args: []any{td.Name, len(fam.Params), len(syntaxTypeArgs)}})
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
	for _, td := range typeDefs {
		if !td.IsData || td.TypeDef == nil {
			continue
		}
		ch.processAssocDataDef(td, syntaxTypeArgs, className, s)
	}
}

// instancesOverlap checks if two instances can match the same type arguments
// using trial unification with fresh metavariables. The unifier state is saved
// and restored so no side effects persist.
func (ch *Checker) instancesOverlap(a, b *InstanceInfo) bool {
	saved := ch.saveState()
	defer ch.restoreState(saved)

	psA := types.PrepareSubst(ch.freshInstanceSubst(a))
	psB := types.PrepareSubst(ch.freshInstanceSubst(b))
	for i := range a.TypeArgs {
		argA := psA.Apply(a.TypeArgs[i])
		argB := psB.Apply(b.TypeArgs[i])
		if err := ch.unifier.Unify(argA, argB); err != nil {
			return false
		}
	}
	return true
}

// collectInstanceFreeVars computes the deduplicated list of free type variable
// names across an instance's type arguments and context. Called once at
// registration time to avoid repeated FreeVars calls during resolution.
// collectInstanceFreeVarsWithKind collects free type variables from instance
// type arguments and context, inferring each variable's kind from its position
// in the type structure.
//
// Kind inference rules:
//   - TyVar at TyCBPV.Pre/Post → Row
//   - TyVar at TyEvidenceRow.Tail → Row
//   - TyVar elsewhere → Type
func collectInstanceFreeVarsWithKind(typeArgs []types.Type, context []env.ConstraintInfo) []env.FreeVarInfo {
	seen := make(map[string]bool)
	var vars []env.FreeVarInfo
	var walk func(ty types.Type, expectedKind types.Type)
	walk = func(ty types.Type, expectedKind types.Type) {
		switch t := ty.(type) {
		case *types.TyVar:
			if !seen[t.Name] {
				seen[t.Name] = true
				vars = append(vars, env.FreeVarInfo{Name: t.Name, Kind: expectedKind})
			}
		case *types.TyApp:
			walk(t.Fun, expectedKind)
			walk(t.Arg, types.TypeOfTypes)
		case *types.TyArrow:
			walk(t.From, types.TypeOfTypes)
			walk(t.To, types.TypeOfTypes)
		case *types.TyCBPV:
			walk(t.Pre, types.TypeOfRows)
			walk(t.Post, types.TypeOfRows)
			walk(t.Result, types.TypeOfTypes)
			if t.Grade != nil {
				walk(t.Grade, types.TypeOfTypes)
			}
		case *types.TyEvidenceRow:
			for _, child := range t.Entries.AllChildren() {
				walk(child, types.TypeOfTypes)
			}
			if t.Tail != nil {
				walk(t.Tail, types.TypeOfRows)
			}
		case *types.TyEvidence:
			walk(t.Constraints, types.TypeOfTypes)
			walk(t.Body, expectedKind)
		case *types.TyForall:
			walk(t.Body, expectedKind)
		}
	}
	for _, ta := range typeArgs {
		walk(ta, types.TypeOfTypes)
	}
	for _, ctx := range context {
		for _, a := range ctx.Args {
			walk(a, types.TypeOfTypes)
		}
	}
	return vars
}
