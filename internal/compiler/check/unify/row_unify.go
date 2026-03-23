package unify

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

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

	shared, onlyLeft, onlyRight := types.ClassifyRowFields(aFieldsN, bFieldsN)

	for _, label := range shared {
		t1 := types.RowFieldType(aFieldsN, label)
		t2 := types.RowFieldType(bFieldsN, label)
		if err := u.Unify(t1, t2); err != nil {
			return err
		}
		// Unify grade annotations pairwise — count must match.
		g1 := types.RowFieldGrades(aFieldsN, label)
		g2 := types.RowFieldGrades(bFieldsN, label)
		if len(g1) != len(g2) {
			return &UnifyError{Kind: UnifyMismatch, Detail: fmt.Sprintf(
				"grade count mismatch for label %q: %d vs %d", label, len(g1), len(g2))}
		}
		for i := range g1 {
			if err := u.Unify(g1[i], g2[i]); err != nil {
				return err
			}
		}
	}

	onlyAEntries := &types.CapabilityEntries{Fields: types.CollectCapFields(aFieldsN, onlyLeft)}
	onlyBEntries := &types.CapabilityEntries{Fields: types.CollectCapFields(bFieldsN, onlyRight)}
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
		u.trailLabelWrite(m.ID)
		u.labels[m.ID] = labels
	}
}

func (u *Unifier) unifyEvConRows(
	aEntries []types.ConstraintEntry, aTail types.Type,
	bEntries []types.ConstraintEntry, bTail types.Type,
) error {
	aN := types.NormalizeConstraints(&types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: aEntries}, Tail: aTail})
	bN := types.NormalizeConstraints(&types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: bEntries}, Tail: bTail})

	shared, onlyLeft, onlyRight := types.ClassifyConstraints(aN.ConEntries(), bN.ConEntries())

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
				"row mismatch: extra entries (left=%d, right=%d)", onlyA.EntryCount(), onlyB.EntryCount())}
		}
	case aTail != nil && bTail == nil:
		if onlyA.EntryCount() > 0 {
			return &UnifyError{Kind: UnifyRowMismatch, Detail: fmt.Sprintf(
				"record has extra field(s): %d", onlyA.EntryCount())}
		}
		return u.solveEvidenceTail(aTail, onlyB, nil)
	case aTail == nil && bTail != nil:
		if onlyB.EntryCount() > 0 {
			return &UnifyError{Kind: UnifyRowMismatch, Detail: fmt.Sprintf(
				"record has extra field(s): %d", onlyB.EntryCount())}
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
