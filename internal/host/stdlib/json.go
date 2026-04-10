package stdlib

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// JSON provides ToJSON/FromJSON type classes for JSON encoding/decoding.
var JSON Pack = func(e Registrar) error {
	// ToJSON primitives
	e.RegisterPrim("_toJSONInt", toJSONIntImpl)
	e.RegisterPrim("_toJSONDouble", toJSONDoubleImpl)
	e.RegisterPrim("_toJSONString", toJSONStringImpl)
	e.RegisterPrim("_toJSONBool", toJSONBoolImpl)
	e.RegisterPrim("_toJSONList", toJSONListImpl)
	e.RegisterPrim("_toJSONMaybe", toJSONMaybeImpl)
	e.RegisterPrim("_toJSONPair", toJSONPairImpl)
	e.RegisterPrim("_toJSONResult", toJSONResultImpl)

	// FromJSON primitives
	e.RegisterPrim("_fromJSONInt", fromJSONIntImpl)
	e.RegisterPrim("_fromJSONDouble", fromJSONDoubleImpl)
	e.RegisterPrim("_fromJSONString", fromJSONStringImpl)
	e.RegisterPrim("_fromJSONBool", fromJSONBoolImpl)
	e.RegisterPrim("_fromJSONList", fromJSONListImpl)
	e.RegisterPrim("_fromJSONMaybe", fromJSONMaybeImpl)
	e.RegisterPrim("_fromJSONPair", fromJSONPairImpl)
	e.RegisterPrim("_fromJSONResult", fromJSONResultImpl)

	// Map/Set JSON primitives
	e.RegisterPrim("_toJSONMap", toJSONMapImpl)
	e.RegisterPrim("_toJSONSet", toJSONSetImpl)
	e.RegisterPrim("_fromJSONMap", fromJSONMapImpl)
	e.RegisterPrim("_fromJSONSet", fromJSONSetImpl)

	// Generic encoder
	e.RegisterPrim("_jsonEncode", jsonEncodeImpl)

	return e.RegisterModule("Data.JSON", jsonSource)
}

var jsonSource = mustReadSource("json")

// --- jsonEncode: runtime-introspective JSON serializer ---

// maxJSONDepth bounds recursive JSON encoding to prevent Go stack overflow.
const maxJSONDepth = 512

func jsonEncodeImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	var buf strings.Builder
	if err := writeJSONValue(ctx, &buf, args[0], 0); err != nil {
		return nil, ce, err
	}
	result := buf.String()
	if err := budget.ChargeAlloc(ctx, int64(len(result))*costPerByte); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
}

func writeJSONValue(ctx context.Context, buf *strings.Builder, v eval.Value, depth int) error {
	if depth > maxJSONDepth {
		return errors.New("json encode: depth limit exceeded")
	}
	if err := budget.CheckContext(ctx); err != nil {
		return err
	}
	switch val := v.(type) {
	case *eval.HostVal:
		writeJSONHost(buf, val.Inner)
	case *eval.ConVal:
		return writeJSONCon(ctx, buf, val, depth)
	case *eval.RecordVal:
		if eval.IsTuple(val) {
			return writeJSONTuple(ctx, buf, val, depth)
		}
		return writeJSONRecord(ctx, buf, val, depth)
	default:
		buf.WriteString("null")
	}
	return nil
}

func writeJSONHost(buf *strings.Builder, v any) {
	switch val := v.(type) {
	case int:
		buf.WriteString(strconv.Itoa(val))
	case int64:
		buf.WriteString(strconv.FormatInt(val, 10))
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			buf.WriteString("null")
		} else {
			buf.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
		}
	case string:
		b, _ := json.Marshal(val)
		buf.Write(b)
	case rune:
		b, _ := json.Marshal(string(val))
		buf.Write(b)
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case byte:
		buf.WriteString(strconv.Itoa(int(val)))
	default:
		buf.WriteString("null")
	}
}

