package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
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
	pg := p.newProgressGuard("type application chain")
	for p.isTypeAtomStart() {
		if !pg.Begin() {
			break
		}
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
	case syn.TokCase:
		return p.parseTypeCase()
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
					Label: syn.TupleLabel(i + 1),
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
	var typeDecls []syn.TyRowTypeDecl
	var tail *syn.TyExprVar

	pg := p.newProgressGuard("row type")
	for {
		if !pg.Begin() {
			break
		}
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
		// Associated type/data declaration: type Name :: Kind; or data Name :: Kind;
		if p.peek().Kind == syn.TokType || p.peek().Kind == syn.TokData {
			tdStart := p.peek().S.Start
			p.advance() // consume 'type' or 'data'
			tName := p.expectUpper()
			// Consume optional params (legacy: data Elem a :: Type)
			for p.peek().Kind == syn.TokLower {
				p.advance()
			}
			p.expect(syn.TokColonColon)
			tKind := p.parseKindExpr()
			typeDecls = append(typeDecls, syn.TyRowTypeDecl{
				Name:    tName,
				KindAnn: tKind,
				S:       span.Span{Start: tdStart, End: p.prevEnd()},
			})
			if p.peek().Kind == syn.TokSemicolon {
				p.advance()
			}
			continue
		}
		// Field: label: Type [@Mult] [:= Default];
		// Label can be lowercase (method) or uppercase (constructor).
		fieldStart := p.peek().S.Start
		var label string
		if p.peek().Kind == syn.TokLower {
			label = p.expectLower()
		} else if p.peek().Kind == syn.TokUpper {
			label = p.expectUpper()
		} else {
			p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected field label in row type")
			p.advance()
			continue
		}
		p.expect(syn.TokColon)
		ty := p.parseType()
		var mult syn.TypeExpr
		if p.peek().Kind == syn.TokAt {
			p.advance()
			mult = p.parseTypeAtom()
		}
		var dflt syn.Expr
		if p.peek().Kind == syn.TokColonEq {
			p.advance()
			dflt = p.parseExpr()
		}
		fields = append(fields, syn.TyRowField{
			Label: label, Type: ty, Mult: mult, Default: dflt,
			S: span.Span{Start: fieldStart, End: p.prevEnd()},
		})
		if p.peek().Kind == syn.TokComma || p.peek().Kind == syn.TokSemicolon {
			p.advance()
		}
	}
	p.expect(syn.TokRBrace)
	return &syn.TyExprRow{
		Fields: fields, TypeDecls: typeDecls, Tail: tail,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseTypeCase parses a type-level case expression (for closed type families).
//
//	case scrutinee { Pattern => Body; ... }
func (p *Parser) parseTypeCase() *syn.TyExprCase {
	start := p.peek().S.Start
	p.expect(syn.TokCase)
	scrutinee := p.parseTypeAtom()

	openTok := p.expect(syn.TokLBrace)
	var alts []syn.TyAlt
	p.parseBody("type case", openTok.S, func() {
		altStart := p.peek().S.Start
		// Parse pattern: stop before => (don't consume it as QualType)
		pattern := p.parseTypeApp()
		p.expect(syn.TokFatArrow)
		// Parse body: allow -> (function type) but not => (which delimits the next alt).
		body := p.parseTypeCaseBody()
		alts = append(alts, syn.TyAlt{
			Pattern: pattern,
			Body:    body,
			S:       span.Span{Start: altStart, End: p.prevEnd()},
		})
	})

	return &syn.TyExprCase{
		Scrutinee: scrutinee,
		Alts:      alts,
		S:         span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseTypeCaseBody parses a type expression inside a case alternative body.
// Allows -> (function arrow) but stops at => (which separates alternatives)
// and \  (which would start a forall — not valid in case bodies without parens).
func (p *Parser) parseTypeCaseBody() syn.TypeExpr {
	left := p.parseTypeApp()
	if p.peek().Kind == syn.TokArrow {
		p.advance()
		right := p.parseTypeCaseBody() // right-associative
		return &syn.TyExprArrow{
			From: left, To: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}
	return left
}

// desugarConstraintTuple delegates to the canonical syntax.DesugarConstraintTuple.
func desugarConstraintTuple(t syn.TypeExpr) []syn.TypeExpr {
	return syn.DesugarConstraintTuple(t)
}
