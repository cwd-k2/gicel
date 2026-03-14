package gicel_test

import (
	"testing"

	"github.com/cwd-k2/gicel"
)

func TestSandboxRun(t *testing.T) {
	result, err := gicel.RunSandbox(`main := True`, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConVal(t, result.Value, "True")
}

func TestSandboxRunWithPacks(t *testing.T) {
	result, err := gicel.RunSandbox("import Std.Num\nmain := 1 + 2", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Num},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostInt(t, result.Value, 3)
}

func TestSandboxRunTimeout(t *testing.T) {
	// This should not hang — step limit will catch infinite recursion.
	_, err := gicel.RunSandbox(`
import Std.Num
loop :: Int -> Int
loop := \n -> loop (n + 1)
main := loop 0
`, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Num},
		MaxSteps: 1000,
	})
	if err == nil {
		t.Fatal("expected error from infinite loop")
	}
}

func TestSandboxRunCompileError(t *testing.T) {
	_, err := gicel.RunSandbox(`main := x`, nil)
	if err == nil {
		t.Fatal("expected compile error")
	}
}

func TestSandboxRunJSON(t *testing.T) {
	result, err := gicel.RunSandbox(`main := True`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stats.Steps == 0 {
		t.Fatal("expected non-zero steps")
	}
}

func TestSandboxDeepNesting(t *testing.T) {
	// Deeply nested expression — parser depth limit should catch this.
	src := "main := "
	for range 500 {
		src += "("
	}
	src += "True"
	for range 500 {
		src += ")"
	}
	_, err := gicel.RunSandbox(src, nil)
	if err == nil {
		t.Fatal("expected error from deeply nested expression")
	}
}

func TestSandboxMalformedDo(t *testing.T) {
	// Malformed do-block that previously caused infinite loop.
	_, err := gicel.RunSandbox("main := do { let x = 42; pure x }", nil)
	if err == nil {
		t.Fatal("expected error from invalid do-block")
	}
}

func TestSandboxDeepRecursion(t *testing.T) {
	// Deep recursion — depth limit should catch.
	_, err := gicel.RunSandbox(`
import Std.Num
f :: Int -> Int
f := \n -> f (n + 1)
main := f 0
`, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Num},
		MaxSteps: 500,
		MaxDepth: 50,
	})
	if err == nil {
		t.Fatal("expected error from deep recursion")
	}
}

func TestSandboxRunAllPacks(t *testing.T) {
	result, err := gicel.RunSandbox("import Std.Num\nimport Std.Str\nimport Std.List\nmain := showInt (foldl (\\acc -> \\x -> acc + x) 0 (Cons 1 (Cons 2 (Cons 3 Nil))))", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Num, gicel.Str, gicel.List},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostString(t, result.Value, "6")
}