func writeJSONCon(ctx context.Context, buf *strings.Builder, val *eval.ConVal, depth int) error {
	// List
	if elems, ok := eval.CollectList(val); ok {
		buf.WriteByte('[')
		for i, e := range elems {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJSONValue(ctx, buf, e, depth+1); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	}
	// Bool
	if b, ok := eval.IsBool(val); ok {
		if b {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil
	}
	// Maybe
	if val.Con == "Nothing" && len(val.Args) == 0 {
		buf.WriteString("null")
		return nil
	}
	if val.Con == "Just" && len(val.Args) == 1 {
		return writeJSONValue(ctx, buf, val.Args[0], depth+1)
	}
	// Result
	if val.Con == "Ok" && len(val.Args) == 1 {
		buf.WriteString(`{"ok":`)
		if err := writeJSONValue(ctx, buf, val.Args[0], depth+1); err != nil {
			return err
		}
		buf.WriteByte('}')
		return nil
	}
	if val.Con == "Err" && len(val.Args) == 1 {
		buf.WriteString(`{"err":`)
		if err := writeJSONValue(ctx, buf, val.Args[0], depth+1); err != nil {
			return err
		}
		buf.WriteByte('}')
		return nil
	}
	// Unit
	if val.Con == "()" && len(val.Args) == 0 {
		buf.WriteString("null")
		return nil
	}
	// Generic ADT
	buf.WriteString(`{"tag":`)
	b, _ := json.Marshal(val.Con)
	buf.Write(b)
	if len(val.Args) > 0 {
		buf.WriteString(`,"fields":[`)
		for i, a := range val.Args {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJSONValue(ctx, buf, a, depth+1); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	}
	buf.WriteByte('}')
	return nil
}

func writeJSONTuple(ctx context.Context, buf *strings.Builder, r *eval.RecordVal, depth int) error {
	n := r.Len()
	if n == 0 {
		buf.WriteString("null")
		return nil
	}
	buf.WriteByte('[')
	for i := range n {
		if i > 0 {
			buf.WriteByte(',')
		}
		v, _ := r.Get(types.TupleLabel(i + 1))
		if err := writeJSONValue(ctx, buf, v, depth+1); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

func writeJSONRecord(ctx context.Context, buf *strings.Builder, r *eval.RecordVal, depth int) error {
	fields := r.RawFields()
	buf.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			buf.WriteByte(',')
		}
		b, _ := json.Marshal(f.Label)
		buf.Write(b)
		buf.WriteByte(':')
		if err := writeJSONValue(ctx, buf, f.Value, depth+1); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

// --- helpers ---

var jsonNothing = &eval.ConVal{Con: "Nothing"}

func jsonJust(v eval.Value) *eval.ConVal {
	return &eval.ConVal{Con: "Just", Args: []eval.Value{v}}
}

func jsonStrVal(s string) eval.Value {
	return &eval.HostVal{Inner: s}
}

// --- ToJSON primitives ---

func toJSONIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "json")
	if err != nil {
		return nil, ce, err
	}
	return jsonStrVal(strconv.FormatInt(n, 10)), ce, nil
}

func toJSONDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64(args[0], "json")
	if err != nil {
		return nil, ce, err
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return jsonStrVal("null"), ce, nil
	}
	return jsonStrVal(strconv.FormatFloat(f, 'f', -1, 64)), ce, nil
}

func toJSONStringImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	bs, err := json.Marshal(s)
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(len(bs))*costPerByte); err != nil {
		return nil, ce, err
	}
	return jsonStrVal(string(bs)), ce, nil
}

func toJSONBoolImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	con, ok := args[0].(*eval.ConVal)
	if !ok {
		return nil, ce, errExpected("json", "Bool", args[0])
	}
	if con.Con == eval.BoolTrue {
		return jsonStrVal("true"), ce, nil
	}
	return jsonStrVal("false"), ce, nil
}

// _toJSONList :: (a -> String) -> List a -> String
func toJSONListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	encoder := args[0]
	list := args[1]
	var parts []string
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("json", "List", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("json", "list node", con.Con)
		}
		result, newCe, err := apply.Apply(encoder, con.Args[0], ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		s, err := asString(result)
		if err != nil {
			return nil, ce, err
		}
		parts = append(parts, s)
		list = con.Args[1]
	}
	out := "[" + strings.Join(parts, ",") + "]"
	if err := budget.ChargeAlloc(ctx, int64(len(out))*costPerByte); err != nil {
		return nil, ce, err
	}
	return jsonStrVal(out), ce, nil
}

