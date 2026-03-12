# Expression Grammar, Parsing Strategy, and Concrete Syntax Design

One-line description: complete formal grammar, token design, precedence resolution, and Pratt parsing algorithm for Gomputation's Haskell-like surface syntax, grounded in the spec v0.2 commitments.

## Table of Contents

1. Overview and Design Constraints
2. Token Design
3. Expression Grammar (EBNF)
4. Type Expression Grammar
5. Declaration Grammar
6. Do-Notation and Statement Grammar
7. Pattern Grammar
8. Precedence and Associativity
9. Layout Rule vs Explicit Delimiters
10. Pratt Parsing: Theory and Algorithm
11. Integrating Special Forms into Pratt Parsing
12. Go Implementation Architecture
13. AST Representation
14. Error Recovery
15. Comparison with Haskell, PureScript, Elm, and Koka
16. Formal Grammar Summary (Collected)
17. Key References

---

## 1. Overview and Design Constraints

### 1.1 What This Document Specifies

The spec v0.2 (Section 20, item 1) identifies "explicit expression grammar" as the top priority for the next draft. This document provides:

- A complete, unambiguous expression grammar for the surface language.
- A token table suitable for hand-written lexing.
- A precedence table that handles user-defined operators.
- A Pratt parsing algorithm adapted for Gomputation's specific forms.
- Go implementation patterns for lexer, parser, and AST.

### 1.2 Surface Syntax Commitments (from Spec v0.2)

The following are already committed. The grammar must accommodate all of them without ambiguity.

| Form | Syntax | Source |
|---|---|---|
| Type annotation (decl) | `Name :: Type` | Section 13.1 |
| Value definition | `Name := Expr` | Section 13.2 |
| Data declaration | `data Name = Constructor*` | Section 13.3 |
| Primitive declaration | `primitive Name :: Type` | Section 13.4 |
| Fixity declaration | `infixl n op` / `infixr n op` / `infix n op` | Section 13.5 |
| Lambda | `\x -> e` | Section 12 |
| Function application | `f x` (left-associative) | Section 12 |
| Type application | `f @Type` | v0.1 Section 13 |
| Do block | `do { Stmt* }` | v0.1 Section 14 |
| Case analysis | `case s of { alts }` | Section 14.7 |
| Pure binding in do | `x := e` | v0.1 Section 15.1 |
| Computation binding in do | `x <- e` | v0.1 Section 15.2 |
| If expression | `if e then e else e` | v0.1 Section 10 |

### 1.3 Design Principles

Three principles govern the grammar design:

**Determinism.** The grammar must be parseable by a deterministic top-down parser with finite lookahead. No ambiguity that requires backtracking beyond a fixed bound.

**Orthogonality to the core.** The surface grammar is a separate concern from the core type theory. Parsing produces an AST that is then elaborated into the core. The grammar may include forms (if-then-else, do-notation, operator expressions) that have no direct core counterpart.

**Minimal surprise for Haskell readers.** Where the spec does not constrain, the grammar should follow Haskell conventions. Where it departs (e.g., `:=` for definition, `::` for annotation), the departure should be consistent and parseable.

### 1.4 Key Tension Points

Several interactions require careful resolution:

1. **Lambda vs operators.** `\x -> x + 1` -- the lambda body extends to the end of the expression, capturing the operator. Lambda has the lowest precedence among expression forms.

2. **Do/case/if vs operators.** `f (do { ... }) + 1` -- block-like expressions appearing as operands. These must either be parenthesized in operator contexts or have clear boundary rules.

3. **Type annotation in expressions.** `(e :: T)` within expressions vs `Name :: T` at declaration level. The `::` token is overloaded and must be disambiguated by context.

4. **`:=` in do vs at top level.** Inside a do block, `x := e` is a pure binding statement. At top level, `Name := Expr` is a value definition. The token `:=` is shared but the context (do body vs program top level) disambiguates.

5. **User-defined operators.** Precedence and associativity are declared by the user, not fixed in the grammar. The parser must be parametric in the operator table.

---

## 2. Token Design

### 2.1 Token Categories

The lexer produces tokens in the following categories.

| Category | Examples | Notes |
|---|---|---|
| **Keyword** | `data`, `case`, `of`, `do`, `if`, `then`, `else`, `forall`, `pure`, `infixl`, `infixr`, `infix`, `primitive`, `where` | Reserved; cannot be identifiers |
| **Identifier** | `x`, `foo`, `dbOpen`, `DB` | Lowercase-initial = term-level; uppercase-initial = type/constructor |
| **Operator** | `+`, `-`, `*`, `.`, `$`, `>>`, `>>=`, `<$>` | Sequences of operator characters |
| **Special operator** | `->`, `::`, `:=`, `<-`, `\`, `@`, `\|`, `=` | Fixed meaning; not user-redefinable |
| **Integer literal** | `0`, `42`, `-1` | Negative literals handled at parse level |
| **String literal** | `"hello"`, `"it's \"quoted\""` | Double-quoted, backslash escaping |
| **Delimiter** | `(`, `)`, `{`, `}`, `[`, `]` | Grouping |
| **Punctuation** | `,`, `;`, `.` | Separators (`.` is also an operator when in operator position) |
| **EOF** | | End of input |

### 2.2 Keyword Table

```text
Keywords (14):
  data      case      of        do
  if        then      else      forall
  pure      infixl    infixr    infix
  primitive where
```

All keywords are lowercase and alphabetic. An identifier that matches a keyword is lexed as that keyword. This is the standard approach.

The keyword `where` is reserved for future use (local definitions). It is not active in the current grammar but is reserved to prevent its use as an identifier.

### 2.3 Identifier Rules

```text
IdentStart  = 'a'..'z' | 'A'..'Z' | '_'
IdentCont   = IdentStart | '0'..'9' | '\''

LowerIdent  = ('a'..'z' | '_') IdentCont*     -- term variables, field labels
UpperIdent  = 'A'..'Z' IdentCont*              -- type names, constructors
```

The prime character (`'`) is permitted in identifiers (e.g., `x'`, `acc'`), following Haskell convention.

Underscore-initial identifiers (`_foo`, `_`) are permitted. The bare underscore `_` is reserved as a wildcard in patterns.

### 2.4 Operator Rules

```text
OpChar  = '!' | '#' | '$' | '%' | '&' | '*' | '+' | '.' | '/'
        | '<' | '=' | '>' | '?' | '@' | '^' | '|' | '-' | '~' | ':'

Operator = OpChar+
```

However, certain character sequences are lexed as special tokens regardless of context:

```text
Special tokens (not user-redefinable):
  ->    ::    :=    <-    \    @    |    =    ..
```

The rule is: the lexer attempts to match special tokens first (longest match among specials), then falls back to the general operator rule. This ensures `->` is never parsed as `-` followed by `>`.

Operators beginning with `:` (other than `::` and `:=`) are reserved for constructor operators (infix constructors), following Haskell convention. This is a future extension point; the current grammar does not use constructor operators but reserves the namespace.

### 2.5 Numeric Literals

```text
IntLiteral   = Digit+
FloatLiteral = Digit+ '.' Digit+

Digit = '0'..'9'
```

Integer and float literals are unsigned at the token level. Negative numbers are parsed as the application of a prefix negation operator.

### 2.6 String Literals

```text
StringLiteral = '"' StringChar* '"'

StringChar = <any character except '"' and '\'>
           | '\n' | '\t' | '\\' | '\"' | '\0'
```

Only double-quoted strings. No multi-line string literals in the initial grammar. No string interpolation.

### 2.7 Comments

```text
LineComment  = '--' <everything until newline>
BlockComment = '{-' <everything until '-}'>    -- nestable
```

Block comments nest, following Haskell. This means `{- {- inner -} outer -}` is a single comment.

### 2.8 Whitespace and Newlines

Whitespace (spaces, tabs, newlines) is skipped between tokens, with the exception noted in Section 9 (layout rules).

If layout rules are adopted, the lexer must track indentation and insert virtual `{`, `}`, and `;` tokens. If explicit delimiters are used exclusively, no layout processing is needed.

---

## 3. Expression Grammar (EBNF)

### 3.1 Grammar Notation

The grammar uses the following EBNF conventions:

```text
A B       -- sequence
A | B     -- alternative
A?        -- optional
A*        -- zero or more
A+        -- one or more
( ... )   -- grouping
'x'       -- terminal token
```

Non-terminals are capitalized. Terminals are quoted or written in `monospace`. Comments use `--`.

### 3.2 Expression Stratification

The expression grammar is stratified into levels that enforce precedence without ambiguity. The key insight is that precedence is handled by nesting: lower-precedence forms are defined in terms of higher-precedence forms.

However, because Gomputation supports user-defined operators with user-declared precedence, the infix-expression level cannot be statically stratified in the grammar. Instead, the grammar defines a single `InfixExpr` non-terminal whose internal structure is resolved by the Pratt parser at parse time.

The static stratification handles the non-operator precedence levels:

```text
Expr          -- top: lambda, if, case, do, let
InfixExpr     -- operators (user-defined precedence)
PrefixExpr    -- prefix operators (negation)
AppExpr       -- function application, type application
AtomExpr      -- literals, variables, parenthesized, sections
```

### 3.3 Top-Level Expression

