package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

const maxResolveDepth = 64

// extractDictField builds a Case expression that extracts the field at fieldIdx
// from a class dictionary constructor. prefix is used for generated variable names.
func (ch *Checker) extractDictField(classInfo *ClassInfo, dictExpr core.Core, fieldIdx int, prefix string, s span.Span) core.Core {
	allFields := len(classInfo.Supers) + len(classInfo.Methods)
	freshBase := ch.fresh()
	var patArgs []core.Pattern
	var fieldExpr core.Core
	for j := 0; j < allFields; j++ {
		argName := fmt.Sprintf("$%s_%d_%d", prefix, j, freshBase)
		patArgs = append(patArgs, &core.PVar{Name: argName, S: s})
		if j == fieldIdx {
			fieldExpr = &core.Var{Name: argName, S: s}
		}
	}
	return &core.Case{
		Scrutinee: dictExpr,
		Alts: []core.Alt{{
			Pattern: &core.PCon{Con: classInfo.DictName, Args: patArgs, S: s},
			Body:    fieldExpr,
			S:       s,
		}},
		S: s,
	}
}

// resolveInstance finds a dictionary expression for a given class constraint.
// Returns a Core expression that evaluates to the dictionary value.
func (ch *Checker) resolveInstance(className string, args []types.Type, s span.Span) core.Core {
	ch.resolveDepth++
	defer func() { ch.resolveDepth-- }()
	if ch.resolveDepth > maxResolveDepth {
		ch.addCodedError(errs.ErrResolutionDepth, s,
			fmt.Sprintf("instance resolution depth limit exceeded for %s %s (possible infinite loop in instance contexts)",
				className, ch.prettyTypeArgs(args)))
		return &core.Var{Name: "<resolution-depth>", S: s}
	}

	// 1. Search context for dictionary variables (from Eq a => parameters).
	var ctxResult core.Core
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if v, ok := entry.(*CtxVar); ok && ch.matchesDictVar(v, className, args) {
			ctxResult = &core.Var{Name: v.Name, S: s}
			return false
		}
		return true
	})
	if ctxResult != nil {
		return ctxResult
	}

	// 1.5. Search context for superclass dictionaries.
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if v, ok := entry.(*CtxVar); ok {
			if expr := ch.extractSuperDict(v, className, args, s); expr != nil {
				ctxResult = expr
				return false
			}
		}
		return true
	})
	if ctxResult != nil {
		return ctxResult
	}

	// 1.6. Search context for quantified evidence that can produce the needed dictionary.
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if e, ok := entry.(*CtxEvidence); ok && e.Quantified != nil {
			if expr := ch.applyQuantifiedEvidence(e, className, args, s); expr != nil {
				ctxResult = expr
				return false
			}
		}
		return true
	})
	if ctxResult != nil {
		return ctxResult
	}

	// 2. Search global instances (indexed by class name).
	for _, inst := range ch.instancesByClass[className] {
		if len(inst.TypeArgs) != len(args) {
			continue
		}
		freshSubst := ch.freshInstanceSubst(inst)
		if !ch.withTrial(func() bool {
			for i := range args {
				instArg := types.SubstMany(inst.TypeArgs[i], freshSubst)
				if err := ch.unifier.Unify(instArg, args[i]); err != nil {
					return false
				}
			}
			return true
		}) {
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
	ty := ch.unifier.Zonk(v.Type)
	head, tyArgs := types.UnwindApp(ty)
	if con, ok := head.(*types.TyCon); ok && con.Name == dictName(className) {
		if len(tyArgs) != len(args) {
			return false
		}
		return ch.withTrial(func() bool {
			for i := range args {
				if err := ch.unifier.Unify(tyArgs[i], args[i]); err != nil {
					return false
				}
			}
			return true
		})
	}
	return false
}

// superDictSearch holds the immutable context for a superclass dictionary search.
type superDictSearch struct {
	ch          *Checker
	targetClass string
	targetArgs  []types.Type
	s           span.Span
	visited     map[string]bool
}

// extractSuperDict checks if a context variable is a dict for a class that
// has the target class as a (possibly transitive) superclass.
func (ch *Checker) extractSuperDict(v *CtxVar, targetClass string, targetArgs []types.Type, s span.Span) core.Core {
	ty := ch.unifier.Zonk(v.Type)
	head, tyArgs := types.UnwindApp(ty)
	con, ok := head.(*types.TyCon)
	if !ok || !isDictName(con.Name) {
		return nil
	}
	search := &superDictSearch{
		ch: ch, targetClass: targetClass, targetArgs: targetArgs,
		s: s, visited: make(map[string]bool),
	}
	return search.chain(&core.Var{Name: v.Name, S: s}, con.Name, tyArgs)
}

// chain recursively searches the superclass hierarchy for the target class,
// building chained Case extractions along the path.
func (sd *superDictSearch) chain(dictExpr core.Core, dictTyName string, dictTyArgs []types.Type) core.Core {
	parentClass := classFromDict(dictTyName)
	if sd.visited[parentClass] {
		return nil
	}
	sd.visited[parentClass] = true

	classInfo, ok := sd.ch.classes[parentClass]
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

		extractExpr := sd.ch.extractDictField(classInfo, dictExpr, superIdx, "sf", sd.s)

		// Direct match: this superclass IS the target.
		if sup.ClassName == sd.targetClass && len(superArgs) == len(sd.targetArgs) {
			if sd.ch.withTrial(func() bool {
				for j := range sd.targetArgs {
					if err := sd.ch.unifier.Unify(superArgs[j], sd.targetArgs[j]); err != nil {
						return false
					}
				}
				return true
			}) {
				return extractExpr
			}
		}

		// Transitive: search within this superclass's dict.
		result := sd.chain(extractExpr, dictName(sup.ClassName), superArgs)
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
	if !ch.withTrial(func() bool {
		for i := range args {
			headArg := types.SubstMany(qc.Head.Args[i], freshSubst)
			if err := ch.unifier.Unify(headArg, args[i]); err != nil {
				return false
			}
		}
		return true
	}) {
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

		if !ch.withTrial(func() bool {
			for i := range qc.Head.Args {
				headArg := types.SubstMany(qc.Head.Args[i], freshSubst)
				instArg := types.SubstMany(inst.TypeArgs[i], instSubst)
				if err := ch.unifier.Unify(headArg, instArg); err != nil {
					return false
				}
			}
			// Verify context compatibility: instance context should subsume quantified context.
			if len(inst.Context) != len(qc.Context) {
				return false
			}
			for i, ic := range inst.Context {
				if ic.ClassName != qc.Context[i].ClassName {
					return false
				}
			}
			return true
		}) {
			continue
		}

		// The instance dict binding has the right type.
		return &core.Var{Name: inst.DictBindName, S: s}
	}

	// Also search context for quantified evidence variables.
	var qcResult core.Core
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if e, ok := entry.(*CtxEvidence); ok && e.Quantified != nil {
			if e.Quantified.Head.ClassName == qc.Head.ClassName {
				qcResult = &core.Var{Name: e.DictName, S: s}
				return false
			}
		}
		return true
	})
	if qcResult != nil {
		return qcResult
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
