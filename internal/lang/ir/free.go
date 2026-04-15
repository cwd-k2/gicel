package ir

import (
	"fmt"
	"sort"
)

// FreeVars returns term-level free variables in a Core expression.
// The second return value is true when the traversal exceeded the depth
// limit and the result may be incomplete — callers must handle overflow
// conservatively (e.g. skip optimizations that depend on FV completeness).
func FreeVars(c Core) (map[VarKey]struct{}, bool) {
	result := traverseFV(c, 0, nil)
	if result.overflow {
		return nil, true
	}
	return fvResultToMap(result), false
}

// fvResultToMap materializes a traversal-local fvResult into the public
// map representation. Unlike fvResultToSlice (which filters qualified
// keys for closure capture), this preserves all VarKeys — callers like
// the optimizer need qualified keys for cross-module reference detection.
func fvResultToMap(r fvResult) map[VarKey]struct{} {
	if r.vars != nil {
		return r.vars
	}
	if r.single == (VarKey{}) {
		return make(map[VarKey]struct{})
	}
	return map[VarKey]struct{}{r.single: {}}
}

// FVInfo carries free-variable metadata for a single annotation point
// (one Lam, one Thunk, or one side of a Merge).
//
//   - Overflow == true  → the FV computation exceeded the traversal depth
//     limit. Vars and Indices are not meaningful; the evaluator must
//     capture the entire enclosing environment instead of trimming.
//   - Overflow == false → Vars holds the sorted free-variable names.
//     Indices is nil until AssignIndices runs, after which it holds the
//     de Bruijn positions in the enclosing env (possibly empty when all
//     free variables are globals).
type FVInfo struct {
	Vars     []string
	Indices  []int
	Overflow bool
}

// MergeFVInfo carries FV metadata for the two sides of a Merge node.
type MergeFVInfo struct {
	Left  FVInfo
	Right FVInfo
}

// FVAnnotations is the side table that holds free-variable metadata for
// every Lam, Thunk, and Merge node in a Core tree. It is populated by
// AnnotateFreeVars (names) and then by AssignIndices (de Bruijn positions).
//
// Pointer-valued map entries let AssignIndices mutate FVInfo in place
// without rewriting the map, and let downstream consumers rely on stable
// pointer identity when caching per-annotation compiled artifacts.
//
// FVAnnotations is the single source of truth: the IR nodes themselves
// carry no FV state, so the same *ir.Lam always has the same structural
// meaning regardless of which pass has run.
type FVAnnotations struct {
	Lams   map[*Lam]*FVInfo
	Thunks map[*Thunk]*FVInfo
	Merges map[*Merge]*MergeFVInfo
}

// NewFVAnnotations allocates an empty annotation table.
func NewFVAnnotations() *FVAnnotations {
	return &FVAnnotations{
		Lams:   make(map[*Lam]*FVInfo),
		Thunks: make(map[*Thunk]*FVInfo),
		Merges: make(map[*Merge]*MergeFVInfo),
	}
}

// LookupLam returns the FV info for a Lam node. Panics if the node is not
// registered — every Lam reachable through a traversed Core tree must have
// an entry after AnnotateFreeVars, so a missing entry indicates a bug.
func (a *FVAnnotations) LookupLam(lam *Lam) *FVInfo {
	info, ok := a.Lams[lam]
	if !ok {
		panic(fmt.Sprintf("ir.FVAnnotations: Lam node missing from annotation table (param=%q, span=%v)", lam.Param, lam.S))
	}
	return info
}

// LookupThunk returns the FV info for a Thunk node.
func (a *FVAnnotations) LookupThunk(th *Thunk) *FVInfo {
	info, ok := a.Thunks[th]
	if !ok {
		panic(fmt.Sprintf("ir.FVAnnotations: Thunk node missing from annotation table (span=%v)", th.S))
	}
	return info
}

