package stdlib

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/eval"
)

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
	return boolVal(a == b), ce, nil
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
	return ordVal(strings.Compare(a, b)), ce, nil
}

func appendStrImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	if err := eval.ChargeAlloc(ctx, int64(len(a)+len(b))*costPerByte); err != nil {
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
	return &eval.HostVal{Inner: int64(utf8.RuneCountInString(s))}, ce, nil
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
	return boolVal(a == b), ce, nil
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
		return ordVal(-1), ce, nil
	case a > b:
		return ordVal(1), ce, nil
	default:
		return ordVal(0), ce, nil
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
	if idx < 0 {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	var i int64
	for _, r := range s {
		if i == idx {
			return &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.HostVal{Inner: r}}}, ce, nil
		}
		i++
	}
	return &eval.ConVal{Con: "Nothing"}, ce, nil
}

func substringImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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
	if start < 0 {
		start = 0
	}
	if count <= 0 {
		return &eval.HostVal{Inner: ""}, ce, nil
	}
	var runeIdx int64
	startByte := len(s)
	endByte := len(s)
	for bytePos := range s {
		if runeIdx == start {
			startByte = bytePos
		}
		if runeIdx == start+count {
			endByte = bytePos
			break
		}
		runeIdx++
	}
	if startByte >= len(s) {
		return &eval.HostVal{Inner: ""}, ce, nil
	}
	result := s[startByte:endByte]
	if err := eval.ChargeAlloc(ctx, int64(len(result))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
}

func toUpperImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := eval.ChargeAlloc(ctx, int64(len(s))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strings.ToUpper(s)}, ce, nil
}

func toLowerImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := eval.ChargeAlloc(ctx, int64(len(s))*costPerByte); err != nil {
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
	return boolVal(strings.Contains(haystack, needle)), ce, nil
}

func splitImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	sep, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	parts := strings.Split(s, sep)
	if err := eval.ChargeAlloc(ctx, int64(len(parts))*costConsNode+int64(len(s))*costPerByte); err != nil {
		return nil, ce, err
	}
	items := make([]eval.Value, len(parts))
	for i, p := range parts {
		items[i] = &eval.HostVal{Inner: p}
	}
	return buildList(items), ce, nil
}

func joinImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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
	result := strings.Join(strs, sep)
	if err := eval.ChargeAlloc(ctx, int64(len(result))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
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

func toRunesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	runes := []rune(s)
	if err := eval.ChargeAlloc(ctx, int64(len(runes))*(4+costConsNode)); err != nil {
		return nil, ce, err
	}
	items := make([]eval.Value, len(runes))
	for i, r := range runes {
		items[i] = &eval.HostVal{Inner: r}
	}
	return buildList(items), ce, nil
}

func fromRunesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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
	result := string(runes)
	if err := eval.ChargeAlloc(ctx, int64(len(result))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
}

func asInt64Str(v eval.Value) (int64, error) { return asInt64(v, "str") }
