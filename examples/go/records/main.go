// Example: records — Anonymous structural records and tuples in GICEL.
//
// Demonstrates record literals, projection (.#), update, row-polymorphic
// functions, tuples as sugar for records, and pattern matching on records.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// Records are anonymous structural types: Record { label: Type, ... }.
// Tuples are sugar: (a, b) ≡ Record { _1: a, _2: b }, () ≡ Record {}.
// Projection uses the .# operator (atom-level precedence).
// Row polymorphism lets functions accept any record with required fields.
const source = `
import Prelude

-- Record literal and projection
point := { x: 3, y: 4 }
px := point.#x

-- Record update (copy + overwrite)
moved := { point | x: 10 }

-- Row-polymorphic function: works on any record with an "x" field
getX :: \ r. Record { x: Int | r } -> Int
getX := \r. r.#x

-- Works with different record shapes
p1 := getX { x: 1, y: 2 }
p2 := getX { x: 5, label: "hello" }

-- Tuples: (a, b) desugars to Record { _1: a, _2: b }
pair := (True, 42)
first := fst pair
second := snd pair

-- Unit: () desugars to Record {}
unit := ()

-- Tuple Eq and Ord instances work out of the box
eqTest := eq (1, 2) (1, 2)
cmpTest := compare (1, 2) (1, 3)

-- Pattern matching on records
swap :: \ a b. (a, b) -> (b, a)
swap := \p. case p { (x, y) -> (y, x) }

-- Nested records
nested := { inner: { a: True, b: False }, tag: 42 }
deep := nested.#inner.#a

main := (px, (getX moved, (p1, (p2, (first, (eqTest, (cmpTest, deep)))))))
`

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	rt, err := eng.NewRuntime(source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	// Walk the result tuple
	p := result.Value
	labels := []string{"point.#x", "moved.#x", "p1", "p2", "fst pair", "eq test", "cmp test", "nested.#inner.#a"}
	for _, label := range labels {
		rv, ok := p.(*gicel.RecordVal)
		if !ok {
			fmt.Printf("%-18s = %v\n", label, p)
			break
		}
		fmt.Printf("%-18s = %v\n", label, rv.Fields["_1"])
		p = rv.Fields["_2"]
	}
	// Output:
	// point.#x           = HostVal(3)
	// moved.#x           = HostVal(10)
	// p1                 = HostVal(1)
	// p2                 = HostVal(5)
	// fst pair           = True
	// eq test            = True
	// cmp test           = LT
	// nested.#inner.#a   = True

	fmt.Printf("\n(steps: %d, max depth: %d)\n", result.Stats.Steps, result.Stats.MaxDepth)
}
