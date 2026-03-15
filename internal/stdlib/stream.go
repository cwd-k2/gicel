package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Stream provides lazy list operations: LCons/LNil data type,
// headS, tailS, toList, fromList, takeS, dropS, mapS, foldrS,
// and Functor/Foldable instances.
//
// Recursive operations (toList, fromList, mapS, foldrS) are expressed
// as GICEL recursive definitions, requiring rec/fix. Only takeS/dropS
// remain as Go primitives due to Int arithmetic.
var Stream Pack = func(e Registrar) error {
	e.RegisterPrim("_takeS", takeSImpl)
	e.RegisterPrim("_dropS", dropSImpl)
	return e.RegisterModuleRec("Std.Stream", streamSource)
}

const streamSource = `
import Prelude

data Stream a = LCons a (() -> Stream a) | LNil

headS :: forall a. Stream a -> Maybe a
headS := \s -> case s { LNil -> Nothing; LCons x _ -> Just x }

tailS :: forall a. Stream a -> Maybe (Stream a)
tailS := \s -> case s { LNil -> Nothing; LCons _ t -> Just (t ()) }

toList :: forall a. Stream a -> List a
toList := fix \self -> \s -> case s {
  LNil -> Nil;
  LCons x t -> Cons x (self (t ()))
}

fromList :: forall a. List a -> Stream a
fromList := fix \self -> \xs -> case xs {
  Nil -> LNil;
  Cons x rest -> LCons x (\_ -> self rest)
}

mapS :: forall a b. (a -> b) -> Stream a -> Stream b
mapS := \f -> fix \self -> \s -> case s {
  LNil -> LNil;
  LCons x t -> LCons (f x) (\_ -> self (t ()))
}

foldrS :: forall a b. (a -> b -> b) -> b -> Stream a -> b
foldrS := \f -> \z -> fix \self -> \s -> case s {
  LNil -> z;
  LCons x t -> f x (self (t ()))
}

_takeS :: forall a. Int -> Stream a -> List a
_takeS := assumption

_dropS :: forall a. Int -> Stream a -> Stream a
_dropS := assumption

takeS :: forall a. Int -> Stream a -> List a
takeS := _takeS

dropS :: forall a. Int -> Stream a -> Stream a
dropS := _dropS

instance Functor Stream { fmap := \f -> fix \self -> \s -> case s {
  LNil -> LNil;
  LCons x t -> LCons (f x) (\_ -> self (t ()))
} }

instance Foldable Stream { foldr := \f -> \z -> fix \self -> \s -> case s {
  LNil -> z;
  LCons x t -> f x (self (t ()))
} }
`

// forceTail calls the thunk `() -> Stream a` by applying it to Unit.
func forceTail(t eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	unit := &eval.RecordVal{Fields: map[string]eval.Value{}}
	return apply(t, unit, ce)
}

func takeSImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "stream")
	if err != nil {
		return nil, ce, err
	}
	stream := args[1]
	var items []eval.Value
	for i := int64(0); i < n; i++ {
		con, ok := stream.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("takeS: expected Stream, got %T", stream)
		}
		if con.Con == "LNil" {
			break
		}
		if con.Con != "LCons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("takeS: malformed stream node: %s", con.Con)
		}
		items = append(items, con.Args[0])
		stream, ce, err = forceTail(con.Args[1], ce, apply)
		if err != nil {
			return nil, ce, err
		}
	}
	return buildList(items), ce, nil
}

func dropSImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "stream")
	if err != nil {
		return nil, ce, err
	}
	stream := args[1]
	for i := int64(0); i < n; i++ {
		con, ok := stream.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("dropS: expected Stream, got %T", stream)
		}
		if con.Con == "LNil" {
			return stream, ce, nil
		}
		if con.Con != "LCons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("dropS: malformed stream node: %s", con.Con)
		}
		stream, ce, err = forceTail(con.Args[1], ce, apply)
		if err != nil {
			return nil, ce, err
		}
	}
	return stream, ce, nil
}
