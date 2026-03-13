package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gomputation/internal/eval"
)

// List provides list operations: fromSlice, toSlice, length, concat.
var List Pack = func(e Registrar) error {
	e.RegisterPrim("_listFromSlice", fromSliceImpl)
	e.RegisterPrim("_listToSlice", toSliceImpl)
	e.RegisterPrim("_listLength", lengthImpl)
	e.RegisterPrim("_listConcat", concatImpl)
	return e.RegisterModule("Std.List", listSource)
}

const listSource = `
import Prelude

_listFromSlice :: forall a. List a -> List a
_listFromSlice := assumption

_listToSlice :: forall a. List a -> List a
_listToSlice := assumption

_listLength :: forall a. List a -> Int
_listLength := assumption

_listConcat :: forall a. List a -> List a -> List a
_listConcat := assumption

fromSlice :: forall a. List a -> List a
fromSlice := _listFromSlice

toSlice :: forall a. List a -> List a
toSlice := _listToSlice

length :: forall a. List a -> Int
length := _listLength

concat :: forall a. List a -> List a -> List a
concat := _listConcat
`

// fromSliceImpl converts a HostVal([]any) to a ConVal chain (Cons/Nil).
// If the input is already a ConVal chain, it passes through unchanged.
func fromSliceImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	v := args[0]
	// If already a ConVal (Cons/Nil), pass through.
	if con, ok := v.(*eval.ConVal); ok {
		if con.Con == "Cons" || con.Con == "Nil" {
			return v, ce, nil
		}
	}
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, ce, fmt.Errorf("fromSlice: expected HostVal or List, got %T", v)
	}
	slice, ok := hv.Inner.([]any)
	if !ok {
		return nil, ce, fmt.Errorf("fromSlice: expected []any, got %T", hv.Inner)
	}
	return sliceToList(slice), ce, nil
}

// toSliceImpl converts a ConVal chain (Cons/Nil) to a HostVal([]any).
// If the input is already a HostVal([]any), it passes through unchanged.
func toSliceImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	v := args[0]
	if hv, ok := v.(*eval.HostVal); ok {
		if _, ok := hv.Inner.([]any); ok {
			return v, ce, nil
		}
	}
	items, ok := listToSlice(v)
	if !ok {
		return nil, ce, fmt.Errorf("toSlice: expected List (Cons/Nil), got %T", v)
	}
	return &eval.HostVal{Inner: items}, ce, nil
}

// lengthImpl counts the elements of a ConVal chain.
func lengthImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	n := int64(0)
	v := args[0]
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("length: expected List, got %T", v)
		}
		if con.Con == "Nil" {
			return &eval.HostVal{Inner: n}, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("length: malformed list node: %s", con.Con)
		}
		n++
		v = con.Args[1]
	}
}

// concatImpl concatenates two ConVal chain lists.
func concatImpl(_ context.Context, ce eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	xs, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, fmt.Errorf("concat: first argument is not a List")
	}
	ys := args[1]
	// Build result from the end: start with ys, prepend xs elements.
	result := ys
	for i := len(xs) - 1; i >= 0; i-- {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{xs[i].(eval.Value), result}}
	}
	return result, ce, nil
}

// sliceToList converts a Go []any to a ConVal chain.
func sliceToList(items []any) eval.Value {
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(eval.Value)
		if !ok {
			item = &eval.HostVal{Inner: items[i]}
		}
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{item, result}}
	}
	return result
}

// listToSlice converts a ConVal chain to a Go []any.
func listToSlice(v eval.Value) ([]any, bool) {
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
