package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/types"
)

// Row unification

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

func fieldType(fields []types.RowField, label string) types.Type {
	for _, f := range fields {
		if f.Label == label {
			return f.Type
		}
	}
	return nil
}

func fieldMult(fields []types.RowField, label string) types.Type {
	for _, f := range fields {
		if f.Label == label {
			return f.Mult
		}
	}
	return nil
}

type constraintMatch struct {
	A, B types.ConstraintEntry
}

// constraintArgsEqual checks if two constraint entries have the same className
// and structurally equal args. Uses types.Equal for semantic comparison
// rather than Pretty-based string matching.
func constraintArgsEqual(a, b types.ConstraintEntry) bool {
	if a.ClassName != b.ClassName || len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if !types.Equal(a.Args[i], b.Args[i]) {
			return false
		}
	}
	return true
}

// classifyConstraints partitions constraint entries into shared (matched by className),
// onlyA, and onlyB. For entries with the same className, we attempt greedy matching.
func classifyConstraints(a, b []types.ConstraintEntry, _ *Unifier) (
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
		// First pass: match by structural equality on args (precise).
		for _, bi := range candidates {
			if bUsed[bi] {
				continue
			}
			eb := b[bi]
			if constraintArgsEqual(ea, eb) {
				shared = append(shared, constraintMatch{A: ea, B: eb})
				bUsed[bi] = true
				matched = true
				break
			}
		}
		if !matched {
			// Fallback: positional match for same className (handles meta variables).
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

// decomposeConstraintType decomposes a concrete constraint type (e.g., TyApp(TyCon("Eq"), TyCon("Bool")))
// into its class name and type arguments. Returns ("Eq", [Bool], true) for the example above.
func decomposeConstraintType(ty types.Type) (className string, args []types.Type, ok bool) {
	head, tArgs := types.UnwindApp(ty)
	if con, isCon := head.(*types.TyCon); isCon {
		return con.Name, tArgs, true
	}
	return "", nil, false
}

// Evidence row unification — dispatches to capability or constraint logic.
func (u *Unifier) unifyEvidenceRows(r1, r2 *types.TyEvidenceRow) error {
	switch a := r1.Entries.(type) {
	case *types.CapabilityEntries:
		b, ok := r2.Entries.(*types.CapabilityEntries)
		if !ok {
			return &UnifyError{Kind: UnifyMismatch, Detail: "cannot unify capability row with constraint row"}
		}
		return u.unifyEvCapRows(a.Fields, r1.Tail, b.Fields, r2.Tail)
	case *types.ConstraintEntries:
		b, ok := r2.Entries.(*types.ConstraintEntries)
		if !ok {
			return &UnifyError{Kind: UnifyMismatch, Detail: "cannot unify constraint row with capability row"}
		}
		return u.unifyEvConRows(a.Entries, r1.Tail, b.Entries, r2.Tail)
	default:
		return &UnifyError{Kind: UnifyMismatch, Detail: "unknown evidence fiber"}
	}
}

func (u *Unifier) unifyEvCapRows(
	aFields []types.RowField, aTail types.Type,
	bFields []types.RowField, bTail types.Type,
) error {
	// Normalize field order.
	an, err := types.NormalizeRow(&types.TyEvidenceRow{Entries: &types.CapabilityEntries{Fields: aFields}, Tail: aTail})
	if err != nil {
		return &UnifyError{Kind: UnifyMismatch, Detail: err.Error()}
	}
	bn, err := types.NormalizeRow(&types.TyEvidenceRow{Entries: &types.CapabilityEntries{Fields: bFields}, Tail: bTail})
	if err != nil {
		return &UnifyError{Kind: UnifyMismatch, Detail: err.Error()}
	}
	aFieldsN := an.CapFields()
	bFieldsN := bn.CapFields()

	// Register label contexts for open-row tails.
	u.registerEvCapLabels(aFieldsN, an.Tail)
	u.registerEvCapLabels(bFieldsN, bn.Tail)

	shared, onlyLeft, onlyRight := classifyFields(aFieldsN, bFieldsN)

	for _, label := range shared {
		t1 := fieldType(aFieldsN, label)
		t2 := fieldType(bFieldsN, label)
		if err := u.Unify(t1, t2); err != nil {
			return err
		}
		// Unify multiplicity annotations if both sides have them.
		m1 := fieldMult(aFieldsN, label)
		m2 := fieldMult(bFieldsN, label)
		if m1 != nil && m2 != nil {
			if err := u.Unify(m1, m2); err != nil {
				return err
			}
		}
	}

	onlyAEntries := &types.CapabilityEntries{Fields: collectEvCapFields(aFieldsN, onlyLeft)}
	onlyBEntries := &types.CapabilityEntries{Fields: collectEvCapFields(bFieldsN, onlyRight)}
	return u.resolveEvidenceTails(an.Tail, bn.Tail, onlyAEntries, onlyBEntries)
}

func (u *Unifier) registerEvCapLabels(fields []types.RowField, tail types.Type) {
	if tail == nil {
		return
	}
	zt := u.Zonk(tail)
	if m, ok := zt.(*types.TyMeta); ok {
		labels := make(map[string]struct{}, len(fields))
		for _, f := range fields {
			labels[f.Label] = struct{}{}
		}
		if existing, ok := u.labels[m.ID]; ok {
			for l := range existing {
				labels[l] = struct{}{}
			}
		}
		u.labels[m.ID] = labels
	}
}

func collectEvCapFields(fields []types.RowField, labels []string) []types.RowField {
	set := make(map[string]bool, len(labels))
	for _, l := range labels {
		set[l] = true
	}
	var result []types.RowField
	for _, f := range fields {
		if set[f.Label] {
			result = append(result, f)
		}
	}
	return result
}

func (u *Unifier) unifyEvConRows(
	aEntries []types.ConstraintEntry, aTail types.Type,
	bEntries []types.ConstraintEntry, bTail types.Type,
) error {
	aN := types.NormalizeConstraints(&types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: aEntries}, Tail: aTail})
	bN := types.NormalizeConstraints(&types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: bEntries}, Tail: bTail})

	shared, onlyLeft, onlyRight := classifyConstraints(aN.ConEntries(), bN.ConEntries(), u)

	for _, m := range shared {
		if len(m.A.Args) != len(m.B.Args) {
			return &UnifyError{Kind: UnifyRowMismatch, Detail: fmt.Sprintf("constraint arg count mismatch: %s has %d args vs %d",
				m.A.ClassName, len(m.A.Args), len(m.B.Args))}
		}
		for i := range m.A.Args {
			if err := u.Unify(m.A.Args[i], m.B.Args[i]); err != nil {
				return err
			}
		}
	}

	onlyAEntries := &types.ConstraintEntries{Entries: onlyLeft}
	onlyBEntries := &types.ConstraintEntries{Entries: onlyRight}
	return u.resolveEvidenceTails(aN.Tail, bN.Tail, onlyAEntries, onlyBEntries)
}