// LookupMerge returns the FV info for a Merge node (both sides).
func (a *FVAnnotations) LookupMerge(m *Merge) *MergeFVInfo {
	info, ok := a.Merges[m]
	if !ok {
		panic(fmt.Sprintf("ir.FVAnnotations: Merge node missing from annotation table (span=%v)", m.S))
	}
	return info
}

// AnnotateFreeVars computes free-variable metadata for every Lam, Thunk,
// and Merge node reachable from c in a single bottom-up pass (O(n)).
// For each Lam, FV = free vars of body ∖ {param}. For each Thunk, FV = free
// vars of comp. For each Merge, FV is computed independently for each side.
//
// Returns a freshly allocated FVAnnotations owned by the caller.
func AnnotateFreeVars(c Core) *FVAnnotations {
	annots := NewFVAnnotations()
	traverseFV(c, 0, annots)
	return annots
}

// AnnotateFreeVarsProgram computes FV metadata for every binding in p.
// Returns a single FVAnnotations spanning all bindings.
func AnnotateFreeVarsProgram(p *Program) *FVAnnotations {
	annots := NewFVAnnotations()
	for _, b := range p.Bindings {
		traverseFV(b.Expr, 0, annots)
	}
	return annots
}

// fvInfoFromResult converts a traversal-local fvResult into a persistent
// FVInfo. Overflow results carry no Vars; non-overflow results carry the
// sorted free-variable name slice.
func fvInfoFromResult(r fvResult) *FVInfo {
	if r.overflow {
		return &FVInfo{Overflow: true}
	}
	return &FVInfo{Vars: fvResultToSlice(r)}
}

// fvResultToSlice materializes the inline-or-map fvResult into a sorted
// slice of variable names (bare names, not VarKeys). The single-var fast
// path returns a 1-element slice without going through map iteration.
// fvResultToSlice materializes the inline-or-map fvResult into a sorted
// slice of local variable names. Qualified (module-prefixed) variables
// are excluded — they are globals resolved via the global slot map,
// not captured in closure environments.
func fvResultToSlice(r fvResult) []string {
	if r.vars == nil {
		if r.single == (VarKey{}) {
			return []string{}
		}
		if !r.single.IsUnqualified() {
			return []string{} // qualified var — global, not captured
		}
		return []string{r.single.Name}
	}
	return setToSlice(r.vars)
}

// deletePatternBindings removes every binding-introducing name in p from
// r. Walks the pattern structure directly without calling Pattern.Bindings,
// which would allocate a fresh slice per PVar visit. This is the hot path
// for traverseFV's Case alternative processing on Prelude (~131K allocs of
// 1-element slices observed before this helper was introduced).
func deletePatternBindings(r *fvResult, p Pattern) {
	switch pat := p.(type) {
	case *PVar:
		r.deleteByName(pat.Name)
	case *PCon:
		for _, arg := range pat.Args {
			deletePatternBindings(r, arg)
		}
	case *PRecord:
		for _, f := range pat.Fields {
			deletePatternBindings(r, f.Pattern)
		}
	}
	// PWild and PLit bind no names — nothing to delete.
}

// fvResult carries the free variable set and an overflow flag.
// When overflow is true, the FV computation was truncated by the depth
// limit; ancestor Lam/Thunk nodes must disable environment trimming
// rather than silently losing deep free variables.
//
// Single-var optimization: when the result holds exactly one variable
// the key is stored inline in `single` and `vars` stays nil. The map
// is allocated only on the first promotion to two-or-more vars (in
// mergeFV). The cold-start profile showed traverseFV's *Var arm
// allocating a fresh single-element map per Var visit (213K objects,
// 3.32% of total); the inline path eliminates that without changing
// observer-visible semantics.
//
// Invariants:
//   - overflow == true                    → result is "unknown / truncated"
//   - vars != nil                         → multi-var; vars holds all entries
//   - vars == nil && single != VarKey{}   → exactly one var
//   - vars == nil && single == VarKey{}   → empty
type fvResult struct {
	single   VarKey
	vars     map[VarKey]struct{}
	overflow bool
}

var fvOverflow = fvResult{overflow: true}

