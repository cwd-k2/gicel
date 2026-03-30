package optimize

import (
	"strings"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// maxInlineSize is the maximum node count for a binding to be considered
// an inline candidate.
const maxInlineSize = 20

// inlineCandidate holds a binding eligible for inlining.
type inlineCandidate struct {
	body ir.Core
}

// collectInlineCandidates scans program bindings for small, non-recursive
// definitions that can be safely inlined at call sites.
func collectInlineCandidates(prog *ir.Program) map[string]*inlineCandidate {
	candidates := make(map[string]*inlineCandidate)
	for _, b := range prog.Bindings {
		// Skip compiler-generated bindings (dictionary constructors, etc.)
		if strings.Contains(b.Name, "$") {
			continue
		}
		// Only inline lambda bodies — avoids inlining dictionary selectors,
		// class methods, and other non-function bindings.
		if _, ok := b.Expr.(*ir.Lam); !ok {
			continue
		}
		size := nodeSize(b.Expr, maxInlineSize+1)
		if size > maxInlineSize {
			continue
		}
		fv := ir.FreeVars(b.Expr)
		if _, recursive := fv[b.Name]; recursive {
			continue
		}
		candidates[b.Name] = &inlineCandidate{body: b.Expr}
	}
	return candidates
}

// inlineRule returns a rewrite rule that replaces variable references
// with their inlined bodies.
func inlineRule(candidates map[string]*inlineCandidate) func(ir.Core) ir.Core {
	if len(candidates) == 0 {
		return func(c ir.Core) ir.Core { return c }
	}
	return func(c ir.Core) ir.Core {
		v, ok := c.(*ir.Var)
		if !ok {
			return c
		}
		cand, ok := candidates[v.Name]
		if !ok {
			return c
		}
		return ir.Clone(cand.body)
	}
}
