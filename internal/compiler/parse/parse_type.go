package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	gtypes "github.com/cwd-k2/gicel/internal/lang/types"
)

func (p *Parser) parseType() syn.TypeExpr {
	if !p.enterRecurse() {
		return &syn.TyExprCon{Name: "<error>", S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	defer p.leaveRecurse()
	return p.parseTypeArrow()
}

func (p *Parser) parseTypeArrow() syn.TypeExpr {
	if p.peek().Kind == syn.TokBackslash {
		return p.parseForallType()
	}
	left := p.parseTypeApp()
	if p.peek().Kind == syn.TokFatArrow {
		p.advance()
		body := p.parseTypeArrow() // right-associative
		// (C1, C2, ...) => T  desugars to  C1 => C2 => ... => T
		if constraints := desugarConstraintTuple(left); constraints != nil {
			result := body
			for i := len(constraints) - 1; i >= 0; i-- {
				result = &syn.TyExprQual{
					Constraint: constraints[i], Body: result,
					S: span.Span{Start: constraints[i].Span().Start, End: result.Span().End},
				}
			}
			return result
		}
		return &syn.TyExprQual{
			Constraint: left, Body: body,
			S: span.Span{Start: left.Span().Start, End: body.Span().End},
		}
	}
	if p.peek().Kind == syn.TokArrow {
		p.advance()
		right := p.parseTypeArrow() // right-associative
		return &syn.TyExprArrow{
			From: left, To: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}
	return left
}

func (p *Parser) parseForallType() syn.TypeExpr {
	start := p.peek().S.Start
	p.expect(syn.TokBackslash)
	var binders []syn.TyBinder
	for p.peek().Kind == syn.TokLower || p.peek().Kind == syn.TokLParen {
		if p.peek().Kind == syn.TokLParen {
			// Kinded binder: (v: Kind)
			lp := p.peek().S.Start
			p.advance()
			name := p.expectLower()
			p.expect(syn.TokColon)
			kind := p.parseKindExpr()
			p.expect(syn.TokRParen)
			binders = append(binders, syn.TyBinder{
				Name: name,
				Kind: kind,
				S:    span.Span{Start: lp, End: p.prevEnd()},
			})
		} else {
			// Bare type variable (kind inferred)
			b := syn.TyBinder{Name: p.peek().Text, S: p.peek().S}
			p.advance()
			binders = append(binders, b)
		}
	}
	p.expect(syn.TokDot)
	body := p.parseType()
	return &syn.TyExprForall{
		Binders: binders, Body: body,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseKindExpr parses a kind expression: Type, Row, or K1 -> K2.
func (p *Parser) parseKindExpr() syn.KindExpr {
	left := p.parseKindAtom()
	if p.peek().Kind == syn.TokArrow {
		p.advance()
		right := p.parseKindExpr() // right-associative
		return &syn.KindExprArrow{
			From: left, To: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}
	return left
}

// parseKindAtom parses an atomic kind: Type, Row, Constraint, user-defined kind, or (K).
func (p *Parser) parseKindAtom() syn.KindExpr {
	switch {
	case p.peek().Kind == syn.TokUpper && p.peek().Text == "Type":
		tok := p.peek()
		p.advance()
		return &syn.KindExprType{S: tok.S}
	case p.peek().Kind == syn.TokUpper && p.peek().Text == "Row":
		tok := p.peek()
		p.advance()
		return &syn.KindExprRow{S: tok.S}
	case p.peek().Kind == syn.TokUpper && p.peek().Text == "Constraint":
		tok := p.peek()
		p.advance()
		return &syn.KindExprConstraint{S: tok.S}
	case p.peek().Kind == syn.TokUpper && p.peek().Text == "Kind":
		tok := p.peek()
		p.advance()
		return &syn.KindExprSort{S: tok.S}
	case p.peek().Kind == syn.TokUpper:
		// DataKinds: user-defined kind name (e.g., DBState, Bool)
		tok := p.peek()
		p.advance()
		return &syn.KindExprName{Name: tok.Text, S: tok.S}
	case p.peek().Kind == syn.TokLower:
		// Kind variable reference (e.g., k in "\ (k: Kind). k -> Type")
		tok := p.peek()
		p.advance()
		return &syn.KindExprName{Name: tok.Text, S: tok.S}
	case p.peek().Kind == syn.TokLParen:
		if !p.enterRecurse() {
			tok := p.peek()
			return &syn.KindExprType{S: tok.S}
		}
		p.advance()
		k := p.parseKindExpr()
		p.expect(syn.TokRParen)
		p.leaveRecurse()
		return k
	default:
		p.addErrorCode(diagnostic.ErrExpectedType, "expected kind (Type, Row, or K -> K)")
		tok := p.peek()
		p.advance()
		return &syn.KindExprType{S: tok.S}
	}
}

func (p *Parser) parseTypeApp() syn.TypeExpr {
	f := p.parseTypeAtom()
	for p.isTypeAtomStart() {
		arg := p.parseTypeAtom()
		f = &syn.TyExprApp{
			Fun: f, Arg: arg,
			S: span.Span{Start: f.Span().Start, End: arg.Span().End},
		}
	}
	return f
}

func (p *Parser) parseTypeAtom() syn.TypeExpr {
	switch p.peek().Kind {
	case syn.TokUnderscore:
		// Wildcard type pattern: _ (used in type family equations)
		tok := p.peek()
		p.advance()
		return &syn.TyExprVar{Name: "_", S: tok.S}
	case syn.TokLower:
		tok := p.peek()
		p.advance()
		return &syn.TyExprVar{Name: tok.Text, S: tok.S}
	case syn.TokUpper:
		tok := p.peek()
		p.advance()
		// Qualified type: Upper.Upper (adjacent, no whitespace) → syn.TyExprQualCon
		if p.peek().Kind == syn.TokDot && tokensAdjacent(tok, p.peek()) {
			dotTok := p.peek()
			if p.pos+1 < len(p.tokens) {
				nextTok := p.tokens[p.pos+1]
				if tokensAdjacent(dotTok, nextTok) && nextTok.Kind == syn.TokUpper {
					p.advance() // consume .
					p.advance() // consume Upper
					return &syn.TyExprQualCon{Qualifier: tok.Text, Name: nextTok.Text,
						S: span.Span{Start: tok.S.Start, End: nextTok.S.End}}
				}
			}
		}
		return &syn.TyExprCon{Name: tok.Text, S: tok.S}
	case syn.TokLParen:
		start := p.peek().S.Start
		p.advance()
		// () → unit type: Record {}
		if p.peek().Kind == syn.TokRParen {
			p.advance()
			s := span.Span{Start: start, End: p.prevEnd()}
			return &syn.TyExprApp{
				Fun: &syn.TyExprCon{Name: "Record", S: s},
				Arg: &syn.TyExprRow{S: s},
				S:   s,
			}
		}
		ty := p.parseType()
		if p.peek().Kind == syn.TokComma {
			// (T1, T2, ...) → tuple type: Record { _1: T1, _2: T2, ... }
			types := []syn.TypeExpr{ty}
			for p.peek().Kind == syn.TokComma {
				p.advance()
				types = append(types, p.parseType())
			}
			p.expect(syn.TokRParen)
			s := span.Span{Start: start, End: p.prevEnd()}
			fields := make([]syn.TyRowField, len(types))
			for i, t := range types {
				fields[i] = syn.TyRowField{
					Label: gtypes.TupleLabel(i + 1),
					Type:  t,
					S:     t.Span(),
				}
			}
			return &syn.TyExprApp{
				Fun: &syn.TyExprCon{Name: "Record", S: s},
				Arg: &syn.TyExprRow{Fields: fields, S: s},
				S:   s,
			}
		}
		p.expect(syn.TokRParen)
		return &syn.TyExprParen{Inner: ty, S: span.Span{Start: start, End: p.prevEnd()}}
	case syn.TokLBrace:
		return p.parseRowType()
	default:
		p.addErrorCode(diagnostic.ErrExpectedType, "expected type")
		tok := p.peek()
		p.advance()
		return &syn.TyExprCon{Name: "<error>", S: tok.S}
	}
}

func (p *Parser) parseRowType() syn.TypeExpr {
	start := p.peek().S.Start
	p.expect(syn.TokLBrace)
	if p.peek().Kind == syn.TokRBrace {
		p.advance()
		return &syn.TyExprRow{S: span.Span{Start: start, End: p.prevEnd()}}
	}
	var fields []syn.TyRowField
	var tail *syn.TyExprVar

	for {
		if p.peek().Kind == syn.TokPipe {
			p.advance()
			if p.peek().Kind == syn.TokLower {
				tok := p.peek()
				tail = &syn.TyExprVar{Name: tok.Text, S: tok.S}
				p.advance()
			}
			break
		}
		if p.peek().Kind == syn.TokRBrace || p.peek().Kind == syn.TokEOF {
			break
		}
		label := p.expectLower()
		p.expect(syn.TokColon)
		ty := p.parseType()
		var mult syn.TypeExpr
		if p.peek().Kind == syn.TokAt {
			p.advance()
			mult = p.parseTypeAtom()
		}
		fields = append(fields, syn.TyRowField{
			Label: label, Type: ty, Mult: mult,
			S: span.Span{Start: span.Pos(p.pos), End: p.prevEnd()},
		})
		if p.peek().Kind == syn.TokComma {
			p.advance()
		}
	}
	p.expect(syn.TokRBrace)
	return &syn.TyExprRow{
		Fields: fields, Tail: tail,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// desugarConstraintTuple detects a tuple type used as a constraint group.
// (C1, C2, ...) parses as syn.TyExprApp(Record, syn.TyExprRow{_1: C1, _2: C2, ...}).
// Returns the constraint types if the pattern matches, nil otherwise.
func desugarConstraintTuple(t syn.TypeExpr) []syn.TypeExpr {
	app, ok := t.(*syn.TyExprApp)
	if !ok {
		return nil
	}
	con, ok := app.Fun.(*syn.TyExprCon)
	if !ok || con.Name != "Record" {
		return nil
	}
	row, ok := app.Arg.(*syn.TyExprRow)
	if !ok || len(row.Fields) < 2 || row.Tail != nil {
		return nil
	}
	// Verify tuple field labels: _1, _2, ...
	constraints := make([]syn.TypeExpr, len(row.Fields))
	for i, f := range row.Fields {
		if f.Label != gtypes.TupleLabel(i+1) {
			return nil
		}
		constraints[i] = f.Type
	}
	return constraints
}
