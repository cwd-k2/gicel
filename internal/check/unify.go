package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/pkg/types"
)

// Unifier manages type unification.
type Unifier struct {
	soln    map[int]types.Type
	labels  map[int]map[string]struct{}
	freshID *int
}

// NewUnifier creates a Unifier with its own internal fresh ID counter.
func NewUnifier() *Unifier {
	id := 0
	return &Unifier{
		soln:    make(map[int]types.Type),
		labels:  make(map[int]map[string]struct{}),
		freshID: &id,
	}
}

// NewUnifierShared creates a Unifier that shares a fresh ID counter
// with the calling Checker, ensuring no ID collisions.
func NewUnifierShared(freshID *int) *Unifier {
	return &Unifier{
		soln:    make(map[int]types.Type),
		labels:  make(map[int]map[string]struct{}),
		freshID: freshID,
	}
}

// freshMeta allocates a fresh metavariable of the given kind.
func (u *Unifier) freshMeta(k types.Kind) *types.TyMeta {
	*u.freshID++
	return &types.TyMeta{ID: *u.freshID, Kind: k}
}

// Solve returns the current solution for a metavariable.
func (u *Unifier) Solve(id int) types.Type {
	return u.soln[id]
}

// Solutions returns the current solution map for introspection (e.g., skolem escape check).
func (u *Unifier) Solutions() map[int]types.Type {
	return u.soln
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
	case *types.TyQual:
		changed := false
		args := make([]types.Type, len(ty.Args))
		for i, a := range ty.Args {
			args[i] = u.Zonk(a)
			if args[i] != a {
				changed = true
			}
		}
		zBody := u.Zonk(ty.Body)
		if !changed && zBody == ty.Body {
			return ty
		}
		return &types.TyQual{ClassName: ty.ClassName, Args: args, Body: zBody, S: ty.S}
	case *types.TyRow:
		changed := false
		fields := make([]types.RowField, len(ty.Fields))
		for i, f := range ty.Fields {
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
		return &types.TyRow{Fields: fields, Tail: tail, S: ty.S}
	case *types.TySkolem:
		return ty
	default:
		return t
	}
}

// Unify solves the constraint a ~ b.
func (u *Unifier) Unify(a, b types.Type) error {
	a = u.Zonk(a)
	b = u.Zonk(b)

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
		return fmt.Errorf("cannot unify rigid type variable #%s with %s", as.Name, types.Pretty(b))
	}
	if bs, ok := b.(*types.TySkolem); ok {
		return fmt.Errorf("cannot unify %s with rigid type variable #%s", types.Pretty(a), bs.Name)
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
	case *types.TyQual:
		if bt, ok := b.(*types.TyQual); ok {
			if at.ClassName != bt.ClassName || len(at.Args) != len(bt.Args) {
				break
			}
			for i := range at.Args {
				if err := u.Unify(at.Args[i], bt.Args[i]); err != nil {
					return err
				}
			}
			return u.Unify(at.Body, bt.Body)
		}
	case *types.TyRow:
		if bt, ok := b.(*types.TyRow); ok {
			return u.unifyRows(at, bt)
		}
	}

	return fmt.Errorf("type mismatch: %s vs %s", types.Pretty(a), types.Pretty(b))
}

func (u *Unifier) solveMeta(m *types.TyMeta, t types.Type) error {
	if tm, ok := t.(*types.TyMeta); ok && tm.ID == m.ID {
		return nil
	}
	// Occurs check.
	if u.occursIn(m.ID, t) {
		return fmt.Errorf("infinite type: ?%d occurs in %s", m.ID, types.Pretty(t))
	}
	// Label uniqueness: if this meta has a label context, verify the
	// solution doesn't introduce duplicate labels (spec §8, §6.3).
	if ctx, ok := u.labels[m.ID]; ok {
		if row, ok := t.(*types.TyRow); ok {
			for _, f := range row.Fields {
				if _, dup := ctx[f.Label]; dup {
					return fmt.Errorf("duplicate label %q in row", f.Label)
				}
			}
		}
	}
	u.soln[m.ID] = t
	return nil
}

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