```ebnf
Expr
  = '\' LamParams '->' Expr                  -- lambda
  | 'if' Expr 'then' Expr 'else' Expr        -- conditional
  | 'case' Expr 'of' '{' Alt (';' Alt)* '}'  -- case analysis
  | 'do' '{' Stmt (';' Stmt)* '}'            -- do block
  | InfixExpr '::' Type                       -- type annotation (low prec)
  | InfixExpr                                 -- fall through to operators

LamParams
  = LamParam+

LamParam
  = LowerIdent                                -- term parameter
  | '(' LowerIdent '::' Type ')'              -- annotated parameter
```

Lambda extends as far right as possible. `\x -> x + 1` parses as `\x -> (x + 1)`, not `(\x -> x) + 1`. This is achieved by making the lambda body a full `Expr`, which includes operators.

Type annotation `e :: T` at the expression level has lower precedence than all operators. This means `x + 1 :: Int` parses as `(x + 1) :: Int`. If annotation of a subexpression is desired, parentheses are required: `(x :: Int) + 1`.

The conditional `if-then-else` similarly extends as far as its three subexpressions allow. `if p then x + 1 else y * 2` is unambiguous because `then` and `else` act as delimiters.

### 3.4 Infix Expressions

```ebnf
InfixExpr
  = PrefixExpr (Operator PrefixExpr)*        -- resolved by Pratt parser
```

At the grammar level, `InfixExpr` is a flat sequence of prefix expressions separated by operators. The Pratt parser uses the operator precedence table to build the correct tree. This is the standard approach for languages with user-defined operators.

The backtick notation allows any identifier to be used as an infix operator:

```ebnf
Operator
  = OpToken                                   -- symbolic operator
  | '`' LowerIdent '`'                        -- identifier used as infix
  | '`' UpperIdent '`'                        -- constructor used as infix
```

### 3.5 Prefix Expressions

```ebnf
PrefixExpr
  = '-' AppExpr                               -- numeric negation
  | AppExpr
```

Prefix negation binds tighter than infix operators but looser than application. `-f x` is `-(f x)`, not `(-f) x`. This matches Haskell.

### 3.6 Application Expressions

```ebnf
AppExpr
  = AppExpr AtomExpr                          -- function application
  | AppExpr '@' TypeAtom                      -- type application
  | AtomExpr

-- Left-recursive definition for clarity. In implementation, parsed iteratively.
```

Function application is left-associative and has the highest precedence among non-atomic forms. `f x y` is `(f x) y`. `f @Int x` is `(f @Int) x`.

Type application uses the `@` sigil. `f @Int` applies the type `Int` to the polymorphic function `f`. The argument to `@` is a `TypeAtom` (a type expression without top-level arrows or foralls -- see Section 4).

### 3.7 Atomic Expressions

```ebnf
AtomExpr
  = LowerIdent                                -- variable
  | UpperIdent                                -- constructor
  | IntLiteral                                -- integer
  | FloatLiteral                              -- float
  | StringLiteral                             -- string
  | '(' Expr ')'                              -- parenthesized expression
  | '(' Operator ')'                          -- operator section (prefix form)
  | '(' Expr Operator ')'                     -- left section
  | '(' Operator Expr ')'                     -- right section
  | 'pure'                                    -- pure as value
```

Operator sections follow Haskell convention. `(+)` denotes the function form of the `+` operator. `(+ 1)` is a right section (a function that adds 1). `(1 +)` is a left section (a function that adds 1 on the left).

The keyword `pure` appears here as an atomic expression because it can be used as a function: `pure x` is application of `pure` to `x`.

### 3.8 Disambiguation: Lambda Body Extent

The critical parsing question: how far does a lambda body extend?

Rule: **The body of a lambda is a full `Expr`.** This means the lambda body captures everything to its right until an enclosing delimiter (`)`, `}`, `;`, `of`, `then`, `else`) or end of input.

Examples:

```text
\x -> x + 1           -- parses as \x -> (x + 1)
\x -> if p then a else b  -- parses as \x -> (if p then a else b)
\x -> \y -> x + y     -- parses as \x -> (\y -> (x + y))
f (\x -> x + 1) y     -- lambda body extends to closing paren
```

This is exactly Haskell's behavior.

### 3.9 Disambiguation: Block Expressions as Operands

Block-like expressions (`do`, `case`, `if`) can appear as arguments to functions or as operands of operators, but only when parenthesized or when the parser can determine the block boundary.

```text
f (do { x <- g; pure x })      -- parenthesized: unambiguous
f do { x <- g; pure x }        -- without parens: allowed by special rule
```

The rule: `do`, `case`, and `if` in argument position (within `AppExpr`) are allowed **only** as the last argument before an operator or end of expression. This avoids ambiguity.

More precisely: when parsing `AppExpr`, encountering `do`, `case`, or `if` terminates function application and begins parsing the block as a single argument. The resulting block expression becomes the last argument.

```text
f x do { ... }          -- parses as f x (do { ... })
f do { ... } + 1        -- parses as (f (do { ... })) + 1
```

This follows the GHC extension `BlockArguments` behavior, which has become standard in modern Haskell. For the initial implementation, requiring parentheses around block arguments is the safer choice.

**Recommended initial policy:** Require parentheses around `do`, `case`, and `if` when used as function arguments. This eliminates all ambiguity and requires no special parsing rules. The `BlockArguments` extension can be added later.

---

## 4. Type Expression Grammar

### 4.1 Type Expression Stratification

Type expressions have their own grammar, separate from term expressions. The two grammars share some tokens (`->`, `forall`) but are invoked in different parsing contexts.

```ebnf
Type
  = 'forall' TypeParam+ '.' Type              -- universal quantification
  | TypeInfix                                  -- function types and user-defined type operators

TypeInfix
  = TypeApp '->' Type                          -- function type (right-associative)
  | TypeApp

TypeApp
  = TypeApp TypeAtom                           -- type application (left-assoc)
  | TypeAtom

TypeAtom
  = UpperIdent                                 -- named type (Int, String, DB)
  | LowerIdent                                 -- type variable (a, r)
  | '(' Type ')'                               -- parenthesized
  | '(' Type ',' Type (',' Type)* ')'          -- tuple type
  | RowType                                    -- row type
  | 'Computation'                              -- the Computation family

TypeParam
  = LowerIdent                                 -- implicitly kinded
  | '(' LowerIdent '::' Kind ')'              -- explicitly kinded
```

### 4.2 Function Type

`->` in types is **right-associative**: `A -> B -> C` is `A -> (B -> C)`. This matches lambda calculus convention and Haskell.

`->` in types binds looser than type application: `F A -> G B` is `(F A) -> (G B)`.

### 4.3 Computation Type

`Computation` is parsed as an ordinary type constructor applied to three arguments:

```text
Computation { db : DB[Opened] | r } { db : DB[Closed] | r } Unit
```

This is `Computation` applied to three `TypeAtom`s: two row types and `Unit`. No special syntax is needed beyond the general type application rule.

### 4.4 Row Types

```ebnf
RowType
  = '{' '}'                                    -- empty row
  | '{' RowField (',' RowField)* '}'           -- closed row
  | '{' RowField (',' RowField)* '|' LowerIdent '}'  -- open row

RowField
  = LowerIdent ':' Type
```

The `|` in row types is the row extension separator, not the alternation operator. It is lexed as a special token in this context (or can be treated as the operator `|` and disambiguated by the parser since it appears inside `{ ... }`).

### 4.5 Indexed Type Arguments

The spec shows types like `DB[Opened]`. The brackets here denote type-level arguments to type constructors. This can be parsed as:

```ebnf
TypeAtom
  = ...
  | UpperIdent '[' Type (',' Type)* ']'        -- indexed type
```

Alternatively, `DB[Opened]` can be treated as syntactic sugar for `DB Opened` (ordinary type application), with the brackets being mere visual markers. The choice is:

**Option A: Brackets are sugar for type application.** `DB[Opened]` desugars to `DB Opened`. Simpler grammar, less visual distinction.

**Option B: Brackets are a distinct syntactic form.** `DB[Opened]` and `DB Opened` are both valid but the bracket form signals "index argument" vs "type argument." More informative surface syntax.

**Recommendation:** Option A for v0. The brackets are parsed as sugar for juxtaposition application. This avoids adding a new syntactic category and keeps the grammar minimal. The visual convention can be enforced by a formatter.

### 4.6 Kind Expressions

Kinds are a small language:

```ebnf
Kind
  = 'Type'
  | 'Row'
  | Kind '->' Kind                             -- higher kinds (future)
  | '(' Kind ')'
```

Kinds appear only in explicit kind annotations: `forall (a :: Type). T` or `forall (r :: Row). T`.

---

## 5. Declaration Grammar

### 5.1 Program Structure

```ebnf
Program
  = Decl*

Decl
  = TypeAnnotation
  | ValueDefinition
  | DataDeclaration
  | PrimitiveDeclaration
  | FixityDeclaration
```

Declarations are separated by semicolons (explicit) or newlines (if layout rules are used).

### 5.2 Individual Declaration Forms

