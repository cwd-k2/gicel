package solve

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// maxResolveDepth default is set via Budget.SetResolveDepthLimit in newChecker.

// ExtractDictField builds a Case expression that extracts the field at fieldIdx
// from a class dictionary constructor. prefix is used for generated variable names.
func (s *Solver) ExtractDictField(classInfo *env.ClassInfo, dictExpr ir.Core, fieldIdx int, prefix string, sp span.Span) ir.Core {
	allFields := len(classInfo.Supers) + len(classInfo.Methods)
	freshBase := s.env.Fresh()
	var patArgs []ir.Pattern
	var fieldExpr ir.Core
	for j := 0; j < allFields; j++ {
		argName := fmt.Sprintf("$%s_%d_%d", prefix, j, freshBase)
		patArgs = append(patArgs, &ir.PVar{Name: argName, S: sp})
		if j == fieldIdx {
			fieldExpr = &ir.Var{Name: argName, S: sp}
		}
	}
	return &ir.Case{
		Scrutinee: dictExpr,
		Alts: []ir.Alt{{
			Pattern:   &ir.PCon{Con: classInfo.DictName, Args: patArgs, S: sp},
			Body:      fieldExpr,
			Generated: true,
			S:         sp,
		}},
		S: sp,
	}
}

// ResolveInstance is the exported entry point for resolveInstance.
func (s *Solver) ResolveInstance(className string, args []types.Type, sp span.Span) ir.Core {
	return s.resolveInstance(className, args, sp)
}

// TryResolveInstance attempts instance resolution without emitting errors.
// Returns the dictionary expression and true on success, or nil and false if
// resolution fails. Any errors and worklist side effects produced during the
// attempt are discarded on failure.
func (s *Solver) TryResolveInstance(className string, args []types.Type, sp span.Span) (ir.Core, bool) {
	savedErrs := s.env.ErrorCount()
	savedWorklist := s.SaveWorklist()
	dict := s.resolveInstance(className, args, sp)
	if s.env.ErrorCount() > savedErrs {
		s.env.TruncateErrors(savedErrs)
		s.RestoreWorklist(savedWorklist) // restore, discard orphans
		return nil, false
	}
	s.RestoreWorklist(append(savedWorklist, s.SaveWorklist()...))
	return dict, true
}

// resolveInstance finds a dictionary expression for a given class constraint.
// Returns a Core expression that evaluates to the dictionary value.
//
// Resolution proceeds through phases in order (see resolve_instance.go):
//  1. Context search — exact dictionary variable match
//  2. Superclass extraction — transitive superclass chain
//  3. Quantified evidence — instantiate quantified context entries
//  4. Fundep improvement — refine type arguments via functional dependencies
//  5. Global instance search — match against registered instances
//
// Recursion and state contracts:
//   - Depth limited by budget.EnterResolve (default 64).
//   - No cycle detection: identical constraints in the call stack are stopped
//     only by depth exhaustion.
//   - Meta solutions accumulate in the shared unifier across recursive calls
//     (no rollback). Instance head unification uses withTrial, but context
//     resolution (recursive resolveInstance) commits permanently.
func (s *Solver) resolveInstance(className string, args []types.Type, sp span.Span) ir.Core {
	if err := s.env.EnterResolve(); err != nil {
		s.env.AddCodedError(diagnostic.ErrResolutionDepth, sp,
			fmt.Sprintf("instance resolution depth limit exceeded for %s %s (possible infinite loop in instance contexts)",
				className, s.prettyTypeArgs(args)))
		return &ir.Var{Name: "<resolution-depth>", S: sp}
	}
	defer s.env.LeaveResolve()

	if dict := s.resolveFromContext(className, args, sp); dict != nil {
		return dict
	}
	if dict := s.resolveFromSuperclasses(className, args, sp); dict != nil {
		return dict
	}
	if dict := s.resolveFromQuantifiedEvidence(className, args, sp); dict != nil {
		return dict
	}
	if dict := s.resolveFromGlobalInstances(className, args, sp); dict != nil {
		return dict
	}

	s.env.AddCodedError(diagnostic.ErrNoInstance, sp,
		fmt.Sprintf("no instance for %s %s", className, s.prettyTypeArgs(args)))
	return &ir.Var{Name: "<no-instance>", S: sp}
}

// matchesDictVar checks if a context variable is a dictionary for the given class and args.
func (s *Solver) matchesDictVar(v *env.CtxVar, className string, args []types.Type) bool {
	ty := s.env.Zonk(v.Type)
	head, tyArgs := types.UnwindApp(ty)
	if con, ok := head.(*types.TyCon); ok && con.Name == env.DictName(className) {
		if len(tyArgs) != len(args) {
			return false
		}
		return s.env.WithTrial(func() bool {
			for i := range args {
				if err := s.env.Unify(tyArgs[i], args[i]); err != nil {
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
	solver      *Solver
	targetClass string
	targetArgs  []types.Type
	s           span.Span
	visited     map[string]bool
}

// extractSuperDict checks if a context variable is a dict for a class that
// has the target class as a (possibly transitive) superclass.
func (s *Solver) extractSuperDict(v *env.CtxVar, targetClass string, targetArgs []types.Type, sp span.Span) ir.Core {
	ty := s.env.Zonk(v.Type)
	head, tyArgs := types.UnwindApp(ty)
	con, ok := head.(*types.TyCon)
	if !ok {
		return nil
	}
	parentClass, isDict := s.env.ClassFromDict(con.Name)
	if !isDict {
		return nil
	}
	// Fast rejection: target class not in this class's transitive superclass set.
	if classInfo, ok := s.env.LookupClass(parentClass); ok {
		if classInfo.SuperClosure != nil && !classInfo.SuperClosure[targetClass] {
			return nil
		}
	}
	search := &superDictSearch{
		solver: s, targetClass: targetClass, targetArgs: targetArgs,
		s: sp, visited: make(map[string]bool),
	}
	return search.chain(&ir.Var{Name: v.Name, Module: v.Module, S: sp}, con.Name, tyArgs)
}

// chain recursively searches the superclass hierarchy for the target class,
// building chained Case extractions along the path.
func (sd *superDictSearch) chain(dictExpr ir.Core, dictTyName string, dictTyArgs []types.Type) ir.Core {
	parentClass, ok := sd.solver.env.ClassFromDict(dictTyName)
	if !ok {
		return nil
	}
	if sd.visited[parentClass] {
		return nil
	}
	sd.visited[parentClass] = true

	classInfo, ok := sd.solver.env.LookupClass(parentClass)
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

		extractExpr := sd.solver.ExtractDictField(classInfo, dictExpr, superIdx, "sf", sd.s)

		// Direct match: this superclass IS the target.
		if sup.ClassName == sd.targetClass && len(superArgs) == len(sd.targetArgs) {
			if sd.solver.env.WithTrial(func() bool {
				for j := range sd.targetArgs {
					if err := sd.solver.env.Unify(superArgs[j], sd.targetArgs[j]); err != nil {
						return false
					}
				}
				return true
			}) {
				return extractExpr
			}
		}

		// Transitive: search within this superclass's dict.
		result := sd.chain(extractExpr, env.DictName(sup.ClassName), superArgs)
		if result != nil {
			return result
		}
	}
	return nil
}

func (s *Solver) prettyTypeArgs(args []types.Type) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = types.PrettyAtom(s.env.Zonk(a))
	}
	return strings.Join(parts, " ")
}
