package syntax

import (
	"strconv"

	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
)

// Fixity holds operator precedence and associativity.
type Fixity struct {
	Assoc Assoc
	Prec  int
}

// Parser is a Pratt parser for the surface language.
type Parser struct {
	tokens []Token
	pos    int
	fixity map[string]Fixity
	errors *errs.Errors
	depth  int // paren/brace nesting depth
}

// NewParser creates a parser from a token stream.
func NewParser(tokens []Token, errors *errs.Errors) *Parser {
	return &Parser{
		tokens: tokens,
		pos:    0,
		fixity: make(map[string]Fixity),
		errors: errors,
	}
}

// ParseProgram parses a complete program.
func (p *Parser) ParseProgram() *AstProgram {
	// First pass: collect fixity declarations.
	p.collectFixity()
	p.pos = 0

	var decls []Decl
	for p.peek().Kind != TokEOF {
		d := p.parseDecl()
		if d != nil {
			decls = append(decls, d)
		}
	}
	return &AstProgram{Decls: decls}
}

// ParseExpr parses a single expression.
func (p *Parser) ParseExpr() Expr {
	return p.parseExpr()
}

// ParseType parses a type expression.
func (p *Parser) ParseType() TypeExpr {
	return p.parseType()
}

// --- Declarations ---

func (p *Parser) collectFixity() {
	saved := p.pos
	for p.peek().Kind != TokEOF {
		if p.isFixityKeyword() {
			d := p.parseFixityDecl()
			if d != nil {
				p.fixity[d.Op] = Fixity{Assoc: d.Assoc, Prec: d.Prec}
			}
		} else {
			p.advance()
		}
	}
	p.pos = saved
}

func (p *Parser) parseDecl() Decl {
	switch {
	case p.peek().Kind == TokData:
		return p.parseDataDecl()
	case p.peek().Kind == TokType:
		return p.parseTypeAlias()
	case p.peek().Kind == TokClass:
		return p.parseClassDecl()
	case p.peek().Kind == TokInstance:
		return p.parseInstanceDecl()
	case p.isFixityKeyword():
		d := p.parseFixityDecl()
		return d
	case p.peek().Kind == TokLower:
		return p.parseNamedDecl()
	default:
		p.addError("expected declaration")
		p.advance()
		return nil
	}
}

