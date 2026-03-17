// Example: custom-prelude — bare-bones and custom prelude modes.
//
// NoPrelude() disables all default types (Bool, Maybe, List, etc.).
// SetPrelude() replaces the default prelude with custom source while
// keeping CoreSource (IxMonad, Effect, Lift, then) intact.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

func main() {
	ctx := context.Background()

	// --- Part 1: NoPrelude — completely bare environment ---
	// Only CoreSource (IxMonad, Computation primitives) is available.
	// You must define your own data types.
	eng := gicel.NewEngine()
	eng.NoPrelude()

	rt, err := eng.NewRuntime(`
data Color = Red | Green | Blue

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

	// --- Part 2: SetPrelude — custom prelude with only what you need ---
	// CoreSource (IxMonad, bind, pure, etc.) is always prepended.
	// Your custom prelude replaces the default Bool/Maybe/List definitions.
	eng2 := gicel.NewEngine()
	eng2.SetPrelude(`
data Bit = On | Off

flip :: Bit -> Bit
flip := \b. case b { On -> Off; Off -> On }
`)

	rt2, err := eng2.NewRuntime(`
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
