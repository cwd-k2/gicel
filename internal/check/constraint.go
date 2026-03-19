package check

import (
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// Ct is a constraint waiting to be solved by the constraint solver.
// The solver processes constraints from a worklist, discharging them
// into the inert set or producing Core evidence terms.
type Ct interface {
	ctMarker()
	ctPlaceholder() string
	ctSpan() span.Span
}

// CtClass represents a type class constraint: className args.
//
// Maps to the three branches of the former deferredConstraint:
//   - quantified != nil:    quantified constraint  (forall a. C a => D (F a))
//   - constraintVar != nil: constraint variable     (Dict reification)
//   - otherwise:            plain className + args  (Num Int, Eq a)
type CtClass struct {
	Placeholder   string
	ClassName     string
	Args          []types.Type
	S             span.Span
	Quantified    *types.QuantifiedConstraint
	ConstraintVar types.Type
}

func (*CtClass) ctMarker()               {}
func (c *CtClass) ctPlaceholder() string { return c.Placeholder }
func (c *CtClass) ctSpan() span.Span     { return c.S }

// CtFunEq represents a stuck type family equation: F args ~ resultMeta.
// When blocking metavariables are solved, the equation is kicked out
// of the inert set back to the worklist for re-processing.
type CtFunEq struct {
	FamilyName string
	Args       []types.Type
	ResultMeta *types.TyMeta
	BlockingOn []int
	S          span.Span
}

func (*CtFunEq) ctMarker()               {}
func (c *CtFunEq) ctPlaceholder() string { return "" }
func (c *CtFunEq) ctSpan() span.Span     { return c.S }

// collectMetaIDs collects all TyMeta IDs from a slice of types.
// Used by the inert set to build the meta-to-constraint index.
func collectMetaIDs(tys []types.Type) []int {
	seen := make(map[int]bool)
	var ids []int
	for _, t := range tys {
		types.AnyType(t, func(ty types.Type) bool {
			if m, ok := ty.(*types.TyMeta); ok && !seen[m.ID] {
				seen[m.ID] = true
				ids = append(ids, m.ID)
			}
			return false
		})
	}
	return ids
}
