// Example: list-interop — Go slice to GICEL list round-trip.
//
// Demonstrates ToList and FromList for converting between Go slices
// and GICEL's Cons/Nil list representation, plus manual construction
// and destructuring via FromCon. The Prelude provides list operations
// like foldl, reverse, and length that work on these lists.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// The source receives a list binding, reverses it, and returns the result.
const source = `
import Prelude

main := reverse xs
`

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Declare `xs` as a List of Int.
	eng.DeclareBinding("xs", gicel.AppType(gicel.ConType("List"), gicel.ConType("Int")))

	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	// Convert a Go slice to a GICEL list via ToList.
	// ToList wraps each element with ToValue, building a Cons/Nil chain.
	goSlice := []any{int64(10), int64(20), int64(30), int64(40)}
	gicelList := gicel.ToList(goSlice)

	bindings := map[string]gicel.Value{"xs": gicelList}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: bindings})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	// Convert the GICEL list back to a Go slice via FromList.
	// Each element comes back as a gicel.Value (here, *HostVal wrapping int64).
	items, ok := gicel.FromList(result.Value)
	if !ok {
		log.Fatal("expected a List result")
	}

	fmt.Print("reversed: [")
	for i, item := range items {
		if i > 0 {
			fmt.Print(", ")
		}
		n := gicel.MustHost[int64](item.(gicel.Value))
		fmt.Print(n)
	}
	fmt.Println("]")
	// Output: reversed: [40, 30, 20, 10]

	// Manual list construction and destructuring via FromCon.
	// Build Cons 1 (Cons 2 Nil) by hand.
	manual := &gicel.ConVal{Con: "Cons", Args: []gicel.Value{
		gicel.ToValue(int64(1)),
		&gicel.ConVal{Con: "Cons", Args: []gicel.Value{
			gicel.ToValue(int64(2)),
			&gicel.ConVal{Con: "Nil"},
		}},
	}}

	// Walk the Cons/Nil chain using FromCon.
	fmt.Print("manual:   [")
	cur := gicel.Value(manual)
	first := true
	for {
		name, args, ok := gicel.FromCon(cur)
		if !ok {
			break
		}
		if name == "Nil" {
			break
		}
		if !first {
			fmt.Print(", ")
		}
		first = false
		fmt.Print(gicel.MustHost[int64](args[0]))
		cur = args[1]
	}
	fmt.Println("]")
	// Output: manual:   [1, 2]
}
