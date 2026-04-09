package solve

import (
	"sync"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// metaSetPool reuses map[int]bool instances for collectMetaIDs.
var metaSetPool = sync.Pool{
	New: func() any { return make(map[int]bool, 8) },
}

// CtFlavor distinguishes given equalities (from GADT refinement) from
// wanted equalities (from type checking obligations). Given equalities
// are processed before wanteds and can trigger kick-out of stuck
// constraints or detect contradictory (inaccessible) branches.
type CtFlavor int

const (
	// CtWanted is the default flavor: an equality the solver must discharge.
	CtWanted CtFlavor = iota
	// CtGiven is a locally-known equality from a GADT pattern refinement.
	CtGiven
)

// Ct is a constraint waiting to be solved by the constraint solver.
// The solver processes constraints from a worklist, discharging them
// into the inert set or producing Core evidence terms.
type Ct interface {
	ctMarker()
	ctSpan() span.Span
}

// CtClass is the sealed interface implemented by every variant of class
// constraint the solver puts on its worklist. Three variants:
//
//	*CtPlainClass       — resolved head: ClassName + Args (Num Int, Eq a).
//	                      This is the only form that ever enters the inert
//	                      set or appears in residuals.
//	*CtVarClass         — constraint variable (Dict reification). Transient:
//	                      processCtVarClass normalizes it into a CtPlainClass
//	                      or reports ErrNoInstance. Never stored in the
//	                      inert set.
//	*CtQuantifiedClass  — quantified constraint (forall a. C a => D (F a)).
//	                      Transient: processCtQuantifiedClass discharges it
//	                      immediately via resolveQuantifiedConstraint and
//	                      never inserts it into the inert set.
//
// Maps directly onto the three branches of the former packed CtClass
// struct (Quantified != nil / ConstraintVar != nil / plain). The sealed
// interface + variant split replaces nil-check discrimination with a
// type-switch, and lets the inert set and residual slices express
// "plain class only" at the type level.
type CtClass interface {
	Ct
	ctClassMarker()
}

// CtPlainClass is a plain class constraint with a resolved head:
// className + zonked args. This is the canonical form the solver works
// with — after processCtVarClass normalization, this is the only variant
// that enters the inert set, the resolution cache, or residuals.
type CtPlainClass struct {
	Placeholder string
	ClassName   string
	Args        []types.Type
	S           span.Span
}

func (*CtPlainClass) ctMarker()           {}
func (c *CtPlainClass) ctSpan() span.Span { return c.S }
func (*CtPlainClass) ctClassMarker()      {}

// CtVarClass is an unresolved constraint variable: the checker has a
// term of kind Constraint whose head class is not yet known. Transient:
// processCtVarClass zonks the variable, decomposes it into (className,
// args), and delegates to processCtPlainClass. Never stored in the
// inert set — there is no observable moment when a CtVarClass coexists
// with a resolved head.
type CtVarClass struct {
	Placeholder   string
	ConstraintVar types.Type
	S             span.Span
}

func (*CtVarClass) ctMarker()           {}
func (c *CtVarClass) ctSpan() span.Span { return c.S }
func (*CtVarClass) ctClassMarker()      {}

// CtQuantifiedClass is a universally quantified constraint like
// `forall a. Eq a => Eq (List a)`. Discharged immediately by
// processCtQuantifiedClass via resolveQuantifiedConstraint; never
// enters the inert set and never becomes a residual.
type CtQuantifiedClass struct {
	Placeholder string
	Quantified  *types.QuantifiedConstraint
	S           span.Span
}

func (*CtQuantifiedClass) ctMarker()           {}
func (c *CtQuantifiedClass) ctSpan() span.Span { return c.S }
func (*CtQuantifiedClass) ctClassMarker()      {}

// Compile-time interface conformance checks. If a variant accidentally
// drops a required method, this fails to compile rather than at runtime.
var (
	_ CtClass = (*CtPlainClass)(nil)
	_ CtClass = (*CtVarClass)(nil)
	_ CtClass = (*CtQuantifiedClass)(nil)
)

// CtClassHeadName returns the class name of a class-headed constraint
// variant (CtPlainClass, or CtQuantifiedClass with a non-nil Head), or
// "" for CtVarClass (the head is not yet resolved). Mirrors the shape
// of types.HeadClassName for ConstraintEntry.
func CtClassHeadName(c CtClass) string {
	switch c := c.(type) {
	case *CtPlainClass:
		return c.ClassName
	case *CtQuantifiedClass:
		if c.Quantified != nil && c.Quantified.Head != nil {
			return c.Quantified.Head.ClassName
		}
	}
	return ""
}

// CtFunEq represents a stuck type family equation: F args ~ resultMeta.
// When blocking metavariables are solved, the equation is kicked out
// of the inert set back to the worklist for re-processing.
//
// OnFailure is an optional callback invoked when the reduced result
// cannot unify with ResultMeta. Used by grade constraints to emit
// domain-specific errors (e.g. ErrMultiplicity). Nil for non-grade families.
type CtFunEq struct {
	FamilyName string
	Args       []types.Type
	ResultMeta *types.TyMeta
	BlockingOn []int
	OnFailure  func(span.Span, types.Type, types.Type) // (span, expected, actual); nil = silent
	S          span.Span
}

func (*CtFunEq) ctMarker()           {}
func (c *CtFunEq) ctSpan() span.Span { return c.S }

// CtOrigin records the generation site context of a constraint.
// When the solver reports errors, it uses Origin to produce semantic
// error messages (matching the quality of inline error reporting).
//
// Context is computed lazily: if LazyCtx is non-nil, it is called on
// first access and the result is cached. This avoids types.Pretty
// traversals at constraint-generation time when the constraint succeeds.
type CtOrigin struct {
	Code    diagnostic.Code // semantic error code (0 = use default ErrTypeMismatch)
	context string          // human-readable context; use GetContext()
	LazyCtx func() string   // deferred context builder (nil = use context directly)
}

// WithContext creates a CtOrigin with a static context string.
func WithContext(code diagnostic.Code, ctx string) *CtOrigin {
	return &CtOrigin{Code: code, context: ctx}
}

// WithLazyContext creates a CtOrigin with a deferred context builder.
func WithLazyContext(code diagnostic.Code, f func() string) *CtOrigin {
	return &CtOrigin{Code: code, LazyCtx: f}
}

// GetContext returns the human-readable context string, invoking
// LazyCtx on first call if present.
func (o *CtOrigin) GetContext() string {
	if o.LazyCtx != nil {
		o.context = o.LazyCtx()
		o.LazyCtx = nil
	}
	return o.context
}

// CtEq represents a type equality constraint: Lhs ~ Rhs.
// Emitted from user-written (a ~ Int) => constraints, checker-generated
// type equalities, and solver-managed GADT given equalities.
// Flavor distinguishes wanted (default) from given equalities.
type CtEq struct {
	Lhs    types.Type
	Rhs    types.Type
	Flavor CtFlavor  // CtWanted (default zero) or CtGiven
	Origin *CtOrigin // nil = generic error message
	S      span.Span
}

func (*CtEq) ctMarker()           {}
func (c *CtEq) ctSpan() span.Span { return c.S }

// CtImplication represents an implication constraint for GADT branches
// and other scoped constraint-solving contexts. It bundles:
//   - Skolems: GADT existential variables introduced by the pattern
//   - GivenEqs: local equalities (skolemID → refinement type)
//   - Wanteds: constraints to solve at the inner implication level
type CtImplication struct {
	Skolems  []*types.TySkolem // GADT existential vars
	GivenEqs map[int]types.Type
	Wanteds  []Ct
	S        span.Span
}

func (*CtImplication) ctMarker()           {}
func (c *CtImplication) ctSpan() span.Span { return c.S }

// collectMetaIDs collects all TyMeta IDs from a slice of types.
// Used by the inert set to build the meta-to-constraint index.
func collectMetaIDs(tys []types.Type) []int {
	seen := metaSetPool.Get().(map[int]bool)
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
	clear(seen)
	metaSetPool.Put(seen)
	return ids
}

// typesMentionAnySkolem reports whether any type in the slice contains
// any TySkolem. Used for cascade suppression: "no instance" errors on
// skolem-containing types are suppressed when a structural eq error
// has already been reported.
func typesMentionAnySkolem(tys []types.Type) bool {
	for _, t := range tys {
		if types.AnyType(t, func(ty types.Type) bool {
			_, ok := ty.(*types.TySkolem)
			return ok
		}) {
			return true
		}
	}
	return false
}

// typeMentionsSkolem reports whether a type tree contains a TySkolem
// with the given ID. Used by given-equality kick-out to detect which
// inert constraints are affected by a newly installed skolem solution.
func typeMentionsSkolem(t types.Type, skolemID int) bool {
	return types.AnyType(t, func(ty types.Type) bool {
		if sk, ok := ty.(*types.TySkolem); ok {
			return sk.ID == skolemID
		}
		return false
	})
}

// typesMentionSkolem reports whether any type in the slice contains
// a TySkolem with the given ID.
func typesMentionSkolem(tys []types.Type, skolemID int) bool {
	for _, t := range tys {
		if typeMentionsSkolem(t, skolemID) {
			return true
		}
	}
	return false
}
