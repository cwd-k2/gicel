package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/budget"
	"github.com/cwd-k2/gicel/internal/eval"
)

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
func toSliceImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
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
	if err := budget.ChargeAlloc(ctx, int64(len(items))*costSlotSize); err != nil {
		return nil, ce, err
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
func concatImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	xs, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, fmt.Errorf("concat: first argument is not a List")
	}
	if err := budget.ChargeAlloc(ctx, int64(len(xs))*costConsNode); err != nil {
		return nil, ce, err
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

func asInt64List(v eval.Value) (int64, error) { return asInt64(v, "list") }

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

func takeImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	if n <= 0 {
		return &eval.ConVal{Con: "Nil"}, ce, nil
	}
	v := args[1]
	var prefix []eval.Value
	for i := int64(0); i < n; i++ {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("take: expected List, got %T", v)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("take: malformed list node: %s", con.Con)
		}
		prefix = append(prefix, con.Args[0])
		v = con.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, int64(len(prefix))*costConsNode); err != nil {
		return nil, ce, err
	}
	return buildList(prefix), ce, nil
}

func dropImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	if n <= 0 {
		return args[1], ce, nil
	}
	v := args[1]
	for i := int64(0); i < n; i++ {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("drop: expected List, got %T", v)
		}
		if con.Con == "Nil" {
			return v, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("drop: malformed list node: %s", con.Con)
		}
		v = con.Args[1]
	}
	return v, ce, nil
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

func replicateImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	if n <= 0 {
		return &eval.ConVal{Con: "Nil"}, ce, nil
	}
	if err := budget.ChargeAlloc(ctx, n*costConsNode); err != nil {
		return nil, ce, err
	}
	elem := args[1]
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := int64(0); i < n; i++ {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{elem, result}}
	}
	return result, ce, nil
}

func reverseImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	// Pass 1: count elements and validate structure.
	v := args[0]
	var n int64
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("reverse: expected List, got %T", v)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("reverse: malformed list node: %s", con.Con)
		}
		n++
		v = con.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, n*costConsNode); err != nil {
		return nil, ce, err
	}
	// Pass 2: build reversed list via foldl-cons.
	v = args[0]
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := int64(0); i < n; i++ {
		con := v.(*eval.ConVal)
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{con.Args[0], result}}
		v = con.Args[1]
	}
	return result, ce, nil
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
		pairs = append(pairs, &eval.RecordVal{Fields: map[string]eval.Value{"_1": xCon.Args[0], "_2": yCon.Args[0]}})
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
		a, ok1 := rec.Fields["_1"]
		b, ok2 := rec.Fields["_2"]
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
		"_1": buildList(prefix),
		"_2": list,
	}}, ce, nil
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
		a, ok1 := pair.Fields["_1"]
		b, ok2 := pair.Fields["_2"]
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
