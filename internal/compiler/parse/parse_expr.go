package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// --- Expressions ---

func (p *Parser) parseExpr() syn.Expr {
	if !p.enterRecurse() {
		return &syn.ExprError{S: span.Span{Start: p.peek().S.Start, End: p.peek().S.Start}}
	}
	defer p.leaveRecurse()
	return p.parseAnnotation()
}

func (p *Parser) parseAnnotation() syn.Expr {
	e := p.parseInfix()
	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		ty := p.parseType()
		e = &syn.ExprAnn{
			Expr: e, AnnType: ty,
			S: span.Span{Start: e.Span().Start, End: p.prevEnd()},
		}
	}
	// Scoped evidence injection: value => expr
	// Binds loosely (below annotation, above nothing). Right-associative:
	// d1 => d2 => expr = d1 => (d2 => expr)
	if p.peek().Kind == syn.TokFatArrow && !p.atStmtBoundary() {
		p.advance()
		body := p.parseExpr() // right-associative: recurse into full parseExpr
		return &syn.ExprEvidence{
			Dict: e,
			Body: body,
			S:    span.Span{Start: e.Span().Start, End: p.prevEnd()},
		}
	}
	return e
}

func (p *Parser) parseInfix() syn.Expr {
	left := p.parseApp()
	return p.continueInfix(left)
}

