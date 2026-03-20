package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// foldlImpl is a strict left fold that uses the Applier callback to apply closures.
func foldlImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]    // (b -> a -> b)
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

// sortByImpl sorts a list using merge sort with a user-supplied comparison function.
func sortByImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	items, ok := listToSlice(args[1])
	if !ok {
		return nil, ce, fmt.Errorf("sortBy: expected List, got %T", args[1])
	}
	if len(items) <= 1 {
		return args[1], ce, nil
	}
	if err := budget.ChargeAlloc(ctx, int64(len(items))*costConsNode); err != nil {
		return nil, ce, err
	}
	sorted, newCe, err := mergeSort(items, cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return buildList(sorted), newCe, nil
}

func mergeSort(items []eval.Value, cmp eval.Value, ce eval.CapEnv, apply eval.Applier) ([]eval.Value, eval.CapEnv, error) {
	if len(items) <= 1 {
		return items, ce, nil
	}
	mid := len(items) / 2
	left, ce, err := mergeSort(items[:mid], cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	right, ce, err := mergeSort(items[mid:], cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return mergeLists(left, right, cmp, ce, apply)
}

func mergeLists(left, right []eval.Value, cmp eval.Value, ce eval.CapEnv, apply eval.Applier) ([]eval.Value, eval.CapEnv, error) {
	result := make([]eval.Value, 0, len(left)+len(right))
	i, j := 0, 0
	for i < len(left) && j < len(right) {
		partial, newCe, err := apply(cmp, left[i], ce)
		if err != nil {
			return nil, ce, err
		}
		ordering, newCe, err := apply(partial, right[j], newCe)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		con, ok := ordering.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("sortBy: comparison must return Ordering, got %T", ordering)
		}
		if con.Con == "GT" {
			result = append(result, right[j])
			j++
		} else {
			result = append(result, left[i])
			i++
		}
	}
	result = append(result, left[i:]...)
	result = append(result, right[j:]...)
	return result, ce, nil
}

// scanlImpl is a left scan that collects all intermediate accumulator values.
func scanlImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	acc := args[1]
	list := args[2]
	var results []eval.Value
	results = append(results, acc)
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("scanl: expected List, got %T", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("scanl: malformed list node: %s", con.Con)
		}
		partial, newCe, err := apply(f, acc, ce)
		if err != nil {
			return nil, ce, err
		}
		acc, ce, err = apply(partial, con.Args[0], newCe)
		if err != nil {
			return nil, ce, err
		}
		results = append(results, acc)
		list = con.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, int64(len(results))*costConsNode); err != nil {
		return nil, ce, err
	}
	return buildList(results), ce, nil
}

// spanImpl splits a list into the longest prefix satisfying pred and the remainder.
func spanImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	list := args[1]
	var prefix []eval.Value
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("span: expected List, got %T", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("span: malformed list node: %s", con.Con)
		}
		result, newCe, err := apply(pred, con.Args[0], ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		boolCon, ok := result.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("span: predicate must return Bool, got %T", result)
		}
		if boolCon.Con == "False" {
			break
		}
		prefix = append(prefix, con.Args[0])
		list = con.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, int64(len(prefix))*costConsNode); err != nil {
		return nil, ce, err
	}
	return &eval.RecordVal{Fields: map[string]eval.Value{
		types.TupleLabel(1): buildList(prefix),
		types.TupleLabel(2): list,
	}}, ce, nil
}

// dropWhileImpl drops elements from the front while the predicate holds.
func dropWhileImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	list := args[1]
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("dropWhile: expected List, got %T", list)
		}
		if con.Con == "Nil" {
			return list, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("dropWhile: malformed list node: %s", con.Con)
		}
		result, newCe, err := apply(pred, con.Args[0], ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		boolCon, ok := result.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("dropWhile: predicate must return Bool, got %T", result)
		}
		if boolCon.Con == "False" {
			return list, ce, nil
		}
		list = con.Args[1]
	}
}

func zipImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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
		pairs = append(pairs, &eval.RecordVal{Fields: map[string]eval.Value{types.TupleLabel(1): xCon.Args[0], types.TupleLabel(2): yCon.Args[0]}})
		xs = xCon.Args[1]
		ys = yCon.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, int64(len(pairs))*(costTupleNode+costConsNode)); err != nil {
		return nil, ce, err
	}
	return buildList(pairs), ce, nil
}

func unzipImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	v := args[0]
	var as, bs []eval.Value
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("unzip: expected List, got %T", v)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("unzip: malformed list node")
		}
		rec, ok := con.Args[0].(*eval.RecordVal)
		if !ok {
			return nil, ce, fmt.Errorf("unzip: expected tuple")
		}
		a, ok1 := rec.Fields[types.TupleLabel(1)]
		b, ok2 := rec.Fields[types.TupleLabel(2)]
		if !ok1 || !ok2 {
			return nil, ce, fmt.Errorf("unzip: expected tuple with _1 and _2")
		}
		as = append(as, a)
		bs = append(bs, b)
		v = con.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, int64(len(as))*2*costConsNode); err != nil {
		return nil, ce, err
	}
	return &eval.RecordVal{Fields: map[string]eval.Value{types.TupleLabel(1): buildList(as), types.TupleLabel(2): buildList(bs)}}, ce, nil
}

// unfoldrImpl builds a list by repeatedly applying f to a seed until Nothing.
func unfoldrImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	seed := args[1]
	var items []eval.Value
	for {
		result, newCe, err := apply(f, seed, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		con, ok := result.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("unfoldr: expected Maybe, got %T", result)
		}
		if con.Con == "Nothing" {
			break
		}
		if con.Con != "Just" || len(con.Args) != 1 {
			return nil, ce, fmt.Errorf("unfoldr: malformed Maybe: %s", con.Con)
		}
		pair, ok := con.Args[0].(*eval.RecordVal)
		if !ok {
			return nil, ce, fmt.Errorf("unfoldr: expected tuple, got %T", con.Args[0])
		}
		a, ok1 := pair.Fields[types.TupleLabel(1)]
		b, ok2 := pair.Fields[types.TupleLabel(2)]
		if !ok1 || !ok2 {
			return nil, ce, fmt.Errorf("unfoldr: expected tuple with _1 and _2")
		}
		items = append(items, a)
		seed = b
	}
	if err := budget.ChargeAlloc(ctx, int64(len(items))*costConsNode); err != nil {
		return nil, ce, err
	}
	return buildList(items), ce, nil
}

// iterateNImpl generates a list of n elements by repeated application of f.
func iterateNImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	f := args[1]
	current := args[2]
	if n <= 0 {
		return &eval.ConVal{Con: "Nil"}, ce, nil
	}
	if err := budget.ChargeAlloc(ctx, n*costConsNode); err != nil {
		return nil, ce, err
	}
	items := make([]eval.Value, 0, n)
	for i := int64(0); i < n; i++ {
		items = append(items, current)
		if i < n-1 {
			next, newCe, err := apply(f, current, ce)
			if err != nil {
				return nil, ce, err
			}
			ce = newCe
			current = next
		}
	}
	return buildList(items), ce, nil
}
