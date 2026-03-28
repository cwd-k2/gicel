## 2. Language Overview

### Keywords (12 + 1 contextual)

| Keyword  | Purpose                                                                                               |
| -------- | ----------------------------------------------------------------------------------------------------- |
| `case`   | Pattern matching                                                                                      |
| `do`     | Monadic do-block                                                                                      |
| `form`   | Algebraic form type / type class declaration                                                          |
| `type`   | Type alias / type family declaration                                                                  |
| `impl`   | Type class instance declaration                                                                       |
| `infixl` | Left-associative operator fixity                                                                      |
| `infixr` | Right-associative operator fixity                                                                     |
| `infixn` | Non-associative operator fixity                                                                       |
| `import` | Module import                                                                                         |
| `if`     | Conditional expression (if-then-else)                                                                 |
| `then`   | Conditional expression (if-then-else)                                                                 |
| `else`   | Conditional expression (if-then-else)                                                                 |
| `as`     | Qualified import alias (contextual -- only special after `import`, usable as variable name elsewhere) |

### Built-in Identifiers

| Identifier   | Role                                            |
| ------------ | ----------------------------------------------- |
| `pure`       | Lift a value into a Computation (the F of CBPV) |
| `bind`       | Monadic sequencing                              |
| `thunk`      | Suspend a Computation into a value (U of CBPV)  |
| `force`      | Eliminate a thunk, resuming the computation     |
| `assumption` | Marker for host-provided primitive bindings     |
| `rec`        | Recursive combinator (gated, must be enabled)   |
| `fix`        | Value-level fixpoint (gated, must be enabled)   |

`pure` and `bind` are always available without any import.

#### Recursive Combinators (`--recursion`)

Both `rec` and `fix` require the `--recursion` flag (CLI) or `eng.EnableRecursion()` (Go API). Without it, their use is a compile error.

**`fix`** — value-level fixpoint. Type: `\a. (a -> a) -> a`. Builds a recursive value by passing "self" as an argument:

```
-- Fibonacci via fix
fib := fix $ \self n. if n <= 1 then n else self (n - 1) + self (n - 2)
```

**`rec`** — computation-level fixpoint. Type: `\(r: Row) a. (Computation r r a -> Computation r r a) -> Computation r r a`. The pre and post rows must be equal (the effect signature is unchanged by the recursive call):

```
-- Stateful loop via rec (run with --recursion flag)
main := do {
  put 0;
  rec (\self. do { x <- get; if x >= 10 then pure x else do { put (x + 1); self } })
}
```

Both `fix` and `rec` require the `--recursion` flag. Use `fix` for pure recursive functions. Use `rec` for recursive effectful computations within `do` blocks where the effect row is preserved across iterations.

### Punctuation and Delimiters

| Token | Meaning                                                                         |
| ----- | ------------------------------------------------------------------------------- |
| `(`   | Grouping / tuple / operator section / import list                               |
| `)`   | Close parenthesis                                                               |
| `{`   | Brace-delimited body (`do`, `case`, GADT)                                       |
| `}`   | Close brace                                                                     |
| `[`   | List literal / list pattern                                                     |
| `]`   | Close bracket                                                                   |
| `,`   | Separator in tuples, import lists, constructor sub-lists                        |
| `;`   | Declaration / statement separator                                               |
| `\|`  | Constructor alternative / row tail                                              |
| `@`   | Explicit type application                                                       |
| `->`  | Function type arrow                                                             |
| `<-`  | Monadic bind in do-block                                                        |
| `=>`  | Constraint qualifier / case alternative / grade annotation / evidence injection |
| `::`  | Type annotation                                                                 |
| `:=`  | Value definition                                                                |
| `:`   | Kind annotation separator                                                       |
| `.`   | Lambda / quantifier body separator (also compose operator)                      |
| `.#`  | Record field projection (`r.#x`)                                                |
| `\`   | Lambda / universal quantification                                               |
| `_`   | Wildcard pattern                                                                |
| `=`   | Reserved (not user-facing; disambiguates from `=>` and operators)               |
| `~`   | Type equality constraint                                                        |

### Comments

```
-- line comment
{- nestable block comment {- inner -} outer -}
```

### Literals

- **Integer:** Unsigned decimal digits `[0-9]+`. Underscore separators allowed: `100_000`. Available with `import Prelude`. Negative values via `negate 5`, not `-5`.
- **Double:** Decimal point or exponent: `3.14`, `1e10`, `1.05e+10`. Available with `import Prelude`.
- **String:** Double-quoted `"hello\nworld"`. Escape sequences: `\n`, `\t`, `\r`, `\\`, `\"`, `\'`, `\0`.
- **Rune:** Single-quoted single character `'a'`, `'\n'`. Same escapes as strings.

### Declaration Boundaries

Top-level declarations are separated by newlines or semicolons. Both are interchangeable at the top level; trailing and repeated semicolons are permitted. A new declaration starts when the next token (preceded by a newline or semicolon at depth 0) is one of: lowercase identifier, uppercase identifier, `form`, `type`, `infixl`, `infixr`, `infixn`, `impl`, `import`, or `(op)`. Inside braces (`do`, `case`, GADT), newlines and semicolons are both accepted as separators between statements/alternatives.

Import declarations must appear before all other declarations.

### Private Names

Value bindings whose name starts with `_` are module-private and excluded from exports. Importing modules cannot access them. Use `_` prefix for internal helpers.

---
