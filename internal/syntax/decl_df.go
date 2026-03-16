package syntax

import "github.com/cwd-k2/gicel/internal/span"

// DeclDataFamily is a standalone data family declaration (future use).
//
//	data family Collection a :: Type
type DeclDataFamily struct {
	Name       string
	Params     []TyBinder
	ResultKind KindExpr
	S          span.Span
}

// AssocDataDecl is an associated data family declaration within a class body.
//
//	data Elem c :: Type
type AssocDataDecl struct {
	Name       string
	Params     []TyBinder
	ResultKind KindExpr
	S          span.Span
}

// AssocDataDef is an associated data family instance within an instance body.
//
//	data Elem (List a) = ListElem a
type AssocDataDef struct {
	Name     string
	Patterns []TypeExpr
	Cons     []DeclCon
	S        span.Span
}

func (*DeclDataFamily) declNode() {}

func (d *DeclDataFamily) Span() span.Span { return d.S }
