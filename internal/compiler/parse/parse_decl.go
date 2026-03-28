package parse

import (
	"fmt"
	"strconv"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// --- Declarations ---

func (p *Parser) collectFixity() {
	saved := p.pos
	localFixity := make(map[string]bool)
	for p.peek().Kind != syn.TokEOF {
		if p.isFixityKeyword() {
			d := p.parseFixityDecl()
			if d != nil {
				if localFixity[d.Op] {
					p.errors.Add(&diagnostic.Error{
						Code:    diagnostic.ErrDuplicateDecl,
						Phase:   diagnostic.PhaseParse,
						Span:    d.S,
						Message: fmt.Sprintf("duplicate fixity declaration for operator %s", d.Op),
					})
				}
				localFixity[d.Op] = true
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
//	form Name [:: Kind] := Body;
//	type Name [:: Kind] := Body;
//	impl [name ::] TypeExpr := Expr;
//	name :: Type;
//	name := Expr;
//	infixl/infixr/infixn N op;
//	import Module;
func (p *Parser) parseDecl() syn.Decl {
	switch {
	case p.peek().Kind == syn.TokForm:
		return p.parseFormDecl()
	case p.peek().Kind == syn.TokType:
		return p.parseTypeDecl()
	case p.peek().Kind == syn.TokImpl:
		return p.parseImplDecl()
	case p.isFixityKeyword():
		return p.parseFixityDecl()
	case p.peek().Kind == syn.TokLParen && p.isOperatorDeclStart():
		return p.parseOperatorDecl()
	case p.peek().Kind == syn.TokLower:
		// Hint for common keywords from other languages.
		switch p.peek().Text {
		case "data":
			p.addErrorCode(diagnostic.ErrUnexpectedToken, "unknown keyword 'data'; use 'form' for type/class declarations")
			p.advance()
			return nil
		case "class":
			p.addErrorCode(diagnostic.ErrUnexpectedToken, "unknown keyword 'class'; use 'form' for type class declarations")
			p.advance()
			return nil
		case "instance":
			p.addErrorCode(diagnostic.ErrUnexpectedToken, "unknown keyword 'instance'; use 'impl' for instance declarations")
			p.advance()
			return nil
		case "where":
			p.addErrorCode(diagnostic.ErrUnexpectedToken, "unknown keyword 'where'; use braces { } for declaration bodies")
			p.advance()
			return nil
		case "let":
			p.addErrorCode(diagnostic.ErrUnexpectedToken, "unknown keyword 'let'; use { name := expr; body } for local bindings")
			p.advance()
			return nil
		}
		return p.parseNamedDecl()
	default:
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected declaration")
		p.advance()
		return nil
	}
}

// parseFormDecl parses a nominal type declaration.
//
//	form Name [:: Kind] := Body;
//
// Body is a type expression: \params. [constraints =>] { fields/constructors }.
//
// ADT shorthand (sugar): form Name [params] := Con1 [fields] | Con2 [fields]
// Desugars to GADT-style row constructors.
func (p *Parser) parseFormDecl() *syn.DeclForm {
	start := p.peek().S.Start
	p.expect(syn.TokForm)
	name := p.expectUpper()

	var kindAnn syn.TypeExpr
	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		kindAnn = p.parseKindAnnotation()
	}

	p.expect(syn.TokColonEq)

	// ADT shorthand: form Name := Con1 | Con2 fields | ...
	if p.peek().Kind == syn.TokUpper && p.looksLikePipeADT() {
		body := p.parseADTConsAsRow(nil, start)
		return &syn.DeclForm{Name: name, KindAnn: kindAnn, Body: body, S: span.Span{Start: start, End: p.prevEnd()}}
	}

	body := p.parseType()

	return &syn.DeclForm{
		Name:    name,
		KindAnn: kindAnn,
		Body:    body,
		S:       span.Span{Start: start, End: p.prevEnd()},
	}
}

// looksLikePipeADT peeks ahead to see if the current position starts a pipe-separated
// ADT constructor list (e.g., Red | Green | Blue).
func (p *Parser) looksLikePipeADT() bool {
	for i := p.pos; i < len(p.tokens); i++ {
		k := p.tokens[i].Kind
		if k == syn.TokPipe {
			return true
		}
		if k == syn.TokSemicolon || k == syn.TokEOF || (p.tokens[i].NewlineBefore && i > p.pos) {
			return false
		}
	}
	return false
}

// parseADTConsAsRow parses ADT constructors (Con1 fields | Con2 fields)
// and synthesizes a TyExprRow with GADT-style full constructor types.
func (p *Parser) parseADTConsAsRow(params []syn.TyBinder, start span.Pos) syn.TypeExpr {
	type adtCon struct {
		name   string
		fields []syn.TypeExpr
		s      span.Span
	}

	var cons []adtCon
	for {
		cStart := p.peek().S.Start
		cName := p.expectUpper()
		var fields []syn.TypeExpr
		for p.isTypeAtomStart() && !p.atStmtBoundary() && p.peek().Kind != syn.TokPipe {
			fields = append(fields, p.parseTypeAtom())
		}
		cons = append(cons, adtCon{name: cName, fields: fields, s: span.Span{Start: cStart, End: p.prevEnd()}})
		if p.peek().Kind == syn.TokPipe {
			p.advance()
		} else {
			break
		}
	}

	// Build the form type name applied to params for return types.
	// e.g., form Maybe a := Just a | Nothing → return type = Maybe a
	// For param-less: form Bool := True | False → return type = Bool
	// We need the form type name from the outer context, but parseADTConsAsRow
	// doesn't have it. Use a placeholder that the checker resolves.
	// Actually, for ADT shorthand, constructors don't include the return type
	// (it's implicit). We synthesize: Con: field1 -> field2 -> ... -> ()
	// where () signals "nullary or ADT-style" to the checker.

	rowFields := make([]syn.TyRowField, len(cons))
	for i, c := range cons {
		var ty syn.TypeExpr
		if len(c.fields) == 0 {
			// Nullary: synthesize unit type ()
			ty = &syn.TyExprApp{
				Fun: &syn.TyExprCon{Name: types.TyConRecord, S: c.s},
				Arg: &syn.TyExprRow{S: c.s},
				S:   c.s,
			}
		} else {
			// Non-nullary: synthesize arrow chain f1 -> f2 -> ... -> ()
			// The trailing () is a sentinel that processFormDeclParts replaces
			// with the actual result type (e.g., Shape).
			ty = &syn.TyExprApp{
				Fun: &syn.TyExprCon{Name: types.TyConRecord, S: c.s},
				Arg: &syn.TyExprRow{S: c.s},
				S:   c.s,
			}
			for j := len(c.fields) - 1; j >= 0; j-- {
				ty = &syn.TyExprArrow{From: c.fields[j], To: ty, S: c.s}
			}
		}
		rowFields[i] = syn.TyRowField{Label: c.name, Type: ty, S: c.s}
	}

	row := &syn.TyExprRow{Fields: rowFields, S: span.Span{Start: start, End: p.prevEnd()}}
	if len(params) > 0 {
		return &syn.TyExprForall{Binders: params, Body: row, S: span.Span{Start: start, End: p.prevEnd()}}
	}
	return row
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

	var kindAnn syn.TypeExpr
	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		kindAnn = p.parseKindAnnotation()
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

	// Parse type annotation, preventing { from being consumed as a row type.
	savedNoBrace := p.noBraceAtom
	p.noBraceAtom = true
	ann := p.parseType()
	p.noBraceAtom = savedNoBrace

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
		// Host-provided assumption: name := assumption
		if p.peek().Kind == syn.TokAssumption {
			p.advance()
			return &syn.DeclValueDef{
				Name: name, IsAssumption: true,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
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
