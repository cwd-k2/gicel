package syntax

import "github.com/cwd-k2/gicel/internal/infra/span"

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
	IfDesugar bool // true when desugared from if-then-else
}

type ExprDo struct {
	Stmts []Stmt
	S     span.Span
}

type ExprBlock struct {
	Binds    []AstBind
	TypeDefs []ImplField // associated type/form definitions (in impl bodies)
	Body     Expr
	S        span.Span
}

type ExprInfix struct {
	Left   Expr
	Op     string
	OpSpan span.Span // source span of the operator token
	Right  Expr
	S      span.Span
}

// ExprInfixSpine is a flat sequence of operands interleaved with operators.
// Produced during parsing when fixity is not yet known; resolved to nested
// ExprInfix trees by post-parse fixity resolution.
// Invariant: len(Operands) == len(Ops) + 1
type ExprInfixSpine struct {
	Operands []Expr
	Ops      []string
	OpSpans  []span.Span
	S        span.Span
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
	OpSpan  span.Span // source span of the operator token
	Arg     Expr
	IsRight bool
	S       span.Span
}

// ExprEvidence is a scoped evidence injection: value => expr.
// The Dict expression is evaluated to a dictionary value, which is then
// injected into the local context for resolution during checking of Body.
type ExprEvidence struct {
	Dict Expr // dictionary/evidence value
	Body Expr // expression checked with Dict in scope
	S    span.Span
}

// ExprIf is a surface-level if-then-else. Desugared to ExprCase
// over True/False by the desugar pass. Preserves the original syntax
// for error messages and LSP hover.
type ExprIf struct {
	Cond Expr
	Then Expr
	Else Expr
	S    span.Span
}

// ExprNegate is a surface-level unary minus: -e. Desugared to
// ExprApp(ExprVar("negate"), e) by the desugar pass.
type ExprNegate struct {
	Expr Expr
	S    span.Span
}

// ExprTuple is a surface-level tuple: (a, b, c). Desugared to
// ExprRecord with _1, _2, ... labels by the desugar pass.
type ExprTuple struct {
	Elems []Expr
	S     span.Span
}

// ExprError represents a parse error placeholder.
// The checker should skip type checking and return an error type/core immediately.
type ExprError struct {
	S span.Span
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
func (*ExprInfixSpine) exprNode()   {}
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
func (*ExprEvidence) exprNode()     {}
func (*ExprIf) exprNode()           {}
func (*ExprNegate) exprNode()       {}
func (*ExprTuple) exprNode()        {}
func (*ExprError) exprNode()        {}

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
func (e *ExprInfixSpine) Span() span.Span   { return e.S }
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
func (e *ExprEvidence) Span() span.Span     { return e.S }
func (e *ExprIf) Span() span.Span           { return e.S }
func (e *ExprNegate) Span() span.Span       { return e.S }
func (e *ExprTuple) Span() span.Span        { return e.S }
func (e *ExprError) Span() span.Span        { return e.S }

// ---- Statements (in do blocks) ----

type Stmt interface {
	stmtNode()
	Span() span.Span
}

type StmtBind struct {
	Pat  Pattern
	Comp Expr
	S    span.Span
}

type StmtPureBind struct {
	Pat  Pattern
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
	Pat  Pattern
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
	Name      string
	Generated bool // true when introduced by compiler desugar (e.g. section parameters)
	S         span.Span
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

// PatList is a list literal pattern: [p1, p2, p3]
// Desugared to Cons p1 (Cons p2 (Cons p3 Nil)) during type checking.
type PatList struct {
	Elems []Pattern
	S     span.Span
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
func (*PatList) patternNode()    {}

func (p *PatVar) Span() span.Span     { return p.S }
func (p *PatWild) Span() span.Span    { return p.S }
func (p *PatCon) Span() span.Span     { return p.S }
func (p *PatQualCon) Span() span.Span { return p.S }
func (p *PatRecord) Span() span.Span  { return p.S }
func (p *PatParen) Span() span.Span   { return p.S }
func (p *PatLit) Span() span.Span     { return p.S }
func (p *PatList) Span() span.Span    { return p.S }

// PatVarName extracts the variable name from a simple pattern.
// Returns the name and true for PatVar, "_" and true for PatWild,
// or "" and false for complex patterns.
func PatVarName(pat Pattern) (string, bool) {
	switch p := pat.(type) {
	case *PatVar:
		return p.Name, true
	case *PatWild:
		return "_", true
	case *PatParen:
		return PatVarName(p.Inner)
	default:
		return "", false
	}
}

// IsIrrefutable returns true if the pattern always matches any value
// of its type. Only irrefutable patterns are allowed in binding positions.
func IsIrrefutable(pat Pattern) bool {
	switch p := pat.(type) {
	case *PatVar:
		return true
	case *PatWild:
		return true
	case *PatParen:
		return IsIrrefutable(p.Inner)
	case *PatRecord:
		for _, f := range p.Fields {
			if !IsIrrefutable(f.Pattern) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