// deleteByName removes a bare-name variable from the result. Used for
// binder removal (Lam param, Fix name, Bind var, pattern bindings) where
// the bound name is always unqualified.
func (r *fvResult) deleteByName(name string) {
	if r.vars != nil {
		// Bare names match local keys; qualified keys are never bound
		// by Lam/Fix/Bind/Case pattern binders.
		delete(r.vars, LocalKey(name))
		return
	}
	if r.single.Name == name && r.single.IsUnqualified() {
		r.single = VarKey{}
	}
}

// traverseFVLeftSpine iteratively descends the left spine of App nodes,
// merging free variable sets from right children.
func traverseFVLeftSpine(app *App, depth int, annots *FVAnnotations) fvResult {
	head, rights := unwindLeftSpine(app)
	result := traverseFV(head, depth+1, annots)
	for i := len(rights) - 1; i >= 0; i-- {
		result = mergeFV(result, traverseFV(rights[i], depth+1, annots))
	}
	return result
}

// traverseFV computes free variables bottom-up, writing annotation metadata
// at each Lam, Thunk, and Merge node into annots (when non-nil).
//
// Lam params are removed from the result but NOT propagated as bound —
// outer Lam params are free from an inner closure's perspective (they are
// captured). Fix names, Case alt bindings, and Bind vars are scoped within
// their body and removed from the result.
//
// When the depth limit is reached, returns fvOverflow so that ancestor
// Lam/Thunk nodes detect the truncation and disable environment trimming
// rather than silently losing deep free variables.
//
// When annots is nil (called from FreeVars), annotation writes are skipped
// and only the top-level free variable set is computed.
func traverseFV(c Core, depth int, annots *FVAnnotations) fvResult {
	if depth > maxTraversalDepth {
		return fvOverflow
	}
	switch n := c.(type) {
	case *Var:
		// Inline single-var path: no map allocation until mergeFV
		// promotes the result to multi-var.
		return fvResult{single: VarKeyOf(n)}
	case *Lam:
		bodyFV := traverseFV(n.Body, depth+1, annots)
		// Remove the param — it comes from application, not from captured env.
		bodyFV.deleteByName(n.Param)
		if annots != nil {
			annots.Lams[n] = fvInfoFromResult(bodyFV)
		}
		if bodyFV.overflow {
			return fvOverflow
		}
		return bodyFV
	case *App:
		return traverseFVLeftSpine(n, depth, annots)
	case *TyApp:
		return traverseFV(n.Expr, depth+1, annots)
	case *TyLam:
		return traverseFV(n.Body, depth+1, annots)
	case *Con:
		var result fvResult
		for _, arg := range n.Args {
			result = mergeFV(result, traverseFV(arg, depth+1, annots))
		}
		return result
	case *Case:
		result := traverseFV(n.Scrutinee, depth+1, annots)
		for _, alt := range n.Alts {
			altFV := traverseFV(alt.Body, depth+1, annots)
			// Remove pattern-bound vars — they are local to each alt.
			deletePatternBindings(&altFV, alt.Pattern)
			result = mergeFV(result, altFV)
		}
		return result
	case *Fix:
		// Fix name is visible in Body — remove it from the result.
		result := traverseFV(n.Body, depth+1, annots)
		result.deleteByName(n.Name)
		return result
	case *Pure:
		return traverseFV(n.Expr, depth+1, annots)
	case *Bind:
		compFV := traverseFV(n.Comp, depth+1, annots)
		bodyFV := traverseFV(n.Body, depth+1, annots)
		// Bind var is local to the body.
		bodyFV.deleteByName(n.Var)
		return mergeFV(compFV, bodyFV)
	case *Thunk:
		compFV := traverseFV(n.Comp, depth+1, annots)
		if annots != nil {
			annots.Thunks[n] = fvInfoFromResult(compFV)
		}
		if compFV.overflow {
			return fvOverflow
		}
		return compFV
	case *Force:
		return traverseFV(n.Expr, depth+1, annots)
	case *Merge:
		leftFV := traverseFV(n.Left, depth+1, annots)
		rightFV := traverseFV(n.Right, depth+1, annots)
		if annots != nil {
			annots.Merges[n] = &MergeFVInfo{
				Left:  *fvInfoFromResult(leftFV),
				Right: *fvInfoFromResult(rightFV),
			}
		}
		return mergeFV(leftFV, rightFV)
	case *PrimOp:
		var result fvResult
		for _, arg := range n.Args {
			result = mergeFV(result, traverseFV(arg, depth+1, annots))
		}
		return result
	case *Lit:
		return fvResult{}
	case *Error:
		return fvResult{}
	case *RecordLit:
		var result fvResult
		for _, f := range n.Fields {
			result = mergeFV(result, traverseFV(f.Value, depth+1, annots))
		}
		return result
	case *RecordProj:
		return traverseFV(n.Record, depth+1, annots)
	case *RecordUpdate:
		result := traverseFV(n.Record, depth+1, annots)
		for _, f := range n.Updates {
			result = mergeFV(result, traverseFV(f.Value, depth+1, annots))
		}
		return result
	case *VariantLit:
		return traverseFV(n.Value, depth+1, annots)
	default:
		panic(fmt.Sprintf("traverseFV: unhandled Core node %T", c))
	}
}

