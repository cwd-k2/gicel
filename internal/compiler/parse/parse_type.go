package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"
	lty "github.com/cwd-k2/gicel/internal/lang/types"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

func (p *Parser) parseType() syn.TypeExpr {
	if !p.enterRecurse() {
		return &syn.TyExprError{S: span.Span{Start: p.peek().S.Start, End: p.peek().S.Start}}
	}
	defer p.leaveRecurse()
	return p.parseTypeArrow()
}

func (p *Parser) parseTypeArrow() syn.TypeExpr {
	if p.peek().Kind == syn.TokBackslash {
		return p.parseForallType()
	}
	left := p.parseTypeDashPipe()
	// Type equality constraint: a ~ b (binds tighter than => and ->).
	if p.peek().Kind == syn.TokTilde {
		p.advance()
		right := p.parseTypeApp()
		left = &syn.TyExprEq{
			Lhs: left, Rhs: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}
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

// parseTypeDashPipe parses the -| (type application) level.
// Binds tighter than ~ / => / -> but looser than juxtaposition.
// Right-associative: F -| A -| B  =  F (A B)  =  TyExprApp(F, TyExprApp(A, B)).
func (p *Parser) parseTypeDashPipe() syn.TypeExpr {
	left := p.parseTypeApp()
	if p.peek().Kind != syn.TokDashPipe {
		return left
	}
	p.advance()
	right := p.parseTypeDashPipe() // right-associative recursion
	return &syn.TyExprApp{
		Fun: left, Arg: right,
		S: span.Span{Start: left.Span().Start, End: right.Span().End},
	}
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
			kind := p.parseKindAnnotation()
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

// parseKindAnnotation parses a kind annotation as a TypeExpr.
// Kind keywords (Type, Row, Constraint, Kind) become TyExprCon nodes;
// arrows and parentheses use the standard TypeExpr AST.
func (p *Parser) parseKindAnnotation() syn.TypeExpr {
	left := p.parseKindAtom()
	if p.peek().Kind == syn.TokArrow {
		p.advance()
		right := p.parseKindAnnotation() // right-associative
		return &syn.TyExprArrow{
			From: left, To: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}
	return left
}

// parseKindAtom parses an atomic kind as a TypeExpr: Type, Row, Constraint,
// Kind, user-defined kind names, kind variables, or (K).
func (p *Parser) parseKindAtom() syn.TypeExpr {
	switch {
	case p.peek().Kind == syn.TokUpper:
		// All uppercase names (Type, Row, Constraint, Kind, user-defined) → TyExprCon.
		tok := p.peek()
		p.advance()
		head := &syn.TyExprCon{Name: tok.Text, S: tok.S}
		// Kind application: "Type l" where l is a level/kind variable.
		if p.peek().Kind == syn.TokLower {
			argTok := p.peek()
			p.advance()
			arg := &syn.TyExprVar{Name: argTok.Text, S: argTok.S}
			return &syn.TyExprApp{
				Fun: head, Arg: arg,
				S: span.Span{Start: tok.S.Start, End: argTok.S.End},
			}
		}
		return head
	case p.peek().Kind == syn.TokLower:
		// Kind variable reference (e.g., k in "\ (k: Kind). k -> Type")
		tok := p.peek()
		p.advance()
		return &syn.TyExprVar{Name: tok.Text, S: tok.S}
	case p.peek().Kind == syn.TokLParen:
		if !p.enterRecurse() {
			tok := p.peek()
			return &syn.TyExprCon{Name: "Type", S: tok.S}
		}
		start := p.peek().S.Start
		p.advance()
		k := p.parseKindAnnotation()
		p.expect(syn.TokRParen)
		p.leaveRecurse()
		return &syn.TyExprParen{Inner: k, S: span.Span{Start: start, End: p.prevEnd()}}
	default:
		p.addErrorCode(diagnostic.ErrExpectedType, "expected kind (Type, Row, or K -> K)")
		tok := p.peek()
		p.advance()
		return &syn.TyExprCon{Name: "Type", S: tok.S}
	}
}

func (p *Parser) parseTypeApp() syn.TypeExpr {
	f := p.parseTypeAtom()
	// Detect @ grade annotation immediately after Computation/Thunk head.
	// @ in other positions is reserved for row field multiplicity annotations.
	gradeAllowed := isCompOrThunkHead(f)
	pg := p.newProgressGuard("type application chain")
	for p.isTypeAtomStart() || (gradeAllowed && p.peek().Kind == syn.TokAt) {
		if !pg.Begin() {
			break
		}
		isGrade := false
		if gradeAllowed && p.peek().Kind == syn.TokAt {
			p.advance()
			isGrade = true
			gradeAllowed = false // only the first @ is a grade
		}
		arg := p.parseTypeAtom()
		f = &syn.TyExprApp{
			Fun: f, Arg: arg, IsGrade: isGrade,
			S: span.Span{Start: f.Span().Start, End: arg.Span().End},
		}
		if isGrade {
			gradeAllowed = false
		}
	}
	return f
}

func isCompOrThunkHead(t syn.TypeExpr) bool {
	if con, ok := t.(*syn.TyExprCon); ok {
		return con.Name == "Computation" || con.Name == "Thunk"
	}
	return false
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
			nextTok := p.tb.peekAt(1)
			if nextTok.Kind != syn.TokEOF && tokensAdjacent(dotTok, nextTok) && nextTok.Kind == syn.TokUpper {
				p.advance() // consume .
				p.advance() // consume Upper
				return &syn.TyExprQualCon{Qualifier: tok.Text, Name: nextTok.Text,
					S: span.Span{Start: tok.S.Start, End: nextTok.S.End}}
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
				Fun: &syn.TyExprCon{Name: lty.TyConRecord, S: s},
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
				Fun: &syn.TyExprCon{Name: lty.TyConRecord, S: s},
				Arg: &syn.TyExprRow{Fields: fields, S: s},
				S:   s,
			}
		}
		p.expect(syn.TokRParen)
		return &syn.TyExprParen{Inner: ty, S: span.Span{Start: start, End: p.prevEnd()}}
	case syn.TokLBrace:
		return p.parseRowType()
	case syn.TokLabelLit:
		tok := p.peek()
		p.advance()
		return &syn.TyExprLabelLit{Label: tok.Text, S: tok.S}
	default:
		p.addErrorCode(diagnostic.ErrExpectedType, "expected type")
		tok := p.peek()
		p.advance()
		return &syn.TyExprError{S: tok.S}
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
		// Associated type/form declaration: type Name :: Kind; or form Name :: Kind;
		if p.peek().Kind == syn.TokType || p.peek().Kind == syn.TokForm {
			tdStart := p.peek().S.Start
			p.advance() // consume 'type' or 'form'
			tName := p.expectUpper()
			// Consume optional params (legacy: form Elem a :: Type)
			for p.peek().Kind == syn.TokLower {
				p.advance()
			}
			p.expect(syn.TokColonColon)
			tKind := p.parseKindAnnotation()
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
	left := p.parseTypeDashPipe()
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
