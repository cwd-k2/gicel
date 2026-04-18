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

// ---------------------------------------------------------------------------
// Construction — span-free (default) and span-aware (At suffix) variants.
//
// Span-free methods are for synthetic type construction where source
// location is not meaningful. At-suffixed methods carry source location
// for types resolved from user syntax.
// ---------------------------------------------------------------------------

// Arrow creates a function type (A -> B).
func (o *TypeOps) Arrow(from, to Type) *TyArrow {
	return &TyArrow{From: from, To: to, Flags: MetaFreeFlags(from, to)}
}

// ArrowAt creates a function type with source location.
func (o *TypeOps) ArrowAt(from, to Type, s span.Span) *TyArrow {
	return &TyArrow{From: from, To: to, Flags: MetaFreeFlags(from, to), S: s}
}

// Forall creates a universally quantified type (∀ v:K. body).
func (o *TypeOps) Forall(v string, k, body Type) *TyForall {
	return &TyForall{Var: v, Kind: k, Body: body, Flags: MetaFreeFlags(k, body)}
}

// ForallAt creates a universally quantified type with source location.
func (o *TypeOps) ForallAt(v string, k, body Type, s span.Span) *TyForall {
	return &TyForall{Var: v, Kind: k, Body: body, Flags: MetaFreeFlags(k, body), S: s}
}

// App creates a type application (F A).
func (o *TypeOps) App(fun, arg Type) *TyApp {
	return &TyApp{Fun: fun, Arg: arg, Flags: MetaFreeFlags(fun, arg)}
}

// AppAt creates a type application with source location.
func (o *TypeOps) AppAt(fun, arg Type, s span.Span) *TyApp {
	return &TyApp{Fun: fun, Arg: arg, Flags: MetaFreeFlags(fun, arg), S: s}
}

// AppGradeAt creates a grade-annotated type application with source location.
func (o *TypeOps) AppGradeAt(fun, arg Type, s span.Span) *TyApp {
	return &TyApp{Fun: fun, Arg: arg, IsGrade: true, Flags: MetaFreeFlags(fun, arg), S: s}
}

// Comp creates a CBPV Computation type. grade == nil produces the ungraded
// (3-arg) form; non-nil produces the graded (4-arg) form.
func (o *TypeOps) Comp(pre, post, result, grade Type) *TyCBPV {
	return makeCBPV(TagComp, pre, post, result, grade, span.Span{})
}

// CompAt creates a CBPV Computation type with source location.
func (o *TypeOps) CompAt(pre, post, result, grade Type, s span.Span) *TyCBPV {
	return makeCBPV(TagComp, pre, post, result, grade, s)
}

// Thunk creates a CBPV Thunk type. grade == nil produces the ungraded
// (3-arg) form; non-nil produces the graded (4-arg) form.
// Symmetric with Comp by the CBPV adjunction (Thunk ⊣ Force).
func (o *TypeOps) Thunk(pre, post, result, grade Type) *TyCBPV {
	return makeCBPV(TagThunk, pre, post, result, grade, span.Span{})
}

// ThunkAt creates a CBPV Thunk type with source location.
func (o *TypeOps) ThunkAt(pre, post, result, grade Type, s span.Span) *TyCBPV {
	return makeCBPV(TagThunk, pre, post, result, grade, s)
}

// makeCBPV is the single construction point for TyCBPV nodes.
// grade == nil selects the ungraded form; non-nil selects the graded form.
func makeCBPV(tag CBPVTag, pre, post, result, grade Type, s span.Span) *TyCBPV {
	if grade != nil {
		return &TyCBPV{Tag: tag, Pre: pre, Post: post, Result: result, Grade: grade, Flags: MetaFreeFlags(pre, post, result, grade), S: s}
	}
	return &TyCBPV{Tag: tag, Pre: pre, Post: post, Result: result, Flags: MetaFreeFlags(pre, post, result), S: s}
}

// Evidence creates a qualified type ({ C1, C2 | c } => body).
func (o *TypeOps) Evidence(entries []ConstraintEntry, body Type) *TyEvidence {
	ce := &ConstraintEntries{Entries: entries}
	cr := &TyEvidenceRow{Entries: ce, Flags: EvidenceRowFlags(ce, nil)}
	return &TyEvidence{Constraints: cr, Body: body, Flags: MetaFreeFlags(cr, body)}
}

// EvidenceAt creates a qualified type with source location.
func (o *TypeOps) EvidenceAt(entries []ConstraintEntry, body Type, s span.Span) *TyEvidence {
	ce := &ConstraintEntries{Entries: entries}
	cr := &TyEvidenceRow{Entries: ce, Flags: EvidenceRowFlags(ce, nil)}
	return &TyEvidence{Constraints: cr, Body: body, Flags: MetaFreeFlags(cr, body), S: s}
}

// EvidenceWrap creates a TyEvidence from an existing evidence row and body.
func (o *TypeOps) EvidenceWrap(constraints *TyEvidenceRow, body Type) *TyEvidence {
	return &TyEvidence{Constraints: constraints, Body: body, Flags: MetaFreeFlags(constraints, body)}
}

// EvidenceWrapAt creates a TyEvidence from an existing evidence row with source location.
func (o *TypeOps) EvidenceWrapAt(constraints *TyEvidenceRow, body Type, s span.Span) *TyEvidence {
	return &TyEvidence{Constraints: constraints, Body: body, Flags: MetaFreeFlags(constraints, body), S: s}
}

// Con returns a type constructor, reusing a singleton for built-in names.
func (o *TypeOps) Con(name string) *TyCon {
	if c, ok := builtinTyCons[name]; ok {
		return c
	}
	return &TyCon{Name: name}
}

// ConAt creates a type constructor with source location. Always allocates
// fresh because the span is position-specific and cannot be shared.
func (o *TypeOps) ConAt(name string, s span.Span) *TyCon {
	return &TyCon{Name: name, S: s}
}

// ConLevel creates a type constructor with an explicit universe level.
func (o *TypeOps) ConLevel(name string, level LevelExpr, isLabel bool) *TyCon {
	return &TyCon{Name: name, Level: level, IsLabel: isLabel}
}

// ConLevelAt creates a type constructor with level and source location.
func (o *TypeOps) ConLevelAt(name string, level LevelExpr, isLabel bool, s span.Span) *TyCon {
	return &TyCon{Name: name, Level: level, IsLabel: isLabel, S: s}
}

// Var creates a type variable.
func (o *TypeOps) Var(name string) *TyVar {
	return &TyVar{Name: name}
}

// VarAt creates a type variable with source location.
func (o *TypeOps) VarAt(name string, s span.Span) *TyVar {
	return &TyVar{Name: name, S: s}
}

// FamilyApp creates a type family application. FlagNoFamilyApp is
// automatically cleared.
func (o *TypeOps) FamilyApp(name string, args []Type, kind Type) *TyFamilyApp {
	return &TyFamilyApp{Name: name, Args: args, Kind: kind, Flags: metaFreeSlice(kind, args) &^ FlagNoFamilyApp}
}

// FamilyAppAt creates a type family application with source location.
func (o *TypeOps) FamilyAppAt(name string, args []Type, kind Type, s span.Span) *TyFamilyApp {
	return &TyFamilyApp{Name: name, Args: args, Kind: kind, Flags: metaFreeSlice(kind, args) &^ FlagNoFamilyApp, S: s}
}
