package parse

import (
	"fmt"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/span"
)

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
	return p.continueInfix(left, minPrec)
}

// continueInfix parses the infix portion of an expression, given an
// already-parsed left operand. This enables parseParen to detect
// left operator sections between parseApp and infix continuation.
func (p *Parser) continueInfix(left Expr, minPrec int) Expr {
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
	// Chain record projections: r.#x.#y → Project(Project(r, "x"), "y")
	for p.peek().Kind == TokDotHash {
		p.advance()
		label := p.expectLower()
		e = &ExprProject{Record: e, Label: label, S: span.Span{Start: e.Span().Start, End: p.prevEnd()}}
	}
	return e
}

func (p *Parser) parseParen() Expr {
	if !p.enterRecurse() {
		return &ExprVar{Name: "<error>", S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	defer p.leaveRecurse()
	start := p.peek().S.Start
	p.expect(TokLParen)

	// () → unit (empty record)
	if p.peek().Kind == TokRParen {
		p.advance()
		return &ExprRecord{S: span.Span{Start: start, End: p.prevEnd()}}
	}

	// (op) → operator as value reference
	// (op expr) → right section: \x -> x op expr
	if p.peek().Kind == TokOp || p.peek().Kind == TokDot {
		opTok := p.peek()
		opName := opTok.Text
		if opTok.Kind == TokDot {
			opName = "."
		}
		// Check for (op) — operator as value
		if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == TokRParen {
			p.advance()
			p.advance()
			return &ExprVar{Name: opName, S: span.Span{Start: start, End: p.prevEnd()}}
		}
		// Try right section: (op expr)
		saved := p.pos
		savedErrLen := p.errors.Len()
		p.advance() // skip op
		arg := p.parseExpr()
		if p.peek().Kind == TokRParen {
			p.advance()
			return &ExprSection{
				Op: opName, Arg: arg, IsRight: true,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		// Not a section — backtrack.
		p.pos = saved
		p.errors.Truncate(savedErrLen)
	}

	// Parse the first sub-expression without infix operators,
	// so we can detect left sections like (1 +).
	firstApp := p.parseApp()
	if firstApp == nil {
		firstApp = &ExprVar{Name: "<error>", S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}

	// (e op) → left section: \x -> e op x
	if p.isInfixOp() && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == TokRParen {
		opTok := p.peek()
		opName := opTok.Text
		if opTok.Kind == TokDot {
			opName = "."
		}
		p.advance() // skip op
		p.advance() // skip )
		return &ExprSection{
			Op: opName, Arg: firstApp, IsRight: false,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// Continue with infix + annotation parsing.
	e := p.continueInfix(firstApp, 0)
	if p.peek().Kind == TokColonColon {
		p.advance()
		ty := p.parseType()
		e = &ExprAnn{Expr: e, AnnType: ty, S: span.Span{Start: e.Span().Start, End: p.prevEnd()}}
	}

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
	savedErrLen := p.errors.Len()
	firstExpr := p.parseExpr()
	if p.peek().Kind == TokPipe {
		p.advance() // skip |
		return p.parseRecordUpdateFields(start, firstExpr)
	}
	// Not a record update — restore and discard phantom errors.
	p.pos = savedForUpdate
	p.errors.Truncate(savedErrLen)

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
	case TokIntLit, TokStrLit, TokRuneLit:
		return p.parseLitPattern()
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
	case TokUpper:
		// Bare constructor as pattern atom (0-arg constructor in nested position).
		tok := p.peek()
		p.advance()
		return &PatCon{Con: tok.Text, S: tok.S}
	case TokIntLit, TokStrLit, TokRuneLit:
		return p.parseLitPattern()
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
	return k == TokLower || k == TokUnderscore || k == TokLParen || k == TokUpper || k == TokIntLit || k == TokStrLit || k == TokRuneLit
}

func (p *Parser) parseLitPattern() Pattern {
	tok := p.peek()
	p.advance()
	var kind LitKind
	switch tok.Kind {
	case TokIntLit:
		kind = LitInt
	case TokStrLit:
		kind = LitString
	case TokRuneLit:
		kind = LitRune
	}
	return &PatLit{Value: tok.Text, Kind: kind, S: tok.S}
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
