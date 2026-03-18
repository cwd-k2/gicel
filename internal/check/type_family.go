package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// TypeFamilyInfo holds the elaborated information for a type family.
type TypeFamilyInfo struct {
	Name       string
	Params     []TFParam
	ResultKind types.Kind
	ResultName string  // non-empty if injective
	Deps       []tfDep // injectivity deps (elaborated)
	Equations  []tfEquation
	IsAssoc    bool   // true if declared as associated type in a class
	ClassName  string // non-empty if IsAssoc
}

// TFParam is a type family parameter with its name and kind.
type TFParam struct {
	Name string
	Kind types.Kind
}

// tfDep is an elaborated functional dependency.
type tfDep struct {
	From string   // result variable name
	To   []string // determined parameter names
}

// tfEquation is an elaborated type family equation with resolved types.
type tfEquation struct {
	Patterns []types.Type // LHS patterns (resolved)
	RHS      types.Type   // RHS body (resolved)
	S        span.Span
}

// Clone returns a deep copy of the TypeFamilyInfo, isolating the Equations
// slice so that subsequent compilations cannot mutate shared module metadata.
func (f *TypeFamilyInfo) Clone() *TypeFamilyInfo {
	cp := *f
	cp.Params = append([]TFParam(nil), f.Params...)
	cp.Deps = append([]tfDep(nil), f.Deps...)
	cp.Equations = append([]tfEquation(nil), f.Equations...)
	return &cp
}

// cloneFamilies returns a new map with cloned TypeFamilyInfo values.
func cloneFamilies(m map[string]*TypeFamilyInfo) map[string]*TypeFamilyInfo {
	out := make(map[string]*TypeFamilyInfo, len(m))
	for k, v := range m {
		out[k] = v.Clone()
	}
	return out
}

// matchResult classifies the outcome of type-level pattern matching.
type matchResult int

const (
	matchSuccess       matchResult = iota // patterns matched, substitution available
	matchFail                             // patterns definitely do not match
	matchIndeterminate                    // cannot decide (unsolved metavariable vs concrete pattern)
)

// processTypeFamily kind-checks and registers a type family declaration.
func (ch *Checker) processTypeFamily(d *syntax.DeclTypeFamily) {
	// Check for duplicate.
	if _, dup := ch.families[d.Name]; dup {
		ch.addCodedError(errs.ErrDuplicateDecl, d.S,
			fmt.Sprintf("duplicate type family: %s", d.Name))
		return
	}
	if _, dup := ch.aliases[d.Name]; dup {
		ch.addCodedError(errs.ErrDuplicateDecl, d.S,
			fmt.Sprintf("type family %s conflicts with type alias of the same name", d.Name))
		return
	}

	// Resolve parameter kinds.
	var params []TFParam
	for _, p := range d.Params {
		params = append(params, TFParam{Name: p.Name, Kind: ch.resolveKindExpr(p.Kind)})
	}

	// Resolve result kind.
	resultKind := ch.resolveKindExpr(d.ResultKind)

	// Elaborate dependencies.
	var deps []tfDep
	for _, fd := range d.Deps {
		deps = append(deps, tfDep{From: fd.From, To: fd.To})
	}

	// Resolve equations.
	var equations []tfEquation
	for _, eq := range d.Equations {
		// Validate equation name matches family name.
		if eq.Name != d.Name {
			ch.addCodedError(errs.ErrTypeFamilyEquation, eq.S,
				fmt.Sprintf("equation name %s does not match type family %s", eq.Name, d.Name))
			continue
		}
		// Validate arity.
		if len(eq.Patterns) != len(params) {
			ch.addCodedError(errs.ErrTypeFamilyEquation, eq.S,
				fmt.Sprintf("type family %s expects %d arguments, equation has %d",
					d.Name, len(params), len(eq.Patterns)))
			continue
		}
		// Resolve patterns and RHS.
		resolvedPats := make([]types.Type, len(eq.Patterns))
		for i, pat := range eq.Patterns {
			resolvedPats[i] = ch.resolveTypeExpr(pat)
		}
		resolvedRHS := ch.resolveTypeExpr(eq.RHS)
		equations = append(equations, tfEquation{
			Patterns: resolvedPats,
			RHS:      resolvedRHS,
			S:        eq.S,
		})
	}

	info := &TypeFamilyInfo{
		Name:       d.Name,
		Params:     params,
		ResultKind: resultKind,
		ResultName: d.ResultName,
		Deps:       deps,
		Equations:  equations,
	}

	// Verify injectivity if declared.
	if d.ResultName != "" {
		ch.verifyInjectivity(info)
	}

	ch.families[d.Name] = info
}

