// Pattern binding and mutable collection stress tests — deep tuples, nested if-then-else, Effect.Map/Set at scale.
// Does NOT cover: type system, parsing, error paths.

package stress_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===========================================================================
// Mutable collections (Effect.Map, Effect.Set)
// ===========================================================================

// TestStressEffectMapLargeInsert inserts 500 entries into a mutable map
// via recursive do-block, then verifies the final size.
func TestStressEffectMapLargeInsert(t *testing.T) {
	const n = 500

	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectMap)
	eng.EnableRecursion()
	eng.SetStepLimit(10_000_000)
	eng.SetDepthLimit(100_000)
	eng.SetAllocLimit(100 * 1024 * 1024)

	source := fmt.Sprintf(`
import Prelude
import Effect.Map as MMap

insertRange :: Int -> Int -> MMap Int Int -> Computation { mmap: () | r } { mmap: () | r } ()
insertRange := fix (\self lo hi m.
  case lo == hi {
    True  => pure ();
    False => do { MMap.insert lo (lo * 10) m; self (lo + 1) hi m }
  })

main := do {
  m <- MMap.new;
  insertRange 0 %d m;
  s := MMap.size m;
  pure s
}
`, n)

	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MMap insert %d entries: steps=%d", n, result.Stats.Steps)

	got := gicel.MustHost[int64](result.Value)
	if got != int64(n) {
		t.Errorf("expected size %d, got %d", n, got)
	}
}

// TestStressEffectSetLargeInsert inserts 500 distinct elements into a
// mutable set via recursive do-block, then verifies the final size.
func TestStressEffectSetLargeInsert(t *testing.T) {
	const n = 500

	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectSet)
	eng.EnableRecursion()
	eng.SetStepLimit(10_000_000)
	eng.SetDepthLimit(100_000)
	eng.SetAllocLimit(100 * 1024 * 1024)

	source := fmt.Sprintf(`
import Prelude
import Effect.Set as MSet

insertRange :: Int -> Int -> MSet Int -> Computation { mset: () | r } { mset: () | r } ()
insertRange := fix (\self lo hi s.
  case lo == hi {
    True  => pure ();
    False => do { MSet.insert lo s; self (lo + 1) hi s }
  })

main := do {
  s <- MSet.new;
  insertRange 0 %d s;
  n := MSet.size s;
  pure n
}
`, n)

	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MSet insert %d elements: steps=%d", n, result.Stats.Steps)

	got := gicel.MustHost[int64](result.Value)
	if got != int64(n) {
		t.Errorf("expected size %d, got %d", n, got)
	}
}

// ===========================================================================
// Deep tuple pattern binds
// ===========================================================================

// TestStressDeepTuplePatternBind destructures a 10-level nested tuple
// and verifies every extracted binding holds the correct value.
func TestStressDeepTuplePatternBind(t *testing.T) {
	// Build: nested := (1, (2, (3, (4, (5, (6, (7, (8, (9, 10)))))))))
	// Bind:  (a, (b, (c, (d, (e, (f, (g, (h, (i, j))))))))) := nested
	// Check: a + b + c + d + e + f + g + h + i + j == 55

	source := `
import Prelude

nested := (1, (2, (3, (4, (5, (6, (7, (8, (9, 10)))))))))

main := {
  (a, (b, (c, (d, (e, (f, (g, (h, (i, j))))))))) := nested;
  a + b + c + d + e + f + g + h + i + j
}
`
	result, err := gicel.RunSandbox(source, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 500_000,
	})
	if err != nil {
		t.Fatal(err)
	}

	got := gicel.MustHost[int64](result.Value)
	if got != 55 {
		t.Errorf("expected 55, got %d", got)
	}
}

// ===========================================================================
// Deep if-then-else nesting
// ===========================================================================

// TestStressDeepIfThenElse generates 20 levels of nested if-then-else
// where every condition is True, and verifies the correct innermost
// "then" leaf is reached (not any "else" sentinel).
func TestStressDeepIfThenElse(t *testing.T) {
	const depth = 20

	// Build:
	//   if True then (if True then (... if True then 42 else 900019) ... else 900001) else 900000
	//
	// Each "else" branch holds a distinct sentinel (900000 + level).
	// Since all conditions are True, only the innermost 42 is returned.

	var sb strings.Builder
	sb.WriteString("import Prelude\nmain := ")
	for range depth {
		sb.WriteString("if True then (")
	}
	sb.WriteString("42")
	for i := depth - 1; i >= 0; i-- {
		sb.WriteString(fmt.Sprintf(") else %d", 900000+i))
	}
	sb.WriteString("\n")

	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 500_000,
	})
	if err != nil {
		t.Fatal(err)
	}

	got := gicel.MustHost[int64](result.Value)
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

// ===========================================================================
// Many pattern binds in a block
// ===========================================================================

// TestStressManyPatternBindsInBlock creates a block with 50 tuple
// pattern bindings in sequence, then sums extracted components.
func TestStressManyPatternBindsInBlock(t *testing.T) {
	const n = 50

	// Generate:
	//   main := {
	//     (a0, b0) := (1, 2);
	//     (a1, b1) := (3, 4);
	//     ...
	//     a0 + b0 + a1 + b1 + ...
	//   }
	//
	// Each pair is ((2*i+1), (2*i+2)), so the total is sum(1..2*n).

	var sb strings.Builder
	sb.WriteString("import Prelude\nmain := {\n")
	for i := range n {
		sb.WriteString(fmt.Sprintf("  (a%d, b%d) := (%d, %d);\n", i, i, 2*i+1, 2*i+2))
	}
	// Sum all bindings.
	sb.WriteString("  ")
	for i := range n {
		if i > 0 {
			sb.WriteString(" + ")
		}
		sb.WriteString(fmt.Sprintf("a%d + b%d", i, i))
	}
	sb.WriteString("\n}\n")

	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}

	// sum(1..100) = 100*101/2 = 5050
	expected := int64(2*n) * int64(2*n+1) / 2
	got := gicel.MustHost[int64](result.Value)
	if got != expected {
		t.Errorf("expected %d, got %d", expected, got)
	}
}

// ===========================================================================
// Many pattern binds in a do-block
// ===========================================================================

// TestStressManyPatternBindsInDo creates a do-block with 30 sequential
// monadic pattern binds using `<- pure (x, y)`, then returns the total.
func TestStressManyPatternBindsInDo(t *testing.T) {
	const n = 30

	// Generate:
	//   main := do {
	//     (x0, y0) <- pure (1, 2);
	//     (x1, y1) <- pure (3, 4);
	//     ...
	//     pure (x0 + y0 + x1 + y1 + ...)
	//   }

	var sb strings.Builder
	sb.WriteString("import Prelude\nmain := do {\n")
	for i := range n {
		sb.WriteString(fmt.Sprintf("  (x%d, y%d) <- pure (%d, %d);\n", i, i, 2*i+1, 2*i+2))
	}
	sb.WriteString("  pure (")
	for i := range n {
		if i > 0 {
			sb.WriteString(" + ")
		}
		sb.WriteString(fmt.Sprintf("x%d + y%d", i, i))
	}
	sb.WriteString(")\n}\n")

	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}

	// sum(1..60) = 60*61/2 = 1830
	expected := int64(2*n) * int64(2*n+1) / 2
	got := gicel.MustHost[int64](result.Value)
	if got != expected {
		t.Errorf("expected %d, got %d", expected, got)
	}
}
