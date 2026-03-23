// Example: assumption — host-provided primitive operations.
//
// An assumption is a binding whose implementation lives in Go. The source
// declares `greet := assumption` with a `::` type annotation, and the host
// provides the actual function via RegisterPrim. This is how side effects
// (IO, network, etc.) are surfaced to the language in a controlled way.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// The source declares `greet` as an assumption with its type annotation
// written directly in the language via `::`.
const source = `
greet: () -> ()
greet := assumption

main := greet ()
`

func main() {
	eng := gicel.NewEngine()

	// Register the Go implementation.
	// PrimImpl receives context, the current capability environment, and arguments.
	// The type is declared in the source via ::, so DeclareAssumption is not needed.
	eng.RegisterPrim("greet", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		fmt.Println("Hello from Go!")
		return gicel.ToValue(nil), capEnv, nil // nil converts to () via ToValue
	})

	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	_, err = rt.RunWith(context.Background(), nil)
	if err != nil {
		log.Fatal("runtime error: ", err)
	}
	// Output: Hello from Go!
}
