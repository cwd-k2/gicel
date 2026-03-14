package parse

import (
	"fmt"
	"strconv"

	. "github.com/cwd-k2/gomputation/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage

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
	tokens      []Token
	pos         int
	fixity      map[string]Fixity
	errors      *errs.Errors
	depth       int  // paren/brace nesting depth
	noBraceAtom bool // when true, { is not an atom start (inside case scrutinee)

	// Safety harness: prevent resource exhaustion on malformed input.
	recurseDepth    int // current recursion depth
	maxRecurseDepth int // limit (default 256)
	steps           int // advance() call count
	maxSteps        int // limit (default len(tokens) * 4)
	halted          bool
}

// NewParser creates a parser from a token stream.
func NewParser(tokens []Token, errors *errs.Errors) *Parser {
	maxSteps := len(tokens) * 4
	if maxSteps < 100 {
		maxSteps = 100
	}
	return &Parser{
		tokens:          tokens,
		pos:             0,
		fixity:          make(map[string]Fixity),
		errors:          errors,
		maxRecurseDepth: 256,
		maxSteps:        maxSteps,
	}
}

// AddFixity seeds the parser with additional fixity declarations (e.g. from imported modules).
func (p *Parser) AddFixity(fixity map[string]Fixity) {
	for op, f := range fixity {
		if _, exists := p.fixity[op]; !exists {
			p.fixity[op] = f
		}
	}
}

// ParseProgram parses a complete program.
func (p *Parser) ParseProgram() *AstProgram {
	// First pass: collect fixity declarations.
	p.collectFixity()
	p.pos = 0
	p.steps = 0
	p.halted = false

	// Parse imports first.
	var imports []DeclImport
	p.skipSemicolons()
	for p.peek().Kind == TokImport {
		imports = append(imports, p.parseImportDecl())
		p.skipSemicolons()
	}

	var decls []Decl
	for p.peek().Kind != TokEOF {
		d := p.parseDecl()
		if d != nil {
			decls = append(decls, d)
		}
		p.skipSemicolons()
	}
	return &AstProgram{Imports: imports, Decls: decls}
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
	case p.peek().Kind == TokLParen && p.isOperatorDeclStart():
		return p.parseOperatorDecl()
	case p.peek().Kind == TokLower:
		return p.parseNamedDecl()
	default:
		p.addError("expected declaration")
		p.advance()
		return nil
	}
}

