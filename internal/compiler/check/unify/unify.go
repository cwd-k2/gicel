package unify

import (
	"strconv"
	"strings"

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

// UnifyError is a structured error returned by the unifier.
// Fields carry structured data; Error() formats lazily so that trial
// unification paths that discard the error pay no formatting cost.
type UnifyError struct {
	Kind    UnifyErrorKind
	TypeA   types.Type // left-hand type (nil when not applicable)
	TypeB   types.Type // right-hand type (nil when not applicable)
	MetaID  int        // metavariable ID (occurs check, untouchable)
	Level   int        // meta level (untouchable)
	SLevel  int        // solver level (untouchable)
	Name    string     // structural identifier: skolem name or class name
	Message string     // human-readable description (row mismatch detail, etc.)
	Label   string     // row label (dup label, grade mismatch)
	Labels  []string   // field/entry labels (row mismatch detail)
	CountA  int        // left count (grade count, arg count, entry count)
	CountB  int        // right count
}

func (e *UnifyError) Error() string {
	switch e.Kind {
	case UnifyMismatch:
		if e.TypeA != nil && e.TypeB != nil {
			return "type mismatch: " + types.Pretty(e.TypeA) + " vs " + types.Pretty(e.TypeB)
		}
		if e.Label != "" && e.CountA >= 0 && e.CountB >= 0 {
			return "grade count mismatch for label " + strconv.Quote(e.Label) + ": " + strconv.Itoa(e.CountA) + " vs " + strconv.Itoa(e.CountB)
		}
		if e.Message != "" {
			return e.Message
		}
		return "type mismatch"
	case UnifyOccursCheck:
		if e.TypeA != nil {
			return "infinite type: ?" + strconv.Itoa(e.MetaID) + " occurs in " + types.Pretty(e.TypeA)
		}
		return "infinite level: ?l" + strconv.Itoa(e.MetaID)
	case UnifyDupLabel:
		return "duplicate label " + strconv.Quote(e.Label) + " in row"
	case UnifyRowMismatch:
		if e.Name != "" {
			return "constraint arg count mismatch: " + e.Name + " has " + strconv.Itoa(e.CountA) + " args vs " + strconv.Itoa(e.CountB)
		}
		if e.CountA > 0 && e.CountB > 0 {
			return "row mismatch: extra entries (left=" + strconv.Itoa(e.CountA) + ", right=" + strconv.Itoa(e.CountB) + ")"
		}
		detail := strconv.Itoa(e.CountA + e.CountB)
		if len(e.Labels) > 0 {
			detail = strings.Join(e.Labels, ", ")
		}
		return "record has unmatched field(s): " + detail
	case UnifySkolemRigid:
		if e.TypeA != nil {
			return "cannot unify " + types.Pretty(e.TypeA) + " with rigid type variable #" + e.Name
		}
		return "cannot unify rigid type variable #" + e.Name + " with " + types.Pretty(e.TypeB)
	case UnifyUntouchable:
		return "untouchable meta ?" + strconv.Itoa(e.MetaID) + " (level " + strconv.Itoa(e.Level) + ") at solver level " + strconv.Itoa(e.SLevel)
	default:
		return "unification error"
	}
}

// IsMismatch reports whether this error is a simple type mismatch
// (as opposed to occurs check, skolem rigidity, etc.).
func (e *UnifyError) IsMismatch() bool {
	return e.Kind == UnifyMismatch && e.TypeA != nil && e.TypeB != nil
}

// AliasExpander is a callback for expanding type aliases during unification.
type AliasExpander func(types.Type) types.Type

// FamilyReducer is a callback for reducing type family applications during unification.
type FamilyReducer func(types.Type) types.Type

// TryReduceFamily attempts to reduce a single saturated type family application.
// Returns (result, true) if the family can be reduced, (nil, false) otherwise.
// Unlike FamilyReducer, this does not walk the type tree or reset the step counter.
type TryReduceFamily func(name string, args []types.Type, s span.Span) (types.Type, bool)

// trailTag discriminates the three maps that a trail entry can target.
type trailTag byte

const (
	trailSoln       trailTag = iota // soln map
	trailLabel                      // labels map
	trailSkolemSoln                 // skolemSoln map
	trailLevelSoln                  // levelSoln map
)

// trailEntry records a single map mutation for undo-log rollback.
// On Restore, entries are replayed in reverse order, restoring the
// pre-mutation value (or deleting the key if it did not exist).
type trailEntry struct {
	tag      trailTag
	id       int
	existed  bool
	oldType  types.Type          // valid when tag == trailSoln or trailSkolemSoln
	oldLbl   map[string]struct{} // valid when tag == trailLabel
	oldLevel types.LevelExpr     // valid when tag == trailLevelSoln
}

// Unifier manages type unification.
type Unifier struct {
	soln       map[int]types.Type
	labels     map[int]map[string]struct{}
	levelSoln  map[int]types.LevelExpr // level metavar solutions
	skolemSoln map[int]types.Type      // GADT given equalities: skolem → type
	freshID    *int

	// Undo trail for O(1) snapshot / O(k) restore.
	trail         []trailEntry
	snapshotDepth int // number of active Snapshot scopes (for trail-free path compression)

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
}

// NewUnifier creates a Unifier with its own internal fresh ID counter.
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

// InstallTempSolution registers a temporary solution for a metavariable.
// The caller must call RemoveTempSolution when done. Used by let-generalization
// to substitute metas with type variables for Zonk, then clean up.
// NOT trailed: callers manage the lifecycle manually outside trial scopes.
func (u *Unifier) InstallTempSolution(id int, ty types.Type) {
	u.soln[id] = ty
}

// RemoveTempSolution removes a previously installed temporary solution.
func (u *Unifier) RemoveTempSolution(id int) {
	delete(u.soln, id)
}

// Solutions returns the current solution map for introspection (e.g., skolem escape check).
func (u *Unifier) Solutions() map[int]types.Type {
	return u.soln
}

// Labels returns the label context map for save/restore during trial unification.
func (u *Unifier) Labels() map[int]map[string]struct{} {
	return u.labels
}

// ---------------------------------------------------------------------------
// Trail-based snapshot / restore
// ---------------------------------------------------------------------------

// Snapshot records the current trail position for later rollback.
// O(1) — no map copying.
type Snapshot struct {
	pos int
}

// Snapshot captures the current unifier state for later rollback.
func (u *Unifier) Snapshot() Snapshot {
	u.snapshotDepth++
	return Snapshot{pos: len(u.trail)}
}

// Restore rolls back the unifier to a previously saved snapshot by replaying
// the trail in reverse. O(k) where k = number of mutations since snapshot.
func (u *Unifier) Restore(snap Snapshot) {
	for i := len(u.trail) - 1; i >= snap.pos; i-- {
		e := &u.trail[i]
		switch e.tag {
		case trailSoln:
			if e.existed {
				u.soln[e.id] = e.oldType
			} else {
				delete(u.soln, e.id)
			}
		case trailLabel:
			if e.existed {
				u.labels[e.id] = e.oldLbl
			} else {
				delete(u.labels, e.id)
			}
		case trailSkolemSoln:
			if e.existed {
				u.skolemSoln[e.id] = e.oldType
			} else {
				delete(u.skolemSoln, e.id)
			}
		case trailLevelSoln:
			if e.existed {
				u.levelSoln[e.id] = e.oldLevel
			} else {
				delete(u.levelSoln, e.id)
			}
		}
	}
	u.trail = u.trail[:snap.pos]
	u.snapshotDepth--
}

// trailSolnWrite records the current soln[id] value before mutation.
func (u *Unifier) trailSolnWrite(id int) {
	old, existed := u.soln[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailSoln, id: id, existed: existed, oldType: old,
	})
}

