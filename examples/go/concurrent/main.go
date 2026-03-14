// Example: concurrent — goroutine-safe Runtime with concurrent execution.
//
// Runtime is immutable after compilation. Multiple goroutines can call
// RunContext on the same Runtime concurrently, each with its own bindings.
// Each execution gets an independent evaluator with its own step counter.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/cwd-k2/gicel"
)

// The source wraps a host-provided integer in Just.
const source = `
main := Just n
`

func main() {
	eng := gicel.NewEngine()
	eng.RegisterType("Int", gicel.KindType())
	eng.DeclareBinding("n", gicel.ConType("Int"))

	// Compile once.
	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	// Run from 5 goroutines concurrently, each with a different binding for n.
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()

			bindings := map[string]gicel.Value{
				"n": gicel.ToValue(val),
			}
			result, err := rt.RunContext(context.Background(), nil, bindings, "main")
			if err != nil {
				log.Printf("goroutine %d: runtime error: %v", val, err)
				return
			}

			_, args, ok := gicel.FromCon(result.Value)
			if !ok || len(args) != 1 {
				log.Printf("goroutine %d: unexpected result: %v", val, result.Value)
				return
			}

			inner := gicel.MustHost[int](args[0])
			fmt.Printf("goroutine %d: Just %d\n", val, inner)
		}(i * 10)
	}

	wg.Wait()
	fmt.Println("all goroutines completed")
}
