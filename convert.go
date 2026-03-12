package gomputation

import (
	"fmt"

	"github.com/cwd-k2/gomputation/internal/eval"
)

// ToValue wraps a Go value as a Gomputation Value.
// Supported types: int, int64, float64, string, bool, nil.
// For other types, use &HostVal{Inner: v} directly.
func ToValue(v any) Value {
	if v == nil {
		return &eval.ConVal{Con: "Unit"}
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

// MustHost extracts the inner Go value from a HostVal, panicking if it is not one.
func MustHost[T any](v Value) T {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		panic(fmt.Sprintf("gomputation: expected HostVal, got %T", v))
	}
	t, ok := hv.Inner.(T)
	if !ok {
		panic(fmt.Sprintf("gomputation: expected HostVal(%T), got HostVal(%T)", *new(T), hv.Inner))
	}
	return t
}
