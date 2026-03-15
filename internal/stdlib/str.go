package stdlib

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
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
	e.RegisterPrim("_fromRunes", fromRunesImpl)
	// Fusion rule: packed roundtrip elimination.
	e.RegisterRewriteRule(strPackedRoundtrip)
	return e.RegisterModule("Std.Str", strSource)
}

// R13: _fromRunes (_toRunes x) → x
func strPackedRoundtrip(c core.Core) core.Core {
	po, ok := c.(*core.PrimOp)
	if !ok || po.Name != "_fromRunes" || len(po.Args) != 1 {
		return c
	}
	inner, ok := po.Args[0].(*core.PrimOp)
	if !ok || inner.Name != "_toRunes" || len(inner.Args) != 1 {
		return c
	}
	return inner.Args[0]
}

var strSource = mustReadSource("str")

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
	end := min(start+max(count, 0), n)
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

func fromRunesImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	items, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, fmt.Errorf("fromRunes: expected List Rune")
	}
	runes := make([]rune, len(items))
	for i, item := range items {
		r, err := asRune(item)
		if err != nil {
			return nil, ce, fmt.Errorf("fromRunes: element %d: %w", i, err)
		}
		runes[i] = r
	}
	return &eval.HostVal{Inner: string(runes)}, ce, nil
}

func asInt64Str(v eval.Value) (int64, error) { return asInt64(v, "str") }
