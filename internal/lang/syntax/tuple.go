package syntax

import (
	"strconv"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TupleLabel returns the canonical field label for a 1-based tuple position.
// Position 1 → "_1", position 2 → "_2", etc.
// This is the single authoritative encoding of tuple position labels,
// used by the parser, type checker, evaluator, and pretty-printers.
func TupleLabel(pos int) string {
	return "_" + strconv.Itoa(pos)
}

// DesugarConstraintTuple detects a tuple type used as a constraint group.
// (C1, C2, ...) parses as TyExprApp(Record, TyExprRow{_1: C1, _2: C2, ...}).
// Returns the individual constraint types if the pattern matches (2+ elements
// with valid tuple labels), nil otherwise.
func DesugarConstraintTuple(t TypeExpr) []TypeExpr {
	app, ok := t.(*TyExprApp)
	if !ok {
		return nil
	}
	con, ok := app.Fun.(*TyExprCon)
	if !ok || con.Name != types.TyConRecord {
		return nil
	}
	row, ok := app.Arg.(*TyExprRow)
	if !ok || len(row.Fields) < 2 || row.Tail != nil {
		return nil
	}
	constraints := make([]TypeExpr, len(row.Fields))
	for i, f := range row.Fields {
		if f.Label != TupleLabel(i+1) {
			return nil
		}
		constraints[i] = f.Type
	}
	return constraints
}
