package stdlib

import (
	"context"
	"fmt"
	"strings"

	"github.com/cwd-k2/gomputation/internal/eval"
)

// Str provides string and rune operations: Eq/Ord/Semigroup/Monoid instances, length.
var Str Pack = func(e Host) error {
	e.RegisterPrim("_eqStr", eqStrImpl)
	e.RegisterPrim("_cmpStr", cmpStrImpl)
	e.RegisterPrim("_appendStr", appendStrImpl)
	e.RegisterPrim("_emptyStr", emptyStrImpl)
	e.RegisterPrim("_lengthStr", lengthStrImpl)
	e.RegisterPrim("_eqRune", eqRuneImpl)
	e.RegisterPrim("_cmpRune", cmpRuneImpl)
	return e.RegisterModule("Std.Str", strSource)
}

const strSource = `
import Prelude

_eqStr :: String -> String -> Bool
_eqStr := assumption

_cmpStr :: String -> String -> Ordering
_cmpStr := assumption

_appendStr :: String -> String -> String
_appendStr := assumption

_emptyStr :: String
_emptyStr := assumption

_lengthStr :: String -> Int
_lengthStr := assumption

_eqRune :: Rune -> Rune -> Bool
_eqRune := assumption

_cmpRune :: Rune -> Rune -> Ordering
_cmpRune := assumption

instance Eq String { eq := _eqStr }
instance Ord String { compare := _cmpStr }
instance Semigroup String { append := _appendStr }
instance Monoid String { empty := _emptyStr }
instance Eq Rune { eq := _eqRune }
instance Ord Rune { compare := _cmpRune }

length :: String -> Int
length := _lengthStr
`

func mustString(v eval.Value) string {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		panic(fmt.Sprintf("stdlib/str: expected HostVal, got %T", v))
	}
	s, ok := hv.Inner.(string)
	if !ok {
		panic(fmt.Sprintf("stdlib/str: expected string, got %T", hv.Inner))
	}
	return s
}

func mustRune(v eval.Value) rune {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		panic(fmt.Sprintf("stdlib/str: expected HostVal, got %T", v))
	}
	r, ok := hv.Inner.(rune)
	if !ok {
		panic(fmt.Sprintf("stdlib/str: expected rune, got %T", hv.Inner))
	}
	return r
}

func eqStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	if mustString(args[0]) == mustString(args[1]) {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func cmpStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	a, b := mustString(args[0]), mustString(args[1])
	switch strings.Compare(a, b) {
	case -1:
		return &eval.ConVal{Con: "LT"}, ce, nil
	case 1:
		return &eval.ConVal{Con: "GT"}, ce, nil
	default:
		return &eval.ConVal{Con: "EQ"}, ce, nil
	}
}

func appendStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: mustString(args[0]) + mustString(args[1])}, ce, nil
}

func emptyStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: ""}, ce, nil
}

func lengthStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: int64(len([]rune(mustString(args[0]))))}, ce, nil
}

func eqRuneImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	if mustRune(args[0]) == mustRune(args[1]) {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func cmpRuneImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	a, b := mustRune(args[0]), mustRune(args[1])
	switch {
	case a < b:
		return &eval.ConVal{Con: "LT"}, ce, nil
	case a > b:
		return &eval.ConVal{Con: "GT"}, ce, nil
	default:
		return &eval.ConVal{Con: "EQ"}, ce, nil
	}
}
