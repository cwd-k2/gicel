// Example: custom-prelude — bare-bones and custom prelude modes.
//
// NewEngine() returns a bare engine with no prelude loaded.
// You can define your own data types from scratch, or register
// a custom "Prelude" module with exactly the types you need.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

func main() {
	ctx := context.Background()

	// --- Part 1: No Prelude — completely bare environment ---
	// NewEngine() has no prelude by default; only built-in literal types
	// (Int, Double, String, Rune) are registered. You must define your
	// own data types.
	eng := gicel.NewEngine()

	rt, err := eng.NewRuntime(`
data Color := Red | Green | Blue

swap :: Color -> Color
swap := \c. case c { Red -> Blue; Blue -> Red; Green -> Green }

main := swap Red
`)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err := rt.RunWith(ctx, nil)
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	name, _, _ := gicel.FromCon(result.Value)
	fmt.Println("no-prelude:", name)
	// Output: no-prelude: Blue

	// --- Part 2: Custom Prelude — register a custom "Prelude" module ---
	// You can register any source as the "Prelude" module; user code
	// must explicitly write `import Prelude` to access it.
	eng2 := gicel.NewEngine()
	err = eng2.RegisterModule("Prelude", `
data Bit := On | Off

flip :: Bit -> Bit
flip := \b. case b { On -> Off; Off -> On }
`)
	if err != nil {
		log.Fatal("register prelude error: ", err)
	}

	rt2, err := eng2.NewRuntime(`
import Prelude
main := flip (flip Off)
`)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err = rt2.RunWith(ctx, nil)
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	name, _, _ = gicel.FromCon(result.Value)
	fmt.Println("custom prelude:", name)
	// Output: custom prelude: Off
}
