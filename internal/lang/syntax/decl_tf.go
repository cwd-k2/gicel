package syntax

import "github.com/cwd-k2/gicel/internal/infra/span"

// ImplField is an associated type/data definition within an impl body.
// Value bindings (methods) are represented as AstBind in ExprBlock, not here.
type ImplField struct {
	Name    string
	IsType  bool     // true for associated type definitions (type Elem := a)
	IsData  bool     // true for associated data definitions (data Elem := ListElem a)
	TypeDef TypeExpr // non-nil: the RHS type expression
	S       span.Span
}
