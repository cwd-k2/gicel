package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// validateAliasGraph checks for cyclic type aliases using DFS three-color marking.
// White (unvisited), gray (in current path), black (fully processed).
// If a gray node is encountered during traversal, a cycle exists.
// Returns true if any cycle was found.
func (ch *Checker) validateAliasGraph() bool {
	type color int
	const (
		white color = iota
		gray
		black
	)

	colors := make(map[string]color, len(ch.aliases))
	for name := range ch.aliases {
		colors[name] = white
	}

	// path tracks the current DFS stack for error reporting.
	var path []string

	var visit func(name string) bool
	visit = func(name string) bool {
		switch colors[name] {
		case black:
			return false
		case gray:
			// Found a cycle. Build the cycle description from path.
			cycleStart := 0
			for i, p := range path {
				if p == name {
					cycleStart = i
					break
				}
			}
			cycle := append(path[cycleStart:], name)
			ch.addCodedError(errs.ErrCyclicAlias, span.Span{}, fmt.Sprintf(
				"cyclic type alias: %s", strings.Join(cycle, " -> ")))
			return true
		}

		colors[name] = gray
		path = append(path, name)

		info := ch.aliases[name]
		refs := collectAliasRefs(info.body, ch.aliases)
		for _, ref := range refs {
			if visit(ref) {
				return true
			}
		}

		path = path[:len(path)-1]
		colors[name] = black
		return false
	}

	hasCycle := false
	for name := range ch.aliases {
		if colors[name] == white {
			if visit(name) {
				hasCycle = true
			}
		}
	}
	return hasCycle
}

// collectAliasRefs returns the names of all TyCon nodes in ty that are also alias names.
func collectAliasRefs(ty types.Type, aliases map[string]*aliasInfo) []string {
	var refs []string
	seen := make(map[string]bool)
	collectAliasRefsRec(ty, aliases, seen, &refs)
	return refs
}

func collectAliasRefsRec(ty types.Type, aliases map[string]*aliasInfo, seen map[string]bool, refs *[]string) {
	if ty == nil {
		return
	}
	switch t := ty.(type) {
	case *types.TyCon:
		if _, ok := aliases[t.Name]; ok && !seen[t.Name] {
			seen[t.Name] = true
			*refs = append(*refs, t.Name)
		}
	case *types.TyApp:
		collectAliasRefsRec(t.Fun, aliases, seen, refs)
		collectAliasRefsRec(t.Arg, aliases, seen, refs)
	case *types.TyArrow:
		collectAliasRefsRec(t.From, aliases, seen, refs)
		collectAliasRefsRec(t.To, aliases, seen, refs)
	case *types.TyForall:
		collectAliasRefsRec(t.Body, aliases, seen, refs)
	case *types.TyComp:
		collectAliasRefsRec(t.Pre, aliases, seen, refs)
		collectAliasRefsRec(t.Post, aliases, seen, refs)
		collectAliasRefsRec(t.Result, aliases, seen, refs)
	case *types.TyThunk:
		collectAliasRefsRec(t.Pre, aliases, seen, refs)
		collectAliasRefsRec(t.Post, aliases, seen, refs)
		collectAliasRefsRec(t.Result, aliases, seen, refs)
	case *types.TyEvidenceRow:
		for _, ch := range t.Entries.AllChildren() {
			collectAliasRefsRec(ch, aliases, seen, refs)
		}
		if t.Tail != nil {
			collectAliasRefsRec(t.Tail, aliases, seen, refs)
		}
	case *types.TyEvidence:
		collectAliasRefsRec(t.Constraints, aliases, seen, refs)
		collectAliasRefsRec(t.Body, aliases, seen, refs)
	case *types.TyVar, *types.TyMeta, *types.TyError:
		// No alias references possible.
	}
}

// installAliasExpander sets up the unifier's alias expansion callback.
// Called after alias validation, before instance processing.
func (ch *Checker) installAliasExpander() {
	if len(ch.aliases) == 0 {
		return
	}
	ch.unifier.aliasExpander = func(ty types.Type) types.Type {
		return ch.expandTypeAliases(ty)
	}
}

const maxAliasExpansionDepth = 256

// expandTypeAliases expands fully-applied type aliases in a type.
func (ch *Checker) expandTypeAliases(ty types.Type) types.Type {
	return ch.expandTypeAliasesN(ty, 0)
}

func (ch *Checker) expandTypeAliasesN(ty types.Type, depth int) types.Type {
	if depth > maxAliasExpansionDepth {
		ch.addCodedError(errs.ErrAliasExpansion, span.Span{},
			"type alias expansion depth limit exceeded (possible cyclic or deeply nested alias)")
		return ty
	}
	app, ok := ty.(*types.TyApp)
	if !ok {
		return ty
	}
	// Collect the spine: TyApp(TyApp(...(head, arg1), arg2), ...)
	head, args := types.UnwindApp(ty)
	con, ok := head.(*types.TyCon)
	if !ok {
		return ty
	}
	info, ok := ch.aliases[con.Name]
	if !ok || len(info.params) != len(args) {
		// Not a fully-applied alias. Check if partial application could be expanded
		// by recursing into sub-expressions.
		newFun := ch.expandTypeAliasesN(app.Fun, depth+1)
		newArg := ch.expandTypeAliasesN(app.Arg, depth+1)
		if newFun == app.Fun && newArg == app.Arg {
			return ty
		}
		result := &types.TyApp{Fun: newFun, Arg: newArg, S: app.S}
		// Re-check after recursive expansion.
		return ch.expandTypeAliasesN(result, depth+1)
	}
	// Expand: substitute params with args in the alias body.
	body := info.body
	for i, p := range info.params {
		body = types.Subst(body, p, args[i])
	}
	// Recursively expand nested aliases.
	return ch.expandTypeAliasesN(body, depth+1)
}
