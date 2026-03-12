package syntax

import "github.com/cwd-k2/gomputation/internal/span"

// Decl is a top-level declaration.
type Decl interface {
	declNode()
	Span() span.Span
}

// DeclTypeAnn is a type annotation (f :: T).
type DeclTypeAnn struct {
	Name string
	Type TypeExpr
	S    span.Span
}

// DeclValueDef is a value definition (f := e).
type DeclValueDef struct {
	Name string
	Expr Expr
	S    span.Span
}

// DeclData is a data type declaration (data T a b = C1 ... | C2 ...).
type DeclData struct {
	Name   string
	Params []TyBinder
	Cons   []DeclCon
	S      span.Span
}

// DeclCon is a single data constructor.
type DeclCon struct {
	Name   string
	Fields []TypeExpr
	S      span.Span
}

// DeclTypeAlias is a type alias (type F a = T).
type DeclTypeAlias struct {
	Name   string
	Params []TyBinder
	Body   TypeExpr
	S      span.Span
}

// DeclFixity is a fixity declaration (infixl N op).
type DeclFixity struct {
	Assoc Assoc
	Prec  int
	Op    string
	S     span.Span
}

// Assoc is operator associativity.
type Assoc int

const (
	AssocLeft  Assoc = iota
	AssocRight
	AssocNone
)

// AstProgram is the top-level AST.
type AstProgram struct {
	Decls []Decl
}

func (*DeclTypeAnn) declNode()   {}
func (*DeclValueDef) declNode()  {}
func (*DeclData) declNode()      {}
func (*DeclTypeAlias) declNode() {}
func (*DeclFixity) declNode()    {}

func (d *DeclTypeAnn) Span() span.Span   { return d.S }
func (d *DeclValueDef) Span() span.Span  { return d.S }
func (d *DeclData) Span() span.Span      { return d.S }
func (d *DeclTypeAlias) Span() span.Span { return d.S }
func (d *DeclFixity) Span() span.Span    { return d.S }
