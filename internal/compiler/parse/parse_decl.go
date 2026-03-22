package parse

import (
	"strconv"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
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

// parseDecl dispatches on the leading keyword to the appropriate declaration parser.
//
// Unified syntax declarations:
//
//	data Name [:: Kind] := TypeExpr;
//	type Name [:: Kind] := TypeExpr;
//	impl [name ::] TypeExpr := Expr;
//	name :: Type;
//	name := Expr;
//	infixl/infixr/infixn N op;
//	import Module;
func (p *Parser) parseDecl() syn.Decl {
	switch {
	case p.peek().Kind == syn.TokData:
		return p.parseDataDecl()
	case p.peek().Kind == syn.TokType:
		return p.parseTypeDecl()
	case p.peek().Kind == syn.TokImpl:
		return p.parseImplDecl()
	case p.isFixityKeyword():
		return p.parseFixityDecl()
	case p.peek().Kind == syn.TokLParen && p.isOperatorDeclStart():
		return p.parseOperatorDecl()
	case p.peek().Kind == syn.TokLower:
		return p.parseNamedDecl()
	default:
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected declaration")
		p.advance()
		return nil
	}
}

// parseDataDecl parses a nominal type declaration.
//
//	data Name [:: Kind] := Body;
//
// Body is a type expression: \params. [constraints =>] { fields/constructors }.
func (p *Parser) parseDataDecl() *syn.DeclData {
	start := p.peek().S.Start
	p.expect(syn.TokData)
	name := p.expectUpper()

	var kindAnn syn.KindExpr
	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		kindAnn = p.parseKindExpr()
	}

	p.expect(syn.TokColonEq)
	body := p.parseType()

	return &syn.DeclData{
		Name:    name,
		KindAnn: kindAnn,
		Body:    body,
		S:       span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseTypeDecl parses a structural type declaration.
//
//	type Name [:: Kind] := Body;
//
// Body is a type expression. For type families, the body contains a case.
func (p *Parser) parseTypeDecl() *syn.DeclTypeAlias {
	start := p.peek().S.Start
	p.expect(syn.TokType)
	name := p.expectUpper()

	var kindAnn syn.KindExpr
	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		kindAnn = p.parseKindExpr()
	}

	p.expect(syn.TokColonEq)
	body := p.parseType()

	return &syn.DeclTypeAlias{
		Name:    name,
		KindAnn: kindAnn,
		Body:    body,
		S:       span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseImplDecl parses an axiom registration.
//
//	impl name :: Type := Expr;    (named)
//	impl Type := Expr;            (unnamed)
//
// Disambiguation: if the token after `impl` is a lowercase name followed by `::`,
// it's a named impl. Otherwise the tokens form the type annotation directly.
func (p *Parser) parseImplDecl() *syn.DeclImpl {
	start := p.peek().S.Start
	p.expect(syn.TokImpl)

	// Disambiguate named vs unnamed.
	var name string
	if p.peek().Kind == syn.TokLower && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == syn.TokColonColon {
		name = p.peek().Text
		p.advance() // consume name
		p.advance() // consume ::
	}

	ann := p.parseType()
	p.expect(syn.TokColonEq)
	body := p.parseExpr()

	return &syn.DeclImpl{
		Name: name,
		Ann:  ann,
		Body: body,
		S:    span.Span{Start: start, End: p.prevEnd()},
	}
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
		p.addErrorCode(diagnostic.ErrFixitySyntax, "expected operator in fixity declaration")
		return nil
	}
	return &syn.DeclFixity{
		Assoc: assoc, Prec: prec, Op: op,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) isOperatorDeclStart() bool {
	return p.pos+2 < len(p.tokens) &&
		(p.tokens[p.pos+1].Kind == syn.TokOp || p.tokens[p.pos+1].Kind == syn.TokDot) &&
		p.tokens[p.pos+2].Kind == syn.TokRParen
}

func (p *Parser) parseOperatorDecl() syn.Decl {
	start := p.expect(syn.TokLParen)
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
		p.addErrorCode(diagnostic.ErrInvalidOperator, "expected :: or := after operator name")
		return nil
	}
}

func (p *Parser) parseNamedDecl() syn.Decl {
	start := p.peek().S.Start
	name := p.peek().Text
	p.advance()

	switch p.peek().Kind {
	case syn.TokColonColon:
		p.advance()
		ty := p.parseType()
		return &syn.DeclTypeAnn{
			Name: name, Type: ty,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	case syn.TokColonEq:
		eqTok := p.advance()
		if p.atStmtBoundary() || p.peek().Kind == syn.TokEOF || p.peek().Kind == syn.TokSemicolon {
			p.errors.Add(&diagnostic.Error{
				Code:    diagnostic.ErrMissingBody,
				Phase:   diagnostic.PhaseParse,
				Span:    eqTok.S,
				Message: "expected expression after :=",
			})
			return nil
		}
		expr := p.parseExpr()
		return &syn.DeclValueDef{
			Name: name, Expr: expr,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	default:
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected :: or := after name")
		return nil
	}
}

// --- Declaration helpers ---

func (p *Parser) isFixityKeyword() bool {
	k := p.peek().Kind
	return k == syn.TokInfixl || k == syn.TokInfixr || k == syn.TokInfixn
}