// Row unification
func (u *Unifier) unifyRows(r1, r2 *types.TyRow) error {
	r1 = types.Normalize(r1)
	r2 = types.Normalize(r2)

	// Register label contexts for open-row tails (spec §8: label uniqueness preservation).
	u.registerRowLabels(r1)
	u.registerRowLabels(r2)

	shared, onlyLeft, onlyRight := classifyFields(r1.Fields, r2.Fields)

	// Unify shared labels.
	for _, label := range shared {
		t1 := fieldType(r1, label)
		t2 := fieldType(r2, label)
		if err := u.Unify(t1, t2); err != nil {
			return err
		}
	}

	switch {
	case r1.Tail == nil && r2.Tail == nil:
		if len(onlyLeft) > 0 || len(onlyRight) > 0 {
			return fmt.Errorf("row mismatch: extra labels %v / %v", onlyLeft, onlyRight)
		}
	case r1.Tail != nil && r2.Tail == nil:
		if len(onlyLeft) > 0 {
			return fmt.Errorf("extra labels in row: %v", onlyLeft)
		}
		return u.solveRowTail(r1.Tail, collectFields(r2, onlyRight), nil)
	case r1.Tail == nil && r2.Tail != nil:
		if len(onlyRight) > 0 {
			return fmt.Errorf("extra labels in row: %v", onlyRight)
		}
		return u.solveRowTail(r2.Tail, collectFields(r1, onlyLeft), nil)
	default:
		// Open-Open: introduce fresh row metavariable.
		// Given { shared, onlyLeft | r1.Tail } ~ { shared, onlyRight | r2.Tail }:
		//   r1.Tail = { onlyRight | r_fresh }
		//   r2.Tail = { onlyLeft  | r_fresh }
		rFresh := u.freshMeta(types.KRow{})
		if err := u.solveRowTail(r1.Tail, collectFields(r2, onlyRight), rFresh); err != nil {
			return err
		}
		return u.solveRowTail(r2.Tail, collectFields(r1, onlyLeft), rFresh)
	}
	return nil
}

func (u *Unifier) solveRowTail(tail types.Type, fields []types.RowField, newTail types.Type) error {
	// { | t } is equivalent to t — unify tail directly when no extra fields.
	if len(fields) == 0 && newTail != nil {
		return u.Unify(tail, newTail)
	}
	solution := &types.TyRow{Fields: fields, Tail: newTail}
	if len(fields) == 0 && newTail == nil {
		solution = types.EmptyRow()
	}
	return u.Unify(tail, solution)
}

// registerRowLabels records a row's field labels as the label context
// for its tail metavariable (if any).
func (u *Unifier) registerRowLabels(r *types.TyRow) {
	if r.Tail == nil {
		return
	}
	tail := u.Zonk(r.Tail)
	if m, ok := tail.(*types.TyMeta); ok {
		labels := make(map[string]struct{}, len(r.Fields))
		for _, f := range r.Fields {
			labels[f.Label] = struct{}{}
		}
		// Merge with any existing context.
		if existing, ok := u.labels[m.ID]; ok {
			for l := range existing {
				labels[l] = struct{}{}
			}
		}
		u.labels[m.ID] = labels
	}
}

func classifyFields(a, b []types.RowField) (shared, onlyA, onlyB []string) {
	aMap := make(map[string]bool)
	bMap := make(map[string]bool)
	for _, f := range a {
		aMap[f.Label] = true
	}
	for _, f := range b {
		bMap[f.Label] = true
	}
	for _, f := range a {
		if bMap[f.Label] {
			shared = append(shared, f.Label)
		} else {
			onlyA = append(onlyA, f.Label)
		}
	}
	for _, f := range b {
		if !aMap[f.Label] {
			onlyB = append(onlyB, f.Label)
		}
	}
	return
}

func fieldType(r *types.TyRow, label string) types.Type {
	for _, f := range r.Fields {
		if f.Label == label {
			return f.Type
		}
	}
	return nil
}

func collectFields(r *types.TyRow, labels []string) []types.RowField {
	set := make(map[string]bool, len(labels))
	for _, l := range labels {
		set[l] = true
	}
	var fields []types.RowField
	for _, f := range r.Fields {
		if set[f.Label] {
			fields = append(fields, f)
		}
	}
	return fields
}