```ebnf
TypeAnnotation
  = LowerIdent '::' Type

ValueDefinition
  = LowerIdent ':=' Expr

DataDeclaration
  = 'data' UpperIdent TypeParam* '=' DataBody

DataBody
  = ConDecl ('|' ConDecl)*

ConDecl
  = UpperIdent TypeAtom*                       -- constructor with fields

PrimitiveDeclaration
  = 'primitive' LowerIdent '::' Type

FixityDeclaration
  = ('infixl' | 'infixr' | 'infix') IntLiteral Operator
```

### 5.3 The `Name ::` vs `Name :=` Disambiguation

Both type annotations and value definitions begin with `LowerIdent`. They are distinguished by the second token:

- `::` means type annotation.
- `:=` means value definition.

This requires one token of lookahead after the identifier. Since the lexer produces `::` and `:=` as distinct tokens, the disambiguation is trivial.

### 5.4 Data Declaration Details

```text
data Maybe a = Nothing | Just a
data List a = Nil | Cons a (List a)
data DBState = Opened | Closed
```

Constructors are `UpperIdent` followed by zero or more `TypeAtom`s (not full `Type`s, to avoid ambiguity with `|`). To use a function type as a constructor field, parentheses are required:

```text
data Callback a = MkCallback (a -> Computation {} {} Unit)
```

This matches Haskell's treatment.

---

## 6. Do-Notation and Statement Grammar

### 6.1 Do Block Structure

```ebnf
DoBlock
  = 'do' '{' StmtList '}'

StmtList
  = Stmt (';' Stmt)*

Stmt
  = LowerIdent ':=' Expr                       -- pure binding
  | LowerIdent '<-' Expr                       -- computation binding
  | Expr                                        -- expression statement or tail
```

The last `Stmt` in a `StmtList` must be an `Expr` (not a binding). This is the tail expression -- the return value of the do block.

### 6.2 Pure Binding

```text
x := expr
```

Elaborates to `(\x -> rest) expr`. The right-hand side is a pure expression; it does not involve `bind`.

### 6.3 Computation Binding

```text
x <- expr
```

Elaborates to `bind expr (\x -> rest)`. The right-hand side must have type `Computation r1 r2 A`.

### 6.4 Expression Statement

```text
expr;
```

An expression statement with no binding. If the expression is a computation, this elaborates to `bind expr (\_ -> rest)`. If the expression is the last in the block (no semicolon), it is the return value.

### 6.5 Disambiguation: Statement Leading Identifier

A statement beginning with `LowerIdent` could be:

1. A pure binding: `x := e`
2. A computation binding: `x <- e`
3. An expression beginning with a variable: `f x + 1`

The parser reads `LowerIdent`, then looks at the next token:

- `:=` -> pure binding
- `<-` -> computation binding
- anything else -> expression (reparse the identifier as the start of an `Expr`)

This requires one token of lookahead after the identifier, which is the same as the declaration-level disambiguation.

### 6.6 Nested Do Blocks

Do blocks nest naturally:

```text
do {
  x <- do {
    y <- getInput;
    pure (process y)
  };
  output x
}
```

The inner `do` is an expression on the right-hand side of `<-`. No special rules are needed.

### 6.7 Operators Inside Do Statements

Operators inside do block statements follow normal expression parsing:

```text
do {
  x := a + b * c;
  y <- compute x >>= transform;
  pure (x + y)
}
```

Each statement's expression part is a full `Expr` (including operator expressions). The semicolon terminates the expression.

---

## 7. Pattern Grammar

### 7.1 Case Expression

```ebnf
CaseExpr
  = 'case' Expr 'of' '{' Alt (';' Alt)* '}'

Alt
  = Pattern '->' Expr
```

### 7.2 Pattern Forms

```ebnf
Pattern
  = LowerIdent                                 -- variable binding
  | '_'                                        -- wildcard
  | UpperIdent Pattern*                        -- constructor pattern
  | IntLiteral                                 -- integer literal pattern
  | StringLiteral                              -- string literal pattern
  | '(' Pattern ')'                            -- parenthesized
  | '(' Pattern ',' Pattern (',' Pattern)* ')' -- tuple pattern
```

Constructor patterns are analogous to constructor application in expressions: `Just x`, `Cons h t`, `Nothing`. Nested patterns require parentheses: `Cons (Just x) rest`.

### 7.3 Exhaustiveness

Pattern exhaustiveness checking is a separate concern from parsing. The parser accepts any syntactically valid pattern; the type checker rejects non-exhaustive case expressions.

---

## 8. Precedence and Associativity

### 8.1 Fixed Precedence Levels

The following precedence levels are fixed in the grammar structure, not by the operator table:

| Level | Form | Associativity | Notes |
|---|---|---|---|
| 0 (lowest) | `\x -> e`, `if-then-else`, `case-of`, `do` | extends right | Body is a full `Expr` |
| 1 | `e :: T` | non-associative | Type annotation |
| 2 | `$` (conventional) | right | Application operator |
| 3--9 | User-defined operators | per declaration | Pratt parser resolves |
| 10 | Prefix negation `-` | prefix | Unary minus |
| 11 (highest) | Function application `f x`, type application `f @T` | left | Juxtaposition |
| 12 | Atoms | n/a | Literals, variables, parens |

### 8.2 User-Defined Operator Precedence

