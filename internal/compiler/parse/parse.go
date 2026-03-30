package parse

import (
	"context"
	"maps"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// errName is the placeholder returned by expectUpper/expectLower when the
// expected token is missing. Contains angle brackets so it cannot collide
// with any valid GICEL identifier.
const errName = "<error>"

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
	g.recurseDepth = 0
	g.halted = false
}

// progressGuard enforces iteration limits on parser loops that are not
// bounded by explicit token types (e.g. comma-separated lists with break).
// It detects stagnation (no position change) and forces recovery.
type progressGuard struct {
	parser  *Parser
	context string
	maxIter int
	iter    int
	before  int
}

// newProgressGuard creates a progress guard for the named loop context.
func (p *Parser) newProgressGuard(context string) progressGuard {
	return progressGuard{
		parser:  p,
		context: context,
		maxIter: max(p.guard.maxSteps/2, 100),
	}
}

// Begin must be called at the top of each loop iteration. It returns false
// if the loop should terminate (iteration limit, cancellation, or halt).
func (pg *progressGuard) Begin() bool {
	pg.iter++
	if pg.iter > pg.maxIter {
		pg.parser.halt("iteration limit exceeded in " + pg.context)
		return false
	}
	select {
	case <-pg.parser.ctx.Done():
		pg.parser.halt("parser cancelled: " + pg.parser.ctx.Err().Error())
	default:
	}
	if pg.parser.guard.isHalted() {
		return false
	}
	pg.before = pg.parser.tb.pos
	return true
}

// DidProgress returns true if the parser position advanced since Begin().
func (pg *progressGuard) DidProgress() bool {
	return pg.parser.tb.pos != pg.before
}

// RecoverStagnation emits an error and forces progress when the parser
// has not advanced during the current iteration.
func (pg *progressGuard) RecoverStagnation() {
	pg.parser.addErrorCode(diagnostic.ErrUnexpectedToken, "unexpected token in "+pg.context)
	pg.parser.syncToStmtBoundary()
	if pg.parser.tb.pos == pg.before {
		pg.parser.advance()
	}
}

// Parser is a streaming parser for the surface language.
type Parser struct {
	ctx     context.Context // external cancellation (nil-safe)
	tb      *tokenBuffer
	scanner *Scanner
	fixity  map[string]Fixity
	errors  *diagnostic.Errors
	depth   int  // paren/brace nesting depth
	noBraceAtom bool // when true, { is not an atom start (inside case scrutinee)

	// stmtBoundaryDepth enables newline-as-separator inside brace-delimited
	// bodies (do-blocks, form/impl bodies, GADT constructors, case alts).
	// When > 0 and p.depth == stmtBoundaryDepth, a token with NewlineBefore
	// acts as a statement boundary, preventing greedy expression consumption.
	stmtBoundaryDepth int

	guard parserGuard // resource safety harness
}

// NewParser creates a streaming parser for the given source. Tokens are
// produced on demand via an internal Scanner — no bulk tokenization.
func NewParser(ctx context.Context, source *span.Source, errors *diagnostic.Errors) *Parser {
	if ctx == nil {
		ctx = context.Background()
	}
	scanner := NewScanner(source)
	maxSteps := max(len(source.Text)*4, 100)
	return &Parser{
		ctx:     ctx,
		tb:      newTokenBuffer(scanner, nil),
		scanner: scanner,
		fixity:  make(map[string]Fixity),
		errors:  errors,
		guard: parserGuard{
			maxRecurseDepth: 256,
			maxSteps:        maxSteps,
		},
	}
}

// LexErrors returns accumulated lexer diagnostics.
func (p *Parser) LexErrors() *diagnostic.Errors {
	return p.scanner.Errors()
}

// AddFixity merges host-supplied fixity declarations into the parser.
func (p *Parser) AddFixity(fixity map[string]Fixity) {
	maps.Copy(p.fixity, fixity)
}

// ParseProgram parses a complete program. Infix expressions with
// multiple operators are parsed as flat spines and resolved to
// nested ExprInfix trees using fixity from:
//   - external modules (via AddFixity, called before ParseProgram)
//   - in-module DeclFixity declarations (collected from the AST)
func (p *Parser) ParseProgram() *syn.AstProgram {
	imports := p.parseImportBlock()
	decls := p.parseDeclBlock()
	ast := &syn.AstProgram{Imports: imports, Decls: decls}

	// Collect in-module fixity from DeclFixity nodes and merge
	// with external fixity already in p.fixity (from AddFixity).
	maps.Copy(p.fixity, CollectModuleFixity(decls))
	ResolveFixity(ast, p.fixity, p.errors)
	return ast
}

