package parse

import (
	"maps"

	syn "github.com/cwd-k2/gicel/internal/syntax"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
)

// Fixity holds operator precedence and associativity.
type Fixity struct {
	Assoc syn.Assoc
	Prec  int
}

// Parser is a Pratt parser for the surface language.
type Parser struct {
	tokens      []syn.Token
	pos         int
	fixity      map[string]Fixity
	errors      *errs.Errors
	depth       int  // paren/brace nesting depth
	noBraceAtom bool // when true, { is not an atom start (inside case scrutinee)

	// stmtBoundaryDepth enables newline-as-separator inside brace-delimited
	// bodies (do-blocks, class/instance bodies, GADT constructors, case alts).
	// When > 0 and p.depth == stmtBoundaryDepth, a token with NewlineBefore
	// acts as a statement boundary, preventing greedy expression consumption.
	stmtBoundaryDepth int

	// Safety harness: prevent resource exhaustion on malformed input.
	recurseDepth    int // current recursion depth
	maxRecurseDepth int // limit (default 256)
	steps           int // advance() call count
	maxSteps        int // limit (default len(tokens) * 4)
	halted          bool
}

// NewParser creates a parser from a token stream.
func NewParser(tokens []syn.Token, errors *errs.Errors) *Parser {
	maxSteps := max(len(tokens)*4, 100)
	return &Parser{
		tokens:          tokens,
		pos:             0,
		fixity:          make(map[string]Fixity),
		errors:          errors,
		maxRecurseDepth: 256,
		maxSteps:        maxSteps,
	}
}

// AddFixity merges host-supplied fixity declarations into the parser.
func (p *Parser) AddFixity(fixity map[string]Fixity) {
	maps.Copy(p.fixity, fixity)
}

// ParseProgram parses a complete program.
func (p *Parser) ParseProgram() *syn.AstProgram {
	// First pass: collect fixity declarations.
	p.collectFixity()
	p.pos = 0
	p.steps = 0
	p.halted = false

	// Parse imports first.
	var imports []syn.DeclImport
	p.skipSemicolons()
	for p.peek().Kind == syn.TokImport {
		imports = append(imports, p.parseImportDecl())
		p.skipSemicolons()
	}

	var decls []syn.Decl
	for p.peek().Kind != syn.TokEOF {
		before := p.pos
		d := p.parseDecl()
		if d != nil {
			decls = append(decls, d)
		} else {
			// Failed declaration: synchronize to the next declaration boundary
			// so that one malformed declaration doesn't swallow subsequent valid ones.
			p.syncToNextDecl()
		}
		if p.pos == before {
			// Safety: ensure progress even if syncToNextDecl didn't advance.
			p.advance()
		}
		p.skipSemicolons()
	}
	return &syn.AstProgram{Imports: imports, Decls: decls}
}

// ParseExpr parses a single expression.
func (p *Parser) ParseExpr() syn.Expr {
	return p.parseExpr()
}

// ParseType parses a type expression.
func (p *Parser) ParseType() syn.TypeExpr {
	return p.parseType()
}

// Declaration parsing is in parse_decl.go.
// Expression parsing is in parse_expr.go.
// Pattern parsing is in parse_pattern.go.
// Type parsing is in parse_type.go.
// Import parsing is in parse_import.go.
// Class/instance parsing is in parse_class.go.

// --- Helpers ---

func (p *Parser) skipSemicolons() {
	for p.peek().Kind == syn.TokSemicolon {
		p.advance()
	}
}