User-declared operators occupy levels 0 through 9 (following Haskell's convention, where 0 is lowest and 9 is highest). The fixity declarations:

```text
infixl 6 +
infixl 6 -
infixl 7 *
infixl 7 /
infixr 9 .
infixr 0 $
```

An operator with no fixity declaration is treated as `infixl 9` (highest user precedence, left-associative). This is a pragmatic default that makes undeclared operators bind tightly.

### 8.3 Precedence Table (Default Library)

A standard prelude would declare:

| Precedence | Associativity | Operators |
|---|---|---|
| 0 | right | `$` |
| 1 | right | `>>=`, `>>` |
| 2 | right | `\|\|` |
| 3 | right | `&&` |
| 4 | non-assoc | `==`, `/=`, `<`, `>`, `<=`, `>=` |
| 5 | right | `:` (cons, future) |
| 6 | left | `+`, `-` |
| 7 | left | `*`, `/` |
| 8 | right | `^` |
| 9 | right | `.` |

These match Haskell's standard precedences. They are not built into the parser; they are declared in source or a prelude and loaded into the operator table before parsing expressions.

### 8.4 The `::` Annotation Precedence

Type annotation `::` sits below all user operators (at effective precedence -1, below the user's 0--9 range). This means:

```text
x + 1 :: Int          -- parses as (x + 1) :: Int
f $ g x :: a -> b     -- parses as (f $ (g x)) :: (a -> b)
```

The `::` is non-associative: `e :: T1 :: T2` is a parse error.

### 8.5 The Interaction with Lambda

Lambda has even lower "precedence" than `::`:

```text
\x -> x + 1 :: Int     -- parses as \x -> ((x + 1) :: Int)
```

This is because the lambda body is a full `Expr`, which includes `::`.

Equivalently: lambda is not really at a "precedence level" -- it is a prefix form whose body extends to the right boundary. The Pratt parser handles it as a nud (null denotation) that consumes everything at the lowest binding power.

---

## 9. Layout Rule vs Explicit Delimiters

### 9.1 The Question

Should the language use Haskell-style layout rules (indentation-sensitive parsing) or require explicit `{`, `}`, `;` delimiters?

### 9.2 Arguments for Explicit Delimiters Only

- **Simpler lexer.** No indentation tracking, no virtual token insertion, no column arithmetic.
- **Simpler parser.** The parser sees exactly the tokens that are in the source.
- **Embeddability.** Programs may be embedded as strings in Go source code. Indentation in embedded strings is fragile.
- **PureScript precedent.** PureScript, the closest sibling to this design space, uses explicit layout-sensitive parsing but also supports explicit delimiters. Elm avoids layout rules entirely for `let` and `case`.
- **Error recovery.** Layout-related parse errors are notoriously confusing to report.

### 9.3 Arguments for Layout Rules

- **Reduced syntactic noise.** `do { x <- foo; y <- bar; pure (x + y) }` vs:
  ```text
  do
    x <- foo
    y <- bar
    pure (x + y)
  ```
- **Familiarity for Haskell users.** The spec explicitly targets a "Haskell-like surface."
- **Readability.** Less punctuation means the structure of programs is communicated by visual indentation, which programmers already maintain.

### 9.4 Recommendation

**Phase 1: Explicit delimiters only.** The initial implementation requires `{ }` and `;` for all block structures (`do`, `case of`, future `where`, `let in`). This keeps the lexer and parser simple and avoids layout-related complexity during the critical bootstrap phase.

**Phase 2: Optional layout rules.** Once the parser is stable, add a layout preprocessing pass that inserts virtual `{`, `}`, `;` tokens based on indentation. Programs written with explicit delimiters continue to work. The layout rule is purely a lexer-level transformation; the parser sees the same token stream in both modes.

### 9.5 Layout Rule Specification (for Phase 2)

The layout rule, when activated, applies after the keywords `do`, `of`, and `where`:

1. After `do`, `of`, or `where`, if the next token is not `{`, the lexer inserts a virtual `{` and records the column of the next token as the block's reference column.
2. Each subsequent line whose first token starts at the reference column gets a virtual `;` inserted before it.
3. Each subsequent line whose first token starts to the left of the reference column gets a virtual `}` inserted before it, closing the block.
4. If end-of-file is reached while a layout block is open, a virtual `}` is inserted.

This is the standard Haskell layout rule, simplified to the three contexts used by Gomputation.

---

## 10. Pratt Parsing: Theory and Algorithm

### 10.1 Overview

Pratt parsing (also called "top-down operator precedence parsing" or "precedence climbing") is a technique for parsing expressions with arbitrary operator precedence and associativity. It was introduced by Vaughan Pratt in 1973.

The core idea: each token has two associated parsing functions:

- **nud** (null denotation): how the token behaves when it appears at the beginning of an expression (as a prefix operator or atom).
- **led** (left denotation): how the token behaves when it appears after a left-hand operand (as an infix or postfix operator).

Each token also has a **binding power** (bp), which is a numeric precedence value. Higher binding power means tighter binding.

### 10.2 Binding Power

Binding power is the key concept. An infix operator with binding power `bp` claims operands that have binding power less than `bp` on the left, and less than `bp` (for left-associative) or less than or equal to `bp` (for right-associative) on the right.

More precisely, each infix operator has a **left binding power** (lbp) and a **right binding power** (rbp):

- **Left-associative:** `lbp = rbp = 2*n`, but the recursive call for the right operand uses `rbp` (same as lbp). Actually, the standard formulation is: `lbp = 2*n` and the right recursive call uses `lbp` (for left-assoc) or `lbp - 1` (for right-assoc).

A cleaner formulation uses two distinct values:

| Associativity | Left BP | Right BP |
|---|---|---|
| Left | 2*n + 1 | 2*n + 2 |
| Right | 2*n + 2 | 2*n + 1 |
| Non-associative | 2*n + 1 | 2*n + 1 |

Wait -- the most common and clearest formulation is as follows.

For a given operator at user-declared precedence level `n` (0 through 9):

| Associativity | Left BP (controls left operand grabbing) | Right BP (recursive call for right operand) |
|---|---|---|
| `infixl` | 2*n + 1 | 2*n + 2 |
| `infixr` | 2*n + 1 | 2*n + 1 |
| `infix` (non-assoc) | 2*n + 1 | 2*n + 2 (but emit error if same-precedence operator follows) |

The key: left-associative operators recurse at a **higher** right binding power, which prevents equal-precedence operators from being captured as the right operand. Right-associative operators recurse at the **same** binding power, which allows equal-precedence operators to be captured.

### 10.3 The Algorithm

```text
function parseExpr(minBP: int) -> AST:
    -- Parse the prefix part (nud)
    tok = advance()
    left = nud(tok)

    -- Parse infix operators (led) while their binding power is high enough
    while peek().lbp > minBP:
        tok = advance()
        left = led(tok, left)

    return left
```

The `nud` function handles:
- Atoms (variables, literals): return the AST node directly.
- Prefix operators (negation): recursively call `parseExpr(prefixBP)`.
- Prefix-like special forms: lambda, if, case, do.

The `led` function handles:
- Infix operators: recursively call `parseExpr(rbp)` for the right operand, then build a binary AST node.
- Postfix operators (if any): build a unary AST node from the left operand.
- Type annotation `::`: parse a type expression as the right operand.

### 10.4 Detailed Pseudocode

```text
type Parser struct {
    tokens   []Token
    pos      int
    fixities map[string]Fixity   // operator -> {prec, assoc}
}

type Fixity struct {
    prec  int       // 0..9
    assoc Assoc     // Left | Right | None
}

function (p *Parser) parseExpr(minBP int) AST {
    // --- NUD phase ---
    tok := p.advance()

    var left AST
    switch {
    case tok.is(LOWER_IDENT) || tok.is(UPPER_IDENT):
        left = Var{tok.text}

    case tok.is(INT_LIT):
        left = IntLit{tok.intVal}

    case tok.is(STRING_LIT):
        left = StringLit{tok.strVal}

    case tok.is(LPAREN):
        // Parenthesized expression or operator section
        if p.peek().is(OPERATOR) && p.peekAt(1).is(RPAREN) {
            // Prefix section: (+)
            op := p.advance()
            p.expect(RPAREN)
            left = OpSection{op.text}
        } else {
            inner := p.parseExpr(0)
            if p.peek().is(OPERATOR) && p.peekAt(1).is(RPAREN) {
                // Left section: (expr +)
                op := p.advance()
                p.expect(RPAREN)
                left = LeftSection{inner, op.text}
            } else {
                p.expect(RPAREN)
                left = Paren{inner}
            }
        }

    case tok.is(BACKSLASH):
        // Lambda: \params -> body
        params := p.parseLamParams()
        p.expect(ARROW)
        body := p.parseExpr(0)      // lowest BP: captures everything
        left = Lambda{params, body}

    case tok.is(KW_IF):
        cond := p.parseExpr(0)
        p.expect(KW_THEN)
        thenE := p.parseExpr(0)
        p.expect(KW_ELSE)
        elseE := p.parseExpr(0)
        left = IfExpr{cond, thenE, elseE}

    case tok.is(KW_CASE):
        scrutinee := p.parseExpr(0)
        p.expect(KW_OF)
        alts := p.parseAlts()
        left = CaseExpr{scrutinee, alts}

    case tok.is(KW_DO):
        stmts := p.parseDoBlock()
        left = DoExpr{stmts}

    case tok.is(KW_PURE):
        left = Var{"pure"}            // pure is a regular identifier here

    case tok.is(MINUS):
        // Prefix negation
        operand := p.parseExpr(NEGATE_BP)   // high BP
        left = Negate{operand}

    default:
        error("unexpected token: " + tok.text)
    }

    // --- LED phase ---
    for {
        next := p.peek()

        // Type annotation
        if next.is(DOUBLE_COLON) && minBP < ANNOTATION_BP {
            p.advance()
            ty := p.parseType()
            left = Annotation{left, ty}
            continue
        }

        // Infix operator
        if next.is(OPERATOR) || next.is(BACKTICK_IDENT) {
            fix := p.lookupFixity(next.text)
            lbp := fix.leftBP()
            if lbp <= minBP {
                break
            }
            p.advance()
            right := p.parseExpr(fix.rightBP())
            left = BinOp{left, next.text, right}
            continue
        }

        // Function application: the next token starts an AtomExpr
        // Application has the highest BP
        if p.isAppStart(next) && APP_BP > minBP {
            arg := p.parseAtomExpr()
            left = App{left, arg}
            continue
        }

        // Type application
        if next.is(AT) && APP_BP > minBP {
            p.advance()
            tyArg := p.parseTypeAtom()
            left = TypeApp{left, tyArg}
            continue
        }

        break
    }

    return left
}
```

### 10.5 Binding Power Constants

```text
ANNOTATION_BP = 1        // e :: T
LAMBDA_BP     = 0        // \x -> e (handled in nud, not via BP)
USER_OP_BASE  = 2        // user operators: BP = 2 + 2*prec ... 2 + 2*9
NEGATE_BP     = 24       // prefix negation: above all user operators
APP_BP        = 26       // function application: above negation
```

User operator at declared precedence `n` (0..9):

```text
leftBP(n)  = 2 + 2*n + 1   -- for infixl: 3, 5, 7, 9, 11, 13, 15, 17, 19, 21
rightBP(n, Left)  = 2 + 2*n + 2
rightBP(n, Right) = 2 + 2*n + 1
rightBP(n, None)  = 2 + 2*n + 2  (and check for adjacent same-prec operators)
```

Verification: user operator at prec 9 has lbp = 21, rbp = 22 (left) or 21 (right). Negation at 24 is above this. Application at 26 is above negation.

### 10.6 Why Pratt Parsing

Pratt parsing is the correct choice for Gomputation because:

1. **User-defined precedence.** The precedence table is not known at parser construction time. Pratt parsing takes the table as runtime data, not compile-time grammar structure.

2. **Simplicity.** The core algorithm is under 50 lines. Recursive descent with explicit precedence levels requires one function per level; Pratt collapses them into a single parameterized function.

3. **Extensibility.** Adding new prefix or infix forms requires adding a nud or led handler, not restructuring the grammar.

4. **Correctness.** The binding-power discipline provably produces the correct AST for any consistent precedence/associativity table.

5. **Go affinity.** Pratt parsers are naturally expressed in imperative style with a loop and a switch. This is idiomatic Go.

### 10.7 Comparison with Pure Recursive Descent

A pure recursive descent parser for operator expressions requires one function per precedence level:

```text
func parseAdd() AST { ... calls parseMul ... }
func parseMul() AST { ... calls parseUnary ... }
func parseUnary() AST { ... calls parseApp ... }
```

This is fine for languages with a fixed set of operators (C, Java, Go). It is awkward for languages with user-defined operators because:

- The number of levels is not known statically.
- Adding a new precedence level means restructuring the call chain.
- The parser must be regenerated or the descent functions must be generic.

Pratt parsing handles all of these naturally. The recursive descent skeleton is still used for non-operator parts of the grammar (declarations, types, statements); only the expression core uses Pratt.

---

## 11. Integrating Special Forms into Pratt Parsing

### 11.1 Lambda

Lambda is handled as a **nud**: when the parser encounters `\` at the start of an expression (or subexpression), it parses parameters, expects `->`, and recursively calls `parseExpr(0)` -- the lowest binding power -- for the body. This means the body captures everything to the right.

The binding power of 0 means: the lambda body extends until a token with lbp <= 0 is encountered, or until a closing delimiter stops the expression.

### 11.2 If-Then-Else

`if` is a nud. It parses three subexpressions separated by `then` and `else`:

```text
nud(IF):
    cond := parseExpr(0)
    expect(THEN)
    thenBranch := parseExpr(0)
    expect(ELSE)
    elseBranch := parseExpr(0)
    return IfExpr{cond, thenBranch, elseBranch}
```

Each branch is a full expression. The keywords `then` and `else` act as terminators for their preceding subexpressions.

Nested if-then-else associates to the right:

```text
if a then b else if c then d else e
= if a then b else (if c then d else e)
```

This is correct because the `else` branch parses a full `Expr`, which may itself begin with `if`.

### 11.3 Case-Of

`case` is a nud. It parses a scrutinee expression, expects `of`, then parses a braced block of alternatives:

```text
nud(CASE):
    scrutinee := parseExpr(0)
    expect(OF)
    expect(LBRACE)
    alts := []
    loop:
        pat := parsePattern()
        expect(ARROW)
        body := parseExpr(0)
        alts = append(alts, Alt{pat, body})
        if peek() == SEMICOLON:
            advance()
            continue loop
        break
    expect(RBRACE)
    return CaseExpr{scrutinee, alts}
```

### 11.4 Do Blocks

`do` is a nud. It parses a braced block of statements:

```text
nud(DO):
    expect(LBRACE)
    stmts := parseStmtList()
    expect(RBRACE)
    return DoExpr{stmts}
```

Statement parsing is described in Section 6.

### 11.5 Type Annotation

`::` is a **led** (left denotation). When the parser encounters `::` after a left-hand expression, it parses a type:

```text
led(DOUBLE_COLON, left):
    ty := parseType()
    return Annotation{left, ty}
```

The binding power of `::` is 1, which is below all user operators. This means `a + b :: T` parses as `(a + b) :: T`.

The `::` is non-associative. If another `::` follows, the parser does not consume it (because the lbp of `::` is not greater than the current minBP after the first `::` consumed its right operand).

### 11.6 Function Application as Implicit Infix

Function application is not triggered by a token; it is triggered by the *absence* of an operator between two expressions. In the Pratt parser, this is handled in the led loop:

```text
// In the led loop, after checking for operators:
if isAppStart(peek()) and APP_BP > minBP:
    arg := parseAtomExpr()
    left = App{left, arg}
    continue
```

`isAppStart` returns true if the next token can begin an atomic expression (identifier, literal, `(`, `[`). Function application has the highest binding power, so it captures arguments before any operator.

This means `f x + g y` parses as `(f x) + (g y)`, not `f (x + g) y` or any other grouping.

### 11.7 Type Application

Type application `f @T` is handled similarly to function application, as a led with the same high binding power:

```text
if peek() == AT and APP_BP > minBP:
    advance()
    tyArg := parseTypeAtom()
    left = TypeApp{left, tyArg}
    continue
```

Type application binds as tightly as function application. `id @Int x` is `(id @Int) x`.

---

## 12. Go Implementation Architecture

### 12.1 Overall Pipeline

```text
Source Text
    |
    v
  Lexer  ------>  []Token
    |
    v
  Parser ------>  AST
    |
    v
  Elaboration -->  Core IR
    |
    v
  Type Checker -->  Typed Core IR
    |
    v
  Evaluator ----->  Value
```

Each stage is a separate Go package. The lexer and parser are hand-written (no parser generators).

### 12.2 Lexer Implementation

The lexer is implemented as a struct with a `Next() Token` method:

```go
package lexer

type Lexer struct {
    src    []byte
    pos    int
    line   int
    col    int
}

type Token struct {
    Kind TokenKind
    Text string
    Pos  Position
}

type Position struct {
    Line   int
    Col    int
    Offset int
}

func New(src []byte) *Lexer { ... }
func (l *Lexer) Next() Token { ... }
```

The `Next` method is the sole interface. It skips whitespace, recognizes the next token, and advances the position. It returns a `Token` with kind, text, and source position.

Token kinds are an enum:

```go
type TokenKind int

const (
    // Literals
    TokInt TokenKind = iota
    TokFloat
    TokString

    // Identifiers
    TokLowerIdent
    TokUpperIdent

    // Operators
    TokOperator

    // Special tokens
    TokArrow        // ->
    TokDoubleColon  // ::
    TokColonEq      // :=
    TokLeftArrow    // <-
    TokBackslash    // \.
    TokAt           // @
    TokPipe         // |
    TokEquals        // =
    TokDotDot       // ..

    // Delimiters
    TokLParen       // (
    TokRParen       // )
    TokLBrace       // {
    TokRBrace       // }
    TokLBracket     // [
    TokRBracket     // ]

    // Punctuation
    TokComma        // ,
    TokSemicolon    // ;

    // Keywords
    TokData
    TokCase
    TokOf
    TokDo
    TokIf
    TokThen
    TokElse
    TokForall
    TokPure
    TokInfixl
    TokInfixr
    TokInfix
    TokPrimitive
    TokWhere

    // Meta
    TokEOF
)
```

### 12.3 Lexer Implementation Pattern: Single Dispatch on First Character

The `Next` method dispatches on the first character of the remaining input:

```go
func (l *Lexer) Next() Token {
    l.skipWhitespaceAndComments()
    if l.atEnd() {
        return l.makeToken(TokEOF, "")
    }

    ch := l.current()
    switch {
    case ch == '"':
        return l.lexString()
    case isDigit(ch):
        return l.lexNumber()
    case isIdentStart(ch):
        return l.lexIdentOrKeyword()
    case ch == '(':
        return l.singleChar(TokLParen)
    case ch == ')':
        return l.singleChar(TokRParen)
    case ch == '{':
        // Check for block comment {-
        if l.peekChar() == '-' {
            return l.skipBlockComment()
        }
        return l.singleChar(TokLBrace)
    case ch == '}':
        return l.singleChar(TokRBrace)
    case ch == '[':
        return l.singleChar(TokLBracket)
    case ch == ']':
        return l.singleChar(TokRBracket)
    case ch == ',':
        return l.singleChar(TokComma)
    case ch == ';':
        return l.singleChar(TokSemicolon)
    case isOpChar(ch):
        return l.lexOperatorOrSpecial()
    default:
        return l.error("unexpected character")
    }
}
```

The `lexOperatorOrSpecial` method reads a maximal run of operator characters, then checks if the result is a special token (`->`, `::`, `:=`, `<-`, etc.).

### 12.4 Parser Implementation

The parser wraps the lexer and maintains a token buffer for lookahead:

```go
package parser

type Parser struct {
    lex      *lexer.Lexer
    current  lexer.Token
    peeked   []lexer.Token
    fixities map[string]Fixity
    errors   []ParseError
}

type Fixity struct {
    Prec  int
    Assoc Assoc
}

type Assoc int
const (
    AssocLeft Assoc = iota
    AssocRight
    AssocNone
)

func New(lex *lexer.Lexer) *Parser { ... }

func (p *Parser) ParseProgram() *Program { ... }
func (p *Parser) parseDecl() Decl { ... }
func (p *Parser) parseExpr(minBP int) Expr { ... }
func (p *Parser) parseType() Type { ... }
```

The parser exposes `ParseProgram()` as the entry point. Internally, it uses `parseExpr(minBP)` for the Pratt expression parser and separate methods for declarations, types, patterns, and statements.

### 12.5 Fixity Table Management

The fixity table is populated in two phases:

1. **Pre-scan.** Before parsing expressions, the parser scans the declaration list for fixity declarations and populates the table. This requires either a two-pass approach or an interleaving of declaration parsing and fixity collection.

2. **Runtime lookup.** During expression parsing, `lookupFixity(op)` returns the precedence and associativity of an operator.

The simplest implementation: parse declarations in order. Fixity declarations are processed immediately. Expression bodies are parsed lazily or in a second pass. This avoids forward-reference issues (a fixity declaration must appear before the first use of the operator it declares).

Alternatively: require all fixity declarations to appear at the top of the file, before any value definitions. This is the strictest and simplest approach.

### 12.6 Position Tracking

Every AST node carries a `Span`:

```go
type Span struct {
    Start Position
    End   Position
}
```

The `Start` is the position of the first token of the construct; `End` is the position of the last token. This is sufficient for error messages and future IDE support.

---

## 13. AST Representation

### 13.1 Expression AST

```go
type Expr interface {
    exprNode()
    Span() Span
}

type Var struct {
    Name string
    S    Span
}

type IntLit struct {
    Value int64
    S     Span
}

type FloatLit struct {
    Value float64
    S     Span
}

type StringLit struct {
    Value string
    S     Span
}

type App struct {
    Func Expr
    Arg  Expr
    S    Span
}

type TypeApp struct {
    Func   Expr
    TyArg  TypeExpr
    S      Span
}

type Lambda struct {
    Params []LamParam
    Body   Expr
    S      Span
}

type LamParam struct {
    Name string
    Ann  *TypeExpr   // nil if unannotated
}

type BinOp struct {
    Left  Expr
    Op    string
    Right Expr
    S     Span
}

type Negate struct {
    Operand Expr
    S       Span
}

type IfExpr struct {
    Cond Expr
    Then Expr
    Else Expr
    S    Span
}

type CaseExpr struct {
    Scrutinee Expr
    Alts      []Alt
    S         Span
}

type Alt struct {
    Pattern Pattern
    Body    Expr
    S       Span
}

type DoExpr struct {
    Stmts []Stmt
    S     Span
}

type Annotation struct {
    Expr Expr
    Type TypeExpr
    S    Span
}

type OpSection struct {
    Op string
    S  Span
}

type LeftSection struct {
    Left Expr
    Op   string
    S    Span
}

type RightSection struct {
    Op    string
    Right Expr
    S     Span
}

type Paren struct {
    Inner Expr
    S     Span
}
```

### 13.2 Statement AST

```go
type Stmt interface {
    stmtNode()
    Span() Span
}

type PureBind struct {
    Name string
    Expr Expr
    S    Span
}

type CompBind struct {
    Name string
    Expr Expr
    S    Span
}

type ExprStmt struct {
    Expr Expr
    S    Span
}
```

### 13.3 Type AST

```go
type TypeExpr interface {
    typeExprNode()
    Span() Span
}

type TyVar struct {
    Name string
    S    Span
}

type TyCon struct {
    Name string
    S    Span
}

type TyApp struct {
    Func TypeExpr
    Arg  TypeExpr
    S    Span
}

type TyArrow struct {
    From TypeExpr
    To   TypeExpr
    S    Span
}

type TyForall struct {
    Params []TypeParam
    Body   TypeExpr
    S      Span
}

type TypeParam struct {
    Name string
    Kind *KindExpr   // nil if implicit
}

type TyRow struct {
    Fields []RowField
    Tail   *string    // nil for closed rows; variable name for open rows
    S      Span
}

type RowField struct {
    Label string
    Type  TypeExpr
}

type TyAnnotated struct {
    Expr TypeExpr
    Kind KindExpr
    S    Span
}
```

### 13.4 Declaration AST

```go
type Decl interface {
    declNode()
    Span() Span
}

type TypeAnnotationDecl struct {
    Name string
    Type TypeExpr
    S    Span
}

type ValueDefDecl struct {
    Name string
    Expr Expr
    S    Span
}

type DataDecl struct {
    Name    string
    Params  []TypeParam
    Constrs []ConDecl
    S       Span
}

type ConDecl struct {
    Name   string
    Fields []TypeExpr
}

type PrimitiveDecl struct {
    Name string
    Type TypeExpr
    S    Span
}

type FixityDecl struct {
    Assoc Assoc
    Prec  int
    Op    string
    S     Span
}
```

### 13.5 Pattern AST

```go
type Pattern interface {
    patternNode()
    Span() Span
}

type PVar struct {
    Name string
    S    Span
}

type PWildcard struct {
    S Span
}

type PCon struct {
    Con    string
    Fields []Pattern
    S      Span
}

type PLit struct {
    Value Expr       // IntLit or StringLit
    S     Span
}

type PParen struct {
    Inner Pattern
    S     Span
}

type PTuple struct {
    Elems []Pattern
    S     Span
}
```

### 13.6 Design Rationale

The AST uses Go interfaces with a marker method (`exprNode()`, `stmtNode()`, etc.) to form closed sum types. This is the standard pattern in Go for representing ADTs. Each concrete node implements the interface.

Pattern matching over the AST uses type switches:

```go
switch e := expr.(type) {
case *Var:
    ...
case *App:
    ...
case *Lambda:
    ...
}
```

This is the standard Go idiom for case analysis over interface types.

---

## 14. Error Recovery

### 14.1 Strategy

The parser uses **panic-mode recovery**: when a parse error is encountered, the parser records the error, then advances tokens until it reaches a synchronization point.

Synchronization points for Gomputation:

- `;` (statement boundary in do blocks, declaration boundary)
- `}` (block boundary)
- Keywords that start declarations: `data`, `primitive`, `infixl`, `infixr`, `infix`, and identifiers at the top level followed by `::` or `:=`.
- `EOF`

### 14.2 Error Accumulation

The parser accumulates errors rather than aborting on the first one:

```go
type ParseError struct {
    Pos     Position
    Message string
}

func (p *Parser) error(msg string) {
    p.errors = append(p.errors, ParseError{
        Pos:     p.current.Pos,
        Message: msg,
    })
    p.synchronize()
}
```

This allows the parser to report multiple errors in a single pass.

### 14.3 Common Error Patterns

| Situation | Error Message | Recovery |
|---|---|---|
| Missing `)` | "expected ')' to close parenthesized expression" | Skip to next `)` or `;` |
| Missing `}` in do block | "expected '}' to close do block" | Skip to next `}` |
| Missing `->` after `\x` | "expected '->' in lambda expression" | Treat next expression as body |
| Missing `then` after `if e` | "expected 'then' after if condition" | Treat next expression as then-branch |
| Unknown operator precedence | "operator 'X' has no fixity declaration; assuming infixl 9" | Use default fixity |
| Associativity conflict | "operators 'X' and 'Y' have same precedence but different associativity" | Treat as left-associative |

### 14.4 Error Position Quality

Every error message includes the source position (line and column) of the offending token. For multi-token constructs, the error points to the token where the parser first detected the problem.

---

## 15. Comparison with Haskell, PureScript, Elm, and Koka

### 15.1 Haskell

**Grammar complexity.** Haskell's grammar (Haskell 2010 Report, Chapter 10) is large and has many interactions. The expression grammar alone has around 20 non-terminals.

**Layout rule.** Haskell's offside rule is context-sensitive and interacts with the parser. It applies after `where`, `let`, `do`, and `of`. The rule is well-defined but has edge cases that surprise programmers.

**Operators.** Haskell allows arbitrary user-defined operators with declared fixity. Operators are parsed by a fixity-resolution pass after an initial parse that produces a flat operator spine. GHC's actual implementation interleaves fixity resolution with parsing.

**Type annotations.** Haskell uses `::` for type annotations both in declarations and expressions. In expressions, `::` is an infix operator with very low precedence.

**Relevant differences from Gomputation:**
- Haskell uses `=` for definition; Gomputation uses `:=`.
- Haskell uses `::` for both declaration-level and expression-level annotation; Gomputation does the same.
- Haskell's `do` and `case` use layout; Gomputation (initially) uses braces.
- Haskell has `let-in` and `where`; Gomputation defers `where` and does not currently have `let-in`.

**Lesson:** Haskell's grammar is a validation that the combination of user-defined operators, layout rules, and Pratt-like parsing works. But Haskell's grammar also shows the cost of accumulated special cases. Gomputation should stay smaller.

### 15.2 PureScript

**Grammar philosophy.** PureScript is explicitly simpler than Haskell. It uses Haskell-like syntax but with fewer special cases.

**Layout rule.** PureScript uses indentation-sensitive parsing similar to Haskell's layout rule, but with some simplifications. It applies after `where`, `do`, `of`, `let`, and `ado`.

**Operators.** PureScript uses the same fixity declaration style as Haskell. Operators are resolved by precedence after parsing.

**Type annotations.** PureScript uses `::` in both positions, like Haskell.

**Relevant differences from Gomputation:**
- PureScript uses `=` for definition.
- PureScript has type classes; Gomputation does not.
- PureScript's row types use `( l :: T | r )` with parentheses and `::` for fields; Gomputation uses `{ l : T | r }` with braces and `:` for fields.

**Lesson:** PureScript demonstrates that a Haskell-like surface can be simplified without losing expressiveness. Its row type syntax (using `::` for fields) would be confusing in Gomputation because `::` already means type annotation. Gomputation's choice of `:` for row fields and `::` for annotations avoids this collision.

### 15.3 Elm

**Grammar philosophy.** Elm prioritizes simplicity and learnability over expressiveness.

**Layout rule.** Elm uses indentation-sensitive parsing but with strict rules: expressions must be more indented than their parent. No explicit `{`, `}`, `;`.

**Operators.** Elm removed user-defined operator precedence in Elm 0.19. All operators have fixed precedence. This was a deliberate choice to reduce cognitive load.

**Type annotations.** Elm uses `:` (single colon) for type annotations, not `::`.

**Relevant differences from Gomputation:**
- Elm has no user-defined operators. Gomputation requires them (the spec includes fixity declarations).
- Elm uses `:` for annotation; Gomputation uses `::`.
- Elm has no `forall` or explicit polymorphism; Gomputation requires it.

**Lesson:** Elm shows that removing user-defined operators radically simplifies the grammar. Gomputation cannot follow this path (fixity declarations are committed in the spec), but should be aware that user-defined operators are the largest source of grammar complexity.

### 15.4 Koka

**Grammar philosophy.** Koka uses a mostly traditional syntax (closer to ML or Rust than Haskell) with indentation-sensitive parsing.

**Effect annotations.** Koka annotates function types with effect rows: `fun(x: int) : <exn, io> int`. This is Koka's signature contribution.

**Operators.** Koka has a fixed set of operators with fixed precedence. User-defined operators are not supported in the Haskell sense.

**Type annotations.** Koka uses `:` for type annotations.

**Layout rule.** Koka uses brace elision based on indentation, similar to Haskell but with different rules (the "offside" rule applies to functions and match expressions).

**Relevant differences from Gomputation:**
- Koka's effect rows appear in function types; Gomputation's rows appear in `Computation` types.
- Koka uses `:` for annotation; Gomputation uses `::`.
- Koka's syntax is more ML-like; Gomputation is Haskell-like.

**Lesson:** Koka demonstrates that row-typed effects can coexist with a practical surface syntax. Its annotation placement (on the function type directly) is more concise than Gomputation's (where effects appear in the `Computation` type), but Gomputation's pre/post pattern requires the more explicit form.

### 15.5 Summary Table

| Feature | Haskell | PureScript | Elm | Koka | **Gomputation** |
|---|---|---|---|---|---|
| **Definition** | `=` | `=` | `=` | `=` / `fun` | `:=` |
| **Type annotation** | `::` | `::` | `:` | `:` | `::` |
| **Row field** | n/a | `::` | n/a | n/a | `:` |
| **User operators** | yes | yes | no | no | yes |
| **Layout rule** | yes | yes | yes | yes | phase 2 |
| **Lambda** | `\x -> e` | `\x -> e` | `\x -> e` | `fn(x) e` | `\x -> e` |
| **Do notation** | `do { }` | `do` (layout) | n/a | n/a | `do { }` |
| **Type application** | `@T` (ext) | n/a | n/a | `<T>` | `@T` |
| **Effect types** | `IO a` | `Effect a` | no effects | `<e> a` | `Computation R R T` |

---

## 16. Formal Grammar Summary (Collected)

This section collects the complete grammar in a single reference.

### 16.1 Lexical Grammar

```ebnf
(* Character classes *)
IdentStart   = 'a'..'z' | 'A'..'Z' | '_' ;
IdentCont    = IdentStart | '0'..'9' | '\'' ;
Digit        = '0'..'9' ;
OpChar       = '!' | '#' | '$' | '%' | '&' | '*' | '+' | '.' | '/'
             | '<' | '=' | '>' | '?' | '@' | '^' | '|' | '-' | '~' | ':' ;

