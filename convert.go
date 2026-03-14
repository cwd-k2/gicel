package gicel

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
)

// ToValue wraps a Go value as a GICEL Value.
// Supported types: int, int64, float64, string, bool, nil.
// For other types, use &HostVal{Inner: v} directly.
func ToValue(v any) Value {
	if v == nil {
		return &eval.RecordVal{Fields: map[string]eval.Value{}}
	}
	switch x := v.(type) {
	case Value:
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
// Returns false and ok=false if the value is not True or False.
func FromBool(v Value) (bool, bool) {
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
// Returns nil and ok=false if the value is not a HostVal.
func FromHost(v Value) (any, bool) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, false
	}
	return hv.Inner, true
}

// FromCon extracts the constructor name and arguments from a ConVal.
// Returns empty string and nil if the value is not a ConVal.
func FromCon(v Value) (name string, args []Value, ok bool) {
	con, ok := v.(*eval.ConVal)
	if !ok {
		return "", nil, false
	}
	return con.Con, con.Args, true
}

// ToList converts a Go slice to a GICEL List (ConVal chain).
func ToList(items []any) Value {
	var result Value = &eval.ConVal{Con: "Nil"}
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(Value)
		if !ok {
			item = ToValue(items[i])
		}
		result = &eval.ConVal{Con: "Cons", Args: []Value{item, result}}
	}
	return result
}

// FromList converts a GICEL List (ConVal chain) to a Go slice.
// Returns nil and false if the value is not a valid List.
func FromList(v Value) ([]any, bool) {
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
// Returns nil and ok=false if the value is not a RecordVal.
func FromRecord(v Value) (map[string]Value, bool) {
	rv, ok := v.(*eval.RecordVal)
	if !ok {
		return nil, false
	}
	return rv.Fields, true
}

// MustHost extracts the inner Go value from a HostVal, panicking if it is not one.
func MustHost[T any](v Value) T {
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
