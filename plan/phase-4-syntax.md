# Phase 4: Syntax

## Objective

Implement the lexer (tokenizer), surface AST, and Pratt parser for the full grammar specified in spec §5. The parser produces surface AST nodes — distinct from Core IR. The checker (Phase 5) consumes AST and produces Core.

## Dependencies

Phase 1 (`types/`). No dependency on `core/` or `eval/`.

## Package: `syntax/`

### 4.1 Tokens (`syntax/token.go`)

```go
package syntax

import "github.com/cwd-k2/gomputation/internal/span"

type TokenKind int

const (
    // Structural
    TokEOF TokenKind = iota
    TokNewline

    // Delimiters
    TokLParen    // (
    TokRParen    // )
    TokLBrace    // {
    TokRBrace    // }
    TokComma     // ,
    TokSemicolon // ;
    TokPipe      // |
    TokAt        // @

    // Punctuation
    TokArrow     // ->
    TokLArrow    // <-
    TokColonColon // ::
    TokColonEq   // :=
    TokColon     // :
    TokDot       // .
    TokBackslash // \
    TokUnderscore // _
    TokEq        // =

    // Keywords (9)
    TokCase
    TokOf
    TokDo
    TokData
    TokType
    TokForall
    TokInfixl
    TokInfixr
    TokInfixn

    // Identifiers
    TokLower     // lowercase-start identifier (variables, labels)
    TokUpper     // uppercase-start identifier (constructors, type constructors)

    // Operators
    TokOp        // operator characters (+, -, *, ==, >>=, etc.)

    // Numeric (for fixity declarations only — not a language literal)
    TokIntLit    // integer literal (only valid in fixity declarations)
)

type Token struct {
    Kind TokenKind
    Text string     // original source text of this token
    S    span.Span
}

// keywords maps keyword strings to their TokenKind.
var keywords = map[string]TokenKind{
    "case":   TokCase,
    "of":     TokOf,
    "do":     TokDo,
    "data":   TokData,
    "type":   TokType,
    "forall": TokForall,
    "infixl": TokInfixl,
    "infixr": TokInfixr,
    "infixn": TokInfixn,
}
```

**Design note**: `TokIntLit` exists only for fixity declarations (`infixl 6 +`). The language has no integer literals — this is a metalanguage construct for fixity precedence.

### 4.2 Lexer (`syntax/lexer.go`)

```go
// Lexer tokenizes source text.
type Lexer struct {
    source *span.Source
    pos    int
    tokens []Token
    errors *errs.Errors
}

// NewLexer creates a lexer for the given source.
func NewLexer(source *span.Source) *Lexer

// Tokenize scans the entire source and returns the token stream.
func (l *Lexer) Tokenize() ([]Token, *errs.Errors)
```

#### Lexer rules