(* Tokens *)
LowerIdent   = ('a'..'z' | '_') , { IdentCont } ;
UpperIdent   = 'A'..'Z' , { IdentCont } ;
IntLiteral   = Digit , { Digit } ;
FloatLiteral = Digit , { Digit } , '.' , Digit , { Digit } ;
StringLiteral= '"' , { StringChar } , '"' ;
Operator     = OpChar , { OpChar } ;

(* Special tokens -- recognized before general operator rule *)
SpecialToken = '->' | '::' | ':=' | '<-' | '\' | '@' | '|' | '=' | '..' ;

(* Keywords *)
Keyword      = 'data' | 'case' | 'of' | 'do' | 'if' | 'then' | 'else'
             | 'forall' | 'pure' | 'infixl' | 'infixr' | 'infix'
             | 'primitive' | 'where' ;

(* Comments *)
LineComment  = '--' , { <any char except newline> } , <newline> ;
BlockComment = '{-' , { <any char or nested BlockComment> } , '-}' ;
```

### 16.2 Syntactic Grammar

```ebnf
(* --- Programs and Declarations --- *)

Program      = { Decl [';'] } ;

Decl         = TypeAnnotation
             | ValueDefinition
             | DataDeclaration
             | PrimitiveDeclaration
             | FixityDeclaration ;

