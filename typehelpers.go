package gomputation

import "github.com/cwd-k2/gomputation/pkg/types"

// Type construction helpers for use with DeclareBinding and DeclareAssumption.
// These wrap pkg/types constructors for convenience.

// ConType creates a simple type constructor (e.g. "Int", "String", "Bool").
func ConType(name string) types.Type {
	return types.Con(name)
}

// ArrowType creates a function type: from -> to.
func ArrowType(from, to types.Type) types.Type {
	return types.MkArrow(from, to)
}

// CompType creates a Computation type: Computation pre post result.
func CompType(pre, post, result types.Type) types.Type {
	return types.MkComp(pre, post, result)
}

// ThunkType creates a Thunk type: Thunk pre post result.
func ThunkType(pre, post, result types.Type) types.Type {
	return types.MkThunk(pre, post, result)
}

// ForallType creates a universally quantified type: forall var. body.
func ForallType(varName string, body types.Type) types.Type {
	return types.MkForall(varName, types.KType{}, body)
}

// ForallRow creates a universally quantified type with Row kind: forall (r : Row). body.
func ForallRow(varName string, body types.Type) types.Type {
	return types.MkForall(varName, types.KRow{}, body)
}

// VarType creates a type variable reference.
func VarType(name string) types.Type {
	return types.Var(name)
}

// AppType creates a type application: f a.
func AppType(f, arg types.Type) types.Type {
	return &types.TyApp{Fun: f, Arg: arg}
}

// RowBuilder helps construct row types incrementally.
type RowBuilder struct {
	fields []types.RowField
	tail   types.Type
}

// NewRow starts building a row type.
func NewRow() *RowBuilder {
	return &RowBuilder{}
}

// And adds a field to the row.
func (rb *RowBuilder) And(label string, ty types.Type) *RowBuilder {
	rb.fields = append(rb.fields, types.RowField{Label: label, Type: ty})
	return rb
}

// Closed builds a closed row (no tail variable).
func (rb *RowBuilder) Closed() types.Type {
	return types.ClosedRow(rb.fields...)
}

// Open builds an open row with a tail variable.
func (rb *RowBuilder) Open(tailVar string) types.Type {
	return types.OpenRow(rb.fields, types.Var(tailVar))
}

// KindType returns the Type kind (for RegisterType).
func KindType() types.Kind {
	return types.KType{}
}

// KindRow returns the Row kind (for RegisterType).
func KindRow() types.Kind {
	return types.KRow{}
}

// EmptyRowType creates an empty closed row type.
func EmptyRowType() types.Type {
	return types.EmptyRow()
}
