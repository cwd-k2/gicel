package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/internal/types"
)

// AliasExpander is a callback for expanding type aliases during unification.
type AliasExpander func(types.Type) types.Type

// Unifier manages type unification.
type Unifier struct {
	soln          map[int]types.Type
	labels        map[int]map[string]struct{}
	freshID       *int
	aliasExpander AliasExpander // optional; set by Checker after alias processing
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
	case *types.TyConstraintRow:
		changed := false
		entries := make([]types.ConstraintEntry, len(ty.Entries))
		for i, e := range ty.Entries {
			entries[i] = u.zonkConstraintEntry(e, &changed)
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
		return &types.TyConstraintRow{Entries: entries, Tail: tail, S: ty.S}
	case *types.TyEvidence:
		zConstraints := u.Zonk(ty.Constraints)
		zBody := u.Zonk(ty.Body)
		if zConstraints == ty.Constraints && zBody == ty.Body {
			return ty
		}
		cr, _ := zConstraints.(*types.TyConstraintRow)
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
	case *types.TyRow:
		if bt, ok := b.(*types.TyRow); ok {
			return u.unifyRows(at, bt)
		}
	case *types.TyConstraintRow:
		if bt, ok := b.(*types.TyConstraintRow); ok {
			return u.unifyConstraintRows(at, bt)
		}
	case *types.TyEvidence:
		if bt, ok := b.(*types.TyEvidence); ok {
			if err := u.Unify(at.Constraints, bt.Constraints); err != nil {
				return err
			}
			return u.Unify(at.Body, bt.Body)
		}
	}

	return fmt.Errorf("type mismatch: %s vs %s", types.Pretty(a), types.Pretty(b))
}