func (p *Parser) parseDataDecl() *DeclData {
	start := p.peek().S.Start
	p.expect(TokData)
	name := p.expectUpper()
	var params []TyBinder
	for p.peek().Kind == TokLower {
		pName := p.peek().Text
		pS := p.peek().S
		p.advance()
		params = append(params, TyBinder{Name: pName, S: pS})
	}
	p.expect(TokEq)
	var cons []DeclCon
	cons = append(cons, p.parseConDecl())
	for p.peek().Kind == TokPipe {
		p.advance()
		cons = append(cons, p.parseConDecl())
	}
	return &DeclData{
		Name:   name,
		Params: params,
		Cons:   cons,
		S:      span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseConDecl() DeclCon {
	start := p.peek().S.Start
	name := p.expectUpper()
	var fields []TypeExpr
	for p.isTypeAtomStart() && !p.atDeclBoundary() {
		fields = append(fields, p.parseTypeAtom())
	}
	return DeclCon{Name: name, Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
}

func (p *Parser) parseTypeAlias() *DeclTypeAlias {
	start := p.peek().S.Start
	p.expect(TokType)
	name := p.expectUpper()
	var params []TyBinder
	for p.peek().Kind == TokLower {
		pName := p.peek().Text
		pS := p.peek().S
		p.advance()
		params = append(params, TyBinder{Name: pName, S: pS})
	}
	p.expect(TokEq)
	body := p.parseType()
	return &DeclTypeAlias{
		Name:   name,
		Params: params,
		Body:   body,
		S:      span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseFixityDecl() *DeclFixity {
	start := p.peek().S.Start
	var assoc Assoc
	switch p.peek().Kind {
	case TokInfixl:
		assoc = AssocLeft
	case TokInfixr:
		assoc = AssocRight
	case TokInfixn:
		assoc = AssocNone
	}
	p.advance()
	prec := 9
	if p.peek().Kind == TokIntLit {
		n, err := strconv.Atoi(p.peek().Text)
		if err == nil {
			prec = n
		}
		p.advance()
	}
	var op string
	if p.peek().Kind == TokOp {
		op = p.peek().Text
		p.advance()
	} else {
		p.addError("expected operator in fixity declaration")
		return nil
	}
	return &DeclFixity{
		Assoc: assoc, Prec: prec, Op: op,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseClassDecl parses: class [Constraint =>] ClassName params { method :: Type; ... }
func (p *Parser) parseClassDecl() *DeclClass {
	start := p.peek().S.Start
	p.expect(TokClass)

	// Parse the head: either "ClassName params" or "Constraint => ClassName params".
	// Strategy: parse type applications, check for => to detect superclass.
	var supers []TypeExpr
	firstName := p.expectUpper()
	var firstArgs []TypeExpr
	for p.peek().Kind == TokLower {
		tok := p.peek()
		p.advance()
		firstArgs = append(firstArgs, &TyExprVar{Name: tok.Text, S: tok.S})
	}

	if p.peek().Kind == TokFatArrow {
		// What we parsed is a superclass constraint.
		var superExpr TypeExpr = &TyExprCon{Name: firstName, S: span.Span{Start: start, End: p.prevEnd()}}
		for _, arg := range firstArgs {
			superExpr = &TyExprApp{Fun: superExpr, Arg: arg, S: span.Span{Start: start, End: arg.Span().End}}
		}
		supers = append(supers, superExpr)
		p.advance() // consume =>

		// Now parse the actual class name and params.
		className := p.expectUpper()
		var params []TyBinder
		for p.peek().Kind == TokLower {
			pName := p.peek().Text
			pS := p.peek().S
			p.advance()
			params = append(params, TyBinder{Name: pName, S: pS})
		}
		// Parse methods block.
		methods := p.parseClassMethods()
		return &DeclClass{
			Supers: supers, Name: className, TyParams: params, Methods: methods,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// No =>, so firstName is the class name, firstArgs are params.
	var params []TyBinder
	for _, arg := range firstArgs {
		v := arg.(*TyExprVar)
		params = append(params, TyBinder{Name: v.Name, S: v.S})
	}
	methods := p.parseClassMethods()
	return &DeclClass{
		Name: firstName, TyParams: params, Methods: methods,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseClassMethods() []ClassMethod {
	p.expect(TokLBrace)
	var methods []ClassMethod
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		mStart := p.peek().S.Start
		name := p.expectLower()
		p.expect(TokColonColon)
		ty := p.parseType()
		methods = append(methods, ClassMethod{Name: name, Type: ty, S: span.Span{Start: mStart, End: p.prevEnd()}})
		if p.peek().Kind == TokSemicolon {
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return methods
}

// parseInstanceDecl parses: instance [Constraint =>] ClassName types { method := expr; ... }
func (p *Parser) parseInstanceDecl() *DeclInstance {
	start := p.peek().S.Start
	p.expect(TokInstance)

	// Parse: either "ClassName types" or "Constraint => ClassName types".
	// Strategy: parse type applications. If => follows, first part is context.
	var context []TypeExpr

	firstName := p.expectUpper()
	var firstArgs []TypeExpr
	for p.isTypeAtomStart() && p.peek().Kind != TokLBrace && !p.atDeclBoundary() {
		firstArgs = append(firstArgs, p.parseTypeAtom())
	}

	if p.peek().Kind == TokFatArrow {
		// What we parsed is a context constraint.
		var ctxExpr TypeExpr = &TyExprCon{Name: firstName, S: span.Span{Start: start, End: p.prevEnd()}}
		for _, arg := range firstArgs {
			ctxExpr = &TyExprApp{Fun: ctxExpr, Arg: arg, S: span.Span{Start: start, End: arg.Span().End}}
		}
		context = append(context, ctxExpr)
		p.advance() // consume =>

		// Parse the actual class name and type args.
		className := p.expectUpper()
		var typeArgs []TypeExpr
		for p.isTypeAtomStart() && p.peek().Kind != TokLBrace && !p.atDeclBoundary() {
			typeArgs = append(typeArgs, p.parseTypeAtom())
		}
		methods := p.parseInstMethods()
		return &DeclInstance{
			Context: context, ClassName: className, TypeArgs: typeArgs, Methods: methods,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// No =>, firstName is the class name.
	methods := p.parseInstMethods()
	return &DeclInstance{
		ClassName: firstName, TypeArgs: firstArgs, Methods: methods,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseInstMethods() []InstMethod {
	p.expect(TokLBrace)
	var methods []InstMethod
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		mStart := p.peek().S.Start
		name := p.expectLower()
		p.expect(TokColonEq)
		expr := p.parseExpr()
		methods = append(methods, InstMethod{Name: name, Expr: expr, S: span.Span{Start: mStart, End: p.prevEnd()}})
		if p.peek().Kind == TokSemicolon {
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return methods
}

func (p *Parser) parseNamedDecl() Decl {
	start := p.peek().S.Start
	name := p.peek().Text
	p.advance()

	switch p.peek().Kind {
	case TokColonColon:
		// Type annotation: f :: T
		p.advance()
		ty := p.parseType()
		return &DeclTypeAnn{
			Name: name, Type: ty,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	case TokColonEq:
		// Value definition: f := e
		p.advance()
		expr := p.parseExpr()
		return &DeclValueDef{
			Name: name, Expr: expr,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	default:
		p.addError("expected :: or := after name")
		return nil
	}
}

// --- Expressions ---

func (p *Parser) parseExpr() Expr {
	return p.parseAnnotation()
}

func (p *Parser) parseAnnotation() Expr {
	e := p.parseInfix(0)
	if p.peek().Kind == TokColonColon {
		p.advance()
		ty := p.parseType()
		return &ExprAnn{
			Expr: e, AnnType: ty,
			S: span.Span{Start: e.Span().Start, End: p.prevEnd()},
		}
	}
	return e
}

func (p *Parser) parseInfix(minPrec int) Expr {
	left := p.parseApp()
	for p.peek().Kind == TokOp {
		op := p.peek().Text
		fix := p.lookupFixity(op)
		if fix.Prec < minPrec {
			break
		}
		p.advance()
		nextMin := fix.Prec + 1
		if fix.Assoc == AssocRight {
			nextMin = fix.Prec
		}
		right := p.parseInfix(nextMin)
		left = &ExprInfix{
			Left: left, Op: op, Right: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}
	return left
}

func (p *Parser) parseApp() Expr {
	f := p.parseAtom()
	if f == nil {
		return &ExprVar{Name: "<error>", S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	for (p.isAtomStart() || p.peek().Kind == TokAt) && !p.atDeclBoundary() {
		if p.peek().Kind == TokAt {
			p.advance()
			ty := p.parseTypeAtom()
			f = &ExprTyApp{
				Expr: f, TyArg: ty,
				S: span.Span{Start: f.Span().Start, End: p.prevEnd()},
			}
		} else {
			arg := p.parseAtom()
			f = &ExprApp{
				Fun: f, Arg: arg,
				S: span.Span{Start: f.Span().Start, End: arg.Span().End},
			}
		}
	}
	return f
}

func (p *Parser) parseAtom() Expr {
	switch p.peek().Kind {
	case TokLower:
		tok := p.peek()
		p.advance()
		return &ExprVar{Name: tok.Text, S: tok.S}
	case TokUpper:
		tok := p.peek()
		p.advance()
		return &ExprCon{Name: tok.Text, S: tok.S}
	case TokLParen:
		return p.parseParen()
	case TokBackslash:
		return p.parseLambda()
	case TokCase:
		return p.parseCase()
	case TokDo:
		return p.parseDo()
	case TokLBrace:
		return p.parseBlock()
	default:
		return nil
	}
}

func (p *Parser) parseParen() Expr {
	start := p.peek().S.Start
	p.expect(TokLParen)
	e := p.parseExpr()
	p.expect(TokRParen)
	return &ExprParen{Inner: e, S: span.Span{Start: start, End: p.prevEnd()}}
}

func (p *Parser) parseLambda() Expr {
	start := p.peek().S.Start
	p.expect(TokBackslash)
	var params []Pattern
	for p.peek().Kind != TokArrow && p.peek().Kind != TokEOF {
		params = append(params, p.parsePattern())
	}
	p.expect(TokArrow)
	body := p.parseExpr()
	return &ExprLam{
		Params: params, Body: body,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseCase() Expr {
	start := p.peek().S.Start
	p.expect(TokCase)
	scrut := p.parseExpr()
	p.expect(TokOf)
	p.expect(TokLBrace)
	var alts []AstAlt
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		alt := p.parseAlt()
		alts = append(alts, alt)
		if p.peek().Kind == TokSemicolon {
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return &ExprCase{
		Scrutinee: scrut, Alts: alts,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseAlt() AstAlt {
	start := p.peek().S.Start
	pat := p.parsePattern()
	p.expect(TokArrow)
	body := p.parseExpr()
	return AstAlt{
		Pattern: pat, Body: body,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseDo() Expr {
	start := p.peek().S.Start
	p.expect(TokDo)
	p.expect(TokLBrace)
	var stmts []Stmt
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		stmt := p.parseStmt()
		stmts = append(stmts, stmt)
		if p.peek().Kind == TokSemicolon {
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return &ExprDo{
		Stmts: stmts,
		S:     span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseStmt() Stmt {
	start := p.peek().S.Start
	// Try: _ <- expr (wildcard bind)
	if p.peek().Kind == TokUnderscore {
		saved := p.pos
		p.advance()
		if p.peek().Kind == TokLArrow {
			p.advance()
			comp := p.parseExpr()
			return &StmtBind{
				Var: "_", Comp: comp,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		// Backtrack — it's an expression.
		p.pos = saved
	}
	// Try: name <- expr  or  name := expr
	if p.peek().Kind == TokLower {
		name := p.peek().Text
		saved := p.pos
		p.advance()
		if p.peek().Kind == TokLArrow {
			p.advance()
			comp := p.parseExpr()
			return &StmtBind{
				Var: name, Comp: comp,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		if p.peek().Kind == TokColonEq {
			p.advance()
			expr := p.parseExpr()
			return &StmtPureBind{
				Var: name, Expr: expr,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		// Backtrack — it's an expression statement.
		p.pos = saved
	}
	expr := p.parseExpr()
	return &StmtExpr{
		Expr: expr,
		S:    span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseBlock() Expr {
	start := p.peek().S.Start
	p.expect(TokLBrace)

	// Try parsing as block expression with bindings.
	var binds []AstBind
	for p.peek().Kind == TokLower {
		saved := p.pos
		name := p.peek().Text
		p.advance()
		if p.peek().Kind == TokColonEq {
			p.advance()
			expr := p.parseExpr()
			binds = append(binds, AstBind{
				Var: name, Expr: expr,
				S: span.Span{Start: span.Pos(saved), End: p.prevEnd()},
			})
			if p.peek().Kind == TokSemicolon {
				p.advance()
			}
		} else {
			// Not a binding — backtrack and parse as expression.
			p.pos = saved
			break
		}
	}

	body := p.parseExpr()
	p.expect(TokRBrace)
	return &ExprBlock{
		Binds: binds, Body: body,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// --- Patterns ---

func (p *Parser) parsePattern() Pattern {
	switch p.peek().Kind {
	case TokUpper:
		return p.parseConPattern()
	case TokLower:
		tok := p.peek()
		p.advance()
		return &PatVar{Name: tok.Text, S: tok.S}
	case TokUnderscore:
		tok := p.peek()
		p.advance()
		return &PatWild{S: tok.S}
	case TokLParen:
		start := p.peek().S.Start
		p.advance()
		inner := p.parsePattern()
		p.expect(TokRParen)
		return &PatParen{Inner: inner, S: span.Span{Start: start, End: p.prevEnd()}}
	default:
		p.addError("expected pattern")
		tok := p.peek()
		p.advance()
		return &PatWild{S: tok.S}
	}
}

func (p *Parser) parseConPattern() Pattern {
	start := p.peek().S.Start
	name := p.expectUpper()
	var args []Pattern
	for p.isPatternAtomStart() {
		args = append(args, p.parsePatternAtom())
	}
	if len(args) == 0 {
		return &PatCon{Con: name, S: span.Span{Start: start, End: p.prevEnd()}}
	}
	return &PatCon{Con: name, Args: args, S: span.Span{Start: start, End: p.prevEnd()}}
}

func (p *Parser) parsePatternAtom() Pattern {
	switch p.peek().Kind {
	case TokLower:
		tok := p.peek()
		p.advance()
		return &PatVar{Name: tok.Text, S: tok.S}
	case TokUnderscore:
		tok := p.peek()
		p.advance()
		return &PatWild{S: tok.S}
	case TokLParen:
		start := p.peek().S.Start
		p.advance()
		inner := p.parsePattern()
		p.expect(TokRParen)
		return &PatParen{Inner: inner, S: span.Span{Start: start, End: p.prevEnd()}}
	default:
		return nil
	}
}

func (p *Parser) isPatternAtomStart() bool {
	if p.atDeclBoundary() {
		return false
	}
	k := p.peek().Kind
	return k == TokLower || k == TokUnderscore || k == TokLParen
}

// --- Type expressions ---

func (p *Parser) parseType() TypeExpr {
	return p.parseTypeArrow()
}

func (p *Parser) parseTypeArrow() TypeExpr {
	if p.peek().Kind == TokForall {
		return p.parseForallType()
	}
	left := p.parseTypeApp()
	if p.peek().Kind == TokFatArrow {
		p.advance()
		body := p.parseTypeArrow() // right-associative
		return &TyExprQual{
			Constraint: left, Body: body,
			S: span.Span{Start: left.Span().Start, End: body.Span().End},
		}
	}
	if p.peek().Kind == TokArrow {
		p.advance()
		right := p.parseTypeArrow() // right-associative
		return &TyExprArrow{
			From: left, To: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}
	return left
}

func (p *Parser) parseForallType() TypeExpr {
	start := p.peek().S.Start
	p.expect(TokForall)
	var binders []TyBinder
	for p.peek().Kind == TokLower || p.peek().Kind == TokLParen {
		if p.peek().Kind == TokLParen {
			// Kinded binder: (v : Kind)
			lp := p.peek().S.Start
			p.advance()
			name := p.expectLower()
			p.expect(TokColon)
			kind := p.parseKindExpr()
			p.expect(TokRParen)
			binders = append(binders, TyBinder{
				Name: name,
				Kind: kind,
				S:    span.Span{Start: lp, End: p.prevEnd()},
			})
		} else {
			// Bare type variable (kind inferred)
			b := TyBinder{Name: p.peek().Text, S: p.peek().S}
			p.advance()
			binders = append(binders, b)
		}
	}
	p.expect(TokDot)
	body := p.parseType()
	return &TyExprForall{
		Binders: binders, Body: body,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseKindExpr parses a kind expression: Type, Row, or K1 -> K2.
func (p *Parser) parseKindExpr() KindExpr {
	left := p.parseKindAtom()
	if p.peek().Kind == TokArrow {
		p.advance()
		right := p.parseKindExpr() // right-associative
		return &KindExprArrow{
			From: left, To: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}
	return left
}

// parseKindAtom parses an atomic kind: Type, Row, or (K).
func (p *Parser) parseKindAtom() KindExpr {
	switch {
	case p.peek().Kind == TokUpper && p.peek().Text == "Type":
		tok := p.peek()
		p.advance()
		return &KindExprType{S: tok.S}
	case p.peek().Kind == TokUpper && p.peek().Text == "Row":
		tok := p.peek()
		p.advance()
		return &KindExprRow{S: tok.S}
	case p.peek().Kind == TokUpper && p.peek().Text == "Constraint":
		tok := p.peek()
		p.advance()
		return &KindExprConstraint{S: tok.S}
	case p.peek().Kind == TokLParen:
		p.advance()
		k := p.parseKindExpr()
		p.expect(TokRParen)
		return k
	default:
		p.addError("expected kind (Type, Row, or K -> K)")
		tok := p.peek()
		p.advance()
		return &KindExprType{S: tok.S}
	}
}

func (p *Parser) parseTypeApp() TypeExpr {
	f := p.parseTypeAtom()
	for p.isTypeAtomStart() {
		arg := p.parseTypeAtom()
		f = &TyExprApp{
			Fun: f, Arg: arg,
			S: span.Span{Start: f.Span().Start, End: arg.Span().End},
		}
	}
	return f
}

func (p *Parser) parseTypeAtom() TypeExpr {
	switch p.peek().Kind {
	case TokLower:
		tok := p.peek()
		p.advance()
		return &TyExprVar{Name: tok.Text, S: tok.S}
	case TokUpper:
		tok := p.peek()
		p.advance()
		return &TyExprCon{Name: tok.Text, S: tok.S}
	case TokLParen:
		start := p.peek().S.Start
		p.advance()
		ty := p.parseType()
		p.expect(TokRParen)
		return &TyExprParen{Inner: ty, S: span.Span{Start: start, End: p.prevEnd()}}
	case TokLBrace:
		return p.parseRowType()
	default:
		p.addError("expected type")
		tok := p.peek()
		p.advance()
		return &TyExprCon{Name: "<error>", S: tok.S}
	}
}

func (p *Parser) parseRowType() TypeExpr {
	start := p.peek().S.Start
	p.expect(TokLBrace)
	if p.peek().Kind == TokRBrace {
		p.advance()
		return &TyExprRow{S: span.Span{Start: start, End: p.prevEnd()}}
	}
	var fields []TyRowField
	var tail *TyExprVar

	for {
		if p.peek().Kind == TokPipe {
			p.advance()
			if p.peek().Kind == TokLower {
				tok := p.peek()
				tail = &TyExprVar{Name: tok.Text, S: tok.S}
				p.advance()
			}
			break
		}
		if p.peek().Kind == TokRBrace || p.peek().Kind == TokEOF {
			break
		}
		label := p.expectLower()
		p.expect(TokColon)
		ty := p.parseType()
		fields = append(fields, TyRowField{
			Label: label, Type: ty,
			S: span.Span{Start: span.Pos(p.pos), End: p.prevEnd()},
		})
		if p.peek().Kind == TokComma {
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return &TyExprRow{
		Fields: fields, Tail: tail,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// --- Helpers ---

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Kind: TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	switch tok.Kind {
	case TokLParen, TokLBrace:
		p.depth++
	case TokRParen, TokRBrace:
		if p.depth > 0 {
			p.depth--
		}
	}
	return tok
}

func (p *Parser) expect(kind TokenKind) Token {
	tok := p.peek()
	if tok.Kind != kind {
		p.addError("expected " + kind.String())
		return tok
	}
	return p.advance()
}

func (p *Parser) expectUpper() string {
	tok := p.peek()
	if tok.Kind != TokUpper {
		p.addError("expected uppercase identifier")
		return "<error>"
	}
	p.advance()
	return tok.Text
}

func (p *Parser) expectLower() string {
	tok := p.peek()
	if tok.Kind != TokLower {
		p.addError("expected identifier")
		return "<error>"
	}
	p.advance()
	return tok.Text
}

func (p *Parser) prevEnd() span.Pos {
	if p.pos > 0 && p.pos <= len(p.tokens) {
		return p.tokens[p.pos-1].S.End
	}
	return span.Pos(p.pos)
}

// atDeclBoundary returns true if the next token is across a newline boundary
// at the top level and could start a new declaration. This prevents greedy
// consumption of tokens that belong to subsequent declarations.
func (p *Parser) atDeclBoundary() bool {
	if p.depth > 0 {
		return false
	}
	tok := p.peek()
	if !tok.NewlineBefore {
		return false
	}
	switch tok.Kind {
	case TokLower, TokUpper, TokData, TokType, TokInfixl, TokInfixr, TokInfixn, TokClass, TokInstance:
		return true
	}
	return false
}

func (p *Parser) isAtomStart() bool {
	if p.atDeclBoundary() {
		return false
	}
	k := p.peek().Kind
	return k == TokLower || k == TokUpper || k == TokLParen || k == TokBackslash || k == TokLBrace || k == TokCase || k == TokDo
}

func (p *Parser) isTypeAtomStart() bool {
	if p.atDeclBoundary() {
		return false
	}
	k := p.peek().Kind
	return k == TokLower || k == TokUpper || k == TokLParen || k == TokLBrace
}

func (p *Parser) isFixityKeyword() bool {
	k := p.peek().Kind
	return k == TokInfixl || k == TokInfixr || k == TokInfixn
}

func (p *Parser) lookupFixity(op string) Fixity {
	if f, ok := p.fixity[op]; ok {
		return f
	}
	return Fixity{Assoc: AssocLeft, Prec: 9} // default
}

func (p *Parser) addError(msg string) {
	tok := p.peek()
	p.errors.Add(&errs.Error{
		Code:    100,
		Phase:   errs.PhaseParse,
		Span:    tok.S,
		Message: msg,
	})
}
