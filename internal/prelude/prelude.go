// Package prelude provides the standard type definitions for Gomputation.
package prelude

// Source is the Gomputation prelude source code.
// It defines the standard algebraic data types used by the language.
const Source = `
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
`