// maxReductionDepth is the fuel limit for type family reduction.
// Prevents non-termination in recursive type families.
const maxReductionDepth = 100

// maxReductionTypeSize is the maximum allowed size (node count) of a type
// produced by type family reduction. Without this bound, a family like
// `type Grow a := Grow (Pair a a)` would produce a type with 2^k nodes after
// k reductions, causing exponential memory and time consumption even though
// the fuel limit eventually fires. This bound ensures that reduction halts
// as soon as the intermediate type becomes unreasonably large.
const maxReductionTypeSize = 10000

// reduceTyFamily attempts to reduce a saturated type family application.
// Returns (result, true) on success, or (nil, false) if stuck/no match.
// Uses a checker-level depth counter to bound recursive reductions.
// The counter is incremented on each successful reduction and never decremented
// within a single normalize() call — it resets at the normalize entry point.
func (ch *Checker) reduceTyFamily(name string, args []types.Type, s span.Span) (types.Type, bool) {
	ch.reductionDepth++
	if ch.reductionDepth > maxReductionDepth {
		ch.addCodedError(errs.ErrTypeFamilyReduction, s,
			fmt.Sprintf("type family %s: reduction depth limit exceeded (possible infinite recursion)", name))
		return nil, false
	}
	fam, ok := ch.families[name]
	if !ok {
		return nil, false
	}
	for _, eq := range fam.Equations {
		subst, result := ch.matchTyPatterns(eq.Patterns, args)
		switch result {
		case matchSuccess:
			rhs := types.SubstMany(eq.RHS, subst)
			// Guard against exponential type growth: e.g., `Grow a =: Grow (Pair a a)`
			// doubles the type on each step. Without this check, 100 steps would
			// produce a type with ~2^100 nodes, causing OOM/hang during Zonk.
			if types.TypeSize(rhs, maxReductionTypeSize) > maxReductionTypeSize {
				ch.addCodedError(errs.ErrTypeFamilyReduction, s,
					fmt.Sprintf("type family %s: result type too large (possible exponential growth)", name))
				return nil, false
			}
			return rhs, true
		case matchFail:
			continue
		case matchIndeterminate:
			return nil, false // stuck — do not try further equations
		}
	}
	return nil, false // no equation matched — stuck
}

// matchTyPatterns attempts to match type patterns against arguments.
// Returns a substitution and match result.
func (ch *Checker) matchTyPatterns(patterns, args []types.Type) (map[string]types.Type, matchResult) {
	if len(patterns) != len(args) {
		return nil, matchFail
	}
	subst := make(map[string]types.Type)
	for i, pat := range patterns {
		result := ch.matchTyPattern(pat, args[i], subst)
		if result != matchSuccess {
			return nil, result
		}
	}
	return subst, matchSuccess
}

