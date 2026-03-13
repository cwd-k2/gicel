package stdlib

// PreludeSource is the default prelude: standard data types, type classes, and instances.
// Auto-loaded unless NoPrelude is set. Uses the same RegisterModule mechanism as stdlib packs.
const PreludeSource = `
data Bool = True | False
data Unit = Unit
data Result e a = Ok a | Err e
data Pair a b = Pair a b
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
data Ordering = LT | EQ | GT
type Effect r a = Computation r r a

class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Ordering }
class Functor f { fmap :: forall a b. (a -> b) -> f a -> f b }
class Foldable t { foldr :: forall a b. (a -> b -> b) -> b -> t a -> b }

class Semigroup a { append :: a -> a -> a }
class Semigroup a => Monoid a { empty :: a }
class Functor f => Applicative f {
  wrap :: forall a. a -> f a;
  ap   :: forall a b. f (a -> b) -> f a -> f b
}
class Functor t => Foldable t => Traversable t {
  traverse :: forall f a b. Applicative f => (a -> f b) -> t a -> f (t b)
}

class IxMonad (m : Row -> Row -> Type -> Type) {
  ixpure :: forall a (r : Row). a -> m r r a;
  ixbind :: forall a b (r1 : Row) (r2 : Row) (r3 : Row).
              m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
}

type Lift (m : Type -> Type) (r1 : Row) (r2 : Row) a = m a

instance Eq Bool { eq := \x y -> case x of {
  True -> case y of { True -> True; False -> False };
  False -> case y of { True -> False; False -> True }
} }

instance Eq Unit { eq := \_ _ -> True }

instance Eq Ordering { eq := \x y -> case x of {
  LT -> case y of { LT -> True; EQ -> False; GT -> False };
  EQ -> case y of { LT -> False; EQ -> True; GT -> False };
  GT -> case y of { LT -> False; EQ -> False; GT -> True }
} }

instance Eq a => Eq (Maybe a) { eq := \x y -> case x of {
  Nothing -> case y of { Nothing -> True; Just _ -> False };
  Just a -> case y of { Nothing -> False; Just b -> eq a b }
} }

instance Eq a => Eq b => Eq (Pair a b) { eq := \x y -> case x of {
  Pair a1 b1 -> case y of {
    Pair a2 b2 -> case eq a1 a2 of { True -> eq b1 b2; False -> False }
  }
} }

instance Functor Maybe { fmap := \f ma -> case ma of {
  Nothing -> Nothing;
  Just a -> Just (f a)
} }

instance Foldable Maybe { foldr := \f z ma -> case ma of {
  Nothing -> z;
  Just a -> f a z
} }

instance Semigroup Unit { append := \_ _ -> Unit }
instance Semigroup Ordering { append := \x y -> case x of { EQ -> y; _ -> x } }
instance Monoid Unit { empty := Unit }
instance Monoid Ordering { empty := EQ }

instance Ord Bool { compare := \x y -> case x of {
  False -> case y of { False -> EQ; True -> LT };
  True  -> case y of { False -> GT; True -> EQ }
} }
instance Ord Unit { compare := \_ _ -> EQ }
instance Ord Ordering { compare := \x y -> case x of {
  LT -> case y of { LT -> EQ; EQ -> LT; GT -> LT };
  EQ -> case y of { LT -> GT; EQ -> EQ; GT -> LT };
  GT -> case y of { LT -> GT; EQ -> GT; GT -> EQ }
} }
instance Ord a => Ord (Maybe a) { compare := \x y -> case x of {
  Nothing -> case y of { Nothing -> EQ; Just _ -> LT };
  Just a  -> case y of { Nothing -> GT; Just b -> compare a b }
} }
instance Ord a => Ord b => Ord (Pair a b) { compare := \x y -> case x of {
  Pair a1 b1 -> case y of {
    Pair a2 b2 -> append (compare a1 a2) (compare b1 b2)
  }
} }

instance Applicative Maybe {
  wrap := \x -> Just x;
  ap := \mf mx -> case mf of {
    Nothing -> Nothing;
    Just f  -> case mx of { Nothing -> Nothing; Just x -> Just (f x) }
  }
}

instance Functor (Pair a) { fmap := \f p -> case p of { Pair a b -> Pair a (f b) } }

instance Foldable (Pair a) { foldr := \f z p -> case p of { Pair _ b -> f b z } }

instance Traversable Maybe {
  traverse := \f x -> case x of {
    Nothing -> wrap Nothing;
    Just a  -> fmap (\b -> Just b) (f a)
  }
}

instance Traversable (Pair a) {
  traverse := \f p -> case p of { Pair a b -> fmap (\c -> Pair a c) (f b) }
}

instance Eq a => Eq (List a) { eq := \xs ys -> case xs of {
  Nil -> case ys of { Nil -> True; Cons _ _ -> False };
  Cons x rest -> case ys of {
    Nil -> False;
    Cons y rest2 -> case eq x y of { True -> eq rest rest2; False -> False }
  }
} }

instance Functor List { fmap := \f xs -> case xs of {
  Nil -> Nil;
  Cons x rest -> Cons (f x) (fmap f rest)
} }

instance Foldable List { foldr := \f z xs -> case xs of {
  Nil -> z;
  Cons x rest -> f x (foldr f z rest)
} }

instance Semigroup (List a) { append := \xs ys -> case xs of {
  Nil -> ys;
  Cons x rest -> Cons x (append rest ys)
} }

instance Monoid (List a) { empty := Nil }
`
