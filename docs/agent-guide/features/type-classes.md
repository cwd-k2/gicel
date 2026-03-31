## Type Classes

Type classes provide ad-hoc polymorphism through dictionary-passing elaboration.

### Class Declaration

Type classes are declared using `form` with lambda parameters and a brace body containing method signatures:

```
form ClassName := \param+. [Constraint =>] {
  method: Type;
  ...
}
```

Superclass constraints follow the lambda parameters, before the brace:

```
form Ord := \a. Eq a => {
  compare: a -> a -> Ordering
}
```

### Instance Declaration

```
impl [Constraint =>] ClassName Type+ := {
  method := expr;
  ...
}
```

Every method must be defined (no defaults). No overlapping instances.

```
impl Eq Bool := {
  eq := \x y. case (x, y) {
    (True, True)   => True;
    (False, False) => True;
    _              => False
  }
}

impl Eq a => Eq (Maybe a) := {
  eq := \x y. case (x, y) {
    (Just a, Just b) => eq a b;
    (Nothing, Nothing) => True;
    _ => False
  }
}
```

### Constraints in Types

Single constraint uses `C => T`. Multiple constraints use tuple syntax:

```
Eq a => a -> a -> Bool
(Eq a, Ord a) => a -> Bool
```

Quantified constraints are supported:

```
(\a. Eq a => Eq (f a)) => f Bool -> f Bool -> Bool
```

### Class Hierarchy

```
Eq --> Ord
Eq --> Num --> Div
Semigroup --> Monoid
Functor --> Applicative --> Alternative
Functor -+-> Traversable
Foldable-+
FromList --> ToList
GIMonad   (independent, requires GradeAlgebra)
Monad     (independent)
Packed    (independent)
Show      (independent)
```

The class hierarchy includes `GIMonad` (Core) and Prelude classes such as `Eq`, `Ord`, `Show`, `Num`, `Div`, `Semigroup`, `Monoid`, `Functor`, `Foldable`, `Applicative`, `Alternative`, `Monad`, `Traversable`, `Packed`, `FromList`, `ToList`, among others.

### Elaboration

Classes compile to dictionaries (data types). No special runtime representation:

```
form Eq := \a. { eq: a -> a -> Bool }
-- becomes: form Eq$Dict := \a. { Eq$MkDict: (a -> a -> Bool) -> Eq$Dict a }
```

Constrained calls have dictionaries inserted automatically:

```
eq True False
-- becomes: (eq @Bool) eq$Bool True False
```