// unifyAppWithTriple decomposes a TyApp chain and unifies its spine against
// a named type constructor with 3 fields (Computation or Thunk).
// This avoids the normalize cycle: normalizeCompApp converts TyApp→TyComp,
// while compToApp converts TyComp→TyApp, causing infinite recursion.
// Instead, we decompose the TyApp into (head, args) and unify each component directly.
func (u *Unifier) unifyAppWithTriple(app types.Type, conName string, fields [3]types.Type) error {
	head, args := types.UnwindApp(app)
	if len(args) < 3 {
		return fmt.Errorf("type mismatch: %s vs %s ...", types.Pretty(app), conName)
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

// Constraint row unification — parallel to unifyRows.
func (u *Unifier) unifyConstraintRows(r1, r2 *types.TyConstraintRow) error {
	r1 = types.NormalizeConstraints(r1)
	r2 = types.NormalizeConstraints(r2)

	shared, onlyLeft, onlyRight := classifyConstraints(r1.Entries, r2.Entries, u)

	// Unify shared entries' Args.
	for _, m := range shared {
		if len(m.A.Args) != len(m.B.Args) {
			return fmt.Errorf("constraint arg count mismatch: %s has %d args vs %d",
				m.A.ClassName, len(m.A.Args), len(m.B.Args))
		}
		for i := range m.A.Args {
			if err := u.Unify(m.A.Args[i], m.B.Args[i]); err != nil {
				return err
			}
		}
	}

	switch {
	case r1.Tail == nil && r2.Tail == nil:
		if len(onlyLeft) > 0 || len(onlyRight) > 0 {
			return fmt.Errorf("constraint row mismatch: extra constraints left=%d right=%d",
				len(onlyLeft), len(onlyRight))
		}
	case r1.Tail != nil && r2.Tail == nil:
		if len(onlyLeft) > 0 {
			return fmt.Errorf("extra constraints in left row: %d", len(onlyLeft))
		}
		return u.solveConstraintTail(r1.Tail, onlyRight, nil)
	case r1.Tail == nil && r2.Tail != nil:
		if len(onlyRight) > 0 {
			return fmt.Errorf("extra constraints in right row: %d", len(onlyRight))
		}
		return u.solveConstraintTail(r2.Tail, onlyLeft, nil)
	default:
		// Open-Open: fresh constraint metavariable.
		cFresh := u.freshMeta(types.KConstraint{})
		if err := u.solveConstraintTail(r1.Tail, onlyRight, cFresh); err != nil {
			return err
		}
		return u.solveConstraintTail(r2.Tail, onlyLeft, cFresh)
	}
	return nil
}

func (u *Unifier) solveConstraintTail(tail types.Type, entries []types.ConstraintEntry, newTail types.Type) error {
	if len(entries) == 0 && newTail != nil {
		return u.Unify(tail, newTail)
	}
	solution := &types.TyConstraintRow{Entries: entries, Tail: newTail}
	if len(entries) == 0 && newTail == nil {
		solution = types.EmptyConstraintRow()
	}
	return u.Unify(tail, solution)
}

type constraintMatch struct {
	A, B types.ConstraintEntry
}

// classifyConstraints partitions constraint entries into shared (matched by className),
// onlyA, and onlyB. For entries with the same className, we attempt greedy matching.
func classifyConstraints(a, b []types.ConstraintEntry, u *Unifier) (
	shared []constraintMatch,
	onlyA, onlyB []types.ConstraintEntry,
) {
	// Build index by className for b entries.
	bByClass := make(map[string][]int)
	for i, e := range b {
		bByClass[e.ClassName] = append(bByClass[e.ClassName], i)
	}
	bUsed := make([]bool, len(b))

	for _, ea := range a {
		matched := false
		candidates := bByClass[ea.ClassName]
		for _, bi := range candidates {
			if bUsed[bi] {
				continue
			}
			eb := b[bi]
			// Match by className equality. Args will be unified later.
			// For same className with multiple entries (e.g., Eq a, Eq b),
			// use canonical key matching as a heuristic.
			if types.ConstraintKey(ea) == types.ConstraintKey(eb) {
				shared = append(shared, constraintMatch{A: ea, B: eb})
				bUsed[bi] = true
				matched = true
				break
			}
		}
		if !matched {
			// Try positional match for same className (handles meta variables).
			for _, bi := range candidates {
				if bUsed[bi] {
					continue
				}
				shared = append(shared, constraintMatch{A: ea, B: b[bi]})
				bUsed[bi] = true
				matched = true
				break
			}
		}
		if !matched {
			onlyA = append(onlyA, ea)
		}
	}
	for i, e := range b {
		if !bUsed[i] {
			onlyB = append(onlyB, e)
		}
	}
	return
}

// zonkConstraintEntry zonks a single constraint entry, including any quantified sub-structure.
func (u *Unifier) zonkConstraintEntry(e types.ConstraintEntry, changed *bool) types.ConstraintEntry {
	args := make([]types.Type, len(e.Args))
	for j, a := range e.Args {
		args[j] = u.Zonk(a)
		if args[j] != a {
			*changed = true
		}
	}
	result := types.ConstraintEntry{ClassName: e.ClassName, Args: args, S: e.S}
	if e.ConstraintVar != nil {
		newCV := u.Zonk(e.ConstraintVar)
		if newCV != e.ConstraintVar {
			*changed = true
		}
		result.ConstraintVar = newCV
		// If zonked ConstraintVar is now concrete, decompose into ClassName + Args.
		if result.ClassName == "" {
			if cn, cArgs, ok := DecomposeConstraintType(newCV); ok {
				result.ClassName = cn
				result.Args = cArgs
			}
		}
	}
	if e.Quantified != nil {
		qc := u.zonkQuantifiedConstraint(e.Quantified, changed)
		result.Quantified = qc
	}
	return result
}

func (u *Unifier) zonkQuantifiedConstraint(qc *types.QuantifiedConstraint, changed *bool) *types.QuantifiedConstraint {
	ctx := make([]types.ConstraintEntry, len(qc.Context))
	for i, c := range qc.Context {
		ctx[i] = u.zonkConstraintEntry(c, changed)
	}
	head := u.zonkConstraintEntry(qc.Head, changed)
	return &types.QuantifiedConstraint{Vars: qc.Vars, Context: ctx, Head: head}
}

// DecomposeConstraintType decomposes a concrete constraint type (e.g., TyApp(TyCon("Eq"), TyCon("Bool")))
// into its class name and type arguments. Returns ("Eq", [Bool], true) for the example above.
func DecomposeConstraintType(ty types.Type) (className string, args []types.Type, ok bool) {
	head, tArgs := types.UnwindApp(ty)
	if con, isCon := head.(*types.TyCon); isCon {
		return con.Name, tArgs, true
	}
	return "", nil, false
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
