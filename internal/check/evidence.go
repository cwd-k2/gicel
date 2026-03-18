package check

import (
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/types"
)

// availableEvidence represents a single piece of type class evidence
// available for constraint resolution.
type availableEvidence struct {
	className string
	args      []types.Type
	dictExpr  core.Core
}

// resolvedEvidence is a matched constraint with its dictionary expression.
type resolvedEvidence struct {
	placeholder string
	dictExpr    core.Core
}

// collectContextEvidence gathers all available evidence from context.
// Parallel to collecting row fields from a row type.
func (ch *Checker) collectContextEvidence() []availableEvidence {
	var result []availableEvidence
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if e, ok := entry.(*CtxEvidence); ok {
			result = append(result, availableEvidence{
				className: e.ClassName,
				args:      e.Args,
				dictExpr:  &core.Var{Name: e.DictName}, // dict params are local (lambda-bound)
			})
		}
		return true
	})
	return result
}

// classifyEvidence partitions wanted constraints against available evidence.
// Parallel to types.ClassifyRowFields.
func (ch *Checker) classifyEvidence(
	wanted []deferredConstraint,
	available []availableEvidence,
) (matched []resolvedEvidence, unmatched []deferredConstraint) {
	// Build index by className.
	availByClass := make(map[string][]int)
	for i, a := range available {
		availByClass[a.className] = append(availByClass[a.className], i)
	}
	availUsed := make([]bool, len(available))

	for _, w := range wanted {
		found := false
		for _, ai := range availByClass[w.className] {
			if availUsed[ai] {
				continue
			}
			a := available[ai]
			if len(a.args) != len(w.args) {
				continue
			}
			// Try to match by unifying args (trial unification with rollback).
			if !ch.withTrial(func() bool {
				for j := range w.args {
					wArg := ch.unifier.Zonk(w.args[j])
					aArg := ch.unifier.Zonk(a.args[j])
					if err := ch.unifier.Unify(wArg, aArg); err != nil {
						return false
					}
				}
				return true
			}) {
				continue
			}
			matched = append(matched, resolvedEvidence{
				placeholder: w.placeholder,
				dictExpr:    a.dictExpr,
			})
			availUsed[ai] = true
			found = true
			break
		}
		if !found {
			unmatched = append(unmatched, w)
		}
	}
	return
}
