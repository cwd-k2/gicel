package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/types"
)

// UnifyErrorKind classifies unification failures for structured error reporting.
type UnifyErrorKind int

const (
	UnifyMismatch    UnifyErrorKind = iota // general type mismatch
	UnifyOccursCheck                       // infinite type (occurs check)
	UnifyDupLabel                          // duplicate label in row
	UnifyRowMismatch                       // row structure mismatch (extra labels, closed row)
	UnifySkolemRigid                       // rigid/skolem variable cannot be unified
)

// UnifyError is a structured error returned by the unifier.
type UnifyError struct {
	Kind   UnifyErrorKind
	Detail string
}

func (e *UnifyError) Error() string { return e.Detail }

// AliasExpander is a callback for expanding type aliases during unification.
type AliasExpander func(types.Type) types.Type

// Unifier manages type unification.
type Unifier struct {
	soln          map[int]types.Type
	labels        map[int]map[string]struct{}
	kindSoln      map[int]types.Kind // kind metavariable solutions
	freshID       *int
	aliasExpander AliasExpander // optional; set by Checker after alias processing
}

// NewUnifier creates a Unifier with its own internal fresh ID counter.
func NewUnifier() *Unifier {
	id := 0
	return &Unifier{
		soln:     make(map[int]types.Type),
		labels:   make(map[int]map[string]struct{}),
		kindSoln: make(map[int]types.Kind),
		freshID:  &id,
	}
}

// NewUnifierShared creates a Unifier that shares a fresh ID counter
// with the calling Checker, ensuring no ID collisions.
func NewUnifierShared(freshID *int) *Unifier {
	return &Unifier{
		soln:     make(map[int]types.Type),
		labels:   make(map[int]map[string]struct{}),
		kindSoln: make(map[int]types.Kind),
		freshID:  freshID,
	}
}

// Solve returns the current solution for a metavariable.
func (u *Unifier) Solve(id int) types.Type {
	return u.soln[id]
}

// Solutions returns the current solution map for introspection (e.g., skolem escape check).
func (u *Unifier) Solutions() map[int]types.Type {
	return u.soln
}

// Labels returns the label context map for save/restore during trial unification.
func (u *Unifier) Labels() map[int]map[string]struct{} {
	return u.labels
}

// KindSolutions returns the kind solution map for save/restore during trial unification.
func (u *Unifier) KindSolutions() map[int]types.Kind {
	return u.kindSoln
}

// UnifierSnapshot captures solutions, label contexts, and kind solutions for rollback.
type UnifierSnapshot struct {
	soln     map[int]types.Type
	labels   map[int]map[string]struct{}
	kindSoln map[int]types.Kind
}

// Snapshot captures the current unifier state for later rollback.
func (u *Unifier) Snapshot() UnifierSnapshot {
	soln := make(map[int]types.Type, len(u.soln))
	for k, v := range u.soln {
		soln[k] = v
	}
	labels := make(map[int]map[string]struct{}, len(u.labels))
	for k, v := range u.labels {
		inner := make(map[string]struct{}, len(v))
		for label := range v {
			inner[label] = struct{}{}
		}
		labels[k] = inner
	}
	kindSoln := make(map[int]types.Kind, len(u.kindSoln))
	for k, v := range u.kindSoln {
		kindSoln[k] = v
	}
	return UnifierSnapshot{soln: soln, labels: labels, kindSoln: kindSoln}
}

// Restore rolls back the unifier to a previously saved snapshot.
func (u *Unifier) Restore(snap UnifierSnapshot) {
	for k := range u.soln {
		if _, existed := snap.soln[k]; !existed {
			delete(u.soln, k)
		}
	}
	for k, v := range snap.soln {
		u.soln[k] = v
	}
	for k := range u.labels {
		if _, existed := snap.labels[k]; !existed {
			delete(u.labels, k)
		}
	}
	for k, v := range snap.labels {
		u.labels[k] = v
	}
	for k := range u.kindSoln {
		if _, existed := snap.kindSoln[k]; !existed {
			delete(u.kindSoln, k)
		}
	}
	for k, v := range snap.kindSoln {
		u.kindSoln[k] = v
	}
}