// trailLabelWrite records the current labels[id] value before mutation.
func (u *Unifier) trailLabelWrite(id int) {
	old, existed := u.labels[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailLabel, id: id, existed: existed, oldLbl: old,
	})
}

// trailSkolemWrite records the current skolemSoln[id] value before mutation.
func (u *Unifier) trailSkolemWrite(id int) {
	if u.skolemSoln == nil {
		u.skolemSoln = make(map[int]types.Type)
	}
	old, existed := u.skolemSoln[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailSkolemSoln, id: id, existed: existed, oldType: old,
	})
}

// trailLevelWrite records the current levelSoln[id] value before mutation.
func (u *Unifier) trailLevelWrite(id int) {
	old, existed := u.levelSoln[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailLevelSoln, id: id, existed: existed, oldLevel: old,
	})
}

// InstallGivenEq records a GADT given equality: the skolem with the given ID
// is locally equal to ty within the current scope. Use Snapshot/Restore to
// limit the lifetime of given equalities to a single case branch.
func (u *Unifier) InstallGivenEq(skolemID int, ty types.Type) {
	u.trailSkolemWrite(skolemID)
	if u.skolemSoln == nil {
		u.skolemSoln = make(map[int]types.Type)
	}
	u.skolemSoln[skolemID] = ty
}

