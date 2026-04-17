package types

import "github.com/cwd-k2/gicel/internal/infra/span"

// TypeOps is the single owner of all Type-level operations: construction,
// structural transformation, equality/query, and display/serialization.
//
// Query and display methods are currently stateless. They are included for
// API consistency — "all type operations go through TypeOps" — not because
// they require shared state today. This eliminates the need for callers to
// distinguish allocation-dependent from allocation-independent operations,
// and prevents the scattered inline construction that caused the Flags
// inconsistency (41 production sites at introduction time).
//
// The zero value is ready to use and reproduces the behavior of the original
// free functions. When allocation optimization (hash-consing, arena, flat
// table) is added, methods internalize the strategy change without altering
// signatures.
type TypeOps struct {
	// future: allocation state (hash table, arena, etc.)
}

// defaultOps is the package-level TypeOps instance used by the backward-
// compatible wrapper functions below. Edge-case callers (ir, optimize)
// that do not receive a TypeOps from the compiler pipeline use these
// wrappers and get the zero-value (default) allocation behavior.
var defaultOps TypeOps

// ---------------------------------------------------------------------------
// Construction — all methods take span.Span for source location.
// Zero span means no location. Flags are always computed correctly.
// ---------------------------------------------------------------------------

// Arrow creates a function type (A -> B).
func (o *TypeOps) Arrow(from, to Type, s span.Span) *TyArrow {
	return &TyArrow{From: from, To: to, Flags: MetaFreeFlags(from, to), S: s}
}

// Forall creates a universally quantified type (∀ v:K. body).
func (o *TypeOps) Forall(v string, k, body Type, s span.Span) *TyForall {
	return &TyForall{Var: v, Kind: k, Body: body, Flags: MetaFreeFlags(k, body), S: s}
}

// App creates a type application (F A).
func (o *TypeOps) App(fun, arg Type, s span.Span) *TyApp {
	return &TyApp{Fun: fun, Arg: arg, Flags: MetaFreeFlags(fun, arg), S: s}
}

// AppGrade creates a grade-annotated type application (F @A).
func (o *TypeOps) AppGrade(fun, arg Type, s span.Span) *TyApp {
	return &TyApp{Fun: fun, Arg: arg, IsGrade: true, Flags: MetaFreeFlags(fun, arg), S: s}
}

// Comp creates a CBPV Computation type. grade == nil produces the ungraded
// (3-arg) form; non-nil produces the graded (4-arg) form.
func (o *TypeOps) Comp(pre, post, result, grade Type, s span.Span) *TyCBPV {
	if grade != nil {
		return &TyCBPV{Tag: TagComp, Pre: pre, Post: post, Result: result, Grade: grade, Flags: MetaFreeFlags(pre, post, result, grade), S: s}
	}
	return &TyCBPV{Tag: TagComp, Pre: pre, Post: post, Result: result, Flags: MetaFreeFlags(pre, post, result), S: s}
}

// Thunk creates a CBPV Thunk type (always ungraded).
func (o *TypeOps) Thunk(pre, post, result Type, s span.Span) *TyCBPV {
	return &TyCBPV{Tag: TagThunk, Pre: pre, Post: post, Result: result, Flags: MetaFreeFlags(pre, post, result), S: s}
}

// ThunkGraded creates a graded CBPV Thunk type.
func (o *TypeOps) ThunkGraded(pre, post, result, grade Type, s span.Span) *TyCBPV {
	if grade != nil {
		return &TyCBPV{Tag: TagThunk, Pre: pre, Post: post, Result: result, Grade: grade, Flags: MetaFreeFlags(pre, post, result, grade), S: s}
	}
	return &TyCBPV{Tag: TagThunk, Pre: pre, Post: post, Result: result, Flags: MetaFreeFlags(pre, post, result), S: s}
}

// Evidence creates a qualified type ({ C1, C2 | c } => body).
func (o *TypeOps) Evidence(entries []ConstraintEntry, body Type, s span.Span) *TyEvidence {
	ce := &ConstraintEntries{Entries: entries}
	cr := &TyEvidenceRow{Entries: ce, Flags: EvidenceRowFlags(ce, nil)}
	return &TyEvidence{
		Constraints: cr,
		Body:        body,
		Flags:       MetaFreeFlags(cr, body),
		S:           s,
	}
}

// Con returns a type constructor. When s is the zero span and name matches
// a built-in type, the shared singleton is returned (same as the original
// Con function). Otherwise a fresh TyCon is allocated with the given span.
func (o *TypeOps) Con(name string, s span.Span) *TyCon {
	if s.IsZero() {
		if c, ok := builtinTyCons[name]; ok {
			return c
		}
	}
	return &TyCon{Name: name, S: s}
}

// Var creates a type variable.
func (o *TypeOps) Var(name string, s span.Span) *TyVar {
	return &TyVar{Name: name, S: s}
}

// FamilyApp creates a type family application. FlagNoFamilyApp is
// automatically cleared (a TyFamilyApp is itself a family application).
func (o *TypeOps) FamilyApp(name string, args []Type, kind Type, s span.Span) *TyFamilyApp {
	return &TyFamilyApp{
		Name:  name,
		Args:  args,
		Kind:  kind,
		Flags: metaFreeSlice(kind, args) &^ FlagNoFamilyApp,
		S:     s,
	}
}

