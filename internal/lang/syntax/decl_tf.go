package syntax

import "github.com/cwd-k2/gicel/internal/infra/span"

// ImplField is an associated type/form definition within an impl body.
// Value bindings (methods) are represented as Bind in ExprBlock, not here.
type ImplField struct {
	Name    string
	IsType  bool     // true for associated type definitions (type Elem := a)
	IsData  bool     // true for associated form definitions (form Elem := ListElem a)
	TypeDef TypeExpr // non-nil: the RHS type expression
	S       span.Span
}
