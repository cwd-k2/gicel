// Example: resource-limits — protecting against runaway programs.
//
// Shows SetStepLimit, SetDepthLimit, and SetAllocLimit on the Engine.
// Each limit targets a different abuse pattern:
//   - Step limit: prevents infinite loops (flat iteration).
//   - Depth limit: prevents stack overflow (deep recursion).
//   - Alloc limit: prevents memory exhaustion.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// An infinite loop via fix — will hit whichever limit is tightest.
const loopSource = `
import Std.Num
loop :: Int -> Int
loop := fix (\self -> \n -> self (n + 1))
main := loop 0
`

func main() {
	ctx := context.Background()

	// --- Step limit: catches flat iteration ---
	eng := gicel.NewEngine()
	eng.EnableRecursion()
	eng.Use(gicel.Num)
	eng.SetStepLimit(500)
	eng.SetDepthLimit(10_000) // high enough to not interfere

	rt, err := eng.NewRuntime(loopSource)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	_, err = rt.RunWith(ctx, nil)
	fmt.Println("step limit test:", err)
	// Output: step limit test: step limit exceeded

	// --- Depth limit: catches deep recursion ---
	eng2 := gicel.NewEngine()
	eng2.EnableRecursion()
	eng2.Use(gicel.Num)
	eng2.SetStepLimit(1_000_000) // high enough to not interfere
	eng2.SetDepthLimit(10)

	rt2, err := eng2.NewRuntime(loopSource)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	_, err = rt2.RunWith(ctx, nil)
	fmt.Println("depth limit test:", err)
	// Output: depth limit test: call depth limit exceeded

	// --- Success with generous limits ---
	eng3 := gicel.NewEngine()
	eng3.Use(gicel.Num)
	eng3.SetStepLimit(1_000_000)
	eng3.SetDepthLimit(1_000)

	rt3, err := eng3.NewRuntime(`
import Std.Num
main := 1 + 2 + 3
`)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err := rt3.RunWith(ctx, nil)
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	val := gicel.MustHost[int64](result.Value)
	fmt.Println("success:", val)
	fmt.Printf("(steps: %d, max depth: %d)\n", result.Stats.Steps, result.Stats.MaxDepth)
	// Output: success: 6
}
