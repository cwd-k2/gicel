// Example: sandbox — single-call compile+execute via RunSandbox.
//
// The RunSandbox API is designed for AI agent scenarios: compile and run
// a GICEL program in one call with conservative resource limits.
// No Engine or Runtime lifecycle management is needed.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/cwd-k2/gicel"
)

func main() {
	// 1. Simplest case: default config (entry "main", 5s timeout, 100k steps).
	result, err := gicel.RunSandbox(`main := not False`, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("simple:", result.Value)
	// Output: simple: True

	// 2. With Packs and custom limits.
	result, err = gicel.RunSandbox(`
import Std.Num
main := (1 + 2) * 3
`, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Num},
		Timeout:  2 * time.Second,
		MaxSteps: 50_000,
		MaxDepth: 50,
		MaxAlloc: 1 << 20, // 1 MiB
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("arithmetic:", result.Value)
	fmt.Printf("(steps: %d, allocated: %d bytes)\n", result.Stats.Steps, result.Stats.Allocated)
	// Output: arithmetic: HostVal(9)

	// 3. Error case: compile error from invalid source.
	_, err = gicel.RunSandbox(`main := unknownVar`, nil)
	if err != nil {
		fmt.Println("expected error:", err)
	}
}
