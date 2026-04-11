package ir

import (
	"fmt"
	"sort"
	"strings"
)

// FreeVars returns term-level free variables in a Core expression.
func FreeVars(c Core) map[string]struct{} {
	fv := make(map[string]struct{})
	bound := make(map[string]int)
	freeVarsRec(c, bound, fv, 0)
	return fv
}

// bind/unbind use a depth counter to handle shadowing without map copies.
func bind(bound map[string]int, name string) { bound[name]++ }
func unbind(bound map[string]int, name string) {
	bound[name]--
	if bound[name] == 0 {
		delete(bound, name)
	}
}

func freeVarsRec(c Core, bound map[string]int, fv map[string]struct{}, depth int) {
	if depth > maxTraversalDepth {
		return
	}
	switch n := c.(type) {
	case *Var:
		// Use the cached key if available; otherwise compute locally.
		// Key is populated later by AnnotateFreeVars / AssignIndices.
		// FreeVars is a read-only traversal and must not mutate IR nodes.
		key := n.Key
		if key == "" {
			key = varKey(n)
		}
		if bound[key] == 0 {
			fv[key] = struct{}{}
		}
	case *Lam:
		bind(bound, n.Param)
		freeVarsRec(n.Body, bound, fv, depth+1)
		unbind(bound, n.Param)
	case *App:
		// Flatten left-spine of App to avoid stack overflow on deeply
		// left-associative operator chains.
		freeVarsLeftSpine(n, bound, fv, depth)
		return
	case *TyApp:
		freeVarsRec(n.Expr, bound, fv, depth+1)
	case *TyLam:
		freeVarsRec(n.Body, bound, fv, depth+1)
	case *Con:
		for _, arg := range n.Args {
			freeVarsRec(arg, bound, fv, depth+1)
		}
	case *Case:
		freeVarsRec(n.Scrutinee, bound, fv, depth+1)
		for _, alt := range n.Alts {
			// Walk the pattern directly to bind/unbind names without
			// allocating the slice that Pattern.Bindings would return
			// (PVar.Bindings is a particularly hot 1-element slice
			// allocation on Prelude).
			bindPatternBindings(bound, alt.Pattern)
			freeVarsRec(alt.Body, bound, fv, depth+1)
			unbindPatternBindings(bound, alt.Pattern)
		}
	case *Fix:
		bind(bound, n.Name)
		freeVarsRec(n.Body, bound, fv, depth+1)
		unbind(bound, n.Name)
	case *Pure:
		freeVarsRec(n.Expr, bound, fv, depth+1)
	case *Bind:
		freeVarsRec(n.Comp, bound, fv, depth+1)
		bind(bound, n.Var)
		freeVarsRec(n.Body, bound, fv, depth+1)
		unbind(bound, n.Var)
	case *Thunk:
		freeVarsRec(n.Comp, bound, fv, depth+1)
	case *Force:
		freeVarsRec(n.Expr, bound, fv, depth+1)
	case *Merge:
		freeVarsRec(n.Left, bound, fv, depth+1)
		freeVarsRec(n.Right, bound, fv, depth+1)
	case *PrimOp:
		for _, arg := range n.Args {
			freeVarsRec(arg, bound, fv, depth+1)
		}
	case *Lit:
		// leaf — no free variables
	case *Error:
		// error placeholder — no free variables
	case *RecordLit:
		for _, f := range n.Fields {
			freeVarsRec(f.Value, bound, fv, depth+1)
		}
	case *RecordProj:
		freeVarsRec(n.Record, bound, fv, depth+1)
	case *RecordUpdate:
		freeVarsRec(n.Record, bound, fv, depth+1)
		for _, f := range n.Updates {
			freeVarsRec(f.Value, bound, fv, depth+1)
		}
	case *VariantLit:
		freeVarsRec(n.Value, bound, fv, depth+1)
	default:
		panic(fmt.Sprintf("freeVarsRec: unhandled Core node %T", c))
	}
}

// freeVarsLeftSpine iteratively descends the left spine of App nodes
// (and transparent TyApp/TyLam wrappers), collecting free variables from
// right-side children. Prevents Go stack overflow on deeply left-nested
// operator chains.
func freeVarsLeftSpine(app *App, bound map[string]int, fv map[string]struct{}, depth int) {
	head, rights := unwindLeftSpine(app)
	freeVarsRec(head, bound, fv, depth+1)
	for i := len(rights) - 1; i >= 0; i-- {
		freeVarsRec(rights[i], bound, fv, depth+1)
	}
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
	annotateCore(c, annots)
	return annots
}

// AnnotateFreeVarsProgram computes FV metadata for every binding in p.
// Returns a single FVAnnotations spanning all bindings.
func AnnotateFreeVarsProgram(p *Program) *FVAnnotations {
	annots := NewFVAnnotations()
	for _, b := range p.Bindings {
		annotateCore(b.Expr, annots)
	}
	return annots
}

