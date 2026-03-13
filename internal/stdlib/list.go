package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gomputation/internal/eval"
)

// List provides list operations: fromSlice, toSlice, length, concat, foldl,
// take, drop, index, replicate, reverse, zip, unzip.
var List Pack = func(e Registrar) error {
	e.RegisterPrim("_listFromSlice", fromSliceImpl)
	e.RegisterPrim("_listToSlice", toSliceImpl)
	e.RegisterPrim("_listLength", lengthImpl)
	e.RegisterPrim("_listConcat", concatImpl)
	e.RegisterPrim("_listFoldl", foldlImpl)
	e.RegisterPrim("_listTake", takeImpl)
	e.RegisterPrim("_listDrop", dropImpl)
	e.RegisterPrim("_listIndex", indexImpl)
	e.RegisterPrim("_listReplicate", replicateImpl)
	e.RegisterPrim("_listReverse", reverseImpl)
	e.RegisterPrim("_listZip", zipImpl)
	e.RegisterPrim("_listUnzip", unzipImpl)
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

_listFoldl :: forall a b. (b -> a -> b) -> b -> List a -> b
_listFoldl := assumption

_listTake :: forall a. Int -> List a -> List a
_listTake := assumption

_listDrop :: forall a. Int -> List a -> List a
_listDrop := assumption

_listIndex :: forall a. Int -> List a -> Maybe a
_listIndex := assumption

_listReplicate :: forall a. Int -> a -> List a
_listReplicate := assumption

_listReverse :: forall a. List a -> List a
_listReverse := assumption

_listZip :: forall a b. List a -> List b -> List (a, b)
_listZip := assumption

_listUnzip :: forall a b. List (a, b) -> (List a, List b)
_listUnzip := assumption

foldl :: forall a b. (b -> a -> b) -> b -> List a -> b
foldl := _listFoldl

take :: forall a. Int -> List a -> List a
take := _listTake

drop :: forall a. Int -> List a -> List a
drop := _listDrop

index :: forall a. Int -> List a -> Maybe a
index := _listIndex

replicate :: forall a. Int -> a -> List a
replicate := _listReplicate

reverse :: forall a. List a -> List a
reverse := _listReverse

zip :: forall a b. List a -> List b -> List (a, b)
zip := _listZip

unzip :: forall a b. List (a, b) -> (List a, List b)
unzip := _listUnzip
`

// fromSliceImpl converts a HostVal([]any) to a ConVal chain (Cons/Nil).
// If the input is already a ConVal chain, it passes through unchanged.
func fromSliceImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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
func toSliceImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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
	anys := make([]any, len(items))
	for i, item := range items {
		anys[i] = item
	}
	return &eval.HostVal{Inner: anys}, ce, nil
}

// lengthImpl counts the elements of a ConVal chain.
func lengthImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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
func concatImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	xs, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, fmt.Errorf("concat: first argument is not a List")
	}
	ys := args[1]
	// Build result from the end: start with ys, prepend xs elements.
	result := ys
	for i := len(xs) - 1; i >= 0; i-- {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{xs[i], result}}
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

// listToSlice converts a ConVal chain to a Go []eval.Value.
func listToSlice(v eval.Value) ([]eval.Value, bool) {
	var result []eval.Value
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

// --- New List primitives ---

func asInt64List(v eval.Value) (int64, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return 0, fmt.Errorf("stdlib/list: expected HostVal(int64), got %T", v)
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		return 0, fmt.Errorf("stdlib/list: expected int64, got %T", hv.Inner)
	}
	return n, nil
}

// foldlImpl is a strict left fold that uses the Applier callback to apply closures.
func foldlImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]   // (b -> a -> b)
	acc := args[1]  // b
	list := args[2] // List a
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("foldl: expected List, got %T", list)
		}
		if con.Con == "Nil" {
			return acc, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("foldl: malformed list node: %s", con.Con)
		}
		// acc = f acc x
		partial, newCe, err := apply(f, acc, ce)
		if err != nil {
			return nil, ce, err
		}
		acc, ce, err = apply(partial, con.Args[0], newCe)
		if err != nil {
			return nil, ce, err
		}
		list = con.Args[1]
	}
}

func takeImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	items, ok := listToSlice(args[1])
	if !ok {
		return nil, ce, fmt.Errorf("take: expected List")
	}
	if n < 0 {
		n = 0
	}
	if int64(len(items)) < n {
		n = int64(len(items))
	}
	return buildList(items[:n]), ce, nil
}

func dropImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	items, ok := listToSlice(args[1])
	if !ok {
		return nil, ce, fmt.Errorf("drop: expected List")
	}
	if n < 0 {
		n = 0
	}
	if int64(len(items)) < n {
		n = int64(len(items))
	}
	return buildList(items[n:]), ce, nil
}

func indexImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	idx, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	v := args[1]
	var i int64
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("index: expected List")
		}
		if con.Con == "Nil" {
			return &eval.ConVal{Con: "Nothing"}, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("index: malformed list")
		}
		if i == idx {
			return &eval.ConVal{Con: "Just", Args: []eval.Value{con.Args[0]}}, ce, nil
		}
		i++
		v = con.Args[1]
	}
}

func replicateImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	elem := args[1]
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := int64(0); i < n; i++ {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{elem, result}}
	}
	return result, ce, nil
}

func reverseImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	items, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, fmt.Errorf("reverse: expected List")
	}
	reversed := make([]eval.Value, len(items))
	for i, item := range items {
		reversed[len(items)-1-i] = item
	}
	return buildList(reversed), ce, nil
}

func zipImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	xs := args[0]
	ys := args[1]
	var pairs []eval.Value
	for {
		xCon, ok := xs.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("zip: expected List for first arg")
		}
		yCon, ok := ys.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("zip: expected List for second arg")
		}
		if xCon.Con == "Nil" || yCon.Con == "Nil" {
			break
		}
		if xCon.Con != "Cons" || len(xCon.Args) != 2 || yCon.Con != "Cons" || len(yCon.Args) != 2 {
			return nil, ce, fmt.Errorf("zip: malformed list")
		}
		pairs = append(pairs, &eval.RecordVal{Fields: map[string]eval.Value{"_1": xCon.Args[0], "_2": yCon.Args[0]}})
		xs = xCon.Args[1]
		ys = yCon.Args[1]
	}
	return buildList(pairs), ce, nil
}

func unzipImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	items, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, fmt.Errorf("unzip: expected List")
	}
	as := make([]eval.Value, len(items))
	bs := make([]eval.Value, len(items))
	for i, item := range items {
		rec, ok := item.(*eval.RecordVal)
		if !ok {
			return nil, ce, fmt.Errorf("unzip: expected tuple at index %d", i)
		}
		a, ok1 := rec.Fields["_1"]
		b, ok2 := rec.Fields["_2"]
		if !ok1 || !ok2 {
			return nil, ce, fmt.Errorf("unzip: expected tuple with _1 and _2 at index %d", i)
		}
		as[i] = a
		bs[i] = b
	}
	return &eval.RecordVal{Fields: map[string]eval.Value{"_1": buildList(as), "_2": buildList(bs)}}, ce, nil
}

// buildList creates a ConVal Cons/Nil chain from a slice.
func buildList(items []eval.Value) eval.Value {
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := len(items) - 1; i >= 0; i-- {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{items[i], result}}
	}
	return result
}
