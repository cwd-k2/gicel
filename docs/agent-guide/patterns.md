## 9. Common Patterns

### Pattern Matching

```
-- Destructure Maybe
describe :: Maybe Bool -> String
describe := \m -> case m {
  Nothing -> "empty";
  Just b  -> case b { True -> "yes"; False -> "no" }
}

-- Nested patterns are not supported directly; nest case expressions.

-- Wildcard for catch-all
isZeroOrd :: Ordering -> Bool
isZeroOrd := \o -> case o { EQ -> True; _ -> False }
```

### List Processing

```
import Std.Num

-- Sum a list of Ints (uses Prelude foldr)
sum :: List Int -> Int
sum := foldr (\x -> \acc -> x + acc) 0

-- Build a list literal
myList :: List Int
myList := Cons 1 (Cons 2 (Cons 3 Nil))

-- Map and filter
evens :: List Int -> List Int
evens := filter (\x -> x `mod` 2 == 0)
```

Note: `mod` here is the function from Std.Num used with backtick syntax.

### Stateful Computation

```
import Std.Num
import Std.State

-- Increment a counter three times and return the final value
counter :: Computation { state : Int } { state : Int } Int
counter := do {
  modify (\n -> n + 1);
  modify (\n -> n + 1);
  modify (\n -> n + 1);
  get
}
```

### Error Handling

```
import Std.Num
import Std.Str
import Std.Fail

-- Parse an Int or fail
parseOrFail :: String -> Computation { fail : () | r } { fail : () | r } Int
parseOrFail := \s -> fromMaybe (readInt s)

-- Typed error
safeDivide :: Int -> Int -> Computation { fail : String | r } { fail : String | r } Int
safeDivide := \x -> \y -> case y == 0 {
  True  -> failWith "division by zero";
  False -> pure (div x y)
}
```

### Function Composition

```
-- The . operator (infixr 9) composes functions right-to-left
-- (f . g) x  =  f (g x)

import Std.Num

doubleNegate :: Int -> Int
doubleNegate := negate . negate

-- Pointfree pipeline
transform :: List Int -> List Int
transform := filter (\x -> x > 0) . map (\x -> x * 2)
```

### Combining Effects

```
import Std.Num
import Std.State
import Std.Fail

-- A computation that uses both state and fail
process :: Computation { state : Int, fail : () } { state : Int, fail : () } Int
process := do {
  n <- get;
  case n > 0 {
    True -> do { put (n - 1); pure n };
    False -> fail
  }
}
```

### Thunk and Force

```
-- Suspend a computation for later
suspended :: Thunk {} {} Bool
suspended := thunk (pure True)

-- Resume it
resumed :: Computation {} {} Bool
resumed := force suspended
```

---

## 10. Pitfalls

### No multi-parameter lambdas

Wrong: `\x y -> x + y`
Correct: `\x -> \y -> x + y`

### Int literals require Std.Num

Without `import Std.Num`, writing `42` is a parse error. The Prelude alone has no numeric literals.

### No negative literals

Wrong: `-5`
Correct: `negate 5` (requires Std.Num)

### Type annotation is a separate declaration

Wrong:

```
f := \x -> x :: forall a. a -> a
```

Correct:

```
f :: forall a. a -> a
f := \x -> x
```

The `:: Type` inside an expression is a type ascription (annotation on the expression), not a declaration-level signature. Declaration-level signatures must be on their own line before the definition.

### case uses braces, not "of"

Wrong: `case x of { True -> 1 }`
Correct: `case x { True -> True }`

### No string interpolation

Wrong: `"count is ${n}"`
Correct: `append "count is " (showInt n)` (requires Std.Str)

### Non-associative operators cannot chain

Wrong: `a == b == c`
Correct: `(a == b) && (b == c)`

### No general recursion without rec/fix

By default, `rec` and `fix` are gated (disabled). Without them, you cannot define recursive functions at the value level. The Prelude's `foldr` and Std.List's `foldl` provide recursion for list processing. The host must call `eng.EnableRecursion()` (or CLI `--recursion`) to unlock `rec`/`fix`.

Self-recursive functions use `fix` explicitly:

```
-- Wrong: countdown references itself but is not in scope during checking
countdown := \n -> case n == 0 { True -> 0; False -> countdown (n - 1) }

-- Correct: fix provides self as an explicit parameter
countdown := fix (\self -> \n -> case n == 0 { True -> 0; False -> self (n - 1) })
```

### The dot is overloaded

`.` is both the `forall` body separator and the compose operator (`infixr 9`). Context disambiguates: after `forall a`, the `.` separates quantifier from body. Everywhere else, it is function composition.

### import must come first

All `import` declarations must appear before any other declarations. This is enforced by the parser.

### Operator definitions require parentheses

To define an operator, wrap it in parentheses:

```
infixl 6 +
(+) :: forall a. Num a => a -> a -> a
(+) := add
```

### String length is `strlen`, not `length`

Std.Str uses `strlen` (not `length`) for string length to avoid collision with `Std.List.length`. Use `toRunes` to convert a string to `List Rune` for character-level processing.

### Prelude names shadow freely

The Prelude provides several names that may overlap with stdlib exports. Importing multiple modules that export the same name can cause ambiguity.

### CapEnv capabilities must be provided by the host

If your program uses `get`/`put` (Std.State), the Go host must supply the initial `"state"` capability. If it uses `print` (Std.IO), the host must supply the `"io"` capability. Forgetting this causes a runtime error, not a compile error.

### Computation {} {} a is pure

A `Computation` with empty row indices `{}` requires no capabilities. It is essentially pure but still lives in the Computation type. You can always `pure x` to create one.

### Semicolons inside braces are required

`;` and newlines are both valid declaration separators at the **top level**. Trailing and repeated semicolons are harmless. **Inside braces** (`do { }`, `case { }`, GADT declarations), semicolons are **required** separators — newlines alone do not separate statements or alternatives within braces.

```
-- Wrong: the parser reads `Nil False` as function application (Nil applied to False)
case xs {
  Nil -> Nil
  Cons x rest -> Cons (f x) rest
}

-- Correct: semicolons separate branches
case xs { Nil -> Nil; Cons x rest -> Cons (f x) rest }

-- Also correct: semicolons on separate lines
case xs {
  Nil -> Nil;
  Cons x rest -> Cons (f x) rest
}
```

The same rule applies to GADT constructor declarations:

```
-- Correct: semicolons between constructors
data Expr a = {
  LitBool :: Bool -> Expr Bool;
  LitInt  :: Int -> Expr Int;
  Add     :: Expr Int -> Expr Int -> Expr Int
}
```
