// Example: modules — code reuse via the module system.
//
// RegisterModule(name, source) compiles a module and makes it importable.
// Modules can define data types, functions, and type class instances.
// The main program imports them with `import ModuleName`.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// A utility module that defines a custom data type and helper functions.
// The prelude (Bool, Maybe, etc.) is automatically available inside modules.
const utilsSource = `
data Pair a b = MkPair a b

mkPair :: forall a b. a -> b -> Pair a b
mkPair := \a -> \b -> MkPair a b

fstPair :: forall a b. Pair a b -> a
fstPair := \p -> case p { MkPair a _ -> a }

sndPair :: forall a b. Pair a b -> b
sndPair := \p -> case p { MkPair _ b -> b }

swapPair :: forall a b. Pair a b -> Pair b a
swapPair := \p -> case p { MkPair a b -> MkPair b a }
`

// The main program uses types and functions from the module.
const mainSource = `
import Utils

main := fstPair (swapPair (mkPair True False))
`

func main() {
	eng := gicel.NewEngine()

	// Register the module — it becomes importable as "Utils".
	if err := eng.RegisterModule("Utils", utilsSource); err != nil {
		log.Fatal("module compile error: ", err)
	}

	// Compile the main program that imports Utils.
	rt, err := eng.NewRuntime(mainSource)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	// swapPair (MkPair True False) => MkPair False True
	// fstPair (MkPair False True) => False
	fmt.Println("fstPair (swapPair (mkPair True False)) =", result.Value)
	// Output: fstPair (swapPair (mkPair True False)) = False
}
