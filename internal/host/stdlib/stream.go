package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Stream provides lazy list operations: LCons/LNil data type,
// headS, tailS, toList, fromList, takeS, dropS, mapS, foldrS,
// and Functor/Foldable instances.
//
// Recursive operations (toList, fromList, mapS, foldrS) are expressed
// as GICEL recursive definitions, requiring rec/fix. Only takeS/dropS
// remain as Go primitives due to Int arithmetic.
var Stream Pack = func(e Registrar) error {
	e.RegisterPrim("_takeS", takeSImpl)
	e.RegisterPrim("_dropS", dropSImpl)
	return e.RegisterModuleRec("Data.Stream", streamSource)
}

var streamSource = mustReadSource("stream")

// forceField forces a lazy co-data field. For VMThunkVal, the VM's apply
// handler forces the thunk (ignoring the argument). For legacy closures
// (() -> T), applies to Unit. For already-evaluated values, returns as-is.
func forceField(v eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	switch v.(type) {
	case *eval.VMThunkVal, *eval.ThunkVal, *eval.Closure, *eval.VMClosure:
		return apply(v, unitVal, ce)
	default:
		return v, ce, nil // already a value
	}
}

func takeSImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "stream")
	if err != nil {
		return nil, ce, err
	}
	stream := args[1]
	var items []eval.Value
	for i := int64(0); i < n; i++ {
		con, ok := stream.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("takeS", "Stream", stream)
		}
		if con.Con == "LNil" {
			break
		}
		if con.Con != "LCons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("takeS", "stream node", con.Con)
		}
		head, ce, err := forceField(con.Args[0], ce, apply)
		if err != nil {
			return nil, ce, err
		}
		items = append(items, head)
		stream, ce, err = forceField(con.Args[1], ce, apply)
		if err != nil {
			return nil, ce, err
		}
	}
	if err := budget.ChargeAlloc(ctx, int64(len(items))*costConsNode); err != nil {
		return nil, ce, err
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
			return nil, ce, errExpected("dropS", "Stream", stream)
		}
		if con.Con == "LNil" {
			return stream, ce, nil
		}
		if con.Con != "LCons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("dropS", "stream node", con.Con)
		}
		stream, ce, err = forceField(con.Args[1], ce, apply)
		if err != nil {
			return nil, ce, err
		}
	}
	return stream, ce, nil
}