func (p *Parser) parseImportDecl() DeclImport {
	start := p.peek().S.Start
	p.expect(TokImport)
	modName := p.expectUpper()
	// Support dotted module names: Std.Num, Std.Str, etc.
	for p.peek().Kind == TokDot {
		p.advance()
		part := p.expectUpper()
		modName = modName + "." + part
	}
	return DeclImport{
		ModuleName: modName,
		S:          span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseDataDecl() *DeclData {
	start := p.peek().S.Start
	p.expect(TokData)
	name := p.expectUpper()
	var params []TyBinder
	for p.peek().Kind == TokLower || p.peek().Kind == TokLParen {
		if p.peek().Kind == TokLParen {
			// Kinded param: (name : Kind)
			lp := p.peek().S.Start
			p.advance()
			pName := p.expectLower()
			p.expect(TokColon)
			kind := p.parseKindExpr()
			p.expect(TokRParen)
			params = append(params, TyBinder{
				Name: pName,
				Kind: kind,
				S:    span.Span{Start: lp, End: p.prevEnd()},
			})
		} else {
			pName := p.peek().Text
			pS := p.peek().S
			p.advance()
			params = append(params, TyBinder{Name: pName, S: pS})
		}
	}
	p.expect(TokEq)

	// GADT mode: `= { ConName :: Type; ... }`
	if p.peek().Kind == TokLBrace {
		gadtCons := p.parseGADTCons()
		return &DeclData{
			Name:     name,
			Params:   params,
			GADTCons: gadtCons,
			S:        span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// ADT mode: `= Con1 fields | Con2 fields`
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

// parseGADTCons parses GADT constructor declarations inside braces.
// Format: { ConName :: Type ; ConName :: Type ; ... }
func (p *Parser) parseGADTCons() []GADTConDecl {
	p.expect(TokLBrace)
	var cons []GADTConDecl
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		before := p.pos
		cStart := p.peek().S.Start
		cName := p.expectUpper()
		p.expect(TokColonColon)
		cType := p.parseType()
		cons = append(cons, GADTConDecl{
			Name: cName,
			Type: cType,
			S:    span.Span{Start: cStart, End: p.prevEnd()},
		})
		if p.peek().Kind == TokSemicolon {
			p.advance()
		} else if p.pos == before {
			p.addError("unexpected token in GADT declaration")
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return cons
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
	for p.peek().Kind == TokLower || (p.peek().Kind == TokLParen && p.isClassKindedBinder()) {
		if p.peek().Kind == TokLParen {
			lp := p.peek().S.Start
			p.advance()
			pName := p.expectLower()
			p.expect(TokColon)
			kind := p.parseKindExpr()
			p.expect(TokRParen)
			params = append(params, TyBinder{Name: pName, Kind: kind, S: span.Span{Start: lp, End: p.prevEnd()}})
		} else {
			pName := p.peek().Text
			pS := p.peek().S
			p.advance()
			params = append(params, TyBinder{Name: pName, S: pS})
		}
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
	} else if p.peek().Kind == TokDot {
		op = "."
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
	// Parse class params: either bare lowercase vars or kinded binders (v : Kind).
	for p.peek().Kind == TokLower || (p.peek().Kind == TokLParen && p.isClassKindedBinder()) {
		if p.peek().Kind == TokLParen {
			// Kinded class param: (m : Kind)
			lp := p.peek().S.Start
			p.advance()
			name := p.expectLower()
			p.expect(TokColon)
			kind := p.parseKindExpr()
			p.expect(TokRParen)
			firstArgs = append(firstArgs, &TyExprVar{
				Name: name,
				S:    span.Span{Start: lp, End: p.prevEnd()},
				Kind: kind,
			})
		} else {
			tok := p.peek()
			p.advance()
			firstArgs = append(firstArgs, &TyExprVar{Name: tok.Text, S: tok.S})
		}
	}

	if p.peek().Kind == TokFatArrow {
		// What we parsed is a superclass constraint.
		var superExpr TypeExpr = &TyExprCon{Name: firstName, S: span.Span{Start: start, End: p.prevEnd()}}
		for _, arg := range firstArgs {
			superExpr = &TyExprApp{Fun: superExpr, Arg: arg, S: span.Span{Start: start, End: arg.Span().End}}
		}
		supers = append(supers, superExpr)
		p.advance() // consume =>

		// Support multiple superclass constraints: Super1 a => Super2 a => ... => ClassName params
		for {
			nextName := p.expectUpper()
			var nextArgs []TypeExpr
			for p.peek().Kind == TokLower || (p.peek().Kind == TokLParen && p.isClassKindedBinder()) {
				if p.peek().Kind == TokLParen {
					lp := p.peek().S.Start
					p.advance()
					name := p.expectLower()
					p.expect(TokColon)
					kind := p.parseKindExpr()
					p.expect(TokRParen)
					nextArgs = append(nextArgs, &TyExprVar{
						Name: name,
						S:    span.Span{Start: lp, End: p.prevEnd()},
						Kind: kind,
					})
				} else {
					tok := p.peek()
					p.advance()
					nextArgs = append(nextArgs, &TyExprVar{Name: tok.Text, S: tok.S})
				}
			}
			if p.peek().Kind == TokFatArrow {
				// Another superclass constraint.
				var nextExpr TypeExpr = &TyExprCon{Name: nextName, S: span.Span{Start: start, End: p.prevEnd()}}
				for _, arg := range nextArgs {
					nextExpr = &TyExprApp{Fun: nextExpr, Arg: arg, S: span.Span{Start: start, End: arg.Span().End}}
				}
				supers = append(supers, nextExpr)
				p.advance() // consume =>
				continue
			}
			// This is the actual class name.
			var params []TyBinder
			for _, arg := range nextArgs {
				v := arg.(*TyExprVar)
				params = append(params, TyBinder{Name: v.Name, Kind: v.Kind, S: v.S})
			}
			methods := p.parseClassMethods()
			return &DeclClass{
				Supers: supers, Name: nextName, TyParams: params, Methods: methods,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
	}

	// No =>, so firstName is the class name, firstArgs are params.
	var params []TyBinder
	for _, arg := range firstArgs {
		v := arg.(*TyExprVar)
		params = append(params, TyBinder{Name: v.Name, Kind: v.Kind, S: v.S})
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
		before := p.pos
		mStart := p.peek().S.Start
		name := p.expectLower()
		p.expect(TokColonColon)
		ty := p.parseType()
		methods = append(methods, ClassMethod{Name: name, Type: ty, S: span.Span{Start: mStart, End: p.prevEnd()}})
		if p.peek().Kind == TokSemicolon {
			p.advance()
		} else if p.pos == before {
			p.addError("unexpected token in class declaration")
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return methods
}

// parseInstanceDecl parses: instance [Constraint =>]* ClassName types { method := expr; ... }
// Supports curried constraints: instance Eq a => Eq b => Eq (Pair a b) { ... }
// Supports parenthesized constraints: instance (Eq a) => Eq (Maybe a) { ... }
func (p *Parser) parseInstanceDecl() *DeclInstance {
	start := p.peek().S.Start
	p.expect(TokInstance)

	var context []TypeExpr

	// Loop: accumulate constraints until we find the actual class head.
	for {
		// Parenthesized constraint: (Eq a) => ...
		if p.peek().Kind == TokLParen {
			saved := p.pos
			savedDepth := p.depth
			ty := p.parseTypeAtom() // parses (Eq a)
			if p.peek().Kind == TokFatArrow {
				if paren, ok := ty.(*TyExprParen); ok {
					context = append(context, paren.Inner)
				} else {
					context = append(context, ty)
				}
				p.advance() // consume =>
				continue
			}
			// Not a constraint — backtrack and parse as class TypeArgs
			p.pos = saved
			p.depth = savedDepth
			break
		}

		if p.peek().Kind != TokUpper {
			break
		}

		// Parse Upper + args, then check for =>
		nameStart := p.peek().S.Start
		firstName := p.peek().Text
		p.advance()

		var firstArgs []TypeExpr
		for p.isTypeAtomStart() && p.peek().Kind != TokLBrace && !p.atDeclBoundary() {
			firstArgs = append(firstArgs, p.parseTypeAtom())
		}

		if p.peek().Kind == TokFatArrow {
			// This is a context constraint.
			var ctxExpr TypeExpr = &TyExprCon{Name: firstName, S: span.Span{Start: nameStart, End: p.prevEnd()}}
			for _, arg := range firstArgs {
				ctxExpr = &TyExprApp{Fun: ctxExpr, Arg: arg, S: span.Span{Start: nameStart, End: arg.Span().End}}
			}
			context = append(context, ctxExpr)
			p.advance() // consume =>
			continue
		}

		// No =>, firstName IS the class name.
		methods := p.parseInstMethods()
		return &DeclInstance{
			Context: context, ClassName: firstName, TypeArgs: firstArgs, Methods: methods,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// Fallback: parse remaining class name + args
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

func (p *Parser) parseInstMethods() []InstMethod {
	p.expect(TokLBrace)
	var methods []InstMethod
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		mStart := p.peek().S.Start
		if p.peek().Kind != TokLower {
			// Skip unexpected token to avoid infinite loop.
			p.addError("expected method name in instance declaration")
			p.advance()
			continue
		}
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

func (p *Parser) isOperatorDeclStart() bool {
	// Check for (operator) pattern: ( op ) — TokDot also valid as operator.
	return p.pos+2 < len(p.tokens) &&
		(p.tokens[p.pos+1].Kind == TokOp || p.tokens[p.pos+1].Kind == TokDot) &&
		p.tokens[p.pos+2].Kind == TokRParen
}

func (p *Parser) parseOperatorDecl() Decl {
	start := p.expect(TokLParen)
	// Accept both TokOp and TokDot as operator names.
	var op Token
	if p.peek().Kind == TokDot {
		op = p.peek()
		op.Text = "."
		p.advance()
	} else {
		op = p.expect(TokOp)
	}
	p.expect(TokRParen)
	switch p.peek().Kind {
	case TokColonColon:
		p.advance()
		ty := p.parseType()
		return &DeclTypeAnn{Name: op.Text, Type: ty, S: span.Span{Start: start.S.Start, End: p.prevEnd()}}
	case TokColonEq:
		p.advance()
		expr := p.parseExpr()
		return &DeclValueDef{Name: op.Text, Expr: expr, S: span.Span{Start: start.S.Start, End: p.prevEnd()}}
	default:
		p.addError("expected :: or := after operator name")
		return nil
	}
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
	if !p.enterRecurse() {
		return &ExprVar{Name: "<error>", S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	defer p.leaveRecurse()
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
	for p.isInfixOp() {
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

// isInfixOp returns true if the current token can act as an infix operator.
// TokDot (.) is treated as an operator in expression context.
func (p *Parser) isInfixOp() bool {
	k := p.peek().Kind
	return k == TokOp || k == TokDot
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
	var e Expr
	switch p.peek().Kind {
	case TokLower:
		tok := p.peek()
		p.advance()
		e = &ExprVar{Name: tok.Text, S: tok.S}
	case TokUpper:
		tok := p.peek()
		p.advance()
		e = &ExprCon{Name: tok.Text, S: tok.S}
	case TokLParen:
		e = p.parseParen()
	case TokBackslash:
		e = p.parseLambda()
	case TokCase:
		e = p.parseCase()
	case TokDo:
		e = p.parseDo()
	case TokLBrace:
		e = p.parseBlock()
	case TokIntLit:
		tok := p.peek()
		p.advance()
		e = &ExprIntLit{Value: tok.Text, S: tok.S}
	case TokStrLit:
		tok := p.peek()
		p.advance()
		e = &ExprStrLit{Value: tok.Text, S: tok.S}
	case TokRuneLit:
		tok := p.peek()
		p.advance()
		runes := []rune(tok.Text)
		var r rune
		if len(runes) > 0 {
			r = runes[0]
		}
		e = &ExprRuneLit{Value: r, S: tok.S}
	case TokLBracket:
		e = p.parseListLit()
	default:
		return nil
	}
	// Chain record projections: r!#x!#y → Project(Project(r, "x"), "y")
	for p.peek().Kind == TokBangHash {
		p.advance()
		label := p.expectLower()
		e = &ExprProject{Record: e, Label: label, S: span.Span{Start: e.Span().Start, End: p.prevEnd()}}
	}
	return e
}

func (p *Parser) parseParen() Expr {
	start := p.peek().S.Start
	p.expect(TokLParen)

	// () → unit (empty record)
	if p.peek().Kind == TokRParen {
		p.advance()
		return &ExprRecord{S: span.Span{Start: start, End: p.prevEnd()}}
	}

	e := p.parseExpr()

	// (e) → grouping
	if p.peek().Kind == TokRParen {
		p.advance()
		return &ExprParen{Inner: e, S: span.Span{Start: start, End: p.prevEnd()}}
	}

	// (e1, e2, ...) → tuple (desugars to record with _1, _2, ...)
	if p.peek().Kind == TokComma {
		elems := []Expr{e}
		for p.peek().Kind == TokComma {
			p.advance()
			elems = append(elems, p.parseExpr())
		}
		p.expect(TokRParen)
		fields := make([]RecordField, len(elems))
		for i, el := range elems {
			fields[i] = RecordField{
				Label: fmt.Sprintf("_%d", i+1),
				Value: el,
				S:     el.Span(),
			}
		}
		return &ExprRecord{Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
	}

	p.expect(TokRParen)
	return &ExprParen{Inner: e, S: span.Span{Start: start, End: p.prevEnd()}}
}

func (p *Parser) parseLambda() Expr {
	start := p.peek().S.Start
	p.expect(TokBackslash)
	param := p.parsePattern()
	p.expect(TokArrow)
	body := p.parseExpr()
	return &ExprLam{
		Params: []Pattern{param}, Body: body,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseCase() Expr {
	start := p.peek().S.Start
	p.expect(TokCase)
	p.noBraceAtom = true
	scrut := p.parseExpr()
	p.noBraceAtom = false
	p.expect(TokLBrace)
	var alts []AstAlt
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		before := p.pos
		alt := p.parseAlt()
		alts = append(alts, alt)
		if p.peek().Kind == TokSemicolon {
			p.advance()
		} else if p.pos == before {
			p.addError("unexpected token in case expression")
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
		before := p.pos
		stmt := p.parseStmt()
		stmts = append(stmts, stmt)
		if p.peek().Kind == TokSemicolon {
			p.advance()
		} else if p.pos == before {
			p.addError("unexpected token in do-block")
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

	// {} → empty record
	if p.peek().Kind == TokRBrace {
		p.advance()
		return &ExprRecord{S: span.Span{Start: start, End: p.prevEnd()}}
	}

	// Disambiguate: record literal vs record update vs block.
	// Peek at first name followed by = (record) vs := (block).
	if p.peek().Kind == TokLower {
		saved := p.pos
		p.advance() // skip name
		switch p.peek().Kind {
		case TokEq:
			// name = ... → record literal
			p.pos = saved
			return p.parseRecordLiteral(start)
		case TokPipe:
			// name | ... → record update (name is the record expression)
			p.pos = saved
			return p.parseRecordUpdate(start)
		default:
			// Could be: name := ... (block), or name as expression (record update with complex expr)
			p.pos = saved
		}
	}

	// Check for record update: expr | field = val, ...
	// We need to detect this by trying to parse an expression, then checking for |.
	// For simplicity, handle the case where the record is a single variable or expression.
	// Attempt: if after parsing the first expression we see |, it's a record update.
	savedForUpdate := p.pos
	firstExpr := p.parseExpr()
	if p.peek().Kind == TokPipe {
		p.advance() // skip |
		return p.parseRecordUpdateFields(start, firstExpr)
	}
	// Not a record update — restore and parse as block.
	p.pos = savedForUpdate

	// Block expression with bindings.
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

func (p *Parser) parseRecordLiteral(start span.Pos) Expr {
	var fields []RecordField
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		fStart := p.peek().S.Start
		label := p.expectLower()
		p.expect(TokEq)
		value := p.parseExpr()
		fields = append(fields, RecordField{
			Label: label, Value: value,
			S: span.Span{Start: fStart, End: p.prevEnd()},
		})
		if p.peek().Kind == TokComma {
			p.advance()
		} else {
			break
		}
	}
	p.expect(TokRBrace)
	return &ExprRecord{Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
}

func (p *Parser) parseRecordUpdate(start span.Pos) Expr {
	record := p.parseExpr()
	p.expect(TokPipe)
	return p.parseRecordUpdateFields(start, record)
}

func (p *Parser) parseRecordUpdateFields(start span.Pos, record Expr) Expr {
	var updates []RecordField
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		fStart := p.peek().S.Start
		label := p.expectLower()
		p.expect(TokEq)
		value := p.parseExpr()
		updates = append(updates, RecordField{
			Label: label, Value: value,
			S: span.Span{Start: fStart, End: p.prevEnd()},
		})
		if p.peek().Kind == TokComma {
			p.advance()
		} else {
			break
		}
	}
	p.expect(TokRBrace)
	return &ExprRecordUpdate{
		Record: record, Updates: updates,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// --- Patterns ---

func (p *Parser) parsePattern() Pattern {
	if !p.enterRecurse() {
		return &PatWild{S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	defer p.leaveRecurse()
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
		// () → unit pattern (empty record pattern)
		if p.peek().Kind == TokRParen {
			p.advance()
			return &PatRecord{S: span.Span{Start: start, End: p.prevEnd()}}
		}
		inner := p.parsePattern()
		if p.peek().Kind == TokComma {
			// (p1, p2, ...) → tuple pattern (desugars to record pattern)
			pats := []Pattern{inner}
			for p.peek().Kind == TokComma {
				p.advance()
				pats = append(pats, p.parsePattern())
			}
			p.expect(TokRParen)
			fields := make([]PatRecordField, len(pats))
			for i, pat := range pats {
				fields[i] = PatRecordField{
					Label:   fmt.Sprintf("_%d", i+1),
					Pattern: pat,
					S:       pat.Span(),
				}
			}
			return &PatRecord{Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
		}
		p.expect(TokRParen)
		return &PatParen{Inner: inner, S: span.Span{Start: start, End: p.prevEnd()}}
	case TokLBrace:
		return p.parseRecordPattern()
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

func (p *Parser) parseRecordPattern() Pattern {
	start := p.peek().S.Start
	p.expect(TokLBrace)
	var fields []PatRecordField
	if p.peek().Kind != TokRBrace {
		for {
			fStart := p.peek().S.Start
			label := p.expectLower()
			p.expect(TokEq)
			pat := p.parsePattern()
			fields = append(fields, PatRecordField{
				Label: label, Pattern: pat,
				S: span.Span{Start: fStart, End: p.prevEnd()},
			})
			if p.peek().Kind == TokComma {
				p.advance()
			} else {
				break
			}
		}
	}
	p.expect(TokRBrace)
	return &PatRecord{Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
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
	if !p.enterRecurse() {
		return &TyExprCon{Name: "<error>", S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	defer p.leaveRecurse()
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

// parseKindAtom parses an atomic kind: Type, Row, Constraint, user-defined kind, or (K).
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
	case p.peek().Kind == TokUpper && p.peek().Text == "Kind":
		tok := p.peek()
		p.advance()
		return &KindExprSort{S: tok.S}
	case p.peek().Kind == TokUpper:
		// DataKinds: user-defined kind name (e.g., DBState, Bool)
		tok := p.peek()
		p.advance()
		return &KindExprName{Name: tok.Text, S: tok.S}
	case p.peek().Kind == TokLower:
		// Kind variable reference (e.g., k in "forall (k : Kind). k -> Type")
		tok := p.peek()
		p.advance()
		return &KindExprName{Name: tok.Text, S: tok.S}
	case p.peek().Kind == TokLParen:
		if !p.enterRecurse() {
			tok := p.peek()
			return &KindExprType{S: tok.S}
		}
		p.advance()
		k := p.parseKindExpr()
		p.expect(TokRParen)
		p.leaveRecurse()
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
		// () → unit type: Record {}
		if p.peek().Kind == TokRParen {
			p.advance()
			s := span.Span{Start: start, End: p.prevEnd()}
			return &TyExprApp{
				Fun: &TyExprCon{Name: "Record", S: s},
				Arg: &TyExprRow{S: s},
				S:   s,
			}
		}
		ty := p.parseType()
		if p.peek().Kind == TokComma {
			// (T1, T2, ...) → tuple type: Record { _1 : T1, _2 : T2, ... }
			types := []TypeExpr{ty}
			for p.peek().Kind == TokComma {
				p.advance()
				types = append(types, p.parseType())
			}
			p.expect(TokRParen)
			s := span.Span{Start: start, End: p.prevEnd()}
			fields := make([]TyRowField, len(types))
			for i, t := range types {
				fields[i] = TyRowField{
					Label: fmt.Sprintf("_%d", i+1),
					Type:  t,
					S:     t.Span(),
				}
			}
			return &TyExprApp{
				Fun: &TyExprCon{Name: "Record", S: s},
				Arg: &TyExprRow{Fields: fields, S: s},
				S:   s,
			}
		}
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

func (p *Parser) skipSemicolons() {
	for p.peek().Kind == TokSemicolon {
		p.advance()
	}
}

func (p *Parser) peek() Token {
	if p.halted || p.pos >= len(p.tokens) {
		return Token{Kind: TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	if p.halted {
		return Token{Kind: TokEOF}
	}
	p.steps++
	if p.steps > p.maxSteps {
		p.halt("parser step limit exceeded")
		return Token{Kind: TokEOF}
	}
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	switch tok.Kind {
	case TokLParen, TokLBrace, TokLBracket:
		p.depth++
	case TokRParen, TokRBrace, TokRBracket:
		if p.depth > 0 {
			p.depth--
		}
	}
	return tok
}

// enterRecurse increments recursion depth and returns false if limit exceeded.
func (p *Parser) enterRecurse() bool {
	if p.halted {
		return false
	}
	p.recurseDepth++
	if p.recurseDepth > p.maxRecurseDepth {
		p.halt("parser recursion depth limit exceeded")
		return false
	}
	return true
}

func (p *Parser) leaveRecurse() {
	p.recurseDepth--
}

func (p *Parser) halt(msg string) {
	if !p.halted {
		p.halted = true
		p.addError(msg)
	}
}

func (p *Parser) expect(kind TokenKind) Token {
	tok := p.peek()
	if tok.Kind != kind {
		p.addError("expected " + kind.String())
		if tok.Kind != TokEOF {
			p.advance() // skip unexpected token to prevent parser stalling
		}
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
	case TokLower, TokUpper, TokData, TokType, TokInfixl, TokInfixr, TokInfixn, TokClass, TokInstance, TokImport:
		return true
	case TokLParen:
		// (op) declaration pattern
		return p.isOperatorDeclStart()
	}
	return false
}

func (p *Parser) parseListLit() Expr {
	start := p.peek().S.Start
	p.expect(TokLBracket)
	var elems []Expr
	if p.peek().Kind != TokRBracket {
		elems = append(elems, p.parseExpr())
		for p.peek().Kind == TokComma {
			p.advance()
			elems = append(elems, p.parseExpr())
		}
	}
	p.expect(TokRBracket)
	return &ExprList{
		Elems: elems,
		S:     span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) isAtomStart() bool {
	if p.atDeclBoundary() {
		return false
	}
	k := p.peek().Kind
	if p.noBraceAtom && k == TokLBrace {
		return false
	}
	return k == TokLower || k == TokUpper || k == TokLParen || k == TokBackslash || k == TokLBrace || k == TokCase || k == TokDo || k == TokIntLit || k == TokStrLit || k == TokRuneLit || k == TokLBracket
}

func (p *Parser) isTypeAtomStart() bool {
	if p.atDeclBoundary() {
		return false
	}
	k := p.peek().Kind
	return k == TokLower || k == TokUpper || k == TokLParen || k == TokLBrace
}

// isClassKindedBinder checks if the next tokens form (name : Kind) pattern.
func (p *Parser) isClassKindedBinder() bool {
	// Current is '(', check if followed by lower : Kind )
	if p.pos+2 >= len(p.tokens) {
		return false
	}
	return p.tokens[p.pos+1].Kind == TokLower && p.tokens[p.pos+2].Kind == TokColon
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
