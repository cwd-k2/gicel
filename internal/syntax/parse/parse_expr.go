package parse

import (
	syn "github.com/cwd-k2/gicel/internal/syntax"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// --- Expressions ---

func (p *Parser) parseExpr() syn.Expr {
	if !p.enterRecurse() {
		return &syn.ExprError{S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	defer p.leaveRecurse()
	return p.parseAnnotation()
}

func (p *Parser) parseAnnotation() syn.Expr {
	e := p.parseInfix(0)
	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		ty := p.parseType()
		return &syn.ExprAnn{
			Expr: e, AnnType: ty,
			S: span.Span{Start: e.Span().Start, End: p.prevEnd()},
		}
	}
	return e
}

func (p *Parser) parseInfix(minPrec int) syn.Expr {
	left := p.parseApp()
	return p.continueInfix(left, minPrec)
}

// continueInfix parses the infix portion of an expression, given an
// already-parsed left operand. This enables parseParen to detect
// left operator sections between parseApp and infix continuation.
func (p *Parser) continueInfix(left syn.Expr, minPrec int) syn.Expr {
	prevNonePrec := -1 // precedence of last non-associative op, or -1
	for p.isInfixOp() {
		op := p.peek().Text
		fix := p.lookupFixity(op)
		if fix.Prec < minPrec {
			break
		}
		// Non-associative operators cannot chain at the same precedence,
		// even with a different operator. e.g. `1 == 2 /= 3` is an error.
		if prevNonePrec >= 0 && fix.Prec == prevNonePrec {
			p.addErrorCode(errs.ErrInvalidOperator, "cannot mix non-associative operators of equal precedence")
			break
		}
		p.advance()
		nextMin := fix.Prec + 1
		if fix.Assoc == syn.AssocRight {
			nextMin = fix.Prec
		}
		right := p.parseInfix(nextMin)
		left = &syn.ExprInfix{
			Left: left, Op: op, Right: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
		if fix.Assoc == syn.AssocNone {
			prevNonePrec = fix.Prec
		} else {
			prevNonePrec = -1
		}
	}
	return left
}

// isInfixOp returns true if the current token can act as an infix operator.
// syn.TokDot (.) is treated as an operator in expression context.
func (p *Parser) isInfixOp() bool {
	k := p.peek().Kind
	return k == syn.TokOp || k == syn.TokDot
}

func (p *Parser) parseApp() syn.Expr {
	f := p.parseAtom()
	if f == nil {
		p.addErrorCode(errs.ErrMissingBody, "expected expression")
		return &syn.ExprError{S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	for (p.isAtomStart() || p.peek().Kind == syn.TokAt) && !p.atStmtBoundary() {
		if p.peek().Kind == syn.TokAt {
			p.advance()
			ty := p.parseTypeAtom()
			f = &syn.ExprTyApp{
				Expr: f, TyArg: ty,
				S: span.Span{Start: f.Span().Start, End: p.prevEnd()},
			}
		} else {
			arg := p.parseAtom()
			f = &syn.ExprApp{
				Fun: f, Arg: arg,
				S: span.Span{Start: f.Span().Start, End: arg.Span().End},
			}
		}
	}
	return f
}

func (p *Parser) parseAtom() syn.Expr {
	var e syn.Expr
	switch p.peek().Kind {
	case syn.TokLower:
		tok := p.peek()
		p.advance()
		e = &syn.ExprVar{Name: tok.Text, S: tok.S}
	case syn.TokUpper:
		tok := p.peek()
		p.advance()
		// Qualified name: Upper.lower → syn.ExprQualVar, Upper.Upper → syn.ExprQualCon
		// Requires all three tokens to be adjacent (no whitespace).
		if e2 := p.tryQualifiedExpr(tok); e2 != nil {
			e = e2
		} else {
			e = &syn.ExprCon{Name: tok.Text, S: tok.S}
		}
	case syn.TokLParen:
		e = p.parseParen()
	case syn.TokBackslash:
		e = p.parseLambda()
	case syn.TokCase:
		e = p.parseCase()
	case syn.TokDo:
		e = p.parseDo()
	case syn.TokLBrace:
		e = p.parseBlock()
	case syn.TokIntLit:
		tok := p.peek()
		p.advance()
		e = &syn.ExprIntLit{Value: tok.Text, S: tok.S}
	case syn.TokDoubleLit:
		tok := p.peek()
		p.advance()
		e = &syn.ExprDoubleLit{Value: tok.Text, S: tok.S}
	case syn.TokStrLit:
		tok := p.peek()
		p.advance()
		e = &syn.ExprStrLit{Value: tok.Text, S: tok.S}
	case syn.TokRuneLit:
		tok := p.peek()
		p.advance()
		runes := []rune(tok.Text)
		var r rune
		if len(runes) > 0 {
			r = runes[0]
		}
		e = &syn.ExprRuneLit{Value: r, S: tok.S}
	case syn.TokLBracket:
		e = p.parseListLit()
	default:
		return nil
	}
	// Chain record projections: r.#x.#y → Project(Project(r, "x"), "y")
	for p.peek().Kind == syn.TokDotHash {
		p.advance()
		label := p.expectLower()
		e = &syn.ExprProject{Record: e, Label: label, S: span.Span{Start: e.Span().Start, End: p.prevEnd()}}
	}
	return e
}

func (p *Parser) parseParen() syn.Expr {
	if !p.enterRecurse() {
		return &syn.ExprError{S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	defer p.leaveRecurse()
	start := p.peek().S.Start
	p.expect(syn.TokLParen)

	// () → unit (empty record)
	if p.peek().Kind == syn.TokRParen {
		p.advance()
		return &syn.ExprRecord{S: span.Span{Start: start, End: p.prevEnd()}}
	}

	// (op) → operator as value reference
	// (op expr) → right section: \x -> x op expr
	if p.peek().Kind == syn.TokOp || p.peek().Kind == syn.TokDot {
		opTok := p.peek()
		opName := opTok.Text
		if opTok.Kind == syn.TokDot {
			opName = "."
		}
		// Check for (op) — operator as value
		if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == syn.TokRParen {
			p.advance()
			p.advance()
			return &syn.ExprVar{Name: opName, S: span.Span{Start: start, End: p.prevEnd()}}
		}
		// Try right section: (op expr)
		var section syn.Expr
		p.speculate(func() bool {
			p.advance() // skip op
			arg := p.parseExpr()
			if p.peek().Kind != syn.TokRParen {
				return false
			}
			p.advance()
			section = &syn.ExprSection{
				Op: opName, Arg: arg, IsRight: true,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
			return true
		})
		if section != nil {
			return section
		}
	}

	// Parse the first sub-expression without infix operators,
	// so we can detect left sections like (1 +).
	firstApp := p.parseApp()
	if firstApp == nil {
		firstApp = &syn.ExprError{S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}

	// (e op) → left section: \x -> e op x
	if p.isInfixOp() && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == syn.TokRParen {
		opTok := p.peek()
		opName := opTok.Text
		if opTok.Kind == syn.TokDot {
			opName = "."
		}
		p.advance() // skip op
		p.advance() // skip )
		return &syn.ExprSection{
			Op: opName, Arg: firstApp, IsRight: false,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// Continue with infix + annotation parsing.
	e := p.continueInfix(firstApp, 0)
	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		ty := p.parseType()
		e = &syn.ExprAnn{Expr: e, AnnType: ty, S: span.Span{Start: e.Span().Start, End: p.prevEnd()}}
	}

	// (e) → grouping
	if p.peek().Kind == syn.TokRParen {
		p.advance()
		return &syn.ExprParen{Inner: e, S: span.Span{Start: start, End: p.prevEnd()}}
	}

	// (e1, e2, ...) → tuple (desugars to record with _1, _2, ...)
	if p.peek().Kind == syn.TokComma {
		elems := []syn.Expr{e}
		for p.peek().Kind == syn.TokComma {
			p.advance()
			elems = append(elems, p.parseExpr())
		}
		p.expect(syn.TokRParen)
		fields := make([]syn.RecordField, len(elems))
		for i, el := range elems {
			fields[i] = syn.RecordField{
				Label: types.TupleLabel(i + 1),
				Value: el,
				S:     el.Span(),
			}
		}
		return &syn.ExprRecord{Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
	}

	p.expect(syn.TokRParen)
	return &syn.ExprParen{Inner: e, S: span.Span{Start: start, End: p.prevEnd()}}
}

func (p *Parser) parseLambda() syn.Expr {
	start := p.peek().S.Start
	p.expect(syn.TokBackslash)
	var params []syn.Pattern
	for p.isPatternAtomStart() {
		params = append(params, p.parsePatternAtom())
	}
	p.expect(syn.TokDot)
	body := p.parseExpr()
	return &syn.ExprLam{
		Params: params, Body: body,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseCase() syn.Expr {
	start := p.peek().S.Start
	p.expect(syn.TokCase)
	p.noBraceAtom = true
	scrut := p.parseExpr()
	p.noBraceAtom = false
	p.expect(syn.TokLBrace)
	var alts []syn.AstAlt
	p.parseBody("case expression", func() {
		alts = append(alts, p.parseAlt())
	})
	return &syn.ExprCase{
		Scrutinee: scrut, Alts: alts,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseAlt() syn.AstAlt {
	start := p.peek().S.Start
	pat := p.parsePattern()
	p.expect(syn.TokArrow)
	body := p.parseExpr()
	return syn.AstAlt{
		Pattern: pat, Body: body,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// tryQualifiedExpr checks if the current position forms a qualified name
// Upper.lower or Upper.Upper where all tokens are adjacent (no whitespace).
// The Upper token has already been consumed and is passed as prevTok.
// Returns nil if no qualified name is formed.
func (p *Parser) tryQualifiedExpr(prevTok syn.Token) syn.Expr {
	if p.peek().Kind != syn.TokDot {
		return nil
	}
	dotTok := p.peek()
	if !tokensAdjacent(prevTok, dotTok) {
		return nil
	}
	if p.pos+1 >= len(p.tokens) {
		return nil
	}
	nextTok := p.tokens[p.pos+1]
	if !tokensAdjacent(dotTok, nextTok) {
		return nil
	}
	switch nextTok.Kind {
	case syn.TokLower:
		p.advance() // consume .
		p.advance() // consume lower
		return &syn.ExprQualVar{Qualifier: prevTok.Text, Name: nextTok.Text,
			S: span.Span{Start: prevTok.S.Start, End: nextTok.S.End}}
	case syn.TokUpper:
		p.advance() // consume .
		p.advance() // consume Upper
		return &syn.ExprQualCon{Qualifier: prevTok.Text, Name: nextTok.Text,
			S: span.Span{Start: prevTok.S.Start, End: nextTok.S.End}}
	default:
		return nil
	}
}

func (p *Parser) isAtomStart() bool {
	if p.atStmtBoundary() {
		return false
	}
	k := p.peek().Kind
	if p.noBraceAtom && k == syn.TokLBrace {
		return false
	}
	return k == syn.TokLower || k == syn.TokUpper || k == syn.TokLParen || k == syn.TokBackslash || k == syn.TokLBrace || k == syn.TokCase || k == syn.TokDo || k == syn.TokIntLit || k == syn.TokDoubleLit || k == syn.TokStrLit || k == syn.TokRuneLit || k == syn.TokLBracket
}
