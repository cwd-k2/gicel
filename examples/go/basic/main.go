// Example: basic — pure Bool logic in GICEL.
//
// Demonstrates the minimal lifecycle: Engine -> Runtime -> RunContext.
// No host bindings or assumptions; everything lives in the language.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// The source defines three functions using only the prelude Bool type.
// There are no literals — True and False are constructors provided by the prelude.
const source = `
not := \b -> case b { True -> False; False -> True }

and := \a -> \b -> case a { True -> b; False -> False }

main := and (not False) True
`

func main() {
	// 1. Create an engine with default limits.
	eng := gicel.NewEngine()

	// 2. Compile the source into an immutable, goroutine-safe Runtime.
	//    This performs lexing, parsing, type-checking, and elaboration to Core IR.
	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	// 3. Execute the "main" binding. No capabilities or host bindings needed here.
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	// 4. Extract the result. Bool values are ConVal with Con = "True" or "False".
	b, ok := gicel.FromBool(result.Value)
	if !ok {
		log.Fatal("expected a Bool value")
	}

	fmt.Println("and (not False) True =", b)
	// Output: and (not False) True = true

	fmt.Printf("(steps: %d, max depth: %d)\n", result.Stats.Steps, result.Stats.MaxDepth)
}
