package stdlib

// CoreSource contains Computation-essential definitions: IxMonad class,
// Computation instance, kind-lifting alias, effect alias, and the then combinator.
// Always loaded as the first section of the Prelude module.
const CoreSource = `
class IxMonad (m : Row -> Row -> Type -> Type) {
  ixpure :: forall a (r : Row). a -> m r r a;
  ixbind :: forall a b (r1 : Row) (r2 : Row) (r3 : Row).
              m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
}

type Lift (m : Type -> Type) (r1 : Row) (r2 : Row) a = m a
type Effect r a = Computation r r a

instance IxMonad Computation {
  ixpure := _builtinPure;
  ixbind := _builtinBind
}

then :: forall a b (r1 : Row) (r2 : Row) (r3 : Row). Computation r1 r2 a -> Computation r2 r3 b -> Computation r1 r3 b
then := \m1 -> \m2 -> bind m1 (\_ -> m2)
`

// PreludeSource is the default prelude: standard data types, type classes, and instances.
// Auto-loaded after CoreSource unless NoPrelude is set.
const PreludeSource = `
data Bool = True | False
data Result e a = Ok a | Err e
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
data Ordering = LT | EQ | GT

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

instance Eq Bool { eq := \x -> \y -> case x {
  True -> case y { True -> True; False -> False };
  False -> case y { True -> False; False -> True }
} }

instance Eq () { eq := \_ -> \_ -> True }

instance Eq Ordering { eq := \x -> \y -> case x {
  LT -> case y { LT -> True; EQ -> False; GT -> False };
  EQ -> case y { LT -> False; EQ -> True; GT -> False };
  GT -> case y { LT -> False; EQ -> False; GT -> True }
} }

instance Eq a => Eq (Maybe a) { eq := \x -> \y -> case x {
  Nothing -> case y { Nothing -> True; Just _ -> False };
  Just a -> case y { Nothing -> False; Just b -> eq a b }
} }

instance Eq a => Eq b => Eq (a, b) { eq := \x -> \y -> case x {
  (a1, b1) -> case y {
    (a2, b2) -> case eq a1 a2 { True -> eq b1 b2; False -> False }
  }
} }

instance Functor Maybe { fmap := \f -> \ma -> case ma {
  Nothing -> Nothing;
  Just a -> Just (f a)
} }

instance Foldable Maybe { foldr := \f -> \z -> \ma -> case ma {
  Nothing -> z;
  Just a -> f a z
} }

instance Semigroup () { append := \_ -> \_ -> () }
instance Semigroup Ordering { append := \x -> \y -> case x { EQ -> y; _ -> x } }
instance Semigroup a => Semigroup (Maybe a) { append := \x -> \y -> case x {
  Nothing -> y;
  Just a -> case y { Nothing -> Just a; Just b -> Just (append a b) }
} }
instance Monoid () { empty := () }
instance Monoid Ordering { empty := EQ }
instance Semigroup a => Monoid (Maybe a) { empty := Nothing }

instance Ord Bool { compare := \x -> \y -> case x {
  False -> case y { False -> EQ; True -> LT };
  True  -> case y { False -> GT; True -> EQ }
} }
instance Ord () { compare := \_ -> \_ -> EQ }
instance Ord Ordering { compare := \x -> \y -> case x {
  LT -> case y { LT -> EQ; EQ -> LT; GT -> LT };
  EQ -> case y { LT -> GT; EQ -> EQ; GT -> LT };
  GT -> case y { LT -> GT; EQ -> GT; GT -> EQ }
} }
instance Ord a => Ord (Maybe a) { compare := \x -> \y -> case x {
  Nothing -> case y { Nothing -> EQ; Just _ -> LT };
  Just a  -> case y { Nothing -> GT; Just b -> compare a b }
} }
instance Ord a => Ord b => Ord (a, b) { compare := \x -> \y -> case x {
  (a1, b1) -> case y {
    (a2, b2) -> append (compare a1 a2) (compare b1 b2)
  }
} }

instance Applicative Maybe {
  wrap := \x -> Just x;
  ap := \mf -> \mx -> case mf {
    Nothing -> Nothing;
    Just f  -> case mx { Nothing -> Nothing; Just x -> Just (f x) }
  }
}

instance Traversable Maybe {
  traverse := \f -> \x -> case x {
    Nothing -> wrap Nothing;
    Just a  -> fmap (\b -> Just b) (f a)
  }
}

instance Eq a => Eq (List a) { eq := \xs -> \ys -> case xs {
  Nil -> case ys { Nil -> True; Cons _ _ -> False };
  Cons x rest -> case ys {
    Nil -> False;
    Cons y rest2 -> case eq x y { True -> eq rest rest2; False -> False }
  }
} }

instance Functor List { fmap := \f -> \xs -> case xs {
  Nil -> Nil;
  Cons x rest -> Cons (f x) (fmap f rest)
} }

instance Foldable List { foldr := \f -> \z -> \xs -> case xs {
  Nil -> z;
  Cons x rest -> f x (foldr f z rest)
} }

instance Semigroup (List a) { append := \xs -> \ys -> case xs {
  Nil -> ys;
  Cons x rest -> Cons x (append rest ys)
} }

instance Monoid (List a) { empty := Nil }

id :: forall a. a -> a
id := \x -> x

const :: forall a b. a -> b -> a
const := \x -> \_ -> x

flip :: forall a b c. (a -> b -> c) -> b -> a -> c
flip := \f -> \b -> \a -> f a b

infixr 9 .
(.) :: forall b c a. (b -> c) -> (a -> b) -> a -> c
(.) := \f -> \g -> \x -> f (g x)

not :: Bool -> Bool
not := \b -> case b { True -> False; False -> True }

infixr 3 &&
(&&) :: Bool -> Bool -> Bool
(&&) := \x -> \y -> case x { False -> False; True -> y }

infixr 2 ||
(||) :: Bool -> Bool -> Bool
(||) := \x -> \y -> case x { True -> True; False -> y }

maybe :: forall a b. b -> (a -> b) -> Maybe a -> b
maybe := \def -> \f -> \m -> case m { Nothing -> def; Just a -> f a }

result :: forall e a b. (e -> b) -> (a -> b) -> Result e a -> b
result := \onErr -> \onOk -> \r -> case r { Err e -> onErr e; Ok a -> onOk a }

fst :: forall a (r : Row). Record { _1 : a | r } -> a
fst := \p -> p!#_1

snd :: forall a (r : Row). Record { _2 : a | r } -> a
snd := \p -> p!#_2

head :: forall a. List a -> Maybe a
head := \xs -> case xs { Nil -> Nothing; Cons x _ -> Just x }

tail :: forall a. List a -> Maybe (List a)
tail := \xs -> case xs { Nil -> Nothing; Cons _ rest -> Just rest }

null :: forall a. List a -> Bool
null := \xs -> case xs { Nil -> True; Cons _ _ -> False }

map :: forall a b. (a -> b) -> List a -> List b
map := fmap

filter :: forall a. (a -> Bool) -> List a -> List a
filter := \p -> foldr (\x -> \acc -> case p x { True -> Cons x acc; False -> acc }) Nil

singleton :: forall a. a -> List a
singleton := \x -> Cons x Nil

infixn 4 ==
infixn 4 /=
infixn 4 <
infixn 4 >
infixn 4 <=
infixn 4 >=

(==) :: forall a. Eq a => a -> a -> Bool
(==) := eq

(/=) :: forall a. Eq a => a -> a -> Bool
(/=) := \x -> \y -> not (eq x y)

(<) :: forall a. Ord a => a -> a -> Bool
(<) := \x -> \y -> case compare x y { LT -> True; _ -> False }

(>) :: forall a. Ord a => a -> a -> Bool
(>) := \x -> \y -> case compare x y { GT -> True; _ -> False }

(<=) :: forall a. Ord a => a -> a -> Bool
(<=) := \x -> \y -> case compare x y { GT -> False; _ -> True }

(>=) :: forall a. Ord a => a -> a -> Bool
(>=) := \x -> \y -> case compare x y { LT -> False; _ -> True }

min :: forall a. Ord a => a -> a -> a
min := \x -> \y -> case compare x y { GT -> y; _ -> x }

max :: forall a. Ord a => a -> a -> a
max := \x -> \y -> case compare x y { LT -> y; _ -> x }

infixr 6 <>
(<>) :: forall a. Semigroup a => a -> a -> a
(<>) := append

class Monad (m : Type -> Type) {
  mpure :: forall a. a -> m a;
  mbind :: forall a b. m a -> (a -> m b) -> m b
}

instance Monad Maybe {
  mpure := \a -> Just a;
  mbind := \ma -> \f -> case ma { Nothing -> Nothing; Just a -> f a }
}

instance Monad List {
  mpure := \a -> Cons a Nil;
  mbind := \xs -> \f -> foldr (\x -> \acc -> append (f x) acc) Nil xs
}

infixl 1 >>=
(>>=) :: forall (m : Type -> Type) a b. Monad m => m a -> (a -> m b) -> m b
(>>=) := mbind

infixl 1 >>
(>>) :: forall (m : Type -> Type) a b. Monad m => m a -> m b -> m b
(>>) := \m1 -> \m2 -> mbind m1 (\_ -> m2)

instance IxMonad Maybe {
  ixpure := \a -> Just a;
  ixbind := \ma -> \f -> case ma {
    Nothing -> Nothing;
    Just a  -> f a
  }
}

instance IxMonad List {
  ixpure := \a -> Cons a Nil;
  ixbind := \xs -> \f -> foldr (\x -> \acc -> append (f x) acc) Nil xs
}

class Packed c e {
  pack   :: List e -> c;
  unpack :: c -> List e
}

instance Packed (List a) a {
  pack   := \xs -> xs;
  unpack := \xs -> xs
}
`
