// Example: trace — evaluation trace hook for debugging.
//
// RunWith accepts a TraceHook that fires before each evaluation step.
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
not := \b. case b { True -> False; False -> True }

main := not True
`

func main() {
	eng := gicel.NewEngine()

	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	// Install a trace hook that logs each evaluation step.
	// The hook receives a TraceEvent with Depth (call depth) and
	// Node (the Core IR node about to be evaluated).
	// %T gives the Core IR node type, e.g. *core.App, *core.Case.
	var steps int
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{
		Trace: func(event gicel.TraceEvent) error {
			steps++
			indent := strings.Repeat("  ", event.Depth)
			tag := fmt.Sprintf("%T", event.Node)
			if i := strings.LastIndex(tag, "."); i >= 0 {
				tag = tag[i+1:]
			}
			if steps <= 20 {
				fmt.Printf("  step %2d %s%s\n", steps, indent, tag)
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
