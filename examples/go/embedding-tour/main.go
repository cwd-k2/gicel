// Example: embedding-tour — a compact tour of the Go embedding API.
//
// Demonstrates: Engine lifecycle, user module with `form`, Prelude,
// host-provided primitive via RegisterPrim, host binding injection,
// and result extraction including constructor and record values.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// A user module defining a stack as an algebraic data type.
// Uses `form` (the current keyword; `data` was renamed).
const stackModule = `
import Prelude

form Stack := \a. { Push: a -> Stack a -> Stack a; Empty: Stack a }

push :: \ a. a -> Stack a -> Stack a
push := \x s. Push x s

peek :: \ a. Stack a -> Maybe a
peek := \s. case s { Push x _ => Just x; Empty => Nothing }

isEmpty :: \ a. Stack a -> Bool
isEmpty := \s. case s { Push _ _ => False; Empty => True }
`

// The main program: builds a stack, peeks, and uses a host-injected value.
const mainSource = `
import Prelude
import Stack

-- Build a small stack from the bottom up.
s0 := push 10 (push 20 (push 30 Empty))

-- Peek at the top — returns Maybe Int.
top := peek s0

-- Check if the stack is empty.
empty := isEmpty s0

-- Use a host-injected value in arithmetic.
doubled := bonus + bonus

main := { top: top, empty: empty, bonus: bonus, doubled: doubled }
`

func main() {
	ctx := context.Background()

	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Register the Stack module.
	if err := eng.RegisterModule("Stack", stackModule); err != nil {
		log.Fatal("module compile error: ", err)
	}

	// Declare a host binding: `bonus` is an Int available at compile time.
	eng.DeclareBinding("bonus", gicel.ConType("Int"))

	// Compile.
	rt, err := eng.NewRuntime(ctx, mainSource)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	// Execute with a concrete value for `bonus`.
	bindings := map[string]gicel.Value{
		"bonus": gicel.ToValue(int64(7)),
	}

	result, err := rt.RunWith(ctx, &gicel.RunOptions{Bindings: bindings})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	// The result is a record { top, empty, bonus, doubled }.
	fields, ok := gicel.FromRecord(result.Value)
	if !ok {
		log.Fatal("expected a record value")
	}

	// top is Maybe Int — extract the constructor.
	topName, topArgs, _ := gicel.FromCon(fields["top"])
	if topName == "Just" {
		fmt.Printf("top     = Just %d\n", gicel.MustHost[int64](topArgs[0]))
	} else {
		fmt.Println("top     = Nothing")
	}

	// empty is a Bool constructor.
	empty, _ := gicel.FromBool(fields["empty"])
	fmt.Printf("empty   = %v\n", empty)

	bonus := gicel.MustHost[int64](fields["bonus"])
	doubled := gicel.MustHost[int64](fields["doubled"])
	fmt.Printf("bonus   = %d\n", bonus)
	fmt.Printf("doubled = %d  (bonus + bonus)\n", doubled)
	fmt.Printf("(steps: %d)\n", result.Stats.Steps)
	// Expected output:
	// top     = Just 10
	// empty   = false
	// bonus   = 7
	// doubled = 14  (bonus + bonus)
}
