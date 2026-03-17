// Example: error-handling — structured compile and runtime error handling.
//
// Shows how to use errors.As to distinguish CompileError from RuntimeError,
// and how to iterate structured Diagnostics for programmatic error reporting.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

func main() {
	// --- Part 1: Compile error with structured diagnostics ---
	eng := gicel.NewEngine()

	// Intentionally broken source: undefined variable.
	_, err := eng.NewRuntime(`main := undefinedVar`)
	if err != nil {
		var ce *gicel.CompileError
		if errors.As(err, &ce) {
			fmt.Println("compile error caught:")
			for _, d := range ce.Diagnostics() {
				fmt.Printf("  [%s] line %d, col %d: %s\n", d.Phase, d.Line, d.Col, d.Message)
			}
		}
	}
	fmt.Println()

	// --- Part 2: Runtime error from step limit exceeded ---
	// EnableRecursion + fix creates a true infinite loop that compiles
	// but exceeds the step limit at runtime.
	eng2 := gicel.NewEngine()
	eng2.EnableRecursion()
	eng2.SetStepLimit(500) // low enough to catch the infinite loop

	rt, err := eng2.NewRuntime(`
loop :: Bool -> Bool
loop := fix (\self b. self b)
main := loop True
`)
	if err != nil {
		log.Fatal("unexpected compile error: ", err)
	}

	_, err = rt.RunWith(context.Background(), nil)
	if err != nil {
		fmt.Println("runtime error caught:", err)
	}

	// --- Part 3: Successful run for contrast ---
	eng3 := gicel.NewEngine()
	rt, err = eng3.NewRuntime(`main := not False`)
	if err != nil {
		log.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nsuccess:", result.Value)
	// Output: success: True
}
