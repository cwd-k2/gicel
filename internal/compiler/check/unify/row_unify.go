package unify

import (
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Evidence row unification — dispatches to capability or constraint logic.
func (u *Unifier) unifyEvidenceRows(r1, r2 *types.TyEvidenceRow) error {
	switch a := r1.Entries.(type) {
	case *types.CapabilityEntries:
		b, ok := r2.Entries.(*types.CapabilityEntries)
		if !ok {
			return &UnifyError{Kind: UnifyMismatch, Name: "cannot unify capability row with constraint row"}
		}
		return u.unifyEvCapRows(a.Fields, r1.Tail, b.Fields, r2.Tail)
	case *types.ConstraintEntries:
		b, ok := r2.Entries.(*types.ConstraintEntries)
		if !ok {
			return &UnifyError{Kind: UnifyMismatch, Name: "cannot unify constraint row with capability row"}
		}
		return u.unifyEvConRows(a.Entries, r1.Tail, b.Entries, r2.Tail)
	default:
		return &UnifyError{Kind: UnifyMismatch, Name: "unknown evidence fiber"}
	}
}

func (u *Unifier) unifyEvCapRows(
	aFields []types.RowField, aTail types.Type,
	bFields []types.RowField, bTail types.Type,
) error {
	// Normalize field order.
	an, err := types.NormalizeRow(&types.TyEvidenceRow{Entries: &types.CapabilityEntries{Fields: aFields}, Tail: aTail})
	if err != nil {
		return &UnifyError{Kind: UnifyMismatch, Name: err.Error()}
	}
	bn, err := types.NormalizeRow(&types.TyEvidenceRow{Entries: &types.CapabilityEntries{Fields: bFields}, Tail: bTail})
	if err != nil {
		return &UnifyError{Kind: UnifyMismatch, Name: err.Error()}
	}
	aFieldsN := an.CapFields()
	bFieldsN := bn.CapFields()

	// Register label contexts for open-row tails.
	u.registerEvCapLabels(aFieldsN, an.Tail)
	u.registerEvCapLabels(bFieldsN, bn.Tail)

	shared, onlyLeft, onlyRight := types.ClassifyRowFields(aFieldsN, bFieldsN)

	// Build label→index maps for O(1) field access during shared-label unification.
	aIndex := make(map[string]int, len(aFieldsN))
	for i, f := range aFieldsN {
		aIndex[f.Label] = i
	}
	bIndex := make(map[string]int, len(bFieldsN))
	for i, f := range bFieldsN {
		bIndex[f.Label] = i
	}

	for _, label := range shared {
		aField := aFieldsN[aIndex[label]]
		bField := bFieldsN[bIndex[label]]
		if err := u.Unify(aField.Type, bField.Type); err != nil {
			return err
		}
		// Unify grade annotations pairwise — count must match.
		if len(aField.Grades) != len(bField.Grades) {
			return &UnifyError{Kind: UnifyMismatch, Name: "grade count mismatch", Label: label, CountA: len(aField.Grades), CountB: len(bField.Grades)}
		}
		for i := range aField.Grades {
			if err := u.Unify(aField.Grades[i], bField.Grades[i]); err != nil {
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

// registerFreshTailLabels registers the combined label context from both
// original tails on a fresh meta created during open/open tail resolution.
// Only applies to capability fibers (constraint rows don't use label tracking).
func (u *Unifier) registerFreshTailLabels(freshID int, aTail, bTail types.Type, onlyA, onlyB types.EvidenceEntries) {
	// Only capability rows use label-uniqueness tracking.
	capA, okA := onlyA.(*types.CapabilityEntries)
	capB, okB := onlyB.(*types.CapabilityEntries)
	if !okA || !okB {
		return
	}

	// Collect labels from both original tails' contexts (which include
	// shared labels + each side's own labels) and the onlyA/onlyB fields.
	combined := make(map[string]struct{})
	if za, ok := u.Zonk(aTail).(*types.TyMeta); ok {
		if existing, ok := u.labels[za.ID]; ok {
			for l := range existing {
				combined[l] = struct{}{}
			}
		}
	}
	if zb, ok := u.Zonk(bTail).(*types.TyMeta); ok {
		if existing, ok := u.labels[zb.ID]; ok {
			for l := range existing {
				combined[l] = struct{}{}
			}
		}
	}
	for _, f := range capA.Fields {
		combined[f.Label] = struct{}{}
	}
	for _, f := range capB.Fields {
		combined[f.Label] = struct{}{}
	}

	if len(combined) > 0 {
		u.trailLabelWrite(freshID)
		u.labels[freshID] = combined
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
			return &UnifyError{Kind: UnifyRowMismatch, Name: m.A.ClassName, CountA: len(m.A.Args), CountB: len(m.B.Args)}
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
			return &UnifyError{Kind: UnifyRowMismatch, CountA: onlyA.EntryCount(), CountB: onlyB.EntryCount()}
		}
	case aTail != nil && bTail == nil:
		if onlyA.EntryCount() > 0 {
			return &UnifyError{Kind: UnifyRowMismatch, CountA: onlyA.EntryCount()}
		}
		return u.solveEvidenceTail(aTail, onlyB, nil)
	case aTail == nil && bTail != nil:
		if onlyB.EntryCount() > 0 {
			return &UnifyError{Kind: UnifyRowMismatch, CountB: onlyB.EntryCount()}
		}
		return u.solveEvidenceTail(bTail, onlyA, nil)
	default:
		*u.freshID++
		rFresh := &types.TyMeta{ID: *u.freshID, Kind: onlyA.FiberKind()}
		// Register combined label context on the fresh tail meta so that
		// future solutions don't reintroduce labels from either side.
		// The original tails' label contexts contain shared + side-specific
		// labels; their union is the full label set rFresh must avoid.
		u.registerFreshTailLabels(rFresh.ID, aTail, bTail, onlyA, onlyB)
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
		e := entries.Empty()
		solution = &types.TyEvidenceRow{Entries: e, Flags: types.EvidenceRowFlags(e, nil)}
	} else {
		solution = &types.TyEvidenceRow{Entries: entries, Tail: newTail, Flags: types.EvidenceRowFlags(entries, newTail)}
	}
	return u.Unify(tail, solution)
}