// matchTyPattern matches a single type pattern against an argument.
func (ch *Checker) matchTyPattern(pat, arg types.Type, subst map[string]types.Type) matchResult {
	// Zonk the argument to resolve solved metavariables.
	arg = ch.unifier.Zonk(arg)

	switch p := pat.(type) {
	case *types.TyVar:
		// Wildcard-like: "_" is conventionally a TyVar named "_".
		if p.Name == "_" {
			return matchSuccess
		}
		// Pattern variable: bind or check consistency.
		if existing, ok := subst[p.Name]; ok {
			// Already bound — must be equal.
			if types.Equal(existing, arg) {
				return matchSuccess
			}
			return matchFail
		}
		subst[p.Name] = arg
		return matchSuccess

	case *types.TyCon:
		// Exact constructor match.
		switch a := arg.(type) {
		case *types.TyCon:
			if p.Name == a.Name {
				return matchSuccess
			}
			return matchFail
		case *types.TyMeta:
			return matchIndeterminate
		default:
			return matchFail
		}

	case *types.TyApp:
		// Decompose application: match head and argument recursively.
		switch a := arg.(type) {
		case *types.TyApp:
			r := ch.matchTyPattern(p.Fun, a.Fun, subst)
			if r != matchSuccess {
				return r
			}
			return ch.matchTyPattern(p.Arg, a.Arg, subst)
		case *types.TyMeta:
			return matchIndeterminate
		default:
			return matchFail
		}

	default:
		// Unsupported pattern form.
		return matchFail
	}
}

// verifyInjectivity checks that a type family satisfies its declared
// injectivity annotation by pairwise equation comparison.
// For every pair of equations, if the RHSes can unify,
// the corresponding LHS patterns must also unify.
//
// Pattern variables are instantiated as fresh metavariables before comparison,
// since TyVar (pattern variable) ≠ TyMeta (solvable) in unification.
func (ch *Checker) verifyInjectivity(info *TypeFamilyInfo) {
	for i := 0; i < len(info.Equations); i++ {
		for j := i + 1; j < len(info.Equations); j++ {
			eqI := info.Equations[i]
			eqJ := info.Equations[j]

			// Instantiate pattern variables as fresh metas for both equations.
			rhsI := ch.instantiatePatVars(eqI.RHS, eqI.Patterns, info.Params)
			rhsJ := ch.instantiatePatVars(eqJ.RHS, eqJ.Patterns, info.Params)
			patsI := ch.instantiatePatVarsList(eqI.Patterns, info.Params)
			patsJ := ch.instantiatePatVarsList(eqJ.Patterns, info.Params)

			// Trial unification on RHSes.
			if ch.tryUnify(rhsI, rhsJ) {
				// RHSes can unify — check that LHSes also unify.
				allMatch := true
				for k := range patsI {
					if !ch.tryUnify(patsI[k], patsJ[k]) {
						allMatch = false
						break
					}
				}
				if !allMatch {
					ch.addCodedError(errs.ErrInjectivity, eqJ.S,
						fmt.Sprintf("type family %s: injectivity violation between equations %d and %d",
							info.Name, i+1, j+1))
				}
			}
		}
	}
}

// instantiatePatVars replaces free TyVars (pattern variables) with fresh metas.
// Uses parameter kinds from the family info to assign correct kinds to metas.
func (ch *Checker) instantiatePatVars(ty types.Type, patterns []types.Type, params []TFParam) types.Type {
	varKinds := collectPatternVarKinds(patterns, params)
	for _, v := range collectPatternVars(patterns) {
		kind := varKinds[v]
		ty = types.Subst(ty, v, ch.freshMeta(kind))
	}
	return ty
}

// instantiatePatVarsList replaces free TyVars in each pattern with fresh metas.
// Uses parameter kinds from the family info to assign correct kinds to metas.
func (ch *Checker) instantiatePatVarsList(patterns []types.Type, params []TFParam) []types.Type {
	vars := collectPatternVars(patterns)
	varKinds := collectPatternVarKinds(patterns, params)
	result := make([]types.Type, len(patterns))
	subs := make(map[string]types.Type, len(vars))
	for _, v := range vars {
		kind := varKinds[v]
		subs[v] = ch.freshMeta(kind)
	}
	for i, p := range patterns {
		result[i] = types.SubstMany(p, subs)
	}
	return result
}

// collectPatternVars collects all TyVar names from type patterns.
func collectPatternVars(patterns []types.Type) []string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range patterns {
		collectPatternVarsRec(p, seen, &result)
	}
	return result
}

func collectPatternVarsRec(t types.Type, seen map[string]bool, result *[]string) {
	switch ty := t.(type) {
	case *types.TyVar:
		if ty.Name != "_" && !seen[ty.Name] {
			seen[ty.Name] = true
			*result = append(*result, ty.Name)
		}
	case *types.TyApp:
		collectPatternVarsRec(ty.Fun, seen, result)
		collectPatternVarsRec(ty.Arg, seen, result)
	}
}

