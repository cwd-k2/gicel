// Solver integration tests — worklist+inert solver path via Engine.
// Does NOT cover: engine_test.go (legacy path).
package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/stdlib"
)

// solverTestCase defines a program to compile and optionally run.
type solverTestCase struct {
	name      string
	source    string
	checkOnly bool // check-only (no eval)
	packs     []stdlib.Pack
}

var solverTestCases = []solverTestCase{
	{name: "basic arithmetic", source: `import Prelude; main := 1 + 2`},
	{name: "operator chain", source: `import Prelude; main := 1 + 2 + 3 + 4 + 5`},
	{name: "long chain", source: `import Prelude; main := 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1`},
	{name: "string", source: `import Prelude; main := "hello"`},
	{name: "bool", source: `import Prelude; main := True`},
	{name: "comparison", source: `import Prelude; main := 1 == 2`},
	{name: "show", source: `import Prelude; main := show 42`},
	{name: "lambda", source: `import Prelude; main := (\x. x + 1) 5`},
	{name: "let generalization", source: `import Prelude; id := \x. x; main := id 1`},
	{name: "polymorphic", source: `import Prelude; id := \x. x; main := id (id 1)`},
	{name: "pair", source: `import Prelude; main := (1, True)`},
	{name: "list", source: `import Prelude; main := [1, 2, 3]`},
	{name: "case", source: `import Prelude; main := case True { True -> 1; False -> 0 }`},
	{name: "do notation", source: `
import Prelude
import Effect.State
main := thunk (do { put 0; x <- get; pure (x + 1) })`,
		packs: []stdlib.Pack{stdlib.State}},
	{name: "type annotation", source: `import Prelude; f :: Int -> Int; f := \x. x + 1; main := f 5`},
	{name: "higher order", source: `import Prelude; apply := \f. \x. f x; main := apply (\x. x + 1) 5`},
	{name: "class constraint", source: `
import Prelude
double :: Num a => a -> a
double := \x. x + x
main := double 5`},
	{name: "nested constraints", source: `
import Prelude
f :: (Eq a, Show a) => a -> String
f := \x. case x == x { True -> show x; False -> "no" }
main := f 42`},
	{name: "type family basic", source: `
import Prelude
type Id (a: Type) :: Type := { Id a =: a }
main := (1 :: Id Int) + 2`},
	{name: "ADT pattern match", source: `
import Prelude
data Color := Red | Green | Blue
name :: Color -> String
name := \c. case c { Red -> "red"; Green -> "green"; Blue -> "blue" }
main := name Red`},
	{name: "record", source: `
import Prelude
main := { x: 1, y: True }`},
	{name: "infix section", source: `import Prelude; main := (+ 1) 5`},
	{name: "composition", source: `import Prelude; main := ((\x. x + 1) . (\x. x * 2)) 3`},
	{name: "check only ADT", source: `
import Prelude
data Protocol := Done
main := Done`, checkOnly: true},
}

func TestSolverIntegration(t *testing.T) {
	for _, tc := range solverTestCases {
		t.Run(tc.name, func(t *testing.T) {
			eng := NewEngine()
			if err := eng.Use(stdlib.Prelude); err != nil {
				t.Fatal(err)
			}
			for _, p := range tc.packs {
				if err := eng.Use(p); err != nil {
					t.Fatal(err)
				}
			}
			if tc.checkOnly {
				_, err := eng.Compile(context.Background(), tc.source)
				if err != nil {
					t.Fatalf("compile failed: %v", err)
				}
				return
			}
			rt, err := eng.NewRuntime(context.Background(), tc.source)
			if err != nil {
				t.Fatalf("compile failed: %v", err)
			}
			_, err = rt.RunWith(context.Background(), nil)
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
		})
	}
}
