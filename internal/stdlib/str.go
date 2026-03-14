package stdlib

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Str provides string and rune operations: Eq/Ord/Semigroup/Monoid instances, length,
// charAt, substring, toUpper, toLower, trim, contains, split, join, showInt, readInt.
var Str Pack = func(e Registrar) error {
	e.RegisterPrim("_eqStr", eqStrImpl)
	e.RegisterPrim("_cmpStr", cmpStrImpl)
	e.RegisterPrim("_appendStr", appendStrImpl)
	e.RegisterPrim("_emptyStr", emptyStrImpl)
	e.RegisterPrim("_lengthStr", lengthStrImpl)
	e.RegisterPrim("_eqRune", eqRuneImpl)
	e.RegisterPrim("_cmpRune", cmpRuneImpl)
	e.RegisterPrim("_charAt", charAtImpl)
	e.RegisterPrim("_substring", substringImpl)
	e.RegisterPrim("_toUpper", toUpperImpl)
	e.RegisterPrim("_toLower", toLowerImpl)
	e.RegisterPrim("_trim", trimImpl)
	e.RegisterPrim("_contains", containsImpl)
	e.RegisterPrim("_split", splitImpl)
	e.RegisterPrim("_join", joinImpl)
	e.RegisterPrim("_showInt", showIntImpl)
	e.RegisterPrim("_readInt", readIntImpl)
	e.RegisterPrim("_toRunes", toRunesImpl)
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

strlen :: String -> Int
strlen := _lengthStr

_charAt :: Int -> String -> Maybe Rune
_charAt := assumption

_substring :: Int -> Int -> String -> String
_substring := assumption

_toUpper :: String -> String
_toUpper := assumption

_toLower :: String -> String
_toLower := assumption

_trim :: String -> String
_trim := assumption

_contains :: String -> String -> Bool
_contains := assumption

_split :: String -> String -> List String
_split := assumption

_join :: String -> List String -> String
_join := assumption

_showInt :: Int -> String
_showInt := assumption

_readInt :: String -> Maybe Int
_readInt := assumption

charAt :: Int -> String -> Maybe Rune
charAt := _charAt

substring :: Int -> Int -> String -> String
substring := _substring

toUpper :: String -> String
toUpper := _toUpper

toLower :: String -> String
toLower := _toLower

trim :: String -> String
trim := _trim

contains :: String -> String -> Bool
contains := _contains

split :: String -> String -> List String
split := _split

join :: String -> List String -> String
join := _join

showInt :: Int -> String
showInt := _showInt

showBool :: Bool -> String
showBool := \b -> case b { True -> "True"; False -> "False" }

readInt :: String -> Maybe Int
readInt := _readInt

_toRunes :: String -> List Rune
_toRunes := assumption

toRunes :: String -> List Rune
toRunes := _toRunes
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

func eqStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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

func cmpStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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

func appendStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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

func emptyStrImpl(_ context.Context, ce eval.CapEnv, _ []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: ""}, ce, nil
}

func lengthStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(len([]rune(s)))}, ce, nil
}

func eqRuneImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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

func cmpRuneImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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

// --- New Str primitives ---

func charAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	idx, err := asInt64Str(args[0])
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	runes := []rune(s)
	if idx < 0 || idx >= int64(len(runes)) {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.HostVal{Inner: runes[idx]}}}, ce, nil
}

func substringImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	start, err := asInt64Str(args[0])
	if err != nil {
		return nil, ce, err
	}
	count, err := asInt64Str(args[1])
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[2])
	if err != nil {
		return nil, ce, err
	}
	runes := []rune(s)
	n := int64(len(runes))
	if start < 0 {
		start = 0
	}
	if start >= n {
		return &eval.HostVal{Inner: ""}, ce, nil
	}
	end := start + count
	if end > n {
		end = n
	}
	return &eval.HostVal{Inner: string(runes[start:end])}, ce, nil
}

func toUpperImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strings.ToUpper(s)}, ce, nil
}

func toLowerImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strings.ToLower(s)}, ce, nil
}

func trimImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strings.TrimSpace(s)}, ce, nil
}

func containsImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	needle, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	haystack, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	if strings.Contains(haystack, needle) {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func splitImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	sep, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	parts := strings.Split(s, sep)
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := len(parts) - 1; i >= 0; i-- {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{&eval.HostVal{Inner: parts[i]}, result}}
	}
	return result, ce, nil
}

func joinImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	sep, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	var strs []string
	v := args[1]
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("join: expected List String")
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("join: malformed list")
		}
		s, err := asString(con.Args[0])
		if err != nil {
			return nil, ce, fmt.Errorf("join: %w", err)
		}
		strs = append(strs, s)
		v = con.Args[1]
	}
	return &eval.HostVal{Inner: strings.Join(strs, sep)}, ce, nil
}

func showIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64Str(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strconv.FormatInt(n, 10)}, ce, nil
}

func readIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.HostVal{Inner: n}}}, ce, nil
}

func toRunesImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	runes := []rune(s)
	for i := len(runes) - 1; i >= 0; i-- {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{&eval.HostVal{Inner: runes[i]}, result}}
	}
	return result, ce, nil
}

func asInt64Str(v eval.Value) (int64, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return 0, fmt.Errorf("stdlib/str: expected HostVal(int64), got %T", v)
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		return 0, fmt.Errorf("stdlib/str: expected int64, got %T", hv.Inner)
	}
	return n, nil
}
