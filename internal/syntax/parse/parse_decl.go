package parse

import (
	"strconv"

	syn "github.com/cwd-k2/gicel/internal/syntax"

	"github.com/cwd-k2/gicel/internal/span"
)

// --- Declarations ---

func (p *Parser) collectFixity() {
	saved := p.pos
	for p.peek().Kind != syn.TokEOF {
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

func (p *Parser) parseDecl() syn.Decl {
	switch {
	case p.peek().Kind == syn.TokData:
		return p.parseDataDecl()
	case p.peek().Kind == syn.TokType:
		return p.parseTypeDecl()
	case p.peek().Kind == syn.TokClass:
		return p.parseClassDecl()
	case p.peek().Kind == syn.TokInstance:
		return p.parseInstanceDecl()
	case p.isFixityKeyword():
		d := p.parseFixityDecl()
		return d
	case p.peek().Kind == syn.TokLParen && p.isOperatorDeclStart():
		return p.parseOperatorDecl()
	case p.peek().Kind == syn.TokLower:
		return p.parseNamedDecl()
	default:
		p.addError("expected declaration")
		p.advance()
		return nil
	}
}

func (p *Parser) parseDataDecl() *syn.DeclData {
	start := p.peek().S.Start
	p.expect(syn.TokData)
	name := p.expectUpper()
	var params []syn.TyBinder
	for p.peek().Kind == syn.TokLower || p.peek().Kind == syn.TokLParen {
		if p.peek().Kind == syn.TokLParen {
			// Kinded param: (name: Kind)
			lp := p.peek().S.Start
			p.advance()
			pName := p.expectLower()
			p.expect(syn.TokColon)
			kind := p.parseKindExpr()
			p.expect(syn.TokRParen)
			params = append(params, syn.TyBinder{
				Name: pName,
				Kind: kind,
				S:    span.Span{Start: lp, End: p.prevEnd()},
			})
		} else {
			pName := p.peek().Text
			pS := p.peek().S
			p.advance()
			params = append(params, syn.TyBinder{Name: pName, S: pS})
		}
	}
	p.expect(syn.TokColonEq)

	// GADT mode: `:= { ConName :: Type; ... }`
	if p.peek().Kind == syn.TokLBrace {
		gadtCons := p.parseGADTCons()
		return &syn.DeclData{
			Name:     name,
			Params:   params,
			GADTCons: gadtCons,
			S:        span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// ADT mode: `= Con1 fields | Con2 fields`
	var cons []syn.DeclCon
	cons = append(cons, p.parseConDecl())
	for p.peek().Kind == syn.TokPipe {
		p.advance()
		cons = append(cons, p.parseConDecl())
	}
	return &syn.DeclData{
		Name:   name,
		Params: params,
		Cons:   cons,
		S:      span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseGADTCons parses GADT constructor declarations inside braces.
// Format: { ConName :: Type ; ConName :: Type ; ... }
func (p *Parser) parseGADTCons() []syn.GADTConDecl {
	p.expect(syn.TokLBrace)
	var cons []syn.GADTConDecl
	for p.peek().Kind != syn.TokRBrace && p.peek().Kind != syn.TokEOF {
		before := p.pos
		cStart := p.peek().S.Start
		cName := p.expectUpper()
		p.expect(syn.TokColonColon)
		cType := p.parseType()
		cons = append(cons, syn.GADTConDecl{
			Name: cName,
			Type: cType,
			S:    span.Span{Start: cStart, End: p.prevEnd()},
		})
		if p.peek().Kind == syn.TokSemicolon {
			p.advance()
		} else if p.pos == before {
			p.addError("unexpected token in GADT declaration")
			p.advance()
		}
	}
	p.expect(syn.TokRBrace)
	return cons
}

func (p *Parser) parseConDecl() syn.DeclCon {
	start := p.peek().S.Start
	name := p.expectUpper()
	var fields []syn.TypeExpr
	for p.isTypeAtomStart() && !p.atDeclBoundary() {
		fields = append(fields, p.parseTypeAtom())
	}
	return syn.DeclCon{Name: name, Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
}

// parseTypeDecl parses either a type alias or a type family declaration.
//
//	type Name params = syn.TypeExpr              → type alias
//	type Name params :: Kind = { equations } → type family
func (p *Parser) parseTypeDecl() syn.Decl {
	start := p.peek().S.Start
	p.expect(syn.TokType)
	name := p.expectUpper()
	params := p.parseTyBinderList()

	if p.peek().Kind == syn.TokColonColon {
		// Type family declaration.
		return p.parseTypeFamilyBody(name, params, start)
	}

	// Type alias.
	p.expect(syn.TokColonEq)
	body := p.parseType()
	return &syn.DeclTypeAlias{
		Name:   name,
		Params: params,
		Body:   body,
		S:      span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseTyBinderList parses a sequence of type binders: bare or kinded.
func (p *Parser) parseTyBinderList() []syn.TyBinder {
	var params []syn.TyBinder
	for p.peek().Kind == syn.TokLower || (p.peek().Kind == syn.TokLParen && p.isClassKindedBinder()) {
		if p.peek().Kind == syn.TokLParen {
			lp := p.peek().S.Start
			p.advance()
			pName := p.expectLower()
			p.expect(syn.TokColon)
			kind := p.parseKindExpr()
			p.expect(syn.TokRParen)
			params = append(params, syn.TyBinder{Name: pName, Kind: kind, S: span.Span{Start: lp, End: p.prevEnd()}})
		} else {
			pName := p.peek().Text
			pS := p.peek().S
			p.advance()
			params = append(params, syn.TyBinder{Name: pName, S: pS})
		}
	}
	return params
}

// parseTypeFamilyBody parses the body of a type family declaration after `::`.
//
//	:: Kind = { equations }
//	:: (r: Kind) | deps = { equations }
func (p *Parser) parseTypeFamilyBody(name string, params []syn.TyBinder, start span.Pos) *syn.DeclTypeFamily {
	p.expect(syn.TokColonColon)
	resultKind, resultName, deps := p.parseResultKind()

	p.expect(syn.TokColonEq)
	equations := p.parseTypeFamilyEquations(name)

	return &syn.DeclTypeFamily{
		Name:       name,
		Params:     params,
		ResultKind: resultKind,
		ResultName: resultName,
		Deps:       deps,
		Equations:  equations,
		S:          span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseResultKind parses either a plain kind or an injective result kind.
//
//	Type                            → (syn.KindExprType, "", nil)
//	(r: Type) | r -> a             → (syn.KindExprType, "r", [{From:"r", To:["a"]}])
func (p *Parser) parseResultKind() (syn.KindExpr, string, []syn.FunDep) {
	// Check for injective form: (name: Kind) | deps
	if p.peek().Kind == syn.TokLParen && p.isInjectiveResult() {
		p.advance() // consume (
		resultName := p.expectLower()
		p.expect(syn.TokColon)
		kind := p.parseKindExpr()
		p.expect(syn.TokRParen)

		// Parse | deps
		p.expect(syn.TokPipe)
		deps := p.parseFunDepList()
		return kind, resultName, deps
	}

	// Plain kind expression.
	kind := p.parseKindExpr()
	return kind, "", nil
}

// isInjectiveResult checks if the tokens at current position form
// (lower: Kind) | ..., which indicates an injective result kind.
func (p *Parser) isInjectiveResult() bool {
	if p.pos+3 >= len(p.tokens) {
		return false
	}
	return p.tokens[p.pos+1].Kind == syn.TokLower && p.tokens[p.pos+2].Kind == syn.TokColon
}

// parseTypeFamilyEquations parses the equation block { Name Pat* = RHS; ... }.
func (p *Parser) parseTypeFamilyEquations(familyName string) []syn.TFEquation {
	p.expect(syn.TokLBrace)
	var equations []syn.TFEquation
	for p.peek().Kind != syn.TokRBrace && p.peek().Kind != syn.TokEOF {
		before := p.pos
		eqStart := p.peek().S.Start
		eqName := p.expectUpper()
		// Parse LHS type patterns.
		var patterns []syn.TypeExpr
		for p.isTypeAtomStart() && p.peek().Kind != syn.TokEqColon {
			patterns = append(patterns, p.parseTypeAtom())
		}
		p.expect(syn.TokEqColon)
		rhs := p.parseType()
		equations = append(equations, syn.TFEquation{
			Name:     eqName,
			Patterns: patterns,
			RHS:      rhs,
			S:        span.Span{Start: eqStart, End: p.prevEnd()},
		})
		if p.peek().Kind == syn.TokSemicolon {
			p.advance()
		} else if p.pos == before {
			p.addError("unexpected token in type family declaration")
			p.advance()
		}
	}
	p.expect(syn.TokRBrace)
	return equations
}

func (p *Parser) parseFixityDecl() *syn.DeclFixity {
	start := p.peek().S.Start
	var assoc syn.Assoc
	switch p.peek().Kind {
	case syn.TokInfixl:
		assoc = syn.AssocLeft
	case syn.TokInfixr:
		assoc = syn.AssocRight
	case syn.TokInfixn:
		assoc = syn.AssocNone
	}
	p.advance()
	prec := 9
	if p.peek().Kind == syn.TokIntLit {
		n, err := strconv.Atoi(p.peek().Text)
		if err == nil {
			prec = n
		}
		p.advance()
	}
	var op string
	if p.peek().Kind == syn.TokOp {
		op = p.peek().Text
		p.advance()
	} else if p.peek().Kind == syn.TokDot {
		op = "."
		p.advance()
	} else {
		p.addError("expected operator in fixity declaration")
		return nil
	}
	return &syn.DeclFixity{
		Assoc: assoc, Prec: prec, Op: op,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) isOperatorDeclStart() bool {
	// Check for (operator) pattern: ( op ) — syn.TokDot also valid as operator.
	return p.pos+2 < len(p.tokens) &&
		(p.tokens[p.pos+1].Kind == syn.TokOp || p.tokens[p.pos+1].Kind == syn.TokDot) &&
		p.tokens[p.pos+2].Kind == syn.TokRParen
}

func (p *Parser) parseOperatorDecl() syn.Decl {
	start := p.expect(syn.TokLParen)
	// Accept both syn.TokOp and syn.TokDot as operator names.
	var op syn.Token
	if p.peek().Kind == syn.TokDot {
		op = p.peek()
		op.Text = "."
		p.advance()
	} else {
		op = p.expect(syn.TokOp)
	}
	p.expect(syn.TokRParen)
	switch p.peek().Kind {
	case syn.TokColonColon:
		p.advance()
		ty := p.parseType()
		return &syn.DeclTypeAnn{Name: op.Text, Type: ty, S: span.Span{Start: start.S.Start, End: p.prevEnd()}}
	case syn.TokColonEq:
		p.advance()
		expr := p.parseExpr()
		return &syn.DeclValueDef{Name: op.Text, Expr: expr, S: span.Span{Start: start.S.Start, End: p.prevEnd()}}
	default:
		p.addError("expected :: or := after operator name")
		return nil
	}
}

func (p *Parser) parseNamedDecl() syn.Decl {
	start := p.peek().S.Start
	name := p.peek().Text
	p.advance()

	switch p.peek().Kind {
	case syn.TokColonColon:
		// Type annotation: f :: T
		p.advance()
		ty := p.parseType()
		return &syn.DeclTypeAnn{
			Name: name, Type: ty,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	case syn.TokColonEq:
		// Value definition: f := e
		p.advance()
		expr := p.parseExpr()
		return &syn.DeclValueDef{
			Name: name, Expr: expr,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	default:
		p.addError("expected :: or := after name")
		return nil
	}
}

// --- Declaration helpers ---

// decomposeTupleConstraint extracts individual constraints from a parsed type.
// (Eq a)       → syn.TyExprParen → [Eq a]
// (Eq a, Ord a) → tuple Record → [Eq a, Ord a]
// other        → [other]
func decomposeTupleConstraint(ty syn.TypeExpr) []syn.TypeExpr {
	if paren, ok := ty.(*syn.TyExprParen); ok {
		return []syn.TypeExpr{paren.Inner}
	}
	// Tuple: syn.TyExprApp { Fun: syn.TyExprCon{Record}, Arg: syn.TyExprRow{Fields: ...} }
	if app, ok := ty.(*syn.TyExprApp); ok {
		if con, ok := app.Fun.(*syn.TyExprCon); ok && con.Name == "Record" {
			if row, ok := app.Arg.(*syn.TyExprRow); ok && len(row.Fields) > 0 {
				cs := make([]syn.TypeExpr, len(row.Fields))
				for i, f := range row.Fields {
					cs[i] = f.Type
				}
				return cs
			}
		}
	}
	return []syn.TypeExpr{ty}
}

func (p *Parser) isFixityKeyword() bool {
	k := p.peek().Kind
	return k == syn.TokInfixl || k == syn.TokInfixr || k == syn.TokInfixn
}

// isClassKindedBinder checks if the next tokens form (name: Kind) pattern.
func (p *Parser) isClassKindedBinder() bool {
	if p.pos+2 >= len(p.tokens) {
		return false
	}
	return p.tokens[p.pos+1].Kind == syn.TokLower && p.tokens[p.pos+2].Kind == syn.TokColon
}
