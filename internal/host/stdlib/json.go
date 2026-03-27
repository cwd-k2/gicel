package stdlib

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
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

	return e.RegisterModule("Data.JSON", jsonSource)
}

var jsonSource = mustReadSource("json")

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
		return nil, ce, fmt.Errorf("json: expected Bool, got %T", args[0])
	}
	if con.Con == "True" {
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
			return nil, ce, fmt.Errorf("json: expected List, got %T", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("json: malformed list")
		}
		result, newCe, err := apply(encoder, con.Args[0], ce)
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
		return nil, ce, fmt.Errorf("json: expected Maybe, got %T", maybe)
	}
	if con.Con == "Nothing" {
		return jsonStrVal("null"), ce, nil
	}
	if con.Con != "Just" || len(con.Args) != 1 {
		return nil, ce, fmt.Errorf("json: malformed Maybe")
	}
	return apply(encoder, con.Args[0], ce)
}

// _toJSONPair :: (a -> String) -> (b -> String) -> (a, b) -> String
func toJSONPairImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	encA := args[0]
	encB := args[1]
	pair, ok := args[2].(*eval.RecordVal)
	if !ok {
		return nil, ce, fmt.Errorf("json: expected tuple, got %T", args[2])
	}
	a, ok1 := pair.Get(ir.TupleLabel(1))
	b, ok2 := pair.Get(ir.TupleLabel(2))
	if !ok1 || !ok2 {
		return nil, ce, fmt.Errorf("json: expected tuple with _1 and _2")
	}
	sa, newCe, err := apply(encA, a, ce)
	if err != nil {
		return nil, ce, err
	}
	sb, newCe, err := apply(encB, b, newCe)
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
		return nil, ce, fmt.Errorf("json: expected Result, got %T", args[2])
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
		return nil, ce, fmt.Errorf("json: unknown Result constructor: %s", r.Con)
	}
	sv, newCe, err := apply(encoder, inner, ce)
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
		result, newCe, err := apply(decoder, jsonStrVal(string(elem)), ce)
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
	result, newCe, err := apply(decoder, jsonStrVal(s), ce)
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
	ra, newCe, err := apply(decA, jsonStrVal(string(raw[0])), ce)
	if err != nil {
		return nil, ce, err
	}
	conA, ok := ra.(*eval.ConVal)
	if !ok || conA.Con == "Nothing" {
		return jsonNothing, newCe, nil
	}
	rb, newCe, err := apply(decB, jsonStrVal(string(raw[1])), newCe)
	if err != nil {
		return nil, ce, err
	}
	conB, ok := rb.(*eval.ConVal)
	if !ok || conB.Con == "Nothing" {
		return jsonNothing, newCe, nil
	}
	pair := eval.NewRecordFromMap(map[string]eval.Value{
		ir.TupleLabel(1): conA.Args[0],
		ir.TupleLabel(2): conB.Args[0],
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
		rv, newCe, err := apply(decA, valStr, ce)
		if err != nil {
			return nil, ce, err
		}
		con, ok := rv.(*eval.ConVal)
		if !ok || con.Con == "Nothing" {
			return jsonNothing, newCe, nil
		}
		return jsonJust(&eval.ConVal{Con: "Ok", Args: []eval.Value{con.Args[0]}}), newCe, nil
	case "Err":
		rv, newCe, err := apply(decE, valStr, ce)
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