// resolveEvidenceTails handles the 4-case tail resolution pattern shared by
// capability and constraint row unification. The fiber kind for fresh metas
// is derived from the entries' FiberKind().
func (u *Unifier) resolveEvidenceTails(aTail, bTail types.Type, onlyA, onlyB types.EvidenceEntries) error {
	switch {
	case aTail == nil && bTail == nil:
		if onlyA.EntryCount() > 0 || onlyB.EntryCount() > 0 {
			return &UnifyError{Kind: UnifyRowMismatch, Detail: fmt.Sprintf(
				"evidence row mismatch: extra entries (left=%d, right=%d)", onlyA.EntryCount(), onlyB.EntryCount())}
		}
	case aTail != nil && bTail == nil:
		if onlyA.EntryCount() > 0 {
			return &UnifyError{Kind: UnifyRowMismatch, Detail: fmt.Sprintf(
				"extra entries in evidence row: %d", onlyA.EntryCount())}
		}
		return u.solveEvidenceTail(aTail, onlyB, nil)
	case aTail == nil && bTail != nil:
		if onlyB.EntryCount() > 0 {
			return &UnifyError{Kind: UnifyRowMismatch, Detail: fmt.Sprintf(
				"extra entries in evidence row: %d", onlyB.EntryCount())}
		}
		return u.solveEvidenceTail(bTail, onlyA, nil)
	default:
		*u.freshID++
		rFresh := &types.TyMeta{ID: *u.freshID, Kind: onlyA.FiberKind()}
		if err := u.solveEvidenceTail(aTail, onlyB, rFresh); err != nil {
			return err
		}
		return u.solveEvidenceTail(bTail, onlyA, rFresh)
	}
	return nil
}

// solveEvidenceTail solves a row tail variable against a set of entries
// and an optional new tail. Works for both capability and constraint fibers.
func (u *Unifier) solveEvidenceTail(tail types.Type, entries types.EvidenceEntries, newTail types.Type) error {
	if entries.EntryCount() == 0 && newTail != nil {
		return u.Unify(tail, newTail)
	}
	var solution types.Type
	if entries.EntryCount() == 0 && newTail == nil {
		solution = &types.TyEvidenceRow{Entries: entries.Empty()}
	} else {
		solution = &types.TyEvidenceRow{Entries: entries, Tail: newTail}
	}
	return u.Unify(tail, solution)
}
