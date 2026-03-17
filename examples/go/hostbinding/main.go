// Example: hostbinding — injecting Go values into GICEL.
//
// This example registers an opaque host type and injects a Go value
// via DeclareBinding (compile-time type) and RunWith (run-time value).
// It uses a host-registered "Int" rather than the Prelude's integer type
// to demonstrate the host binding mechanism.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// The source references `n` — a host-provided variable of type Int.
// It wraps the value in Just (from the prelude: data Maybe a := Just a | Nothing).
const source = `
import Prelude

main := Just n
`

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Register "Int" as an opaque host type (kind: Type).
	eng.RegisterType("Int", gicel.KindType())

	// Declare that `n` exists at type Int.
	// The checker will verify all uses of `n` are consistent with this type.
	eng.DeclareBinding("n", gicel.ConType("Int"))

	// Compile.
	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	// Provide the actual value for `n` at runtime.
	// The same Runtime can be executed many times with different values.
	bindings := map[string]gicel.Value{
		"n": gicel.ToValue(42),
	}

	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: bindings})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	// The result is (Just HostVal(42)).
	// Destructure the ConVal to reach the inner Go value.
	name, args, ok := gicel.FromCon(result.Value)
	if !ok || name != "Just" || len(args) != 1 {
		log.Fatal("expected Just <value>")
	}

	inner := gicel.MustHost[int](args[0])
	fmt.Printf("Just %d\n", inner)
	// Output: Just 42
}