// collectPatternVarKinds maps each pattern variable to its kind based on the
// parameter position where it first appears. A bare TyVar at position i gets
// params[i].Kind; a TyVar nested inside TyApp (e.g., List a) defaults to KType.
func collectPatternVarKinds(patterns []types.Type, params []TFParam) map[string]types.Kind {
	result := make(map[string]types.Kind)
	for i, p := range patterns {
		var paramKind types.Kind
		if i < len(params) {
			paramKind = params[i].Kind
		} else {
			paramKind = types.KType{}
		}
		collectVarKindsRec(p, paramKind, result)
	}
	return result
}

// collectVarKindsRec assigns kinds to pattern variables. A bare TyVar gets
// the contextual kind; variables inside TyApp positions get their kind
// inferred from the application structure: the head has kind (argKind -> contextKind)
// and the argument has kind argKind (defaulting to KType for the argument).
func collectVarKindsRec(t types.Type, contextKind types.Kind, result map[string]types.Kind) {
	switch ty := t.(type) {
	case *types.TyVar:
		if ty.Name != "_" {
			if _, exists := result[ty.Name]; !exists {
				result[ty.Name] = contextKind
			}
		}
	case *types.TyApp:
		// In `F a` where the whole application has contextKind,
		// F has kind (KType -> contextKind) and a has kind KType.
		funKind := &types.KArrow{From: types.KType{}, To: contextKind}
		collectVarKindsRec(ty.Fun, funKind, result)
		collectVarKindsRec(ty.Arg, types.KType{}, result)
	}
}

// reduceFamilyInType reduces type family applications within a type.
// Used by exhaustiveness checking to resolve data family instances.
func (ch *Checker) reduceFamilyInType(t types.Type) types.Type {
	if ch.unifier.familyReducer != nil {
		return ch.unifier.familyReducer(t)
	}
	return t
}

// mangledDataFamilyName produces a mangled name for a data family instance.
// E.g., Elem applied to (List a) → "Elem$$1$List".
// The format is: familyName "$$" arity "$" pat1 "$" pat2 ...
// The "$$" arity prefix prevents collisions between:
//   - Family "F" with patterns [A, B]  → "F$$2$A$B"
//   - Family "F" with pattern  [A$B]   → "F$$1$A$B"   (different)
//   - Family "F$A" with pattern [B]    → "F$A$$1$B"   (different)
func (ch *Checker) mangledDataFamilyName(familyName string, patterns []types.Type) string {
	name := fmt.Sprintf("%s$$%d", familyName, len(patterns))
	for _, p := range patterns {
		name += "$" + typeNameForMangling(p)
	}
	return name
}

// typeNameForMangling extracts a short name from a type for mangling purposes.
func typeNameForMangling(t types.Type) string {
	switch ty := t.(type) {
	case *types.TyCon:
		return ty.Name
	case *types.TyApp:
		head, _ := types.UnwindApp(t)
		return typeNameForMangling(head)
	case *types.TyVar:
		return ty.Name
	default:
		return "X"
	}
}

// installFamilyReducer sets the family reducer callback in the unifier.
func (ch *Checker) installFamilyReducer() {
	if len(ch.families) == 0 {
		return
	}
	ch.unifier.familyReducer = func(t types.Type) types.Type {
		ch.reductionDepth = 0 // reset per normalize() call
		return ch.reduceFamilyApps(t)
	}
}

// reduceFamilyApps walks a type and reduces any TyFamilyApp nodes
// or TyApp chains that form a saturated type family application.
// It also recurses into type structure (TyApp, TyArrow, TyCBPV, etc.)
// so that nested family applications are reduced — this is important
// for exhaustiveness checking which calls reduceFamilyInType on the
// full scrutinee type.
//
// A per-call memoization cache converts exponential re-computation
// (e.g., Grow a =: Pair (Grow a) (Grow a)) into DAG-shared linear work.
// Stuck applications are not cached because they produce unique placeholder metas.
func (ch *Checker) reduceFamilyApps(t types.Type) types.Type {
	cache := make(map[string]types.Type)
	return ch.reduceFamilyAppsN(t, cache)
}

