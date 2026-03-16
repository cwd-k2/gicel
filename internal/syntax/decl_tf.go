package syntax

import "github.com/cwd-k2/gicel/internal/span"

// DeclTypeFamily is a closed type family declaration.
//
//	type Elem (c :: Type) :: Type = {
//	  Elem (List a) = a;
//	  Elem String   = Rune
//	}
type DeclTypeFamily struct {
	Name       string
	Params     []TyBinder
	ResultKind KindExpr     // result kind annotation
	ResultName string       // non-empty if injective: (r :: Kind) | deps
	Deps       []FunDep     // injectivity dependencies
	Equations  []TFEquation // nil for associated type declarations in class bodies
	S          span.Span
}

// TFEquation is a single equation in a type family declaration.
//
//	Elem (List a) = a
type TFEquation struct {
	Name     string     // repeated family name (validated against declaration)
	Patterns []TypeExpr // left-hand side type patterns
	RHS      TypeExpr   // right-hand side type expression
	S        span.Span
}

// FunDep is a functional dependency annotation: result -> determined params.
//
//	| r -> c      means the result r determines parameter c
type FunDep struct {
	From string   // result variable name
	To   []string // determined parameter names
}

// AssocTypeDecl is an associated type declaration within a class body.
//
//	type Elem c :: Type
type AssocTypeDecl struct {
	Name       string
	Params     []TyBinder
	ResultKind KindExpr
	ResultName string
	Deps       []FunDep
	S          span.Span
}

// AssocTypeDef is an associated type definition within an instance body.
//
//	type Elem (List a) = a
type AssocTypeDef struct {
	Name     string
	Patterns []TypeExpr
	RHS      TypeExpr
	S        span.Span
}

func (*DeclTypeFamily) declNode() {}

func (d *DeclTypeFamily) Span() span.Span { return d.S }
