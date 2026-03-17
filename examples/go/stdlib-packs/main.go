// Example: stdlib-packs — demonstrating standard library packs.
//
// GICEL provides packs via eng.Use(): Prelude (arithmetic, strings, lists),
// EffectFail, EffectState, EffectIO, DataStream, DataSlice, DataMap, DataSet.
// Each pack registers primitive operations and a GICEL module that the source
// can import. This example compiles and runs a small program for several packs.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

func main() {
	ctx := context.Background()

	// --- 1. Prelude: arithmetic, strings, lists ---
	run(ctx, "Prelude-Num", func(eng *gicel.Engine) { eng.Use(gicel.Prelude) }, `
main := (3 + 4) * 2
`, func(v gicel.Value) {
		fmt.Println("Num:   (3+4)*2 =", gicel.MustHost[int64](v))
	})

	// --- 2. Prelude includes Str operations ---
	run(ctx, "Prelude-Str", func(eng *gicel.Engine) { eng.Use(gicel.Prelude) }, `
main := toUpper (append "hello" " world")
`, func(v gicel.Value) {
		fmt.Println("Str:   toUpper =", gicel.MustHost[string](v))
	})

	// --- 3. Prelude includes List operations ---
	run(ctx, "Prelude-List", func(eng *gicel.Engine) {
		eng.Use(gicel.Prelude)
		eng.EnableRecursion()
	}, `
main := foldl (+) 0 [1, 2, 3]
`, func(v gicel.Value) {
		fmt.Println("List:  foldl (+) =", gicel.MustHost[int64](v))
	})

	// --- 4. Fail: error signaling via the fail effect ---
	runFail(ctx)

	// --- 5. State: get/put/modify via CapEnv ---
	runState(ctx)

	// --- 6. IO: print to CapEnv buffer ---
	runIO(ctx)
}

// run compiles and executes a source snippet, applying packs via setup.
func run(ctx context.Context, label string, setup func(*gicel.Engine), source string, inspect func(gicel.Value)) {
	eng := gicel.NewEngine()
	setup(eng)
	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatalf("%s compile error: %v", label, err)
	}
	result, err := rt.RunWith(ctx, nil)
	if err != nil {
		log.Fatalf("%s runtime error: %v", label, err)
	}
	inspect(result.Value)
}

// runFail demonstrates the Fail pack. failWith triggers a RuntimeError.
func runFail(ctx context.Context) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectFail)

	rt, err := eng.NewRuntime(`
import Effect.Fail
main := do { _ <- failWith (); pure True }
`)
	if err != nil {
		log.Fatal("Fail compile error: ", err)
	}

	_, err = rt.RunWith(ctx, &gicel.RunOptions{Caps: map[string]any{"fail": nil}})
	if err != nil {
		// The error wraps a RuntimeError from the fail primitive.
		var re *gicel.RuntimeError
		if errors.As(err, &re) {
			fmt.Println("Fail:  caught RuntimeError:", re.Message)
		} else {
			fmt.Println("Fail:  error:", err)
		}
	}
}

// runState demonstrates the State pack: get, put, modify.
func runState(ctx context.Context) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)

	rt, err := eng.NewRuntime(`
import Effect.State
main := do {
  s <- get;
  put (s + 10);
  get
}
`)
	if err != nil {
		log.Fatal("State compile error: ", err)
	}

	// State pack reads/writes the "state" key in CapEnv.
	// The value must be a gicel.Value (not a raw Go value).
	caps := map[string]any{"state": gicel.ToValue(int64(32))}
	result, err := rt.RunWith(ctx, &gicel.RunOptions{Caps: caps})
	if err != nil {
		log.Fatal("State runtime error: ", err)
	}

	fmt.Println("State: get after put =", gicel.MustHost[int64](result.Value))
}

// runIO demonstrates the IO pack: print accumulates into the CapEnv buffer.
func runIO(ctx context.Context) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectIO)

	rt, err := eng.NewRuntime(`
import Effect.IO
main := do {
  _ <- print "line 1";
  print "line 2"
}
`)
	if err != nil {
		log.Fatal("IO compile error: ", err)
	}

	// IO pack reads/writes the "io" key in CapEnv as []string.
	caps := map[string]any{"io": nil}
	result, err := rt.RunWith(ctx, &gicel.RunOptions{Caps: caps})
	if err != nil {
		log.Fatal("IO runtime error: ", err)
	}

	// Retrieve the accumulated output from the final CapEnv.
	buf, _ := result.CapEnv.Get("io")
	lines := buf.([]string)
	for _, line := range lines {
		fmt.Println("IO:   ", line)
	}
}

// Output:
// Num:   (3+4)*2 = 14
// Str:   toUpper = HELLO WORLD
// List:  foldl (+) = 6
// Fail:  caught RuntimeError: fail
// State: get after put = 42
// IO:    line 1
// IO:    line 2
