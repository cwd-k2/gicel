package types

import "github.com/cwd-k2/gomputation/internal/span"

// Pre-defined type constructor kinds.
var (
	KindOfComputation = &KArrow{KRow{}, &KArrow{KRow{}, &KArrow{KType{}, KType{}}}}
	KindOfThunk       = &KArrow{KRow{}, &KArrow{KRow{}, &KArrow{KType{}, KType{}}}}
)

var noSpan = span.Span{}

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

// EmptyRow creates an empty closed row.
func EmptyRow() *TyRow {
	return &TyRow{}
}

// ClosedRow creates a closed row from fields.
func ClosedRow(fields ...RowField) *TyRow {
	return Normalize(&TyRow{Fields: fields})
}

// OpenRow creates an open row with a tail.
func OpenRow(fields []RowField, tail Type) *TyRow {
	return Normalize(&TyRow{Fields: fields, Tail: tail})
}

// Con creates a TyCon.
func Con(name string) *TyCon {
	return &TyCon{Name: name}
}

// Var creates a TyVar.
func Var(name string) *TyVar {
	return &TyVar{Name: name}
}

// MkQual creates a qualified (constrained) type.
func MkQual(className string, args []Type, body Type) *TyQual {
	return &TyQual{ClassName: className, Args: args, Body: body}
}
