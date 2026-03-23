package syntax

import "github.com/cwd-k2/gicel/internal/infra/span"

// Decl is a top-level declaration.
type Decl interface {
	declNode()
	Span() span.Span
}

// DeclTypeAnn is a type annotation (name :: Type).
type DeclTypeAnn struct {
	Name string
	Type TypeExpr
	S    span.Span
}

// DeclValueDef is a value definition (name := expr).
type DeclValueDef struct {
	Name string
	Expr Expr
	S    span.Span
}

// DeclForm is a nominal type declaration.
//
// Unified syntax: form Name [:: Kind] := Body;
// Body is a type expression: \params. [constraints =>] { fields/constructors }.
// The checker decomposes Body into parameters, constraints, and fields.
//
// Examples:
//
//	form Maybe := \a. { Nothing: (); Just: a; };
//	form Eq := \a. { eq: a -> a -> Bool; neq: a -> a -> Bool := \x y. not (eq x y); };
//	form Ord := \a. Eq a => { compare: a -> a -> Ordering; };
//	form Collection := \(c: Type). { type Elem :: Type; empty: c; insert: Elem c -> c -> c; };
type DeclForm struct {
	Name    string
	KindAnn KindExpr // optional :: Kind annotation (nil if omitted)
	Body    TypeExpr // \params. [constraints =>] { fields }
	S       span.Span
}

// DeclTypeAlias is a structural type declaration.
//
// Unified syntax: type Name [:: Kind] := Body;
// Body is a type expression: \params. type-expr.
// For closed type families, the body contains a case expression.
//
// Examples:
//
//	type Pair := \a b. { fst: a; snd: b; };
//	type Dual :: Type -> Type := \s. case s { Send s => Recv (Dual s); ... };
type DeclTypeAlias struct {
	Name    string
	KindAnn KindExpr // optional :: Kind annotation (nil if omitted)
	Body    TypeExpr // \params. type-expr (or case for type families)
	S       span.Span
}

// DeclImpl is an axiom registration (evidence for a type class).
//
// Unified syntax:
//
//	impl name :: Type := expr;    -- named (exported, solver-visible)
//	impl Type := expr;            -- unnamed (exported, solver-visible)
//	impl _name :: Type := expr;   -- private (solver-invisible)
//
// Examples:
//
//	impl eqBool :: Eq Bool := { eq := \x y. True; };
//	impl Monad Maybe := { pure := \x. Just x; bind := ...; };
//	impl Collection (List a) := { type Elem := a; empty := Nil; insert := Cons; };
type DeclImpl struct {
	Name string   // "" for unnamed impl, "_"-prefix for private
	Ann  TypeExpr // type being implemented (e.g., Eq Bool, \a. Equal a a)
	Body Expr     // implementation expression
	S    span.Span
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

// ImportName identifies a single name in a selective import list.
type ImportName struct {
	Name    string   // "add", "Bool", "Maybe", "(+)"
	SubList []string // nil = name only, non-nil = explicit sub-list
	HasSub  bool     // true if parenthesized form used: Name(...) or Name(..)
	AllSubs bool     // true if (..) used
}

// DeclImport is a module import declaration.
//
//	import ModuleName                       -- open: all names unqualified
//	import ModuleName (name, T(..), C(A))   -- selective: specified names only
//	import ModuleName as Alias              -- qualified: Alias.name access only
type DeclImport struct {
	ModuleName string
	Alias      string       // "" = open/selective, non-empty = qualified
	Names      []ImportName // nil = import all (open), non-nil = selective
	S          span.Span
}

// AstProgram is the top-level AST.
type AstProgram struct {
	Imports []DeclImport
	Decls   []Decl
}

func (*DeclTypeAnn) declNode()   {}
func (*DeclValueDef) declNode()  {}
func (*DeclForm) declNode()      {}
func (*DeclTypeAlias) declNode() {}
func (*DeclImpl) declNode()      {}
func (*DeclFixity) declNode()    {}
func (*DeclImport) declNode()    {}

func (d *DeclTypeAnn) Span() span.Span   { return d.S }
func (d *DeclValueDef) Span() span.Span  { return d.S }
func (d *DeclForm) Span() span.Span      { return d.S }
func (d *DeclTypeAlias) Span() span.Span { return d.S }
func (d *DeclImpl) Span() span.Span      { return d.S }
func (d *DeclFixity) Span() span.Span    { return d.S }
func (d *DeclImport) Span() span.Span    { return d.S }
