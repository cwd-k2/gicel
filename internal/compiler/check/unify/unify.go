// Unifier — core unification algorithm.
// Files in this package:
//   unify.go            core Unify dispatch, solveMeta, occurs, errors
//   unify_trail.go      snapshot/restore via undo log
//   unify_normalize.go  alias/family/CBPV normalization
//   level_unify.go      universe level unification
//   row_unify.go        row / evidence row unification
//   zonk.go             metavariable substitution

package unify

import (
	"strconv"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// UnifyErrorKind classifies unification failures for structured error reporting.
type UnifyErrorKind int

const (
	UnifyMismatch    UnifyErrorKind = iota // general type mismatch
	UnifyOccursCheck                       // infinite type (occurs check)
	UnifyDupLabel                          // duplicate label in row
	UnifyRowMismatch                       // row structure mismatch (extra labels, closed row)
	UnifySkolemRigid                       // rigid/skolem variable cannot be unified
	UnifyUntouchable                       // meta is untouchable at current solver level
)

// The UnifyError interface and its 10 variant types are defined in
// unify_error.go. Constructors for each variant are inlined at the
// generation sites below; the unifier never names UnifyError as a struct.

// AliasExpander is a callback for expanding type aliases during unification.
type AliasExpander func(types.Type) types.Type

// FamilyReducer is a callback for reducing type family applications during unification.
type FamilyReducer func(types.Type) types.Type

// TryReduceFamily attempts to reduce a single saturated type family application.
// Returns (result, true) if the family can be reduced, (nil, false) otherwise.
// Unlike FamilyReducer, this does not walk the type tree or reset the step counter.
type TryReduceFamily func(name string, args []types.Type, s span.Span) (types.Type, bool)

// Unifier manages type unification.
type Unifier struct {
	soln       map[int]types.Type
	tempSoln   map[int]types.Type // generalization overlay (see InstallTempSolution)
	labels     map[int]map[string]struct{}
	levelSoln  map[int]types.LevelExpr // level metavar solutions
	skolemSoln map[int]types.Type      // GADT given equalities: skolem → type
	freshID    *int

	// Undo trail for O(1) snapshot / O(k) restore.
	trail         []trailEntry
	snapshotDepth int // number of active Snapshot scopes (for trail-free path compression)

	// Normalization callbacks — lazy-installed by the Checker as each
	// processing phase completes. Installation order matters:
	//   1. AliasExpander  — after alias validation (decl phase 2)
	//   2. FamilyReducer  — after type family processing (decl phase 4)
	//   3. TryReduceFamily — same phase as FamilyReducer
	// Using a callback before its phase is installed is safe (nil = no-op)
	// but may produce suboptimal results (aliases/families not normalized).
	AliasExpander   AliasExpander   // optional; set by Checker after alias processing
	FamilyReducer   FamilyReducer   // optional; set by Checker after type family processing
	TryReduceFamily TryReduceFamily // optional; set by Checker — single-node reduction for zonking

	// OnSolve is called when a metavariable is solved.
	// The checker uses this to re-activate stuck type family applications.
	OnSolve func(metaID int)

	// Budget tracks structural nesting depth. If nil, nesting is unbounded.
	Budget *budget.CheckBudget

	// SolverLevel is the current implication nesting depth for touchability.
	// -1 means disabled (legacy/trial mode). >= 0 enables touchability:
	// metas with Level < SolverLevel are untouchable.
	SolverLevel int

	// FlexSkolems allows skolem variables to be solved like metas.
	// Used for GADT accessibility testing (canUnifyWith). INVARIANT:
	// must only be set on a FRESH trial unifier, never on the shared
	// checker unifier — the flex path skips the occurs check, which
	// is safe only when the types being unified are freshly instantiated.
	FlexSkolems bool

	// IsInjective reports whether a named type family is injective.
	// When non-nil and returning true, Unify decomposes stuck
	// TyFamilyApp(F, args) ~ TyFamilyApp(F, args') into pairwise
	// arg constraints. When nil or returning false, decomposition is
	// not justified and the constraint falls through to MismatchError
	// (which the solver may re-register as stuck).
	IsInjective func(name string) bool

	// zonkEntriesFn is a pre-bound method value for u.zonkInner used as
	// the callback to EvidenceEntries.ZonkEntries on TyEvidenceRow nodes.
	// Constructed once at NewUnifier time so the per-call method-value
	// closure allocation that the alloc profile showed dominating the
	// TyEvidenceRow zonk path (425K objects on cold start) is paid once
	// per Unifier instead of once per evidence row visited.
	zonkEntriesFn func(types.Type) types.Type
}

// NewUnifier creates a Unifier with its own internal fresh ID counter.
//
// zonkEntriesFn is left nil and bound lazily on the first TyEvidenceRow
// zonk; trial unifiers that never touch evidence rows therefore pay no
// closure-allocation cost at all (the Tier 4 micro benchmarks observed
// a 1-alloc/iter regression when the binding was eager in NewUnifier).
func NewUnifier() *Unifier {
	id := 0
	return &Unifier{
		soln:        make(map[int]types.Type),
		labels:      make(map[int]map[string]struct{}),
		levelSoln:   make(map[int]types.LevelExpr),
		freshID:     &id,
		SolverLevel: -1,
	}
}

// NewUnifierShared creates a Unifier that shares a fresh ID counter
// with the calling Checker, ensuring no ID collisions.
func NewUnifierShared(freshID *int) *Unifier {
	return &Unifier{
		soln:        make(map[int]types.Type),
		labels:      make(map[int]map[string]struct{}),
		levelSoln:   make(map[int]types.LevelExpr),
		freshID:     freshID,
		SolverLevel: -1,
	}
}

// FreshLevelMeta creates a fresh universe level metavariable.
func (u *Unifier) FreshLevelMeta() *types.LevelMeta {
	id := *u.freshID
	*u.freshID++
	return &types.LevelMeta{ID: id}
}

// Solve returns the current solution for a metavariable.
func (u *Unifier) Solve(id int) types.Type {
	return u.soln[id]
}

// InstallTempSolution registers a temporary solution in the generalization
// overlay. Temp solutions shadow the main solution map during Zonk so that
// generalized metavariables resolve to TyVar names (a, b, ...) in hover
// entries and the generalized type itself.
//
// Temp solutions live in a separate map (tempSoln) rather than soln to
// prevent Zonk path compression from propagating transient TyVar values
// into permanent outer-scope entries. Without separation, Zonk's path
// compression can rewrite soln[outer] = TyVar{a} when resolving a chain
// outer→inner→TyVar{a}, and RemoveTempSolution only deletes soln[inner],
// leaving the contaminated outer entry.
func (u *Unifier) InstallTempSolution(id int, ty types.Type) {
	if u.tempSoln == nil {
		u.tempSoln = make(map[int]types.Type, 4)
	}
	u.tempSoln[id] = ty
}

// RemoveTempSolution removes a temporary solution from the overlay.
func (u *Unifier) RemoveTempSolution(id int) {
	delete(u.tempSoln, id)
	if len(u.tempSoln) == 0 {
		u.tempSoln = nil
	}
}

// lookupSoln resolves a meta ID, checking the temp overlay first.
func (u *Unifier) lookupSoln(id int) (types.Type, bool) {
	if u.tempSoln != nil {
		if ty, ok := u.tempSoln[id]; ok {
			return ty, true
		}
	}
	ty, ok := u.soln[id]
	return ty, ok
}

// Solutions returns the current solution map for introspection (e.g., skolem escape check).
func (u *Unifier) Solutions() map[int]types.Type {
	return u.soln
}

// Labels returns the label context map for save/restore during trial unification.
func (u *Unifier) Labels() map[int]map[string]struct{} {
	return u.labels
}

// Unify solves the constraint a ~ b.
func (u *Unifier) Unify(a, b types.Type) error {
	if u.Budget != nil {
		if err := u.Budget.Nest(); err != nil {
			return err
		}
		defer u.Budget.Unnest()
	}
	a = u.Zonk(a)
	b = u.Zonk(b)

	// Normalize special type applications and expand aliases.
	a = u.normalize(a)
	b = u.normalize(b)

	// Error types unify with anything (poison absorption for error recovery).
	// This prevents cascading errors when one side is already an error.
	// Note: types.Equal does NOT treat TyError this way — Equal is structural
	// ("are these the same type?"), while Unify is error-aware ("can these
	// coexist without reporting a new error?"). See equal.go TyError comment.
	if _, ok := a.(*types.TyError); ok {
		return nil
	}
	if _, ok := b.(*types.TyError); ok {
		return nil
	}

	// Metavariable solving.
	if am, ok := a.(*types.TyMeta); ok {
		return u.solveMeta(am, b)
	}
	if bm, ok := b.(*types.TyMeta); ok {
		return u.solveMeta(bm, a)
	}

	// Skolem: check given equalities (GADT refinement) before rigid check.
	if as, ok := a.(*types.TySkolem); ok {
		if u.skolemSoln != nil {
			if soln, ok := u.skolemSoln[as.ID]; ok {
				return u.Unify(soln, b)
			}
		}
		if bs, ok := b.(*types.TySkolem); ok && as.ID == bs.ID {
			return nil
		}
		if u.FlexSkolems {
			u.trailSkolemWrite(as.ID)
			if u.skolemSoln == nil {
				u.skolemSoln = make(map[int]types.Type)
			}
			u.skolemSoln[as.ID] = b
			return nil
		}
		return &SkolemRigidError{SkolemName: as.Name, SkolemID: as.ID, Other: b, SkolemOnLeft: true}
	}
	if bs, ok := b.(*types.TySkolem); ok {
		if u.skolemSoln != nil {
			if soln, ok := u.skolemSoln[bs.ID]; ok {
				return u.Unify(a, soln)
			}
		}
		if u.FlexSkolems {
			u.trailSkolemWrite(bs.ID)
			if u.skolemSoln == nil {
				u.skolemSoln = make(map[int]types.Type)
			}
			u.skolemSoln[bs.ID] = a
			return nil
		}
		return &SkolemRigidError{SkolemName: bs.Name, SkolemID: bs.ID, Other: a, SkolemOnLeft: false}
	}

	switch at := a.(type) {
	case *types.TyVar:
		if bt, ok := b.(*types.TyVar); ok && at.Name == bt.Name {
			return nil
		}
	case *types.TyCon:
		if bt, ok := b.(*types.TyCon); ok && at.Name == bt.Name {
			return u.UnifyLevels(at.Level, bt.Level)
		}
		// Cumulativity: a kind at level ℓ inhabits the sort at level ℓ+1.
		// This allows ground kinds (L1) to appear where Sort₀ (L2) is expected,
		// and generalizes to arbitrary adjacent levels: ℓ ↔ ℓ+1.
		// Does NOT apply between different names at the same level
		// (e.g. Type vs Row are distinct kinds, both at L1).
		if bt, ok := b.(*types.TyCon); ok {
			if u.levelAdjacentCumulativity(at.Level, bt.Level) ||
				u.levelAdjacentCumulativity(bt.Level, at.Level) {
				return nil
			}
		}
	case *types.TyArrow:
		if bt, ok := b.(*types.TyArrow); ok {
			if err := u.Unify(at.From, bt.From); err != nil {
				return err
			}
			return u.Unify(at.To, bt.To)
		}
	case *types.TyApp:
		if bt, ok := b.(*types.TyApp); ok {
			if err := u.Unify(at.Fun, bt.Fun); err != nil {
				return err
			}
			return u.Unify(at.Arg, bt.Arg)
		}
		// Cross-case: Record(row) unifies with bare row.
		if con, ok := at.Fun.(*types.TyCon); ok && con.Name == types.TyConRecord {
			if row, ok := at.Arg.(*types.TyEvidenceRow); ok {
				if bRow, ok := b.(*types.TyEvidenceRow); ok {
					return u.unifyEvidenceRows(row, bRow)
				}
			}
		}
		// Cross-case: decompose TyApp spine directly against TyCBPV
		// to avoid the normalize cycle (normalizeCompApp ↔ compToApp).
		if cbpv, ok := b.(*types.TyCBPV); ok {
			name := types.TyConComputation
			if cbpv.Tag == types.TagThunk {
				name = types.TyConThunk
			}
			return u.unifyAppWithTriple(a, name, [3]types.Type{cbpv.Pre, cbpv.Post, cbpv.Result})
		}
	case *types.TyForall:
		if bt, ok := b.(*types.TyForall); ok {
			// Kind check: quantified variables must have compatible kinds.
			if at.Kind != nil && bt.Kind != nil {
				if err := u.Unify(at.Kind, bt.Kind); err != nil {
					return err
				}
			}
			// Unify bodies under alpha-equivalence. Use a globally fresh
			// name to avoid variable capture: if at.Var appeared free in
			// bt.Body (or vice versa), reusing it would conflate bound
			// and free occurrences. A gensym name is guaranteed disjoint
			// from any user-written or previously generated name.
			freshName := "$u" + strconv.Itoa(*u.freshID)
			*u.freshID++
			fresh := &types.TyVar{Name: freshName}
			bodyA := types.Subst(at.Body, at.Var, fresh)
			bodyB := types.Subst(bt.Body, bt.Var, fresh)
			return u.Unify(bodyA, bodyB)
		}
	case *types.TyCBPV:
		if bt, ok := b.(*types.TyCBPV); ok && at.Tag == bt.Tag {
			if err := u.Unify(at.Pre, bt.Pre); err != nil {
				return err
			}
			if err := u.Unify(at.Post, bt.Post); err != nil {
				return err
			}
			if err := u.Unify(at.Result, bt.Result); err != nil {
				return err
			}
			// CBPV grade duality (see types/doc.go): the ungraded 3-arg form
			// (Grade == nil) is treated by Unify as compatible with any
			// graded 4-arg form. This is the language-level sugar that lets
			// users write `Computation pre post a` without committing to a
			// specific grade. Equal and TypeKey remain strict for soundness
			// of inert-set caching and substitution.
			if at.IsGraded() && bt.IsGraded() {
				return u.Unify(at.Grade, bt.Grade)
			}
			return nil
		}
		if _, ok := b.(*types.TyApp); ok {
			name := types.TyConComputation
			if at.Tag == types.TagThunk {
				name = types.TyConThunk
			}
			return u.unifyAppWithTriple(b, name, [3]types.Type{at.Pre, at.Post, at.Result})
		}
	case *types.TyEvidenceRow:
		if bt, ok := b.(*types.TyEvidenceRow); ok {
			return u.unifyEvidenceRows(at, bt)
		}
		// Cross-case: bare row unifies with Record(row).
		// Type-position `{}` produces bare TyEvidenceRow; expression-position
		// `{}` produces TyApp(TyCon("Record"), TyEvidenceRow). Allow matching.
		if app, ok := b.(*types.TyApp); ok {
			if con, ok := app.Fun.(*types.TyCon); ok && con.Name == types.TyConRecord {
				if row, ok := app.Arg.(*types.TyEvidenceRow); ok {
					return u.unifyEvidenceRows(at, row)
				}
			}
		}
	case *types.TyEvidence:
		if bt, ok := b.(*types.TyEvidence); ok {
			if err := u.Unify(at.Constraints, bt.Constraints); err != nil {
				return err
			}
			return u.Unify(at.Body, bt.Body)
		}
	case *types.TyFamilyApp:
		if bt, ok := b.(*types.TyFamilyApp); ok && at.Name == bt.Name && len(at.Args) == len(bt.Args) {
			// Reflexivity: structurally identical stuck applications always
			// unify regardless of injectivity. Both sides are already zonked
			// (lines 195-196), so Equal sees resolved args.
			if types.Equal(at, bt) {
				return nil
			}
			// Decompose F(a₁..aₙ) ~ F(b₁..bₙ) only when F is injective.
			// Non-injective families must not be decomposed: G(Int) = G(String) = Bool
			// does not imply Int ~ String. The solver handles non-injective stuck
			// equalities via CtFunEq re-activation (B-2 soundness fix).
			if u.IsInjective != nil && u.IsInjective(at.Name) {
				for i := range at.Args {
					if err := u.Unify(at.Args[i], bt.Args[i]); err != nil {
						return err
					}
				}
				return nil
			}
		}
	}

	return &MismatchError{A: a, B: b}
}

// unifyAppWithTriple decomposes a TyApp chain and unifies its spine against
// a named type constructor with 3 fields (Computation or Thunk).
// This avoids the normalize cycle: normalizeCompApp converts TyApp→TyCBPV,
// while compToApp converts TyCBPV→TyApp, causing infinite recursion.
// Instead, we decompose the TyApp into (head, args) and unify each component directly.
func (u *Unifier) unifyAppWithTriple(app types.Type, conName string, fields [3]types.Type) error {
	head, args := types.UnwindApp(app)
	if len(args) < 3 {
		return &MismatchError{A: app, B: types.Con(conName)}
	}
	// Reconstruct head with excess leading args (handles len(args) > 3).
	conHead := head
	for _, arg := range args[:len(args)-3] {
		conHead = &types.TyApp{Fun: conHead, Arg: arg}
	}
	if err := u.Unify(conHead, types.Con(conName)); err != nil {
		return err
	}
	for i := range 3 {
		if err := u.Unify(args[len(args)-3+i], fields[i]); err != nil {
			return err
		}
	}
	return nil
}

func (u *Unifier) solveMeta(m *types.TyMeta, t types.Type) error {
	if tm, ok := t.(*types.TyMeta); ok && tm.ID == m.ID {
		return nil
	}
	// Touchability: a meta created at an outer level cannot be solved
	// from within an implication (inner level).
	if u.SolverLevel >= 0 && m.Level < u.SolverLevel {
		return &UntouchableMetaError{
			MetaID: m.ID,
			Level:  m.Level,
			SLevel: u.SolverLevel,
		}
	}
	// Occurs check.
	if u.occursIn(m.ID, t) {
		return &OccursError{MetaID: m.ID, Type: t}
	}
	// Label uniqueness: if this meta has a label context, verify the
	// solution doesn't introduce duplicate labels (spec §8, §6.3).
	if ctx, ok := u.labels[m.ID]; ok {
		if ev, ok := t.(*types.TyEvidenceRow); ok {
			if cap, ok := ev.Entries.(*types.CapabilityEntries); ok {
				for _, f := range cap.Fields {
					if _, dup := ctx[f.Label]; dup {
						return &DupLabelError{Label: f.Label}
					}
				}
			}
		}
	}
	u.trailSolnWrite(m.ID)
	u.soln[m.ID] = t
	// Re-activation callback: notify the checker that a meta was solved.
	if u.OnSolve != nil {
		u.OnSolve(m.ID)
	}
	return nil
}

// SolveFreshMeta directly solves a fresh (unsolved) metavariable to a value.
// Used as a trivial shortcut during constraint generation: when a meta is
// freshly created and immediately unifiable with a known type, this skips
// the overhead of emitting a CtEq and processing it through the solver.
// Precondition: m must be unsolved (Solve(m.ID) == nil).
// Returns false if the meta is untouchable at the current solver level.
func (u *Unifier) SolveFreshMeta(m *types.TyMeta, t types.Type) bool {
	// Touchability: reject if meta was created at an outer level.
	if u.SolverLevel >= 0 && m.Level < u.SolverLevel {
		return false
	}
	// Occurs check: reject infinite types (e.g., ?m = List ?m).
	if u.occursIn(m.ID, t) {
		return false
	}
	u.trailSolnWrite(m.ID)
	u.soln[m.ID] = t
	if u.OnSolve != nil {
		u.OnSolve(m.ID)
	}
	return true
}

// CollectBlockingMetas collects all unsolved meta IDs in the given types,
// using the current solution map to resolve already-solved metas.
func (u *Unifier) CollectBlockingMetas(tys []types.Type) []int {
	var ids []int
	seen := make(map[int]bool)
	for _, t := range tys {
		u.collectMetaIDsRec(u.Zonk(t), seen, &ids)
	}
	return ids
}

func (u *Unifier) collectMetaIDsRec(t types.Type, seen map[int]bool, ids *[]int) {
	switch ty := t.(type) {
	case *types.TyMeta:
		if !seen[ty.ID] {
			seen[ty.ID] = true
			*ids = append(*ids, ty.ID)
		}
	default:
		types.ForEachChild(t, func(ch types.Type) bool {
			u.collectMetaIDsRec(u.Zonk(ch), seen, ids)
			return true
		})
	}
}

// occursIn checks whether a meta variable with the given ID appears in type t.
// Zonks the entire type once at entry, then traverses the zonked tree without
// further Zonk calls. This avoids O(N²) repeated traversals on deep types.
func (u *Unifier) occursIn(id int, t types.Type) bool {
	return u.occursInZonked(id, u.Zonk(t))
}

// occursInZonked walks a pre-zonked type tree checking for meta variable id.
// No budget check: recursion depth is bounded by the structural size of the
// type, which is finite and bounded by the outer Unify/Zonk budget.
func (u *Unifier) occursInZonked(id int, t types.Type) bool {
	// Resolve one level of meta indirection (the child may itself be solved).
	if m, ok := t.(*types.TyMeta); ok {
		if s, exists := u.soln[m.ID]; exists {
			t = s
		}
	}
	switch ty := t.(type) {
	case *types.TyMeta:
		return ty.ID == id
	case *types.TySkolem:
		return false
	default:
		found := false
		types.ForEachChild(t, func(ch types.Type) bool {
			if u.occursInZonked(id, ch) {
				found = true
				return false
			}
			return true
		})
		return found
	}
}