// RegisterLabelContext records the surrounding labels for a row metavariable.
func (u *Unifier) RegisterLabelContext(id int, labels map[string]struct{}) {
	u.labels[id] = labels
}

// Zonk replaces all solved metavariables in a type.
// Optimizations:
//   - Path compression: meta chains (m1 → m2 → Int) are compressed so
//     soln[m1] points directly to the final answer.
//   - Structural identity: if all children are unchanged (pointer-equal),
//     the original node is returned (avoids allocation).
func (u *Unifier) Zonk(t types.Type) types.Type {
	switch ty := t.(type) {
	case *types.TyMeta:
		soln, ok := u.soln[ty.ID]
		if !ok {
			return ty
		}
		result := u.Zonk(soln)
		if result != soln {
			u.soln[ty.ID] = result // path compression
		}
		return result
	case *types.TyApp:
		zFun := u.Zonk(ty.Fun)
		zArg := u.Zonk(ty.Arg)
		if zFun == ty.Fun && zArg == ty.Arg {
			return ty
		}
		return &types.TyApp{Fun: zFun, Arg: zArg, S: ty.S}
	case *types.TyArrow:
		zFrom := u.Zonk(ty.From)
		zTo := u.Zonk(ty.To)
		if zFrom == ty.From && zTo == ty.To {
			return ty
		}
		return &types.TyArrow{From: zFrom, To: zTo, S: ty.S}
	case *types.TyForall:
		zBody := u.Zonk(ty.Body)
		if zBody == ty.Body {
			return ty
		}
		return &types.TyForall{Var: ty.Var, Kind: ty.Kind, Body: zBody, S: ty.S}
	case *types.TyComp:
		zPre := u.Zonk(ty.Pre)
		zPost := u.Zonk(ty.Post)
		zResult := u.Zonk(ty.Result)
		if zPre == ty.Pre && zPost == ty.Post && zResult == ty.Result {
			return ty
		}
		return &types.TyComp{Pre: zPre, Post: zPost, Result: zResult, S: ty.S}
	case *types.TyThunk:
		zPre := u.Zonk(ty.Pre)
		zPost := u.Zonk(ty.Post)
		zResult := u.Zonk(ty.Result)
		if zPre == ty.Pre && zPost == ty.Post && zResult == ty.Result {
			return ty
		}
		return &types.TyThunk{Pre: zPre, Post: zPost, Result: zResult, S: ty.S}
	case *types.TyEvidenceRow:
		switch entries := ty.Entries.(type) {
		case *types.CapabilityEntries:
			changed := false
			fields := make([]types.RowField, len(entries.Fields))
			for i, f := range entries.Fields {
				zTy := u.Zonk(f.Type)
				fields[i] = types.RowField{Label: f.Label, Type: zTy, S: f.S}
				if zTy != f.Type {
					changed = true
				}
			}
			var tail types.Type
			if ty.Tail != nil {
				tail = u.Zonk(ty.Tail)
				if tail != ty.Tail {
					changed = true
				}
			}
			if !changed {
				return ty
			}
			return &types.TyEvidenceRow{Entries: &types.CapabilityEntries{Fields: fields}, Tail: tail, S: ty.S}
		case *types.ConstraintEntries:
			changed := false
			ces := make([]types.ConstraintEntry, len(entries.Entries))
			for i, e := range entries.Entries {
				ces[i] = u.zonkConstraintEntry(e, &changed)
			}
			var tail types.Type
			if ty.Tail != nil {
				tail = u.Zonk(ty.Tail)
				if tail != ty.Tail {
					changed = true
				}
			}
			if !changed {
				return ty
			}
			return &types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: ces}, Tail: tail, S: ty.S}
		default:
			return ty
		}
	case *types.TyEvidence:
		zConstraints := u.Zonk(ty.Constraints)
		zBody := u.Zonk(ty.Body)
		if zConstraints == ty.Constraints && zBody == ty.Body {
			return ty
		}
		cr, ok := zConstraints.(*types.TyEvidenceRow)
		if !ok {
			// Zonk produced a non-evidence-row (e.g., solved meta);
			// preserve original constraints to avoid nil dereference.
			return &types.TyEvidence{Constraints: ty.Constraints, Body: zBody, S: ty.S}
		}
		return &types.TyEvidence{Constraints: cr, Body: zBody, S: ty.S}
	case *types.TySkolem:
		return ty
	default:
		return t
	}
}