// RemoveGivenEq removes a given equality for the specified skolem.
// Used to scope given equalities to individual GADT case branches.
func (u *Unifier) RemoveGivenEq(skolemID int) {
	if u.skolemSoln != nil {
		delete(u.skolemSoln, skolemID)
	}
}

// RegisterLabelContext records the surrounding labels for a row metavariable.
func (u *Unifier) RegisterLabelContext(id int, labels map[string]struct{}) {
	u.trailLabelWrite(id)
	u.labels[id] = labels
}

// normalize applies alias expansion, type family reduction, and special
// type normalization. Type family reduction is eager here for compatibility:
// many inference paths depend on TyFamilyApp being reduced before unification.
// The solver's CtFunEq path (L2-b) handles deferred reduction for stuck
// applications whose args contain unsolved metas.
func (u *Unifier) normalize(t types.Type) types.Type {
	if u.AliasExpander != nil {
		t = u.AliasExpander(t)
	}
	if u.FamilyReducer != nil {
		t = u.FamilyReducer(t)
	}
	return normalizeCompApp(t)
}

// normalizeCompApp converts fully-applied TyApp chains to their special type
// representations. Handles both 4-arg (graded) and 3-arg (legacy) forms:
//
//	4-arg: TyApp(TyApp(TyApp(TyApp(TyCon("Computation"), grade), pre), post), result)
//	3-arg: TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
//
// This arises when a class type parameter is substituted with Computation.
func normalizeCompApp(t types.Type) types.Type {
	app1, ok := t.(*types.TyApp)
	if !ok {
		return t
	}
	app2, ok := app1.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	app3, ok := app2.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	// Try 4-arg form: Computation grade pre post result
	if app4, ok := app3.Fun.(*types.TyApp); ok {
		if con, ok := app4.Fun.(*types.TyCon); ok {
			switch con.Name {
			case types.TyConComputation:
				return &types.TyCBPV{Tag: types.TagComp, Grade: app4.Arg, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, Flags: types.MetaFreeFlags(app4.Arg, app3.Arg, app2.Arg, app1.Arg), S: t.Span()}
			case types.TyConThunk:
				return &types.TyCBPV{Tag: types.TagThunk, Grade: app4.Arg, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, Flags: types.MetaFreeFlags(app4.Arg, app3.Arg, app2.Arg, app1.Arg), S: t.Span()}
			}
		}
	}
	// 3-arg legacy: Computation pre post result (grade omitted).
	// normalizeCompApp runs during unification/zonking, where the full chain
	// is visible. Safe to normalize without Row restriction because depth-3
	// with Computation head can only be 3-arg at this point (4-arg would
	// have been caught by the 4-arg check above which requires depth-4).
	con, ok := app3.Fun.(*types.TyCon)
	if !ok {
		return t
	}
	switch con.Name {
	case types.TyConComputation:
		return &types.TyCBPV{Tag: types.TagComp, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, S: t.Span()}
	case types.TyConThunk:
		return &types.TyCBPV{Tag: types.TagThunk, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, S: t.Span()}
	}
	return t
}

