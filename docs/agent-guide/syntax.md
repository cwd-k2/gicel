## 2. Language Overview

### Keywords (10)

| Keyword    | Purpose                           |
| ---------- | --------------------------------- |
| `case`     | Pattern matching                  |
| `do`       | Monadic do-block                  |
| `data`     | Algebraic data type declaration   |
| `type`     | Type alias declaration            |
| `infixl`   | Left-associative operator fixity  |
| `infixr`   | Right-associative operator fixity |
| `infixn`   | Non-associative operator fixity   |
| `class`    | Type class declaration            |
| `instance` | Type class instance declaration   |
| `import`   | Module import                     |

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

| Token | Meaning                                                    |
| ----- | ---------------------------------------------------------- |
| `->`  | Function type arrow / case alternative                     |
| `<-`  | Monadic bind in do-block                                   |
| `=>`  | Constraint qualifier                                       |
| `::`  | Type annotation                                            |
| `:=`  | Value definition                                           |
| `:`   | Kind annotation separator                                  |
| `.`   | Lambda / quantifier body separator (also compose operator) |
| `\`   | Lambda / universal quantification                          |
| `_`   | Wildcard pattern                                           |
| `=`   | Data constructor separator                                 |
| `@`   | Explicit type application                                  |
| `\|`  | Constructor alternative / row tail                         |
| `;`   | Declaration / statement separator                          |

### Comments

```
-- line comment
{- nestable block comment {- inner -} outer -}
```

### Literals

- **Integer:** Unsigned decimal digits `[0-9]+`. Requires `import Std.Num` to use. Negative values via `negate 5`, not `-5`.
- **String:** Double-quoted `"hello\nworld"`. Escape sequences: `\n`, `\t`, `\r`, `\\`, `\"`, `\'`, `\0`.
- **Rune:** Single-quoted single character `'a'`, `'\n'`. Same escapes as strings.

### Declaration Boundaries

Top-level declarations are separated by newlines or semicolons. Both are interchangeable at the top level; trailing and repeated semicolons are permitted. A new declaration starts when the next token (preceded by a newline or semicolon at depth 0) is one of: lowercase identifier, uppercase identifier, `data`, `type`, `infixl`, `infixr`, `infixn`, `class`, `instance`, `import`, or `(op)`. Inside braces (`do`, `case`, GADT), semicolons are required between statements/alternatives.

Import declarations must appear before all other declarations.

---
