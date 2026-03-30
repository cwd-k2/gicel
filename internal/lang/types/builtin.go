package types

import "github.com/cwd-k2/gicel/internal/infra/span"

// Pre-defined type constructor kinds (expressed as Type after Kind→Type unification).
var (
	KindOfComputation = &TyArrow{From: TypeOfRows, To: &TyArrow{From: TypeOfRows, To: &TyArrow{From: TypeOfTypes, To: TypeOfTypes, Flags: FlagMetaFree}, Flags: FlagMetaFree}, Flags: FlagMetaFree}
	KindOfThunk       = &TyArrow{From: TypeOfRows, To: &TyArrow{From: TypeOfRows, To: &TyArrow{From: TypeOfTypes, To: TypeOfTypes, Flags: FlagMetaFree}, Flags: FlagMetaFree}, Flags: FlagMetaFree}
)

// Universe-level type constants for the Type/Kind unified representation.
// These are used after the Kind→Type migration (Step 2+) and coexist with
// the old Kind hierarchy until it is fully removed.
var (
	// TypeOfTypes is the kind of value types: Type :: Kind (level 1).
	TypeOfTypes = &TyCon{Name: "Type", Level: L1}
	// TypeOfRows is the kind of row types: Row :: Kind (level 1).
	TypeOfRows = &TyCon{Name: "Row", Level: L1}
	// TypeOfConstraints is the kind of constraint types: Constraint :: Kind (level 1).
	TypeOfConstraints = &TyCon{Name: "Constraint", Level: L1}
	// TypeOfLabels is the kind of type-level label literals: Label :: Kind (level 1).
	TypeOfLabels = &TyCon{Name: "Label", Level: L1}
	// SortZero is the sort of kinds: Kind :: Sort₂ (level 2).
	SortZero = &TyCon{Name: "Kind", Level: L2}
)

// IsBuiltinKindCon reports whether a TyCon is one of the built-in kind
// constants (Type, Row, Constraint, Label, Kind). Identified by name and
// level, not pointer identity — safe for TyCons constructed via any path.
func IsBuiltinKindCon(t *TyCon) bool {
	if t.Level == nil {
		return false
	}
	switch t.Name {
	case "Type", "Row", "Constraint", "Label":
		return LevelEqual(t.Level, L1)
	case "Kind":
		return LevelEqual(t.Level, L2)
	}
	return false
}

// PromotedDataKind creates a kind-level TyCon for a promoted data type (DataKinds).
func PromotedDataKind(name string) *TyCon {
	return &TyCon{Name: name, Level: L1}
}

// SortAt creates a sort at the given level (for Sort₁, Sort₂, ...).
func SortAt(n int) *TyCon {
	return &TyCon{Name: "Sort", Level: &LevelLit{N: n + 2}}
}

// MkComp creates a Computation type.
func MkComp(pre, post, result Type) *TyCBPV {
	return &TyCBPV{Tag: TagComp, Pre: pre, Post: post, Result: result, Flags: MetaFreeFlags(pre, post, result)}
}

// MkThunk creates a Thunk type.
func MkThunk(pre, post, result Type) *TyCBPV {
	return &TyCBPV{Tag: TagThunk, Pre: pre, Post: post, Result: result, Flags: MetaFreeFlags(pre, post, result)}
}

// MkArrow creates a function type.
func MkArrow(from, to Type) *TyArrow {
	return &TyArrow{From: from, To: to, Flags: MetaFreeFlags(from, to)}
}

// MkForall creates a universally quantified type.
func MkForall(v string, k Type, body Type) *TyForall {
	return &TyForall{Var: v, Kind: k, Body: body, Flags: MetaFreeFlags(k, body)}
}

// builtinTyCons holds singleton instances for frequently used type constructors.
// Safe to share: type equality is structural (Name ==), not pointer-based.
// Singleton pointer identity improves Zonk's unchanged-detection hit rate.
var builtinTyCons = map[string]*TyCon{
	"Int": {Name: "Int"}, "String": {Name: "String"},
	"Bool": {Name: "Bool"}, "Double": {Name: "Double"},
	"Rune": {Name: "Rune"}, "Unit": {Name: "Unit"},
	"Computation": {Name: "Computation"}, "Thunk": {Name: "Thunk"},
	TyConRecord: {Name: TyConRecord}, "List": {Name: "List"},
	"Pair": {Name: "Pair"}, "Lift": {Name: "Lift"},
	"Zero": {Name: "Zero"}, "Linear": {Name: "Linear"},
	"Affine": {Name: "Affine"}, "Unrestricted": {Name: "Unrestricted"},
}

// Con returns a TyCon for the given name, reusing a singleton for built-in names.
// Use ConAt when source position must be preserved.
func Con(name string) *TyCon {
	if c, ok := builtinTyCons[name]; ok {
		return c
	}
	return &TyCon{Name: name}
}

// ConAt creates a TyCon with a source span. Always allocates a fresh struct
// because the span is position-specific and cannot be shared.
func ConAt(name string, s span.Span) *TyCon {
	return &TyCon{Name: name, S: s}
}

// Var creates a TyVar.
func Var(name string) *TyVar {
	return &TyVar{Name: name}
}

// MkEvidence creates a TyEvidence from constraint entries and a body type.
func MkEvidence(entries []ConstraintEntry, body Type) *TyEvidence {
	ce := &ConstraintEntries{Entries: entries}
	cr := &TyEvidenceRow{Entries: ce, Flags: EvidenceRowFlags(ce, nil)}
	return &TyEvidence{
		Constraints: cr,
		Body:        body,
		Flags:       MetaFreeFlags(cr, body),
	}
}
