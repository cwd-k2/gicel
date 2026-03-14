// Example: reuse-runtime — running the same compiled program with different inputs.
//
// A Runtime is immutable and goroutine-safe. Once compiled, it can be
// executed many times with different host-provided bindings. Each call
// to RunContext is independent: bindings do not leak between executions.
// This pattern is useful for template-like evaluation or batch processing.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// The source references `n`, a host-provided binding of type Int.
// The program squares the input.
const source = `
import Std.Num

main := n * n
`

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Num)

	// Declare that `n` will be provided at runtime as an Int.
	eng.DeclareBinding("n", gicel.ConType("Int"))

	// Compile once.
	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	ctx := context.Background()

	// Run the same program with different values for n.
	for _, input := range []int64{3, 7, 12} {
		bindings := map[string]gicel.Value{
			"n": gicel.ToValue(input),
		}
		result, err := rt.RunContext(ctx, nil, bindings, "main")
		if err != nil {
			log.Fatal("runtime error: ", err)
		}
		output := gicel.MustHost[int64](result.Value)
		fmt.Printf("n=%2d -> n*n=%3d  (steps: %d)\n", input, output, result.Stats.Steps)
	}
	// Output:
	// n= 3 -> n*n=  9  (steps: ...)
	// n= 7 -> n*n= 49  (steps: ...)
	// n=12 -> n*n=144  (steps: ...)
}
