// Example: resource-limits — protecting against runaway programs.
//
// Shows SetStepLimit and SetDepthLimit on the Engine.
//   - Step limit: prevents infinite loops (catches runaway iteration).
//   - Depth limit: bounds call nesting (defense-in-depth; the evaluator's
//     trampoline keeps depth near zero for GICEL-level recursion, so this
//     limit primarily guards against deep nesting from host callbacks).
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// An infinite loop via fix — will hit whichever limit is tightest.
const loopSource = `
import Prelude

loop :: Int -> Int
loop := fix (\self n. self (n + 1))
main := loop 0
`

func main() {
	ctx := context.Background()

	// --- Step limit: catches flat iteration ---
	eng := gicel.NewEngine()
	eng.EnableRecursion()
	eng.Use(gicel.Prelude)
	eng.SetStepLimit(500)
	eng.SetDepthLimit(10_000) // high enough to not interfere

	rt, err := eng.NewRuntime(context.Background(), loopSource)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	_, err = rt.RunWith(ctx, nil)
	fmt.Println("step limit test:", err)
	// Output: step limit test: step limit exceeded

	// --- Success with generous limits ---
	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	eng2.SetStepLimit(1_000_000)
	eng2.SetDepthLimit(1_000)

	rt2, err := eng2.NewRuntime(context.Background(), `
import Prelude

main := 1 + 2 + 3
`)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err := rt2.RunWith(ctx, nil)
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	val := gicel.MustHost[int64](result.Value)
	fmt.Println("success:", val)
	fmt.Printf("(steps: %d, max depth: %d)\n", result.Stats.Steps, result.Stats.MaxDepth)
	// Output: success: 6
}
