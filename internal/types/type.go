package types

import "github.com/cwd-k2/gicel/internal/span"

// Type is the unified representation for value types, computation types, and row types.
type Type interface {
	typeNode()
	Span() span.Span
	Children() []Type
}

// TyVar is a type or row variable.
type TyVar struct {
	Name string
	S    span.Span
}

// TyCon is a named type constructor.
type TyCon struct {
	Name string
	S    span.Span
}

// TyApp is a general type application (F T).
type TyApp struct {
	Fun Type
	Arg Type
	S   span.Span
}

// TyArrow is a function type (A -> B).
type TyArrow struct {
	From Type
	To   Type
	S    span.Span
}

// TyForall is a universal quantification (forall a:K. T).
type TyForall struct {
	Var  string
	Kind Kind
	Body Type
	S    span.Span
}

// TyComp is a Computation pre post a type.
type TyComp struct {
	Pre    Type
	Post   Type
	Result Type
	S      span.Span
}

// TyThunk is a Thunk pre post a type.
type TyThunk struct {
	Pre    Type
	Post   Type
	Result Type
	S      span.Span
}

// RowField is a single label:type pair in a row, with optional multiplicity.
// Mult is nil for unrestricted (the default); non-nil for graded capabilities.
type RowField struct {
	Label string
	Type  Type
	Mult  Type // nil = Unrestricted; e.g., TyCon("Linear"), TyCon("Affine")
	S     span.Span
}

// ConstraintEntry is a single class constraint in a constraint row.
// For simple constraints (Eq a), ClassName and Args describe the constraint.
// For quantified constraints (forall a. Eq a => Eq (f a)), Quantified is non-nil
// and ClassName/Args reflect the head constraint.
// For constraint variables (c : Constraint), ConstraintVar is non-nil and
// ClassName/Args are derived from it after substitution/zonking.
type ConstraintEntry struct {
	ClassName     string
	Args          []Type
	Quantified    *QuantifiedConstraint // non-nil for forall-quantified constraints
	ConstraintVar Type                  // non-nil for constraint variable references
	S             span.Span
}

// QuantifiedConstraint represents a universally quantified constraint:
//
//	forall vars. context => head
//
// Evidence for this constraint is a function from context dicts to a head dict.
type QuantifiedConstraint struct {
	Vars    []ForallBinder
	Context []ConstraintEntry // premise constraints
	Head    ConstraintEntry   // conclusion constraint
}

// ForallBinder is a universally quantified type variable with its kind.
type ForallBinder struct {
	Name string
	Kind Kind
}

// TyEvidence is a qualified type: { C1, C2 | c } => Body.
// Successor to TyQual; represents multiple constraints via an evidence row.
type TyEvidence struct {
	Constraints *TyEvidenceRow
	Body        Type
	S           span.Span
}

// TyMeta is a unification metavariable (created by the checker).
type TyMeta struct {
	ID   int
	Kind Kind
	S    span.Span
}

// TySkolem is a rigid (skolem) type variable for existentials and higher-rank.
// Unlike TyMeta, skolem variables cannot be solved by unification.
type TySkolem struct {
	ID   int
	Name string // original variable name (for error messages)
	Kind Kind
	S    span.Span
}

// TyError is a poison type for error recovery.
type TyError struct {
	S span.Span
}

// --- typeNode markers ---

func (*TyVar) typeNode()      {}
func (*TyCon) typeNode()      {}
func (*TyApp) typeNode()      {}
func (*TyArrow) typeNode()    {}
func (*TyForall) typeNode()   {}
func (*TyComp) typeNode()     {}
func (*TyThunk) typeNode()    {}
func (*TyEvidence) typeNode() {}
func (*TySkolem) typeNode()   {}
func (*TyMeta) typeNode()     {}
func (*TyError) typeNode()    {}

// --- Span accessors ---

func (t *TyVar) Span() span.Span      { return t.S }
func (t *TyCon) Span() span.Span      { return t.S }
func (t *TyApp) Span() span.Span      { return t.S }
func (t *TyArrow) Span() span.Span    { return t.S }
func (t *TyForall) Span() span.Span   { return t.S }
func (t *TyComp) Span() span.Span     { return t.S }
func (t *TyThunk) Span() span.Span    { return t.S }
func (t *TyEvidence) Span() span.Span { return t.S }
func (t *TySkolem) Span() span.Span   { return t.S }
func (t *TyMeta) Span() span.Span     { return t.S }
func (t *TyError) Span() span.Span    { return t.S }

// --- Children ---

func (t *TyVar) Children() []Type      { return nil }
func (t *TyCon) Children() []Type      { return nil }
func (t *TyApp) Children() []Type      { return []Type{t.Fun, t.Arg} }
func (t *TyArrow) Children() []Type    { return []Type{t.From, t.To} }
func (t *TyForall) Children() []Type   { return []Type{t.Body} }
func (t *TyComp) Children() []Type     { return []Type{t.Pre, t.Post, t.Result} }
func (t *TyThunk) Children() []Type    { return []Type{t.Pre, t.Post, t.Result} }
func (t *TyEvidence) Children() []Type { return []Type{t.Constraints, t.Body} }
func (t *TySkolem) Children() []Type   { return nil }
func (t *TyMeta) Children() []Type     { return nil }
func (t *TyError) Children() []Type    { return nil }

// UnwindApp decomposes a chain of TyApp into the head type and arguments.
// E.g., TyApp(TyApp(TyCon("F"), A), B) → (TyCon("F"), [A, B]).
func UnwindApp(ty Type) (Type, []Type) {
	var args []Type
	for {
		app, ok := ty.(*TyApp)
		if !ok {
			return ty, args
		}
		args = append([]Type{app.Arg}, args...)
		ty = app.Fun
	}
}
