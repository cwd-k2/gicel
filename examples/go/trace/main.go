// Example: trace — evaluation trace hook for debugging.
//
// RunWith accepts a TraceHook that fires before each evaluation step.
// The TraceEvent exposes the current depth, the Core IR node kind (e.g.
// "Var", "App", "Case"), and a human-readable description. This is useful
// for understanding evaluation order and diagnosing performance issues.
// Returning a non-nil error from the hook aborts evaluation immediately.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cwd-k2/gicel"
)

const source = `
import Prelude

myNot := \b. case b { True => False; False => True }

main := myNot True
`

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	// Install a trace hook that logs each evaluation step.
	// The hook receives a TraceEvent with Depth (call depth),
	// NodeKind (e.g. "App", "Case"), and NodeDesc (human-readable).
	var steps int
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{
		Trace: func(event gicel.TraceEvent) error {
			steps++
			indent := strings.Repeat("  ", event.Depth)
			if steps <= 20 {
				fmt.Printf("  step %2d %s%s\n", steps, indent, event.NodeKind)
			}
			return nil
		},
	})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	if steps > 20 {
		fmt.Printf("  ... (%d more steps)\n", steps-20)
	}
	fmt.Println()
	fmt.Println("result:", gicel.PrettyValue(result.Value))
	fmt.Printf("total steps: %d, max depth: %d\n", result.Stats.Steps, result.Stats.MaxDepth)
}