// mergeFV combines two free-variable result sets, promoting the inline
// single-var representation to a map only when the merged set has 2+
// distinct variables. The arguments are consumed by-value but the
// returned result may share or mutate the map of one argument when
// possible to avoid extra copies.
func mergeFV(a, b fvResult) fvResult {
	if a.overflow || b.overflow {
		return fvOverflow
	}
	// Empty + anything → anything.
	aEmpty := a.vars == nil && a.single == (VarKey{})
	bEmpty := b.vars == nil && b.single == (VarKey{})
	if aEmpty {
		return b
	}
	if bEmpty {
		return a
	}
	// Single + single: stay inline if same key, promote otherwise.
	if a.vars == nil && b.vars == nil {
		if a.single == b.single {
			return a
		}
		return fvResult{vars: map[VarKey]struct{}{a.single: {}, b.single: {}}}
	}
	// Single + multi: add a's single into b's map (mutating b).
	if a.vars == nil {
		b.vars[a.single] = struct{}{}
		return b
	}
	// Multi + single: add b's single into a's map (mutating a).
	if b.vars == nil {
		a.vars[b.single] = struct{}{}
		return a
	}
	// Multi + multi: copy b's keys into a (mutating a).
	for k := range b.vars {
		a.vars[k] = struct{}{}
	}
	return a
}

// setToSlice extracts bare names from a VarKey set for FVInfo.Vars.
// Only local (unqualified) variables are included — qualified module
// references are globals resolved by VarKey in the global slot map,
// not captured in closure environments.
func setToSlice(s map[VarKey]struct{}) []string {
	if len(s) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(s))
	for k := range s {
		if k.IsUnqualified() {
			result = append(result, k.Name)
		}
	}
	sort.Strings(result)
	return result
}

// VarKey is the canonical environment lookup key for a variable.
// Module is empty for local/unqualified variables.
type VarKey struct {
	Module string
	Name   string
}

// IsUnqualified reports whether this key lacks a module qualifier.
// Unqualified keys arise from the current compilation unit: local
// binders (Lam, Fix, Bind, Case patterns) and top-level main bindings.
// Qualified keys originate from imported modules.
func (k VarKey) IsUnqualified() bool { return k.Module == "" }

// VarKeyOf returns the environment lookup key for a Var node.
func VarKeyOf(v *Var) VarKey {
	return VarKey{Module: v.Module, Name: v.Name}
}

// QualifiedKey builds a qualified environment key from module and name.
func QualifiedKey(module, name string) VarKey {
	return VarKey{Module: module, Name: name}
}

// LocalKey builds an unqualified environment key from a bare name.
func LocalKey(name string) VarKey {
	return VarKey{Name: name}
}