TypeAnnotation      = LowerIdent , '::' , Type ;
ValueDefinition     = LowerIdent , ':=' , Expr ;
DataDeclaration     = 'data' , UpperIdent , { TypeParam } , '=' , DataBody ;
PrimitiveDeclaration= 'primitive' , LowerIdent , '::' , Type ;
FixityDeclaration   = ('infixl' | 'infixr' | 'infix') , IntLiteral , Operator ;

DataBody     = ConDecl , { '|' , ConDecl } ;
ConDecl      = UpperIdent , { TypeAtom } ;

(* --- Expressions --- *)

Expr         = '\' , LamParam , { LamParam } , '->' , Expr
             | 'if' , Expr , 'then' , Expr , 'else' , Expr
             | 'case' , Expr , 'of' , '{' , Alt , { ';' , Alt } , '}'
             | 'do' , '{' , Stmt , { ';' , Stmt } , '}'
             | InfixExpr , '::' , Type
             | InfixExpr ;

LamParam     = LowerIdent
             | '(' , LowerIdent , '::' , Type , ')' ;

InfixExpr    = PrefixExpr , { InfixOp , PrefixExpr } ;
                (* resolved by Pratt parser using fixity table *)

InfixOp      = Operator
             | '`' , LowerIdent , '`'
             | '`' , UpperIdent , '`' ;

