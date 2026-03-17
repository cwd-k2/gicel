// Example: higher-order-prim — using the Applier to call GICEL functions from Go.
//
// When a PrimImpl receives a GICEL closure as an argument, it cannot
// call that closure directly. The Applier callback bridges the gap:
// it delegates back into the evaluator, threading the capability
// environment correctly. This is how higher-order primitives like
// foldl and map are implemented internally.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// myApply takes a GICEL function and a value, applies the function via
// the Applier callback, and returns the result.
const source = `
myApply :: (Int -> Int) -> Int -> Int
myApply := assumption

double :: Int -> Int
double := \x. x + x

main := myApply double 21
`

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// The fourth parameter (applier) is the key: it lets Go code invoke
	// GICEL closures. Here we apply fn to arg and return the result.
	eng.RegisterPrim("myApply", func(
		ctx context.Context,
		capEnv gicel.CapEnv,
		args []gicel.Value,
		applier gicel.Applier,
	) (gicel.Value, gicel.CapEnv, error) {
		fn := args[0]  // the GICEL function (a Closure value)
		arg := args[1] // the argument value

		// Applier calls back into the evaluator: fn(arg).
		// It returns the result value and the (possibly updated) CapEnv.
		result, newCapEnv, err := applier(fn, arg, capEnv)
		if err != nil {
			return nil, capEnv, err
		}
		return result, newCapEnv, nil
	})

	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	n := gicel.MustHost[int64](result.Value)
	fmt.Println("myApply double 21 =", n)
	// Output: myApply double 21 = 42
}
