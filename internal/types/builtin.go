package types

// Pre-defined type constructor kinds.
var (
	KindOfComputation = &KArrow{KRow{}, &KArrow{KRow{}, &KArrow{KType{}, KType{}}}}
	KindOfThunk       = &KArrow{KRow{}, &KArrow{KRow{}, &KArrow{KType{}, KType{}}}}
)

// MkComp creates a Computation type.
func MkComp(pre, post, result Type) *TyComp {
	return &TyComp{Pre: pre, Post: post, Result: result}
}

// MkThunk creates a Thunk type.
func MkThunk(pre, post, result Type) *TyThunk {
	return &TyThunk{Pre: pre, Post: post, Result: result}
}

// MkArrow creates a function type.
func MkArrow(from, to Type) *TyArrow {
	return &TyArrow{From: from, To: to}
}

// MkForall creates a universally quantified type.
func MkForall(v string, k Kind, body Type) *TyForall {
	return &TyForall{Var: v, Kind: k, Body: body}
}

// Con creates a TyCon.
func Con(name string) *TyCon {
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