// annotateCore runs traverseFV with an observer that writes into annots.
func annotateCore(c Core, annots *FVAnnotations) {
	traverseFV(c, 0, annotateObs{annots: annots})
}

// fvObserver receives computed free variables at annotation points during
// bottom-up FV traversal. The traversal calls the observer at each Lam,
// Thunk, and Merge node with the computed FV result.
type fvObserver interface {
	OnLam(lam *Lam, bodyFV fvResult)
	OnThunk(th *Thunk, compFV fvResult)
	OnMerge(m *Merge, leftFV, rightFV fvResult)
}

// annotateObs writes FV results into a FVAnnotations side table.
type annotateObs struct {
	annots *FVAnnotations
}

func (a annotateObs) OnLam(lam *Lam, bodyFV fvResult) {
	a.annots.Lams[lam] = fvInfoFromResult(bodyFV)
}

func (a annotateObs) OnThunk(th *Thunk, compFV fvResult) {
	a.annots.Thunks[th] = fvInfoFromResult(compFV)
}

func (a annotateObs) OnMerge(m *Merge, leftFV, rightFV fvResult) {
	a.annots.Merges[m] = &MergeFVInfo{
		Left:  *fvInfoFromResult(leftFV),
		Right: *fvInfoFromResult(rightFV),
	}
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
// slice of variable names. The single-var fast path returns a 1-element
// slice without going through map iteration.
func fvResultToSlice(r fvResult) []string {
	if r.vars == nil {
		if r.single == "" {
			return []string{}
		}
		return []string{r.single}
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
		r.delete(pat.Name)
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

// bindPatternBindings increments the bound-count for every binding-introducing
// name in p. Used by freeVarsRec's Case alternative processing to avoid the
// slice allocation that Pattern.Bindings would incur.
func bindPatternBindings(bound map[string]int, p Pattern) {
	switch pat := p.(type) {
	case *PVar:
		bind(bound, pat.Name)
	case *PCon:
		for _, arg := range pat.Args {
			bindPatternBindings(bound, arg)
		}
	case *PRecord:
		for _, f := range pat.Fields {
			bindPatternBindings(bound, f.Pattern)
		}
	}
}

// unbindPatternBindings reverses bindPatternBindings.
func unbindPatternBindings(bound map[string]int, p Pattern) {
	switch pat := p.(type) {
	case *PVar:
		unbind(bound, pat.Name)
	case *PCon:
		for _, arg := range pat.Args {
			unbindPatternBindings(bound, arg)
		}
	case *PRecord:
		for _, f := range pat.Fields {
			unbindPatternBindings(bound, f.Pattern)
		}
	}
}

// fvResult carries the free variable set and an overflow flag.
// When overflow is true, the FV computation was truncated by the depth
// limit; ancestor Lam/Thunk nodes must disable environment trimming
// rather than silently losing deep free variables.
//
// Single-var optimization: when the result holds exactly one variable
// the name is stored inline in `single` and `vars` stays nil. The map
// is allocated only on the first promotion to two-or-more vars (in
// mergeFV). The cold-start profile showed traverseFV's *Var arm
// allocating a fresh single-element map per Var visit (213K objects,
// 3.32% of total); the inline path eliminates that without changing
// observer-visible semantics.
//
// Invariants:
//   - overflow == true                → result is "unknown / truncated"
//   - vars != nil                     → multi-var; vars holds all entries
//   - vars == nil && single != ""     → exactly one var named `single`
//   - vars == nil && single == ""     → empty
type fvResult struct {
	single   string
	vars     map[string]struct{}
	overflow bool
}

var fvOverflow = fvResult{overflow: true}

func (r *fvResult) delete(name string) {
	if r.vars != nil {
		delete(r.vars, name)
		return
	}
	if r.single == name {
		r.single = ""
	}
}

// traverseFVLeftSpine iteratively descends the left spine of App nodes,
// merging free variable sets from right children.
func traverseFVLeftSpine(app *App, depth int, obs fvObserver) fvResult {
	head, rights := unwindLeftSpine(app)
	result := traverseFV(head, depth+1, obs)
	for i := len(rights) - 1; i >= 0; i-- {
		result = mergeFV(result, traverseFV(rights[i], depth+1, obs))
	}
	return result
}

// traverseFV computes free variables bottom-up, notifying the observer at
// each annotation point (Lam, Thunk, Merge).
// Unlike FreeVars/freeVarsRec, this does NOT propagate Lam params into bound —
// outer Lam params are free from an inner closure's perspective (they are captured).
// Only Fix names, Case alt bindings, and Bind vars are propagated as bound,
// since they are resolved within the same scope.
//
// When the depth limit is reached, returns fvOverflow so that ancestor
// Lam/Thunk nodes detect the truncation and disable environment trimming
// rather than silently losing deep free variables.
func traverseFV(c Core, depth int, obs fvObserver) fvResult {
	if depth > maxTraversalDepth {
		return fvOverflow
	}
	switch n := c.(type) {
	case *Var:
		// Pre-compute the environment lookup key to avoid string concat at eval time.
		if n.Key == "" {
			n.Key = varKey(n)
		}
		// Inline single-var path: no map allocation until mergeFV
		// promotes the result to multi-var.
		return fvResult{single: n.Key}
	case *Lam:
		bodyFV := traverseFV(n.Body, depth+1, obs)
		// Remove the param — it comes from application, not from captured env.
		bodyFV.delete(n.Param)
		if obs != nil {
			obs.OnLam(n, bodyFV)
		}
		if bodyFV.overflow {
			return fvOverflow
		}
		return bodyFV
	case *App:
		return traverseFVLeftSpine(n, depth, obs)
	case *TyApp:
		return traverseFV(n.Expr, depth+1, obs)
	case *TyLam:
		return traverseFV(n.Body, depth+1, obs)
	case *Con:
		var result fvResult
		for _, arg := range n.Args {
			result = mergeFV(result, traverseFV(arg, depth+1, obs))
		}
		return result
	case *Case:
		result := traverseFV(n.Scrutinee, depth+1, obs)
		for _, alt := range n.Alts {
			altFV := traverseFV(alt.Body, depth+1, obs)
			// Remove pattern-bound vars — they are local to each alt.
			// Walk the pattern inline (instead of calling Bindings(),
			// which would allocate a fresh slice per PVar) and delete
			// each name from altFV in place.
			deletePatternBindings(&altFV, alt.Pattern)
			result = mergeFV(result, altFV)
		}
		return result
	case *Fix:
		// Fix name is visible in Body — remove it from the result.
		result := traverseFV(n.Body, depth+1, obs)
		result.delete(n.Name)
		return result
	case *Pure:
		return traverseFV(n.Expr, depth+1, obs)
	case *Bind:
		compFV := traverseFV(n.Comp, depth+1, obs)
		bodyFV := traverseFV(n.Body, depth+1, obs)
		// Bind var is local to the body.
		bodyFV.delete(n.Var)
		return mergeFV(compFV, bodyFV)
	case *Thunk:
		compFV := traverseFV(n.Comp, depth+1, obs)
		if obs != nil {
			obs.OnThunk(n, compFV)
		}
		if compFV.overflow {
			return fvOverflow
		}
		return compFV
	case *Force:
		return traverseFV(n.Expr, depth+1, obs)
	case *Merge:
		leftFV := traverseFV(n.Left, depth+1, obs)
		rightFV := traverseFV(n.Right, depth+1, obs)
		if obs != nil {
			obs.OnMerge(n, leftFV, rightFV)
		}
		return mergeFV(leftFV, rightFV)
	case *PrimOp:
		var result fvResult
		for _, arg := range n.Args {
			result = mergeFV(result, traverseFV(arg, depth+1, obs))
		}
		return result
	case *Lit:
		return fvResult{}
	case *Error:
		return fvResult{}
	case *RecordLit:
		var result fvResult
		for _, f := range n.Fields {
			result = mergeFV(result, traverseFV(f.Value, depth+1, obs))
		}
		return result
	case *RecordProj:
		return traverseFV(n.Record, depth+1, obs)
	case *RecordUpdate:
		result := traverseFV(n.Record, depth+1, obs)
		for _, f := range n.Updates {
			result = mergeFV(result, traverseFV(f.Value, depth+1, obs))
		}
		return result
	case *VariantLit:
		return traverseFV(n.Value, depth+1, obs)
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
	aEmpty := a.vars == nil && a.single == ""
	bEmpty := b.vars == nil && b.single == ""
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
		return fvResult{vars: map[string]struct{}{a.single: {}, b.single: {}}}
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

func setToSlice(s map[string]struct{}) []string {
	if len(s) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(s))
	for k := range s {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// varKey returns a map key for a Var node. Qualified vars use "module\x00name"
// to avoid collisions with local names.
func varKey(v *Var) string {
	if v.Module != "" {
		return v.Module + "\x00" + v.Name
	}
	return v.Name
}

// VarKey returns the map key for a Var node (exported for use in evaluator).
// Uses the pre-computed Key field when available.
func VarKey(v *Var) string {
	if v.Key != "" {
		return v.Key
	}
	return varKey(v)
}

// QualifiedKey builds a qualified environment key from module and name.
// This is the canonical constructor for the "module\x00name" key format.
func QualifiedKey(module, name string) string {
	return module + "\x00" + name
}

// SplitQualifiedKey decomposes a qualified key into (module, name).
// For unqualified keys, module is "" and name is the key itself.
func SplitQualifiedKey(key string) (module, name string) {
	if before, after, ok := strings.Cut(key, "\x00"); ok {
		return before, after
	}
	return "", key
}
