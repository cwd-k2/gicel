package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Multiplicity enforcement ---

// multStep records a single do-chain step's inferred pre/post for multiplicity analysis.
type multStep struct {
	pre  types.Type
	post types.Type
	s    span.Span
}

// checkMultiplicity verifies multiplicity constraints on @Mult-annotated labels.
// For each label annotated with @Linear or @Affine, counts the number of
// same-type preservation events (steps where the label appears in both pre
// and post with structurally equal types). Type-changing preservations
// (protocol state transitions) and consumption events do not count.
func (ch *Checker) checkMultiplicity(comp *types.TyCBPV, steps []multStep, s span.Span) {
	if len(steps) == 0 {
		return
	}

	// Collect all @Mult-annotated labels from step pre/post states
	// and the overall computation's pre-state.
	mults := make(map[string]types.Type) // label → zonked multiplicity
	for _, step := range steps {
		collectMultLabels(ch, step.pre, mults)
		collectMultLabels(ch, step.post, mults)
	}
	collectMultLabels(ch, comp.Pre, mults)

	if len(mults) == 0 {
		return
	}

	// For each @Mult label, count same-type preservations.
	for label, mult := range mults {
		limit := multLimit(mult)
		if limit < 0 {
			continue
		}

		count := 0
		for _, step := range steps {
			preTy := capFieldType(ch, step.pre, label)
			postTy := capFieldType(ch, step.post, label)
			if preTy != nil && postTy != nil && types.Equal(preTy, postTy) {
				count++
			}
		}

		if count > limit {
			ch.addCodedError(diagnostic.ErrMultiplicity, s,
				fmt.Sprintf("@%s capability %q accessed %d times (maximum %d)",
					types.Pretty(mult), label, count, limit))
		}
	}
}

// collectMultLabels extracts @Mult-annotated labels from a zonked capability row.
func collectMultLabels(ch *Checker, ty types.Type, out map[string]types.Type) {
	ty = ch.unifier.Zonk(ty)
	ev, ok := ty.(*types.TyEvidenceRow)
	if !ok {
		return
	}
	cap, ok := ev.Entries.(*types.CapabilityEntries)
	if !ok {
		return
	}
	for _, f := range cap.Fields {
		if f.Mult != nil {
			if _, exists := out[f.Label]; !exists {
				out[f.Label] = ch.unifier.Zonk(f.Mult)
			}
		}
	}
}

// capFieldType returns the zonked type for a label in a capability row, or nil.
func capFieldType(ch *Checker, ty types.Type, label string) types.Type {
	ty = ch.unifier.Zonk(ty)
	ev, ok := ty.(*types.TyEvidenceRow)
	if !ok {
		return nil
	}
	cap, ok := ev.Entries.(*types.CapabilityEntries)
	if !ok {
		return nil
	}
	for _, f := range cap.Fields {
		if f.Label == label {
			return ch.unifier.Zonk(f.Type)
		}
	}
	return nil
}

// multLimit returns the maximum allowed same-type preservations for a multiplicity.
// Returns -1 for unrestricted (no limit).
func multLimit(mult types.Type) int {
	if con, ok := mult.(*types.TyCon); ok {
		switch con.Name {
		case "Linear":
			return 1
		case "Affine":
			return 1
		}
	}
	return -1
}
