package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// Compound expression forms: do-blocks, block expressions, records, lists.
// Core Pratt parser (atoms, infix, application, parenthesized, lambda,
// case) is in parse_expr.go.

func (p *Parser) parseDo() syn.Expr {
	start := p.peek().S.Start
	p.expect(syn.TokDo)
	openTok := p.expect(syn.TokLBrace)
	var stmts []syn.Stmt
	p.parseBody("do-block", openTok.S, func() {
		stmts = append(stmts, p.parseStmt())
	})
	return &syn.ExprDo{
		Stmts: stmts,
		S:     span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseStmt() syn.Stmt {
	start := p.peek().S.Start
	// Try: _ <- expr (wildcard bind)
	if p.peek().Kind == syn.TokUnderscore {
		saved := p.pos
		p.advance()
		if p.peek().Kind == syn.TokLArrow {
			p.advance()
			comp := p.parseExpr()
			return &syn.StmtBind{
				Var: "_", Comp: comp,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		// Backtrack — it's an expression.
		p.pos = saved
	}
	// Try: name <- expr  or  name := expr
	if p.peek().Kind == syn.TokLower {
		name := p.peek().Text
		saved := p.pos
		p.advance()
		if p.peek().Kind == syn.TokLArrow {
			p.advance()
			comp := p.parseExpr()
			return &syn.StmtBind{
				Var: name, Comp: comp,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		if p.peek().Kind == syn.TokColonEq {
			p.advance()
			expr := p.parseExpr()
			return &syn.StmtPureBind{
				Var: name, Expr: expr,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		// Backtrack — it's an expression statement.
		p.pos = saved
	}
	expr := p.parseExpr()
	return &syn.StmtExpr{
		Expr: expr,
		S:    span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseBlock() syn.Expr {
	start := p.peek().S.Start
	openTok := p.expect(syn.TokLBrace)

	// {} → empty record
	if p.peek().Kind == syn.TokRBrace {
		p.advance()
		return &syn.ExprRecord{S: span.Span{Start: start, End: p.prevEnd()}}
	}

	// Disambiguate: record literal vs record update vs block.
	// Peek at first name followed by = (record) vs := (block).
	if p.peek().Kind == syn.TokLower {
		saved := p.pos
		p.advance() // skip name
		switch p.peek().Kind {
		case syn.TokColon:
			// name: ... → record literal
			p.pos = saved
			return p.parseRecordLiteral(start, openTok.S)
		case syn.TokPipe:
			// name | ... → record update (name is the record expression)
			p.pos = saved
			return p.parseRecordUpdate(start, openTok.S)
		default:
			// Could be: name := ... (block), or name as expression (record update with complex expr)
			p.pos = saved
		}
	}

	// Check for record update: expr | field = val, ...
	// We need to detect this by trying to parse an expression, then checking for |.
	// For simplicity, handle the case where the record is a single variable or expression.
	// Attempt: if after parsing the first expression we see |, it's a record update.
	var updateExpr syn.Expr
	p.speculate(func() bool {
		firstExpr := p.parseExpr()
		if p.peek().Kind != syn.TokPipe {
			return false
		}
		p.advance() // skip |
		updateExpr = p.parseRecordUpdateFields(start, openTok.S, firstExpr)
		return true
	})
	if updateExpr != nil {
		return updateExpr
	}

	// Block expression with bindings.
	// Handles: name := expr; (value binding)
	//          type Name Pats =: RHS; (associated type definition, inside impl body)
	//          data Name Pats =: Cons; (associated data definition, inside impl body)
	var binds []syn.AstBind
	var typeDefs []syn.ImplField
	for p.peek().Kind == syn.TokLower || p.peek().Kind == syn.TokType || p.peek().Kind == syn.TokData {
		saved := p.pos

		// Parse type/data associated definitions in impl bodies.
		// New syntax: type Elem := a;  or  data Elem := ListElem a | Empty;
		if p.peek().Kind == syn.TokType || p.peek().Kind == syn.TokData {
			tdStart := p.peek().S.Start
			p.advance() // consume type/data
			tdName := p.expectUpper()
			p.expect(syn.TokColonEq)
			var tdBody syn.TypeExpr
			tdBody = p.parseType()
			// For data family defs, skip | separated constructors
			for p.peek().Kind == syn.TokPipe {
				p.advance()
				for p.peek().Kind != syn.TokPipe && p.peek().Kind != syn.TokSemicolon &&
					p.peek().Kind != syn.TokRBrace && p.peek().Kind != syn.TokEOF {
					p.advance()
				}
			}
			typeDefs = append(typeDefs, syn.ImplField{
				Name:    tdName,
				IsType:  true,
				TypeDef: tdBody,
				S:       span.Span{Start: tdStart, End: p.prevEnd()},
			})
			if p.peek().Kind == syn.TokSemicolon {
				p.advance()
			}
			continue
		}

		name := p.peek().Text
		p.advance()
		if p.peek().Kind == syn.TokColonEq {
			p.advance()
			expr := p.parseExpr()
			binds = append(binds, syn.AstBind{
				Var: name, Expr: expr,
				S: span.Span{Start: span.Pos(saved), End: p.prevEnd()},
			})
			if p.peek().Kind == syn.TokSemicolon {
				p.advance()
			}
		} else {
			// Not a binding — backtrack and parse as expression.
			p.pos = saved
			break
		}
	}

	// If we have bindings/typeDefs and the next token is }, treat as binding-only block (impl body).
	if (len(binds) > 0 || len(typeDefs) > 0) && p.peek().Kind == syn.TokRBrace {
		p.advance() // consume }
		return &syn.ExprBlock{
			Binds:    binds,
			TypeDefs: typeDefs,
			S:        span.Span{Start: start, End: p.prevEnd()},
		}
	}
	body := p.parseExpr()
	p.expectClosing(syn.TokRBrace, openTok.S)
	return &syn.ExprBlock{
		Binds: binds, TypeDefs: typeDefs, Body: body,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseRecordLiteral(start span.Pos, openSpan span.Span) syn.Expr {
	var fields []syn.RecordField
	pg := p.newProgressGuard("record literal")
	for p.peek().Kind != syn.TokRBrace && p.peek().Kind != syn.TokEOF {
		if !pg.Begin() {
			break
		}
		fStart := p.peek().S.Start
		label := p.expectLower()
		p.expect(syn.TokColon)
		value := p.parseExpr()
		fields = append(fields, syn.RecordField{
			Label: label, Value: value,
			S: span.Span{Start: fStart, End: p.prevEnd()},
		})
		if p.peek().Kind == syn.TokComma {
			p.advance()
		} else {
			break
		}
	}
	p.expectClosing(syn.TokRBrace, openSpan)
	return &syn.ExprRecord{Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
}

func (p *Parser) parseRecordUpdate(start span.Pos, openSpan span.Span) syn.Expr {
	record := p.parseExpr()
	p.expect(syn.TokPipe)
	return p.parseRecordUpdateFields(start, openSpan, record)
}

func (p *Parser) parseRecordUpdateFields(start span.Pos, openSpan span.Span, record syn.Expr) syn.Expr {
	var updates []syn.RecordField
	pg := p.newProgressGuard("record update")
	for p.peek().Kind != syn.TokRBrace && p.peek().Kind != syn.TokEOF {
		if !pg.Begin() {
			break
		}
		fStart := p.peek().S.Start
		label := p.expectLower()
		p.expect(syn.TokColon)
		value := p.parseExpr()
		updates = append(updates, syn.RecordField{
			Label: label, Value: value,
			S: span.Span{Start: fStart, End: p.prevEnd()},
		})
		if p.peek().Kind == syn.TokComma {
			p.advance()
		} else {
			break
		}
	}
	p.expectClosing(syn.TokRBrace, openSpan)
	return &syn.ExprRecordUpdate{
		Record: record, Updates: updates,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseListLit() syn.Expr {
	start := p.peek().S.Start
	openTok := p.expect(syn.TokLBracket)
	var elems []syn.Expr
	if p.peek().Kind != syn.TokRBracket {
		elems = append(elems, p.parseExpr())
		for p.peek().Kind == syn.TokComma {
			p.advance()
			elems = append(elems, p.parseExpr())
		}
	}
	p.expectClosing(syn.TokRBracket, openTok.S)
	return &syn.ExprList{
		Elems: elems,
		S:     span.Span{Start: start, End: p.prevEnd()},
	}
}
