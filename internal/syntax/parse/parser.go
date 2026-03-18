package parse

import (
	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
)

// Fixity holds operator precedence and associativity.
type Fixity struct {
	Assoc Assoc
	Prec  int
}

// Parser is a Pratt parser for the surface language.
type Parser struct {
	tokens      []Token
	pos         int
	fixity      map[string]Fixity
	errors      *errs.Errors
	depth       int  // paren/brace nesting depth
	noBraceAtom bool // when true, { is not an atom start (inside case scrutinee)

	// Safety harness: prevent resource exhaustion on malformed input.
	recurseDepth    int // current recursion depth
	maxRecurseDepth int // limit (default 256)
	steps           int // advance() call count
	maxSteps        int // limit (default len(tokens) * 4)
	halted          bool
}

// NewParser creates a parser from a token stream.
func NewParser(tokens []Token, errors *errs.Errors) *Parser {
	maxSteps := len(tokens) * 4
	if maxSteps < 100 {
		maxSteps = 100
	}
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
	for op, f := range fixity {
		p.fixity[op] = f
	}
}

// ParseProgram parses a complete program.
func (p *Parser) ParseProgram() *AstProgram {
	// First pass: collect fixity declarations.
	p.collectFixity()
	p.pos = 0
	p.steps = 0
	p.halted = false

	// Parse imports first.
	var imports []DeclImport
	p.skipSemicolons()
	for p.peek().Kind == TokImport {
		imports = append(imports, p.parseImportDecl())
		p.skipSemicolons()
	}

	var decls []Decl
	for p.peek().Kind != TokEOF {
		d := p.parseDecl()
		if d != nil {
			decls = append(decls, d)
		}
		p.skipSemicolons()
	}
	return &AstProgram{Imports: imports, Decls: decls}
}

// ParseExpr parses a single expression.
func (p *Parser) ParseExpr() Expr {
	return p.parseExpr()
}

// ParseType parses a type expression.
func (p *Parser) ParseType() TypeExpr {
	return p.parseType()
}

// Declaration parsing is in parse_decl.go.
// Expression and pattern parsing is in parse_expr.go.

// --- Helpers ---

func (p *Parser) skipSemicolons() {
	for p.peek().Kind == TokSemicolon {
		p.advance()
	}
}

func (p *Parser) peek() Token {
	if p.halted || p.pos >= len(p.tokens) {
		return Token{Kind: TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	if p.halted {
		return Token{Kind: TokEOF}
	}
	p.steps++
	if p.steps > p.maxSteps {
		p.halt("parser step limit exceeded")
		return Token{Kind: TokEOF}
	}
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	switch tok.Kind {
	case TokLParen, TokLBrace, TokLBracket:
		p.depth++
	case TokRParen, TokRBrace, TokRBracket:
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
		p.addError(msg)
	}
}

func (p *Parser) expect(kind TokenKind) Token {
	tok := p.peek()
	if tok.Kind != kind {
		p.addError("expected " + kind.String())
		if tok.Kind != TokEOF {
			p.advance() // skip unexpected token to prevent parser stalling
		}
		return tok
	}
	return p.advance()
}

func (p *Parser) expectUpper() string {
	tok := p.peek()
	if tok.Kind != TokUpper {
		p.addError("expected uppercase identifier")
		return "<error>"
	}
	p.advance()
	return tok.Text
}

func (p *Parser) expectLower() string {
	tok := p.peek()
	if tok.Kind != TokLower {
		p.addError("expected identifier")
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
	case TokLower, TokUpper, TokData, TokType, TokInfixl, TokInfixr, TokInfixn, TokClass, TokInstance, TokImport:
		return true
	case TokLParen:
		// (op) declaration pattern
		return p.isOperatorDeclStart()
	}
	return false
}

func (p *Parser) isTypeAtomStart() bool {
	if p.atDeclBoundary() {
		return false
	}
	k := p.peek().Kind
	return k == TokLower || k == TokUpper || k == TokLParen || k == TokLBrace || k == TokUnderscore
}

func (p *Parser) lookupFixity(op string) Fixity {
	if f, ok := p.fixity[op]; ok {
		return f
	}
	return Fixity{Assoc: AssocLeft, Prec: 9} // default
}

// tokensAdjacent checks if two tokens have no whitespace between them.
func tokensAdjacent(a, b Token) bool {
	return a.S.End == b.S.Start
}

func (p *Parser) addError(msg string) {
	tok := p.peek()
	p.errors.Add(&errs.Error{
		Code:    errs.ErrParseSyntax,
		Phase:   errs.PhaseParse,
		Span:    tok.S,
		Message: msg,
	})
}
