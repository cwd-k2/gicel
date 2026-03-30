package optimize

import "github.com/cwd-k2/gicel/internal/lang/ir"

// maxDuplicateSize is the maximum total node count of the outer case
// alternatives before case-of-case transformation is suppressed to
// prevent code blowup.
const maxDuplicateSize = 50

// caseOfCase transforms case (case e innerAlts) outerAlts by pushing
// the outer case into each branch of the inner case.
//
// Before: case (case e { p1 => b1; p2 => b2 }) { q1 => w1; q2 => w2 }
// After:  case e { p1 => case b1 { q1 => w1; q2 => w2 }
//                   p2 => case b2 { q1 => w1; q2 => w2 } }
func caseOfCase(c ir.Core) ir.Core {
	outer, ok := c.(*ir.Case)
	if !ok {
		return c
	}
	inner, ok := outer.Scrutinee.(*ir.Case)
	if !ok {
		return c
	}

	// Guard against code blowup: measure outer alternatives total size.
	outerSize := 0
	for _, alt := range outer.Alts {
		outerSize += nodeSize(alt.Body, maxDuplicateSize+1)
		if outerSize > maxDuplicateSize {
			return c
		}
	}

	// Push outer case into each inner branch.
	newAlts := make([]ir.Alt, len(inner.Alts))
	for i, alt := range inner.Alts {
		newAlts[i] = ir.Alt{
			Pattern: alt.Pattern,
			Body: &ir.Case{
				Scrutinee: alt.Body,
				Alts:      cloneAlts(outer.Alts),
				S:         outer.S,
			},
			Generated: alt.Generated,
			S:         alt.S,
		}
	}
	return &ir.Case{Scrutinee: inner.Scrutinee, Alts: newAlts, S: inner.S}
}

// cloneAlts deep-copies a slice of alternatives.
func cloneAlts(alts []ir.Alt) []ir.Alt {
	out := make([]ir.Alt, len(alts))
	for i, a := range alts {
		out[i] = ir.Alt{
			Pattern:   a.Pattern,
			Body:      ir.Clone(a.Body),
			Generated: a.Generated,
			S:         a.S,
		}
	}
	return out
}