// normalize applies alias expansion and special type normalization.
func (u *Unifier) normalize(t types.Type) types.Type {
	if u.aliasExpander != nil {
		t = u.aliasExpander(t)
	}
	return normalizeCompApp(t)
}

// normalizeCompApp converts fully-applied TyApp chains to their special type
// representations. e.g. TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
// becomes TyComp{pre, post, result}. This arises when a class type parameter
// (m : Row -> Row -> Type -> Type) is substituted with Computation.
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
	con, ok := app3.Fun.(*types.TyCon)
	if !ok {
		return t
	}
	switch con.Name {
	case "Computation":
		return &types.TyComp{Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, S: t.Span()}
	case "Thunk":
		return &types.TyThunk{Pre: app3.Arg, Post: app2.Arg, Result: app1.Arg, S: t.Span()}
	}
	return t
}

// Unify solves the constraint a ~ b.
func (u *Unifier) Unify(a, b types.Type) error {
	a = u.Zonk(a)
	b = u.Zonk(b)

	// Normalize special type applications and expand aliases.
	a = u.normalize(a)
	b = u.normalize(b)

	// Error types unify with anything.
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

	// Skolem: rigid type variables cannot be unified with anything except themselves.
	if as, ok := a.(*types.TySkolem); ok {
		if bs, ok := b.(*types.TySkolem); ok && as.ID == bs.ID {
			return nil
		}
		return &UnifyError{Kind: UnifySkolemRigid, Detail: fmt.Sprintf("cannot unify rigid type variable #%s with %s", as.Name, types.Pretty(b))}
	}
	if bs, ok := b.(*types.TySkolem); ok {
		return &UnifyError{Kind: UnifySkolemRigid, Detail: fmt.Sprintf("cannot unify %s with rigid type variable #%s", types.Pretty(a), bs.Name)}
	}

	switch at := a.(type) {
	case *types.TyVar:
		if bt, ok := b.(*types.TyVar); ok && at.Name == bt.Name {
			return nil
		}
	case *types.TyCon:
		if bt, ok := b.(*types.TyCon); ok && at.Name == bt.Name {
			return nil
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
		// Cross-case: decompose TyApp spine directly against TyComp/TyThunk
		// to avoid the normalize cycle (normalizeCompApp ↔ compToApp).
		if comp, ok := b.(*types.TyComp); ok {
			return u.unifyAppWithTriple(a, "Computation", [3]types.Type{comp.Pre, comp.Post, comp.Result})
		}
		if thk, ok := b.(*types.TyThunk); ok {
			return u.unifyAppWithTriple(a, "Thunk", [3]types.Type{thk.Pre, thk.Post, thk.Result})
		}
	case *types.TyForall:
		if bt, ok := b.(*types.TyForall); ok {
			// Unify bodies with bound variables treated as equal.
			return u.Unify(at.Body, types.Subst(bt.Body, bt.Var, &types.TyVar{Name: at.Var}))
		}
	case *types.TyComp:
		if bt, ok := b.(*types.TyComp); ok {
			if err := u.Unify(at.Pre, bt.Pre); err != nil {
				return err
			}
			if err := u.Unify(at.Post, bt.Post); err != nil {
				return err
			}
			return u.Unify(at.Result, bt.Result)
		}
		if _, ok := b.(*types.TyApp); ok {
			return u.unifyAppWithTriple(b, "Computation", [3]types.Type{at.Pre, at.Post, at.Result})
		}
	case *types.TyThunk:
		if bt, ok := b.(*types.TyThunk); ok {
			if err := u.Unify(at.Pre, bt.Pre); err != nil {
				return err
			}
			if err := u.Unify(at.Post, bt.Post); err != nil {
				return err
			}
			return u.Unify(at.Result, bt.Result)
		}
		if _, ok := b.(*types.TyApp); ok {
			return u.unifyAppWithTriple(b, "Thunk", [3]types.Type{at.Pre, at.Post, at.Result})
		}
	case *types.TyEvidenceRow:
		if bt, ok := b.(*types.TyEvidenceRow); ok {
			return u.unifyEvidenceRows(at, bt)
		}
	case *types.TyEvidence:
		if bt, ok := b.(*types.TyEvidence); ok {
			if err := u.Unify(at.Constraints, bt.Constraints); err != nil {
				return err
			}
			return u.Unify(at.Body, bt.Body)
		}
	}

	return &UnifyError{Kind: UnifyMismatch, Detail: fmt.Sprintf("type mismatch: %s vs %s", types.Pretty(a), types.Pretty(b))}
}

// unifyAppWithTriple decomposes a TyApp chain and unifies its spine against
// a named type constructor with 3 fields (Computation or Thunk).
// This avoids the normalize cycle: normalizeCompApp converts TyApp→TyComp,
// while compToApp converts TyComp→TyApp, causing infinite recursion.
// Instead, we decompose the TyApp into (head, args) and unify each component directly.
func (u *Unifier) unifyAppWithTriple(app types.Type, conName string, fields [3]types.Type) error {
	head, args := types.UnwindApp(app)
	if len(args) < 3 {
		return &UnifyError{Kind: UnifyMismatch, Detail: fmt.Sprintf("type mismatch: %s vs %s ...", types.Pretty(app), conName)}
	}
	// Reconstruct head with excess leading args (handles len(args) > 3).
	conHead := head
	for _, arg := range args[:len(args)-3] {
		conHead = &types.TyApp{Fun: conHead, Arg: arg}
	}
	if err := u.Unify(conHead, &types.TyCon{Name: conName}); err != nil {
		return err
	}
	for i := 0; i < 3; i++ {
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
	// Occurs check.
	if u.occursIn(m.ID, t) {
		return &UnifyError{Kind: UnifyOccursCheck, Detail: fmt.Sprintf("infinite type: ?%d occurs in %s", m.ID, types.Pretty(t))}
	}
	// Label uniqueness: if this meta has a label context, verify the
	// solution doesn't introduce duplicate labels (spec §8, §6.3).
	if ctx, ok := u.labels[m.ID]; ok {
		if ev, ok := t.(*types.TyEvidenceRow); ok {
			if cap, ok := ev.Entries.(*types.CapabilityEntries); ok {
				for _, f := range cap.Fields {
					if _, dup := ctx[f.Label]; dup {
						return &UnifyError{Kind: UnifyDupLabel, Detail: fmt.Sprintf("duplicate label %q in row", f.Label)}
					}
				}
			}
		}
	}
	u.soln[m.ID] = t
	return nil
}

// occursIn uses Children() for generic traversal, unlike Zonk which uses
// manual recursion for identity-preserving path compression.
func (u *Unifier) occursIn(id int, t types.Type) bool {
	t = u.Zonk(t)
	switch ty := t.(type) {
	case *types.TyMeta:
		return ty.ID == id
	case *types.TySkolem:
		_ = ty
		return false // skolem IDs are in a different namespace
	default:
		for _, ch := range t.Children() {
			if u.occursIn(id, ch) {
				return true
			}
		}
		return false
	}
}
