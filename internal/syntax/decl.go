package syntax

import "github.com/cwd-k2/gicel/internal/span"

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
// If GADTCons is non-empty, this is a GADT declaration (data T a = { C :: Type }).
type DeclData struct {
	Name     string
	Params   []TyBinder
	Cons     []DeclCon     // ADT constructors
	GADTCons []GADTConDecl // GADT constructors (mutually exclusive with Cons)
	S        span.Span
}

// GADTConDecl is a GADT constructor with a full type signature.
type GADTConDecl struct {
	Name string
	Type TypeExpr // full type: arg1 -> arg2 -> ... -> T args
	S    span.Span
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
	AssocLeft Assoc = iota
	AssocRight
	AssocNone
)

// DeclClass is a type class declaration (class Super => ClassName params | deps { methods }).
type DeclClass struct {
	Supers        []TypeExpr // superclass constraints
	Name          string
	TyParams      []TyBinder
	FunDeps       []FunDep        // functional dependencies: | a -> b
	Methods       []ClassMethod
	AssocTypes    []AssocTypeDecl  // associated type declarations
	AssocDataDecls []AssocDataDecl // associated data family declarations
	S             span.Span
}

// ClassMethod is a method signature in a class declaration.
type ClassMethod struct {
	Name string
	Type TypeExpr
	S    span.Span
}

// DeclInstance is a type class instance (instance Context => ClassName types { methods }).
type DeclInstance struct {
	Context       []TypeExpr // instance constraints
	ClassName     string
	TypeArgs      []TypeExpr
	Methods       []InstMethod
	AssocTypeDefs []AssocTypeDef  // associated type definitions
	AssocDataDefs []AssocDataDef  // associated data family definitions
	S             span.Span
}

// InstMethod is a method definition in an instance declaration.
type InstMethod struct {
	Name string
	Expr Expr
	S    span.Span
}

// DeclImport is a module import declaration (import ModuleName).
type DeclImport struct {
	ModuleName string
	S          span.Span
}

// AstProgram is the top-level AST.
type AstProgram struct {
	Imports []DeclImport
	Decls   []Decl
}

func (*DeclTypeAnn) declNode()   {}
func (*DeclValueDef) declNode()  {}
func (*DeclData) declNode()      {}
func (*DeclTypeAlias) declNode() {}
func (*DeclFixity) declNode()    {}
func (*DeclClass) declNode()     {}
func (*DeclInstance) declNode()  {}
func (*DeclImport) declNode()    {}

func (d *DeclTypeAnn) Span() span.Span   { return d.S }
func (d *DeclValueDef) Span() span.Span  { return d.S }
func (d *DeclData) Span() span.Span      { return d.S }
func (d *DeclTypeAlias) Span() span.Span { return d.S }
func (d *DeclFixity) Span() span.Span    { return d.S }
func (d *DeclClass) Span() span.Span     { return d.S }
func (d *DeclInstance) Span() span.Span  { return d.S }
func (d *DeclImport) Span() span.Span    { return d.S }
