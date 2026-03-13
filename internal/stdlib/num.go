package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gomputation/internal/eval"
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
`

func mustInt64(v eval.Value) int64 {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		panic(fmt.Sprintf("stdlib/num: expected HostVal, got %T", v))
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		panic(fmt.Sprintf("stdlib/num: expected int64, got %T", hv.Inner))
	}
	return n
}

func intResult(n int64, ce eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: n}, ce, nil
}

func addIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	return intResult(mustInt64(args[0])+mustInt64(args[1]), ce)
}

func subIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	return intResult(mustInt64(args[0])-mustInt64(args[1]), ce)
}

func mulIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	return intResult(mustInt64(args[0])*mustInt64(args[1]), ce)
}

func divIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	b := mustInt64(args[1])
	if b == 0 {
		return nil, ce, fmt.Errorf("division by zero")
	}
	return intResult(mustInt64(args[0])/b, ce)
}

func modIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	b := mustInt64(args[1])
	if b == 0 {
		return nil, ce, fmt.Errorf("modulo by zero")
	}
	return intResult(mustInt64(args[0])%b, ce)
}

func negIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	return intResult(-mustInt64(args[0]), ce)
}

func eqIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	if mustInt64(args[0]) == mustInt64(args[1]) {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func cmpIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	a, b := mustInt64(args[0]), mustInt64(args[1])
	switch {
	case a < b:
		return &eval.ConVal{Con: "LT"}, ce, nil
	case a > b:
		return &eval.ConVal{Con: "GT"}, ce, nil
	default:
		return &eval.ConVal{Con: "EQ"}, ce, nil
	}
}
