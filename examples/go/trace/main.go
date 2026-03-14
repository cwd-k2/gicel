// Example: trace — evaluation trace hook for debugging.
//
// SetTraceHook installs a callback invoked before each evaluation step.
// The TraceEvent exposes the current depth and the Core IR node being
// evaluated. This is useful for understanding evaluation order and
// diagnosing performance issues. Returning a non-nil error from the
// hook aborts evaluation immediately.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cwd-k2/gicel"
)

const source = `
not := \b -> case b { True -> False; False -> True }

main := not True
`

func main() {
	eng := gicel.NewEngine()

	// Install a trace hook that logs each evaluation step.
	// The hook receives a TraceEvent with Depth (call depth) and
	// Node (the Core IR node about to be evaluated).
	// %T gives the Core IR node type, e.g. *core.App, *core.Case.
	var steps int
	eng.SetTraceHook(func(event gicel.TraceEvent) error {
		steps++
		indent := strings.Repeat("  ", event.Depth)
		tag := fmt.Sprintf("%T", event.Node)
		// Strip the package prefix for readability.
		if i := strings.LastIndex(tag, "."); i >= 0 {
			tag = tag[i+1:]
		}
		// Only print user-program steps (skip the many prelude elaboration steps).
		// In practice, most prelude steps happen before the entry point.
		// Here we print them all, but limit to 20 lines to keep output readable.
		if steps <= 20 {
			fmt.Printf("  step %2d %s%s\n", steps, indent, tag)
		}
		return nil
	})

	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	if steps > 20 {
		fmt.Printf("  ... (%d more steps)\n", steps-20)
	}
	fmt.Println()
	fmt.Println("result:", result.Value)
	fmt.Printf("total steps: %d, max depth: %d\n", result.Stats.Steps, result.Stats.MaxDepth)
	// Output (partial trace, then summary):
	// result: False
}
