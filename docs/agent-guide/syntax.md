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

### Punctuation and Delimiters

| Token | Meaning                                                                         |
| ----- | ------------------------------------------------------------------------------- |
| `->`  | Function type arrow                                                             |
| `<-`  | Monadic bind in do-block                                                        |
| `=>`  | Constraint qualifier / case alternative / grade annotation / evidence injection |
| `::`  | Type annotation                                                                 |
| `:=`  | Value definition                                                                |
| `:`   | Kind annotation separator                                                       |
| `.`   | Lambda / quantifier body separator (also compose operator)                      |
| `\`   | Lambda / universal quantification                                               |
| `_`   | Wildcard pattern                                                                |
| `~`   | Type equality constraint                                                        |
| `@`   | Explicit type application                                                       |
| `\|`  | Constructor alternative / row tail                                              |
| `;`   | Declaration / statement separator                                               |

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

Top-level declarations are separated by newlines or semicolons. Both are interchangeable at the top level; trailing and repeated semicolons are permitted. A new declaration starts when the next token (preceded by a newline or semicolon at depth 0) is one of: lowercase identifier, uppercase identifier, `form`, `type`, `infixl`, `infixr`, `infixn`, `impl`, `import`, or `(op)`. Inside braces (`do`, `case`, GADT), semicolons are required between statements/alternatives.

Import declarations must appear before all other declarations.

### Private Names

Value bindings whose name starts with `_` are module-private and excluded from exports. Importing modules cannot access them. Use `_` prefix for internal helpers.

---
