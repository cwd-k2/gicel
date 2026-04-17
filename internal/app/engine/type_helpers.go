package engine

import (
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Type is the unified type representation used across the public API.
type Type = types.Type

// Kind classifies types and rows.
type Kind = types.Type

// RowField is a single label:type pair in a row.
type RowField = types.RowField

// TypeOps is the single owner of all type-level operations.
// The zero value is ready to use. When constructing types for
// DeclareBinding / DeclareAssumption, create a TypeOps and call its
// methods directly (e.g. ops.Con("Int"), ops.Arrow(a, b)).
type TypeOps = types.TypeOps

// RowBuilder helps construct row types incrementally.
type RowBuilder struct {
	ops    *types.TypeOps
	fields []types.RowField
	tail   types.Type
}

// NewRow starts building a row type.
func NewRow(ops *types.TypeOps) *RowBuilder {
	return &RowBuilder{ops: ops}
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
	return types.OpenRow(rb.fields, rb.ops.Var(tailVar))
}

// KindType returns the Type kind (for RegisterType).
func KindType() types.Type {
	return types.TypeOfTypes
}

// KindRow returns the Row kind (for RegisterType).
func KindRow() types.Type {
	return types.TypeOfRows
}

// EmptyRowType creates an empty closed row type.
func EmptyRowType() types.Type {
	return types.EmptyRow()
}

// ClosedRowType creates a closed row type from fields.
func ClosedRowType(fields ...types.RowField) types.Type {
	return types.ClosedRow(fields...)
}
