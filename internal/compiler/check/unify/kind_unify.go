package unify

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// UnifyKinds solves the constraint k1 ~ k2 at the kind level.
func (u *Unifier) UnifyKinds(k1, k2 types.Kind) error {
	k1 = u.ZonkKind(k1)
	k2 = u.ZonkKind(k2)

	// Meta solving.
	if m, ok := k1.(*types.KMeta); ok {
		return u.solveKindMeta(m, k2)
	}
	if m, ok := k2.(*types.KMeta); ok {
		return u.solveKindMeta(m, k1)
	}

	switch a := k1.(type) {
	case types.KType:
		if _, ok := k2.(types.KType); ok {
			return nil
		}
	case types.KRow:
		if _, ok := k2.(types.KRow); ok {
			return nil
		}
	case types.KConstraint:
		if _, ok := k2.(types.KConstraint); ok {
			return nil
		}
	case types.KData:
		if b, ok := k2.(types.KData); ok && a.Name == b.Name {
			return nil
		}
	case types.KVar:
		if b, ok := k2.(types.KVar); ok && a.Name == b.Name {
			return nil
		}
	case types.KSort:
		if b, ok := k2.(types.KSort); ok && a.Level == b.Level {
			return nil
		}
	case *types.KArrow:
		if b, ok := k2.(*types.KArrow); ok {
			if err := u.UnifyKinds(a.From, b.From); err != nil {
				return err
			}
			return u.UnifyKinds(a.To, b.To)
		}
	}

	// Cumulativity: ground kinds (Type, Row, Constraint, KData) are
	// sub-kinds of Sort₀ (= Kind). This allows kind-polymorphic binders
	// like \(k: Kind). ... to accept Type, Row, etc. as arguments.
	if isGroundKind(k1) && isSortLevel(k2, 0) {
		return nil
	}
	if isSortLevel(k1, 0) && isGroundKind(k2) {
		return nil
	}

	return &UnifyError{Kind: UnifyMismatch, Detail: fmt.Sprintf("kind mismatch: %s vs %s", k1, k2)}
}

// isGroundKind returns true if k is a concrete kind that inhabits Sort₀.
func isGroundKind(k types.Kind) bool {
	switch k.(type) {
	case types.KType, types.KRow, types.KConstraint, types.KData:
		return true
	default:
		return false
	}
}

// isSortLevel returns true if k is KSort at the given level.
func isSortLevel(k types.Kind, level int) bool {
	if s, ok := k.(types.KSort); ok {
		return s.Level == level
	}
	return false
}

func (u *Unifier) solveKindMeta(m *types.KMeta, k types.Kind) error {
	if m2, ok := k.(*types.KMeta); ok && m2.ID == m.ID {
		return nil
	}
	if u.kindOccursIn(m.ID, k) {
		return &UnifyError{Kind: UnifyOccursCheck, Detail: fmt.Sprintf("infinite kind: ?k%d occurs in %s", m.ID, k)}
	}
	u.trailKindWrite(m.ID)
	u.kindSoln[m.ID] = k
	return nil
}

func (u *Unifier) kindOccursIn(id int, k types.Kind) bool {
	k = u.ZonkKind(k)
	switch kk := k.(type) {
	case *types.KMeta:
		return kk.ID == id
	case *types.KArrow:
		return u.kindOccursIn(id, kk.From) || u.kindOccursIn(id, kk.To)
	default:
		return false
	}
}

// ZonkKind replaces all solved kind metavariables.
func (u *Unifier) ZonkKind(k types.Kind) types.Kind {
	switch kk := k.(type) {
	case *types.KMeta:
		soln, ok := u.kindSoln[kk.ID]
		if !ok {
			return kk
		}
		result := u.ZonkKind(soln)
		if result != soln {
			u.trailKindWrite(kk.ID)
			u.kindSoln[kk.ID] = result // path compression
		}
		return result
	case *types.KArrow:
		zFrom := u.ZonkKind(kk.From)
		zTo := u.ZonkKind(kk.To)
		if zFrom == kk.From && zTo == kk.To {
			return kk
		}
		return &types.KArrow{From: zFrom, To: zTo}
	default:
		return k
	}
}
