// MonoLocalBinds regression tests — Bind boundary dict scope.
// Does NOT cover: general VM tests (engine_vm_test.go).
package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

// TestMonoLocalBinds_FromListBind verifies that binding a fromList result
// in a block expression correctly resolves type family equations.
// Regression: fromList in a Bind caused non-exhaustive pattern match because
// localLetGen over-generalized the binding, orphaning the Elem type family
// equation that links (Int, String) to Map key/value types.
func TestMonoLocalBinds_FromListBind(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.Map)

	src := `import Prelude
import Data.Map
main := { m := fromList [(1, "a"), (2, "b")]; size m }`

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertVMInt(t, res, 2)
}

// TestMonoLocalBinds_FromListBindSingle verifies single-element fromList in a Bind.
func TestMonoLocalBinds_FromListBindSingle(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.Map)

	src := `import Prelude
import Data.Map
main := { m := fromList [(1, "a")]; size m }`

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertVMInt(t, res, 1)
}

// TestMonoLocalBinds_FromListDirect verifies direct fromList (no Bind) still works.
func TestMonoLocalBinds_FromListDirect(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.Map)

	src := `import Prelude
import Data.Map
main := size (fromList [(1, "a"), (2, "b")])`

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertVMInt(t, res, 2)
}

// TestMonoLocalBinds_SetFromListBind verifies fromList with Set in a Bind.
func TestMonoLocalBinds_SetFromListBind(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.Set)

	src := `import Prelude
import Data.Set
main := { s := fromList [1, 2, 3]; size s }`

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertVMInt(t, res, 3)
}

// TestMonoLocalBinds_BindNoAssocType verifies that Bind with non-associated-type
// classes still generalizes correctly (e.g. compare, show).
func TestMonoLocalBinds_BindNoAssocType(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)

	src := `import Prelude
main := { x := 1 + 2; x }`

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertVMInt(t, res, 3)
}

// TestMonoLocalBinds_MultiBind verifies multiple Bind with fromList.
func TestMonoLocalBinds_MultiBind(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.Map)

	src := `import Prelude
import Data.Map
main := {
  m1 := fromList [(1, "a")];
  m2 := fromList [(2, "b"), (3, "c")];
  size m1 + size m2
}`

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertVMInt(t, res, 3)
}
