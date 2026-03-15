package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Stream provides lazy list operations: LCons/LNil data type,
// headS, tailS, toList, fromList, takeS, dropS,
// and Functor/Foldable instances.
var Stream Pack = func(e Registrar) error {
	e.RegisterPrim("_streamConst", streamConstImpl)
	e.RegisterPrim("_toListS", toListSImpl)
	e.RegisterPrim("_fromListS", fromListSImpl)
	e.RegisterPrim("_takeS", takeSImpl)
	e.RegisterPrim("_dropS", dropSImpl)
	e.RegisterPrim("_mapS", mapSImpl)
	e.RegisterPrim("_foldrS", foldrSImpl)
	return e.RegisterModule("Std.Stream", streamSource)
}

const streamSource = `
import Prelude

data Stream a = LCons a (() -> Stream a) | LNil

headS :: forall a. Stream a -> Maybe a
headS := \s -> case s { LNil -> Nothing; LCons x _ -> Just x }

tailS :: forall a. Stream a -> Maybe (Stream a)
tailS := \s -> case s { LNil -> Nothing; LCons _ t -> Just (t ()) }

_toListS :: forall a. Stream a -> List a
_toListS := assumption

_fromListS :: forall a. List a -> Stream a
_fromListS := assumption

_takeS :: forall a. Int -> Stream a -> List a
_takeS := assumption

_dropS :: forall a. Int -> Stream a -> Stream a
_dropS := assumption

_mapS :: forall a b. (a -> b) -> Stream a -> Stream b
_mapS := assumption

_foldrS :: forall a b. (a -> b -> b) -> b -> Stream a -> b
_foldrS := assumption

toList :: forall a. Stream a -> List a
toList := _toListS

fromList :: forall a. List a -> Stream a
fromList := _fromListS

takeS :: forall a. Int -> Stream a -> List a
takeS := _takeS

dropS :: forall a. Int -> Stream a -> Stream a
dropS := _dropS

mapS :: forall a b. (a -> b) -> Stream a -> Stream b
mapS := _mapS

instance Functor Stream { fmap := _mapS }
instance Foldable Stream { foldr := _foldrS }
`

// streamConstImpl implements _streamConst: a 2-arity primitive where
// the first arg is the captured value and the second is the ignored unit.
// Used to create constant thunks for stream tails.
func streamConstImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return args[0], ce, nil
}

// streamThunk creates a PrimVal that acts as `\_ -> val` by partially
// applying _streamConst with the captured value.
func streamThunk(val eval.Value) eval.Value {
	return &eval.PrimVal{
		Name:  "_streamConst",
		Arity: 2,
		Args:  []eval.Value{val},
	}
}

// forceTail calls the thunk `() -> Stream a` by applying it to Unit.
func forceTail(t eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	unit := &eval.RecordVal{Fields: map[string]eval.Value{}}
	return apply(t, unit, ce)
}

// buildStream creates an LCons/LNil chain from a slice.
// Each tail is a partially-applied _streamConst PrimVal.
func buildStream(items []eval.Value) eval.Value {
	var result eval.Value = &eval.ConVal{Con: "LNil"}
	for i := len(items) - 1; i >= 0; i-- {
		result = &eval.ConVal{Con: "LCons", Args: []eval.Value{
			items[i],
			streamThunk(result),
		}}
	}
	return result
}

func toListSImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	stream := args[0]
	var items []eval.Value
	for {
		con, ok := stream.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("toList: expected Stream, got %T", stream)
		}
		if con.Con == "LNil" {
			break
		}
		if con.Con != "LCons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("toList: malformed stream node: %s", con.Con)
		}
		items = append(items, con.Args[0])
		var err error
		stream, ce, err = forceTail(con.Args[1], ce, apply)
		if err != nil {
			return nil, ce, err
		}
	}
	return buildList(items), ce, nil
}

func fromListSImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	items, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, fmt.Errorf("fromList: expected List")
	}
	return buildStream(items), ce, nil
}

func takeSImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "stream")
	if err != nil {
		return nil, ce, err
	}
	stream := args[1]
	var items []eval.Value
	for i := int64(0); i < n; i++ {
		con, ok := stream.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("takeS: expected Stream, got %T", stream)
		}
		if con.Con == "LNil" {
			break
		}
		if con.Con != "LCons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("takeS: malformed stream node: %s", con.Con)
		}
		items = append(items, con.Args[0])
		stream, ce, err = forceTail(con.Args[1], ce, apply)
		if err != nil {
			return nil, ce, err
		}
	}
	return buildList(items), ce, nil
}

func dropSImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "stream")
	if err != nil {
		return nil, ce, err
	}
	stream := args[1]
	for i := int64(0); i < n; i++ {
		con, ok := stream.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("dropS: expected Stream, got %T", stream)
		}
		if con.Con == "LNil" {
			return stream, ce, nil
		}
		if con.Con != "LCons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("dropS: malformed stream node: %s", con.Con)
		}
		stream, ce, err = forceTail(con.Args[1], ce, apply)
		if err != nil {
			return nil, ce, err
		}
	}
	return stream, ce, nil
}

func mapSImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	stream := args[1]
	var items []eval.Value
	for {
		con, ok := stream.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("mapS: expected Stream, got %T", stream)
		}
		if con.Con == "LNil" {
			break
		}
		if con.Con != "LCons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("mapS: malformed stream node: %s", con.Con)
		}
		mapped, newCe, err := apply(f, con.Args[0], ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		items = append(items, mapped)
		stream, ce, err = forceTail(con.Args[1], ce, apply)
		if err != nil {
			return nil, ce, err
		}
	}
	return buildStream(items), ce, nil
}

func foldrSImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	z := args[1]
	stream := args[2]
	var items []eval.Value
	for {
		con, ok := stream.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("foldrS: expected Stream, got %T", stream)
		}
		if con.Con == "LNil" {
			break
		}
		if con.Con != "LCons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("foldrS: malformed stream node: %s", con.Con)
		}
		items = append(items, con.Args[0])
		var err error
		stream, ce, err = forceTail(con.Args[1], ce, apply)
		if err != nil {
			return nil, ce, err
		}
	}
	acc := z
	for i := len(items) - 1; i >= 0; i-- {
		partial, newCe, err := apply(f, items[i], ce)
		if err != nil {
			return nil, ce, err
		}
		acc, ce, err = apply(partial, acc, newCe)
		if err != nil {
			return nil, ce, err
		}
	}
	return acc, ce, nil
}