func (p *Parser) peek() syn.Token {
	if p.halted || p.pos >= len(p.tokens) {
		return syn.Token{Kind: syn.TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() syn.Token {
	if p.halted {
		return syn.Token{Kind: syn.TokEOF}
	}
	p.steps++
	if p.steps > p.maxSteps {
		p.halt("parser step limit exceeded")
		return syn.Token{Kind: syn.TokEOF}
	}
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	switch tok.Kind {
	case syn.TokLParen, syn.TokLBrace, syn.TokLBracket:
		p.depth++
	case syn.TokRParen, syn.TokRBrace, syn.TokRBracket:
		if p.depth > 0 {
			p.depth--
		}
	}
	return tok
}

// enterRecurse increments recursion depth and returns false if limit exceeded.
func (p *Parser) enterRecurse() bool {
	if p.halted {
		return false
	}
	p.recurseDepth++
	if p.recurseDepth > p.maxRecurseDepth {
		p.halt("parser recursion depth limit exceeded")
		return false
	}
	return true
}

func (p *Parser) leaveRecurse() {
	p.recurseDepth--
}

func (p *Parser) halt(msg string) {
	if !p.halted {
		p.halted = true
		p.addErrorCode(errs.ErrParserLimit, msg)
	}
}

func (p *Parser) expect(kind syn.TokenKind) syn.Token {
	tok := p.peek()
	if tok.Kind != kind {
		code := errs.ErrUnexpectedToken
		switch kind {
		case syn.TokRParen, syn.TokRBrace, syn.TokRBracket:
			code = errs.ErrUnclosedDelim
		}
		p.addErrorCode(code, "expected "+kind.String())
		if tok.Kind != syn.TokEOF {
			p.advance() // skip unexpected token to prevent parser stalling
		}
		return tok
	}
	return p.advance()
}

func (p *Parser) expectUpper() string {
	tok := p.peek()
	if tok.Kind != syn.TokUpper {
		p.addErrorCode(errs.ErrUnexpectedToken, "expected uppercase identifier")
		return "<error>"
	}
	p.advance()
	return tok.Text
}

func (p *Parser) expectLower() string {
	tok := p.peek()
	if tok.Kind != syn.TokLower {
		p.addErrorCode(errs.ErrUnexpectedToken, "expected identifier")
		return "<error>"
	}
	p.advance()
	return tok.Text
}

func (p *Parser) prevEnd() span.Pos {
	if p.pos > 0 && p.pos <= len(p.tokens) {
		return p.tokens[p.pos-1].S.End
	}
	return span.Pos(p.pos)
}

// atDeclBoundary returns true if the next token is across a newline boundary
// at the top level and could start a new declaration. This prevents greedy
// consumption of tokens that belong to subsequent declarations.
func (p *Parser) atDeclBoundary() bool {
	if p.depth > 0 {
		return false
	}
	tok := p.peek()
	if !tok.NewlineBefore {
		return false
	}
	switch tok.Kind {
	case syn.TokLower, syn.TokUpper, syn.TokData, syn.TokType, syn.TokInfixl, syn.TokInfixr, syn.TokInfixn, syn.TokClass, syn.TokInstance, syn.TokImport:
		return true
	case syn.TokLParen:
		// (op) declaration pattern
		return p.isOperatorDeclStart()
	}
	return false
}

// atStmtBoundary returns true if the next token is at a statement boundary:
// either at the top level (atDeclBoundary), or inside a brace-delimited body
// at the matching depth where a newline acts as an implicit separator.
func (p *Parser) atStmtBoundary() bool {
	if p.atDeclBoundary() {
		return true
	}
	return p.stmtBoundaryDepth > 0 && p.depth == p.stmtBoundaryDepth && p.peek().NewlineBefore
}

// parseBody runs a brace-delimited body loop with consistent separator
// handling and stagnation recovery.  The parse callback is invoked for each
// item; context is used in the stagnation error message.
func (p *Parser) parseBody(context string, parse func()) {
	savedBoundary := p.stmtBoundaryDepth
	p.stmtBoundaryDepth = p.depth
	for p.peek().Kind != syn.TokRBrace && p.peek().Kind != syn.TokEOF {
		before := p.pos
		parse()
		if p.peek().Kind == syn.TokSemicolon {
			p.advance()
		} else if p.peek().NewlineBefore || p.peek().Kind == syn.TokRBrace {
			// newline or closing brace — implicit separator
		} else if p.pos == before {
			p.addErrorCode(errs.ErrUnexpectedToken, "unexpected token in "+context)
			p.advance()
		}
	}
	p.stmtBoundaryDepth = savedBoundary
	p.expect(syn.TokRBrace)
}

func (p *Parser) isTypeAtomStart() bool {
	if p.atStmtBoundary() {
		return false
	}
	k := p.peek().Kind
	return k == syn.TokLower || k == syn.TokUpper || k == syn.TokLParen || k == syn.TokLBrace || k == syn.TokUnderscore
}

// syncToNextDecl advances the parser to the next token that could start a
// declaration at the top level. Used for error recovery after a failed
// declaration parse, so that one malformed declaration doesn't swallow
// subsequent valid ones.
func (p *Parser) syncToNextDecl() {
	for p.peek().Kind != syn.TokEOF {
		tok := p.peek()
		if tok.NewlineBefore {
			switch tok.Kind {
			case syn.TokLower, syn.TokUpper, syn.TokData, syn.TokType,
				syn.TokInfixl, syn.TokInfixr, syn.TokInfixn,
				syn.TokClass, syn.TokInstance, syn.TokImport, syn.TokLParen:
				return
			}
		}
		if tok.Kind == syn.TokSemicolon {
			return
		}
		p.advance()
	}
}

func (p *Parser) lookupFixity(op string) Fixity {
	if f, ok := p.fixity[op]; ok {
		return f
	}
	return Fixity{Assoc: syn.AssocLeft, Prec: 9} // default
}

// tokensAdjacent checks if two tokens have no whitespace between them.
func tokensAdjacent(a, b syn.Token) bool {
	return a.S.End == b.S.Start
}

func (p *Parser) addError(msg string) {
	p.addErrorCode(errs.ErrParseSyntax, msg)
}

func (p *Parser) addErrorCode(code errs.Code, msg string) {
	tok := p.peek()
	p.errors.Add(&errs.Error{
		Code:    code,
		Phase:   errs.PhaseParse,
		Span:    tok.S,
		Message: msg,
	})
}