// ---------------------------------------------------------------------------
// Backward-compatible wrapper functions.
//
// These delegate to defaultOps and preserve the original signatures.
// Core callers (checker, engine) should use TypeOps methods directly;
// these wrappers exist for edge-case callers (ir, optimize) and tests
// that do not have a TypeOps instance.
// ---------------------------------------------------------------------------

// --- Construction wrappers ---

// MkArrow creates a function type (backward-compatible wrapper).
func MkArrow(from, to Type) *TyArrow { return defaultOps.Arrow(from, to, span.Span{}) }

// MkForall creates a universally quantified type (backward-compatible wrapper).
func MkForall(v string, k Type, body Type) *TyForall {
	return defaultOps.Forall(v, k, body, span.Span{})
}

// MkComp creates an ungraded Computation type (backward-compatible wrapper).
func MkComp(pre, post, result Type) *TyCBPV {
	return defaultOps.Comp(pre, post, result, nil, span.Span{})
}

// MkCompGraded creates a graded Computation type (backward-compatible wrapper).
func MkCompGraded(pre, post, result, grade Type) *TyCBPV {
	return defaultOps.Comp(pre, post, result, grade, span.Span{})
}

// MkThunk creates a Thunk type (backward-compatible wrapper).
func MkThunk(pre, post, result Type) *TyCBPV {
	return defaultOps.Thunk(pre, post, result, span.Span{})
}

// MkEvidence creates a TyEvidence (backward-compatible wrapper).
func MkEvidence(entries []ConstraintEntry, body Type) *TyEvidence {
	return defaultOps.Evidence(entries, body, span.Span{})
}

// Con returns a TyCon for the given name (backward-compatible wrapper).
func Con(name string) *TyCon { return defaultOps.Con(name, span.Span{}) }

// ConAt creates a TyCon with a source span (backward-compatible wrapper).
func ConAt(name string, s span.Span) *TyCon { return defaultOps.Con(name, s) }

// Var creates a TyVar (backward-compatible wrapper).
func Var(name string) *TyVar { return defaultOps.Var(name, span.Span{}) }

// --- Transformation wrappers ---

// MapType applies f to every direct child (backward-compatible wrapper).
func MapType(t Type, f func(Type) Type) Type { return defaultOps.MapType(t, f) }

// Subst applies [varName := replacement] (backward-compatible wrapper).
func Subst(t Type, varName string, replacement Type) Type {
	return defaultOps.Subst(t, varName, replacement)
}

// SubstMany applies parallel substitution (backward-compatible wrapper).
func SubstMany(t Type, typeSubs map[string]Type, levelSubs map[string]LevelExpr) Type {
	return defaultOps.SubstMany(t, typeSubs, levelSubs)
}

// SubstLevel replaces a level variable (backward-compatible wrapper).
func SubstLevel(t Type, levelVarName string, replacement LevelExpr) Type {
	return defaultOps.SubstLevel(t, levelVarName, replacement)
}

// PeelForalls instantiates a chain of TyForall binders (backward-compatible wrapper).
func PeelForalls(t Type, visit func(f *TyForall) (typeRepl Type, levelRepl LevelExpr)) Type {
	return defaultOps.PeelForalls(t, visit)
}

// PrepareSubst creates a PreparedSubst (backward-compatible wrapper).
func PrepareSubst(subs map[string]Type) *PreparedSubst {
	return defaultOps.PrepareSubst(subs)
}

// --- Query wrappers ---

// Equal checks structural equality (backward-compatible wrapper).
func Equal(a, b Type) bool { return defaultOps.Equal(a, b) }

// FreeVars returns the set of free type/row variables (backward-compatible wrapper).
func FreeVars(t Type) map[string]struct{} { return defaultOps.FreeVars(t) }

// OccursIn checks if a variable name appears free (backward-compatible wrapper).
func OccursIn(name string, t Type) bool { return defaultOps.OccursIn(name, t) }

// --- Display wrappers ---

// Pretty renders a type as human-readable text (backward-compatible wrapper).
func Pretty(t Type) string { return defaultOps.Pretty(t) }

// PrettyDisplay renders a type for IDE display (backward-compatible wrapper).
func PrettyDisplay(t Type) string { return defaultOps.PrettyDisplay(t) }

// PrettyTypeAsKind renders a kind-level type (backward-compatible wrapper).
func PrettyTypeAsKind(t Type) string { return defaultOps.PrettyTypeAsKind(t) }

// WriteTypeKey writes a canonical structural key (backward-compatible wrapper).
func WriteTypeKey(b KeyWriter, t Type) { defaultOps.WriteTypeKey(b, t) }

// TypeKey returns the canonical structural key as a string (backward-compatible wrapper).
func TypeKey(t Type) string { return defaultOps.TypeKey(t) }

// TypeListKey serializes a prefix with type arguments (backward-compatible wrapper).
func TypeListKey(prefix string, sep byte, args []Type) string {
	return defaultOps.TypeListKey(prefix, sep, args)
}
