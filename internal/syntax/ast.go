package syntax

import "github.com/cwd-k2/gomputation/internal/span"

// ---- Expressions ----

// Expr is a surface-level expression.
type Expr interface {
	exprNode()
	Span() span.Span
}

type ExprVar struct {
	Name string
	S    span.Span
}

type ExprCon struct {
	Name string
	S    span.Span
}

type ExprApp struct {
	Fun Expr
	Arg Expr
	S   span.Span
}

type ExprTyApp struct {
	Expr    Expr
	TyArg   TypeExpr
	S       span.Span
}

type ExprLam struct {
	Params []Pattern
	Body   Expr
	S      span.Span
}

type ExprCase struct {
	Scrutinee Expr
	Alts      []AstAlt
	S         span.Span
}

type ExprDo struct {
	Stmts []Stmt
	S     span.Span
}

type ExprBlock struct {
	Binds []AstBind
	Body  Expr
	S     span.Span
}

type ExprInfix struct {
	Left  Expr
	Op    string
	Right Expr
	S     span.Span
}

type ExprAnn struct {
	Expr    Expr
	AnnType TypeExpr
	S       span.Span
}

type ExprParen struct {
	Inner Expr
	S     span.Span
}

func (*ExprVar) exprNode()    {}
func (*ExprCon) exprNode()    {}
func (*ExprApp) exprNode()    {}
func (*ExprTyApp) exprNode()  {}
func (*ExprLam) exprNode()    {}
func (*ExprCase) exprNode()   {}
func (*ExprDo) exprNode()     {}
func (*ExprBlock) exprNode()  {}
func (*ExprInfix) exprNode()  {}
func (*ExprAnn) exprNode()    {}
func (*ExprParen) exprNode()  {}

func (e *ExprVar) Span() span.Span    { return e.S }
func (e *ExprCon) Span() span.Span    { return e.S }
func (e *ExprApp) Span() span.Span    { return e.S }
func (e *ExprTyApp) Span() span.Span  { return e.S }
func (e *ExprLam) Span() span.Span    { return e.S }
func (e *ExprCase) Span() span.Span   { return e.S }
func (e *ExprDo) Span() span.Span     { return e.S }
func (e *ExprBlock) Span() span.Span  { return e.S }
func (e *ExprInfix) Span() span.Span  { return e.S }
func (e *ExprAnn) Span() span.Span    { return e.S }
func (e *ExprParen) Span() span.Span  { return e.S }

// ---- Statements (in do blocks) ----

type Stmt interface {
	stmtNode()
	Span() span.Span
}

type StmtBind struct {
	Var  string
	Comp Expr
	S    span.Span
}

type StmtPureBind struct {
	Var  string
	Expr Expr
	S    span.Span
}

type StmtExpr struct {
	Expr Expr
	S    span.Span
}

func (*StmtBind) stmtNode()     {}
func (*StmtPureBind) stmtNode() {}
func (*StmtExpr) stmtNode()     {}

func (s *StmtBind) Span() span.Span     { return s.S }
func (s *StmtPureBind) Span() span.Span { return s.S }
func (s *StmtExpr) Span() span.Span     { return s.S }

// ---- Block bindings ----

type AstBind struct {
	Var  string
	Expr Expr
	S    span.Span
}

// ---- Case alternatives ----

type AstAlt struct {
	Pattern Pattern
	Body    Expr
	S       span.Span
}

// ---- Patterns ----

type Pattern interface {
	patternNode()
	Span() span.Span
}

type PatVar struct {
	Name string
	S    span.Span
}

type PatWild struct {
	S span.Span
}

type PatCon struct {
	Con  string
	Args []Pattern
	S    span.Span
}

type PatParen struct {
	Inner Pattern
	S     span.Span
}

func (*PatVar) patternNode()   {}
func (*PatWild) patternNode()  {}
func (*PatCon) patternNode()   {}
func (*PatParen) patternNode() {}

func (p *PatVar) Span() span.Span   { return p.S }
func (p *PatWild) Span() span.Span  { return p.S }
func (p *PatCon) Span() span.Span   { return p.S }
func (p *PatParen) Span() span.Span { return p.S }
