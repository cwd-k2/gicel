// Example: multi-entry — evaluating different entry points from a single Runtime.
//
// A compiled Runtime contains all top-level bindings. By passing different
// entry point names to RunWith, different parts of the same program can
// be evaluated without recompilation. This is useful for programs that
// expose multiple named results or services.
//
// Note: when an entry point is selected, all other bindings are evaluated
// first as non-entry bindings. Bindings that reference the entry point
// by name will fail, so each potential entry point should be self-contained
// (shared helpers are fine since they stay in the non-entry set).
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// The source defines a shared helper (square) and three independent entry
// points that each use the helper but do not depend on each other.
const source = `
import Prelude

square :: Int -> Int
square := \x. x * x

greeting := "hello, GICEL"

answer := square 6 + 6

doubled := square 8
`

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Compile once.
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	ctx := context.Background()

	// Evaluate "greeting" — a String value.
	r1, err := rt.RunWith(ctx, &gicel.RunOptions{Entry: "greeting"})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}
	fmt.Println("greeting:", gicel.MustHost[string](r1.Value))
	// Output: greeting: hello, GICEL

	// Evaluate "answer" — an Int value.
	r2, err := rt.RunWith(ctx, &gicel.RunOptions{Entry: "answer"})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}
	fmt.Println("answer:  ", gicel.MustHost[int64](r2.Value))
	// Output: answer:   42

	// Evaluate "doubled" — another Int value using the same helper.
	r3, err := rt.RunWith(ctx, &gicel.RunOptions{Entry: "doubled"})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}
	fmt.Println("doubled: ", gicel.MustHost[int64](r3.Value))
	// Output: doubled:  64
}
