package syntax

import "github.com/cwd-k2/gicel/internal/infra/span"

// TypeExpr is a surface-level type expression (before kind checking).
type TypeExpr interface {
	typeExprNode()
	Span() span.Span
}

type TyExprVar struct {
	Name string
	Kind KindExpr // non-nil when used as a kinded class type parameter
	S    span.Span
}

type TyExprCon struct {
	Name string
	S    span.Span
}

// TyExprQualCon is a qualified type constructor reference: N.Int
type TyExprQualCon struct {
	Qualifier string
	Name      string
	S         span.Span
}

type TyExprApp struct {
	Fun TypeExpr
	Arg TypeExpr
	S   span.Span
}

type TyExprArrow struct {
	From TypeExpr
	To   TypeExpr
	S    span.Span
}

type TyExprForall struct {
	Binders []TyBinder
	Body    TypeExpr
	S       span.Span
}

type TyExprRow struct {
	Fields    []TyRowField
	TypeDecls []TyRowTypeDecl // associated type declarations (in data bodies only)
	Tail      *TyExprVar
	S         span.Span
}

// TyRowTypeDecl is an associated type declaration within a data body row.
//
//	type Elem :: Type;
type TyRowTypeDecl struct {
	Name    string
	KindAnn KindExpr
	S       span.Span
}

type TyExprParen struct {
	Inner TypeExpr
	S     span.Span
}

// TyExprCase is a type-level case expression (closed type family body).
//
//	case scrutinee { Pattern => Body; ... }
type TyExprCase struct {
	Scrutinee TypeExpr
	Alts      []TyAlt
	S         span.Span
}

// TyAlt is a single alternative in a type-level case.
type TyAlt struct {
	EqName     string   // equation name (from legacy format, for validation)
	RawPatCount int     // number of patterns before synthesis (for arity validation)
	Pattern    TypeExpr // type pattern (e.g., Send s, List a)
	Body       TypeExpr // result type
	S          span.Span
}

// TyExprQual is a qualified type: Constraint => Body.
type TyExprQual struct {
	Constraint TypeExpr
	Body       TypeExpr
	S          span.Span
}

type TyBinder struct {
	Name string
	Kind KindExpr // nil means kind not annotated (inferred)
	S    span.Span
}

type TyRowField struct {
	Label   string
	Type    TypeExpr
	Mult    TypeExpr // nil if no @Mult annotation (surface syntax, pre-grade migration)
	Default Expr     // nil if no default value (for method defaults in data bodies)
	S       span.Span
}

// ---- Kind expressions ----

type KindExpr interface {
	kindExprNode()
	Span() span.Span
}

type KindExprType struct{ S span.Span }
type KindExprRow struct{ S span.Span }
type KindExprConstraint struct{ S span.Span }
type KindExprArrow struct {
	From KindExpr
	To   KindExpr
	S    span.Span
}

// KindExprName is a user-defined kind name (DataKinds promotion).
type KindExprName struct {
	Name string
	S    span.Span
}

// KindExprSort is the sort of kinds: Kind.
// Used in \ binders: \ (k: Kind). ...
type KindExprSort struct{ S span.Span }

func (*TyExprVar) typeExprNode()     {}
func (*TyExprCon) typeExprNode()     {}
func (*TyExprQualCon) typeExprNode() {}
func (*TyExprApp) typeExprNode()     {}
func (*TyExprArrow) typeExprNode()   {}
func (*TyExprForall) typeExprNode()  {}
func (*TyExprRow) typeExprNode()     {}
func (*TyExprParen) typeExprNode()   {}
func (*TyExprCase) typeExprNode()    {}
func (*TyExprQual) typeExprNode()    {}

func (t *TyExprVar) Span() span.Span     { return t.S }
func (t *TyExprCon) Span() span.Span     { return t.S }
func (t *TyExprQualCon) Span() span.Span { return t.S }
func (t *TyExprApp) Span() span.Span     { return t.S }
func (t *TyExprArrow) Span() span.Span   { return t.S }
func (t *TyExprForall) Span() span.Span  { return t.S }
func (t *TyExprRow) Span() span.Span     { return t.S }
func (t *TyExprParen) Span() span.Span   { return t.S }
func (t *TyExprCase) Span() span.Span    { return t.S }
func (t *TyExprQual) Span() span.Span    { return t.S }

func (*KindExprType) kindExprNode()       {}
func (*KindExprRow) kindExprNode()        {}
func (*KindExprConstraint) kindExprNode() {}
func (*KindExprArrow) kindExprNode()      {}
func (*KindExprName) kindExprNode()       {}
func (*KindExprSort) kindExprNode()       {}

func (k *KindExprType) Span() span.Span       { return k.S }
func (k *KindExprRow) Span() span.Span        { return k.S }
func (k *KindExprConstraint) Span() span.Span { return k.S }
func (k *KindExprArrow) Span() span.Span      { return k.S }
func (k *KindExprName) Span() span.Span       { return k.S }
func (k *KindExprSort) Span() span.Span       { return k.S }
