package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/pkg/types"
)

// Unifier manages type unification.
type Unifier struct {
	soln   map[int]types.Type
	labels map[int]map[string]struct{}
}

func NewUnifier() *Unifier {
	return &Unifier{
		soln:   make(map[int]types.Type),
		labels: make(map[int]map[string]struct{}),
	}
}

// Solve returns the current solution for a metavariable.
func (u *Unifier) Solve(id int) types.Type {
	return u.soln[id]
}

// RegisterLabelContext records the surrounding labels for a row metavariable.
func (u *Unifier) RegisterLabelContext(id int, labels map[string]struct{}) {
	u.labels[id] = labels
}

// Zonk replaces all solved metavariables in a type.
func (u *Unifier) Zonk(t types.Type) types.Type {
	switch ty := t.(type) {
	case *types.TyMeta:
		if soln, ok := u.soln[ty.ID]; ok {
			return u.Zonk(soln)
		}
		return ty
	case *types.TyApp:
		return &types.TyApp{Fun: u.Zonk(ty.Fun), Arg: u.Zonk(ty.Arg), S: ty.S}
	case *types.TyArrow:
		return &types.TyArrow{From: u.Zonk(ty.From), To: u.Zonk(ty.To), S: ty.S}
	case *types.TyForall:
		return &types.TyForall{Var: ty.Var, Kind: ty.Kind, Body: u.Zonk(ty.Body), S: ty.S}
	case *types.TyComp:
		return &types.TyComp{Pre: u.Zonk(ty.Pre), Post: u.Zonk(ty.Post), Result: u.Zonk(ty.Result), S: ty.S}
	case *types.TyThunk:
		return &types.TyThunk{Pre: u.Zonk(ty.Pre), Post: u.Zonk(ty.Post), Result: u.Zonk(ty.Result), S: ty.S}
	case *types.TyRow:
		fields := make([]types.RowField, len(ty.Fields))
		for i, f := range ty.Fields {
			fields[i] = types.RowField{Label: f.Label, Type: u.Zonk(f.Type), S: f.S}
		}
		var tail types.Type
		if ty.Tail != nil {
			tail = u.Zonk(ty.Tail)
		}
		return &types.TyRow{Fields: fields, Tail: tail, S: ty.S}
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
	u.soln[m.ID] = t
	return nil
}

func (u *Unifier) occursIn(id int, t types.Type) bool {
	t = u.Zonk(t)
	switch ty := t.(type) {
	case *types.TyMeta:
		return ty.ID == id
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
		// Open-Open: introduce fresh row variable.
		fresh := &types.TyMeta{ID: -1} // placeholder
		_ = fresh
		if err := u.solveRowTail(r1.Tail, collectFields(r2, onlyRight), r2.Tail); err != nil {
			return err
		}
		return u.solveRowTail(r2.Tail, collectFields(r1, onlyLeft), r1.Tail)
	}
	return nil
}

func (u *Unifier) solveRowTail(tail types.Type, fields []types.RowField, newTail types.Type) error {
	solution := &types.TyRow{Fields: fields, Tail: newTail}
	if len(fields) == 0 && newTail == nil {
		solution = types.EmptyRow()
	}
	return u.Unify(tail, solution)
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
