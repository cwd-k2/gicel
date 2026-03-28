## 9. Common Patterns

### Pattern Matching

```
-- Destructure Maybe with nested patterns
describe :: Maybe Bool -> String
describe := \m. case m {
  Just True  => "yes";
  Just False => "no";
  Nothing    => "empty"
}

-- Wildcard for catch-all
isZeroOrd :: Ordering -> Bool
isZeroOrd := \o. case o { EQ => True; _ => False }
```

### Nested Patterns

Constructor patterns can appear inside other constructor patterns. Nullary constructors (like `Nothing`, `True`) need no parentheses; multi-argument constructors must be parenthesized:

```
-- Bare nullary constructor as argument
case xs { Cons Nothing rest => rest; Cons (Just x) rest => rest; Nil => Nil }

-- Deep nesting
case m { Just (Just (Just True)) => "deep"; _ => "other" }
```

### Literal Patterns

Integer, string, and rune literals can be used directly in case patterns:

```
import Prelude

classify :: Int -> String
classify := \n. case n { 0 => "zero"; 1 => "one"; _ => "other" }

greet :: String -> String
greet := \name. case name { "Alice" => "hello"; _ => "hi" }
```

Literal types are open (cannot enumerate all values), so a wildcard or variable catch-all is always required.

### List Processing

```
import Prelude

-- Use list literals, not Cons/Nil
myList :: List Int
myList := [1, 2, 3, 4, 5]

-- Operator sections replace verbose lambdas:
--   (* 2)  is  \x. x * 2   (right section)
--   (> 0)  is  \x. x > 0   (right section)
--   (+)    is  \x y. x + y  (operator as function)
sum :: List Int -> Int
sum := foldr (+) 0

evens :: List Int -> List Int
evens := filter (\x. mod x 2 == 0)

-- Compose a pipeline: filter, transform, fold
pipeline := foldl (+) 0 $ (\x. x * x) <$> filter (> 0) myList
```

### Stateful Computation

```
import Prelude
import Effect.State

-- Top-level computations must be suspended with thunk
counter :: Suspended { state: Int } Int
counter := thunk do { put 0; modify (+ 1); modify (+ 1); modify (+ 1); get }

-- Force a thunk inside do to execute it
main := do { r <- force counter; pure r }
```

### Error Handling

```
import Prelude
import Effect.Fail

parseOrFail :: String -> Effect { fail: () | r } Int
parseOrFail := \s. fromMaybe (readInt s)

safeDivide :: Int -> Int -> Effect { fail: String | r } Int
safeDivide := \x y. if y == 0 then failWith "division by zero" else pure (div x y)
```

### Function Composition

```
import Prelude

doubleNegate :: Int -> Int
doubleNegate := negate . negate

transform :: List Int -> List Int
transform := filter (> 0) . map (* 2)
```

### Combining Effects

```
import Prelude
import Effect.State
import Effect.Fail

-- Guard with fail (pure-only), then perform stateful operations
process :: Suspended { state: Int, fail: () } Int
process := thunk do {
  put 5;
  n <- get;
  _ <- if n > 0 then pure n else fail;
  put (n - 1);
  pure n
}

main := do { r <- force process; pure r }
```

### Thunk and Force

```
import Prelude

suspended :: Suspended {} Bool
suspended := thunk (pure True)

-- force returns a Computation — use inside do or as entry point
main := force suspended
```

---

## 10. Pitfalls

### Syntax

- **Lambda uses `.` not `->`.** `\x. e`, not `\x -> e`. Multi-parameter: `\x y. e` (desugars to `\x. \y. e`)
- **Arithmetic operators require Prelude.** Without `import Prelude`, `+`, `-`, `*`, etc. are unbound. Literals like `42` work without Prelude.
- **No negative literals.** Use `negate 5`, not `-5`.
- **Type annotation is a declaration.** `f :: T` then `f := expr`, not `f := expr :: T`.
- **case uses braces, not "of".** `case x { ... }`, not `case x of { ... }`.
- **No string interpolation.** Use `"count: " <> showInt n`.
- **Non-associative operators cannot chain.** `a == b == c` is an error; use `(a == b) && (b == c)`.
- **Operator defs need parens.** `(+) :: ... ; (+) := add`
- **import must come first.** All imports before any other declaration.

### Semicolons inside braces

`;` and newlines are both valid at the **top level**. Inside braces (`do { }`, `case { }`, GADT), semicolons are **required** separators.

```
-- Wrong: parser reads `[] 0` as application
case xs { Nil => 0
           Cons x _ => x }

-- Correct: semicolons separate branches inside braces
case xs { Nil => 0; Cons x _ => x }
```

Note: for list traversal, prefer Prelude's `map`/`filter`/`foldr` over manual `case` on `Cons`/`Nil`.

### Recursion

`rec`/`fix` keywords are gated by default. Enable with `eng.EnableRecursion()` or `--recursion`.

A top-level binding with a preceding type annotation may self-reference without `fix`:

```
-- OK: type annotation enables self-reference
countdown :: Int -> Int
countdown := \n. if n == 0 then 0 else countdown (n - 1)
```

Without a type annotation, use `fix` (requires `--recursion`):

```
-- fix provides self as parameter
countdown := fix (\self n. if n == 0 then 0 else self (n - 1))
```

For simple iteration, Prelude's `foldr`/`foldl` avoid recursion entirely.

### The dot serves triple duty

`.` is the lambda body separator (`\x. e`), the quantifier body separator (`\a. T`), and the compose operator (`infixr 9`). Context disambiguates.

### Naming collisions

- `strlen` (not `length`) for string length, to avoid collision with the list `length` function.
- Names may overlap between stdlib modules. Importing multiple modules with the same name causes ambiguity.

### CapEnv must be provided by host

Programs using `get`/`put`, `print`, etc. require the host to supply the corresponding capability (`"state"`, `"io"`). Missing capabilities cause runtime errors, not compile errors.

### `Effect {} a` is pure

Empty row indices require no capabilities. Essentially pure, but still in the Computation type (via `Effect r a := Computation r r a`).