// _toJSONMaybe :: (a -> String) -> Maybe a -> String
func toJSONMaybeImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	encoder := args[0]
	maybe := args[1]
	con, ok := maybe.(*eval.ConVal)
	if !ok {
		return nil, ce, errExpected("json", "Maybe", maybe)
	}
	if con.Con == "Nothing" {
		return jsonStrVal("null"), ce, nil
	}
	if con.Con != "Just" || len(con.Args) != 1 {
		return nil, ce, errMalformed("json", "Maybe", con.Con)
	}
	return apply.Apply(encoder, con.Args[0], ce)
}

// _toJSONPair :: (a -> String) -> (b -> String) -> (a, b) -> String
func toJSONPairImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	encA := args[0]
	encB := args[1]
	pair, ok := args[2].(*eval.RecordVal)
	if !ok {
		return nil, ce, errExpected("json", "tuple", args[2])
	}
	a, ok1 := pair.Get(types.TupleLabel(1))
	b, ok2 := pair.Get(types.TupleLabel(2))
	if !ok1 || !ok2 {
		return nil, ce, errors.New("json: expected tuple with _1 and _2")
	}
	sa, newCe, err := apply.Apply(encA, a, ce)
	if err != nil {
		return nil, ce, err
	}
	sb, newCe, err := apply.Apply(encB, b, newCe)
	if err != nil {
		return nil, ce, err
	}
	sA, err := asString(sa)
	if err != nil {
		return nil, ce, err
	}
	sB, err := asString(sb)
	if err != nil {
		return nil, ce, err
	}
	out := "[" + sA + "," + sB + "]"
	if err := budget.ChargeAlloc(ctx, int64(len(out))*costPerByte); err != nil {
		return nil, newCe, err
	}
	return jsonStrVal(out), newCe, nil
}

// _toJSONResult :: (e -> String) -> (a -> String) -> Result e a -> String
func toJSONResultImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	encE := args[0]
	encA := args[1]
	r, ok := args[2].(*eval.ConVal)
	if !ok {
		return nil, ce, errExpected("json", "Result", args[2])
	}
	var tag string
	var encoder, inner eval.Value
	switch r.Con {
	case "Ok":
		tag = "Ok"
		encoder = encA
		inner = r.Args[0]
	case "Err":
		tag = "Err"
		encoder = encE
		inner = r.Args[0]
	default:
		return nil, ce, errMalformed("json", "Result constructor", r.Con)
	}
	sv, newCe, err := apply.Apply(encoder, inner, ce)
	if err != nil {
		return nil, ce, err
	}
	s, err := asString(sv)
	if err != nil {
		return nil, ce, err
	}
	out := `{"tag":"` + tag + `","value":` + s + "}"
	if err := budget.ChargeAlloc(ctx, int64(len(out))*costPerByte); err != nil {
		return nil, newCe, err
	}
	return jsonStrVal(out), newCe, nil
}

// --- FromJSON primitives ---

func fromJSONIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	if strings.TrimSpace(s) == "null" {
		return jsonNothing, ce, nil
	}
	var f float64
	if err := json.Unmarshal([]byte(s), &f); err != nil {
		return jsonNothing, ce, nil
	}
	n := int64(f)
	if float64(n) != f {
		return jsonNothing, ce, nil
	}
	return jsonJust(&eval.HostVal{Inner: n}), ce, nil
}

func fromJSONDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	if strings.TrimSpace(s) == "null" {
		return jsonNothing, ce, nil
	}
	var f float64
	if err := json.Unmarshal([]byte(s), &f); err != nil {
		return jsonNothing, ce, nil
	}
	return jsonJust(&eval.HostVal{Inner: f}), ce, nil
}

func fromJSONStringImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	if strings.TrimSpace(s) == "null" {
		return jsonNothing, ce, nil
	}
	var out string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return jsonNothing, ce, nil
	}
	return jsonJust(&eval.HostVal{Inner: out}), ce, nil
}

func fromJSONBoolImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	if strings.TrimSpace(s) == "null" {
		return jsonNothing, ce, nil
	}
	var b bool
	if err := json.Unmarshal([]byte(s), &b); err != nil {
		return jsonNothing, ce, nil
	}
	return jsonJust(boolVal(b)), ce, nil
}

