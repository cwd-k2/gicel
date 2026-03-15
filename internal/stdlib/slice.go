package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Slice provides contiguous array operations: O(1) length/index,
// fromList/toList conversion, and Functor/Foldable/Semigroup/Monoid/Packed instances.
var Slice Pack = func(e Registrar) error {
	e.RegisterPrim("_sliceEmpty", sliceEmptyImpl)
	e.RegisterPrim("_sliceSingleton", sliceSingletonImpl)
	e.RegisterPrim("_sliceCons", sliceConsImpl)
	e.RegisterPrim("_sliceSnoc", sliceSnocImpl)
	e.RegisterPrim("_sliceLength", sliceLengthImpl)
	e.RegisterPrim("_sliceIndex", sliceIndexImpl)
	e.RegisterPrim("_sliceFromList", sliceFromListImpl)
	e.RegisterPrim("_sliceToList", sliceToListImpl)
	e.RegisterPrim("_sliceAppend", sliceAppendImpl)
	e.RegisterPrim("_sliceFoldr", sliceFoldrImpl)
	e.RegisterPrim("_sliceMap", sliceMapImpl)
	e.RegisterPrim("_sliceFoldl", sliceFoldlImpl)
	return e.RegisterModule("Std.Slice", sliceSource)
}

const sliceSource = `
import Prelude

_sliceEmpty :: forall a. Slice a
_sliceEmpty := assumption

_sliceSingleton :: forall a. a -> Slice a
_sliceSingleton := assumption

_sliceCons :: forall a. a -> Slice a -> Slice a
_sliceCons := assumption

_sliceSnoc :: forall a. Slice a -> a -> Slice a
_sliceSnoc := assumption

_sliceLength :: forall a. Slice a -> Int
_sliceLength := assumption

_sliceIndex :: forall a. Int -> Slice a -> Maybe a
_sliceIndex := assumption

_sliceFromList :: forall a. List a -> Slice a
_sliceFromList := assumption

_sliceToList :: forall a. Slice a -> List a
_sliceToList := assumption

_sliceAppend :: forall a. Slice a -> Slice a -> Slice a
_sliceAppend := assumption

_sliceFoldr :: forall a b. (a -> b -> b) -> b -> Slice a -> b
_sliceFoldr := assumption

_sliceMap :: forall a b. (a -> b) -> Slice a -> Slice b
_sliceMap := assumption

_sliceFoldl :: forall a b. (b -> a -> b) -> b -> Slice a -> b
_sliceFoldl := assumption

sliceEmpty :: forall a. Slice a
sliceEmpty := _sliceEmpty

sliceSingleton :: forall a. a -> Slice a
sliceSingleton := _sliceSingleton

sliceCons :: forall a. a -> Slice a -> Slice a
sliceCons := _sliceCons

sliceSnoc :: forall a. Slice a -> a -> Slice a
sliceSnoc := _sliceSnoc

sliceLength :: forall a. Slice a -> Int
sliceLength := _sliceLength

sliceIndex :: forall a. Int -> Slice a -> Maybe a
sliceIndex := _sliceIndex

sliceFromList :: forall a. List a -> Slice a
sliceFromList := _sliceFromList

sliceToList :: forall a. Slice a -> List a
sliceToList := _sliceToList

sliceAppend :: forall a. Slice a -> Slice a -> Slice a
sliceAppend := _sliceAppend

sliceFoldr :: forall a b. (a -> b -> b) -> b -> Slice a -> b
sliceFoldr := _sliceFoldr

sliceFoldl :: forall a b. (b -> a -> b) -> b -> Slice a -> b
sliceFoldl := _sliceFoldl

instance Functor Slice  { fmap := _sliceMap }
instance Foldable Slice { foldr := _sliceFoldr }
instance Semigroup (Slice a) { append := _sliceAppend }
instance Monoid (Slice a)    { empty  := _sliceEmpty }
instance Packed (Slice a) a  { pack := _sliceFromList; unpack := _sliceToList }
`

func asSlice(v eval.Value) ([]eval.Value, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, fmt.Errorf("stdlib/slice: expected HostVal, got %T", v)
	}
	s, ok := hv.Inner.([]eval.Value)
	if !ok {
		return nil, fmt.Errorf("stdlib/slice: expected []Value, got %T", hv.Inner)
	}
	return s, nil
}

func sliceVal(items []eval.Value) eval.Value {
	return &eval.HostVal{Inner: items}
}

func sliceEmptyImpl(_ context.Context, ce eval.CapEnv, _ []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return sliceVal([]eval.Value{}), ce, nil
}

func sliceSingletonImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return sliceVal([]eval.Value{args[0]}), ce, nil
}

func sliceConsImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, len(s)+1)
	result[0] = args[0]
	copy(result[1:], s)
	return sliceVal(result), ce, nil
}

func sliceSnocImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSlice(args[0])
	if err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, len(s)+1)
	copy(result, s)
	result[len(s)] = args[1]
	return sliceVal(result), ce, nil
}

func sliceLengthImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSlice(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(len(s))}, ce, nil
}

func sliceIndexImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	idx, err := asInt64(args[0], "slice")
	if err != nil {
		return nil, ce, err
	}
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	if idx < 0 || idx >= int64(len(s)) {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{s[idx]}}, ce, nil
}

func sliceFromListImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	items, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, fmt.Errorf("sliceFromList: expected List")
	}
	result := make([]eval.Value, len(items))
	copy(result, items)
	return sliceVal(result), ce, nil
}

func sliceToListImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSlice(args[0])
	if err != nil {
		return nil, ce, err
	}
	return buildList(s), ce, nil
}

func sliceAppendImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asSlice(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, len(a)+len(b))
	copy(result, a)
	copy(result[len(a):], b)
	return sliceVal(result), ce, nil
}

func sliceFoldrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	z := args[1]
	s, err := asSlice(args[2])
	if err != nil {
		return nil, ce, err
	}
	acc := z
	for i := len(s) - 1; i >= 0; i-- {
		partial, newCe, err := apply(f, s[i], ce)
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

func sliceMapImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, len(s))
	for i, item := range s {
		mapped, newCe, err := apply(f, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		result[i] = mapped
	}
	return sliceVal(result), ce, nil
}

func sliceFoldlImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	acc := args[1]
	s, err := asSlice(args[2])
	if err != nil {
		return nil, ce, err
	}
	for _, item := range s {
		partial, newCe, err := apply(f, acc, ce)
		if err != nil {
			return nil, ce, err
		}
		acc, ce, err = apply(partial, item, newCe)
		if err != nil {
			return nil, ce, err
		}
	}
	return acc, ce, nil
}
