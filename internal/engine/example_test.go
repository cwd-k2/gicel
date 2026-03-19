package engine

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/reg"
	"github.com/cwd-k2/gicel/internal/stdlib"
)

func ExampleRunSandbox() {
	result, err := RunSandbox(`
import Prelude
main := 2 + 3
`, &SandboxConfig{
		Packs: []reg.Pack{stdlib.Prelude},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(MustHost[int64](result.Value))
	// Output: 5
}

func ExampleEngine_NewRuntime() {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)

	rt, err := eng.NewRuntime(context.Background(), `
		import Prelude
		not := \b. case b { True -> False; False -> True }
		main := not False
	`)
	if err != nil {
		panic(err)
	}

	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		panic(err)
	}

	fmt.Println(result.Value)
	// Output: True
}

func ExampleCompileError_Diagnostics() {
	eng := NewEngine()
	_, err := eng.NewRuntime(context.Background(), `main := x`)
	if err != nil {
		ce := err.(*CompileError)
		for _, d := range ce.Diagnostics() {
			fmt.Printf("%s:%d:%d: %s\n", d.Phase, d.Line, d.Col, d.Message)
		}
	}
	// Output: check:1:9: unbound variable: x
}
