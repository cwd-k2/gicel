package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// maxResolveDepth default is set via Budget.SetResolveDepthLimit in newChecker.

// extractDictField builds a Case expression that extracts the field at fieldIdx
// from a class dictionary constructor. prefix is used for generated variable names.
func (ch *Checker) extractDictField(classInfo *ClassInfo, dictExpr ir.Core, fieldIdx int, prefix string, s span.Span) ir.Core {
	allFields := len(classInfo.Supers) + len(classInfo.Methods)
	freshBase := ch.fresh()
	var patArgs []ir.Pattern
	var fieldExpr ir.Core
	for j := 0; j < allFields; j++ {
		argName := fmt.Sprintf("$%s_%d_%d", prefix, j, freshBase)
		patArgs = append(patArgs, &ir.PVar{Name: argName, S: s})
		if j == fieldIdx {
			fieldExpr = &ir.Var{Name: argName, S: s}
		}
	}
	return &ir.Case{
		Scrutinee: dictExpr,
		Alts: []ir.Alt{{
			Pattern:   &ir.PCon{Con: classInfo.DictName, Args: patArgs, S: s},
			Body:      fieldExpr,
			Generated: true,
			S:         s,
		}},
		S: s,
	}
}

// tryResolveInstance attempts instance resolution without emitting errors.
// Returns the dictionary expression and true on success, or nil and false if
// resolution fails. Any errors produced during the attempt are discarded.
func (ch *Checker) tryResolveInstance(className string, args []types.Type, s span.Span) (ir.Core, bool) {
	saved := ch.errors.Len()
	dict := ch.resolveInstance(className, args, s)
	if ch.errors.Len() > saved {
		ch.errors.Truncate(saved)
		return nil, false
	}
	return dict, true
}

// resolveInstance finds a dictionary expression for a given class constraint.
// Returns a Core expression that evaluates to the dictionary value.
func (ch *Checker) resolveInstance(className string, args []types.Type, s span.Span) ir.Core {
	if err := ch.budget.EnterResolve(); err != nil {
		ch.addCodedError(diagnostic.ErrResolutionDepth, s,
			fmt.Sprintf("instance resolution depth limit exceeded for %s %s (possible infinite loop in instance contexts)",
				className, ch.prettyTypeArgs(args)))
		return &ir.Var{Name: "<resolution-depth>", S: s}
	}
	defer ch.budget.LeaveResolve()

	// 1. Search context for dictionary variables (from Eq a => parameters).
	var ctxResult ir.Core
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if v, ok := entry.(*CtxVar); ok && ch.matchesDictVar(v, className, args) {
			ctxResult = &ir.Var{Name: v.Name, Module: v.Module, S: s}
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

	// 1.7. Apply functional dependency improvement before instance search.
	// If the class has fundeps and some "from" args are determined,
	// search instances to unify "to" positions.
	ch.applyFunDepImprovement(className, args)

	// 2. Search global instances (indexed by class name).
	for _, inst := range ch.reg.instancesByClass[className] {
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
		var dictExpr ir.Core = &ir.Var{Name: inst.DictBindName, Module: inst.Module, S: s}
		for _, ctx := range inst.Context {
			ctxArgs := make([]types.Type, len(ctx.Args))
			for j, a := range ctx.Args {
				ctxArgs[j] = ch.unifier.Zonk(types.SubstMany(a, freshSubst))
			}
			ctxDict := ch.resolveInstance(ctx.ClassName, ctxArgs, s)
			dictExpr = &ir.App{Fun: dictExpr, Arg: ctxDict, S: s}
		}
		return dictExpr
	}

	ch.addCodedError(diagnostic.ErrNoInstance, s,
		fmt.Sprintf("no instance for %s %s", className, ch.prettyTypeArgs(args)))
	return &ir.Var{Name: "<no-instance>", S: s}
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
func (ch *Checker) extractSuperDict(v *CtxVar, targetClass string, targetArgs []types.Type, s span.Span) ir.Core {
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
	return search.chain(&ir.Var{Name: v.Name, Module: v.Module, S: s}, con.Name, tyArgs)
}

// chain recursively searches the superclass hierarchy for the target class,
// building chained Case extractions along the path.
func (sd *superDictSearch) chain(dictExpr ir.Core, dictTyName string, dictTyArgs []types.Type) ir.Core {
	parentClass := classFromDict(dictTyName)
	if sd.visited[parentClass] {
		return nil
	}
	sd.visited[parentClass] = true

	classInfo, ok := sd.ch.reg.classes[parentClass]
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

// applyFunDepImprovement uses functional dependencies to improve type inference.
// For each fundep a -> b in the class, if the "from" args are determined (no metas),
// search instances whose "from" positions match, and unify the "to" positions.
func (ch *Checker) applyFunDepImprovement(className string, args []types.Type) {
	classInfo, ok := ch.reg.classes[className]
	if !ok || len(classInfo.FunDeps) == 0 {
		return
	}
	for _, fd := range classInfo.FunDeps {
		// Check if all "from" positions are determined (no unsolved metas after zonk).
		allDetermined := true
		for _, fromIdx := range fd.From {
			if fromIdx >= len(args) {
				allDetermined = false
				break
			}
			zonked := ch.unifier.Zonk(args[fromIdx])
			if _, isMeta := zonked.(*types.TyMeta); isMeta {
				allDetermined = false
				break
			}
		}
		if !allDetermined {
			continue
		}
		// Search instances: if "from" positions match, unify "to" positions.
		for _, inst := range ch.reg.instancesByClass[className] {
			if len(inst.TypeArgs) != len(args) {
				continue
			}
			freshSubst := ch.freshInstanceSubst(inst)
			fromMatch := ch.withTrial(func() bool {
				for _, fromIdx := range fd.From {
					instArg := types.SubstMany(inst.TypeArgs[fromIdx], freshSubst)
					if err := ch.unifier.Unify(instArg, args[fromIdx]); err != nil {
						return false
					}
				}
				return true
			})
			if fromMatch {
				// "From" positions match — unify "to" positions.
				for _, toIdx := range fd.To {
					if toIdx >= len(args) {
						continue
					}
					instArg := ch.unifier.Zonk(types.SubstMany(inst.TypeArgs[toIdx], freshSubst))
					// Fundep improvement is best-effort: if the "to" position
					// cannot be unified (e.g., already constrained to a different
					// type), we silently skip. This is intentional — fundep
					// improvement is advisory, not mandatory. A hard error here
					// would reject valid programs where the fundep simply provides
					// no additional information.
					_ = ch.unifier.Unify(args[toIdx], instArg)
				}
				break // first matching instance wins
			}
		}
	}
}

func (ch *Checker) prettyTypeArgs(args []types.Type) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = types.Pretty(ch.unifier.Zonk(a))
	}
	return strings.Join(parts, " ")
}