PrefixExpr   = '-' , AppExpr
             | AppExpr ;

AppExpr      = AppExpr , AtomExpr                   (* function application *)
             | AppExpr , '@' , TypeAtom             (* type application *)
             | AtomExpr ;

AtomExpr     = LowerIdent
             | UpperIdent
             | IntLiteral
             | FloatLiteral
             | StringLiteral
             | '(' , Expr , ')'
             | '(' , Operator , ')'                 (* operator section *)
             | '(' , Expr , Operator , ')'          (* left section *)
             | '(' , Operator , Expr , ')'          (* right section *)
             | 'pure' ;

(* --- Statements (in do blocks) --- *)

Stmt         = LowerIdent , ':=' , Expr             (* pure binding *)
             | LowerIdent , '<-' , Expr             (* computation binding *)
             | Expr ;                                (* expression / tail *)

(* --- Alternatives (in case expressions) --- *)

Alt          = Pattern , '->' , Expr ;

(* --- Patterns --- *)

Pattern      = LowerIdent
             | '_'
             | UpperIdent , { PatternAtom }
             | IntLiteral
             | StringLiteral
             | '(' , Pattern , ')'
             | '(' , Pattern , ',' , Pattern , { ',' , Pattern } , ')' ;

PatternAtom  = LowerIdent
             | '_'
             | UpperIdent
             | IntLiteral
             | StringLiteral
             | '(' , Pattern , ')' ;

(* --- Types --- *)

Type         = 'forall' , TypeParam , { TypeParam } , '.' , Type
             | TypeInfix ;

TypeInfix    = TypeApp , '->' , Type                (* right-associative *)
             | TypeApp ;

TypeApp      = TypeApp , TypeAtom
             | TypeAtom ;

TypeAtom     = UpperIdent
             | LowerIdent
             | '(' , Type , ')'
             | '(' , Type , ',' , Type , { ',' , Type } , ')'
             | RowType
             | 'Computation' ;

RowType      = '{' , '}'
             | '{' , RowField , { ',' , RowField } , ['|' , LowerIdent] , '}' ;

RowField     = LowerIdent , ':' , Type ;

TypeParam    = LowerIdent
             | '(' , LowerIdent , '::' , Kind , ')' ;

(* --- Kinds --- *)

Kind         = KindAtom , '->' , Kind
             | KindAtom ;

KindAtom     = 'Type'
             | 'Row'
             | '(' , Kind , ')' ;