// _fromJSONList :: (String -> Maybe a) -> String -> Maybe (List a)
func fromJSONListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	decoder := args[0]
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return jsonNothing, ce, nil
	}
	if err := budget.ChargeAlloc(ctx, int64(len(raw))*costConsNode); err != nil {
		return nil, ce, err
	}
	items := make([]eval.Value, 0, len(raw))
	for _, elem := range raw {
		result, newCe, err := apply.Apply(decoder, jsonStrVal(string(elem)), ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		con, ok := result.(*eval.ConVal)
		if !ok || con.Con == "Nothing" {
			return jsonNothing, ce, nil
		}
		items = append(items, con.Args[0])
	}
	return jsonJust(buildList(items)), ce, nil
}

// _fromJSONMaybe :: (String -> Maybe a) -> String -> Maybe (Maybe a)
func fromJSONMaybeImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	decoder := args[0]
	s, err := asString(args[1])
	if err != nil {
		return nil, ce, err
	}
	s = strings.TrimSpace(s)
	if s == "null" {
		// JSON null → Just Nothing
		return jsonJust(&eval.ConVal{Con: "Nothing"}), ce, nil
	}
	result, newCe, err := apply.Apply(decoder, jsonStrVal(s), ce)
	if err != nil {
		return nil, ce, err
	}
	con, ok := result.(*eval.ConVal)
	if !ok || con.Con == "Nothing" {
		return jsonNothing, newCe, nil
	}
	// Just (Just x)
	return jsonJust(jsonJust(con.Args[0])), newCe, nil
}

// _fromJSONPair :: (String -> Maybe a) -> (String -> Maybe b) -> String -> Maybe (a, b)
func fromJSONPairImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	decA := args[0]
	decB := args[1]
	s, err := asString(args[2])
	if err != nil {
		return nil, ce, err
	}
	var raw [2]json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return jsonNothing, ce, nil
	}
	ra, newCe, err := apply.Apply(decA, jsonStrVal(string(raw[0])), ce)
	if err != nil {
		return nil, ce, err
	}
	conA, ok := ra.(*eval.ConVal)
	if !ok || conA.Con == "Nothing" {
		return jsonNothing, newCe, nil
	}
	rb, newCe, err := apply.Apply(decB, jsonStrVal(string(raw[1])), newCe)
	if err != nil {
		return nil, ce, err
	}
	conB, ok := rb.(*eval.ConVal)
	if !ok || conB.Con == "Nothing" {
		return jsonNothing, newCe, nil
	}
	pair := eval.NewRecordFromMap(map[string]eval.Value{
		types.TupleLabel(1): conA.Args[0],
		types.TupleLabel(2): conB.Args[0],
	})
	return jsonJust(pair), newCe, nil
}

