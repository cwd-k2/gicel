package parse

import (
	"context"
	"maps"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// Fixity holds operator precedence and associativity.
type Fixity struct {
	Assoc syn.Assoc
	Prec  int
}

// parserGuard tracks resource consumption to prevent runaway parsing.
// Separated from parsing logic to make the safety boundary identifiable.
type parserGuard struct {
	recurseDepth    int // current recursion depth
	maxRecurseDepth int // limit (default 256)
	steps           int // advance() call count
	maxSteps        int // limit (default len(tokens) * 4)
	halted          bool
}

func (g *parserGuard) isHalted() bool { return g.halted }

func (g *parserGuard) countStep() bool {
	if g.halted {
		return false
	}
	g.steps++
	return g.steps <= g.maxSteps
}

func (g *parserGuard) enterRecurse() bool {
	if g.halted {
		return false
	}
	g.recurseDepth++
	return g.recurseDepth <= g.maxRecurseDepth
}

func (g *parserGuard) leaveRecurse() {
	g.recurseDepth--
}

func (g *parserGuard) reset() {
	g.steps = 0
	g.halted = false
}

// Parser is a Pratt parser for the surface language.
type Parser struct {
	ctx         context.Context // external cancellation (nil-safe)
	tokens      []syn.Token
	pos         int
	fixity      map[string]Fixity
	errors      *diagnostic.Errors
	depth       int  // paren/brace nesting depth
	noBraceAtom bool // when true, { is not an atom start (inside case scrutinee)

	// stmtBoundaryDepth enables newline-as-separator inside brace-delimited
	// bodies (do-blocks, class/instance bodies, GADT constructors, case alts).
	// When > 0 and p.depth == stmtBoundaryDepth, a token with NewlineBefore
	// acts as a statement boundary, preventing greedy expression consumption.
	stmtBoundaryDepth int

	// methodBodyMode refines statement boundary detection for instance/class
	// method bodies: a newline is only a boundary when followed by a method
	// declaration start (lower + := or lower + ::) or associated type/data.
	// This allows multiline function applications as method bodies (V6 fix).
	methodBodyMode bool

	guard parserGuard // resource safety harness
}

// NewParser creates a parser from a token stream. ctx enables external
// cancellation; pass context.Background() when cancellation is not needed.
func NewParser(ctx context.Context, tokens []syn.Token, errors *diagnostic.Errors) *Parser {
	if ctx == nil {
		ctx = context.Background()
	}
	maxSteps := max(len(tokens)*4, 100)
	return &Parser{
		ctx:    ctx,
		tokens: tokens,
		pos:    0,
		fixity: make(map[string]Fixity),
		errors: errors,
		guard: parserGuard{
			maxRecurseDepth: 256,
			maxSteps:        maxSteps,
		},
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
	p.guard.reset()

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
	if p.guard.isHalted() || p.pos >= len(p.tokens) {
		return syn.Token{Kind: syn.TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() syn.Token {
	if p.guard.isHalted() {
		return syn.Token{Kind: syn.TokEOF}
	}
	if !p.guard.countStep() {
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
	if !p.guard.enterRecurse() {
		if !p.guard.halted {
			p.halt("parser recursion depth limit exceeded")
		}
		return false
	}
	return true
}

func (p *Parser) leaveRecurse() {
	p.guard.leaveRecurse()
}

func (p *Parser) halt(msg string) {
	if !p.guard.halted {
		p.guard.halted = true
		p.addErrorCode(diagnostic.ErrParserLimit, msg)
	}
}

func (p *Parser) expect(kind syn.TokenKind) syn.Token {
	tok := p.peek()
	if tok.Kind != kind {
		code := diagnostic.ErrUnexpectedToken
		switch kind {
		case syn.TokRParen, syn.TokRBrace, syn.TokRBracket:
			code = diagnostic.ErrUnclosedDelim
		}
		p.addErrorCode(code, "expected "+kind.String())
		if tok.Kind != syn.TokEOF {
			p.advance() // skip unexpected token to prevent parser stalling
		}
		return tok
	}
	return p.advance()
}

// expectClosing expects a closing delimiter and attaches a hint pointing
// to the opening delimiter's span when the closing delimiter is missing.
func (p *Parser) expectClosing(kind syn.TokenKind, openSpan span.Span) syn.Token {
	tok := p.peek()
	if tok.Kind == kind {
		return p.advance()
	}
	p.errors.Add(&diagnostic.Error{
		Code:    diagnostic.ErrUnclosedDelim,
		Phase:   diagnostic.PhaseParse,
		Span:    tok.S,
		Message: "expected " + kind.String(),
		Hints:   []diagnostic.Hint{{Span: openSpan, Message: "opening delimiter here"}},
	})
	if tok.Kind != syn.TokEOF {
		p.advance()
	}
	return tok
}

func (p *Parser) expectUpper() string {
	tok := p.peek()
	if tok.Kind != syn.TokUpper {
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected uppercase identifier")
		return "<error>"
	}
	p.advance()
	return tok.Text
}

func (p *Parser) expectLower() string {
	tok := p.peek()
	if tok.Kind != syn.TokLower {
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected identifier")
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
	if p.stmtBoundaryDepth <= 0 || p.depth != p.stmtBoundaryDepth || !p.peek().NewlineBefore {
		return false
	}
	if !p.methodBodyMode {
		return true
	}
	// In method body mode (V6 fix): only treat a newline as a boundary when
	// the next token clearly starts a new declaration, not a continuation line.
	switch p.peek().Kind {
	case syn.TokData, syn.TokType:
		return true
	case syn.TokLower:
		// 2-token lookahead: lower followed by := or :: starts a new method.
		if p.pos+1 < len(p.tokens) {
			next := p.tokens[p.pos+1].Kind
			return next == syn.TokColonEq || next == syn.TokColonColon
		}
		return false
	default:
		return false
	}
}

// parseBody runs a brace-delimited body loop with consistent separator
// handling and stagnation recovery. The opening brace must already be
// consumed; openSpan points to it for diagnostic hints. The parse
// callback is invoked for each item; context is used in stagnation
// error messages.
func (p *Parser) parseBody(bodyCtx string, openSpan span.Span, parse func()) {
	savedBoundary := p.stmtBoundaryDepth
	p.stmtBoundaryDepth = p.depth
	// Iteration limit: at most 2× the total token count. Any well-formed
	// body terminates far below this; the limit catches pathological
	// stagnation that advance() alone cannot prevent (e.g. V6 pattern).
	maxIter := max(len(p.tokens)*2, 100)
	iter := 0
	for p.peek().Kind != syn.TokRBrace && p.peek().Kind != syn.TokEOF {
		iter++
		if iter > maxIter {
			p.halt("parseBody iteration limit exceeded in " + bodyCtx)
			break
		}
		// Context cancellation check (non-blocking).
		select {
		case <-p.ctx.Done():
			p.halt("parser cancelled: " + p.ctx.Err().Error())
		default:
		}
		if p.guard.isHalted() {
			break
		}
		before := p.pos
		parse()
		if p.pos == before {
			// Stagnation: callback consumed nothing. Error-recover before
			// checking separators to prevent infinite loops (V6: a newline
			// token with NewlineBefore would otherwise be treated as an
			// implicit separator, masking the stagnation).
			p.addErrorCode(diagnostic.ErrUnexpectedToken, "unexpected token in "+bodyCtx)
			p.syncToStmtBoundary()
			if p.pos == before {
				// syncToStmtBoundary did not advance — force progress.
				p.advance()
			}
		} else if p.peek().Kind == syn.TokSemicolon {
			p.advance()
		} else if p.peek().NewlineBefore || p.peek().Kind == syn.TokRBrace {
			// newline or closing brace — implicit separator
		}
	}
	p.stmtBoundaryDepth = savedBoundary
	p.expectClosing(syn.TokRBrace, openSpan)
}

func (p *Parser) isTypeAtomStart() bool {
	if p.atStmtBoundary() {
		return false
	}
	k := p.peek().Kind
	return k == syn.TokLower || k == syn.TokUpper || k == syn.TokLParen || k == syn.TokLBrace || k == syn.TokUnderscore
}

// syncToStmtBoundary advances to the next statement boundary within a
// brace-delimited body. Used for expression-level recovery in do-blocks,
// case alts, and similar constructs so that one broken statement/alt
// doesn't swallow subsequent valid ones.
func (p *Parser) syncToStmtBoundary() {
	for p.peek().Kind != syn.TokEOF {
		if p.peek().Kind == syn.TokSemicolon {
			return
		}
		if p.peek().Kind == syn.TokRBrace {
			return
		}
		if p.stmtBoundaryDepth > 0 && p.depth == p.stmtBoundaryDepth && p.peek().NewlineBefore {
			return
		}
		p.advance()
	}
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

// speculate runs fn as a tentative parse. If fn returns true, the
// parse succeeds and all side effects (position, depth, errors) are
// kept. If fn returns false, position, depth, and errors are rolled
// back to the state before the call.
func (p *Parser) speculate(fn func() bool) bool {
	savedPos := p.pos
	savedDepth := p.depth
	savedErrLen := p.errors.Len()
	if fn() {
		return true
	}
	p.pos = savedPos
	p.depth = savedDepth
	p.errors.Truncate(savedErrLen)
	return false
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
	p.addErrorCode(diagnostic.ErrParseSyntax, msg)
}

func (p *Parser) addErrorCode(code diagnostic.Code, msg string) {
	tok := p.peek()
	p.errors.Add(&diagnostic.Error{
		Code:    code,
		Phase:   diagnostic.PhaseParse,
		Span:    tok.S,
		Message: msg,
	})
}
