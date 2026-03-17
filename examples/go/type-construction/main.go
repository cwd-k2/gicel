// Example: type-construction — building types from Go for the checker.
//
// The type construction API lets you declare complex types from Go
// using DeclareBinding and DeclareAssumption. This is useful when
// the type structure is driven by host-side configuration rather than
// hardcoded in GICEL source.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Num)

	// --- ConType: simple type constructor ---
	intTy := gicel.ConType("Int")

	// --- ArrowType: function type Int -> Int ---
	_ = gicel.ArrowType(intTy, intTy) // Int -> Int

	// --- ForallType: polymorphic types ---
	// \ a. a -> Maybe a
	wrapMaybeTy := gicel.ForallType("a",
		gicel.ArrowType(gicel.VarType("a"), gicel.AppType(gicel.ConType("Maybe"), gicel.VarType("a"))))

	// DeclareAssumption from Go (alternative to :: in source).
	eng.DeclareAssumption("wrapJust", wrapMaybeTy)
	eng.RegisterPrim("wrapJust", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return &gicel.ConVal{Con: "Just", Args: []gicel.Value{args[0]}}, capEnv, nil
	})

	// --- RowBuilder: record types ---
	// Record { x : Int, y : Int }
	pointRow := gicel.NewRow().
		And("x", intTy).
		And("y", intTy).
		Closed()
	pointTy := gicel.AppType(gicel.ConType("Record"), pointRow)

	eng.DeclareBinding("origin", pointTy)

	// --- ForallRow: row-polymorphic types ---
	// \ (r : Row). Record { x : Int | r } -> Int
	getXTy := gicel.ForallRow("r",
		gicel.ArrowType(
			gicel.AppType(gicel.ConType("Record"),
				gicel.NewRow().And("x", intTy).Open("r")),
			intTy))

	// The source uses wrapJust (declared from Go) and origin (bound from Go).
	// wrapJust := assumption is not needed in source — DeclareAssumption handles it.
	rt, err := eng.NewRuntime(`
import Std.Num

wrapJust := assumption

main := wrapJust (origin.#x + origin.#y)
`)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	bindings := map[string]gicel.Value{
		"origin": &gicel.RecordVal{Fields: map[string]gicel.Value{
			"x": gicel.ToValue(int64(3)),
			"y": gicel.ToValue(int64(4)),
		}},
	}

	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: bindings})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	fmt.Println("wrapJust (3 + 4) =", result.Value)
	// Output: wrapJust (3 + 4) = Just HostVal(7)

	// --- TypePretty: inspect constructed types ---
	fmt.Println("pointTy:", gicel.TypePretty(pointTy))
	fmt.Println("wrapMaybeTy:", gicel.TypePretty(wrapMaybeTy))
	fmt.Println("getXTy:", gicel.TypePretty(getXTy))
}
