// Example: capability — stateful operations via the capability environment.
//
// The capability environment (CapEnv) is a map[string]any that threads through
// evaluation with copy-on-write semantics. Primitives can read and write it,
// enabling stateful patterns without mutable variables in the language itself.
//
// This example implements a simple counter: getCounter reads the current count,
// incCounter increments it. The do-block sequences the operations.
//
// Note: type annotations are written in GICEL source via `::`,
// so the Go side only needs RegisterPrim (not DeclareAssumption).
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// The source declares types with :: annotations directly in the language.
// Assumptions with Computation return types can be sequenced with `<-` in do-blocks.
const source = `
getCounter: () -> Computation {} {} Int
getCounter := assumption

incCounter: () -> Computation {} {} ()
incCounter := assumption

main := do {
  incCounter ();
  incCounter ();
  getCounter ()
}
`

func main() {
	eng := gicel.NewEngine()

	// Register Int as an opaque host type.
	eng.RegisterType("Int", gicel.KindType())

	// Only RegisterPrim is needed — types are declared in the source via ::.

	// getCounter reads "counter" from CapEnv.
	eng.RegisterPrim("getCounter", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		v, _ := capEnv.Get("counter")
		n, _ := v.(int)
		return gicel.ToValue(n), capEnv, nil
	})

	// incCounter increments "counter" in CapEnv.
	eng.RegisterPrim("incCounter", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		v, _ := capEnv.Get("counter")
		n, _ := v.(int)
		return gicel.ToValue(nil), capEnv.Set("counter", n+1), nil
	})

	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	// Provide the initial capability environment with counter = 0.
	caps := map[string]any{"counter": 0}

	// RunWith returns the final CapEnv alongside the value.
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	// The returned value is the counter after two increments.
	counter := gicel.MustHost[int](result.Value)
	fmt.Println("counter value:", counter)
	// Output: counter value: 2

	// The final CapEnv also reflects the mutations.
	finalCount, _ := result.CapEnv.Get("counter")
	fmt.Println("capenv counter:", finalCount)
	// Output: capenv counter: 2
}
