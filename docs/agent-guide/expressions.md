## 4. Expressions

### Lambda

Single-parameter only. Curry for multiple parameters:

```
\x -> expr
\x -> \y -> \z -> expr
\(Just x) -> expr              -- constructor pattern
\(a, b) -> expr                -- tuple pattern
\{ x, y } -> expr              -- record pattern
```

Pattern parameters are desugared to `case`: `\(a, b) -> body` becomes `\$p -> case $p { (a, b) -> body }`.

### Application

Left-associative:

```
f x                            -- function application
f x y                          -- = (f x) y
f @Int                         -- explicit type application
```

**Explicit type application** (`@`) passes a type argument to a polymorphic function or constructor. Works with any user-defined polymorphic binding:

```
id @Bool True                  -- instantiate id at Bool
Just @Bool True                -- instantiate Just at Bool
eq @(Maybe Int) x y            -- instantiate eq at Maybe Int
```

### Case Expression

```
case scrutinee {
  Con x y -> expr;
  _       -> expr
}
```

No `of` keyword. Branches separated by `;`. Patterns:

```
x                              -- variable binding
_                              -- wildcard
42                             -- integer literal
"hello"                        -- string literal
'a'                            -- rune literal
Con                            -- nullary constructor
Con x y                        -- constructor with arguments
Con (Just x) y                 -- nested constructor (parens for multi-arg)
Con Nothing y                  -- nested nullary constructor (no parens needed)
(Con x y)                      -- parenthesized pattern
```

Literal patterns require a wildcard catch-all (literal types cannot be exhaustively enumerated).

### Block Expression (let-bindings)

```
{ x := e1; y := e2; body }
```

Desugars to `(\x -> (\y -> body) e2) e1`.

### Do Block

```
do {
  x <- computation;            -- monadic bind
  y := pure_expr;              -- pure let-bind
  _ <- side_effect;            -- discard result
  pure result                  -- final expression (must be a Computation)
}
```

`x <- expr` desugars to `bind expr (\x -> ...)`. The entire do-block produces a `Computation`.

### Infix Operators

```
x + y                          -- operator syntax (if fixity is declared)
x `plus` y                     -- backtick syntax for any binary function
```

Wrap an operator in parentheses to use it as a first-class value:

```
foldr (+) 0 xs                 -- pass operator to higher-order function
(.) f g x                     -- use composition as a regular function
```

**Operator sections** partially apply one argument:

```
(+ 1)                          -- right section: \x -> x + 1
(1 +)                          -- left section:  \x -> 1 + x
```

Right sections bind the right argument, left sections bind the left. Both desugar to single-argument lambdas:

```
map (+ 1) xs                   -- increment each element
filter (0 <) xs                -- keep positives
map (show) xs                  -- (op) alone is the prefix form
```

### Special Forms

```
thunk computation              -- suspend a computation into a value (term former)
force thunked_value            -- resume a suspended computation (term former)
```

### Built-in Functions

```
pure expr                      -- lift a value into Computation
bind comp (\x -> body)         -- explicit monadic bind
```

`pure` and `bind` are first-class functions — they can be partially applied (`map pure xs`) and passed to higher-order functions.

### Expression Precedence

| Level | Form              | Associativity    |
| ----- | ----------------- | ---------------- |
| 0     | `:: Type`         | right            |
| 1-9   | Infix operators   | per `infixl/r/n` |
| 10    | Application `f x` | left             |
| --    | Atoms             | --               |

### Type Expression Precedence

| Level | Form        | Associativity |
| ----- | ----------- | ------------- |
| 0     | `forall`    | --            |
| 1     | `=>`        | right         |
| 2     | `->`        | right         |
| 3     | Application | left          |
| --    | Atoms       | --            |

---
