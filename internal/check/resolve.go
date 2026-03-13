package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/types"
)

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

	// 1.5. Search context for superclass dictionaries.
	for i := len(ch.ctx.entries) - 1; i >= 0; i-- {
		if v, ok := ch.ctx.entries[i].(*CtxVar); ok {
			if expr := ch.extractSuperDict(v, className, args, s); expr != nil {
				return expr
			}
		}
	}

	// 1.6. Search context for quantified evidence that can produce the needed dictionary.
	for i := len(ch.ctx.entries) - 1; i >= 0; i-- {
		if e, ok := ch.ctx.entries[i].(*CtxEvidence); ok && e.Quantified != nil {
			if expr := ch.applyQuantifiedEvidence(e, className, args, s); expr != nil {
				return expr
			}
		}
	}

	// 2. Search global instances (indexed by class name).
	for _, inst := range ch.instancesByClass[className] {
		if len(inst.TypeArgs) != len(args) {
			continue
		}
		freshSubst := ch.freshInstanceSubst(inst)
		matched := true
		for i := range args {
			instArg := types.SubstMany(inst.TypeArgs[i], freshSubst)
			if err := ch.unifier.Unify(instArg, args[i]); err != nil {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		var dictExpr core.Core = &core.Var{Name: inst.DictBindName, S: s}
		for _, ctx := range inst.Context {
			ctxArgs := make([]types.Type, len(ctx.Args))
			for j, a := range ctx.Args {
				ctxArgs[j] = ch.unifier.Zonk(types.SubstMany(a, freshSubst))
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

// extractSuperDict checks if a context variable is a dict for a class that
// has the target class as a (possibly transitive) superclass.
func (ch *Checker) extractSuperDict(v *CtxVar, targetClass string, targetArgs []types.Type, s span.Span) core.Core {
	ty := ch.unifier.Zonk(v.Type)
	head, tyArgs := types.UnwindApp(ty)
	con, ok := head.(*types.TyCon)
	if !ok || !strings.HasSuffix(con.Name, "$Dict") {
		return nil
	}
	return ch.extractSuperDictChain(
		&core.Var{Name: v.Name, S: s},
		con.Name, tyArgs,
		targetClass, targetArgs,
		s, nil,
	)
}

// extractSuperDictChain recursively searches the superclass hierarchy for
// the target class, building chained Case extractions along the path.
func (ch *Checker) extractSuperDictChain(
	dictExpr core.Core, dictTyName string, dictTyArgs []types.Type,
	targetClass string, targetArgs []types.Type,
	s span.Span, visited map[string]bool,
) core.Core {
	if visited == nil {
		visited = make(map[string]bool)
	}
	parentClass := strings.TrimSuffix(dictTyName, "$Dict")
	if visited[parentClass] {
		return nil
	}
	visited[parentClass] = true

	classInfo, ok := ch.classes[parentClass]
	if !ok {
		return nil
	}

	// Build substitution: class type params → actual dict type args.
	subst := make(map[string]types.Type)
	for j, p := range classInfo.TyParams {
		if j < len(dictTyArgs) {
			subst[p] = dictTyArgs[j]
		}
	}

	for superIdx, sup := range classInfo.Supers {
		superArgs := make([]types.Type, len(sup.Args))
		for j, a := range sup.Args {
			superArgs[j] = types.SubstMany(a, subst)
		}

		// Build Case extraction for this super field.
		allFields := len(classInfo.Supers) + len(classInfo.Methods)
		var patArgs []core.Pattern
		var fieldExpr core.Core
		freshBase := ch.fresh()
		for j := 0; j < allFields; j++ {
			argName := fmt.Sprintf("$sf_%d_%d_%d", superIdx, j, freshBase)
			patArgs = append(patArgs, &core.PVar{Name: argName, S: s})
			if j == superIdx {
				fieldExpr = &core.Var{Name: argName, S: s}
			}
		}
		extractExpr := &core.Case{
			Scrutinee: dictExpr,
			Alts: []core.Alt{{
				Pattern: &core.PCon{Con: classInfo.DictConName, Args: patArgs, S: s},
				Body:    fieldExpr,
				S:       s,
			}},
			S: s,
		}

		// Direct match: this superclass IS the target.
		if sup.ClassName == targetClass {
			match := len(superArgs) == len(targetArgs)
			if match {
				for j := range targetArgs {
					if err := ch.unifier.Unify(superArgs[j], targetArgs[j]); err != nil {
						match = false
						break
					}
				}
			}
			if match {
				return extractExpr
			}
		}

		// Transitive: search within this superclass's dict.
		superDictTyName := sup.ClassName + "$Dict"
		result := ch.extractSuperDictChain(
			extractExpr, superDictTyName, superArgs,
			targetClass, targetArgs,
			s, visited,
		)
		if result != nil {
			return result
		}
	}
	return nil
}

// applyQuantifiedEvidence tries to use a quantified evidence entry to produce
// a dictionary for the given className and args.
// For example, if evidence is `forall a. Eq a => Eq (g a)` and we need `Eq (g Bool)`,
// it instantiates `a = Bool`, resolves `Eq Bool`, and builds the application.
func (ch *Checker) applyQuantifiedEvidence(e *CtxEvidence, className string, args []types.Type, s span.Span) core.Core {
	qc := e.Quantified
	// Head must match the target class.
	if qc.Head.ClassName != className {
		return nil
	}
	if len(qc.Head.Args) != len(args) {
		return nil
	}

	// Create fresh metas for the quantified variables.
	freshSubst := make(map[string]types.Type, len(qc.Vars))
	for _, v := range qc.Vars {
		freshSubst[v.Name] = ch.freshMeta(v.Kind)
	}

	// Try to unify head args with wanted args.
	// Save unifier state so we can roll back on failure.
	savedSoln := make(map[int]types.Type)
	for k, v := range ch.unifier.Solutions() {
		savedSoln[k] = v
	}

	allMatched := true
	for i := range args {
		headArg := types.SubstMany(qc.Head.Args[i], freshSubst)
		if err := ch.unifier.Unify(headArg, args[i]); err != nil {
			allMatched = false
			break
		}
	}

	if !allMatched {
		// Roll back unification changes.
		for k := range ch.unifier.Solutions() {
			if _, existed := savedSoln[k]; !existed {
				delete(ch.unifier.Solutions(), k)
			}
		}
		for k, v := range savedSoln {
			ch.unifier.Solutions()[k] = v
		}
		return nil
	}

	// Build the evidence expression by applying the quantified evidence function.
	var dictExpr core.Core = &core.Var{Name: e.DictName, S: s}

	// Apply type arguments.
	for _, v := range qc.Vars {
		tyArg := ch.unifier.Zonk(freshSubst[v.Name])
		dictExpr = &core.TyApp{Expr: dictExpr, TyArg: tyArg, S: s}
	}

	// Resolve and apply context (premise) dictionaries.
	for _, ctx := range qc.Context {
		ctxArgs := make([]types.Type, len(ctx.Args))
		for j, a := range ctx.Args {
			ctxArgs[j] = ch.unifier.Zonk(types.SubstMany(a, freshSubst))
		}
		ctxDict := ch.resolveInstance(ctx.ClassName, ctxArgs, s)
		dictExpr = &core.App{Fun: dictExpr, Arg: ctxDict, S: s}
	}

	return dictExpr
}

// resolveQuantifiedConstraint finds evidence for a quantified constraint.
// For `forall a. Eq a => Eq (F a)`, it searches for an instance whose structure
// matches (e.g., `instance Eq a => Eq (F a)`) and returns its dict binding.
func (ch *Checker) resolveQuantifiedConstraint(qc *types.QuantifiedConstraint, s span.Span) core.Core {
	// Strategy: the quantified constraint `forall a. C1 a => C2 (F a)` is satisfied
	// by a global instance `C2 (F a)` with context `C1 a`, which already has the
	// right type: `forall a. C1$Dict a -> C2$Dict (F a)`.
	//
	// Search global instances for a match on the head.
	for _, inst := range ch.instancesByClass[qc.Head.ClassName] {
		if len(inst.TypeArgs) != len(qc.Head.Args) {
			continue
		}
		// Check if this instance structurally matches the quantified constraint head.
		// Create a fresh substitution for the quantified vars.
		freshSubst := make(map[string]types.Type, len(qc.Vars))
		for _, v := range qc.Vars {
			freshSubst[v.Name] = ch.freshMeta(v.Kind)
		}

		// Also create fresh metas for the instance's own free vars.
		instSubst := ch.freshInstanceSubst(inst)

		matched := true
		for i := range qc.Head.Args {
			headArg := types.SubstMany(qc.Head.Args[i], freshSubst)
			instArg := types.SubstMany(inst.TypeArgs[i], instSubst)
			if err := ch.unifier.Unify(headArg, instArg); err != nil {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}

		// Verify context compatibility: instance context should subsume quantified context.
		// For simple cases, just check that the instance context matches.
		if len(inst.Context) != len(qc.Context) {
			continue
		}
		ctxMatch := true
		for i, ic := range inst.Context {
			if ic.ClassName != qc.Context[i].ClassName {
				ctxMatch = false
				break
			}
		}
		if !ctxMatch {
			continue
		}

		// The instance dict binding has the right type.
		return &core.Var{Name: inst.DictBindName, S: s}
	}

	// Also search context for quantified evidence variables.
	for i := len(ch.ctx.entries) - 1; i >= 0; i-- {
		if e, ok := ch.ctx.entries[i].(*CtxEvidence); ok && e.Quantified != nil {
			if e.Quantified.Head.ClassName == qc.Head.ClassName {
				return &core.Var{Name: e.DictName, S: s}
			}
		}
	}

	ch.addCodedError(errs.ErrNoInstance, s,
		fmt.Sprintf("no instance for %s %s", qc.Head.ClassName, ch.prettyTypeArgs(qc.Head.Args)))
	return &core.Var{Name: "<no-instance>", S: s}
}

// freshInstanceSubst creates a substitution mapping each free type variable
// in an instance's type arguments and context to a fresh meta variable.
func (ch *Checker) freshInstanceSubst(inst *InstanceInfo) map[string]types.Type {
	seen := make(map[string]bool)
	subst := make(map[string]types.Type)
	collect := func(ty types.Type) {
		for v := range types.FreeVars(ty) {
			if !seen[v] {
				seen[v] = true
				subst[v] = ch.freshMeta(types.KType{})
			}
		}
	}
	for _, ta := range inst.TypeArgs {
		collect(ta)
	}
	for _, ctx := range inst.Context {
		for _, a := range ctx.Args {
			collect(a)
		}
	}
	return subst
}

func (ch *Checker) prettyTypeArgs(args []types.Type) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = types.Pretty(ch.unifier.Zonk(a))
	}
	return strings.Join(parts, " ")
}
