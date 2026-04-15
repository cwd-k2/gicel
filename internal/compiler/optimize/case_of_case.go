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
//
//	p2 => case b2 { q1 => w1; q2 => w2 } }
//
// Capture avoidance: inner pattern bindings that collide with free
// variables of the outer alternatives are alpha-renamed to prevent
// accidental variable capture after the transformation.
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

	// Collect free variables of all outer alt bodies (bare names). These
	// are the names that must not be captured by inner pattern bindings.
	outerFV := make(map[string]struct{})
	for _, alt := range outer.Alts {
		for k := range ir.FreeVars(alt.Body) {
			outerFV[k.Name] = struct{}{}
		}
	}

	// Push outer case into each inner branch, alpha-renaming inner
	// pattern bindings that would capture outer free variables.
	newAlts := make([]ir.Alt, len(inner.Alts))
	for i, alt := range inner.Alts {
		pat, body := avoidCapture(alt.Pattern, alt.Body, outerFV)
		newAlts[i] = ir.Alt{
			Pattern: pat,
			Body: &ir.Case{
				Scrutinee: body,
				Alts:      cloneAlts(outer.Alts),
				S:         outer.S,
			},
			Generated: alt.Generated,
			S:         alt.S,
		}
	}
	return &ir.Case{Scrutinee: inner.Scrutinee, Alts: newAlts, S: inner.S}
}

// avoidCapture alpha-renames any bindings in pat whose names collide with
// freeNames, applying the same renaming to body.
func avoidCapture(pat ir.Pattern, body ir.Core, freeNames map[string]struct{}) (ir.Pattern, ir.Core) {
	bindings := pat.Bindings()
	var renames [][2]string // (old, new) pairs
	for _, name := range bindings {
		if _, captured := freeNames[name]; captured {
			renames = append(renames, [2]string{name, freshName(name)})
		}
	}
	if len(renames) == 0 {
		return pat, body
	}
	for _, r := range renames {
		pat = renamePatternVar(pat, r[0], r[1])
		fv := map[string]struct{}{r[1]: {}}
		body = substMany(body, map[string]ir.Core{r[0]: &ir.Var{Name: r[1], Index: -1, S: body.Span()}}, fv)
	}
	return pat, body
}

// bindOfCase transforms bind (case e alts) x body by pushing the bind
// continuation into each branch of the case.
//
// Before: bind (case e { p1 => b1; p2 => b2 }) x body
// After:  case e { p1 => bind b1 x body; p2 => bind b2 x body }
func bindOfCase(c ir.Core) ir.Core {
	bind, ok := c.(*ir.Bind)
	if !ok {
		return c
	}
	inner, ok := bind.Comp.(*ir.Case)
	if !ok {
		return c
	}

	// Guard against code blowup: measure outer body size.
	bodySize := nodeSize(bind.Body, maxDuplicateSize+1)
	if bodySize > maxDuplicateSize {
		return c
	}

	// Collect free variables of the bind body for capture avoidance
	// (bare names, since pattern binders are unqualified).
	bindBodyFVRaw := ir.FreeVars(bind.Body)
	bindBodyFV := make(map[string]struct{}, len(bindBodyFVRaw))
	for k := range bindBodyFVRaw {
		bindBodyFV[k.Name] = struct{}{}
	}

	// Push bind into each branch, alpha-renaming inner pattern
	// bindings that would capture free variables of the bind body.
	newAlts := make([]ir.Alt, len(inner.Alts))
	for i, alt := range inner.Alts {
		pat, comp := avoidCapture(alt.Pattern, alt.Body, bindBodyFV)
		newAlts[i] = ir.Alt{
			Pattern: pat,
			Body: &ir.Bind{
				Comp:      comp,
				Var:       bind.Var,
				IsDiscard:   bind.IsDiscard,
				Body:      ir.Clone(bind.Body),
				Generated: bind.Generated,
				S:         bind.S,
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