// ParseImports parses the import block. After this call, the parser
// is positioned at the first non-import declaration.
func (p *Parser) ParseImports() []syn.DeclImport {
	return p.parseImportBlock()
}

// ParseDecls parses all remaining declarations after imports.
func (p *Parser) ParseDecls() []syn.Decl {
	return p.parseDeclBlock()
}

func (p *Parser) parseImportBlock() []syn.DeclImport {
	var imports []syn.DeclImport
	p.skipSemicolons()
	for p.peek().Kind == syn.TokImport {
		imports = append(imports, p.parseImportDecl())
		p.skipSemicolons()
	}
	return imports
}

func (p *Parser) parseDeclBlock() []syn.Decl {
	var decls []syn.Decl
	for p.peek().Kind != syn.TokEOF {
		before := p.tb.pos
		d := p.parseDecl()
		if d != nil {
			decls = append(decls, d)
		} else {
			p.syncToNextDecl()
		}
		if p.tb.pos == before {
			p.advance()
		}
		p.skipSemicolons()
	}
	return decls
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

// --- Helpers ---

func (p *Parser) skipSemicolons() {
	for p.peek().Kind == syn.TokSemicolon {
		p.advance()
	}
}

func (p *Parser) peek() syn.Token {
	if p.guard.isHalted() {
		return syn.Token{Kind: syn.TokEOF}
	}
	return p.tb.peek()
}

func (p *Parser) advance() syn.Token {
	if p.guard.isHalted() {
		return syn.Token{Kind: syn.TokEOF}
	}
	if !p.guard.countStep() {
		p.halt("parser step limit exceeded")
		return syn.Token{Kind: syn.TokEOF}
	}
	tok := p.tb.advance()
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
		return errName
	}
	p.advance()
	return tok.Text
}

func (p *Parser) expectLower() string {
	tok := p.peek()
	if tok.Kind != syn.TokLower {
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected identifier")
		return errName
	}
	p.advance()
	return tok.Text
}

func (p *Parser) prevEnd() span.Pos {
	return p.tb.prevEnd()
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
	case syn.TokLower, syn.TokUpper, syn.TokForm, syn.TokType, syn.TokInfixl, syn.TokInfixr, syn.TokInfixn, syn.TokImpl, syn.TokImport:
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
	return true
}

// parseBody runs a brace-delimited body loop with consistent separator
// handling and stagnation recovery. The opening brace must already be
// consumed; openSpan points to it for diagnostic hints. The parse
// callback is invoked for each item; context is used in stagnation
// error messages.
func (p *Parser) parseBody(bodyCtx string, openSpan span.Span, parse func()) {
	savedBoundary := p.stmtBoundaryDepth
	p.stmtBoundaryDepth = p.depth
	pg := p.newProgressGuard(bodyCtx)
	for p.peek().Kind != syn.TokRBrace && p.peek().Kind != syn.TokEOF {
		if !pg.Begin() {
			break
		}
		parse()
		if !pg.DidProgress() {
			pg.RecoverStagnation()
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
	if p.noBraceAtom && k == syn.TokLBrace {
		return false
	}
	return k == syn.TokLower || k == syn.TokUpper ||
		k == syn.TokLParen || k == syn.TokLBrace ||
		k == syn.TokUnderscore || k == syn.TokCase || k == syn.TokLabelLit
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
			case syn.TokLower, syn.TokUpper, syn.TokForm, syn.TokType,
				syn.TokInfixl, syn.TokInfixr, syn.TokInfixn,
				syn.TokImpl, syn.TokImport, syn.TokLParen:
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
	p.tb.save()
	savedDepth := p.depth
	savedErrLen := p.errors.Len()
	savedSteps := p.guard.steps
	if fn() {
		p.tb.commit()
		return true
	}
	p.tb.restore()
	p.depth = savedDepth
	p.errors.Truncate(savedErrLen)
	p.guard.steps = savedSteps
	return false
}

// tokensAdjacent checks if two tokens have no whitespace between them.
func tokensAdjacent(a, b syn.Token) bool {
	return a.S.End == b.S.Start
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