```

### 16.3 Disambiguation Summary

| Ambiguity | Resolution |
|---|---|
| `Name ::` vs `Name :=` | Lookahead 1 token after `LowerIdent` |
| `x := e` in do vs `\x -> e` | `:=` and `<-` checked after identifier in statement context |
| Lambda body extent | Body is full `Expr`; extends to enclosing delimiter |
| `::` in expression vs declaration | Context: expression parser sees `::` as led; declaration parser expects it after `LowerIdent` |
| Operator precedence | Pratt parser with runtime fixity table |
| `-` as negation vs subtraction | Negation in prefix position (nud); subtraction in infix position (led) |
| `\|` in data declaration vs row type | `\|` separates constructors in `DataBody`; separates row tail in `RowType`. Different parsing contexts. |
| `->` in types vs expressions | Types and expressions are separate grammars, invoked in known contexts |

### 16.4 Operator Precedence Table (Definitive)

The following table lists all fixed-precedence entities, from lowest to highest. User-defined operators occupy the "User operators" band.

| Binding Power | Entity | Associativity | Category |
|---|---|---|---|
| 0 | `\x -> e`, `if-then-else`, `case-of`, `do { }` | extends right | Special forms (nud) |
| 1 | `::` (type annotation) | non-associative | Annotation (led) |
| 2--21 | User operators (prec 0--9) | per declaration | Infix operators (led) |
| 24 | Prefix `-` (negation) | prefix | Prefix (nud) |
| 26 | Function application `f x`, type application `f @T` | left | Juxtaposition (led) |

Within the user operator band:

| User Prec | Left BP | Right BP (left) | Right BP (right) | Right BP (non) |
|---|---|---|---|---|
| 0 | 3 | 4 | 3 | 4 |
| 1 | 5 | 6 | 5 | 6 |
| 2 | 7 | 8 | 7 | 8 |
| 3 | 9 | 10 | 9 | 10 |
| 4 | 11 | 12 | 11 | 12 |
| 5 | 13 | 14 | 13 | 14 |
| 6 | 15 | 16 | 15 | 16 |
| 7 | 17 | 18 | 17 | 18 |
| 8 | 19 | 20 | 19 | 20 |
| 9 | 21 | 22 | 21 | 22 |

---

## 17. Key References

### Parsing

1. Vaughan Pratt. "Top Down Operator Precedence." *Proceedings of the 1st Annual ACM SIGACT-SIGPLAN Symposium on Principles of Programming Languages*, 1973, pp. 41--51. The original Pratt parsing paper.

2. Andy Chu. "Pratt Parsing Index and Updates." 2016--2020. Available at: https://www.oilshell.org/blog/2017/03/31.html. Comprehensive survey of Pratt parsing implementations and variations.

3. Bob Nystrom. "Pratt Parsers: Expression Parsing Made Easy." *Journal of Bob Nystrom*, 2011. Available at: https://journal.stuffwithstuff.com/2011/03/19/pratt-parsers-expression-parsing-made-easy/. The most cited tutorial on Pratt parsing.

4. Aleksey Kladov. "Simple but Powerful Pratt Parsing." 2020. Available at: https://matklad.github.io/2020/04/13/simple-but-powerful-pratt-parsing.html. Modern treatment emphasizing the left/right binding power formulation.

5. Theodore Norvell. "Parsing Expressions by Recursive Descent." 1999, updated 2016. Available at: https://www.engr.mun.ca/~theo/Misc/exp_parsing.htm. Detailed comparison of precedence climbing, Pratt parsing, and shunting-yard.

### Language Grammars

6. Simon Marlow (ed.). *Haskell 2010 Language Report*, Chapter 10 (Syntax Reference). Available at: https://www.haskell.org/onlinereport/haskell2010/haskellch10.html. The definitive Haskell grammar.

7. PureScript Language Reference. Available at: https://github.com/purescript/documentation/tree/master/language. PureScript's grammar and syntax description.

8. Evan Czaplicki. "Elm Syntax." Available at: https://elm-lang.org/docs/syntax. Elm's simplified grammar.

9. Daan Leijen. "Koka Language Specification." Available at: https://koka-lang.github.io/koka/doc/spec.html. Koka's grammar including effect type syntax.

### Go Parser Implementation

10. Robert Griesemer, Rob Pike, Ken Thompson. The Go compiler's parser: `go/parser` package. Available at: https://pkg.go.dev/go/parser. Go's own hand-written recursive descent parser, a model of clarity.

11. Thorsten Ball. *Writing an Interpreter in Go*. 2016. A practical guide to building a Pratt parser in Go, covering lexer, parser, and evaluator.

12. Robert Nystrom. *Crafting Interpreters*. 2021. Chapter 17 (Compiling Expressions) covers Pratt parsing in detail. Chapter 6 covers hand-written lexing. Available at: https://craftinginterpreters.com/.

### Theoretical Background

13. Andrew Appel. *Modern Compiler Implementation in ML* (or Java, or C). 1998. Chapter 3 covers parsing, including operator precedence parsing.

14. Keith Clarke. "The top-down parsing of expressions." Technical report, Department of Computer Science, QMW, University of London, 1986. A formalization of Pratt's method.

15. Annika Aasa. "Precedences in Specifications and Implementations of Programming Languages." *Theoretical Computer Science*, 142(1):3--26, 1995. Formal treatment of precedence in grammar design.

---

## Appendix A: Complete Worked Example

### A.1 Source Program

```text
infixl 6 +
infixl 7 *
infixr 0 $

id :: forall a. a -> a
id := \x -> x

double :: Int -> Int
double := \x -> x + x

main :: Computation {} {} String
main := do {
  x := double 21;
  y <- compute $ id @Int x;
  result := case y of {
    0 -> "zero";
    _ -> "nonzero"
  };
  pure result
}
```

### A.2 Token Stream (abbreviated)

```text
INFIXL  INT(6)  OP(+)
INFIXL  INT(7)  OP(*)
INFIXR  INT(0)  OP($)
LOWER(id)  DCOLON  FORALL  LOWER(a)  DOT  LOWER(a)  ARROW  LOWER(a)
LOWER(id)  COLEQ  BSLASH  LOWER(x)  ARROW  LOWER(x)
LOWER(double)  DCOLON  UPPER(Int)  ARROW  UPPER(Int)
LOWER(double)  COLEQ  BSLASH  LOWER(x)  ARROW  LOWER(x)  OP(+)  LOWER(x)
LOWER(main)  DCOLON  UPPER(Computation)  LBRACE  RBRACE  LBRACE  RBRACE  UPPER(String)
LOWER(main)  COLEQ  DO  LBRACE
  LOWER(x)  COLEQ  LOWER(double)  INT(21)  SEMI
  LOWER(y)  LARROW  LOWER(compute)  OP($)  LOWER(id)  AT  UPPER(Int)  LOWER(x)  SEMI
  LOWER(result)  COLEQ  CASE  LOWER(y)  OF  LBRACE
    INT(0)  ARROW  STRING("zero")  SEMI
    LOWER(_)  ARROW  STRING("nonzero")
  RBRACE  SEMI
  LOWER(pure)  LOWER(result)
RBRACE
```

### A.3 Parse Tree for `compute $ id @Int x`

Fixity table: `$` is `infixr 0`, application is highest.

```text
parseExpr(0):
  nud: LOWER(compute) -> Var("compute")
  led loop:
    peek = OP($), lbp of $ = 3 > minBP 0  -> enter led
    led($): advance OP($)
      parseExpr(rbp=3):  -- right-assoc, so rbp = 3 (same as lbp)
        nud: LOWER(id) -> Var("id")
        led loop:
          peek = AT, APP_BP = 26 > minBP 3 -> enter type app
          TypeApp(Var("id"), TyCon("Int"))
          peek = LOWER(x), APP_BP = 26 > minBP 3 -> enter app
          App(TypeApp(Var("id"), TyCon("Int")), Var("x"))
          peek = SEMI -> break
        return App(TypeApp(Var("id"), TyCon("Int")), Var("x"))
    BinOp(Var("compute"), "$", App(TypeApp(Var("id"), TyCon("Int")), Var("x")))
    peek = SEMI -> break

Result:
  BinOp(
    Var("compute"),
    "$",
    App(
      TypeApp(Var("id"), TyCon("Int")),
      Var("x")
    )
  )
```

This corresponds to `compute $ ((id @Int) x)`, which is the correct parse.

### A.4 Parse Tree for `\x -> x + x`

```text
parseExpr(0):
  nud: BSLASH -> lambda
    params: [LamParam("x")]
    expect ARROW
    body = parseExpr(0):
      nud: LOWER(x) -> Var("x")
      led loop:
        peek = OP(+), lbp of + = 15 > minBP 0 -> enter led
        led(+): advance OP(+)
          parseExpr(rbp=16):   -- left-assoc, rbp > lbp
            nud: LOWER(x) -> Var("x")
            led loop:
              peek = <next decl or EOF>, lbp <= 16 -> break
            return Var("x")
        BinOp(Var("x"), "+", Var("x"))
        peek = <next decl or EOF>, lbp <= 0 -> break
      return BinOp(Var("x"), "+", Var("x"))
    return Lambda(["x"], BinOp(Var("x"), "+", Var("x")))
```

Result: `\x -> (x + x)` -- correct.

---

## Appendix B: Grammar Design Decisions Deferred

The following are explicitly not resolved in this document, reserved for future specification work:

1. **`let-in` expressions.** `let x = e in body`. Likely syntax; deferred until `where` blocks are designed.

2. **`where` clauses.** `expr where { decls }`. Reserved keyword; no grammar yet.

3. **Record syntax.** `{ field = value, ... }`. Potentially conflicts with row types and do blocks. Requires careful design if ever added.

4. **List/collection literals.** `[1, 2, 3]`. Square brackets are reserved as delimiters. Syntax TBD.

5. **Guard syntax in patterns.** `| guard -> body`. Interacts with row type `|`. Deferred.

6. **Constructor operator syntax.** Operators beginning with `:` as infix constructors (e.g., `::`  for cons). Namespace reserved; grammar TBD.

7. **Qualified names.** `Module.name`. Requires a module system, which is a spec non-goal.

8. **Multi-clause function definitions.** `f (Just x) := ...; f Nothing := ...`. Would require merging pattern matching into definition syntax. Deferred.

---

## Appendix C: Invariants the Parser Must Preserve

For the benefit of downstream elaboration and type checking, the parser guarantees the following structural properties of the AST:

1. **Every `DoExpr` has at least one `Stmt`.** An empty do block (`do { }`) is a parse error.

2. **The last `Stmt` in a `DoExpr` is an `ExprStmt`.** If the last statement is a binding (`x := e` or `x <- e`), it is a parse error ("do block must end with an expression").

3. **Every `CaseExpr` has at least one `Alt`.** An empty case (`case x of { }`) is a parse error.

4. **Every `Lambda` has at least one `LamParam`.** A nullary lambda (`\ -> e`) is a parse error.

5. **`BinOp` nodes are always binary.** The Pratt parser never produces a `BinOp` with a missing operand.

6. **Fixity conflicts are reported.** If two operators of the same precedence but different associativity appear adjacent, the parser emits an error rather than silently choosing one.

7. **Source positions are monotonically increasing.** Every AST node's `Span.Start` is at or after the `Span.End` of the preceding node in a left-to-right traversal.
