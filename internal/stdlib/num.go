package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Num provides integer arithmetic: Num class, Eq/Ord Int instances, and operators.
var Num Pack = func(e Registrar) error {
	e.RegisterPrim("_addInt", addIntImpl)
	e.RegisterPrim("_subInt", subIntImpl)
	e.RegisterPrim("_mulInt", mulIntImpl)
	e.RegisterPrim("_divInt", divIntImpl)
	e.RegisterPrim("_modInt", modIntImpl)
	e.RegisterPrim("_negInt", negIntImpl)
	e.RegisterPrim("_eqInt", eqIntImpl)
	e.RegisterPrim("_cmpInt", cmpIntImpl)
	return e.RegisterModule("Std.Num", numSource)
}

const numSource = `
import Prelude

_addInt :: Int -> Int -> Int
_addInt := assumption

_subInt :: Int -> Int -> Int
_subInt := assumption

_mulInt :: Int -> Int -> Int
_mulInt := assumption

_divInt :: Int -> Int -> Int
_divInt := assumption

_modInt :: Int -> Int -> Int
_modInt := assumption

_negInt :: Int -> Int
_negInt := assumption

_eqInt :: Int -> Int -> Bool
_eqInt := assumption

_cmpInt :: Int -> Int -> Ordering
_cmpInt := assumption

instance Eq Int { eq := _eqInt }
instance Ord Int { compare := _cmpInt }
instance Semigroup Int { append := _addInt }
instance Monoid Int { empty := 0 }

class Eq a => Num a {
  add    :: a -> a -> a;
  sub    :: a -> a -> a;
  mul    :: a -> a -> a;
  negate :: a -> a
}

instance Num Int {
  add := _addInt;
  sub := _subInt;
  mul := _mulInt;
  negate := _negInt
}

div :: Int -> Int -> Int
div := _divInt

mod :: Int -> Int -> Int
mod := _modInt

infixl 6 +
infixl 6 -
infixl 7 *
infixl 7 /

(+) :: forall a. Num a => a -> a -> a
(+) := add

(-) :: forall a. Num a => a -> a -> a
(-) := sub

(*) :: forall a. Num a => a -> a -> a
(*) := mul

(/) :: Int -> Int -> Int
(/) := div

abs :: Int -> Int
abs := \x -> case _cmpInt x 0 { LT -> negate x; _ -> x }

sign :: Int -> Int
sign := \x -> case _cmpInt x 0 { LT -> negate 1; EQ -> 0; GT -> 1 }
`

func asInt64Num(v eval.Value) (int64, error) { return asInt64(v, "num") }

func intResult(n int64, ce eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: n}, ce, nil
}

func addIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return intResult(a+b, ce)
}

func subIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return intResult(a-b, ce)
}

func mulIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return intResult(a*b, ce)
}

func divIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	if b == 0 {
		return nil, ce, fmt.Errorf("division by zero")
	}
	return intResult(a/b, ce)
}

func modIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	if b == 0 {
		return nil, ce, fmt.Errorf("modulo by zero")
	}
	return intResult(a%b, ce)
}

func negIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(-a, ce)
}

func eqIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	if a == b {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func cmpIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	switch {
	case a < b:
		return &eval.ConVal{Con: "LT"}, ce, nil
	case a > b:
		return &eval.ConVal{Con: "GT"}, ce, nil
	default:
		return &eval.ConVal{Con: "EQ"}, ce, nil
	}
}
