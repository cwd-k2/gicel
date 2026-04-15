package types

import (
	"slices"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

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
// Level indicates the universe level:
//   - nil or L0: value types (Int, Bool, List, ...)
//   - L1: kinds (Type, Row, Constraint, promoted data kinds)
//   - L2: sort of kinds (Kind = Sort₀)
//
// IsLabel marks label literals at L1 (e.g., #foo). These are structurally
// distinct from promoted data constructors and grade constants.
type TyCon struct {
	Name    string
	Level   LevelExpr // nil = L0 (value type)
	IsLabel bool      // true for label literals at L1
	S       span.Span
}

// TyApp is a general type application (F T).
type TyApp struct {
	Fun     Type
	Arg     Type
	IsGrade bool  // true when Arg is a grade annotation via @
	Flags   uint8 // see FlagMetaFree
	S       span.Span
}

// TyArrow is a function type (A -> B).
type TyArrow struct {
	From  Type
	To    Type
	Flags uint8
	S     span.Span
}

// TyForall is a universal quantification (\ a:K. T).
// Kind holds the kind of the bound variable as a Type at universe level >= 1.
type TyForall struct {
	Var   string
	Kind  Type
	Body  Type
	Flags uint8
	S     span.Span
}

// CBPVTag distinguishes Computation and Thunk types.
type CBPVTag int

const (
	TagComp  CBPVTag = iota // Computation pre post a
	TagThunk                // Thunk pre post a
)

// Canonical type constructor names.
const (
	TyConComputation = "Computation"
	TyConThunk       = "Thunk"
	TyConRecord      = "Record"
	TyConVariant     = "Variant"
)

// TyCBPV is a CBPV computation or thunk type. It has two surface forms:
//
//   - Ungraded (3-arg):  Computation pre post a   / Thunk pre post a
//   - Graded   (4-arg):  Computation @g pre post a / Thunk @g pre post a
//
// Grade is the optional per-computation usage grade. A nil Grade means the
// type was written in ungraded form — the user did not commit to a specific
// grade algebra. See the package comment under "CBPV grade duality" for the
// semantic relationship between the two forms; in short, ungraded types are
// treated by the unifier as compatible with any graded form (the grade check
// is skipped when either side is nil), while structural Equal and TypeKey
// keep them distinct.
type TyCBPV struct {
	Tag               CBPVTag
	Pre, Post, Result Type
	Grade             Type // nil = ungraded surface form (3-arg)
	Flags             uint8
	S                 span.Span
}

// IsGraded reports whether this CBPV type carries a grade annotation (4-arg form).
// When false, the type is in the ungraded surface form (3-arg).
func (t *TyCBPV) IsGraded() bool { return t.Grade != nil }

// CBPVAdjunctionParts checks whether two TyCBPV types are adjunction-compatible:
// opposite tags with structurally unifiable components. Returns the component
// pairs that need unification and true, or nil and false if the types are not
// adjunction-compatible. Grade duality: nil grade is compatible with any grade.
//
// This is the shared predicate used by tryCBPVCoercion (subsCheck),
// tryCBPVComponentUnify (solver), and autoForceIfThunk (inferBind).
// The caller is responsible for unifying the returned pairs and wrapping
// the expression in Thunk/Force as needed.
func CBPVAdjunctionParts(a, b *TyCBPV) (pairs [][2]Type, ok bool) {
	if a.Tag == b.Tag {
		return nil, false
	}
	pairs = [][2]Type{
		{a.Pre, b.Pre},
		{a.Post, b.Post},
		{a.Result, b.Result},
	}
	if a.IsGraded() && b.IsGraded() {
		pairs = append(pairs, [2]Type{a.Grade, b.Grade})
	}
	return pairs, true
}

// RowField is a single label:type pair in a row, with optional grade annotations.
// Grades is nil/empty for unrestricted (the default); each element is a grade
// from a potentially different grade algebra (e.g., TyCon("Linear"), TyCon("Secret")).
type RowField struct {
	Label      string
	Type       Type
	Grades     []Type // nil = no grade constraints (unrestricted default)
	IsLabelVar bool   // true when Label originates from a label-kinded forall variable
	S          span.Span
}

// IsGraded reports whether this row field carries grade annotations.
func (f RowField) IsGraded() bool { return len(f.Grades) > 0 }

// ConstraintEntry and QuantifiedConstraint are defined in constraint_entry.go
// as a sealed interface with four concrete variants (ClassEntry, EqualityEntry,
// VarEntry, *QuantifiedConstraint). Field validity is type-enforced; see that
// file for the full variant design.

// ForallBinder is a universally quantified type variable with its kind.
type ForallBinder struct {
	Name string
	Kind Type
}

// TyEvidence is a qualified type: { C1, C2 | c } => Body.
// Successor to TyQual; represents multiple constraints via an evidence row.
type TyEvidence struct {
	Constraints *TyEvidenceRow
	Body        Type
	Flags       uint8
	S           span.Span
}

// TyMeta is a unification metavariable (created by the checker).
//
// Level tracks the implication nesting depth at creation time. Used for
// touchability: a meta at level k is touchable only when the solver is
// operating at level ≥ k (OutsideIn). Metas created at the top level
// have Level 0; those created inside implication scopes (GADT branches)
// have the solver's current level at creation time.
//
// Generalization uses a separate mechanism (ID-based watermarks in
// decl_generalize.go) to determine which metas to quantify. The two
// systems are complementary: Level controls unification permission,
// watermarks control generalization scope.
type TyMeta struct {
	ID    int
	Kind  Type
	Level int // implication nesting depth (0 = top-level)
	S     span.Span
}

// TySkolem is a rigid (skolem) type variable for existentials and higher-rank.
// Unlike TyMeta, skolem variables cannot be solved by unification.
type TySkolem struct {
	ID   int
	Name string // original variable name (for error messages)
	Kind Type
	S    span.Span
}

// TyError is a poison type for error recovery.
type TyError struct {
	S span.Span
}

// FlagMetaFree indicates that a type's subtree is known to contain no TyMeta
// or TySkolem. The zero value (no flag set) is conservative: the subtree may
// or may not contain zonk-relevant nodes. This ensures safety when construction
// sites do not set flags.
const FlagMetaFree uint8 = 1 << 0

// FlagNoFamilyApp indicates that a type's subtree contains no TyFamilyApp nodes.
// Combined with FlagMetaFree, this allows type family reduction to skip the
// entire subtree in O(1). The combination is required because a meta could
// resolve to a family application.
const FlagNoFamilyApp uint8 = 1 << 1

// FlagStable combines FlagMetaFree and FlagNoFamilyApp. A type with both flags
// set requires neither zonking nor family reduction traversal.
const FlagStable = FlagMetaFree | FlagNoFamilyApp

// HasFamilyApp reports whether a type may contain TyFamilyApp nodes that
// require family reduction traversal. Returns false only when both
// FlagMetaFree and FlagNoFamilyApp are set (no metas that could resolve
// to family apps, and no explicit family apps).
func HasFamilyApp(t Type) bool {
	return nodeFlags(t)&FlagStable != FlagStable
}

// HasMeta reports whether a type may require zonk traversal — i.e., it may
// contain TyMeta (unification metavariables) or TySkolem (rigid variables that
// may be resolved via skolemSoln in GADT refinement).
// Returns false only when the type is a known-leaf without metas/skolems or
// when FlagMetaFree has been explicitly set on a composite type.
// This is conservative: false means "definitely needs no zonking".
func HasMeta(t Type) bool {
	switch ty := t.(type) {
	case *TyMeta:
		return true
	case *TySkolem:
		return true // skolems may be resolved via skolemSoln in GADT refinement
	case *TyVar, *TyError:
		return false
	case *TyCon:
		return levelHasMeta(ty.Level)
	case *TyApp:
		return ty.Flags&FlagMetaFree == 0
	case *TyArrow:
		return ty.Flags&FlagMetaFree == 0
	case *TyForall:
		return ty.Flags&FlagMetaFree == 0
	case *TyCBPV:
		return ty.Flags&FlagMetaFree == 0
	case *TyEvidence:
		return ty.Flags&FlagMetaFree == 0
	case *TyEvidenceRow:
		return ty.Flags&FlagMetaFree == 0
	case *TyFamilyApp:
		return ty.Flags&FlagMetaFree == 0
	default:
		// Conservative: unknown type assumed to contain metas.
		// This is intentionally not a panic — HasMeta is a fast-path
		// optimization hint, not a correctness invariant.
		return true
	}
}

// MetaFreeFlags computes FlagMetaFree and FlagNoFamilyApp for a set of children.
// Returns the intersection of child flags: a flag is set only if ALL children have it.
func MetaFreeFlags(ts ...Type) uint8 {
	flags := FlagStable
	for _, t := range ts {
		if t == nil {
			continue // nil children (e.g. optional Grade) don't affect flags
		}
		flags &= nodeFlags(t)
		if flags == 0 {
			return 0
		}
	}
	return flags
}

// nodeFlags returns the stable flags for a single type node.
func nodeFlags(t Type) uint8 {
	switch ty := t.(type) {
	case *TyMeta, *TySkolem:
		return 0
	case *TyVar, *TyError:
		return FlagStable
	case *TyCon:
		if levelHasMeta(ty.Level) {
			return 0
		}
		return FlagStable
	case *TyApp:
		return ty.Flags
	case *TyArrow:
		return ty.Flags
	case *TyForall:
		return ty.Flags
	case *TyCBPV:
		return ty.Flags
	case *TyEvidence:
		return ty.Flags
	case *TyEvidenceRow:
		return ty.Flags
	case *TyFamilyApp:
		// FlagNoFamilyApp is never set on TyFamilyApp itself.
		return ty.Flags &^ FlagNoFamilyApp
	default:
		// Conservative: unknown type assumed to have no stable flags.
		// This forces traversal on unknown nodes, which is safe.
		return 0
	}
}

// levelHasMeta reports whether a LevelExpr contains a LevelMeta.
func levelHasMeta(l LevelExpr) bool {
	switch lv := l.(type) {
	case *LevelMeta:
		return true
	case *LevelMax:
		return levelHasMeta(lv.A) || levelHasMeta(lv.B)
	case *LevelSucc:
		return levelHasMeta(lv.E)
	default:
		return false // LevelLit, LevelVar, nil
	}
}

// metaFreeSlice computes flags for a type with an extra child and a slice of children.
// Unlike MetaFreeFlags, this avoids variadic/append allocation for slice-based types.
func metaFreeSlice(extra Type, ts []Type) uint8 {
	flags := nodeFlags(extra)
	if flags == 0 {
		return 0
	}
	for _, t := range ts {
		flags &= nodeFlags(t)
		if flags == 0 {
			return 0
		}
	}
	return flags
}

// --- typeNode markers ---

func (*TyVar) typeNode()      {}
func (*TyCon) typeNode()      {}
func (*TyApp) typeNode()      {}
func (*TyArrow) typeNode()    {}
func (*TyForall) typeNode()   {}
func (*TyCBPV) typeNode()     {}
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
func (t *TyCBPV) Span() span.Span     { return t.S }
func (t *TyEvidence) Span() span.Span { return t.S }
func (t *TySkolem) Span() span.Span   { return t.S }
func (t *TyMeta) Span() span.Span     { return t.S }
func (t *TyError) Span() span.Span    { return t.S }

// --- Children ---

func (t *TyVar) Children() []Type    { return nil }
func (t *TyCon) Children() []Type    { return nil }
func (t *TyApp) Children() []Type    { return []Type{t.Fun, t.Arg} }
func (t *TyArrow) Children() []Type  { return []Type{t.From, t.To} }
func (t *TyForall) Children() []Type { return []Type{t.Kind, t.Body} }
func (t *TyCBPV) Children() []Type {
	if t.IsGraded() {
		return []Type{t.Pre, t.Post, t.Result, t.Grade}
	}
	return []Type{t.Pre, t.Post, t.Result}
}
func (t *TyEvidence) Children() []Type { return []Type{t.Constraints, t.Body} }
func (t *TySkolem) Children() []Type   { return nil }
func (t *TyMeta) Children() []Type     { return nil }
func (t *TyError) Children() []Type    { return nil }

// ContainsMetaOrSkolem returns true if the type contains any TyMeta or TySkolem.
// A type that returns false is "ground" — Zonk cannot reveal hidden skolems.
//
// Composite types whose FlagMetaFree is set are answered in O(1) via the
// HasMeta fast path. The recursive walk is only entered for types that
// HasMeta cannot rule out — i.e., subtrees that may contain a meta or
// skolem at some depth.
func ContainsMetaOrSkolem(t Type) bool {
	if !HasMeta(t) {
		return false
	}
	switch t.(type) {
	case *TyMeta, *TySkolem:
		return true
	}
	found := false
	ForEachChild(t, func(child Type) bool {
		if ContainsMetaOrSkolem(child) {
			found = true
			return false
		}
		return true
	})
	return found
}

// ForEachChild calls fn for each direct child of t. If fn returns false,
// iteration stops early. Leaf nodes (TyVar, TyCon, TyMeta, TySkolem, TyError)
// have no children. This avoids the slice allocation of Children().
func ForEachChild(t Type, fn func(Type) bool) {
	switch ty := t.(type) {
	case *TyApp:
		if fn(ty.Fun) {
			fn(ty.Arg)
		}
	case *TyArrow:
		if fn(ty.From) {
			fn(ty.To)
		}
	case *TyForall:
		if fn(ty.Kind) {
			fn(ty.Body)
		}
	case *TyCBPV:
		if fn(ty.Pre) && fn(ty.Post) && fn(ty.Result) && ty.IsGraded() {
			fn(ty.Grade)
		}
	case *TyEvidence:
		if fn(ty.Constraints) {
			fn(ty.Body)
		}
	case *TyEvidenceRow:
		switch e := ty.Entries.(type) {
		case *CapabilityEntries:
			for _, f := range e.Fields {
				if !fn(f.Type) {
					return
				}
				for _, g := range f.Grades {
					if !fn(g) {
						return
					}
				}
			}
		case *ConstraintEntries:
			stopped := false
			e.ForEachChild(func(child Type) bool {
				if !fn(child) {
					stopped = true
					return false
				}
				return true
			})
			if stopped {
				return
			}
		}
		if ty.IsOpen() {
			fn(ty.Tail)
		}
	case *TyFamilyApp:
		for _, a := range ty.Args {
			if !fn(a) {
				return
			}
		}
		if ty.Kind != nil {
			fn(ty.Kind)
		}
	case *TyVar, *TyCon, *TyMeta, *TySkolem, *TyError:
		// Leaves — no children.
	default:
		panic(unhandledTypeMsg("ForEachChild", t))
	}
}

// TypeSize returns the number of nodes in a type, up to a limit.
// If the type has more than limit nodes, it returns limit+1 and stops early.
// This is used to bound allocation during type family reduction.
func TypeSize(t Type, limit int) int {
	return typeSizeRec(t, limit, 0)
}

func typeSizeRec(t Type, limit, acc int) int {
	if acc > limit {
		return acc
	}
	acc++
	ForEachChild(t, func(child Type) bool {
		acc = typeSizeRec(child, limit, acc)
		return acc <= limit
	})
	return acc
}

// AppSpineHead returns the head of a TyApp chain and the spine depth (number
// of applied arguments), without allocating a slice.
func AppSpineHead(ty Type) (head Type, depth int) {
	for {
		app, ok := ty.(*TyApp)
		if !ok {
			return ty, depth
		}
		depth++
		ty = app.Fun
	}
}

// UnwindApp decomposes a chain of TyApp into the head type and arguments.
// E.g., TyApp(TyApp(TyCon("F"), A), B) → (TyCon("F"), [A, B]).
func UnwindApp(ty Type) (Type, []Type) {
	var args []Type
	for {
		app, ok := ty.(*TyApp)
		if !ok {
			slices.Reverse(args)
			return ty, args
		}
		args = append(args, app.Arg)
		ty = app.Fun
	}
}

// unhandledTypeMsg constructs the panic message for sealed-sum exhaustiveness
// guards. These are unreachable in production — they fire only when a GICEL
// developer adds a new variant without updating the caller. Callers use
// `panic(unhandledTypeMsg("op", t))` so the compiler sees the panic directly.
func unhandledTypeMsg(op string, t Type) string {
	return op + ": unhandled Type " + typeName(t)
}

// typeName returns the Go type name of a Type value without importing
// fmt or reflect. The sealed sum is small enough to enumerate.
func typeName(t Type) string {
	switch t.(type) {
	case *TyVar:
		return "*TyVar"
	case *TyCon:
		return "*TyCon"
	case *TyApp:
		return "*TyApp"
	case *TyArrow:
		return "*TyArrow"
	case *TyForall:
		return "*TyForall"
	case *TyCBPV:
		return "*TyCBPV"
	case *TyEvidence:
		return "*TyEvidence"
	case *TyEvidenceRow:
		return "*TyEvidenceRow"
	case *TyFamilyApp:
		return "*TyFamilyApp"
	case *TyMeta:
		return "*TyMeta"
	case *TySkolem:
		return "*TySkolem"
	case *TyError:
		return "*TyError"
	default:
		return "<unknown>"
	}
}