// normalizeCompApp4Only normalizes only 4-arg Computation/Thunk TyApp chains.
// Used in Zonk where 3-arg normalization would be premature.
func normalizeCompApp4Only(t types.Type) types.Type {
	app1, ok := t.(*types.TyApp)
	if !ok {
		return t
	}
	app2, ok := app1.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	app3, ok := app2.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	app4, ok := app3.Fun.(*types.TyApp)
	if !ok {
		return t
	}
	con, ok := app4.Fun.(*types.TyCon)
	if !ok {
		return t
	}
	switch con.Name {
	case types.TyConComputation:
		return &types.TyCBPV{Tag: types.TagComp, Grade: app4.Arg, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, Flags: types.MetaFreeFlags(app4.Arg, app3.Arg, app2.Arg, app1.Arg), S: t.Span()}
	case types.TyConThunk:
		return &types.TyCBPV{Tag: types.TagThunk, Grade: app4.Arg, Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, Flags: types.MetaFreeFlags(app4.Arg, app3.Arg, app2.Arg, app1.Arg), S: t.Span()}
	}
	return t
}

// levelAdjacentCumulativity returns true if level a is exactly one level
// below level b: a + 1 == b. This captures Russell-style cumulativity
// where a kind at level ℓ inhabits the sort at level ℓ+1.
// Conservative: returns false if either side contains a LevelMeta.
func (u *Unifier) levelAdjacentCumulativity(a, b types.LevelExpr) bool {
	a = u.zonkLevel(a)
	b = u.zonkLevel(b)
	la, okA := a.(*types.LevelLit)
	lb, okB := b.(*types.LevelLit)
	if okA && okB {
		return la.N+1 == lb.N
	}
	return false
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
		return &UnifyError{Kind: UnifySkolemRigid, TypeB: b, Name: as.Name}
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
		return &UnifyError{Kind: UnifySkolemRigid, TypeA: a, Name: bs.Name}
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
			// Unify bodies with bound variables treated as equal.
			// Use a fresh variable to avoid capture: substitute both sides
			// to a common name that cannot clash with free variables.
			fresh := &types.TyVar{Name: at.Var}
			bodyA := at.Body
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
			// Unify grades: both nil = OK, one nil = skip (ungraded compat).
			if at.Grade != nil && bt.Grade != nil {
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
			for i := range at.Args {
				if err := u.Unify(at.Args[i], bt.Args[i]); err != nil {
					return err
				}
			}
			return nil
		}
	}

	return &UnifyError{Kind: UnifyMismatch, TypeA: a, TypeB: b}
}

// unifyAppWithTriple decomposes a TyApp chain and unifies its spine against
// a named type constructor with 3 fields (Computation or Thunk).
// This avoids the normalize cycle: normalizeCompApp converts TyApp→TyCBPV,
// while compToApp converts TyCBPV→TyApp, causing infinite recursion.
// Instead, we decompose the TyApp into (head, args) and unify each component directly.
func (u *Unifier) unifyAppWithTriple(app types.Type, conName string, fields [3]types.Type) error {
	head, args := types.UnwindApp(app)
	if len(args) < 3 {
		return &UnifyError{Kind: UnifyMismatch, TypeA: app, TypeB: types.Con(conName)}
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
		return &UnifyError{
			Kind:   UnifyUntouchable,
			MetaID: m.ID,
			Level:  m.Level,
			SLevel: u.SolverLevel,
		}
	}
	// Occurs check.
	if u.occursIn(m.ID, t) {
		return &UnifyError{Kind: UnifyOccursCheck, MetaID: m.ID, TypeA: t}
	}
	// Label uniqueness: if this meta has a label context, verify the
	// solution doesn't introduce duplicate labels (spec §8, §6.3).
	if ctx, ok := u.labels[m.ID]; ok {
		if ev, ok := t.(*types.TyEvidenceRow); ok {
			if cap, ok := ev.Entries.(*types.CapabilityEntries); ok {
				for _, f := range cap.Fields {
					if _, dup := ctx[f.Label]; dup {
						return &UnifyError{Kind: UnifyDupLabel, Label: f.Label}
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
