package types

// Pre-defined type constructor kinds.
var (
	KindOfComputation = &KArrow{KRow{}, &KArrow{KRow{}, &KArrow{KType{}, KType{}}}}
	KindOfThunk       = &KArrow{KRow{}, &KArrow{KRow{}, &KArrow{KType{}, KType{}}}}
)

// MkComp creates a Computation type.
func MkComp(pre, post, result Type) *TyCBPV {
	return &TyCBPV{Tag: TagComp, Pre: pre, Post: post, Result: result}
}

// MkThunk creates a Thunk type.
func MkThunk(pre, post, result Type) *TyCBPV {
	return &TyCBPV{Tag: TagThunk, Pre: pre, Post: post, Result: result}
}

// MkArrow creates a function type.
func MkArrow(from, to Type) *TyArrow {
	return &TyArrow{From: from, To: to}
}

// MkForall creates a universally quantified type.
func MkForall(v string, k Kind, body Type) *TyForall {
	return &TyForall{Var: v, Kind: k, Body: body}
}

// builtinTyCons holds singleton instances for frequently used type constructors.
// Safe to share: type equality is structural (Name ==), not pointer-based.
// Singleton pointer identity improves Zonk's unchanged-detection hit rate.
var builtinTyCons = map[string]*TyCon{
	"Int": {Name: "Int"}, "String": {Name: "String"},
	"Bool": {Name: "Bool"}, "Double": {Name: "Double"},
	"Rune": {Name: "Rune"}, "Unit": {Name: "Unit"},
	"Computation": {Name: "Computation"}, "Thunk": {Name: "Thunk"},
	"Record": {Name: "Record"}, "List": {Name: "List"},
	"Pair": {Name: "Pair"}, "Lift": {Name: "Lift"},
	"Zero": {Name: "Zero"}, "Linear": {Name: "Linear"},
	"Affine": {Name: "Affine"}, "Unrestricted": {Name: "Unrestricted"},
}

// Con returns a TyCon for the given name, reusing a singleton for built-in names.
func Con(name string) *TyCon {
	if c, ok := builtinTyCons[name]; ok {
		return c
	}
	return &TyCon{Name: name}
}

// Var creates a TyVar.
func Var(name string) *TyVar {
	return &TyVar{Name: name}
}

// MkEvidence creates a TyEvidence from constraint entries and a body type.
func MkEvidence(entries []ConstraintEntry, body Type) *TyEvidence {
	return &TyEvidence{
		Constraints: &TyEvidenceRow{Entries: &ConstraintEntries{Entries: entries}},
		Body:        body,
	}
}
