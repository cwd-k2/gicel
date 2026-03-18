package syntax

import "github.com/cwd-k2/gicel/internal/span"

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

// ExprQualVar is a qualified variable reference: N.add
type ExprQualVar struct {
	Qualifier string
	Name      string
	S         span.Span
}

// ExprQualCon is a qualified constructor reference: N.Just
type ExprQualCon struct {
	Qualifier string
	Name      string
	S         span.Span
}

type ExprApp struct {
	Fun Expr
	Arg Expr
	S   span.Span
}

type ExprTyApp struct {
	Expr  Expr
	TyArg TypeExpr
	S     span.Span
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

type ExprIntLit struct {
	Value string // raw text "42"
	S     span.Span
}

type ExprStrLit struct {
	Value string // decoded string (escapes resolved by lexer)
	S     span.Span
}

type ExprRuneLit struct {
	Value rune
	S     span.Span
}

type ExprDoubleLit struct {
	Value string // raw text "3.14", "1e10"
	S     span.Span
}

type ExprList struct {
	Elems []Expr
	S     span.Span
}

// ExprRecord is a record literal: { x: 1, y: True }
type ExprRecord struct {
	Fields []RecordField
	S      span.Span
}

// RecordField is a field in a record literal or update.
type RecordField struct {
	Label string
	Value Expr
	S     span.Span
}

// ExprRecordUpdate is a record update: { r | x: 1 }
type ExprRecordUpdate struct {
	Record  Expr
	Updates []RecordField
	S       span.Span
}

// ExprProject is a record projection: r.#x
type ExprProject struct {
	Record Expr
	Label  string
	S      span.Span
}

// ExprSection is an operator section: (+ 1) or (1 +).
// IsRight=true → right section (+ 1) = \x. x + 1
// IsRight=false → left section (1 +) = \x. 1 + x
type ExprSection struct {
	Op      string
	Arg     Expr
	IsRight bool
	S       span.Span
}

func (*ExprVar) exprNode()          {}
func (*ExprCon) exprNode()          {}
func (*ExprQualVar) exprNode()      {}
func (*ExprQualCon) exprNode()      {}
func (*ExprApp) exprNode()          {}
func (*ExprTyApp) exprNode()        {}
func (*ExprLam) exprNode()          {}
func (*ExprCase) exprNode()         {}
func (*ExprDo) exprNode()           {}
func (*ExprBlock) exprNode()        {}
func (*ExprInfix) exprNode()        {}
func (*ExprAnn) exprNode()          {}
func (*ExprParen) exprNode()        {}
func (*ExprIntLit) exprNode()       {}
func (*ExprStrLit) exprNode()       {}
func (*ExprRuneLit) exprNode()      {}
func (*ExprDoubleLit) exprNode()    {}
func (*ExprList) exprNode()         {}
func (*ExprRecord) exprNode()       {}
func (*ExprRecordUpdate) exprNode() {}
func (*ExprProject) exprNode()      {}
func (*ExprSection) exprNode()      {}

func (e *ExprVar) Span() span.Span          { return e.S }
func (e *ExprCon) Span() span.Span          { return e.S }
func (e *ExprQualVar) Span() span.Span      { return e.S }
func (e *ExprQualCon) Span() span.Span      { return e.S }
func (e *ExprApp) Span() span.Span          { return e.S }
func (e *ExprTyApp) Span() span.Span        { return e.S }
func (e *ExprLam) Span() span.Span          { return e.S }
func (e *ExprCase) Span() span.Span         { return e.S }
func (e *ExprDo) Span() span.Span           { return e.S }
func (e *ExprBlock) Span() span.Span        { return e.S }
func (e *ExprInfix) Span() span.Span        { return e.S }
func (e *ExprAnn) Span() span.Span          { return e.S }
func (e *ExprParen) Span() span.Span        { return e.S }
func (e *ExprIntLit) Span() span.Span       { return e.S }
func (e *ExprStrLit) Span() span.Span       { return e.S }
func (e *ExprRuneLit) Span() span.Span      { return e.S }
func (e *ExprDoubleLit) Span() span.Span    { return e.S }
func (e *ExprList) Span() span.Span         { return e.S }
func (e *ExprRecord) Span() span.Span       { return e.S }
func (e *ExprRecordUpdate) Span() span.Span { return e.S }
func (e *ExprProject) Span() span.Span      { return e.S }
func (e *ExprSection) Span() span.Span      { return e.S }

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

// PatQualCon is a qualified constructor pattern: Q.Con p1 ... pn
type PatQualCon struct {
	Qualifier string
	Con       string
	Args      []Pattern
	S         span.Span
}

type PatParen struct {
	Inner Pattern
	S     span.Span
}

// PatRecord is a record pattern: { x: a, y: b }
type PatRecord struct {
	Fields []PatRecordField
	S      span.Span
}

type PatRecordField struct {
	Label   string
	Pattern Pattern
	S       span.Span
}

// LitKind identifies the type of a literal pattern.
type LitKind int

const (
	LitInt    LitKind = iota // integer literal
	LitDouble                // double literal
	LitString                // string literal
	LitRune                  // rune literal
)

// PatLit is a literal pattern: 42, "hello", 'x'
type PatLit struct {
	Value string // raw text (for Int: "42", for String: "hello", for Rune: "x")
	Kind  LitKind
	S     span.Span
}

func (*PatVar) patternNode()     {}
func (*PatWild) patternNode()    {}
func (*PatCon) patternNode()     {}
func (*PatQualCon) patternNode() {}
func (*PatParen) patternNode()   {}
func (*PatRecord) patternNode()  {}
func (*PatLit) patternNode()     {}

func (p *PatVar) Span() span.Span     { return p.S }
func (p *PatWild) Span() span.Span    { return p.S }
func (p *PatCon) Span() span.Span     { return p.S }
func (p *PatQualCon) Span() span.Span { return p.S }
func (p *PatRecord) Span() span.Span  { return p.S }
func (p *PatParen) Span() span.Span   { return p.S }
func (p *PatLit) Span() span.Span     { return p.S }