1. **Whitespace**: spaces, tabs, newlines — skip (newlines may be significant for layout in future; for now, skip).
2. **Comments**: `--` to end of line; `{- ... -}` block comments (nestable).
3. **Lower identifier**: `[a-z_][a-zA-Z0-9_']*` — check against keyword table.
4. **Upper identifier**: `[A-Z][a-zA-Z0-9_']*`.
5. **Operator**: `[!#$%&*+./<=>?@\\^|~-]+` — but exclude reserved sequences (`->`, `<-`, `::`, `:=`, `\`, `_`, `=`, `|`, `@`, `:`, `.`).
6. **Integer**: `[0-9]+` — only for fixity declarations.
7. **Special single chars**: `(`, `)`, `{`, `}`, `,`, `;`.
8. **Multi-char special**: `->`, `<-`, `::`, `:=` — disambiguate by longest match.

#### Operator vs special disambiguation

`|` alone is `TokPipe` (row syntax). `||` is `TokOp`. `@` alone is `TokAt` (type application). `\` alone is `TokBackslash` (lambda). `_` alone is `TokUnderscore` (wildcard). `=` alone is `TokEq` (data declaration). `==` is `TokOp`.

### 4.3 Surface AST (`syntax/ast.go`)

Surface AST is distinct from Core IR. It represents parsed syntax before desugaring and elaboration.

```go
// ---- Expressions ----

type Expr interface {
    exprNode()
    Span() span.Span
}

type ExprVar struct {        // variable
    Name string
    S    span.Span
}

type ExprCon struct {        // constructor
    Name string
    S    span.Span
}

type ExprApp struct {        // function application
    Fun Expr
    Arg Expr
    S   span.Span
}

type ExprTyApp struct {      // type application (e @T)
    Expr    Expr
    TyArg   TypeExpr
    S       span.Span
}

type ExprLam struct {        // lambda (\p1 p2 -> e)
    Params []Pattern
    Body   Expr
    S      span.Span
}

type ExprCase struct {       // case e of { alts }
    Scrutinee Expr
    Alts      []AstAlt
    S         span.Span
}

type ExprDo struct {         // do { stmts }
    Stmts []Stmt
    S     span.Span
}

type ExprBlock struct {      // { x := e; body }
    Binds []AstBind
    Body  Expr
    S     span.Span
}

type ExprInfix struct {      // a `op` b (before fixity resolution)
    Left  Expr
    Op    string
    Right Expr
    S     span.Span
}

type ExprAnn struct {        // e :: T
    Expr    Expr
    AnnType TypeExpr
    S       span.Span
}

type ExprParen struct {      // (e)
    Inner Expr
    S     span.Span
}

// ---- Statements (in do blocks) ----

type Stmt interface {
    stmtNode()
    Span() span.Span
}

type StmtBind struct {       // x <- c;
    Var  string
    Comp Expr
    S    span.Span
}

type StmtPureBind struct {   // x := e;
    Var  string
    Expr Expr
    S    span.Span
}

type StmtExpr struct {       // c;
    Expr Expr
    S    span.Span
}

type StmtTail struct {       // c  (final expression, no semicolon)
    Expr Expr
    S    span.Span
}

// ---- Block bindings ----

type AstBind struct {        // x := e;  (in block expressions)
    Var  string
    Expr Expr
    S    span.Span
}

// ---- Case alternatives ----

type AstAlt struct {         // pattern -> expr;
    Pattern Pattern
    Body    Expr
    S       span.Span
}

// ---- Patterns ----

type Pattern interface {
    patternNode()
    Span() span.Span
}

type PatVar struct {
    Name string
    S    span.Span
}

type PatWild struct {
    S span.Span
}

type PatCon struct {
    Con  string
    Args []Pattern
    S    span.Span
}

type PatParen struct {
    Inner Pattern
    S     span.Span
}
```

### 4.4 Type Syntax AST (`syntax/typeast.go`)

```go
// TypeExpr is a surface-level type expression (before kind checking).
type TypeExpr interface {
    typeExprNode()
    Span() span.Span
}

type TyExprVar struct {
    Name string
    S    span.Span
}

type TyExprCon struct {
    Name string
    S    span.Span
}

type TyExprApp struct {
    Fun TypeExpr
    Arg TypeExpr
    S   span.Span
}

type TyExprArrow struct {
    From TypeExpr
    To   TypeExpr
    S    span.Span
}

type TyExprForall struct {
    Binders []TyBinder
    Body    TypeExpr
    S       span.Span
}

type TyExprRow struct {
    Fields []TyRowField
    Tail   *TyExprVar   // nil = closed row
    S      span.Span
}

type TyExprParen struct {
    Inner TypeExpr
    S     span.Span
}

type TyBinder struct {
    Name string
    Kind *KindExpr   // nil = inferred
    S    span.Span
}

type TyRowField struct {
    Label string
    Type  TypeExpr
    S     span.Span
}

// ---- Kind expressions ----

type KindExpr interface {
    kindExprNode()
    Span() span.Span
}

type KindExprType struct { S span.Span }
type KindExprRow  struct { S span.Span }
type KindExprArrow struct {
    From KindExpr
    To   KindExpr
    S    span.Span
}
```

### 4.5 Declarations AST (`syntax/decl.go`)

```go
type Decl interface {
    declNode()
    Span() span.Span
}

type DeclTypeAnn struct {       // f :: T
    Name string
    Type TypeExpr
    S    span.Span
}

type DeclValueDef struct {      // f := e
    Name string
    Expr Expr
    S    span.Span
}

type DeclData struct {          // data T a b = C1 ... | C2 ...
    Name     string
    Params   []TyBinder
    Cons     []DeclCon
    S        span.Span
}

type DeclCon struct {           // C T1 T2
    Name   string
    Fields []TypeExpr
    S      span.Span
}

type DeclTypeAlias struct {     // type F a = T
    Name   string
    Params []TyBinder
    Body   TypeExpr
    S      span.Span
}

type DeclFixity struct {        // infixl N op
    Assoc Assoc
    Prec  int
    Op    string
    S     span.Span
}

type Assoc int
const (
    AssocLeft  Assoc = iota
    AssocRight
    AssocNone
)

// Program is the top-level AST.
type AstProgram struct {
    Decls []Decl
}
```

### 4.6 Parser (`syntax/parser.go`)

Pratt parser with 5 precedence levels (spec §5.5).

```go
type Parser struct {
    tokens  []Token
    pos     int
    fixity  map[string]Fixity
    errors  *errs.Errors
}

type Fixity struct {
    Assoc Assoc
    Prec  int
}

// NewParser creates a parser from a token stream.
func NewParser(tokens []Token, errors *errs.Errors) *Parser

// ParseProgram parses a complete program.
func (p *Parser) ParseProgram() *AstProgram

// ParseExpr parses a single expression (for REPL or testing).
func (p *Parser) ParseExpr() Expr

// ParseType parses a type expression.
func (p *Parser) ParseType() TypeExpr
```

#### Pratt parsing algorithm

```go
// parseExpr is the core Pratt entry point.
func (p *Parser) parseExpr() Expr {
    return p.parseAnnotation()
}

func (p *Parser) parseAnnotation() Expr {
    e := p.parseInfix(0)
    if p.match(TokColonColon) {
        ty := p.parseType()
        return &ExprAnn{Expr: e, AnnType: ty, ...}
    }
    return e
}

func (p *Parser) parseInfix(minPrec int) Expr {
    left := p.parseApp()
    for p.peek().Kind == TokOp {
        op := p.peek().Text
        fix := p.lookupFixity(op) // default: infixl 9
        if fix.Prec < minPrec {
            break
        }
        p.advance()
        // Right-binding power depends on associativity:
        //   left-assoc:  nextMinPrec = fix.Prec + 1
        //   right-assoc: nextMinPrec = fix.Prec
        //   non-assoc:   nextMinPrec = fix.Prec + 1 (and disallow chaining)
        nextMin := fix.Prec + 1
        if fix.Assoc == AssocRight {
            nextMin = fix.Prec
        }
        right := p.parseInfix(nextMin)
        left = &ExprInfix{Left: left, Op: op, Right: right, ...}
    }
    return left
}

func (p *Parser) parseApp() Expr {
    f := p.parseAtom()
    for p.isAtomStart() || p.peek().Kind == TokAt {
        if p.match(TokAt) {
            ty := p.parseTypeAtom()
            f = &ExprTyApp{Expr: f, TyArg: ty, ...}
        } else {
            arg := p.parseAtom()
            f = &ExprApp{Fun: f, Arg: arg, ...}
        }
    }
    return f
}

func (p *Parser) parseAtom() Expr {
    switch p.peek().Kind {
    case TokLower:     return &ExprVar{...}
    case TokUpper:     return &ExprCon{...}
    case TokLParen:    return p.parseParen()
    case TokBackslash: return p.parseLambda()
    case TokLBrace:    return p.parseBlockOrEmpty()
    case TokCase:      return p.parseCase()
    case TokDo:        return p.parseDo()
    default:
        p.error("expected expression")
    }
}
```

#### Block vs do disambiguation

Both start with `{`. Resolution:
- `do { ... }` is preceded by `TokDo`.
- `{ x := e; ... }` or `{ e }` is a block expression, parsed when `{` appears at atom position without `do`.

#### Declaration parsing

```go
func (p *Parser) parseDecl() Decl {
    switch {
    case p.peek().Kind == TokData:
        return p.parseDataDecl()
    case p.peek().Kind == TokType:
        return p.parseTypeAlias()
    case p.isFixityKeyword():
        return p.parseFixityDecl()
    case p.peek().Kind == TokLower:
        // Could be TypeAnnotation (f :: T) or ValueDefinition (f := e).
        // Peek ahead: if next is ::, it's an annotation; if :=, it's a definition.
        return p.parseNamedDecl()
    }
}
```

#### Fixity processing

Fixity declarations (`infixl 6 +`) are processed in a **first pass** before expression parsing. The parser scans all declarations, extracts fixity declarations, builds the fixity table, then parses expressions with resolved precedences.

Alternative: two-pass parsing is avoided if fixity declarations must appear before use. For simplicity, require fixity declarations to precede all value definitions.

### 4.7 Error Recovery

The parser should not stop at the first error. Synchronization strategy:
- On error in a declaration, skip to the next declaration boundary (next top-level identifier at column 0 or next keyword).
- On error in an expression, skip to the next `;` or `}`.
- Record all errors in `*errs.Errors`.

## Test Strategy

### Lexer tests

- Single token types: keywords, identifiers, operators, delimiters.
- Multi-char disambiguation: `->` vs `-`, `::` vs `:`, `:=` vs `:`.
- Comments: line comments, nested block comments.
- Error cases: unterminated block comment, invalid characters.
- Token spans: verify start/end positions.

### Parser tests

- **Expressions**: variable, constructor, application (left-assoc), lambda (multi-param), case, do block, block expression, type annotation, type application, infix operators.
- **Operator precedence**: `a + b * c` with different fixity tables.
- **Associativity**: left-assoc, right-assoc, non-assoc (error on chaining).
- **Types**: arrow (right-assoc), forall, row types, type application.
- **Declarations**: data declarations (multi-constructor), type alias, value definitions, fixity declarations, type annotations.
- **Programs**: full programs with multiple declarations.
- **Error recovery**: malformed expressions produce errors but parsing continues.
- **Assumption syntax**: `f := assumption` parses as a value definition where the RHS is `ExprVar{Name: "assumption"}`.

### Round-trip tests

Parse → pretty-print AST → verify structure matches input (modulo whitespace).

## Completion Criteria

- [ ] Lexer tokenizes all spec §5 constructs correctly
- [ ] Parser produces correct AST for all expression forms
- [ ] Pratt parsing handles fixity correctly
- [ ] Type syntax parser handles forall, rows, arrows
- [ ] Declaration parser handles all 5 declaration forms
- [ ] do block and block expression syntax both work
- [ ] Error recovery keeps parsing after first error
- [ ] All tests pass
- [ ] No dependency on `core/` or `eval/`