func (ch *Checker) reduceFamilyAppsN(t types.Type, cache map[string]types.Type) types.Type {
	// Bail out if we've exceeded the reduction depth (set by reduceTyFamily calls).
	if ch.reductionDepth > maxReductionDepth {
		return t
	}
	// Case 1: explicit TyFamilyApp.
	if tf, ok := t.(*types.TyFamilyApp); ok {
		// Recurse into arguments first.
		args := make([]types.Type, len(tf.Args))
		for i, a := range tf.Args {
			args[i] = ch.reduceFamilyAppsN(a, cache)
		}
		key := familyAppKey(tf.Name, args)
		if cached, ok := cache[key]; ok {
			return cached
		}
		result, reduced := ch.reduceTyFamily(tf.Name, args, tf.S)
		if reduced {
			r := ch.reduceFamilyAppsN(result, cache)
			cache[key] = r
			return r
		}
		// Stuck — don't cache (placeholder meta is unique per registration).
		if placeholder := ch.registerStuckFamily(tf.Name, args, tf.Kind, tf.S); placeholder != nil {
			return placeholder
		}
		return &types.TyFamilyApp{Name: tf.Name, Args: args, Kind: tf.Kind, S: tf.S}
	}
	// Case 2: TyApp chain with TyCon head that is a known type family.
	// This arises from substitution (e.g., Elem c [c := List a]).
	if app, ok := t.(*types.TyApp); ok {
		head, args := types.UnwindApp(t)
		if con, ok := head.(*types.TyCon); ok {
			if fam, ok := ch.families[con.Name]; ok && len(fam.Params) == len(args) {
				// Recurse into arguments first.
				for i, a := range args {
					args[i] = ch.reduceFamilyAppsN(a, cache)
				}
				key := familyAppKey(con.Name, args)
				if cached, ok := cache[key]; ok {
					return cached
				}
				result, reduced := ch.reduceTyFamily(con.Name, args, t.Span())
				if reduced {
					r := ch.reduceFamilyAppsN(result, cache)
					cache[key] = r
					return r
				}
				// Stuck — don't cache.
				if placeholder := ch.registerStuckFamily(con.Name, args, fam.ResultKind, t.Span()); placeholder != nil {
					return placeholder
				}
				// Not reduced — rewrite to TyFamilyApp for cleaner types.
				return &types.TyFamilyApp{Name: con.Name, Args: args, Kind: fam.ResultKind, S: t.Span()}
			}
		}
		// Not a type family application — recurse into Fun and Arg.
		rFun := ch.reduceFamilyAppsN(app.Fun, cache)
		rArg := ch.reduceFamilyAppsN(app.Arg, cache)
		if rFun == app.Fun && rArg == app.Arg {
			return t
		}
		return &types.TyApp{Fun: rFun, Arg: rArg, S: app.S}
	}
	// Case 3: structural recursion into other type formers (no caching needed).
	return types.MapType(t, func(child types.Type) types.Type {
		return ch.reduceFamilyAppsN(child, cache)
	})
}

// familyAppKey produces a structural cache key for a type family application.
// Unlike types.Pretty (which is display-oriented), this generates a canonical
// representation that uniquely identifies the type structure. Within a single
// reduceFamilyApps call, meta variables are stable (not solved mid-traversal),
// so ?ID references remain consistent.
func familyAppKey(name string, args []types.Type) string {
	var b strings.Builder
	b.WriteString(name)
	for _, a := range args {
		b.WriteByte(' ')
		writeTypeKey(&b, a)
	}
	return b.String()
}

