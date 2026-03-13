package check

import (
	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/types"
)

// evidenceSource indicates where available evidence comes from.
type evidenceSource int

const (
	evidenceContext    evidenceSource = iota // from CtxEvidence in context
	evidenceSuperclass                      // extracted from superclass dict
	evidenceGlobal                          // from global instance
)

// availableEvidence represents a single piece of type class evidence
// available for constraint resolution.
type availableEvidence struct {
	className string
	args      []types.Type
	dictExpr  core.Core
	source    evidenceSource
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
	for i := len(ch.ctx.entries) - 1; i >= 0; i-- {
		if e, ok := ch.ctx.entries[i].(*CtxEvidence); ok {
			result = append(result, availableEvidence{
				className: e.ClassName,
				args:      e.Args,
				dictExpr:  &core.Var{Name: e.DictName},
				source:    evidenceContext,
			})
		}
	}
	return result
}

// classifyEvidence partitions wanted constraints against available evidence.
// Parallel to classifyFields in unifyRows.
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
			saved := ch.saveUnifierState()
			allMatch := true
			for j := range w.args {
				wArg := ch.unifier.Zonk(w.args[j])
				aArg := ch.unifier.Zonk(a.args[j])
				if err := ch.unifier.Unify(wArg, aArg); err != nil {
					allMatch = false
					break
				}
			}
			if !allMatch {
				ch.restoreUnifierState(saved)
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
