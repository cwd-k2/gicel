## Language Overview

### Keywords (15)

| Keyword      | Purpose                                                                                               |
| ------------ | ----------------------------------------------------------------------------------------------------- |
| `case`       | Pattern matching                                                                                      |
| `do`         | Monadic do-block                                                                                      |
| `form`       | Algebraic form type / type class declaration                                                          |
| `lazy`       | Lazy co-data declaration (constructor args implicitly wrapped in Thunk)                               |
| `type`       | Type alias / type family declaration                                                                  |
| `impl`       | Type class instance declaration                                                                       |
| `infixl`     | Left-associative operator fixity                                                                      |
| `infixr`     | Right-associative operator fixity                                                                     |
| `infixn`     | Non-associative operator fixity                                                                       |
| `import`     | Module import                                                                                         |
| `if`         | Conditional expression (if-then-else)                                                                 |
| `then`       | Conditional expression (if-then-else)                                                                 |
| `else`       | Conditional expression (if-then-else)                                                                 |
| `as`         | Qualified import alias (contextual -- only special after `import`, usable as variable name elsewhere) |
| `assumption` | Host-provided primitive marker                                                                        |

### Built-in Identifiers

| Identifier | Role                                            | Kind                                      |
| ---------- | ----------------------------------------------- | ----------------------------------------- |
| `pure`     | Lift a value into a Computation (the F of CBPV) | First-class value                         |
| `bind`     | Monadic sequencing                              | First-class value                         |
| `thunk`    | Suspend a Computation into a value (U of CBPV)  | Syntactic special form (no runtime value) |
| `force`    | Eliminate a thunk, resuming the computation     | Syntactic special form (no runtime value) |
| `rec`      | Recursive combinator (gated, must be enabled)   | First-class value                         |
| `fix`      | Value-level fixpoint (gated, must be enabled)   | First-class value                         |

`pure` and `bind` are always available without any import.

`thunk` and `force` are kept as syntactic forms rather than
first-class functions because CBPV cannot express `thunk` as a
CBV-callable function (the argument would be evaluated before the
suspension, defeating the purpose) and because the type-directed
CBPV auto-coercion (see [features.effects](features.effects))
already inserts them wherever the type context is unambiguous. A
bare `thunk` or `force` reference is a compile-time error; write
them applied (`thunk e`, `force e`) or let the auto-coercion handle
the boundary.

#### Recursive Combinators (`--recursion`)

Both `rec` and `fix` require the `--recursion` flag (CLI) or `eng.EnableRecursion()` (Go API). Without it, their use is a compile error.

**`fix`** — value-level fixpoint. Type: `\a. (a -> a) -> a`. Builds a recursive value by passing "self" as an argument:

```
-- Fibonacci via fix
fib := fix $ \self n. if n <= 1 then n else self (n - 1) + self (n - 2)
```

**`rec`** — computation-level fixpoint. Type: `\(r: Row) a g. (Computation r r a @g -> Computation r r a @g) -> Computation r r a @g`. The pre and post rows must be equal (the effect signature is unchanged by the recursive call):

```
-- Stateful loop via rec (run with --recursion flag)
main := do {
  put 0;
  rec (\self. do { x <- get; if x >= 10 then pure x else do { put (x + 1); self } })
}
```

Both `fix` and `rec` require the `--recursion` flag. Use `fix` for pure recursive functions. Use `rec` for recursive effectful computations within `do` blocks where the effect row is preserved across iterations.

### Entry Point

The `main` binding (or the name set via `--entry`) is the program's
entry point. It may be a plain value (`main := 42`), a `Computation`
(`main := do { ... }`), or even a `Thunk` of a Computation
(`main := someSuspended`) — the checker auto-forces a Thunk-typed
entry point so the runtime can drive it as the program.

Other top-level bindings are stored as values. A `Computation`-typed
RHS at a non-entry binding is silently auto-thunked by the checker
(see [features.effects](features.effects) for the full CBPV coercion
table), so helper definitions like `basic := do { ... }` end up with
type `Suspended`, and `<- basic` in main auto-forces them.

```
-- OK: plain value
main := 42

-- OK: computation (runs at program start)
main := do { putLine "hello" }

-- OK: the helper is auto-thunked, main auto-forces at <-
basic := do { putLine "hello"; pure 42 }
main := do { v <- basic; putLine ("got " <> show v) }
```

### Shebang

Source files may start with a `#!` shebang line, which the compiler ignores:

```
#!/usr/bin/env gicel run
import Prelude
main := 42
```

### Header Directives

Leading `-- gicel:` comments declare module dependencies and compiler options. See [Module System](features.modules) for details.

```
-- gicel: --module Lib=./lib.gicel
-- gicel: --recursion
import Prelude
import Lib
main := ...
```

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
| `->`  | Function type arrow (universe-polymorphic)                                      |
| `-\|` | Type-level application (right-associative, desugars to `TyApp`)                 |
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

- **Integer:** Unsigned decimal digits `[0-9]+`. Underscore separators allowed: `100_000`. Available with `import Prelude`. Negative values: `-5` desugars to `negate 5` at parse time.
- **Double:** Decimal point or exponent: `3.14`, `1e10`, `1.05e+10`. Available with `import Prelude`.
- **String:** Double-quoted `"hello\nworld"`. Escape sequences: `\n`, `\t`, `\r`, `\\`, `\"`, `\'`, `\0`.
- **Rune:** Single-quoted single character `'a'`, `'\n'`. Same escapes as strings.

### Declaration Boundaries

Top-level declarations are separated by newlines or semicolons. Both are interchangeable at the top level; trailing and repeated semicolons are permitted. A new declaration starts when the next token (preceded by a newline or semicolon at depth 0) is one of: lowercase identifier, uppercase identifier, `form`, `lazy`, `type`, `infixl`, `infixr`, `infixn`, `impl`, `import`, or `(op)`. Inside braces (`do`, `case`, GADT), newlines and semicolons are both accepted as separators between statements/alternatives.

Import declarations must appear before all other declarations.

### Private Names

Value bindings whose name starts with `_` are module-private and excluded from exports. Importing modules cannot access them. Use `_` prefix for internal helpers.

---
