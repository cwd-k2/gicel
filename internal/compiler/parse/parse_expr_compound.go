package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
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
		p.tb.save()
		p.advance()
		if p.peek().Kind == syn.TokLArrow {
			p.tb.commit()
			p.advance()
			comp := p.parseExpr()
			return &syn.StmtBind{
				Pat: &syn.PatWild{S: span.Span{Start: start, End: start + 1}}, Comp: comp,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		// Backtrack — it's an expression.
		p.tb.restore()
	}
	// Try: name <- expr  or  name := expr
	if p.peek().Kind == syn.TokLower {
		name := p.peek().Text
		nameTok := p.peek()
		p.tb.save()
		p.advance()
		if p.peek().Kind == syn.TokLArrow {
			p.tb.commit()
			p.advance()
			comp := p.parseExpr()
			return &syn.StmtBind{
				Pat: &syn.PatVar{Name: name, S: nameTok.S}, Comp: comp,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		if p.peek().Kind == syn.TokColonEq {
			p.tb.commit()
			p.advance()
			expr := p.parseExpr()
			return &syn.StmtPureBind{
				Pat: &syn.PatVar{Name: name, S: nameTok.S}, Expr: expr,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
		// Backtrack — it's an expression statement.
		p.tb.restore()
	}
	// Try: pattern <- expr  or  pattern := expr (irrefutable pattern bind)
	if p.peek().Kind == syn.TokLParen {
		var result syn.Stmt
		p.speculate(func() bool {
			pat := p.parsePattern()
			switch p.peek().Kind {
			case syn.TokLArrow:
				if !syn.IsIrrefutable(pat) {
					p.addErrorCode(diagnostic.ErrInvalidPattern, "refutable pattern in binding; only irrefutable patterns (tuples, records, wildcards) are allowed")
				}
				p.advance()
				comp := p.parseExpr()
				result = &syn.StmtBind{
					Pat: pat, Comp: comp,
					S: span.Span{Start: start, End: p.prevEnd()},
				}
				return true
			case syn.TokColonEq:
				if !syn.IsIrrefutable(pat) {
					p.addErrorCode(diagnostic.ErrInvalidPattern, "refutable pattern in binding; only irrefutable patterns (tuples, records, wildcards) are allowed")
				}
				p.advance()
				expr := p.parseExpr()
				result = &syn.StmtPureBind{
					Pat: pat, Expr: expr,
					S: span.Span{Start: start, End: p.prevEnd()},
				}
				return true
			}
			return false
		})
		if result != nil {
			return result
		}
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
		p.tb.save()
		nameTok := p.peek()
		p.advance() // skip name
		switch p.peek().Kind {
		case syn.TokColon:
			// name: ... → record literal
			p.tb.restore()
			return p.parseRecordLiteral(start, openTok.S)
		case syn.TokPipe:
			// name | ... → record update (name is the record expression)
			p.tb.commit()
			p.advance() // skip |
			record := &syn.ExprVar{Name: nameTok.Text, S: nameTok.S}
			return p.parseRecordUpdateFields(start, openTok.S, record)
		default:
			// Could be: name := ... (block), or name as expression (record update with complex expr)
			p.tb.restore()
		}
	}

	// Check for record update with complex expression: { expr | field = val, ... }
	// The simple lower-name case is handled above with a 2-token lookahead.
	// For general expressions (applications, projections, etc.) we must speculatively
	// parse the full expression to find | — fixed lookahead cannot determine where
	// a variable-length expression ends.
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
	//          pattern := expr; (irrefutable pattern binding)
	//          type Name Pats =: RHS; (associated type definition, inside impl body)
	//          form Name Pats =: Cons; (associated form definition, inside impl body)
	var binds []syn.Bind
	var typeDefs []syn.ImplField
	for p.peek().Kind == syn.TokLower || p.peek().Kind == syn.TokType || p.peek().Kind == syn.TokForm || p.peek().Kind == syn.TokLParen {
		bindStart := p.peek().S.Start
		p.tb.save()

		// Parse type/form associated definitions in impl bodies.
		// New syntax: type Elem := a;  or  form Elem := ListElem a | Empty;
		if p.peek().Kind == syn.TokType || p.peek().Kind == syn.TokForm {
			p.tb.commit()
			tdStart := p.peek().S.Start
			isData := p.peek().Kind == syn.TokForm
			p.advance() // consume type/form
			tdName := p.expectUpper()
			p.expect(syn.TokColonEq)
			var tdBody syn.TypeExpr
			tdBody = p.parseType()
			// Multi-constructor data family defs: only the first constructor is captured.
			// Emit a diagnostic for additional | constructors rather than silently dropping.
			if p.peek().Kind == syn.TokPipe {
				p.addErrorCode(diagnostic.ErrParseSyntax,
					"multi-constructor associated form families are not yet supported; only the first constructor is used")
				for p.peek().Kind == syn.TokPipe {
					p.advance()
					for p.peek().Kind != syn.TokPipe && p.peek().Kind != syn.TokSemicolon &&
						p.peek().Kind != syn.TokRBrace && p.peek().Kind != syn.TokEOF {
						p.advance()
					}
				}
			}
			typeDefs = append(typeDefs, syn.ImplField{
				Name:    tdName,
				IsType:  !isData,
				IsData:  isData,
				TypeDef: tdBody,
				S:       span.Span{Start: tdStart, End: p.prevEnd()},
			})
			if p.peek().Kind == syn.TokSemicolon {
				p.advance()
			}
			continue
		}

		// Try pattern binding: (pat) := expr
		if p.peek().Kind == syn.TokLParen {
			p.tb.commit()
			matched := p.speculate(func() bool {
				pat := p.parsePattern()
				if p.peek().Kind != syn.TokColonEq {
					return false
				}
				if !syn.IsIrrefutable(pat) {
					p.addErrorCode(diagnostic.ErrInvalidPattern, "refutable pattern in binding; only irrefutable patterns (tuples, records, wildcards) are allowed")
				}
				p.advance()
				expr := p.parseExpr()
				binds = append(binds, syn.Bind{
					Pat: pat, Expr: expr,
					S: span.Span{Start: bindStart, End: p.prevEnd()},
				})
				if p.peek().Kind == syn.TokSemicolon {
					p.advance()
				}
				return true
			})
			if !matched {
				break
			}
			continue
		}

		name := p.peek().Text
		nameTok := p.peek()
		p.advance()
		if p.peek().Kind == syn.TokColonEq {
			p.tb.commit()
			p.advance()
			expr := p.parseExpr()
			binds = append(binds, syn.Bind{
				Pat: &syn.PatVar{Name: name, S: nameTok.S}, Expr: expr,
				S: span.Span{Start: bindStart, End: p.prevEnd()},
			})
			if p.peek().Kind == syn.TokSemicolon {
				p.advance()
			}
		} else {
			// Not a binding — backtrack and parse as expression.
			p.tb.restore()
			break
		}
	}

	// If we have bindings/typeDefs and the next token is }, treat as binding-only block (impl body).
	if (len(binds) > 0 || len(typeDefs) > 0) && p.peek().Kind == syn.TokRBrace {
		p.expectClosing(syn.TokRBrace, openTok.S)
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
