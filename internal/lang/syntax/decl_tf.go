package syntax

import "github.com/cwd-k2/gicel/internal/infra/span"

// ImplField is a method/field definition within an impl body.
// Covers both value bindings and associated type/data definitions.
type ImplField struct {
	Name    string
	IsType  bool     // true for associated type definitions (type Elem := a)
	IsData  bool     // true for associated data definitions (data Elem := ListElem a)
	TypeDef TypeExpr // non-nil when IsType (the RHS type)
	ValDef  Expr     // non-nil when !IsType (the RHS expression)
	S       span.Span
}
