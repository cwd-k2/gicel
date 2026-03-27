package engine

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// ToValue wraps a Go value as a GICEL Value.
// nil becomes unit (empty record), bool becomes True/False constructor,
// and all other types are wrapped as HostVal.
func ToValue(v any) eval.Value {
	if v == nil {
		return eval.NewRecordFromMap(map[string]eval.Value{})
	}
	switch x := v.(type) {
	case eval.Value:
		return x
	case bool:
		if x {
			return &eval.ConVal{Con: "True"}
		}
		return &eval.ConVal{Con: "False"}
	default:
		return &eval.HostVal{Inner: v}
	}
}

// FromBool extracts a bool from a Bool constructor value.
func FromBool(v eval.Value) (bool, bool) {
	con, ok := v.(*eval.ConVal)
	if !ok {
		return false, false
	}
	switch con.Con {
	case "True":
		return true, true
	case "False":
		return false, true
	}
	return false, false
}

// FromHost extracts the inner Go value from a HostVal.
func FromHost(v eval.Value) (any, bool) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, false
	}
	return hv.Inner, true
}

// FromCon extracts the constructor name and arguments from a ConVal.
func FromCon(v eval.Value) (name string, args []eval.Value, ok bool) {
	con, ok := v.(*eval.ConVal)
	if !ok {
		return "", nil, false
	}
	args = make([]eval.Value, len(con.Args))
	copy(args, con.Args)
	return con.Con, args, true
}

// ToList converts a Go slice to a GICEL List (ConVal chain).
func ToList(items []any) eval.Value {
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(eval.Value)
		if !ok {
			item = ToValue(items[i])
		}
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{item, result}}
	}
	return result
}

// FromList converts a GICEL List (ConVal chain) to a Go slice.
func FromList(v eval.Value) ([]any, bool) {
	var result []any
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, false
		}
		switch con.Con {
		case "Nil":
			return result, true
		case "Cons":
			if len(con.Args) != 2 {
				return nil, false
			}
			result = append(result, con.Args[0])
			v = con.Args[1]
		default:
			return nil, false
		}
	}
}

// FromRecord extracts the field map from a RecordVal.
func FromRecord(v eval.Value) (map[string]eval.Value, bool) {
	rv, ok := v.(*eval.RecordVal)
	if !ok {
		return nil, false
	}
	return rv.AsMap(), true
}

// MustHost extracts the inner Go value from a HostVal, panicking if it is not one.
func MustHost[T any](v eval.Value) T {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		panic(fmt.Sprintf("gicel: expected HostVal, got %T", v))
	}
	t, ok := hv.Inner.(T)
	if !ok {
		panic(fmt.Sprintf("gicel: expected HostVal(%T), got HostVal(%T)", *new(T), hv.Inner))
	}
	return t
}
