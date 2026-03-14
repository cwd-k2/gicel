// Example: hostbinding — injecting Go values into GICEL.
//
// GICEL has no literals. All data enters from the Go host via
// DeclareBinding (compile-time type declaration) and RunContext (run-time
// value provision). This example registers an opaque "Int" type, declares
// a binding of that type, and wraps it in a Maybe from GICEL source.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// The source references `n` — a host-provided variable of type Int.
// It wraps the value in Just (from the prelude: data Maybe a = Just a | Nothing).
const source = `
main := Just n
`

func main() {
	eng := gicel.NewEngine()

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

	result, err := rt.RunContext(context.Background(), nil, bindings, "main")
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
