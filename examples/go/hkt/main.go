// Example: hkt — Higher-Kinded Types in GICEL.
//
// Demonstrates kind-polymorphic type classes, kind variables in \
// binders, and poly-kinded instance resolution.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// The source uses the Prelude's Functor class and Maybe type, and adds
// a kind-polymorphic identity function to demonstrate HKT features.
// Key features demonstrated:
// - Kind sort (Kind) in \ binders: \ (k: Kind). ...
// - Kind variable references in kind annotations: \ (f: k -> Type). ...
// - Poly-kinded Functor from the Prelude (class Functor (f: k -> Type))
// - Kind unification during instance resolution
const source = `
import Prelude

-- Kind-polymorphic identity function
id_k :: \ (k: Kind). \ (a: k). a -> a
id_k := \x. x

-- Use fmap: (Bool -> Bool) -> Maybe Bool -> Maybe Bool
main := fmap (\b. case b { True => False; False => True }) (Just True)
`

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	fmt.Println("fmap not (Just True) =", result.Value)
	// Output: fmap not (Just True) = Just False

	fmt.Printf("(steps: %d, max depth: %d)\n", result.Stats.Steps, result.Stats.MaxDepth)
}
