package gomputation_test

import (
	"testing"

	gmp "github.com/cwd-k2/gomputation"
)

func TestSandboxRun(t *testing.T) {
	result, err := gmp.RunSandbox(`main := True`, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConVal(t, result.Value, "True")
}

func TestSandboxRunWithPacks(t *testing.T) {
	result, err := gmp.RunSandbox("import Std.Num\nmain := 1 + 2", &gmp.SandboxConfig{
		Packs: []gmp.Pack{gmp.Num},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostInt(t, result.Value, 3)
}

func TestSandboxRunTimeout(t *testing.T) {
	// This should not hang — step limit will catch infinite recursion.
	_, err := gmp.RunSandbox(`
import Std.Num
loop :: Int -> Int
loop := \n -> loop (n + 1)
main := loop 0
`, &gmp.SandboxConfig{
		Packs:    []gmp.Pack{gmp.Num},
		MaxSteps: 1000,
	})
	if err == nil {
		t.Fatal("expected error from infinite loop")
	}
}

func TestSandboxRunCompileError(t *testing.T) {
	_, err := gmp.RunSandbox(`main := x`, nil)
	if err == nil {
		t.Fatal("expected compile error")
	}
}

func TestSandboxRunJSON(t *testing.T) {
	result, err := gmp.RunSandbox(`main := True`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stats.Steps == 0 {
		t.Fatal("expected non-zero steps")
	}
}

func TestSandboxRunAllPacks(t *testing.T) {
	result, err := gmp.RunSandbox("import Std.Num\nimport Std.Str\nimport Std.List\nmain := showInt (foldl (\\acc -> \\x -> acc + x) 0 (Cons 1 (Cons 2 (Cons 3 Nil))))", &gmp.SandboxConfig{
		Packs: []gmp.Pack{gmp.Num, gmp.Str, gmp.List},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostString(t, result.Value, "6")
}
