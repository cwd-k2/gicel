package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

func (p *Parser) parseImportDecl() syn.DeclImport {
	start := p.peek().S.Start
	p.expect(syn.TokImport)
	modTok := p.peek()
	modName := p.expectUpper()
	// Support dotted module names: Std.Num, Std.Str, etc.
	// Only adjacent dots (no whitespace) form module paths.
	for p.peek().Kind == syn.TokDot && tokensAdjacent(modTok, p.peek()) {
		p.advance()
		part := p.expectUpper()
		modName = modName + "." + part
		modTok = p.tokens[p.pos-1] // update for next adjacency check
	}

	imp := syn.DeclImport{ModuleName: modName}

	// Check for qualified import: import M as N
	if p.peek().Kind == syn.TokAs {
		p.advance() // consume "as"
		imp.Alias = p.expectUpper()
		imp.S = span.Span{Start: start, End: p.prevEnd()}
		return imp
	}

	// Check for selective import: import M (name, T(..), C(A,B))
	if p.peek().Kind == syn.TokLParen {
		imp.Names = p.parseImportList()
	}

	imp.S = span.Span{Start: start, End: p.prevEnd()}
	return imp
}

// parseImportList parses the parenthesized import name list: (name, T(..), (op), C(A,B))
func (p *Parser) parseImportList() []syn.ImportName {
	p.expect(syn.TokLParen)
	names := []syn.ImportName{} // non-nil empty slice distinguishes import M () from import M
	for p.peek().Kind != syn.TokRParen && p.peek().Kind != syn.TokEOF {
		names = append(names, p.parseImportName())
		if p.peek().Kind == syn.TokComma {
			p.advance()
		} else {
			break
		}
	}
	p.expect(syn.TokRParen)
	return names
}

// parseImportName parses one entry in an import list.
//
//	lower            → value binding
//	(op)             → operator
//	Upper            → type/form bare
//	Upper (..)       → type/form with all subs
//	Upper (A, B)     → type/form with specific subs
func (p *Parser) parseImportName() syn.ImportName {
	// Operator: (op)
	if p.peek().Kind == syn.TokLParen {
		p.advance()
		var opName string
		if p.peek().Kind == syn.TokOp {
			opName = p.peek().Text
			p.advance()
		} else if p.peek().Kind == syn.TokDot {
			opName = "."
			p.advance()
		} else {
			p.addErrorCode(diagnostic.ErrImportSyntax, "expected operator in import list")
			// Skip to closing paren for error recovery.
			for p.peek().Kind != syn.TokRParen && p.peek().Kind != syn.TokEOF {
				p.advance()
			}
			opName = errName
		}
		p.expect(syn.TokRParen)
		return syn.ImportName{Name: opName, Error: opName == errName}
	}

	// Value binding: lower
	if p.peek().Kind == syn.TokLower {
		name := p.peek().Text
		p.advance()
		return syn.ImportName{Name: name}
	}

	// Reject unexpected tokens early with recovery.
	if p.peek().Kind != syn.TokUpper {
		p.addErrorCode(diagnostic.ErrImportSyntax, "expected name in import list")
		// Skip to next comma or closing paren.
		for p.peek().Kind != syn.TokComma && p.peek().Kind != syn.TokRParen && p.peek().Kind != syn.TokEOF {
			p.advance()
		}
		return syn.ImportName{Name: errName, Error: true}
	}

	// Type/class: Upper [(..) | (A, B)]
	name := p.expectUpper()
	in := syn.ImportName{Name: name}

	if p.peek().Kind == syn.TokLParen {
		p.advance()
		in.HasSub = true
		if p.peek().Kind == syn.TokDot && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == syn.TokDot {
			// (..)
			p.advance() // first .
			p.advance() // second .
			in.AllSubs = true
		} else if p.peek().Kind != syn.TokRParen {
			// Explicit sub-list: (A, B, methodName, ...)
			for {
				var sub string
				if p.peek().Kind == syn.TokUpper {
					sub = p.peek().Text
					p.advance()
				} else if p.peek().Kind == syn.TokLower {
					sub = p.peek().Text
					p.advance()
				} else {
					p.addErrorCode(diagnostic.ErrImportSyntax, "expected name in import sub-list")
					break
				}
				in.SubList = append(in.SubList, sub)
				if p.peek().Kind == syn.TokComma {
					p.advance()
				} else {
					break
				}
			}
		}
		p.expect(syn.TokRParen)
	}

	return in
}
