package parse

import (
	"fmt"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/span"
)

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
	case TokUnderscore:
		// Wildcard type pattern: _ (used in type family equations)
		tok := p.peek()
		p.advance()
		return &TyExprVar{Name: "_", S: tok.S}
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
		var mult TypeExpr
		if p.peek().Kind == TokAt {
			p.advance()
			mult = p.parseTypeAtom()
		}
		fields = append(fields, TyRowField{
			Label: label, Type: ty, Mult: mult,
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
