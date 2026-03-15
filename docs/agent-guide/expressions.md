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
Con                            -- nullary constructor
Con x y                        -- constructor with arguments
(Con x y)                      -- parenthesized pattern
```

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

### Special Forms

```
pure expr                      -- lift a value into Computation
bind comp (\x -> body)         -- explicit monadic bind
thunk computation              -- suspend a computation into a value
force thunked_value            -- resume a suspended computation
```

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
