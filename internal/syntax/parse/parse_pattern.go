package parse

import (
	"fmt"

	syn "github.com/cwd-k2/gicel/internal/syntax"

	"github.com/cwd-k2/gicel/internal/span"
)

// --- Patterns ---

func (p *Parser) parsePattern() syn.Pattern {
	if !p.enterRecurse() {
		return &syn.PatWild{S: span.Span{Start: span.Pos(p.pos), End: span.Pos(p.pos)}}
	}
	defer p.leaveRecurse()
	switch p.peek().Kind {
	case syn.TokUpper:
		return p.parseConPattern()
	case syn.TokLower:
		tok := p.peek()
		p.advance()
		return &syn.PatVar{Name: tok.Text, S: tok.S}
	case syn.TokUnderscore:
		tok := p.peek()
		p.advance()
		return &syn.PatWild{S: tok.S}
	case syn.TokIntLit, syn.TokDoubleLit, syn.TokStrLit, syn.TokRuneLit:
		return p.parseLitPattern()
	case syn.TokLParen:
		start := p.peek().S.Start
		p.advance()
		// () → unit pattern (empty record pattern)
		if p.peek().Kind == syn.TokRParen {
			p.advance()
			return &syn.PatRecord{S: span.Span{Start: start, End: p.prevEnd()}}
		}
		inner := p.parsePattern()
		if p.peek().Kind == syn.TokComma {
			return p.parseTuplePatternTail(start, inner)
		}
		p.expect(syn.TokRParen)
		return &syn.PatParen{Inner: inner, S: span.Span{Start: start, End: p.prevEnd()}}
	case syn.TokLBrace:
		return p.parseRecordPattern()
	default:
		p.addError("expected pattern")
		tok := p.peek()
		p.advance()
		return &syn.PatWild{S: tok.S}
	}
}

func (p *Parser) parseConPattern() syn.Pattern {
	start := p.peek().S.Start
	tok := p.peek()
	name := p.expectUpper()
	// Qualified constructor pattern: Q.Con — adjacency-disambiguated.
	qualifier := ""
	if p.peek().Kind == syn.TokDot && tokensAdjacent(tok, p.peek()) {
		if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == syn.TokUpper && tokensAdjacent(p.peek(), p.tokens[p.pos+1]) {
			qualifier = name
			p.advance() // skip .
			name = p.expectUpper()
		}
	}
	var args []syn.Pattern
	for p.isPatternAtomStart() {
		args = append(args, p.parsePatternAtom())
	}
	s := span.Span{Start: start, End: p.prevEnd()}
	if qualifier != "" {
		return &syn.PatQualCon{Qualifier: qualifier, Con: name, Args: args, S: s}
	}
	if len(args) == 0 {
		return &syn.PatCon{Con: name, S: s}
	}
	return &syn.PatCon{Con: name, Args: args, S: s}
}

func (p *Parser) parseRecordPattern() syn.Pattern {
	start := p.peek().S.Start
	p.expect(syn.TokLBrace)
	var fields []syn.PatRecordField
	if p.peek().Kind != syn.TokRBrace {
		for {
			fStart := p.peek().S.Start
			label := p.expectLower()
			p.expect(syn.TokColon)
			pat := p.parsePattern()
			fields = append(fields, syn.PatRecordField{
				Label: label, Pattern: pat,
				S: span.Span{Start: fStart, End: p.prevEnd()},
			})
			if p.peek().Kind == syn.TokComma {
				p.advance()
			} else {
				break
			}
		}
	}
	p.expect(syn.TokRBrace)
	return &syn.PatRecord{Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
}

func (p *Parser) parsePatternAtom() syn.Pattern {
	switch p.peek().Kind {
	case syn.TokLower:
		tok := p.peek()
		p.advance()
		return &syn.PatVar{Name: tok.Text, S: tok.S}
	case syn.TokUnderscore:
		tok := p.peek()
		p.advance()
		return &syn.PatWild{S: tok.S}
	case syn.TokUpper:
		// Bare constructor as pattern atom (0-arg constructor in nested position).
		// May be qualified: Q.Con
		tok := p.peek()
		p.advance()
		if p.peek().Kind == syn.TokDot && tokensAdjacent(tok, p.peek()) {
			if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == syn.TokUpper && tokensAdjacent(p.peek(), p.tokens[p.pos+1]) {
				qualifier := tok.Text
				p.advance() // skip .
				conTok := p.peek()
				p.advance() // consume Upper
				return &syn.PatQualCon{Qualifier: qualifier, Con: conTok.Text, S: span.Span{Start: tok.S.Start, End: conTok.S.End}}
			}
		}
		return &syn.PatCon{Con: tok.Text, S: tok.S}
	case syn.TokIntLit, syn.TokDoubleLit, syn.TokStrLit, syn.TokRuneLit:
		return p.parseLitPattern()
	case syn.TokLParen:
		start := p.peek().S.Start
		p.advance()
		// () → unit pattern
		if p.peek().Kind == syn.TokRParen {
			p.advance()
			return &syn.PatRecord{S: span.Span{Start: start, End: p.prevEnd()}}
		}
		inner := p.parsePattern()
		// (p1, p2, ...) → tuple pattern
		if p.peek().Kind == syn.TokComma {
			return p.parseTuplePatternTail(start, inner)
		}
		p.expect(syn.TokRParen)
		return &syn.PatParen{Inner: inner, S: span.Span{Start: start, End: p.prevEnd()}}
	case syn.TokLBrace:
		return p.parseRecordPattern()
	default:
		return nil
	}
}

// parseTuplePatternTail parses the remaining comma-separated patterns after the first
// element, closing the paren and returning a record pattern with _1, _2, ... labels.
func (p *Parser) parseTuplePatternTail(start span.Pos, first syn.Pattern) *syn.PatRecord {
	pats := []syn.Pattern{first}
	for p.peek().Kind == syn.TokComma {
		p.advance()
		pats = append(pats, p.parsePattern())
	}
	p.expect(syn.TokRParen)
	fields := make([]syn.PatRecordField, len(pats))
	for i, pat := range pats {
		fields[i] = syn.PatRecordField{
			Label:   fmt.Sprintf("_%d", i+1),
			Pattern: pat,
			S:       pat.Span(),
		}
	}
	return &syn.PatRecord{Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
}

func (p *Parser) isPatternAtomStart() bool {
	if p.atDeclBoundary() {
		return false
	}
	k := p.peek().Kind
	return k == syn.TokLower || k == syn.TokUnderscore || k == syn.TokLParen || k == syn.TokUpper || k == syn.TokIntLit || k == syn.TokDoubleLit || k == syn.TokStrLit || k == syn.TokRuneLit || k == syn.TokLBrace
}

func (p *Parser) parseLitPattern() syn.Pattern {
	tok := p.peek()
	p.advance()
	var kind syn.LitKind
	switch tok.Kind {
	case syn.TokIntLit:
		kind = syn.LitInt
	case syn.TokDoubleLit:
		kind = syn.LitDouble
	case syn.TokStrLit:
		kind = syn.LitString
	case syn.TokRuneLit:
		kind = syn.LitRune
	}
	return &syn.PatLit{Value: tok.Text, Kind: kind, S: tok.S}
}