// continueInfix collects a flat operator spine. Fixity resolution
// happens post-parse via ResolveFixity (shunting-yard).
func (p *Parser) continueInfix(left syn.Expr) syn.Expr {
	if !p.isInfixOp() {
		return left
	}
	operands := []syn.Expr{left}
	var ops []string
	var opSpans []span.Span
	pg := p.newProgressGuard("infix chain")
	for p.isInfixOp() {
		if !pg.Begin() {
			break
		}
		opTok := p.peek()
		opName := opTok.Text
		if opTok.Kind == syn.TokDot {
			opName = "."
		}
		ops = append(ops, opName)
		opSpans = append(opSpans, opTok.S)
		p.advance()
		operands = append(operands, p.parseApp())
	}
	if len(ops) == 1 {
		// Single operator: produce ExprInfix directly (no fixity needed).
		return &syn.ExprInfix{
			Left: operands[0], Op: ops[0], OpSpan: opSpans[0], Right: operands[1],
			S: span.Span{Start: operands[0].Span().Start, End: operands[1].Span().End},
		}
	}
	return &syn.ExprInfixSpine{
		Operands: operands, Ops: ops, OpSpans: opSpans,
		S: span.Span{Start: operands[0].Span().Start, End: operands[len(operands)-1].Span().End},
	}
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
		tok := p.peek()
		// Unary minus: -e. Desugar pass converts to negate application.
		if tok.Kind == syn.TokOp && tok.Text == "-" {
			p.advance() // consume -
			arg := p.parseAtom()
			if arg == nil {
				p.addErrorCode(diagnostic.ErrMissingBody, "expected expression after '-'")
				return &syn.ExprError{S: span.Span{Start: tok.S.Start, End: tok.S.Start}}
			}
			f = &syn.ExprNegate{
				Expr: arg,
				S:    span.Span{Start: tok.S.Start, End: p.prevEnd()},
			}
		} else {
			p.addErrorCode(diagnostic.ErrMissingBody, "expected expression")
			return &syn.ExprError{S: span.Span{Start: tok.S.Start, End: tok.S.Start}}
		}
	}
	pg := p.newProgressGuard("application chain")
	for (p.isAtomStart() || p.peek().Kind == syn.TokAt) && !p.atStmtBoundary() {
		if !pg.Begin() {
			break
		}
		if p.peek().Kind == syn.TokAt {
			p.advance()
			ty := p.parseTypeAtom()
			f = &syn.ExprTyApp{
				Expr: f, TyArg: ty,
				S: span.Span{Start: f.Span().Start, End: p.prevEnd()},
			}
		} else {
			arg := p.parseAtom()
			if arg == nil {
				// parseAtom returned nil for an atom-start token — it consumed
				// nothing we can use. Break to avoid a nil-deref on arg.Span()
				// and let the outer diagnostic stand.
				break
			}
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
	case syn.TokIf:
		e = p.parseIf()
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
		return &syn.ExprError{S: span.Span{Start: p.peek().S.Start, End: p.peek().S.Start}}
	}
	defer p.leaveRecurse()
	// Inside parens, braces are always valid atom starts — a case expression
	// like (case x { ... }) must see { as starting alts, not as an outer
	// case's opening brace. Save and reset noBraceAtom (V10 fix).
	savedNoBrace := p.noBraceAtom
	p.noBraceAtom = false
	defer func() { p.noBraceAtom = savedNoBrace }()
	start := p.peek().S.Start
	openTok := p.expect(syn.TokLParen)

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
		if p.tb.peekAt(1).Kind == syn.TokRParen {
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
				Op: opName, OpSpan: opTok.S, Arg: arg, IsRight: true,
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
		firstApp = &syn.ExprError{S: span.Span{Start: p.peek().S.Start, End: p.peek().S.Start}}
	}

	// (e op) → left section: \x -> e op x
	if p.isInfixOp() && p.tb.peekAt(1).Kind == syn.TokRParen {
		opTok := p.peek()
		opName := opTok.Text
		if opTok.Kind == syn.TokDot {
			opName = "."
		}
		p.advance() // skip op
		p.advance() // skip )
		return &syn.ExprSection{
			Op: opName, OpSpan: opTok.S, Arg: firstApp, IsRight: false,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// Continue with infix + annotation parsing.
	e := p.continueInfix(firstApp)
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

	// (e1, e2, ...) → tuple. Desugar pass converts to record with _1, _2, ...
	if p.peek().Kind == syn.TokComma {
		elems := []syn.Expr{e}
		for p.peek().Kind == syn.TokComma {
			p.advance()
			elems = append(elems, p.parseExpr())
		}
		p.expectClosing(syn.TokRParen, openTok.S)
		return &syn.ExprTuple{Elems: elems, S: span.Span{Start: start, End: p.prevEnd()}}
	}

	p.expectClosing(syn.TokRParen, openTok.S)
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
	savedNoBrace := p.noBraceAtom
	p.noBraceAtom = true
	scrut := p.parseExpr()
	p.noBraceAtom = savedNoBrace
	openTok := p.expect(syn.TokLBrace)
	var alts []syn.AstAlt
	p.parseBody("case expression", openTok.S, func() {
		alts = append(alts, p.parseAlt())
	})
	return &syn.ExprCase{
		Scrutinee: scrut, Alts: alts,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseIf desugars: if cond then t else f → case cond { True => t; False => f }
func (p *Parser) parseIf() syn.Expr {
	start := p.peek().S.Start
	p.expect(syn.TokIf)
	cond := p.parseExpr()
	var thenExpr, elseExpr syn.Expr
	if p.peek().Kind == syn.TokThen {
		p.advance()
		thenExpr = p.parseExpr()
	} else if p.peek().Kind == syn.TokElse {
		// Missing 'then' but 'else' present — the condition parse consumed
		// the then-expression as part of the condition. Report a single error.
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected 'then' keyword in if-expression")
		thenExpr = &syn.ExprError{S: span.Span{Start: p.peek().S.Start, End: p.peek().S.Start}}
	} else {
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected 'then' keyword in if-expression")
		thenExpr = p.parseExpr()
	}
	if p.peek().Kind != syn.TokElse {
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected 'else' after 'then' branch")
		if p.peek().Kind != syn.TokEOF {
			p.advance()
		}
	} else {
		p.advance()
	}
	elseExpr = p.parseExpr()
	end := p.prevEnd()
	return &syn.ExprIf{
		Cond: cond,
		Then: thenExpr,
		Else: elseExpr,
		S:    span.Span{Start: start, End: end},
	}
}

func (p *Parser) parseAlt() syn.AstAlt {
	start := p.peek().S.Start
	pat := p.parsePattern()
	if p.peek().Kind != syn.TokFatArrow {
		// Missing =>: report and recover to next alt boundary.
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected => in case alternative")
		p.syncToStmtBoundary()
		return syn.AstAlt{
			Pattern: pat,
			Body:    &syn.ExprError{S: span.Span{Start: p.peek().S.Start, End: p.peek().S.Start}},
			S:       span.Span{Start: start, End: p.prevEnd()},
		}
	}
	p.advance() // consume =>
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
	nextTok := p.tb.peekAt(1)
	if nextTok.Kind == syn.TokEOF {
		return nil
	}
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
	return k == syn.TokLower || k == syn.TokUpper ||
		k == syn.TokLParen || k == syn.TokLBrace || k == syn.TokLBracket ||
		k == syn.TokBackslash || k == syn.TokCase || k == syn.TokIf || k == syn.TokDo ||
		k == syn.TokIntLit || k == syn.TokDoubleLit || k == syn.TokStrLit || k == syn.TokRuneLit
}
