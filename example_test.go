package gicel_test

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel"
)

func ExampleRunSandbox() {
	result, err := gicel.RunSandbox(`
import Std.Num
main := 2 + 3
`, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Num},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(gicel.MustHost[int64](result.Value))
	// Output: 5
}

func ExampleEngine_NewRuntime() {
	eng := gicel.NewEngine()

	rt, err := eng.NewRuntime(`
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
	eng := gicel.NewEngine()
	_, err := eng.NewRuntime(`main := x`)
	if err != nil {
		ce := err.(*gicel.CompileError)
		for _, d := range ce.Diagnostics() {
			fmt.Printf("%s:%d:%d: %s\n", d.Phase, d.Line, d.Col, d.Message)
		}
	}
	// Output: check:1:9: unbound variable: x
}
