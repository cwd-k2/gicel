## Type Classes

Type classes provide ad-hoc polymorphism through dictionary-passing elaboration.

### Class Declaration

Type classes are declared using `data` with a brace body containing method signatures:

```
data ClassName param+ [Constraint =>] {
  method: Type ;
  ...
}
```

Superclass constraints follow the parameters:

```
data Ord a Eq a => {
  compare: a -> a -> Ordering
}
```

### Instance Declaration

```
impl [Constraint =>] ClassName Type+ := {
  method := expr ;
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
                       --> Monad
Functor -+-> Traversable
Foldable-+
IxMonad   (independent)
Packed    (independent)
```

17 type classes total: `IxMonad` (Core) + `Eq`, `Ord`, `Show`, `Num`, `Div`, `Semigroup`, `Monoid`, `Functor`, `Foldable`, `Applicative`, `Alternative`, `Monad`, `Traversable`, `Packed`, `FromList`, `ToList` (Prelude).

### Elaboration

Classes compile to dictionaries (data types). No special runtime representation:

```
data Eq a { eq: a -> a -> Bool }
-- becomes: data Eq$Dict a := Eq$MkDict (a -> a -> Bool)
```

Constrained calls have dictionaries inserted automatically:

```
eq True False
-- becomes: (eq @Bool) eq$Bool True False
```

See the language specification (Chapter 6) for resolution rules and interaction with Computation types.
