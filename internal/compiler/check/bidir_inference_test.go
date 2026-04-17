// Bidir inference tests — data declarations, identity, application, literals, type aliases, host bindings.
// Does NOT cover: unification (unify_test.go), exhaustiveness (exhaustiveness_test.go), errors (checker_error_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func TestCheckDataDecl(t *testing.T) {
	prog := checkSource(t, "form Bool := { True: Bool; False: Bool; }", nil)
	if len(prog.DataDecls) != 1 {
		t.Fatalf("expected 1 data decl, got %d", len(prog.DataDecls))
	}
	if prog.DataDecls[0].Name != "Bool" {
		t.Errorf("expected Bool, got %s", prog.DataDecls[0].Name)
	}
}

func TestCheckIdentity(t *testing.T) {
	source := `id := \x. x`
	prog := checkSource(t, source, nil)
	if len(prog.Bindings) != 1 || prog.Bindings[0].Name != "id" {
		t.Fatal("expected binding 'id'")
	}
	_, ok := prog.Bindings[0].Expr.(*ir.Lam)
	if !ok {
		t.Errorf("expected Lam, got %T", prog.Bindings[0].Expr)
	}
}

func TestCheckApplication(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
id := \x. x
main := id True`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

func TestCheckAssumption(t *testing.T) {
	config := &CheckConfig{
		Assumptions: map[string]types.Type{
			"dbOpen": types.MkArrow(types.MkCon("Unit"), types.MkCon("Unit")),
		},
	}
	source := `form Unit := { Unit: Unit; }
dbOpen := assumption`
	prog := checkSource(t, source, config)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "dbOpen" {
			if _, ok := b.Expr.(*ir.PrimOp); ok {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected PrimOp for dbOpen")
	}
}

func TestInferIntLit(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	prog := checkSource(t, `main := 42`, config)
	if len(prog.Bindings) != 1 {
		t.Fatal("expected 1 binding")
	}
	lit, ok := prog.Bindings[0].Expr.(*ir.Lit)
	if !ok {
		t.Fatalf("expected Lit, got %T", prog.Bindings[0].Expr)
	}
	if lit.Value != int64(42) {
		t.Errorf("expected 42, got %v", lit.Value)
	}
}

func TestInferStrLit(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"String": types.TypeOfTypes},
	}
	prog := checkSource(t, `main := "hello"`, config)
	lit, ok := prog.Bindings[0].Expr.(*ir.Lit)
	if !ok {
		t.Fatalf("expected Lit, got %T", prog.Bindings[0].Expr)
	}
	if lit.Value != "hello" {
		t.Errorf("expected hello, got %v", lit.Value)
	}
}

func TestInferRuneLit(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Rune": types.TypeOfTypes},
	}
	prog := checkSource(t, "main := 'a'", config)
	lit, ok := prog.Bindings[0].Expr.(*ir.Lit)
	if !ok {
		t.Fatalf("expected Lit, got %T", prog.Bindings[0].Expr)
	}
	if lit.Value != rune('a') {
		t.Errorf("expected 'a', got %v", lit.Value)
	}
}

func TestCheckLitMismatch(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes, "String": types.TypeOfTypes},
	}
	checkSourceExpectCode(t, `main := (42 :: String)`, config, diagnostic.ErrTypeMismatch)
}

func TestCheckDoBlock(t *testing.T) {
	source := `form Unit := { Unit: Unit; }
main := do { pure Unit }`
	prog := checkSource(t, source, nil)
	if len(prog.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(prog.Bindings))
	}
}

func TestCheckTypeAlias(t *testing.T) {
	// Test with inferred Computation type via pure.
	source := `form Unit := { Unit: Unit; }
main := pure Unit`
	prog := checkSource(t, source, nil)
	if len(prog.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(prog.Bindings))
	}
}

func TestCheckHostBinding(t *testing.T) {
	config := &CheckConfig{
		Bindings:        map[string]types.Type{"x": types.MkCon("Int")},
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	source := `y := x`
	prog := checkSource(t, source, config)
	if len(prog.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(prog.Bindings))
	}
}
