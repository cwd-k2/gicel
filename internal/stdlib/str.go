package stdlib

import (
	"context"
	"fmt"
	"strings"

	"github.com/cwd-k2/gomputation/internal/eval"
)

// Str provides string and rune operations: Eq/Ord/Semigroup/Monoid instances, length.
var Str Pack = func(e Registrar) error {
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

func asString(v eval.Value) (string, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return "", fmt.Errorf("stdlib/str: expected HostVal, got %T", v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		return "", fmt.Errorf("stdlib/str: expected string, got %T", hv.Inner)
	}
	return s, nil
}

func asRune(v eval.Value) (rune, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return 0, fmt.Errorf("stdlib/str: expected HostVal, got %T", v)
	}
	r, ok := hv.Inner.(rune)
	if !ok {
		return 0, fmt.Errorf("stdlib/str: expected rune, got %T", hv.Inner)
	}
	return r, nil
}

func eqStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	a, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	if a == b {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func cmpStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	a, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
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
	a, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: a + b}, ce, nil
}

func emptyStrImpl(_ context.Context, ce eval.CapEnv, _ []eval.Value) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: ""}, ce, nil
}

func lengthStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(len([]rune(s)))}, ce, nil
}

func eqRuneImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	a, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asRune(args[1])
	if err != nil {
		return nil, ce, err
	}
	if a == b {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func cmpRuneImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	a, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asRune(args[1])
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