// _fromJSONResult :: (String -> Maybe e) -> (String -> Maybe a) -> String -> Maybe (Result e a)
func fromJSONResultImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	decE := args[0]
	decA := args[1]
	s, err := asString(args[2])
	if err != nil {
		return nil, ce, err
	}
	var obj struct {
		Tag   string          `json:"tag"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return jsonNothing, ce, nil
	}
	valStr := jsonStrVal(string(obj.Value))
	switch obj.Tag {
	case "Ok":
		rv, newCe, err := apply.Apply(decA, valStr, ce)
		if err != nil {
			return nil, ce, err
		}
		con, ok := rv.(*eval.ConVal)
		if !ok || con.Con == "Nothing" {
			return jsonNothing, newCe, nil
		}
		return jsonJust(&eval.ConVal{Con: "Ok", Args: []eval.Value{con.Args[0]}}), newCe, nil
	case "Err":
		rv, newCe, err := apply.Apply(decE, valStr, ce)
		if err != nil {
			return nil, ce, err
		}
		con, ok := rv.(*eval.ConVal)
		if !ok || con.Con == "Nothing" {
			return jsonNothing, newCe, nil
		}
		return jsonJust(&eval.ConVal{Con: "Err", Args: []eval.Value{con.Args[0]}}), newCe, nil
	default:
		return jsonNothing, ce, nil
	}
}

// --- Map/Set JSON support ---

// _toJSONMap :: (k -> String) -> (v -> String) -> Map k v -> String
// Serializes as array of [key, value] pairs: [[k1,v1],[k2,v2],...]
func toJSONMapImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	encK := args[0]
	encV := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	var parts []string
	ce, err = avlFoldInOrder(m.root, ce, func(n *avlNode, ce eval.CapEnv) (eval.CapEnv, error) {
		sk, newCe, err := apply.Apply(encK, n.key, ce)
		if err != nil {
			return ce, err
		}
		sv, newCe, err := apply.Apply(encV, n.value, newCe)
		if err != nil {
			return ce, err
		}
		sK, err := asString(sk)
		if err != nil {
			return ce, err
		}
		sV, err := asString(sv)
		if err != nil {
			return ce, err
		}
		parts = append(parts, "["+sK+","+sV+"]")
		return newCe, nil
	})
	if err != nil {
		return nil, ce, err
	}
	out := "[" + strings.Join(parts, ",") + "]"
	if err := budget.ChargeAlloc(ctx, int64(len(out))*costPerByte); err != nil {
		return nil, ce, err
	}
	return jsonStrVal(out), ce, nil
}

// _toJSONSet :: (k -> String) -> Set k -> String
// Serializes as array of values: [v1,v2,v3,...]
func toJSONSetImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	enc := args[0]
	m, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	var parts []string
	ce, err = avlFoldInOrder(m.root, ce, func(n *avlNode, ce eval.CapEnv) (eval.CapEnv, error) {
		sv, newCe, err := apply.Apply(enc, n.key, ce)
		if err != nil {
			return ce, err
		}
		s, err := asString(sv)
		if err != nil {
			return ce, err
		}
		parts = append(parts, s)
		return newCe, nil
	})
	if err != nil {
		return nil, ce, err
	}
	out := "[" + strings.Join(parts, ",") + "]"
	if err := budget.ChargeAlloc(ctx, int64(len(out))*costPerByte); err != nil {
		return nil, ce, err
	}
	return jsonStrVal(out), ce, nil
}

// _fromJSONMap :: (k -> k -> Ordering) -> (String -> Maybe k) -> (String -> Maybe v) -> String -> Maybe (Map k v)
func fromJSONMapImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmpFn := args[0]
	decK := args[1]
	decV := args[2]
	s, err := asString(args[3])
	if err != nil {
		return nil, ce, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return jsonNothing, ce, nil
	}
	if err := budget.ChargeAlloc(ctx, int64(len(raw))*costAVLNode); err != nil {
		return nil, ce, err
	}
	m := &mapVal{cmp: cmpFn}
	for _, elem := range raw {
		var pair [2]json.RawMessage
		if err := json.Unmarshal(elem, &pair); err != nil {
			return jsonNothing, ce, nil
		}
		rk, newCe, err := apply.Apply(decK, jsonStrVal(string(pair[0])), ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		conK, ok := rk.(*eval.ConVal)
		if !ok || conK.Con == "Nothing" {
			return jsonNothing, ce, nil
		}
		rv, newCe, err := apply.Apply(decV, jsonStrVal(string(pair[1])), ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		conV, ok := rv.(*eval.ConVal)
		if !ok || conV.Con == "Nothing" {
			return jsonNothing, ce, nil
		}
		var inserted bool
		m.root, inserted, ce, err = avlInsert(m.root, conK.Args[0], conV.Args[0], cmpFn, ce, apply)
		if err != nil {
			return nil, ce, err
		}
		if inserted {
			m.size++
		}
	}
	return jsonJust(&eval.HostVal{Inner: m}), ce, nil
}

// _fromJSONSet :: (k -> k -> Ordering) -> (String -> Maybe k) -> String -> Maybe (Set k)
func fromJSONSetImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmpFn := args[0]
	decK := args[1]
	s, err := asString(args[2])
	if err != nil {
		return nil, ce, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return jsonNothing, ce, nil
	}
	if err := budget.ChargeAlloc(ctx, int64(len(raw))*costAVLNode); err != nil {
		return nil, ce, err
	}
	m := &mapVal{cmp: cmpFn}
	for _, elem := range raw {
		rk, newCe, err := apply.Apply(decK, jsonStrVal(string(elem)), ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		conK, ok := rk.(*eval.ConVal)
		if !ok || conK.Con == "Nothing" {
			return jsonNothing, ce, nil
		}
		var inserted bool
		m.root, inserted, ce, err = avlInsert(m.root, conK.Args[0], unitVal, cmpFn, ce, apply)
		if err != nil {
			return nil, ce, err
		}
		if inserted {
			m.size++
		}
	}
	return jsonJust(&eval.HostVal{Inner: m}), ce, nil
}
