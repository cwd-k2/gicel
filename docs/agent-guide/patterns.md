## 9. Common Patterns

### Pattern Matching

```
-- Destructure Maybe with nested patterns
describe :: Maybe Bool -> String
describe := \m -> case m {
  Just True  -> "yes";
  Just False -> "no";
  Nothing    -> "empty"
}

-- Wildcard for catch-all
isZeroOrd :: Ordering -> Bool
isZeroOrd := \o -> case o { EQ -> True; _ -> False }
```

### Nested Patterns

Constructor patterns can appear inside other constructor patterns. Nullary constructors (like `Nothing`, `True`) need no parentheses; multi-argument constructors must be parenthesized:

```
-- Bare nullary constructor as argument
case xs { Cons Nothing rest -> rest; Cons (Just x) rest -> rest; Nil -> Nil }

-- Deep nesting
case m { Just (Just (Just True)) -> "deep"; _ -> "other" }
```

### Literal Patterns

Integer, string, and rune literals can be used directly in case patterns:

```
import Std.Num

classify :: Int -> String
classify := \n -> case n { 0 -> "zero"; 1 -> "one"; _ -> "other" }

greet :: String -> String
greet := \name -> case name { "Alice" -> "hello"; _ -> "hi" }
```

Literal types are open (cannot enumerate all values), so a wildcard or variable catch-all is always required.

### List Processing

```
import Std.Num

-- Use list literals, not Cons/Nil
myList :: List Int
myList := [1, 2, 3, 4, 5]

-- Operator sections instead of verbose lambdas
sum :: List Int -> Int
sum := foldr (+) 0

evens :: List Int -> List Int
evens := filter (\x -> mod x 2 == 0)

-- Compose a pipeline: filter, transform, fold
pipeline := foldl (+) 0 $ (\x -> x * x) <$> filter (0 <) myList
```

### Stateful Computation

```
import Std.Num
import Std.State

counter :: Computation { state : Int } { state : Int } Int
counter := do { modify (\n -> n + 1); modify (\n -> n + 1); modify (\n -> n + 1); get }
```

### Error Handling

```
import Std.Num
import Std.Str
import Std.Fail

parseOrFail :: String -> Computation { fail : () | r } { fail : () | r } Int
parseOrFail := \s -> fromMaybe (readInt s)

safeDivide :: Int -> Int -> Computation { fail : String | r } { fail : String | r } Int
safeDivide := \x -> \y -> case y == 0 {
  True  -> failWith "division by zero";
  False -> pure (div x y)
}
```

### Function Composition

```
import Std.Num

doubleNegate :: Int -> Int
doubleNegate := negate . negate

transform :: List Int -> List Int
transform := filter (\x -> x > 0) . map (\x -> x * 2)
```

### Combining Effects

```
import Std.Num
import Std.State
import Std.Fail

process :: Computation { state : Int, fail : () } { state : Int, fail : () } Int
process := do {
  n <- get;
  case n > 0 { True -> do { put (n - 1); pure n }; False -> fail }
}
```

### Thunk and Force

```
suspended :: Thunk {} {} Bool
suspended := thunk (pure True)

resumed :: Computation {} {} Bool
resumed := force suspended
```

---

## 10. Pitfalls

### Syntax

- **No multi-parameter lambdas.** `\x y -> ...` is wrong; use `\x -> \y -> ...`
- **Int literals require Std.Num.** Without `import Std.Num`, `42` is a parse error.
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
case xs { Nil -> 0
           Cons x _ -> x }

-- Correct: semicolons separate branches inside braces
case xs { Nil -> 0; Cons x _ -> x }
```

Note: for list traversal, prefer Prelude's `map`/`filter`/`foldr` over manual `case` on `Cons`/`Nil`.

### Recursion

`rec`/`fix` are gated by default. Without them, use Prelude's `foldr` or Std.List's `foldl`. Enable with `eng.EnableRecursion()` or `--recursion`.

```
-- Wrong: self-reference without fix
countdown := \n -> case n == 0 { True -> 0; False -> countdown (n - 1) }

-- Correct: fix provides self as parameter
countdown := fix (\self -> \n -> case n == 0 { True -> 0; False -> self (n - 1) })
```

### The dot is overloaded

`.` is both the `forall` body separator and the compose operator (`infixr 9`). Context disambiguates.

### Naming collisions

- `strlen` (not `length`) for string length, to avoid collision with `Std.List.length`.
- Prelude names may overlap with stdlib exports. Importing multiple modules with the same name causes ambiguity.

### CapEnv must be provided by host

Programs using `get`/`put`, `print`, etc. require the host to supply the corresponding capability (`"state"`, `"io"`). Missing capabilities cause runtime errors, not compile errors.

### `Computation {} {} a` is pure

Empty row indices require no capabilities. Essentially pure, but still in the Computation type.
