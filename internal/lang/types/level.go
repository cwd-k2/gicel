package types

import "fmt"

// LevelExpr represents a universe level expression.
// During Step 1-5 of the Type/Kind unification, only LevelLit is actively used.
// LevelVar, LevelMax, LevelSucc, and LevelMeta are activated in Step 6
// (universe polymorphism / Level Metavar).
type LevelExpr interface {
	levelExprNode()
	LevelString() string
}

// LevelLit is a concrete universe level: 0 (value types), 1 (kinds), 2 (sort), ...
type LevelLit struct{ N int }

// LevelVar is a universe level variable (for level polymorphism).
type LevelVar struct{ Name string }

// LevelMax is the maximum of two levels: max(A, B).
type LevelMax struct{ A, B LevelExpr }

// LevelSucc is the successor of a level: succ(E).
type LevelSucc struct{ E LevelExpr }

// LevelMeta is a universe level metavariable (for level inference).
type LevelMeta struct{ ID int }

func (*LevelLit) levelExprNode()  {}
func (*LevelVar) levelExprNode()  {}
func (*LevelMax) levelExprNode()  {}
func (*LevelSucc) levelExprNode() {}
func (*LevelMeta) levelExprNode() {}

func (l *LevelLit) LevelString() string { return fmt.Sprintf("%d", l.N) }
func (l *LevelVar) LevelString() string { return l.Name }
func (l *LevelMax) LevelString() string {
	return fmt.Sprintf("max(%s, %s)", l.A.LevelString(), l.B.LevelString())
}
func (l *LevelSucc) LevelString() string { return fmt.Sprintf("succ(%s)", l.E.LevelString()) }
func (l *LevelMeta) LevelString() string { return fmt.Sprintf("?l%d", l.ID) }

// Well-known level constants.
var (
	L0 = &LevelLit{N: 0} // value types: Int, Bool, List a, ...
	L1 = &LevelLit{N: 1} // kinds: Type, Row, Constraint, promoted data kinds
	L2 = &LevelLit{N: 2} // sort of kinds: Kind (= Sort₀)
)

// LevelEqual checks structural equality of two level expressions.
// nil is treated as L0 (value type level).
func LevelEqual(a, b LevelExpr) bool {
	a = normalizeLevel(a)
	b = normalizeLevel(b)
	switch aa := a.(type) {
	case *LevelLit:
		bb, ok := b.(*LevelLit)
		return ok && aa.N == bb.N
	case *LevelVar:
		bb, ok := b.(*LevelVar)
		return ok && aa.Name == bb.Name
	case *LevelMax:
		bb, ok := b.(*LevelMax)
		return ok && LevelEqual(aa.A, bb.A) && LevelEqual(aa.B, bb.B)
	case *LevelSucc:
		bb, ok := b.(*LevelSucc)
		return ok && LevelEqual(aa.E, bb.E)
	case *LevelMeta:
		bb, ok := b.(*LevelMeta)
		return ok && aa.ID == bb.ID
	default:
		return false
	}
}

// normalizeLevel replaces nil with L0.
func normalizeLevel(l LevelExpr) LevelExpr {
	if l == nil {
		return L0
	}
	return l
}

// IsValueLevel returns true if the level is 0 (value type level).
func IsValueLevel(l LevelExpr) bool {
	l = normalizeLevel(l)
	if lit, ok := l.(*LevelLit); ok {
		return lit.N == 0
	}
	return false
}

// IsKindLevel returns true if the level is 1 (kind level).
func IsKindLevel(l LevelExpr) bool {
	if lit, ok := l.(*LevelLit); ok {
		return lit.N == 1
	}
	return false
}
