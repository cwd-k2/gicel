// Package types defines the type representation used by the checker,
// the optimizer, and the evaluator's runtime type errors. Types are
// immutable trees; transformations construct new trees and share
// unchanged subtrees where possible.
//
// # Universe stratification
//
// TyCon carries a Level (LevelExpr) placing it in a universe:
//
//	nil or L0  value types (Int, Bool, List, ...)
//	L1         kinds (Type, Row, Constraint, promoted data kinds)
//	L2         sort of kinds (Kind = Sort₀)
//
// L1 kinds may be promoted from L0 data declarations. IsLabel marks
// label literals at L1 (e.g., #foo), which are structurally distinct
// from ordinary TyCons.
//
// # Flags
//
// TyApp, TyArrow, TyForall, TyCBPV, and TyEvidence carry a uint8
// Flags field encoding two fast-path predicates:
//
//	FlagMetaFree     subtree contains no TyMeta or TySkolem
//	FlagNoFamilyApp  subtree contains no TyFamilyApp
//
// Zero is the conservative default: absence of the flag does not
// imply the subtree contains metas, only that the constructor did
// not prove otherwise. Any construction that transforms a subtree
// must recompute flags from the new children via MetaFreeFlags; the
// subst.go and zonk paths do this automatically. Only code that
// rebuilds flag-bearing nodes manually needs to be flag-aware.
//
// # Evidence rows
//
// TyEvidenceRow is the unified representation for capability rows
// (effect tracking) and constraint rows (type-class evidence). A row
// holds an EvidenceEntries value which is exactly one of:
//
//	*CapabilityEntries  a sorted list of RowField (label: type @grades)
//	*ConstraintEntries  a list of ConstraintEntry (class constraints,
//	                    quantified constraints, constraint variables,
//	                    or equality constraints)
//
// A single TyEvidenceRow never mixes fibers. IsCapabilityRow /
// IsConstraintRow select between them; CapFields / ConEntries extract
// the underlying slice after the predicate confirms the fiber.
//
// Capability row fields are maintained in lexicographic label order
// (NormalizeRow enforces this on every builder-produced row).
// Duplicate labels are rejected at construction time.
//
// Constraint entries are kept in insertion order; ConstraintKey
// provides the canonical serialization used for stable display and
// set membership. Quantified constraints (forall a. Ctx => Head)
// are represented by a non-nil ConstraintEntry.Quantified; the outer
// ClassName/Args describe the head constraint for fast matching.
//
// # Meta variables
//
// TyMeta is a unification variable created by the checker. Level
// tracks implication nesting depth at creation time for touchability
// (OutsideIn(X)). Metas are single-assignment: once unified to a
// type, they must not be unified again. Zonk threads assignments
// through a type tree, producing a Meta-free copy when FlagMetaFree
// does not already hold.
//
// TySkolem is a rigid type variable introduced by forall instantiation
// at negative positions. Skolems cannot be solved by unification and
// escape-check failures are reported at solve time.
package types
