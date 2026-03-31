package stdlib

import (
	"context"
	"errors"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// listMapImpl maps a function over a Cons/Nil list, returning a new list.
// _listMap :: (a -> b) -> List a -> List b
func listMapImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	items, ok := listToSlice(args[1])
	if !ok {
		return nil, ce, errExpected("listMap", "List", args[1])
	}
	if err := budget.ChargeAlloc(ctx, int64(len(items))*costConsNode); err != nil {
		return nil, ce, err
	}
	mapped := make([]eval.Value, len(items))
	for i, item := range items {
		result, newCe, err := apply(f, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		mapped[i] = result
	}
	return buildList(mapped), ce, nil
}

// listFoldrImpl is a right fold over a Cons/Nil list.
// _listFoldr :: (a -> b -> b) -> b -> List a -> b
func listFoldrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	z := args[1]
	items, ok := listToSlice(args[2])
	if !ok {
		return nil, ce, errExpected("listFoldr", "List", args[2])
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

// foldlImpl is a strict left fold that uses the Applier callback to apply closures.
func foldlImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]    // (b -> a -> b)
	acc := args[1]  // b
	list := args[2] // List a
	var i int64
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("foldl", "List", list)
		}
		if con.Con == "Nil" {
			return acc, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("foldl", "list node", con.Con)
		}
		if i&1023 == 0 {
			if err := budget.CheckContext(ctx); err != nil {
				return nil, ce, err
			}
		}
		i++
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
		return nil, ce, errExpected("sortBy", "List", args[1])
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
			return nil, ce, errExpected("sortBy", "Ordering", ordering)
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
			return nil, ce, errExpected("scanl", "List", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("scanl", "list node", con.Con)
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
			return nil, ce, errExpected("span", "List", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("span", "list node", con.Con)
		}
		result, newCe, err := apply(pred, con.Args[0], ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		boolCon, ok := result.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("span", "Bool", result)
		}
		if boolCon.Con == eval.BoolFalse {
			break
		}
		prefix = append(prefix, con.Args[0])
		list = con.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, int64(len(prefix))*costConsNode); err != nil {
		return nil, ce, err
	}
	return eval.NewRecordFromMap(map[string]eval.Value{
		ir.TupleLabel(1): buildList(prefix),
		ir.TupleLabel(2): list,
	}), ce, nil
}

// dropWhileImpl drops elements from the front while the predicate holds.
func dropWhileImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	list := args[1]
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("dropWhile", "List", list)
		}
		if con.Con == "Nil" {
			return list, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("dropWhile", "list node", con.Con)
		}
		result, newCe, err := apply(pred, con.Args[0], ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		boolCon, ok := result.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("dropWhile", "Bool", result)
		}
		if boolCon.Con == eval.BoolFalse {
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
			return nil, ce, errors.New("zip: expected List for first arg")
		}
		yCon, ok := ys.(*eval.ConVal)
		if !ok {
			return nil, ce, errors.New("zip: expected List for second arg")
		}
		if xCon.Con == "Nil" || yCon.Con == "Nil" {
			break
		}
		if xCon.Con != "Cons" || len(xCon.Args) != 2 || yCon.Con != "Cons" || len(yCon.Args) != 2 {
			return nil, ce, errMalformed("zip", "list node", xCon.Con+"/"+yCon.Con)
		}
		pairs = append(pairs, eval.NewRecordFromMap(map[string]eval.Value{ir.TupleLabel(1): xCon.Args[0], ir.TupleLabel(2): yCon.Args[0]}))
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
			return nil, ce, errExpected("unzip", "List", v)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("unzip", "list node", con.Con)
		}
		rec, ok := con.Args[0].(*eval.RecordVal)
		if !ok {
			return nil, ce, errors.New("unzip: expected tuple")
		}
		a, ok1 := rec.Get(ir.TupleLabel(1))
		b, ok2 := rec.Get(ir.TupleLabel(2))
		if !ok1 || !ok2 {
			return nil, ce, errors.New("unzip: expected tuple with _1 and _2")
		}
		as = append(as, a)
		bs = append(bs, b)
		v = con.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, int64(len(as))*2*costConsNode); err != nil {
		return nil, ce, err
	}
	return eval.NewRecordFromMap(map[string]eval.Value{ir.TupleLabel(1): buildList(as), ir.TupleLabel(2): buildList(bs)}), ce, nil
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
			return nil, ce, errExpected("unfoldr", "Maybe", result)
		}
		if con.Con == "Nothing" {
			break
		}
		if con.Con != "Just" || len(con.Args) != 1 {
			return nil, ce, errMalformed("unfoldr", "Maybe", con.Con)
		}
		pair, ok := con.Args[0].(*eval.RecordVal)
		if !ok {
			return nil, ce, errExpected("unfoldr", "tuple", con.Args[0])
		}
		a, ok1 := pair.Get(ir.TupleLabel(1))
		b, ok2 := pair.Get(ir.TupleLabel(2))
		if !ok1 || !ok2 {
			return nil, ce, errors.New("unfoldr: expected tuple with _1 and _2")
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

// groupByImpl groups consecutive elements by a user-supplied equality predicate.
// _listGroupBy :: (a -> a -> Bool) -> List a -> List (List a)
func groupByImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	eq := args[0]
	list := args[1]
	var groups []eval.Value
	var current []eval.Value
	var currentKey eval.Value
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("groupBy", "List", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("groupBy", "list node", con.Con)
		}
		elem := con.Args[0]
		if currentKey == nil {
			current = []eval.Value{elem}
			currentKey = elem
		} else {
			partial, newCe, err := apply(eq, currentKey, ce)
			if err != nil {
				return nil, ce, err
			}
			result, newCe2, err := apply(partial, elem, newCe)
			if err != nil {
				return nil, ce, err
			}
			ce = newCe2
			boolCon, ok := result.(*eval.ConVal)
			if !ok {
				return nil, ce, errExpected("groupBy", "Bool", result)
			}
			if boolCon.Con == eval.BoolTrue {
				current = append(current, elem)
			} else {
				groups = append(groups, buildList(current))
				current = []eval.Value{elem}
				currentKey = elem
			}
		}
		list = con.Args[1]
	}
	if len(current) > 0 {
		groups = append(groups, buildList(current))
	}
	if err := budget.ChargeAlloc(ctx, int64(len(groups))*costConsNode); err != nil {
		return nil, ce, err
	}
	return buildList(groups), ce, nil
}
