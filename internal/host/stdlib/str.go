package stdlib

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Primitive names for string pack/unpack operations used in registration and fusion rules.
const (
	primFromRunes   = "_fromRunes"
	primToRunes     = "_toRunes"
	primPackRunes   = "_packRunes"
	primUnpackRunes = "_unpackRunes"
	primPackBytes   = "_packBytes"
	primUnpackBytes = "_unpackBytes"
)

// R13: _fromRunes (_toRunes x) → x
func strPackedRoundtrip(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || len(po.Args) != 1 {
		return c
	}
	inner, ok := po.Args[0].(*ir.PrimOp)
	if !ok || len(inner.Args) != 1 {
		return c
	}
	// List-based roundtrip: _fromRunes (_toRunes x) → x
	// Slice-based roundtrip: _packRunes (_unpackRunes x) → x
	// Slice-based roundtrip: _packBytes (_unpackBytes x) → x
	switch {
	case po.Name == primFromRunes && inner.Name == primToRunes:
		return inner.Args[0]
	case po.Name == primPackRunes && inner.Name == primUnpackRunes:
		return inner.Args[0]
	case po.Name == primPackBytes && inner.Name == primUnpackBytes:
		return inner.Args[0]
	}
	return c
}

func asString(v eval.Value) (string, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return "", errExpected("stdlib/str", "HostVal", v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		return "", errExpected("stdlib/str", "string", hv.Inner)
	}
	return s, nil
}

func asRune(v eval.Value) (rune, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return 0, errExpected("stdlib/str", "HostVal", v)
	}
	r, ok := hv.Inner.(rune)
	if !ok {
		return 0, errExpected("stdlib/str", "rune", hv.Inner)
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
	if err := budget.ChargeAlloc(ctx, int64(len(a)+len(b))*costPerByte); err != nil {
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
	if err := budget.ChargeAlloc(ctx, int64(len(result))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
}

func toUpperImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(len(s))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strings.ToUpper(s)}, ce, nil
}

func toLowerImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(len(s))*costPerByte); err != nil {
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
	if err := budget.ChargeAlloc(ctx, int64(len(parts))*costConsNode+int64(len(s))*costPerByte); err != nil {
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
			return nil, ce, errors.New("join: expected List String")
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("join", "list node", con.Con)
		}
		s, err := asString(con.Args[0])
		if err != nil {
			return nil, ce, fmt.Errorf("join: %w", err)
		}
		strs = append(strs, s)
		if len(strs)&1023 == 0 {
			if err := budget.CheckContext(ctx); err != nil {
				return nil, ce, err
			}
		}
		v = con.Args[1]
	}
	result := strings.Join(strs, sep)
	if err := budget.ChargeAlloc(ctx, int64(len(result))*costPerByte); err != nil {
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

func readDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.HostVal{Inner: f}}}, ce, nil
}

func wordsImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	fields := strings.Fields(s)
	if err := budget.ChargeAlloc(ctx, int64(len(fields))*costConsNode+int64(len(s))*costPerByte); err != nil {
		return nil, ce, err
	}
	items := make([]eval.Value, len(fields))
	for i, f := range fields {
		items[i] = &eval.HostVal{Inner: f}
	}
	return buildList(items), ce, nil
}

func toRunesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	runes := []rune(s)
	if err := budget.ChargeAlloc(ctx, int64(len(runes))*(4+costConsNode)); err != nil {
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
		return nil, ce, errors.New("fromRunes: expected List Rune")
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
	if err := budget.ChargeAlloc(ctx, int64(len(result))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
}

func asInt64Str(v eval.Value) (int64, error) { return asInt64(v, "str") }

// --- Packed (Slice-based) primitives ---

func packRunesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSliceStr(args[0])
	if err != nil {
		return nil, ce, err
	}
	runes := make([]rune, len(s))
	for i, item := range s {
		r, err := asRune(item)
		if err != nil {
			return nil, ce, fmt.Errorf("packRunes: element %d: %w", i, err)
		}
		runes[i] = r
	}
	result := string(runes)
	if err := budget.ChargeAlloc(ctx, int64(len(result))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
}

func unpackRunesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	runes := []rune(s)
	if err := budget.ChargeAlloc(ctx, int64(len(runes))*costSlotSize); err != nil {
		return nil, ce, err
	}
	items := make([]eval.Value, len(runes))
	for i, r := range runes {
		items[i] = &eval.HostVal{Inner: r}
	}
	return &eval.HostVal{Inner: items}, ce, nil
}

func packBytesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSliceStr(args[0])
	if err != nil {
		return nil, ce, err
	}
	bs := make([]byte, len(s))
	for i, item := range s {
		b, err := asByte(item)
		if err != nil {
			return nil, ce, fmt.Errorf("packBytes: element %d: %w", i, err)
		}
		bs[i] = b
	}
	if err := budget.ChargeAlloc(ctx, int64(len(bs))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: string(bs)}, ce, nil
}

func unpackBytesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	bs := []byte(s)
	if err := budget.ChargeAlloc(ctx, int64(len(bs))*costSlotSize); err != nil {
		return nil, ce, err
	}
	items := make([]eval.Value, len(bs))
	for i, b := range bs {
		items[i] = &eval.HostVal{Inner: b}
	}
	return &eval.HostVal{Inner: items}, ce, nil
}

// --- String additional primitives ---

func linesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	// Match Haskell: split on '\n', final empty line from trailing newline is dropped.
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if err := budget.ChargeAlloc(ctx, int64(len(parts))*costConsNode+int64(len(s))*costPerByte); err != nil {
		return nil, ce, err
	}
	items := make([]eval.Value, len(parts))
	for i, p := range parts {
		items[i] = &eval.HostVal{Inner: p}
	}
	return buildList(items), ce, nil
}

func unlinesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	items, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, errors.New("unlines: expected List String")
	}
	strs := make([]string, len(items))
	totalLen := 0
	for i, item := range items {
		s, err := asString(item)
		if err != nil {
			return nil, ce, fmt.Errorf("unlines: element %d: %w", i, err)
		}
		strs[i] = s
		totalLen += len(s) + 1 // +1 for '\n'
	}
	if err := budget.ChargeAlloc(ctx, int64(totalLen)*costPerByte); err != nil {
		return nil, ce, err
	}
	var b strings.Builder
	b.Grow(totalLen)
	for _, s := range strs {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	return &eval.HostVal{Inner: b.String()}, ce, nil
}

func isPrefixOfStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	prefix, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(strings.HasPrefix(s, prefix)), ce, nil
}

func isSuffixOfStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	suffix, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(strings.HasSuffix(s, suffix)), ce, nil
}

func asSliceStr(v eval.Value) ([]eval.Value, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, errExpected("stdlib/str", "HostVal", v)
	}
	s, ok := hv.Inner.([]eval.Value)
	if !ok {
		return nil, errExpected("stdlib/str", "[]Value", hv.Inner)
	}
	return s, nil
}

// --- Byte primitives ---

func asByte(v eval.Value) (byte, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return 0, errExpected("stdlib/byte", "HostVal", v)
	}
	b, ok := hv.Inner.(byte)
	if !ok {
		return 0, errExpected("stdlib/byte", "byte", hv.Inner)
	}
	return b, nil
}

func eqByteImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asByte(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asByte(args[1])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(a == b), ce, nil
}

func cmpByteImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asByte(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asByte(args[1])
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

func showByteImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	b, err := asByte(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strconv.FormatUint(uint64(b), 10)}, ce, nil
}

func byteToIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	b, err := asByte(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(b)}, ce, nil
}

func intToByteImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "byte")
	if err != nil {
		return nil, ce, err
	}
	if n < 0 || n > 255 {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.HostVal{Inner: byte(n)}}}, ce, nil
}

// --- String enhancement (2026-04-11) ---

func indexOfStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	needle, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	haystack, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	idx := strings.Index(haystack, needle)
	if idx < 0 {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{eval.IntVal(int64(idx))}}, ce, nil
}

func lastIndexOfStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	needle, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	haystack, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	idx := strings.LastIndex(haystack, needle)
	if idx < 0 {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{eval.IntVal(int64(idx))}}, ce, nil
}

func countStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	needle, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	haystack, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	return eval.IntVal(int64(strings.Count(haystack, needle))), ce, nil
}

func replaceStrImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	old, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	new_, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[2])
	if err != nil {
		return nil, ce, err
	}
	result := strings.ReplaceAll(s, old, new_)
	if err := budget.ChargeAlloc(ctx, int64(len(result))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
}

func reverseStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return &eval.HostVal{Inner: string(runes)}, ce, nil
}

func replicateStrImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "str")
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	if n <= 0 {
		return &eval.HostVal{Inner: ""}, ce, nil
	}
	if err := budget.ChargeAlloc(ctx, int64(n)*int64(len(s))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strings.Repeat(s, int(n))}, ce, nil
}

func stripPrefixStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	prefix, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	if result, ok := strings.CutPrefix(s, prefix); ok {
		return &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.HostVal{Inner: result}}}, ce, nil
	}
	return &eval.ConVal{Con: "Nothing"}, ce, nil
}

func stripSuffixStrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	suffix, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	if result, ok := strings.CutSuffix(s, suffix); ok {
		return &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.HostVal{Inner: result}}}, ce, nil
	}
	return &eval.ConVal{Con: "Nothing"}, ce, nil
}