func writeTypeKey(b *strings.Builder, t types.Type) {
	switch ty := t.(type) {
	case *types.TyCon:
		b.WriteString(ty.Name)
	case *types.TyVar:
		b.WriteByte('\'')
		b.WriteString(ty.Name)
	case *types.TyMeta:
		fmt.Fprintf(b, "?%d", ty.ID)
	case *types.TyApp:
		b.WriteByte('(')
		writeTypeKey(b, ty.Fun)
		b.WriteByte(' ')
		writeTypeKey(b, ty.Arg)
		b.WriteByte(')')
	case *types.TyArrow:
		b.WriteByte('(')
		writeTypeKey(b, ty.From)
		b.WriteString("->")
		writeTypeKey(b, ty.To)
		b.WriteByte(')')
	case *types.TyCBPV:
		if ty.Tag == types.TagComp {
			b.WriteString("{C ")
		} else {
			b.WriteString("{T ")
		}
		writeTypeKey(b, ty.Pre)
		b.WriteByte(' ')
		writeTypeKey(b, ty.Post)
		b.WriteByte(' ')
		writeTypeKey(b, ty.Result)
		b.WriteByte('}')
	case *types.TyForall:
		b.WriteString("{V ")
		b.WriteString(ty.Var)
		b.WriteByte(' ')
		writeTypeKey(b, ty.Body)
		b.WriteByte('}')
	case *types.TyFamilyApp:
		b.WriteByte('[')
		b.WriteString(ty.Name)
		for _, a := range ty.Args {
			b.WriteByte(' ')
			writeTypeKey(b, a)
		}
		b.WriteByte(']')
	case *types.TySkolem:
		fmt.Fprintf(b, "#%d", ty.ID)
	default:
		fmt.Fprintf(b, "<%s>", types.Pretty(t))
	}
}

// registerStuckFamily checks whether a stuck family application has unsolved
// meta arguments and, if so, registers it in the unifier's re-activation index.
// Returns a fresh result meta that stands in for the family application,
// or nil if the family is not blocked on metas (e.g., genuinely unmatched).
func (ch *Checker) registerStuckFamily(name string, args []types.Type, resultKind types.Kind, s span.Span) *types.TyMeta {
	blocking := ch.unifier.collectBlockingMetas(args)
	if len(blocking) == 0 {
		// Not blocked on metas — the family is genuinely stuck (no matching equation).
		return nil
	}
	resultMeta := ch.freshMeta(resultKind)
	entry := &stuckFamilyEntry{
		familyName: name,
		args:       args,
		span:       s,
		resultMeta: resultMeta,
		blockingOn: blocking,
	}
	ch.unifier.RegisterStuckFamily(entry)
	return resultMeta
}

// maxReworkIterations bounds the number of rework processing iterations
// to prevent runaway loops from cascading re-activations.
const maxReworkIterations = 200

// ProcessRework attempts to reduce stuck type family applications that were
// unblocked by recent meta solutions. On success, the result meta is unified
// with the reduced type. Entries that remain stuck are re-registered.
// The reduction depth counter scopes globally across one rework session.
func (ch *Checker) ProcessRework() {
	ch.reductionDepth = 0
	for range maxReworkIterations {
		entries := ch.unifier.DrainRework()
		if len(entries) == 0 {
			return
		}
		// Deduplicate: an entry may appear in multiple meta buckets.
		seen := make(map[*stuckFamilyEntry]bool, len(entries))
		for _, entry := range entries {
			if seen[entry] {
				continue
			}
			seen[entry] = true
			// Zonk arguments to incorporate newly solved metas.
			zonked := make([]types.Type, len(entry.args))
			for i, a := range entry.args {
				zonked[i] = ch.unifier.Zonk(a)
			}
			result, reduced := ch.reduceTyFamily(entry.familyName, zonked, entry.span)
			if reduced {
				// Unify the result meta with the reduced type.
				// This may solve further metas, triggering more rework.
				_ = ch.unifier.Unify(entry.resultMeta, result)
				continue
			}
			// Still stuck — re-register with updated blocking metas.
			blocking := ch.unifier.collectBlockingMetas(zonked)
			if len(blocking) == 0 {
				// No longer blocked on metas but still didn't reduce —
				// genuinely unmatched. Leave the result meta unsolved.
				continue
			}
			entry.args = zonked
			entry.blockingOn = blocking
			ch.unifier.RegisterStuckFamily(entry)
		}
	}
}
