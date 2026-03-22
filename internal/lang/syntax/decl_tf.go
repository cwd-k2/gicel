package syntax

import "github.com/cwd-k2/gicel/internal/infra/span"

// ImplTypeDef is an associated type definition within an impl body.
//
//	type Elem := a;
type ImplTypeDef struct {
	Name string
	Body TypeExpr
	S    span.Span
}

// ImplField is a method/field definition within an impl body.
// Covers both value bindings and associated type definitions.
type ImplField struct {
	Name    string
	IsType  bool     // true for associated type definitions (type Elem := a)
	TypeDef TypeExpr // non-nil when IsType (the RHS type)
	ValDef  Expr     // non-nil when !IsType (the RHS expression)
	S       span.Span
}
